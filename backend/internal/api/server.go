package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mathwiki/internal/ai"
	"mathwiki/internal/mail"
	"mathwiki/internal/model"
	"mathwiki/internal/search"
	"mathwiki/internal/storage"
)

type Server struct {
	db          *gorm.DB
	search      *search.Client
	explainer   *ai.Explainer
	sttProvider string
	limiter     authLimiter
	mailer      mail.Sender
	avatars     storage.AvatarStore
}

func New(db *gorm.DB, searchClient *search.Client, explainer *ai.Explainer, sttProvider string) *Server {
	return &Server{db: db, search: searchClient, explainer: explainer, sttProvider: sttProvider, limiter: authLimiter{entries: make(map[string]attempt)}, mailer: mail.FromEnv(), avatars: storage.FromEnv()}
}

func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), cors())
	_ = r.SetTrustedProxies(nil)
	r.GET("/health", s.health)
	v1 := r.Group("/api/v1", s.authenticate, s.auditAdmin)
	v1.POST("/auth/reset-code", s.authRateLimit, s.sendPasswordResetCode)
	v1.POST("/auth/reset-password", s.authRateLimit, s.resetPassword)
	v1.GET("/admin/users", requireAdmin, s.adminUsers)
	v1.PATCH("/admin/users/:id", requireAdmin, s.banUser)
	v1.GET("/admin/audits", requireAdmin, s.adminAudits)
	v1.POST("/auth/register", s.authRateLimit, s.register)
	v1.POST("/auth/email-code", s.authRateLimit, s.sendRegistrationCode)
	v1.GET("/auth/config", func(c *gin.Context) {
		c.JSON(200, gin.H{"emailVerificationRequired": true, "emailConfigured": s.mailer.Configured()})
	})
	v1.POST("/auth/login", s.authRateLimit, s.login)
	v1.POST("/auth/logout", s.logout)
	v1.PUT("/auth/avatar", requireLogin, s.avatarRateLimit, s.uploadAvatar)
	v1.DELETE("/auth/avatar", requireLogin, s.resetAvatar)
	v1.PUT("/auth/profile", requireLogin, s.updateProfile)
	v1.GET("/auth/profile-config", requireLogin, func(c *gin.Context) { c.JSON(200, gin.H{"avatarConfigured": s.avatars.Configured()}) })
	v1.GET("/auth/me", func(c *gin.Context) {
		if userID(c) == 0 {
			c.JSON(200, gin.H{"user": nil})
			return
		}
		c.JSON(200, gin.H{"user": currentUser(c)})
	})
	v1.GET("/articles/:id/comments", s.listArticleComments)
	v1.POST("/articles/:id/comments", requireLogin, s.createArticleComment)
	v1.DELETE("/article-comments/:id", requireLogin, s.deleteArticleComment)
	v1.DELETE("/comments/:id", requireLogin, s.deleteComment)
	v1.DELETE("/annotations/:id", requireLogin, s.deleteAnnotation)
	v1.GET("/admin/annotations", requireAdmin, s.moderationQueue)
	v1.GET("/admin/articles", requireAdmin, s.adminArticles)
	v1.GET("/admin/articles/:id", requireAdmin, s.adminArticle)
	v1.PATCH("/admin/articles/:id", requireAdmin, s.reviewArticle)
	v1.DELETE("/admin/articles/:id", requireAdmin, s.deleteArticle)
	v1.POST("/admin/articles/:id/restore", requireAdmin, s.restoreArticle)
	v1.GET("/admin/feedback", requireAdmin, s.feedbackList)
	v1.PATCH("/admin/feedback/:id", requireAdmin, s.reviewFeedback)
	v1.GET("/feedback/mine", requireLogin, s.feedbackList)
	v1.PATCH("/admin/annotations/:id", requireAdmin, s.reviewAnnotation)
	{
		v1.GET("/stats", s.stats)
		v1.GET("/categories", s.listCategories)
		v1.POST("/categories", requireAdmin, s.createCategory)
		v1.GET("/capabilities", func(c *gin.Context) {
			c.JSON(200, gin.H{"aiConfigured": s.explainer.Configured(), "speechConfigured": false})
		})
		v1.GET("/articles", s.listArticles)
		v1.POST("/articles", requireAdmin, s.createArticle)
		v1.PUT("/articles/:id/read", requireLogin, s.markRead)
		v1.GET("/articles/:id", s.getArticle)
		v1.GET("/search", s.searchArticles)
		v1.PUT("/articles/:id/favorite", requireLogin, s.setFavorite)
		v1.GET("/articles/:id/annotations", s.listAnnotations)
		v1.POST("/annotations", requireLogin, s.createAnnotation)
		v1.POST("/annotations/:id/comments", requireLogin, s.createComment)
		v1.GET("/annotations/:id/comments", s.listComments)
		v1.POST("/annotations/:id/like", requireLogin, s.setAnnotationLike)
		v1.DELETE("/annotations/:id/like", requireLogin, s.setAnnotationLike)
		v1.POST("/explanations", requireLogin, s.explain)
		v1.POST("/explanations/stream", requireLogin, s.explainStream)
		v1.POST("/feedback", requireLogin, s.createFeedback)
		v1.POST("/speech/transcribe", requireLogin, s.transcribe)
	}
	return r
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Same-origin API via Vite/Nginx; foreign-origin credentialed requests are not allowed.
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-MathWiki-Request")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 1500*time.Millisecond)
	defer cancel()
	sqlDB, err := s.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "degraded", "database": "unavailable", "language": "zh-Hans"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "connected", "databaseDriver": s.db.Dialector.Name(), "language": "zh-Hans"})
}

