package api

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

func normalizeEmail(raw string) (string, bool) {
	email := strings.ToLower(strings.TrimSpace(raw))
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", false
	}
	for _, r := range email {
		if r > 127 {
			return "", false
		}
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 || len(parts[0]) > 64 || !strings.Contains(parts[1], ".") {
		return "", false
	}
	return email, true
}
func (s *Server) sendRegistrationCode(c *gin.Context) {
	var input struct {
		Email string `json:"email"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "请输入有效邮箱"})
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok {
		c.JSON(400, gin.H{"error": "请输入有效邮箱地址"})
		return
	}
	if !s.mailer.Configured() {
		c.JSON(503, gin.H{"error": "管理员尚未配置发信服务，暂时无法注册"})
		return
	}
	var count int64
	if err := s.db.Model(&model.User{}).Where("email = ?", email).Count(&count).Error; err != nil {
		fail(c, err)
		return
	}
	if count > 0 {
		c.JSON(409, gin.H{"error": "该邮箱已注册，请直接登录"})
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		fail(c, err)
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	hash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		fail(c, err)
		return
	}
	now := time.Now()
	retryAfter := 0
	err = s.db.Transaction(func(tx *gorm.DB) error {
		seed := model.EmailVerification{Email: email, SentAt: now.Add(-time.Hour), ExpiresAt: now, WindowStart: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var v model.EmailVerification
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, "email = ?", email).Error; err != nil {
			return err
		}
		if now.Before(v.SentAt.Add(time.Minute)) {
			retryAfter = int(v.SentAt.Add(time.Minute).Sub(now).Seconds()) + 1
			return nil
		}
		if now.Sub(v.WindowStart) >= time.Hour {
			v.WindowStart = now
			v.SendCount = 0
		}
		if v.SendCount >= 10 {
			retryAfter = int(v.WindowStart.Add(time.Hour).Sub(now).Seconds()) + 1
			return nil
		}
		v.CodeHash = string(hash)
		v.Ready = false
		v.ExpiresAt = now.Add(10 * time.Minute)
		v.SentAt = now
		v.Attempts = 0
		v.SendCount++
		return tx.Save(&v).Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	if retryAfter > 0 {
		c.Header("Retry-After", fmt.Sprint(retryAfter))
		c.JSON(429, gin.H{"error": "验证码发送过于频繁，请稍后重试", "retryAfter": retryAfter})
		return
	}
	if err = s.mailer.SendCode(c.Request.Context(), email, code); err != nil {
		// Keep the quota/cooldown but invalidate failed delivery. Never log SMTP credentials or the code.
		s.db.Model(&model.EmailVerification{}).Where("email = ? AND code_hash = ?", email, string(hash)).Updates(map[string]any{"code_hash": "", "ready": false})
		c.JSON(502, gin.H{"error": "邮件发送失败，请稍后重试或联系管理员检查 SMTP 配置"})
		return
	}
	result := s.db.Model(&model.EmailVerification{}).Where("email = ? AND code_hash = ?", email, string(hash)).Update("ready", true)
	if result.Error != nil || result.RowsAffected != 1 {
		c.JSON(503, gin.H{"error": "验证码保存失败，请稍后重新发送"})
		return
	}
	c.JSON(200, gin.H{"message": "验证码已发送，请检查收件箱及垃圾邮件", "retryAfter": 60, "expiresIn": 600})
}

var errEmailCode = errors.New("验证码不正确、已过期或尝试次数过多，请重新获取")

// The verification row is locked until account creation and code consumption commit together.
func (s *Server) createVerifiedUser(u *model.User, code string) error {
	var verificationErr error
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var v model.EmailVerification
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, "email = ?", *u.Email).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				verificationErr = errEmailCode
				return nil
			}
			return err
		}
		if !v.Ready || time.Now().After(v.ExpiresAt) || v.Attempts >= 5 || v.CodeHash == "" {
			verificationErr = errEmailCode
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(v.CodeHash), []byte(code)) != nil {
			verificationErr = errEmailCode
			return tx.Model(&v).Update("attempts", v.Attempts+1).Error
		}
		now := time.Now()
		u.EmailVerifiedAt = &now
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		return tx.Model(&v).Updates(map[string]any{"code_hash": "", "ready": false}).Error
	})
	if err != nil {
		return err
	}
	return verificationErr
}
