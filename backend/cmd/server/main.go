package main

import (
	"log"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"mathwiki/internal/ai"
	"mathwiki/internal/api"
	"mathwiki/internal/config"
	"mathwiki/internal/model"
	"mathwiki/internal/search"
	"mathwiki/internal/seed"
)

func main() {
	cfg := config.Load()
	var dialector gorm.Dialector
	if strings.EqualFold(cfg.DatabaseDriver, "sqlite") {
		dialector = sqlite.Open(cfg.SQLitePath)
	} else {
		dialector = mysql.Open(cfg.MySQLDSN)
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	if err := model.Migrate(db); err != nil {
		log.Fatalf("迁移数据库失败: %v", err)
	}
	if err := seed.Ensure(db, cfg.DemoSeed); err != nil {
		log.Fatalf("初始化中文演示数据失败: %v", err)
	}
	if sqlDB, poolErr := db.DB(); poolErr == nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(50)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	server := api.New(db, search.New(cfg.ElasticsearchURL, cfg.ElasticsearchIndex), ai.New(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel), cfg.STTProvider)
	log.Printf("MathWiki 中文 API 已启动: http://localhost:%s", cfg.Port)
	if err := server.Router().Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}