func (s *Server) stats(c *gin.Context) {
	var articleCount, sectionCount, annotationCount, favoriteCount, recentCount int64
	if err := s.db.Model(&model.Article{}).Scopes(publicArticles).Where("language = ?", "zh-Hans").Count(&articleCount).Error; err != nil {
		fail(c, err)
		return
	}
	_ = s.db.Model(&model.Section{}).Where("hidden = ? AND article_id IN (?)", false, s.publicArticleIDs()).Count(&sectionCount).Error
	_ = s.db.Model(&model.Annotation{}).Where("user_id = ? AND article_id IN (?)", userID(c), s.publicArticleIDs()).Count(&annotationCount).Error
	_ = s.db.Model(&model.ReadingHistory{}).Where("user_id = ? AND article_id IN (?)", userID(c), s.publicArticleIDs()).Count(&recentCount).Error
	_ = s.db.Model(&model.Favorite{}).Where("user_id = ? AND article_id IN (?)", userID(c), s.publicArticleIDs()).Count(&favoriteCount).Error

	type categoryCount struct {
		Category string `json:"category"`
		Count    int64  `json:"count"`
	}
	categories := []categoryCount{}
	_ = s.db.Model(&model.Article{}).Scopes(publicArticles).
		Select("category, COUNT(*) AS count").
		Where("published = ? AND language = ?", true, "zh-Hans").
		Group("category").Order("count DESC").Limit(12).Scan(&categories).Error

	c.JSON(http.StatusOK, gin.H{
		"databaseDriver": s.db.Dialector.Name(),
		"articleCount":   articleCount, "sectionCount": sectionCount, "annotationCount": annotationCount,
		"favoriteCount": favoriteCount, "recentCount": recentCount, "categories": categories, "language": "zh-Hans",
	})
}

