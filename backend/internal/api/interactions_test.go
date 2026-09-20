package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"mathwiki/internal/seed"
)

func TestWorkspaceInteractionsPersistAndFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "interactions.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = seed.Ensure(db, true); err != nil {
		t.Fatal(err)
	}
	router := New(db, nil, ai.New("", "", "demo"), "mock").Router()
	db.Model(&model.User{}).Where("id = ?", 1).Update("role", "admin")
	token := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	db.Create(&model.Session{ID: tokenHash(token), UserID: 1, ExpiresAt: time.Now().Add(time.Hour)})
	call := func(method, path string, body any, status int) []byte {
		t.Helper()
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MathWiki-Request", "1")
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: got %d, want %d: %s", method, path, res.Code, status, res.Body.String())
		}
		return res.Body.Bytes()
	}
	var health struct {
		DatabaseDriver string `json:"databaseDriver"`
	}
	if err := json.Unmarshal(call("GET", "/health", nil, 200), &health); err != nil || health.DatabaseDriver != "sqlite" {
		t.Fatalf("health must report the actual database driver: %+v, %v", health, err)
	}
	var stats struct {
		DatabaseDriver string `json:"databaseDriver"`
	}
	if err := json.Unmarshal(call("GET", "/api/v1/stats", nil, 200), &stats); err != nil || stats.DatabaseDriver != "sqlite" {
		t.Fatalf("stats must report the actual database driver: %+v, %v", stats, err)
	}
	list := func(params string) []articleResponse {
		t.Helper()
		var result []articleResponse
		if err := json.Unmarshal(call("GET", "/api/v1/articles?"+params, nil, 200), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := list("category=线性代数"); len(got) != 0 {
		t.Fatalf("empty category returned %d articles", len(got))
	}
	if got := list("category=概率与统计"); len(got) != 1 || got[0].Title != "贝叶斯定理" {
		t.Fatalf("wrong category: %+v", got)
	}
	if got := list("collection=recent"); len(got) != 0 {
		t.Fatal("history should start empty")
	}
	var article articleResponse
	json.Unmarshal(call("GET", "/api/v1/articles/dijkstra", nil, 200), &article)
	id := article.ID
	call("PUT", fmt.Sprintf("/api/v1/articles/%d/read", id), nil, 200)
	call("PUT", fmt.Sprintf("/api/v1/articles/%d/read", id), nil, 200)
	if got := list("collection=recent"); len(got) != 1 || got[0].ID != id {
		t.Fatal("reading history must persist without duplicates")
	}
	call("PUT", fmt.Sprintf("/api/v1/articles/%d/favorite", id), gin.H{"saved": false}, 200)
	if got := list("collection=favorites"); len(got) != 0 {
		t.Fatal("removed favorite is still visible")
	}
	call("PUT", fmt.Sprintf("/api/v1/articles/%d/favorite", id), gin.H{"saved": true}, 200)
	if got := list("collection=favorites"); len(got) != 1 || !got[0].Saved {
		t.Fatal("favorite was not persisted")
	}
	var note annotationResponse
	json.Unmarshal(call("POST", "/api/v1/annotations", gin.H{"articleId": id, "quote": "Dijkstra 算法维护一个已经确定最短距离的顶点集合。", "body": "为什么需要非负权？"}, 201), &note)
	if got := list("collection=annotated"); len(got) != 1 || got[0].ID != id {
		t.Fatal("my annotations filter is wrong")
	}
	path := fmt.Sprintf("/api/v1/annotations/%d/comments", note.ID)
	call("POST", path, gin.H{"body": "因为贪心选择依赖非负边权。"}, 201)
	var comments []commentResponse
	json.Unmarshal(call("GET", path, nil, 200), &comments)
	if len(comments) != 1 || comments[0].Body != "因为贪心选择依赖非负边权。" || comments[0].Author != "演示用户" {
		t.Fatalf("reply was not persisted: %+v", comments)
	}
	call("POST", path, gin.H{"body": " "}, 400)
	call("POST", path, gin.H{"body": "reply", "parentId": 999999}, 400)
	call("POST", "/api/v1/categories", gin.H{"name": "计算几何"}, 201)
	call("POST", "/api/v1/categories", gin.H{"name": "计算几何"}, 409)
	var created articleResponse
	json.Unmarshal(call("POST", "/api/v1/articles", gin.H{"title": "凸包", "category": "计算几何", "kind": "概念", "difficulty": "入门", "content": "凸包是包含给定点集的最小凸集。"}, 201), &created)
	if got := list("category=计算几何&kind=概念"); len(got) != 0 {
		t.Fatal("pending article must not be published")
	}
	call("PATCH", fmt.Sprintf("/api/v1/admin/articles/%d", created.ID), gin.H{"status": "approved", "expectedStatus": "community_pending"}, 200)
	if got := list("category=计算几何&kind=概念"); len(got) != 1 || got[0].Title != "凸包" {
		t.Fatal("created article is not in its category")
	}
	call("GET", "/api/v1/articles?sort=difficulty&format=page", nil, http.StatusOK)
	call("POST", "/api/v1/feedback", gin.H{"articleId": id, "kind": "content_suggestion", "body": "补充例题"}, 201)
	var count int64
	db.Model(&model.Feedback{}).Count(&count)
	if count != 1 {
		t.Fatal("feedback was not persisted")
	}
}
