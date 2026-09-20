package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"mathwiki/internal/model"
)

const sessionCookie = "mathwiki_session"

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,40}$`)
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-account-password"), bcrypt.DefaultCost)

func userID(c *gin.Context) uint { return currentUser(c).ID }
func currentUser(c *gin.Context) model.User {
	if u, ok := c.Get("user"); ok {
		return u.(model.User)
	}
	return model.User{}
}
func isAdmin(c *gin.Context) bool { return currentUser(c).Role == "admin" }
func requireLogin(c *gin.Context) {
	if userID(c) == 0 {
		c.AbortWithStatusJSON(401, gin.H{"error": "请先登录账户"})
	}
}
func requireAdmin(c *gin.Context) {
	requireLogin(c)
	if !c.IsAborted() && !isAdmin(c) {
		c.AbortWithStatusJSON(403, gin.H{"error": "此操作需要管理员权限"})
	}
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) authenticate(c *gin.Context) {
	// A custom header prevents cross-site form requests; CORS grants no foreign origins.
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" && c.GetHeader("X-MathWiki-Request") != "1" {
		c.AbortWithStatusJSON(403, gin.H{"error": "请求校验失败，请刷新页面重试"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	c.Header("Cache-Control", "no-store")
	if token, err := c.Cookie(sessionCookie); err == nil && len(token) == 64 {
		var session model.Session
		err = s.db.Where("id = ? AND expires_at > ?", tokenHash(token), time.Now()).First(&session).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(503, gin.H{"error": "账户服务暂时不可用"})
			return
		}
		if err == nil {
			var u model.User
			if err = s.db.First(&u, session.UserID).Error; err != nil {
				c.AbortWithStatusJSON(503, gin.H{"error": "账户服务暂时不可用"})
				return
			}
			c.Set("user", u)
		}
	}
	c.Next()
}

type attempt struct {
	Count int
	Until time.Time
}
type authLimiter struct {
	sync.Mutex
	entries map[string]attempt
}

func (s *Server) authRateLimit(c *gin.Context) {
	ip, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	s.limiter.Lock()
	defer s.limiter.Unlock()
	now := time.Now()
	for k, v := range s.limiter.entries {
		if now.After(v.Until) {
			delete(s.limiter.entries, k)
		}
	}
	entry := s.limiter.entries[ip]
	if entry.Count >= 20 {
		c.AbortWithStatusJSON(429, gin.H{"error": "操作过于频繁，请在 15 分钟后重试"})
		return
	}
	if entry.Count == 0 {
		entry.Until = now.Add(15 * time.Minute)
	}
	entry.Count++
	s.limiter.entries[ip] = entry
}

func (s *Server) setSession(c *gin.Context, u model.User) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fail(c, err)
		return
	}
	token := hex.EncodeToString(raw)
	session := model.Session{ID: tokenHash(token), UserID: u.ID, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
	if err := s.db.Create(&session).Error; err != nil {
		fail(c, err)
		return
	}
	if old, err := c.Cookie(sessionCookie); err == nil {
		s.db.Where("id = ?", tokenHash(old)).Delete(&model.Session{})
	}
	s.db.Where("expires_at < ?", time.Now()).Delete(&model.Session{})
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookie, token, 7*24*3600, "/", "", os.Getenv("AUTH_COOKIE_SECURE") == "true", true)
	c.JSON(200, gin.H{"user": u})
}
func (s *Server) register(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "账户格式不正确"})
		return
	}
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.Name = strings.TrimSpace(input.Name)
	email, validEmail := normalizeEmail(input.Email)
	if !validEmail || len(input.Code) != 6 || strings.Trim(input.Code, "0123456789") != "" {
		c.JSON(400, gin.H{"error": "请填写有效邮箱和 6 位邮箱验证码"})
		return
	}
	if !usernamePattern.MatchString(input.Username) || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 40 || utf8.RuneCountInString(input.Password) < 8 || len(input.Password) > 72 {
		c.JSON(400, gin.H{"error": "账号需为 3–40 位字母、数字或下划线；昵称 1–40 字；密码至少 8 字且不超过 72 字节"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, err)
		return
	}
	u := model.User{Username: &input.Username, Email: &email, Name: input.Name, PasswordHash: string(hash), Role: "learner"}
	if err = s.createVerifiedUser(&u, input.Code); err != nil {
		if errors.Is(err, errEmailCode) {
			c.JSON(400, gin.H{"error": errEmailCode.Error()})
			return
		}
		var count int64
		s.db.Model(&model.User{}).Where("username = ? OR email = ?", input.Username, email).Count(&count)
		if count > 0 {
			c.JSON(409, gin.H{"error": "该账号或邮箱已被注册，请直接登录"})
			return
		}
		fail(c, err)
		return
	}
	s.setSession(c, u)
}
func (s *Server) login(c *gin.Context) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil || len(input.Password) > 72 {
		c.JSON(400, gin.H{"error": "账户格式不正确"})
		return
	}
	var u model.User
	err := s.db.Where("username = ?", strings.ToLower(strings.TrimSpace(input.Username))).First(&u).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		fail(c, err)
		return
	}
	hash := []byte(u.PasswordHash)
	if len(hash) == 0 {
		hash = dummyHash
	}
	valid := bcrypt.CompareHashAndPassword(hash, []byte(input.Password)) == nil
	if err != nil || u.PasswordHash == "" || !valid {
		c.JSON(401, gin.H{"error": "账号或密码不正确"})
		return
	}
	s.setSession(c, u)
}
func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(sessionCookie); err == nil {
		if err = s.db.Where("id = ?", tokenHash(token)).Delete(&model.Session{}).Error; err != nil {
			fail(c, err)
			return
		}
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", os.Getenv("AUTH_COOKIE_SECURE") == "true", true)
	c.JSON(200, gin.H{"ok": true})
}
