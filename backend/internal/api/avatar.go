package api

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mathwiki/internal/model"
)

const avatarMaxUpload = 5 << 20

func (s *Server) avatarRateLimit(c *gin.Context) {
	s.limiter.Lock()
	defer s.limiter.Unlock()
	now := time.Now()
	for k, v := range s.limiter.entries {
		if now.After(v.Until) {
			delete(s.limiter.entries, k)
		}
	}
	key := fmt.Sprintf("avatar:%d", userID(c))
	entry := s.limiter.entries[key]
	if entry.Count >= 10 {
		c.Header("Retry-After", strconv.Itoa(int(time.Until(entry.Until).Seconds())+1))
		c.AbortWithStatusJSON(429, gin.H{"error": "更换头像过于频繁，请稍后重试"})
		return
	}
	if entry.Count == 0 {
		entry.Until = now.Add(15 * time.Minute)
	}
	entry.Count++
	s.limiter.entries[key] = entry
}

func avatarJPEG(raw []byte) ([]byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, fmt.Errorf("请选择有效的 JPG 或 PNG 图片")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return nil, fmt.Errorf("图片过大，请选择不超过 4096×4096 且总像素不超过 1200 万的图片")
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("图片损坏或无法读取")
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	left := b.Min.X + (b.Dx()-side)/2
	top := b.Min.Y + (b.Dy()-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, 256, 256))
	// Bilinear sampling of the centered square, flattening transparent pixels onto white.
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			sx := math.Max(0, (float64(x)+0.5)*float64(side)/256-0.5)
			sy := math.Max(0, (float64(y)+0.5)*float64(side)/256-0.5)
			x0, y0 := int(sx), int(sy)
			x1, y1 := min(x0+1, side-1), min(y0+1, side-1)
			fx, fy := sx-float64(x0), sy-float64(y0)
			coords := [4][2]int{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}}
			weights := [4]float64{(1 - fx) * (1 - fy), fx * (1 - fy), (1 - fx) * fy, fx * fy}
			var r, g, bl float64
			for i, p := range coords {
				rr, gg, bb, aa := src.At(left+p[0], top+p[1]).RGBA()
				r += float64(rr+65535-aa) * weights[i]
				g += float64(gg+65535-aa) * weights[i]
				bl += float64(bb+65535-aa) * weights[i]
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(math.Min(255, r/257)), uint8(math.Min(255, g/257)), uint8(math.Min(255, bl/257)), 255})
		}
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *Server) uploadAvatar(c *gin.Context) {
	if !s.avatars.Configured() {
		c.JSON(503, gin.H{"error": "尚未配置腾讯云 COS，暂时无法上传头像"})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, avatarMaxUpload+(64<<10))
	if err := c.Request.ParseMultipartForm(avatarMaxUpload + (64 << 10)); err != nil {
		c.JSON(400, gin.H{"error": "上传失败，请选择不超过 5 MB 的 JPG/PNG 图片"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	file, header, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(400, gin.H{"error": "请选择头像图片"})
		return
	}
	defer file.Close()
	if header.Size > avatarMaxUpload {
		c.JSON(413, gin.H{"error": "头像图片不能超过 5 MB"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(file, avatarMaxUpload+1))
	if err != nil || len(raw) > avatarMaxUpload {
		c.JSON(413, gin.H{"error": "头像图片无法读取或超过 5 MB"})
		return
	}
	data, err := avatarJPEG(raw)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	nonce := make([]byte, 16)
	if _, err = rand.Read(nonce); err != nil {
		fail(c, err)
		return
	}
	version := hex.EncodeToString(nonce)
	var u model.User
	key := fmt.Sprintf("%d/%s.jpg", userID(c), version)
	target, err := s.avatars.PublicURL(key)
	if err != nil {
		c.JSON(503, gin.H{"error": "头像存储配置不可用"})
		return
	}
	if err = s.avatars.Put(c.Request.Context(), key, data); err != nil {
		c.JSON(502, gin.H{"error": "头像上传到 COS 失败，请稍后重试或检查存储配置"})
		return
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID(c)).Error; err != nil {
			return err
		}
		item := model.UserAvatar{UserID: u.ID, Version: version, ObjectKey: key}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"version", "object_key"})}).Create(&item).Error; err != nil {
			return err
		}
		u.Avatar = target
		return tx.Model(&u).Update("avatar", u.Avatar).Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"user": u})
}
func (s *Server) resetAvatar(c *gin.Context) {
	var u model.User
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&u, userID(c)).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", u.ID).Delete(&model.UserAvatar{}).Error; err != nil {
			return err
		}
		u.Avatar = ""
		return tx.Model(&u).Update("avatar", "").Error
	})
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"user": u})
}