func (s *Server) listArticles(c *gin.Context) {
	var articles []model.Article
	query := s.db.Model(&model.Article{}).Scopes(publicArticles).Where("language = ?", "zh-Hans")
	if category := strings.TrimSpace(c.Query("category")); category != "" {
		query = categoryFilter(query, category)
	}
	if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
		query = query.Where("kind = ?", kind)
	}
	keyword := strings.TrimSpace(c.Query("q"))
	if keyword != "" {
		query = matchKnowledge(query, keyword)
	}
	switch c.Query("collection") {
	case "favorites":
		query = query.Where("id IN (?)", s.db.Model(&model.Favorite{}).Select("article_id").Where("user_id = ?", userID(c)))
	case "annotated":
		query = query.Where("id IN (?)", s.db.Model(&model.Annotation{}).Select("article_id").Where("user_id = ?", userID(c)))
	case "recent":
		query = query.Where("id IN (?)", s.db.Model(&model.ReadingHistory{}).Select("article_id").Where("user_id = ?", userID(c)))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}

	if keyword != "" && (c.Query("sort") == "" || c.Query("sort") == "relevance") {
		query = orderKnowledge(query, keyword)
	} else if c.Query("collection") == "recent" && keyword == "" {
		query = query.Clauses(clause.OrderBy{Expression: clause.Expr{SQL: "(SELECT MAX(updated_at) FROM reading_histories WHERE reading_histories.article_id = articles.id AND reading_histories.user_id = ?) DESC, articles.id ASC", Vars: []any{userID(c)}, WithoutParentheses: true}})
	} else {
		switch c.Query("sort") {
		case "title":
			query = query.Order("title ASC, id ASC")
		case "difficulty":
			query = query.Order("CASE difficulty WHEN '入门' THEN 0 WHEN '进阶' THEN 1 ELSE 2 END ASC, title ASC, id ASC")
		default:
			query = query.Order("updated_at DESC, id ASC")
		}
	}
	page := positiveInt(c.Query("page"), 1)
	pageSize := limit(c, 50)
	if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Find(&articles).Error; err != nil {
		fail(c, err)
		return
	}
	c.Header("X-Total-Count", strconv.FormatInt(total, 10))
	if c.Query("format") == "page" {
		c.JSON(200, gin.H{"items": s.articleList(c, articles), "total": total})
		return
	}
	c.JSON(http.StatusOK, s.articleList(c, articles))
}

func (s *Server) getArticle(c *gin.Context) {
	var article model.Article
	if err := s.db.Scopes(publicArticles).Preload("Sections", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Where("slug = ? AND published = ?", c.Param("id"), true).First(&article).Error; err != nil {
		respondDB(c, err)
		return
	}
	c.JSON(http.StatusOK, toArticleResponse(article, s.isFavorite(c, article.ID)))
}

func (s *Server) searchArticles(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, []articleResponse{})
		return
	}
	maxResults := limit(c, 30)
	if s.search != nil {
		if hits, err := s.search.Search(c.Request.Context(), q, maxResults); err == nil && len(hits) > 0 {
			pageIDs := make([]int64, 0, len(hits))
			for _, hit := range hits {
				pageIDs = append(pageIDs, hit.PageID)
			}
			var articles []model.Article
			if err := s.db.Scopes(publicArticles).Where("page_id IN ?", pageIDs).Find(&articles).Error; err == nil {
				byPageID := make(map[int64]articleResponse, len(articles))
				for _, item := range s.articleList(c, articles) {
					byPageID[item.PageID] = item
				}
				ordered := make([]articleResponse, 0, len(pageIDs))
				for _, pageID := range pageIDs {
					if item, ok := byPageID[pageID]; ok {
						ordered = append(ordered, item)
					}
				}
				c.JSON(http.StatusOK, ordered)
				return
			}
		}
	}

	var articles []model.Article
	query := matchKnowledge(s.db.Scopes(publicArticles).Where("language = ?", "zh-Hans"), q)
	if err := orderKnowledge(query, q).
		Limit(maxResults).Find(&articles).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, s.articleList(c, articles))
}

