package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

func (s *Server) sendPasswordResetCode(c *gin.Context) {
	var input struct {
		Email string `json:"email"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "请输入有效邮箱"})
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok {
		c.JSON(400, gin.H{"error": "请输入有效邮箱"})
		return
	}
	sender, supported := s.mailer.(interface {
		SendResetCode(context.Context, string, string) error
	})
	if !s.mailer.Configured() || !supported {
		c.JSON(503, gin.H{"error": "密码找回发信服务暂不可用，请联系管理员"})
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
	retry := 0
	err = s.db.Transaction(func(tx *gorm.DB) error {
		seed := model.PasswordReset{EmailVerification: model.EmailVerification{Email: email, SentAt: now.Add(-time.Hour), WindowStart: now, ExpiresAt: now}}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var v model.PasswordReset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, "email = ?", email).Error; err != nil {
			return err
		}
		if now.Before(v.SentAt.Add(time.Minute)) {
			retry = int(v.SentAt.Add(time.Minute).Sub(now).Seconds()) + 1
			return nil
		}
		if now.Sub(v.WindowStart) >= time.Hour {
			v.WindowStart = now
			v.SendCount = 0
		}
		if v.SendCount >= 5 {
			retry = int(v.WindowStart.Add(time.Hour).Sub(now).Seconds()) + 1
			return nil
		}
		v.CodeHash = string(hash)
		v.Ready = false
		v.SentAt = now
		v.ExpiresAt = now.Add(10 * time.Minute)
		v.Attempts = 0
		v.SendCount++
		return tx.Save(&v).Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	if retry > 0 {
		c.Header("Retry-After", fmt.Sprint(retry))
		c.JSON(429, gin.H{"error": "请求过于频繁，请稍后再试"})
		return
	}
	var eligible int64
	if err = s.db.Model(&model.User{}).Where("email = ? AND email_verified_at IS NOT NULL AND banned = ?", email, false).Count(&eligible).Error; err != nil {
		fail(c, err)
		return
	}
	// Use identical public responses for unknown, unverified and banned addresses.
	if eligible > 0 && sender.SendResetCode(c.Request.Context(), email, code) == nil {
		if err = s.db.Model(&model.PasswordReset{}).Where("email = ? AND code_hash = ?", email, string(hash)).Update("ready", true).Error; err != nil {
			fail(c, err)
			return
		}
	}
	c.JSON(200, gin.H{"message": "如果该邮箱绑定了可用账户，你将收到密码重置验证码；未收到请稍后重试或联系管理员", "retryAfter": 60, "expiresIn": 600})
}

func (s *Server) resetPassword(c *gin.Context) {
	var input struct {
		Email    string `json:"email"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "请求格式不正确"})
		return
	}
	email, ok := normalizeEmail(input.Email)
	if !ok || len(input.Code) != 6 || strings.Trim(input.Code, "0123456789") != "" || utf8.RuneCountInString(input.Password) < 8 || len(input.Password) > 72 {
		c.JSON(400, gin.H{"error": "请填写有效邮箱、6 位验证码及至少 8 字且不超过 72 字节的新密码"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, err)
		return
	}
	var codeErr error
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// User first: serialize session issuance, banning and password changes.
		var u model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ? AND email_verified_at IS NOT NULL AND banned = ?", email, false).First(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				codeErr = errEmailCode
				return nil
			}
			return err
		}
		var v model.PasswordReset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&v, "email = ?", email).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				codeErr = errEmailCode
				return nil
			}
			return err
		}
		if !v.Ready || v.Attempts >= 5 || time.Now().After(v.ExpiresAt) {
			codeErr = errEmailCode
			return nil
		}
		if bcrypt.CompareHashAndPassword([]byte(v.CodeHash), []byte(input.Code)) != nil {
			codeErr = errEmailCode
			return tx.Model(&v).Update("attempts", v.Attempts+1).Error
		}
		if err := tx.Model(&u).Update("password_hash", string(hash)).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&model.Session{}).Error; err != nil {
			return err
		}
		return tx.Model(&v).Updates(map[string]any{"ready": false, "code_hash": ""}).Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	if codeErr != nil {
		c.JSON(400, gin.H{"error": codeErr.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "密码已重置，所有旧登录已退出，请使用新密码登录"})
}
