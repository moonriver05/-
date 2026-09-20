package api

import (
	"github.com/gin-gonic/gin"
	"mathwiki/internal/model"
	"net/url"
	"strings"
	"unicode/utf8"
)

func (s *Server) updateProfile(c *gin.Context) {
	var input struct {
		Name     string `json:"name"`
		Bio      string `json:"bio"`
		Location string `json:"location"`
		Website  string `json:"website"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "个人资料格式不正确"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Bio = strings.TrimSpace(input.Bio)
	input.Location = strings.TrimSpace(input.Location)
	input.Website = strings.TrimSpace(input.Website)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 40 || utf8.RuneCountInString(input.Bio) > 400 || utf8.RuneCountInString(input.Location) > 80 || len(input.Website) > 512 {
		c.JSON(400, gin.H{"error": "昵称限 1–40 字，简介最多 400 字，所在地最多 80 字，网址最多 512 字节"})
		return
	}
	if input.Website != "" {
		u, err := url.Parse(input.Website)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
			c.JSON(400, gin.H{"error": "个人网站请填写完整的 http:// 或 https:// 地址"})
			return
		}
	}
	// Explicit allowlist: username, email, password, role and avatar cannot be changed here.
	err := s.db.Model(&model.User{}).Where("id = ?", userID(c)).Updates(map[string]any{"name": input.Name, "bio": input.Bio, "location": input.Location, "website": input.Website}).Error
	if err != nil {
		fail(c, err)
		return
	}
	var u model.User
	if err = s.db.First(&u, userID(c)).Error; err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"user": u})
}