func (s *Server) setFavorite(c *gin.Context) {
	articleID, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "条目编号不正确"})
		return
	}
	var input struct {
		Saved *bool `json:"saved"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.Saved == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请提供收藏状态"})
		return
	}
	var count int64
	if err := s.db.Model(&model.Article{}).Scopes(publicArticles).Where("id = ?", articleID).Count(&count).Error; err != nil || count == 0 {
		if err != nil {
			fail(c, err)
		} else {
			c.JSON(http.StatusNotFound, gin.H{"error": "没有找到该中文条目"})
		}
		return
	}
	if *input.Saved {
		item := model.Favorite{UserID: userID(c), ArticleID: articleID}
		if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
			fail(c, err)
			return
		}
	} else if err := s.db.Where("user_id = ? AND article_id = ?", userID(c), articleID).Delete(&model.Favorite{}).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"articleId": articleID, "saved": *input.Saved})
}

func (s *Server) listAnnotations(c *gin.Context) {
	if _, ok := s.publishedArticle(c); !ok {
		return
	}
	articleID, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "条目编号不正确"})
		return
	}
	var items []model.Annotation
	if err := s.annotationScope(c).Preload("User").Preload("Comments", "status = ?", "visible").Where("article_id = ?", articleID).
		Order("created_at DESC").Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	result := make([]annotationResponse, 0, len(items))
	votes := []model.AnnotationLike{}
	if userID(c) != 0 {
		if err := s.db.Where("user_id = ? AND annotation_id IN (?)", userID(c), s.db.Model(&model.Annotation{}).Select("id").Where("article_id = ?", articleID)).Find(&votes).Error; err != nil {
			fail(c, err)
			return
		}
	}
	liked := map[uint]bool{}
	for _, vote := range votes {
		liked[vote.AnnotationID] = true
	}
	for _, item := range items {
		note := toAnnotationResponse(item)
		note.Liked = liked[item.ID]
		result = append(result, note)
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) createAnnotation(c *gin.Context) {
	var input struct {
		ArticleID   uint   `json:"articleId"`
		SectionID   *uint  `json:"sectionId"`
		Quote       string `json:"quote"`
		Prefix      string `json:"prefix"`
		Suffix      string `json:"suffix"`
		StartOffset int    `json:"startOffset"`
		EndOffset   int    `json:"endOffset"`
		Body        string `json:"body"`
		Color       string `json:"color"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批注格式不正确"})
		return
	}
	input.Quote = strings.TrimSpace(input.Quote)
	input.Body = strings.TrimSpace(input.Body)
	if input.ArticleID == 0 || input.Quote == "" || input.Body == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "条目、原文引用和批注内容不能为空"})
		return
	}
	if utf8.RuneCountInString(input.Quote) > 240 || utf8.RuneCountInString(input.Body) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "引用或批注内容过长"})
		return
	}
	var article model.Article
	if err := s.db.Scopes(publicArticles).Preload("Sections").Where("id = ?", input.ArticleID).First(&article).Error; err != nil {
		respondDB(c, err)
		return
	}
	if !quoteBelongsToArticle(article, input.Quote, input.SectionID) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "引用内容不属于当前条目"})
		return
	}
	color := input.Color
	if color == "" {
		color = "#e5a84b"
	}
	item := model.Annotation{
		ArticleID: input.ArticleID, SectionID: input.SectionID, UserID: userID(c), Quote: input.Quote,
		Prefix: input.Prefix, Suffix: input.Suffix, StartOffset: input.StartOffset, EndOffset: input.EndOffset,
		Body: input.Body, Color: color, Status: "pending",
	}
	if isAdmin(c) {
		item.Status = "visible"
	}
	if !s.createCommunity(c, &item, submissionKey("annotation", item.ArticleID, item.Quote+"\n"+item.Body)) {
		return
	}
	_ = s.db.Preload("User").First(&item, item.ID).Error
	c.JSON(http.StatusCreated, toAnnotationResponse(item))
}

func (s *Server) createComment(c *gin.Context) {
	annotationID, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "批注编号不正确"})
		return
	}
	var input struct {
		Body     string `json:"body"`
		ParentID *uint  `json:"parentId"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Body) == "" || utf8.RuneCountInString(input.Body) > 2000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "回复内容应为 1 到 2000 个字"})
		return
	}
	var count int64
	if err := s.db.Model(&model.Annotation{}).Where("id = ? AND status = ? AND article_id IN (?)", annotationID, "visible", s.publicArticleIDs()).Count(&count).Error; err != nil || count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "批注不存在"})
		return
	}
	item := model.Comment{AnnotationID: annotationID, UserID: userID(c), ParentID: input.ParentID, Body: strings.TrimSpace(input.Body), Status: "visible"}
	if input.ParentID != nil {
		var parent model.Comment
		if err := s.db.Where("id = ? AND annotation_id = ? AND status = ?", *input.ParentID, annotationID, "visible").First(&parent).Error; err != nil {
			c.JSON(400, gin.H{"error": "被回复的评论不属于当前批注"})
			return
		}
	}
	if !s.createCommunity(c, &item, submissionKey("reply", item.AnnotationID, item.Body)) {
		return
	}
	_ = s.db.Preload("User").First(&item, item.ID).Error
	c.JSON(http.StatusCreated, toCommentResponse(item))
}

func (s *Server) explain(c *gin.Context) {
	var request struct {
		ArticleID uint   `json:"articleId"`
		Question  string `json:"question"`
		Audience  string `json:"audience"`
		Style     string `json:"style"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入要解释的问题"})
		return
	}
	var article model.Article
	if err := s.db.Scopes(publicArticles).Preload("Sections", func(db *gorm.DB) *gorm.DB { return db.Order("position ASC") }).
		Where("id = ? AND published = ?", request.ArticleID, true).First(&article).Error; err != nil {
		respondDB(c, err)
		return
	}
	result, err := s.explainer.Explain(c.Request.Context(), article, request.Question, request.Audience, request.Style)
	if err != nil {
		fail(c, err)
		return
	}
	encoded, _ := json.Marshal(result)
	record := model.ExplanationRecord{
		ArticleID: article.ID, UserID: userID(c), Question: request.Question, Audience: request.Audience,
		Style: request.Style, AnswerJSON: string(encoded), Model: s.explainer.Model(), Confidence: result.Confidence,
	}
	_ = s.db.Create(&record).Error
	c.JSON(http.StatusOK, result)
}

