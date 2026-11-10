package api

import (
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mathwiki/internal/model"
)

func (s *Server) appendArticleSupplement(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if !ok || c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "补充格式不正确"})
		return
	}
	title := strings.TrimSpace(input.Title)
	content := strings.TrimSpace(input.Content)
	if title == "" || content == "" || utf8.RuneCountInString(title) > 255 || utf8.RuneCountInString(content) > 100000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请填写小节标题和正文，标题最多 255 字，正文最多 10 万字"})
		return
	}

	var section model.Section
	err := s.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		// Serialize appends on the parent. Imported articles have their sections
		// replaced by the importer, so appends are available on community articles.
		var article model.Article
		if err := tx.Scopes(publicArticles).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("page_id < 0").Select("id").First(&article, id).Error; err != nil {
			return err
		}
		var lastPosition int
		if err := tx.Model(&model.Section{}).Where("article_id = ?", id).
			Select("COALESCE(MAX(position), 0)").Scan(&lastPosition).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		section = model.Section{
			ArticleID: id, Position: lastPosition + 1, Level: 2,
			Title: title, Content: content, Formulae: "[]", SupplementedAt: &now,
		}
		if err := tx.Create(&section).Error; err != nil {
			return err
		}
		return tx.Model(&article).Update("updated_at", now).Error
	})
	if err != nil {
		respondDB(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"section": toSectionResponse(section), "updatedAt": section.SupplementedAt})
}
