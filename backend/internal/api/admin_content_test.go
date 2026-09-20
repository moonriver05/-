package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContentModerationLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "moderation.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, _ := db.DB()
	defer connection.Close()
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	admin := model.User{Name: "管理员", Role: "admin"}
	reader := model.User{Name: "读者", Role: "learner"}
	other := model.User{Name: "其他人", Role: "learner"}
	for i, u := range []*model.User{&admin, &reader, &other} {
		if err = db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
		token := strings.Repeat(fmt.Sprint(i+1), 64)
		if err = db.Create(&model.Session{ID: tokenHash(token), UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := New(db, nil, ai.New("", "", "demo"), "mock").Router()
	call := func(method, path string, body any, who int, want int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MathWiki-Request", "1")
		if who > 0 {
			req.Header.Set("Cookie", sessionCookie+"="+strings.Repeat(fmt.Sprint(who), 64))
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s: want %d got %d %s", method, path, want, res.Code, res.Body.String())
		}
		return res
	}
	for _, route := range []struct{ method, path string }{{"GET", "/admin/articles"}, {"GET", "/admin/articles/1"}, {"PATCH", "/admin/articles/1"}, {"DELETE", "/admin/articles/1"}, {"POST", "/admin/articles/1/restore"}, {"GET", "/admin/feedback"}, {"PATCH", "/admin/feedback/1"}} {
		call(route.method, route.path, nil, 0, 401)
		call(route.method, route.path, nil, 2, 403)
	}
	input := gin.H{"title": "待审测试概念", "category": "测试分类", "kind": "概念", "difficulty": "入门", "content": "这是完整的测试正文。", "published": true, "reviewStatus": "approved"}
	call("POST", "/articles", input, 2, 403)
	var article articleResponse
	json.Unmarshal(call("POST", "/articles", input, 1, 201).Body.Bytes(), &article)
	if article.Published || article.ReviewStatus != "community_pending" {
		t.Fatal("created article bypassed moderation")
	}
	articlePath := fmt.Sprintf("/articles/%d", article.ID)
	adminPath := fmt.Sprintf("/admin/articles/%d", article.ID)
	call("GET", "/articles/"+article.Slug, nil, 0, 404)
	if !strings.Contains(call("GET", "/admin/articles?status=pending", nil, 1, 200).Body.String(), article.Title) {
		t.Fatal("pending article missing")
	}
	if !strings.Contains(call("GET", adminPath, nil, 1, 200).Body.String(), "完整的测试正文") {
		t.Fatal("admin preview lacks content")
	}
	// Existing pre-feature community entries must also be gated despite published=true.
	if err = db.Model(&model.Article{}).Where("id = ?", article.ID).Update("published", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/articles?format=page", "/search?q=待审", "/stats", "/categories"} {
		if strings.Contains(call("GET", path, nil, 0, 200).Body.String(), article.Title) {
			t.Fatal("legacy pending article leaked")
		}
	}
	call("GET", "/articles/"+article.Slug, nil, 0, 404)
	call("PATCH", adminPath, gin.H{"status": "approved", "expectedStatus": "community_pending"}, 1, 200)
	call("PATCH", adminPath, gin.H{"status": "rejected", "expectedStatus": "community_pending"}, 1, 409)
	call("GET", "/articles/"+article.Slug, nil, 0, 200)
	call("PUT", articlePath+"/favorite", gin.H{"saved": true}, 2, 200)
	call("PUT", articlePath+"/read", nil, 2, 200)
	call("POST", articlePath+"/comments", gin.H{"body": "公开的文章评论"}, 2, 201)
	var note annotationResponse
	json.Unmarshal(call("POST", "/annotations", gin.H{"articleId": article.ID, "quote": "完整的测试正文", "body": "审核后的批注"}, 2, 201).Body.Bytes(), &note)
	notePath := fmt.Sprintf("/annotations/%d", note.ID)
	call("PATCH", fmt.Sprintf("/admin/annotations/%d", note.ID), gin.H{"status": "visible"}, 1, 200)
	call("POST", notePath+"/comments", gin.H{"body": "关联回复"}, 2, 201)
	var feedback model.Feedback
	json.Unmarshal(call("POST", "/feedback", gin.H{"articleId": article.ID, "kind": "content_suggestion", "body": "需要补充定义", "status": "resolved", "resolution": "伪造说明", "reviewerId": admin.ID, "userId": other.ID}, 2, 201).Body.Bytes(), &feedback)
	if feedback.Status != "pending" || feedback.UserID != reader.ID || feedback.Resolution != "" || feedback.ReviewerID != nil {
		t.Fatal("feedback mass assignment")
	}
	call("POST", "/feedback", gin.H{"articleId": 99999, "kind": "content_suggestion", "body": "错误引用"}, 2, 404)
	call("POST", "/feedback", gin.H{"articleId": article.ID, "kind": "content_suggestion", "body": " "}, 2, 400)
	call("GET", "/feedback/mine", nil, 0, 401)
	if strings.Contains(call("GET", "/feedback/mine?status=all", nil, 3, 200).Body.String(), "需要补充定义") {
		t.Fatal("other user's feedback leaked")
	}
	if !strings.Contains(call("GET", "/admin/feedback?status=pending", nil, 1, 200).Body.String(), "需要补充定义") {
		t.Fatal("feedback missing from queue")
	}
	reviewPath := fmt.Sprintf("/admin/feedback/%d", feedback.ID)
	call("PATCH", reviewPath, gin.H{"status": "resolved", "resolution": ""}, 1, 400)
	call("PATCH", reviewPath, gin.H{"status": "resolved", "resolution": "已核对定义，建议已记录"}, 1, 200)
	call("PATCH", reviewPath, gin.H{"status": "dismissed", "resolution": "重复处理"}, 1, 409)
	if !strings.Contains(call("GET", "/feedback/mine?status=resolved", nil, 2, 200).Body.String(), "已核对定义") {
		t.Fatal("resolution not visible to submitter")
	}
	// Deletion hides every related public read/write but preserves data for restore.
	call("DELETE", adminPath, nil, 1, 200)
	for _, path := range []string{"/articles/" + article.Slug, articlePath + "/annotations", articlePath + "/comments", notePath + "/comments"} {
		call("GET", path, nil, 0, 404)
	}
	call("POST", notePath+"/comments", gin.H{"body": "不能新增"}, 2, 404)
	call("POST", notePath+"/like", gin.H{}, 2, 404)
	call("POST", articlePath+"/comments", gin.H{"body": "不能新增"}, 2, 404)
	call("POST", "/feedback", gin.H{"articleId": article.ID, "kind": "helpful"}, 2, 404)
	call("POST", "/explanations", gin.H{"articleId": article.ID, "question": "解释一下"}, 2, 404)
	call("PUT", articlePath+"/favorite", gin.H{"saved": true}, 2, 404)
	call("PUT", articlePath+"/read", nil, 2, 404)
	if strings.Contains(call("GET", "/search?q=待审", nil, 0, 200).Body.String(), article.Title) {
		t.Fatal("deleted article searchable")
	}
	var stats struct{ ArticleCount, SectionCount, FavoriteCount, RecentCount int }
	json.Unmarshal(call("GET", "/stats", nil, 2, 200).Body.Bytes(), &stats)
	if stats.ArticleCount != 0 || stats.SectionCount != 0 || stats.FavoriteCount != 0 || stats.RecentCount != 0 {
		t.Fatalf("deleted stats counted: %+v", stats)
	}
	var stored model.Article
	if err = db.Unscoped().First(&stored, article.ID).Error; err != nil || !stored.DeletedAt.Valid {
		t.Fatal("not soft-deleted")
	}
	if !strings.Contains(call("GET", "/admin/articles?status=deleted", nil, 1, 200).Body.String(), article.Title) {
		t.Fatal("recycle bin missing")
	}
	call("POST", adminPath+"/restore", nil, 1, 200)
	call("POST", adminPath+"/restore", nil, 1, 409)
	call("GET", "/articles/"+article.Slug, nil, 0, 200)
	if !strings.Contains(call("GET", articlePath+"/comments", nil, 0, 200).Body.String(), "公开的文章评论") {
		t.Fatal("restore lost comments")
	}
	if !strings.Contains(call("GET", notePath+"/comments", nil, 0, 200).Body.String(), "关联回复") {
		t.Fatal("restore lost replies")
	}
	call("PATCH", adminPath, gin.H{"status": "rejected", "expectedStatus": "approved"}, 1, 200)
	call("GET", "/articles/"+article.Slug, nil, 0, 404)
	call("DELETE", adminPath, nil, 1, 200)
	call("POST", adminPath+"/restore", nil, 1, 200)
	call("GET", "/articles/"+article.Slug, nil, 0, 404)
	call("GET", "/admin/articles?status=bad", nil, 1, 400)
	call("GET", "/admin/feedback?status=bad", nil, 1, 400)
	// Imported wiki entries remain public and have a separate review bucket.
	imported := model.Article{PageID: 123, Title: "维基导入", Slug: "imported", Language: "zh-Hans", ReviewStatus: "machine_selected", Published: true}
	if err = db.Create(&imported).Error; err != nil {
		t.Fatal(err)
	}
	call("GET", "/articles/imported", nil, 0, 200)
	if !strings.Contains(call("GET", "/admin/articles?status=unreviewed", nil, 1, 200).Body.String(), "维基导入") {
		t.Fatal("import review bucket missing")
	}
}
