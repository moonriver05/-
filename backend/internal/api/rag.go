package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"mathwiki/internal/rag"
	"strings"
)

func preloadEvidence(q *gorm.DB) *gorm.DB {
	return q.Select("id, article_id, position, SUBSTR(title, 1, 255) AS title, SUBSTR(content, 1, 12000) AS content, hidden").Where("hidden = ? AND position < ?", false, 1000000).Order("position ASC, id ASC").Limit(60)
}

func (s *Server) retrieveEvidence(ctx context.Context, article model.Article, question string) (model.Article, []rag.Evidence, rag.Status) {
	evidence, status := s.rag.Retrieve(ctx, s.db, article, question)
	return rag.SourceArticle(article, evidence), evidence, status
}

func decorateEvidence(answer *ai.Explanation, evidence []rag.Evidence, status rag.Status) error {
	if answer == nil {
		return errors.New("missing answer")
	}
	answer.Retrieval = &ai.Retrieval{Mode: status.Mode, Reason: status.Reason, Count: status.Count}
	if answer.Mode != "model" || answer.Kind == "conversation" {
		return nil
	}
	valid := make([]ai.Citation, 0, len(answer.Citations))
	for _, citation := range answer.Citations {
		var index int
		if _, err := fmt.Sscanf(strings.TrimSpace(citation.Section), "S%d", &index); err != nil || index < 1 || index > len(evidence) {
			continue
		}
		source := evidence[index-1]
		quote := strings.TrimSpace(citation.Quote)
		if quote == "" || !strings.Contains(source.Content, quote) {
			continue
		}
		// Ignore model-supplied IDs/slugs. Only server-validated source metadata is used.
		valid = append(valid, ai.Citation{Section: fmt.Sprintf("S%d｜%s｜%s", index, source.ArticleTitle, source.SectionTitle), Quote: quote, ArticleID: source.ArticleID, ArticleSlug: source.Slug, SectionID: source.SectionID})
	}
	if len(valid) == 0 {
		return errors.New("answer contains no exact source quotations")
	}
	answer.Citations = valid
	return nil
}

func (s *Server) ragStatus(c *gin.Context) {
	if !s.rag.Configured() {
		c.JSON(200, gin.H{"configured": false, "ready": false})
		return
	}
	count, err := s.rag.Count(c.Request.Context())
	c.JSON(200, gin.H{"configured": true, "ready": err == nil && count > 0, "chunks": count, "dimensions": rag.Dimensions, "model": s.rag.Model})
}
