package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"mathwiki/internal/rag"
)

func (s *Server) explainStream(c *gin.Context) {
	var request struct {
		ArticleID uint         `json:"articleId"`
		Question  string       `json:"question"`
		Audience  string       `json:"audience"`
		Style     string       `json:"style"`
		History   []ai.Message `json:"history"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Question) == "" || utf8.RuneCountInString(request.Question) > 2000 || len(request.Audience) > 100 || len(request.Style) > 100 {
		c.JSON(400, gin.H{"error": "请输入有效的问题（最多 2000 字）"})
		return
	}
	if !validHistory(request.History) {
		c.JSON(400, gin.H{"error": "对话上下文过长或格式错误，请开始新对话"})
		return
	}
	var article model.Article
	if err := s.db.Scopes(publicArticles).Select("id", "title", "slug", "lead").Preload("Sections", preloadEvidence).Where("id = ? AND published = ?", request.ArticleID, true).First(&article).Error; err != nil {
		respondDB(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	var evidence []rag.Evidence
	status := rag.Status{Mode: "current_article", Reason: "disabled"}
	emit := func(event ai.StreamEvent) error {
		if err := c.Request.Context().Err(); err != nil {
			return err
		}
		if event.Type == "done" {
			if err := decorateEvidence(event.Explanation, evidence, status); err != nil {
				return err
			}
		}
		data, _ := json.Marshal(event)
		if _, err := c.Writer.Write(append(append([]byte("data: "), data...), []byte("\n\n")...)); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	sources := article
	if s.explainer.Configured() {
		if s.rag.Configured() {
			if err := emit(ai.StreamEvent{Type: "phase", Text: "retrieving"}); err != nil {
				return
			}
		}
		sources, evidence, status = s.retrieveEvidence(c.Request.Context(), article, request.Question)
	}
	result, err := s.explainer.Stream(c.Request.Context(), sources, strings.TrimSpace(request.Question), request.Audience, request.Style, request.History, emit)
	if err != nil {
		if c.Request.Context().Err() == nil {
			_ = emit(ai.StreamEvent{Type: "error", Text: "回答未完成，请重试。已显示的内容尚未完成核验。"})
		}
		return
	}
	encoded, _ := json.Marshal(result)
	_ = s.db.Create(&model.ExplanationRecord{ArticleID: article.ID, UserID: userID(c), Question: request.Question, Audience: request.Audience, Style: request.Style, AnswerJSON: string(encoded), Model: s.explainer.Model(), Confidence: result.Confidence}).Error
}

func validHistory(history []ai.Message) bool {
	if len(history) > 12 || len(history)%2 != 0 {
		return false
	}
	total := 0
	for i, message := range history {
		expected := "user"
		if i%2 == 1 {
			expected = "assistant"
		}
		size := utf8.RuneCountInString(message.Content)
		if message.Role != expected || size == 0 || size > 8000 {
			return false
		}
		total += size
	}
	return total <= 32000
}
