package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"mathwiki/internal/seed"
)

func (m *testMailer) SendResetCode(ctx context.Context, email, code string) error {
	return m.SendCode(ctx, email, code)
}

type securityFixture struct {
	t                   *testing.T
	db                  *gorm.DB
	s                   *Server
	router              *gin.Engine
	learner, admin      model.User
	cookie, adminCookie *http.Cookie
	article             model.Article
}

func newSecurityFixture(t *testing.T) *securityFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "security.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = seed.Ensure(db, true); err != nil {
		t.Fatal(err)
	}
	f := &securityFixture{t: t, db: db}
	f.s = New(db, nil, ai.New("", "", "demo"), "mock")
	f.router = f.s.Router()
	hash, _ := bcrypt.GenerateFromPassword([]byte("Original-password"), bcrypt.MinCost)
	now := time.Now()
	name, email := "security_learner", "security@example.com"
	f.learner = model.User{Username: &name, Email: &email, EmailVerifiedAt: &now, Name: "测试用户", Role: "learner", PasswordHash: string(hash)}
	if err = db.Create(&f.learner).Error; err != nil {
		t.Fatal(err)
	}
	adminName := "security_admin"
	f.admin = model.User{Username: &adminName, Name: "测试管理员", Role: "admin", PasswordHash: string(hash)}
	if err = db.Create(&f.admin).Error; err != nil {
		t.Fatal(err)
	}
	f.cookie = f.session(f.learner.ID, "a")
	f.adminCookie = f.session(f.admin.ID, "b")
	if err = db.Where("slug = ?", "dijkstra").First(&f.article).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *securityFixture) session(id uint, letter string) *http.Cookie {
	f.t.Helper()
	token := strings.Repeat(letter, 64)
	if err := f.db.Create(&model.Session{ID: tokenHash(token), UserID: id, ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		f.t.Fatal(err)
	}
	return &http.Cookie{Name: sessionCookie, Value: token}
}
func (f *securityFixture) call(method, path string, body any, cookie *http.Cookie, want int) *httptest.ResponseRecorder {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MathWiki-Request", "1")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res := httptest.NewRecorder()
	f.router.ServeHTTP(res, req)
	if res.Code != want {
		f.t.Fatalf("%s %s: got %d want %d: %s", method, path, res.Code, want, res.Body.String())
	}
	return res
}

func TestAnnotationLikesAreIdempotentAndReversible(t *testing.T) {
	f := newSecurityFixture(t)
	note := model.Annotation{ArticleID: f.article.ID, UserID: f.admin.ID, Quote: "测试", Body: "正文", Status: "visible", Likes: 999}
	if err := f.db.Create(&note).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/annotations/%d/like", note.ID)
	f.call("POST", path, nil, nil, 401)
	for i := 0; i < 3; i++ {
		res := f.call("POST", path, nil, f.cookie, 200)
		if !strings.Contains(res.Body.String(), `"likes":1`) {
			t.Fatal(res.Body.String())
		}
	}
	f.call("POST", path, nil, f.adminCookie, 200)
	list := f.call("GET", fmt.Sprintf("/articles/%d/annotations", f.article.ID), nil, f.cookie, 200)
	var notes []annotationResponse
	json.Unmarshal(list.Body.Bytes(), &notes)
	found := false
	for _, n := range notes {
		if n.ID == note.ID {
			found = n.Liked && n.Likes == 2
		}
	}
	if !found {
		t.Fatal("missing personal like state")
	}
	for i := 0; i < 2; i++ {
		res := f.call("DELETE", path, nil, f.cookie, 200)
		if !strings.Contains(res.Body.String(), `"likes":1`) || !strings.Contains(res.Body.String(), `"liked":false`) {
			t.Fatal(res.Body.String())
		}
	}
	f.db.Model(&note).Update("status", "pending")
	f.call("POST", path, nil, f.cookie, 404)
	var count int64
	f.db.Model(&model.AnnotationLike{}).Where("annotation_id = ?", note.ID).Count(&count)
	if count != 1 {
		t.Fatal(count)
	}
}

func TestCommunityDuplicateAndPersistentQuota(t *testing.T) {
	f := newSecurityFixture(t)
	path := fmt.Sprintf("/articles/%d/comments", f.article.ID)
	f.call("POST", path, gin.H{"body": "  相同   评论  "}, f.cookie, 201)
	f.call("POST", path, gin.H{"body": "相同 评论"}, f.cookie, 409)
	f.call("POST", path, gin.H{"body": "相同 评论"}, f.adminCookie, 201)
	annotation := gin.H{"articleId": f.article.ID, "quote": "Dijkstra 算法维护一个已经确定最短距离的顶点集合。", "body": "防重批注"}
	f.call("POST", "/annotations", annotation, f.cookie, 201)
	f.call("POST", "/annotations", annotation, f.cookie, 409)
	note := model.Annotation{ArticleID: f.article.ID, UserID: f.admin.ID, Status: "visible", Quote: "原文", Body: "正文"}
	f.db.Create(&note)
	replies := fmt.Sprintf("/annotations/%d/comments", note.ID)
	f.call("POST", replies, gin.H{"body": "回复"}, f.cookie, 201)
	f.call("POST", replies, gin.H{"body": "回复"}, f.cookie, 409)
	for i := 3; i < 20; i++ {
		f.call("POST", path, gin.H{"body": fmt.Sprintf("不同评论 %d", i)}, f.cookie, 201)
	}
	// A fresh Server cannot reset database-backed posting limits.
	f.router = New(f.db, nil, ai.New("", "", "demo"), "mock").Router()
	r := f.call("POST", path, gin.H{"body": "超出额度"}, f.cookie, 429)
	if r.Header().Get("Retry-After") == "" {
		t.Fatal("missing retry header")
	}
	f.db.Model(&model.CommunitySubmission{}).Where("user_id = ?", f.learner.ID).Update("created_at", time.Now().Add(-11*time.Minute))
	f.call("POST", path, gin.H{"body": "相同 评论"}, f.cookie, 201)
}

func TestBanningRevokesSessionsAndAuditsAdministrativeActions(t *testing.T) {
	f := newSecurityFixture(t)
	path := fmt.Sprintf("/admin/users/%d", f.learner.ID)
	f.call("GET", "/admin/users", nil, f.cookie, 403)
	f.call("GET", "/admin/audits", nil, nil, 401)
	f.call("PATCH", path, gin.H{"banned": true, "reason": "垃圾评论"}, f.cookie, 403)
	f.call("PATCH", fmt.Sprintf("/admin/users/%d", f.admin.ID), gin.H{"banned": true, "reason": "测试"}, f.adminCookie, 409)
	f.call("PATCH", path, gin.H{"banned": true}, f.adminCookie, 400)
	f.call("PATCH", path, gin.H{"banned": true, "reason": "垃圾评论"}, f.adminCookie, 200)
	var count int64
	f.db.Model(&model.Session{}).Where("user_id = ?", f.learner.ID).Count(&count)
	if count != 0 {
		t.Fatal("old sessions survived ban")
	}
	f.call("POST", fmt.Sprintf("/articles/%d/comments", f.article.ID), gin.H{"body": "被封禁"}, f.cookie, 401)
	f.call("POST", "/auth/login", gin.H{"username": "security_learner", "password": "Original-password"}, nil, 401)
	f.call("PATCH", path, gin.H{"banned": false, "reason": "复核通过"}, f.adminCookie, 200)
	f.call("POST", "/auth/login", gin.H{"username": "security_learner", "password": "Original-password"}, nil, 200)
	f.call("GET", "/admin/users?q=security_learner", nil, f.adminCookie, 200)
	logs := f.call("GET", "/admin/audits", nil, f.adminCookie, 200).Body.String()
	if !strings.Contains(logs, "垃圾评论") || !strings.Contains(logs, "复核通过") || strings.Contains(logs, "Original-password") {
		t.Fatal(logs)
	}
	// Audit also covers the existing moderation and delete endpoints.
	note := model.Annotation{ArticleID: f.article.ID, UserID: f.learner.ID, Status: "pending", Quote: "原文", Body: "正文"}
	f.db.Create(&note)
	f.call("PATCH", fmt.Sprintf("/admin/annotations/%d", note.ID), gin.H{"status": "visible"}, f.adminCookie, 200)
	f.call("DELETE", fmt.Sprintf("/annotations/%d", note.ID), nil, f.adminCookie, 200)
	f.db.Model(&model.AdminAudit{}).Where("action = ? AND status = ?", "DELETE /api/v1/annotations/:id", 200).Count(&count)
	if count != 1 {
		t.Fatal("delete not audited")
	}
}

func TestPasswordResetIsolationConsumptionAndSessionRevocation(t *testing.T) {
	f := newSecurityFixture(t)
	mailer := &testMailer{configured: true}
	f.s.mailer = mailer
	f.call("POST", "/auth/reset-code", gin.H{"email": "security@example.com"}, nil, 200)
	code := mailer.code
	if len(code) != 6 {
		t.Fatal("mail missing")
	}
	var stored model.PasswordReset
	f.db.First(&stored, "email = ?", "security@example.com")
	if stored.CodeHash == code || !stored.Ready {
		t.Fatal("unsafe code storage")
	}
	f.call("POST", "/auth/reset-code", gin.H{"email": "security@example.com"}, nil, 429)
	unknown := f.call("POST", "/auth/reset-code", gin.H{"email": "unknown@example.com"}, nil, 200)
	if mailer.sends != 1 || !strings.Contains(unknown.Body.String(), "如果") {
		t.Fatal("unknown user received reset mail")
	}
	seedVerification(t, f.db, "security@example.com")
	payload := gin.H{"email": "security@example.com", "code": "123456", "password": "Replacement-password"}
	if code == "123456" {
		payload["code"] = "654321"
	}
	f.call("POST", "/auth/reset-password", payload, nil, 400)
	payload["code"] = code
	f.call("POST", "/auth/reset-password", payload, nil, 200)
	var count int64
	f.db.Model(&model.Session{}).Where("user_id = ?", f.learner.ID).Count(&count)
	if count != 0 {
		t.Fatal("password reset did not revoke sessions")
	}
	f.call("POST", "/auth/reset-password", payload, nil, 400)
	f.call("POST", "/auth/login", gin.H{"username": "security_learner", "password": "Original-password"}, nil, 401)
	f.call("POST", "/auth/login", gin.H{"username": "security_learner", "password": "Replacement-password"}, nil, 200)
}

func TestPasswordResetRejectsExpiredExhaustedAndFailedDelivery(t *testing.T) {
	f := newSecurityFixture(t)
	mailer := &testMailer{configured: true, fail: true}
	f.s.mailer = mailer
	f.call("POST", "/auth/reset-code", gin.H{"email": "security@example.com"}, nil, 200)
	payload := gin.H{"email": "security@example.com", "code": mailer.code, "password": "Replacement-password"}
	f.call("POST", "/auth/reset-password", payload, nil, 400)
	hash, _ := bcrypt.GenerateFromPassword([]byte("987654"), bcrypt.MinCost)
	f.db.Model(&model.PasswordReset{}).Where("email = ?", "security@example.com").Updates(map[string]any{"ready": true, "code_hash": string(hash), "expires_at": time.Now().Add(-time.Minute)})
	payload["code"] = "987654"
	f.call("POST", "/auth/reset-password", payload, nil, 400)
	f.db.Model(&model.PasswordReset{}).Where("email = ?", "security@example.com").Update("expires_at", time.Now().Add(time.Minute))
	payload["code"] = "000000"
	for i := 0; i < 5; i++ {
		f.call("POST", "/auth/reset-password", payload, nil, 400)
	}
	payload["code"] = "987654"
	f.call("POST", "/auth/reset-password", payload, nil, 400)
}

func TestAuditFailurePreventsAdministrativeMutation(t *testing.T) {
	f := newSecurityFixture(t)
	if err := f.db.Migrator().DropTable(&model.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	f.call("PATCH", fmt.Sprintf("/admin/users/%d", f.learner.ID), gin.H{"banned": true, "reason": "审计失败测试"}, f.adminCookie, 500)
	var user model.User
	f.db.First(&user, f.learner.ID)
	if user.Banned {
		t.Fatal("mutation ran without audit record")
	}
}

func TestCommunityFailedInsertRollsBackReceipt(t *testing.T) {
	f := newSecurityFixture(t)
	if err := f.db.Migrator().DropTable(&model.ArticleComment{}); err != nil {
		t.Fatal(err)
	}
	f.call("POST", fmt.Sprintf("/articles/%d/comments", f.article.ID), gin.H{"body": "不能保存"}, f.cookie, 500)
	var count int64
	f.db.Model(&model.CommunitySubmission{}).Where("user_id = ?", f.learner.ID).Count(&count)
	if count != 0 {
		t.Fatal("failed insert consumed posting quota")
	}
}
