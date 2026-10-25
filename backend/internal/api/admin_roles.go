package api

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

var errRoleActor = errors.New("acting administrator no longer authorized")
var errRoleTarget = errors.New("target cannot receive role changes")

func (s *Server) changeUserRole(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	id, valid := parseID(c.Param("id"))
	var input struct {
		Role   string `json:"role"`
		Reason string `json:"reason"`
	}
	if !valid || c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "角色请求格式不正确"})
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if (input.Role != "admin" && input.Role != "learner") || input.Reason == "" || utf8.RuneCountInString(input.Reason) > 200 {
		c.JSON(400, gin.H{"error": "请选择管理员或普通用户，并填写 1–200 字原因"})
		return
	}
	actorID := userID(c)
	if id == actorID {
		c.JSON(409, gin.H{"error": "不能修改自己的管理员身份，请由其他管理员操作"})
		return
	}
	changed := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Lock both accounts in ID order, then re-check authority inside the same
		// transaction. Concurrent mutual demotions cannot remove every administrator.
		var accounts []model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []uint{actorID, id}).Order("id ASC").Find(&accounts).Error; err != nil {
			return err
		}
		var actor, target *model.User
		for i := range accounts {
			if accounts[i].ID == actorID {
				actor = &accounts[i]
			}
			if accounts[i].ID == id {
				target = &accounts[i]
			}
		}
		if actor == nil || actor.Role != "admin" || actor.Banned {
			return errRoleActor
		}
		if target == nil {
			return gorm.ErrRecordNotFound
		}
		if target.Banned || (target.Role != "admin" && target.Role != "learner") || (input.Role == "admin" && (target.Username == nil || target.PasswordHash == "")) {
			return errRoleTarget
		}
		auditID, exists := c.Get("auditID")
		if !exists {
			return errors.New("role change requires an audit receipt")
		}
		action := "ROLE_NO_CHANGE /api/v1/admin/users/:id/role"
		if target.Role != input.Role {
			if err := tx.Model(target).Update("role", input.Role).Error; err != nil {
				return err
			}
			// A changed privilege requires a new login; old sessions are removed
			// atomically with the role update, for promotion and revocation alike.
			if err := tx.Where("user_id = ?", id).Delete(&model.Session{}).Error; err != nil {
				return err
			}
			changed = true
			action = "GRANT_ADMIN /api/v1/admin/users/:id/role"
			if input.Role == "learner" {
				action = "REVOKE_ADMIN /api/v1/admin/users/:id/role"
			}
		}
		result := tx.Model(&model.AdminAudit{}).Where("id = ? AND user_id = ?", auditID, actorID).Updates(map[string]any{"action": action, "reason": input.Reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("missing role audit receipt")
		}
		return nil
	})
	switch {
	case errors.Is(err, errRoleActor):
		c.JSON(403, gin.H{"error": "管理员权限已变更，请重新登录"})
	case errors.Is(err, errRoleTarget):
		c.JSON(409, gin.H{"error": "请先解除封禁，并确认目标为可登录的正常账户"})
	case err != nil:
		respondDB(c, err)
	default:
		c.JSON(200, gin.H{"ok": true, "changed": changed, "message": "角色已确认；身份发生变化的账户需重新登录"})
	}
}
