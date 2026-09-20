package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
)

type testMailer struct {
	code       string
	configured bool
	fail       bool
	sends      int
}

func (m *testMailer) Configured() bool { return m.configured }
func (m *testMailer) SendCode(_ context.Context, _ string, code string) error {
	m.sends++
	m.code = code
	if m.fail {
		return errors.New("fake SMTP failure")
	}
	return nil
}
func seedVerification(t *testing.T, db *gorm.DB, email string) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("123456"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	v := model.EmailVerification{Email: email, CodeHash: string(hash), Ready: true, ExpiresAt: now.Add(time.Minute), SentAt: now, WindowStart: now}
	if err = db.Save(&v).Error; err != nil {
		t.Fatal(err)
	}
}
func TestEmailVerificationFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "email.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	sender := &testMailer{}
	server := New(db, nil, ai.New("", "", "demo"), "mock")
	server.mailer = sender
	router := server.Router()
	requestNo := 0
	call := func(path string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		requestNo++
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-MathWiki-Request", "1")
		// Separate clients so this suite exercises persisted email limits independently of the IP limiter.
		req.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", requestNo)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s got %d want %d: %s", path, res.Code, want, res.Body.String())
		}
		return res
	}
	input := func(email, code, username string) gin.H {
		return gin.H{"email": email, "code": code, "username": username, "name": "学习者", "password": "valid-password"}
	}
	call("/auth/register", input("", "", "missing"), 400)
	call("/auth/register", input("absent@example.com", "123456", "absent"), 400)
	call("/auth/email-code", gin.H{"email": "hello@example.com"}, 503)
	if sender.sends != 0 {
		t.Fatal("unconfigured sender was called")
	}
	sender.configured = true
	call("/auth/email-code", gin.H{"email": "Name <a@example.com>"}, 400)
	call("/auth/email-code", gin.H{"email": "a@example.com\r\nBcc: b@example.com"}, 400)
	res := call("/auth/email-code", gin.H{"email": " HELLO@EXAMPLE.COM "}, 200)
	if len(sender.code) != 6 || strings.Contains(res.Body.String(), sender.code) {
		t.Fatal("code invalid or leaked")
	}
	original := sender.code
	call("/auth/email-code", gin.H{"email": "hello@example.com"}, 429)
	wrong := "000000"
	if original == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		call("/auth/register", input("hello@example.com", wrong, "hello"), 400)
	}
	call("/auth/register", input("hello@example.com", original, "hello"), 400)
	db.Model(&model.EmailVerification{}).Where("email = ?", "hello@example.com").Update("sent_at", time.Now().Add(-time.Minute*2))
	call("/auth/email-code", gin.H{"email": "hello@example.com"}, 200)
	call("/auth/register", input("different@example.com", sender.code, "wrong_email"), 400)
	db.Model(&model.EmailVerification{}).Where("email = ?", "hello@example.com").Update("expires_at", time.Now().Add(-time.Minute))
	call("/auth/register", input("hello@example.com", sender.code, "hello"), 400)
	db.Model(&model.EmailVerification{}).Where("email = ?", "hello@example.com").Update("sent_at", time.Now().Add(-time.Minute*2))
	call("/auth/email-code", gin.H{"email": "hello@example.com"}, 200)
	code := sender.code
	call("/auth/register", input("hello@example.com", code, "hello"), 200)
	var user model.User
	db.Where("username = ?", "hello").First(&user)
	if user.Email == nil || *user.Email != "hello@example.com" || user.EmailVerifiedAt == nil {
		t.Fatal("verified email was not persisted")
	}
	call("/auth/register", input("hello@example.com", code, "replay"), 400)
	call("/auth/email-code", gin.H{"email": "hello@example.com"}, 409)
	// Failed SMTP never leaves a usable code, even if the transport captured it.
	sender.fail = true
	call("/auth/email-code", gin.H{"email": "failure@example.com"}, 502)
	call("/auth/register", input("failure@example.com", sender.code, "failure"), 400)
	sender.fail = false
	seedVerification(t, db, "quota@example.com")
	db.Model(&model.EmailVerification{}).Where("email = ?", "quota@example.com").Updates(map[string]any{"sent_at": time.Now().Add(-2 * time.Minute), "send_count": 10})
	call("/auth/email-code", gin.H{"email": "quota@example.com"}, 429)
	// A duplicate username rolls back consumption; the verified email can choose another username.
	seedVerification(t, db, "other@example.com")
	call("/auth/register", input("other@example.com", "123456", "hello"), 409)
	call("/auth/register", input("other@example.com", "123456", "other"), 200)
}