func (s *Server) createFeedback(c *gin.Context) {
	var input struct {
		ArticleID *uint  `json:"articleId"`
		Kind      string `json:"kind"`
		Body      string `json:"body"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Kind) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "反馈格式不正确"})
		return
	}
	input.Body = strings.TrimSpace(input.Body)
	if (input.Kind != "content_suggestion" && input.Kind != "helpful" && input.Kind != "needs_improvement") || utf8.RuneCountInString(input.Body) > 4000 || (input.Kind == "content_suggestion" && input.Body == "") {
		c.JSON(400, gin.H{"error": "反馈类型不正确或内容超过 4000 字"})
		return
	}
	if input.ArticleID == nil {
		c.JSON(400, gin.H{"error": "请选择反馈关联的条目"})
		return
	}
	var article model.Article
	if err := s.db.Scopes(publicArticles).First(&article, *input.ArticleID).Error; err != nil {
		respondDB(c, err)
		return
	}
	item := model.Feedback{UserID: userID(c), ArticleID: input.ArticleID, Kind: input.Kind, Body: input.Body, Status: "pending"}
	if err := s.db.Create(&item).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (s *Server) transcribe(c *gin.Context) {
	file, err := c.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少音频文件"})
		return
	}
	if file.Size > 20<<20 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "音频不能超过 20 MB"})
		return
	}
	if s.sttProvider == "mock" {
		c.JSON(http.StatusOK, gin.H{"text": "请用一个生活中的例子解释这个算法", "provider": "mock"})
		return
	}
	c.JSON(http.StatusNotImplemented, gin.H{"error": "请配置腾讯云或火山引擎语音转写凭据", "provider": s.sttProvider})
}

func (s *Server) articleList(c *gin.Context, articles []model.Article) []articleResponse {
	ids := make([]uint, 0, len(articles))
	for _, article := range articles {
		ids = append(ids, article.ID)
	}
	favorites := map[uint]bool{}
	if len(ids) > 0 {
		var favoriteRows []model.Favorite
		_ = s.db.Where("user_id = ? AND article_id IN ?", userID(c), ids).Find(&favoriteRows).Error
		for _, item := range favoriteRows {
			favorites[item.ArticleID] = true
		}
	}
	result := make([]articleResponse, 0, len(articles))
	for _, article := range articles {
		result = append(result, toArticleResponse(article, favorites[article.ID]))
	}
	return result
}

func (s *Server) isFavorite(c *gin.Context, articleID uint) bool {
	var count int64
	_ = s.db.Model(&model.Favorite{}).Where("user_id = ? AND article_id = ?", userID(c), articleID).Count(&count).Error
	return count > 0
}

func quoteBelongsToArticle(article model.Article, quote string, sectionID *uint) bool {
	if sectionID != nil {
		for _, section := range article.Sections {
			if section.ID == *sectionID {
				return strings.Contains(section.Content, quote)
			}
		}
		return false
	}
	if strings.Contains(article.PlainText, quote) || strings.Contains(article.Lead, quote) {
		return true
	}
	for _, section := range article.Sections {
		if strings.Contains(section.Content, quote) {
			return true
		}
	}
	return false
}

func parseID(value string) (uint, bool) {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return uint(parsed), err == nil && parsed > 0
}

func positiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func limit(c *gin.Context, fallback int) int {
	n := positiveInt(c.Query("limit"), fallback)
	if n > 100 {
		return 100
	}
	return n
}

func fail(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "服务暂时不可用，请稍后重试"})
}

func respondDB(c *gin.Context, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "没有找到该中文条目"})
		return
	}
	fail(c, err)
}
