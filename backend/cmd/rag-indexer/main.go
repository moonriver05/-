package main

import (
	"context"
	"flag"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"log"
	"mathwiki/internal/config"
	"mathwiki/internal/rag"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	limit := flag.Int("limit", 1000, "total prefix of chunks to index (1–10000); identical chunks skipped")
	articleID := flag.Uint("article-id", 0, "optional public article ID to index; 0 scans the knowledge base prefix")
	flag.Parse()
	cfg := config.Load()
	db, err := gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{})
	if err != nil {
		log.Fatal("database connection failed; check MYSQL_DSN")
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("database unavailable")
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(1)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := rag.FromEnv()
	if *articleID > 0 {
		log.Printf("RAG 定向建库：词条 %d", *articleID)
	}
	if err := client.IndexArticle(ctx, db, *limit, *articleID, func(total, added int) { log.Printf("RAG 已处理 %d 段，本次新增 %d 段", total, added) }); err != nil {
		log.Fatal(err)
	}
	count, err := client.Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("RAG 建库完成，当前集合 %d 段；请检查 docker stats 后再扩大规模", count)
}
