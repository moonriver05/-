package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"image"
	"image/png"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type avatarFake struct {
	enabled, fail bool
	key           string
	data          []byte
}

func (f *avatarFake) Configured() bool { return f.enabled }
func (f *avatarFake) Put(_ context.Context, key string, data []byte) error {
	if f.fail {
		return errors.New("fake failure")
	}
	f.key = key
	f.data = data
	return nil
}
func (f *avatarFake) PublicURL(key string) (string, error) {
	return "https://test.cos.ap-guangzhou.myqcloud.com/mathwiki/avatars/" + key, nil
}

func TestProfileAndAvatar(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "profile.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	u := model.User{Name: "原昵称", Role: "learner"}
	other := model.User{Name: "其他用户", Role: "admin"}
	if err = db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&other)
	token := strings.Repeat("a", 64)
	db.Create(&model.Session{ID: tokenHash(token), UserID: u.ID, ExpiresAt: time.Now().Add(time.Hour)})
	s := New(db, nil, ai.New("", "", "demo"), "mock")
	fake := &avatarFake{enabled: true}
	s.avatars = fake
	r := s.Router()
	call := func(method, path, contentType string, body []byte, authenticated, csrf bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if authenticated {
			req.Header.Set("Cookie", sessionCookie+"="+token)
		}
		if csrf {
			req.Header.Set("X-MathWiki-Request", "1")
		}
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s %s got %d want %d: %s", method, path, res.Code, want, res.Body.String())
		}
		return res
	}
	profile := func(input string, want int) {
		t.Helper()
		call("PUT", "/auth/profile", "application/json", []byte(input), true, true, want)
	}
	call("PUT", "/auth/profile", "application/json", []byte(`{"name":"new"}`), false, true, 401)
	call("PUT", "/auth/profile", "application/json", []byte(`{"name":"new"}`), true, false, 403)
	profile(`{"name":" 新昵称 ","bio":" 学数学 ","location":"北京","website":"https://example.com","role":"admin","id":2,"avatar":"https://evil.example/x"}`, 200)
	var got model.User
	db.First(&got, u.ID)
	if got.Name != "新昵称" || got.Bio != "学数学" || got.Location != "北京" || got.Website != "https://example.com" || got.Role != "learner" || got.Avatar != "" {
		t.Fatal("profile allowlist or normalization failed")
	}
	db.First(&other, other.ID)
	if other.Name != "其他用户" {
		t.Fatal("another account changed")
	}
	for _, input := range []string{`{"name":""}`, `{"name":"x","website":"javascript:alert(1)"}`, `{"name":"x","website":"https://user:pass@example.com"}`, `{"name":"x","bio":"` + strings.Repeat("中", 401) + `"}`} {
		profile(input, 400)
	}
	upload := func(data []byte, authenticated, csrf bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, _ := w.CreateFormFile("avatar", "photo.png")
		part.Write(data)
		w.WriteField("userId", "2")
		w.Close()
		return call("PUT", "/auth/avatar", w.FormDataContentType(), buf.Bytes(), authenticated, csrf, want)
	}
	var raw bytes.Buffer
	png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 400, 200)))
	upload(raw.Bytes(), false, true, 401)
	upload(raw.Bytes(), true, false, 403)
	fake.enabled = false
	upload(raw.Bytes(), true, true, 503)
	fake.enabled = true
	upload([]byte(`<svg onload="alert(1)"></svg>`), true, true, 400)
	upload(bytes.Repeat([]byte("x"), avatarMaxUpload+1), true, true, 413)
	res := upload(raw.Bytes(), true, true, 200)
	var payload struct{ User model.User }
	json.Unmarshal(res.Body.Bytes(), &payload)
	if payload.User.Avatar == "" || payload.User.Role != "learner" || !strings.HasPrefix(fake.key, "1/") {
		t.Fatal("avatar ownership or response failed")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(fake.data))
	if err != nil || format != "jpeg" || cfg.Width != 256 || cfg.Height != 256 {
		t.Fatal("avatar not normalized to 256 JPEG")
	}
	first := payload.User.Avatar
	fake.fail = true
	upload(raw.Bytes(), true, true, 502)
	db.First(&got, u.ID)
	if got.Avatar != first {
		t.Fatal("failed upload changed avatar")
	}
	fake.fail = false
	upload(raw.Bytes(), true, true, 200)
	db.First(&got, u.ID)
	if got.Avatar == first {
		t.Fatal("new avatar should have a new cache-safe URL")
	}
	db.First(&other, other.ID)
	if other.Avatar != "" {
		t.Fatal("changed other avatar")
	}
	if toCommentResponse(model.Comment{User: got}).Avatar != got.Avatar || articleCommentDTO(model.ArticleComment{User: got}).Avatar != got.Avatar {
		t.Fatal("community DTO missing avatar")
	}
	call("DELETE", "/auth/avatar", "application/json", nil, true, true, 200)
	db.First(&got, u.ID)
	var count int64
	db.Model(&model.UserAvatar{}).Where("user_id = ?", u.ID).Count(&count)
	if got.Avatar != "" || count != 0 {
		t.Fatal("reset did not remove current avatar reference")
	}
	s.limiter.entries["avatar:1"] = attempt{Count: 10, Until: time.Now().Add(time.Minute)}
	upload(raw.Bytes(), true, true, 429)
}

func TestAvatarDecoderLimits(t *testing.T) {
	var b bytes.Buffer
	png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4097, 1)))
	if _, err := avatarJPEG(b.Bytes()); err == nil {
		t.Fatal("oversized dimensions accepted")
	}
	if _, err := avatarJPEG([]byte("not an image")); err == nil {
		t.Fatal("invalid image accepted")
	}
}
