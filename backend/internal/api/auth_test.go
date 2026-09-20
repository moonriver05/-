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
	"mathwiki/internal/seed"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAccountsIsolationAndModeration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "accounts.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = seed.Ensure(db, true); err != nil {
		t.Fatal(err)
	}
	router := New(db, nil, ai.New("", "", "demo"), "mock").Router()
	call := func(method, path string, body any, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MathWiki-Request", "1")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	register := func(username string) (*http.Cookie, model.User) {
		t.Helper()
		seedVerification(t, db, username+"@example.com")
		r := call("POST", "/auth/register", gin.H{"username": username, "name": username, "password": "Good-test-password", "role": "admin", "id": 1, "email": username + "@example.com", "code": "123456"}, nil, 200)
		var result struct{ User model.User }
		json.Unmarshal(r.Body.Bytes(), &result)
		if result.User.Role != "learner" || strings.Contains(r.Body.String(), "PasswordHash") || strings.Contains(r.Body.String(), "password") {
			t.Fatal("registration leaked secrets or elevated role")
		}
		cookies := r.Result().Cookies()
		if len(cookies) == 0 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
			t.Fatal("unsafe session cookie")
		}
		return cookies[0], result.User
	}
	a, alice := register("alice")
	b, _ := register("bob")
	admin, adminUser := register("admin_account")
	db.Model(&adminUser).Update("role", "admin")
	seedVerification(t, db, "duplicate@example.com")
	call("POST", "/auth/register", gin.H{"username": "ALICE", "name": "other", "password": "Good-test-password", "email": "duplicate@example.com", "code": "123456"}, nil, 409)
	call("POST", "/auth/login", gin.H{"username": "alice", "password": "incorrect"}, nil, 401)
	call("POST", "/auth/login", gin.H{"username": "nonexistent", "password": "incorrect"}, nil, 401)
	call("GET", "/admin/annotations", nil, nil, 401)
	call("GET", "/admin/annotations", nil, a, 403)
	call("POST", "/categories", gin.H{"name": "private"}, a, 403)
	var article model.Article
	db.Where("slug = ?", "dijkstra").First(&article)
	path := fmt.Sprintf("/articles/%d", article.ID)
	call("PUT", path+"/favorite", gin.H{"saved": true}, nil, 401)
	call("PUT", path+"/favorite", gin.H{"saved": true}, a, 200)
	for _, cookie := range []*http.Cookie{b, nil} {
		if call("GET", "/articles?collection=favorites", nil, cookie, 200).Body.String() != "[]" {
			t.Fatal("favorites leaked")
		}
	}
	call("PUT", path+"/read", nil, a, 200)
	if call("GET", "/articles?collection=recent", nil, b, 200).Body.String() != "[]" {
		t.Fatal("history leaked")
	}
	var note annotationResponse
	r := call("POST", "/annotations", gin.H{"articleId": article.ID, "quote": "Dijkstra 算法维护一个已经确定最短距离的顶点集合。", "body": "待审核正文", "userId": 1, "status": "visible"}, a, 201)
	json.Unmarshal(r.Body.Bytes(), &note)
	if note.UserID != alice.ID || note.Status != "pending" {
		t.Fatal("annotation identity/status injection")
	}
	for _, cookie := range []*http.Cookie{b, nil} {
		if strings.Contains(call("GET", path+"/annotations", nil, cookie, 200).Body.String(), "待审核正文") {
			t.Fatal("pending annotation leaked")
		}
	}
	if !strings.Contains(call("GET", path+"/annotations", nil, a, 200).Body.String(), "待审核正文") {
		t.Fatal("author cannot see pending")
	}
	if !strings.Contains(call("GET", "/admin/annotations", nil, admin, 200).Body.String(), "待审核正文") {
		t.Fatal("queue missing note")
	}
	notePath := fmt.Sprintf("/annotations/%d", note.ID)
	reviewPath := fmt.Sprintf("/admin/annotations/%d", note.ID)
	call("PATCH", reviewPath, gin.H{"status": "visible"}, b, 403)
	call("DELETE", notePath, nil, b, 403)
	call("GET", notePath+"/comments", nil, b, 404)
	call("POST", notePath+"/comments", gin.H{"body": "premature"}, b, 404)
	call("PATCH", reviewPath, gin.H{"status": "visible"}, admin, 200)
	call("PATCH", reviewPath, gin.H{"status": "rejected"}, admin, 409)
	if !strings.Contains(call("GET", path+"/annotations", nil, nil, 200).Body.String(), "待审核正文") {
		t.Fatal("approved annotation not public")
	}
	var reply commentResponse
	json.Unmarshal(call("POST", notePath+"/comments", gin.H{"body": "reply"}, b, 201).Body.Bytes(), &reply)
	replyPath := fmt.Sprintf("/comments/%d", reply.ID)
	call("DELETE", replyPath, nil, a, 403)
	call("DELETE", replyPath, nil, admin, 200)
	var comment commentResponse
	json.Unmarshal(call("POST", path+"/comments", gin.H{"body": "文章底部评论", "userId": 1}, a, 201).Body.Bytes(), &comment)
	if comment.UserID != alice.ID {
		t.Fatal("comment user spoofed")
	}
	call("POST", path+"/comments", gin.H{"body": " "}, a, 400)
	if !strings.Contains(call("GET", path+"/comments", nil, nil, 200).Body.String(), "文章底部评论") {
		t.Fatal("article comment missing")
	}
	commentPath := fmt.Sprintf("/article-comments/%d", comment.ID)
	call("DELETE", commentPath, nil, b, 403)
	call("DELETE", commentPath, nil, a, 200)
	call("DELETE", notePath, nil, admin, 200)
	var deleted model.Annotation
	if db.Unscoped().First(&deleted, note.ID).Error != nil || !deleted.DeletedAt.Valid {
		t.Fatal("delete should be recoverable")
	}
	req := httptest.NewRequest("POST", "/api/v1/annotations", strings.NewReader(`{}`))
	req.AddCookie(a)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != 403 {
		t.Fatal("CSRF header not enforced")
	}
	call("POST", "/auth/logout", nil, a, 200)
	call("PUT", path+"/read", nil, a, 401)
	login := call("POST", "/auth/login", gin.H{"username": "ALICE", "password": "Good-test-password"}, nil, 200)
	session := login.Result().Cookies()[0]
	db.Model(&model.Session{}).Where("id = ?", tokenHash(session.Value)).Update("expires_at", time.Now().Add(-time.Hour))
	call("PUT", path+"/read", nil, session, 401)
}
