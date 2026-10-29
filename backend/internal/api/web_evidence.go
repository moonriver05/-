package api

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"mathwiki/internal/rag"
	"mathwiki/internal/websearch"
)

func (s *Server) answerOptions(c *gin.Context, mode string, search bool) (string, bool) {
	if mode == "" {
		mode = ai.StrictAnswer
	}
	if mode != ai.BalancedAnswer && mode != ai.StrictAnswer {
		c.JSON(400, gin.H{"error": "请选择有效的回答范围"})
		return "", false
	}
	if search && (!s.explainer.Configured() || !s.webSearch.Configured()) {
		c.JSON(400, gin.H{"error": "服务器尚未开启 DeepSeek 联网搜索"})
		return "", false
	}
	return mode, true
}

func webQuery(title, question string, history []ai.Message) string {
	text := question
	if followupReference.MatchString(question) {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == "user" {
				text += " " + retrievalExcerpt(history[i].Content, 60)
				break
			}
		}
		text += " " + retrievalExcerpt(title, 32)
	}
	return retrievalExcerpt(strings.Join(strings.Fields(text), " "), 128)
}

func (s *Server) addWebEvidence(ctx context.Context, article model.Article, evidence []rag.Evidence, query, question string) (model.Article, []rag.Evidence, []websearch.Result, *ai.WebSearch) {
	results, err := s.webSearch.Search(ctx, query, question)
	info := &ai.WebSearch{Status: "ok", Count: len(results)}
	if err != nil {
		info.Status = "failed"
	} else if len(results) == 0 {
		info.Status = "empty"
	}
	for _, item := range results {
		info.Sources = append(info.Sources, ai.WebSource{Title: item.Title, URL: item.URL, SummaryOnly: item.SummaryOnly})
		kind := "网页正文节选"
		if item.SummaryOnly {
			kind = "搜索摘要（非完整原文）"
		}
		evidence = append(evidence, rag.Evidence{ArticleTitle: item.Title, SectionTitle: kind, Content: item.Content})
	}
	return rag.SourceArticle(article, evidence), evidence, results, info
}
