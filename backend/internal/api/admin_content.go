package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mathwiki/internal/model"
	"strings"
	"time"
	"unicode/utf8"
)

// Older community submissions were marked published before review existed.
// Filter them too, without changing or deleting the stored content.
func publicArticles(q *gorm.DB) *gorm.DB {
	return q.Where("published = ? AND review_status NOT IN ?", true, []string{"community_pending", "rejected"})
}
func (s *Server) publicArticleIDs() *gorm.DB {
	return s.db.Model(&model.Article{}).Scopes(publicArticles).Select("id")
}

func (s *Server) adminArticles(c *gin.Context) {
	status := c.DefaultQuery("status", "pending")
	q := s.db.Model(&model.Article{})
	switch status {
	case "pending":
		q = q.Where("review_status = ?", "community_pending")
	case "unreviewed":
		q = q.Where("review_status NOT IN ?", []string{"community_pending", "approved", "rejected"})
	case "approved", "rejected":
		q = q.Where("review_status = ?", status)
	case "deleted":
		q = q.Unscoped().Where("deleted_at IS NOT NULL")
	default:
		c.JSON(400, gin.H{"error": "条目审核状态不正确"})
		return
	}
	if keyword := strings.TrimSpace(c.Query("q")); keyword != "" {
		q = q.Where("title LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	var items []model.Article
	if err := q.Order("updated_at DESC, id DESC").Offset((positiveInt(c.Query("page"), 1) - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": s.articleList(c, items), "total": total})
}
func (s *Server) adminArticle(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "条目编号不正确"})
		return
	}
	var item model.Article
	if err := s.db.Unscoped().Preload("Sections", func(q *gorm.DB) *gorm.DB { return q.Order("position ASC") }).First(&item, id).Error; err != nil {
		respondDB(c, err)
		return
	}
	result := toArticleResponse(item, false)
	if item.DeletedAt.Valid {
		result.Published = false
	}
	c.JSON(200, result)
}
func (s *Server) reviewArticle(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	var input struct {
		Status         string `json:"status"`
		ExpectedStatus string `json:"expectedStatus"`
	}
	if !ok || c.ShouldBindJSON(&input) != nil || (input.Status != "approved" && input.Status != "rejected") || input.ExpectedStatus == "" {
		c.JSON(400, gin.H{"error": "请选择通过或驳回，并刷新条目状态"})
		return
	}
	result := s.db.Model(&model.Article{}).Where("id = ? AND review_status = ?", id, input.ExpectedStatus).Updates(map[string]any{"review_status": input.Status, "published": input.Status == "approved"})
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "条目已被处理或删除，请刷新列表"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) deleteArticle(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "条目编号不正确"})
		return
	}
	// Soft delete only. Dependent content is preserved, but public routes gate on the article.
	result := s.db.Delete(&model.Article{}, id)
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "条目不存在或已删除"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) restoreArticle(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "条目编号不正确"})
		return
	}
	result := s.db.Unscoped().Model(&model.Article{}).Where("id = ? AND deleted_at IS NOT NULL", id).Update("deleted_at", nil)
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "条目不在回收站中，请刷新列表"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

type feedbackResponse struct {
	model.Feedback
	Author       string `json:"author"`
	ArticleTitle string `json:"articleTitle"`
}

func (s *Server) feedbackList(c *gin.Context) {
	q := s.db.Model(&model.Feedback{})
	if !strings.HasPrefix(c.FullPath(), "/api/v1/admin/") {
		q = q.Where("user_id = ?", userID(c))
	}
	status := c.DefaultQuery("status", "pending")
	if status != "all" {
		if status != "pending" && status != "resolved" && status != "dismissed" {
			c.JSON(400, gin.H{"error": "反馈状态不正确"})
			return
		}
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	var items []model.Feedback
	if err := q.Order("created_at DESC, id DESC").Offset((positiveInt(c.Query("page"), 1) - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	userIDs, articleIDs := []uint{}, []uint{}
	for _, item := range items {
		userIDs = append(userIDs, item.UserID)
		if item.ArticleID != nil {
			articleIDs = append(articleIDs, *item.ArticleID)
		}
	}
	var users []model.User
	var articles []model.Article
	if len(userIDs) > 0 {
		if err := s.db.Select("id", "name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			fail(c, err)
			return
		}
	}
	if len(articleIDs) > 0 {
		if err := s.db.Unscoped().Select("id", "title").Where("id IN ?", articleIDs).Find(&articles).Error; err != nil {
			fail(c, err)
			return
		}
	}
	names, titles := map[uint]string{}, map[uint]string{}
	for _, u := range users {
		names[u.ID] = u.Name
	}
	for _, a := range articles {
		titles[a.ID] = a.Title
	}
	result := make([]feedbackResponse, 0, len(items))
	for _, item := range items {
		title := ""
		if item.ArticleID != nil {
			title = titles[*item.ArticleID]
		}
		result = append(result, feedbackResponse{Feedback: item, Author: names[item.UserID], ArticleTitle: title})
	}
	c.JSON(200, gin.H{"items": result, "total": total})
}
func (s *Server) reviewFeedback(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	var input struct {
		Status     string `json:"status"`
		Resolution string `json:"resolution"`
	}
	if !ok || c.ShouldBindJSON(&input) != nil || (input.Status != "resolved" && input.Status != "dismissed") {
		c.JSON(400, gin.H{"error": "请选择已处理或不采纳"})
		return
	}
	input.Resolution = strings.TrimSpace(input.Resolution)
	if input.Resolution == "" || utf8.RuneCountInString(input.Resolution) > 2000 {
		c.JSON(400, gin.H{"error": "请填写处理说明（1–2000 字），提交者可以查看"})
		return
	}
	result := s.db.Model(&model.Feedback{}).Where("id = ? AND status = ?", id, "pending").Updates(map[string]any{"status": input.Status, "resolution": input.Resolution, "reviewer_id": userID(c), "reviewed_at": time.Now()})
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "反馈已处理或不存在，请刷新列表"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
