package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

var categoryKeywords = map[string][]string{
	"数学基础":  {"数学基础", "线性代数", "微积分", "离散数学", "矩阵", "集合", "极限"},
	"算法设计":  {"算法", "动态规划", "图论", "搜索", "排序", "优化"},
	"图论算法":  {"图论", "最短路径"},
	"概率与统计": {"概率", "统计", "贝叶斯"},
	"搜索与排序": {"搜索", "排序"},
	"机器学习":  {"机器学习", "梯度下降", "神经网络"},
	"微积分":   {"微积分", "导数", "积分", "极限"},
	"线性代数":  {"线性代数", "矩阵", "向量"},
}

func categoryFilter(query *gorm.DB, name string) *gorm.DB {
	terms := categoryKeywords[name]
	if len(terms) == 0 {
		terms = []string{name}
	}
	parts := []string{}
	args := []any{}
	for _, term := range terms {
		parts = append(parts, "(category LIKE ? OR title LIKE ? OR tags LIKE ?)")
		for i := 0; i < 3; i++ {
			args = append(args, "%"+term+"%")
		}
	}
	return query.Where("("+strings.Join(parts, " OR ")+")", args...)
}

type categoryResponse struct {
	Name     string             `json:"name"`
	Count    int64              `json:"count"`
	Children []categoryResponse `json:"children"`
}

func (s *Server) listCategories(c *gin.Context) {
	presets := []categoryResponse{
		{Name: "数学基础", Children: []categoryResponse{{Name: "线性代数"}, {Name: "微积分"}, {Name: "离散数学"}}},
		{Name: "算法设计", Children: []categoryResponse{{Name: "图论算法"}, {Name: "动态规划"}, {Name: "搜索与排序"}}},
		{Name: "概率与统计"}, {Name: "机器学习"}, {Name: "数值计算"},
	}
	var custom []model.Category
	if err := s.db.Order("created_at ASC").Find(&custom).Error; err != nil {
		fail(c, err)
		return
	}
	for _, item := range custom {
		presets = append(presets, categoryResponse{Name: item.Name})
	}
	var fill func(*categoryResponse) error
	fill = func(item *categoryResponse) error {
		if item.Children == nil {
			item.Children = []categoryResponse{}
		}
		if err := categoryFilter(s.db.Model(&model.Article{}).Scopes(publicArticles).Where("language = ?", "zh-Hans"), item.Name).Count(&item.Count).Error; err != nil {
			return err
		}
		for i := range item.Children {
			if err := fill(&item.Children[i]); err != nil {
				return err
			}
		}
		return nil
	}
	for i := range presets {
		if err := fill(&presets[i]); err != nil {
			fail(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, presets)
}

func (s *Server) createCategory(c *gin.Context) {
	var input struct {
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "分类格式不正确"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 80 {
		c.JSON(400, gin.H{"error": "分类名称应为 1 到 80 个字"})
		return
	}
	reserved := []string{"全部条目", "我的收藏", "最近阅读", "我的批注", "数学基础", "线性代数", "微积分", "离散数学", "算法设计", "图论算法", "动态规划", "搜索与排序", "概率与统计", "机器学习", "数值计算"}
	for _, name := range reserved {
		if name == input.Name {
			c.JSON(409, gin.H{"error": "该分类已经存在"})
			return
		}
	}
	category := model.Category{Name: input.Name}
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&category)
	if result.Error != nil {
		fail(c, result.Error)
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "该分类已经存在"})
		return
	}
	c.JSON(201, category)
}

func (s *Server) markRead(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "条目编号不正确"})
		return
	}
	var article model.Article
	if err := s.db.Scopes(publicArticles).Where("id = ?", id).First(&article).Error; err != nil {
		respondDB(c, err)
		return
	}
	item := model.ReadingHistory{UserID: userID(c), ArticleID: id, UpdatedAt: time.Now()}
	err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}, {Name: "article_id"}}, DoUpdates: clause.AssignmentColumns([]string{"updated_at"})}).Create(&item).Error
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"articleId": id, "readAt": item.UpdatedAt})
}

func (s *Server) createArticle(c *gin.Context) {
	var input struct {
		Title      string `json:"title"`
		Category   string `json:"category"`
		Kind       string `json:"kind"`
		Difficulty string `json:"difficulty"`
		Content    string `json:"content"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "条目格式不正确"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Category = strings.TrimSpace(input.Category)
	input.Content = strings.TrimSpace(input.Content)
	if input.Title == "" || input.Category == "" || input.Content == "" || utf8.RuneCountInString(input.Title) > 255 || utf8.RuneCountInString(input.Category) > 80 || utf8.RuneCountInString(input.Content) > 100000 {
		c.JSON(400, gin.H{"error": "请填写标题、分类和正文；标题最多 255 字，正文最多 10 万字"})
		return
	}
	if input.Kind != "概念" && input.Kind != "算法" && input.Kind != "例题" {
		c.JSON(400, gin.H{"error": "请选择有效的条目类型"})
		return
	}
	if input.Difficulty != "入门" && input.Difficulty != "进阶" && input.Difficulty != "高级" {
		c.JSON(400, gin.H{"error": "请选择有效的难度"})
		return
	}
	tags, _ := json.Marshal([]string{input.Category})
	lead := []rune(input.Content)
	if len(lead) > 160 {
		lead = lead[:160]
	}
	now := time.Now().UnixNano()
	article := model.Article{PageID: -now, Slug: fmt.Sprintf("community-%d", now), Title: input.Title, Lead: string(lead), PlainText: input.Content, Category: input.Category, Categories: string(tags), Tags: string(tags), Kind: input.Kind, Difficulty: input.Difficulty, ReadMinutes: max(1, utf8.RuneCountInString(input.Content)/400), Language: "zh-Hans", License: "CC BY-SA 4.0", ReviewStatus: "community_pending", Published: false,
		Sections: []model.Section{{Position: 1, Title: "正文", Content: input.Content, Formulae: "[]"}},
	}
	if err := s.db.Create(&article).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, toArticleResponse(article, false))
}

type commentResponse struct {
	Avatar    string    `json:"avatar,omitempty"`
	UserID    uint      `json:"userId"`
	ID        uint      `json:"id"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

func toCommentResponse(item model.Comment) commentResponse {
	name := item.User.Name
	if name == "" {
		name = "用户"
	}
	return commentResponse{ID: item.ID, Avatar: item.User.Avatar, UserID: item.UserID, Body: item.Body, Author: name, CreatedAt: item.CreatedAt}
}

func (s *Server) listComments(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "批注编号不正确"})
		return
	}
	var note model.Annotation
	if err := s.db.Where("id = ? AND status = ? AND article_id IN (?)", id, "visible", s.publicArticleIDs()).First(&note).Error; err != nil {
		respondDB(c, err)
		return
	}
	var items []model.Comment
	if err := s.db.Preload("User").Where("annotation_id = ? AND status = ?", id, "visible").Order("created_at ASC").Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	result := make([]commentResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toCommentResponse(item))
	}
	c.JSON(200, result)
}
