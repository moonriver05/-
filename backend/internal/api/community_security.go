package api

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

var errSubmissionDuplicate = errors.New("相同内容已提交，请勿重复发布")
var errSubmissionRate = errors.New("发布过于频繁，10 分钟内最多发布 20 条评论或批注")
var errAccountUnavailable = errors.New("账户不可用，请重新登录或联系管理员")

// Lock the user row so concurrent requests and multiple API instances share one quota.
// The receipt and content commit together; failed validation/storage consumes no quota.
func (s *Server) createCommunity(c *gin.Context, item any, fingerprint string) bool {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var u model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID(c)).Error; err != nil {
			return err
		}
		if u.Banned {
			return errAccountUnavailable
		}
		cutoff := time.Now().Add(-10 * time.Minute)
		q := tx.Model(&model.CommunitySubmission{}).Where("user_id = ? AND created_at > ?", u.ID, cutoff)
		var count int64
		if err := q.Where("fingerprint = ?", tokenHash(fingerprint)).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errSubmissionDuplicate
		}
		if err := tx.Model(&model.CommunitySubmission{}).Where("user_id = ? AND created_at > ?", u.ID, cutoff).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return errSubmissionRate
		}
		if err := tx.Where("user_id = ? AND created_at <= ?", u.ID, cutoff).Delete(&model.CommunitySubmission{}).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.CommunitySubmission{UserID: u.ID, Fingerprint: tokenHash(fingerprint)}).Error; err != nil {
			return err
		}
		return tx.Create(item).Error
	})
	switch {
	case err == nil:
		return true
	case errors.Is(err, errSubmissionDuplicate):
		c.JSON(409, gin.H{"error": err.Error()})
	case errors.Is(err, errSubmissionRate):
		c.Header("Retry-After", "600")
		c.JSON(429, gin.H{"error": err.Error()})
	case errors.Is(err, errAccountUnavailable):
		c.JSON(403, gin.H{"error": err.Error()})
	default:
		fail(c, err)
	}
	return false
}

func (s *Server) setAnnotationLike(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.JSON(400, gin.H{"error": "批注编号不正确"})
		return
	}
	liked := c.Request.Method != "DELETE"
	var likes int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var note model.Annotation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND article_id IN (?)", id, "visible", s.publicArticleIDs()).First(&note).Error; err != nil {
			return err
		}
		vote := model.AnnotationLike{AnnotationID: id, UserID: userID(c)}
		if liked {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&vote).Error; err != nil {
				return err
			}
		} else if err := tx.Where("annotation_id = ? AND user_id = ?", id, userID(c)).Delete(&model.AnnotationLike{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AnnotationLike{}).Where("annotation_id = ?", id).Count(&likes).Error; err != nil {
			return err
		}
		return tx.Model(&note).UpdateColumn("likes", likes).Error
	})
	if err != nil {
		respondDB(c, err)
		return
	}
	c.JSON(200, gin.H{"id": id, "likes": likes, "liked": liked})
}

// Store an attempt before executing an administrative mutation (fail closed).
// Status 0 means interrupted/unknown, never an asserted success. Bodies, passwords,
// email addresses and cookies are deliberately excluded from audit records.
func (s *Server) auditAdmin(c *gin.Context) {
	path := c.FullPath()
	mutation := c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS"
	administrative := strings.Contains(path, "/admin/") || path == "/api/v1/categories" || path == "/api/v1/articles" || (c.Request.Method == "DELETE" && (strings.Contains(path, "/annotations/") || strings.Contains(path, "/comments/") || strings.Contains(path, "/article-comments/")))
	if !isAdmin(c) || !mutation || !administrative {
		c.Next()
		return
	}
	entry := model.AdminAudit{UserID: userID(c), Action: c.Request.Method + " " + path, Target: c.Param("id")}
	if err := s.db.Create(&entry).Error; err != nil {
		c.Abort()
		fail(c, err)
		return
	}
	c.Set("auditID", entry.ID)
	c.Next()
	s.db.Model(&entry).Update("status", c.Writer.Status())
}

func (s *Server) adminUsers(c *gin.Context) {
	q := s.db.Model(&model.User{})
	if text := strings.TrimSpace(c.Query("q")); text != "" {
		q = q.Where("username = ? OR name = ?", text, text)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	items := []model.User{}
	if err := q.Order("id DESC").Offset((positiveInt(c.Query("page"), 1) - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total})
}

func (s *Server) banUser(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	var input struct {
		Banned *bool  `json:"banned"`
		Reason string `json:"reason"`
	}
	if !ok || c.ShouldBindJSON(&input) != nil || input.Banned == nil || strings.TrimSpace(input.Reason) == "" || utf8.RuneCountInString(input.Reason) > 200 {
		c.JSON(400, gin.H{"error": "请选择封禁状态并填写 1–200 字原因"})
		return
	}
	if id == userID(c) {
		c.JSON(409, gin.H{"error": "不能封禁自己的账户"})
		return
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var u model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, id).Error; err != nil {
			return err
		}
		if u.Role == "admin" {
			return errors.New("不能在此封禁管理员账户")
		}
		if err := tx.Model(&u).Update("banned", *input.Banned).Error; err != nil {
			return err
		}
		if *input.Banned {
			if err := tx.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
				return err
			}
		}
		auditID, exists := c.Get("auditID")
		if !exists {
			return errors.New("操作审计不可用")
		}
		action := "UNBAN /api/v1/admin/users/:id"
		if *input.Banned {
			action = "BAN /api/v1/admin/users/:id"
		}
		return tx.Model(&model.AdminAudit{}).Where("id = ?", auditID).Updates(map[string]any{"reason": strings.TrimSpace(input.Reason), "action": action}).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondDB(c, err)
		} else {
			c.JSON(409, gin.H{"error": "无法变更该账户，请确认目标不是管理员并重试"})
		}
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

func (s *Server) adminAudits(c *gin.Context) {
	var total int64
	if err := s.db.Model(&model.AdminAudit{}).Count(&total).Error; err != nil {
		fail(c, err)
		return
	}
	items := []model.AdminAudit{}
	if err := s.db.Order("id DESC").Offset((positiveInt(c.Query("page"), 1) - 1) * 50).Limit(50).Find(&items).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"items": items, "total": total})
}

func submissionKey(kind string, id uint, body string) string {
	return fmt.Sprintf("%s:%d:%s", kind, id, strings.Join(strings.Fields(body), " "))
}
