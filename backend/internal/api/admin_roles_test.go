package api

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"mathwiki/internal/model"
)

func TestAdministratorCanGrantAndRevokeOtherUsersWithAudit(t *testing.T) {
	f := newSecurityFixture(t)
	path := fmt.Sprintf("/admin/users/%d/role", f.learner.ID)
	input := gin.H{"role": "admin", "reason": "协助内容审核"}
	f.call("PATCH", path, input, nil, 401)
	f.call("PATCH", path, input, f.cookie, 403)
	f.call("PATCH", path, gin.H{"role": "owner", "reason": "测试"}, f.adminCookie, 400)
	f.call("PATCH", path, gin.H{"role": "admin", "reason": " "}, f.adminCookie, 400)
	f.call("PATCH", path, gin.H{"role": "admin", "reason": strings.Repeat("字", 201)}, f.adminCookie, 400)
	f.call("PATCH", fmt.Sprintf("/admin/users/%d/role", f.admin.ID), input, f.adminCookie, 409)
	f.call("PATCH", "/admin/users/999999/role", input, f.adminCookie, 404)
	f.db.Model(&f.learner).Update("banned", true)
	f.call("PATCH", path, input, f.adminCookie, 409)
	f.db.Model(&f.learner).Update("banned", false)
	demo := model.User{Name: "无可登录身份", Role: "learner"}
	f.db.Create(&demo)
	f.call("PATCH", fmt.Sprintf("/admin/users/%d/role", demo.ID), input, f.adminCookie, 409)
	f.call("PATCH", path, input, f.adminCookie, 200)
	var target model.User
	f.db.First(&target, f.learner.ID)
	if target.Role != "admin" {
		t.Fatal("promotion not stored")
	}
	var count int64
	f.db.Model(&model.Session{}).Where("user_id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Fatal("promotion kept old sessions")
	}
	f.call("GET", "/admin/users", nil, f.cookie, 401)
	promotedCookie := f.session(target.ID, "c")
	f.call("GET", "/admin/users", nil, promotedCookie, 200)
	// A repeated assignment is an explicit audited no-op, not another logout.
	f.call("PATCH", path, input, f.adminCookie, 200)
	f.call("GET", "/admin/users", nil, promotedCookie, 200)
	f.call("PATCH", path, gin.H{"role": "learner", "reason": "审核职责结束"}, f.adminCookie, 200)
	f.call("GET", "/admin/users", nil, promotedCookie, 401)
	learnerCookie := f.session(target.ID, "d")
	f.call("GET", "/admin/users", nil, learnerCookie, 403)
	f.db.First(&target, target.ID)
	if target.Role != "learner" {
		t.Fatal("revocation not stored")
	}
	for _, action := range []string{"GRANT_ADMIN", "REVOKE_ADMIN", "ROLE_NO_CHANGE"} {
		f.db.Model(&model.AdminAudit{}).Where("action = ? AND user_id = ? AND target = ? AND status = ?", action+" /api/v1/admin/users/:id/role", f.admin.ID, fmt.Sprint(target.ID), 200).Count(&count)
		if count != 1 {
			t.Fatalf("missing successful audit: %s", action)
		}
	}
}

func TestRoleChangeRollsBackWithoutAuditAndRechecksActor(t *testing.T) {
	f := newSecurityFixture(t)
	path := fmt.Sprintf("/admin/users/%d/role", f.learner.ID)
	if err := f.db.Callback().Update().Before("gorm:update").Register("test:fail_role_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "admin_audits" {
			tx.AddError(errors.New("test audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	f.call("PATCH", path, gin.H{"role": "admin", "reason": "不能脱离审计"}, f.adminCookie, 500)
	f.db.Callback().Update().Remove("test:fail_role_audit")
	var target model.User
	f.db.First(&target, f.learner.ID)
	var sessions int64
	f.db.Model(&model.Session{}).Where("user_id = ?", target.ID).Count(&sessions)
	if target.Role != "learner" || sessions != 1 {
		t.Fatal("failed audit did not roll back role and sessions")
	}
	// Simulate a role change between authentication and transaction admission.
	f.db.Model(&f.admin).Update("role", "learner")
	r := gin.New()
	r.PATCH("/api/v1/admin/users/:id/role", func(c *gin.Context) { c.Set("user", model.User{ID: f.admin.ID, Role: "admin"}) }, f.s.auditAdmin, requireAdmin, f.s.changeUserRole)
	f.router = r
	f.call("PATCH", path, gin.H{"role": "admin", "reason": "旧权限不能授权"}, nil, 403)
	f.db.First(&target, target.ID)
	if target.Role != "learner" {
		t.Fatal("stale administrator granted a role")
	}
}
