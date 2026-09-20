package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mathwiki/internal/model"
	"strings"
	"unicode/utf8"
)

func (s *Server) annotationScope(c *gin.Context) *gorm.DB {
	q := s.db.Model(&model.Annotation{})
	if isAdmin(c) {
		return q
	}
	if userID(c) > 0 {
		return q.Where("(status = ? OR user_id = ?)", "visible", userID(c))
	}
	return q.Where("status = ?", "visible")
}
func (s *Server) moderationQueue(c *gin.Context) {
	status := c.DefaultQuery("status", "pending")
	if status != "pending" && status != "visible" && status != "rejected" {
		c.JSON(400, gin.H{"error": "审核状态不正确"})
		return
	}
	var items []model.Annotation
	q := s.db.Model(&model.Annotation{}).Where("status = ? AND article_id IN (?)", status, s.publicArticleIDs())
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	if err := q.Preload("User").Order("created_at ASC").Offset((positiveInt(c.Query("page"), 1) - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	result := make([]annotationResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toAnnotationResponse(item))
	}
	c.JSON(200, gin.H{"items": result, "total": total})
}
func (s *Server) reviewAnnotation(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	var input struct {
		Status string `json:"status"`
	}
	if !ok || c.ShouldBindJSON(&input) != nil || (input.Status != "visible" && input.Status != "rejected") {
		c.JSON(400, gin.H{"error": "请选择通过或驳回"})
		return
	}
	result := s.db.Model(&model.Annotation{}).Where("id = ? AND status = ? AND article_id IN (?)", id, "pending", s.publicArticleIDs()).Update("status", input.Status)
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "批注已被处理或已删除，请刷新列表"})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) deleteAnnotation(c *gin.Context) {
	var item model.Annotation
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "编号不正确"})
		return
	}
	if err := s.db.First(&item, id).Error; err != nil {
		respondDB(c, err)
		return
	}
	if !isAdmin(c) && item.UserID != userID(c) {
		c.JSON(403, gin.H{"error": "只能删除自己的批注"})
		return
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("annotation_id = ?", id).Delete(&model.Comment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&item).Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) deleteComment(c *gin.Context) {
	var item model.Comment
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "编号不正确"})
		return
	}
	if err := s.db.First(&item, id).Error; err != nil {
		respondDB(c, err)
		return
	}
	if !isAdmin(c) && item.UserID != userID(c) {
		c.JSON(403, gin.H{"error": "只能删除自己的回复"})
		return
	}
	if err := s.db.Delete(&item).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) publishedArticle(c *gin.Context) (uint, bool) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "条目编号不正确"})
		return 0, false
	}
	var a model.Article
	if err := s.db.Scopes(publicArticles).Where("id = ?", id).First(&a).Error; err != nil {
		respondDB(c, err)
		return 0, false
	}
	return id, true
}
func articleCommentDTO(item model.ArticleComment) commentResponse {
	return commentResponse{ID: item.ID, Avatar: item.User.Avatar, UserID: item.UserID, Body: item.Body, Author: item.User.Name, CreatedAt: item.CreatedAt}
}
func (s *Server) listArticleComments(c *gin.Context) {
	id, ok := s.publishedArticle(c)
	if !ok {
		return
	}
	q := s.db.Model(&model.ArticleComment{}).Where("article_id = ?", id)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	var items []model.ArticleComment
	if err := q.Preload("User").Order("created_at DESC, id DESC").Offset((positiveInt(c.Query("page"), 1) - 1) * 30).Limit(30).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	result := make([]commentResponse, 0, len(items))
	for _, item := range items {
		result = append(result, articleCommentDTO(item))
	}
	c.JSON(200, gin.H{"items": result, "total": total})
}
func (s *Server) createArticleComment(c *gin.Context) {
	id, ok := s.publishedArticle(c)
	if !ok {
		return
	}
	var input struct {
		Body string `json:"body"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "评论格式不正确"})
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" || utf8.RuneCountInString(input.Body) > 2000 {
		c.JSON(400, gin.H{"error": "评论应为 1 到 2000 个字"})
		return
	}
	item := model.ArticleComment{ArticleID: id, UserID: userID(c), Body: input.Body}
	if err := s.db.Create(&item).Error; err != nil {
		fail(c, err)
		return
	}
	item.User = currentUser(c)
	c.JSON(201, articleCommentDTO(item))
}
func (s *Server) deleteArticleComment(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "编号不正确"})
		return
	}
	var item model.ArticleComment
	if err := s.db.First(&item, id).Error; err != nil {
		respondDB(c, err)
		return
	}
	if !isAdmin(c) && item.UserID != userID(c) {
		c.JSON(403, gin.H{"error": "只能删除自己的评论"})
		return
	}
	if err := s.db.Delete(&item).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
