package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in smoke test against a migrated MySQL schema; every inserted row is rolled back.
func TestMySQLAccountTransaction(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN for a rollback-only integration test")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("MySQL connection failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	router := New(tx, nil, ai.New("", "", "demo"), "mock").Router()
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
			t.Fatalf("%s %s: got %d: %s", method, path, res.Code, res.Body.String())
		}
		return res
	}
	username := fmt.Sprintf("test_rollback_%d", time.Now().UnixNano())
	seedVerification(t, tx, username+"@example.com")
	res := call("POST", "/auth/register", gin.H{"username": username, "name": "事务回滚测试", "password": "Temporary-test-only-password", "email": username + "@example.com", "code": "123456"}, nil, 200)
	cookie := res.Result().Cookies()[0]
	var result struct{ User model.User }
	json.Unmarshal(res.Body.Bytes(), &result)
	if result.User.Role != "learner" {
		t.Fatal("wrong role")
	}
	call("GET", "/auth/me", nil, cookie, 200)
	var article model.Article
	if err := tx.Where("published = ?", true).First(&article).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/articles/%d/comments", article.ID)
	call("POST", path, gin.H{"body": "仅事务内测试，不保留此评论"}, cookie, 201)
	call("POST", "/auth/logout", nil, cookie, 200)
	call("POST", path, gin.H{"body": "not authorized"}, cookie, 401)
	call("POST", "/auth/login", gin.H{"username": username, "password": "Temporary-test-only-password"}, nil, 200)
}
