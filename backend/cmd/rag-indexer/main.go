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
	planOnly := flag.Bool("plan", false, "estimate entire public knowledge base without embedding/API calls")
	coverage := flag.Bool("coverage", false, "index the first valid chunk of every public article")
	all := flag.Bool("all", false, "index all indexable public chunks within -max-records budget")
	maxRecords := flag.Int("max-records", 10000, "collection record budget for -coverage/-all (1–100000; not a RAM reservation)")
	flag.Parse()
	modes := 0
	for _, on := range []bool{*planOnly, *coverage, *all, *articleID > 0} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		log.Fatal("use only one of -plan, -coverage, -all, -article-id")
	}
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
	if *planOnly {
		plan, err := rag.Estimate(ctx, db)
		if err != nil {
			log.Fatal("database estimate failed")
		}
		log.Printf("规模预估：%d 篇可索引词条，%d 个有效章节，%d 个正文片段", plan.Articles, plan.Sections, plan.Chunks)
		log.Printf("仅1024维向量原始数据：全正文约 %.1f MiB；每篇一段覆盖约 %.1f MiB；均不含服务和索引开销", plan.RawVectorMiB, float64(plan.Articles)*1024*4/(1024*1024))
		log.Printf("%d 个章节达到12000字截取上限；本次未调用embedding、未修改索引", plan.CappedSections)
		return
	}
	if *articleID > 0 {
		log.Printf("RAG 定向建库：词条 %d", *articleID)
	}
	progress := func(total, added int) { log.Printf("RAG 已处理 %d 段，本次新增 %d 段", total, added) }
	if *coverage {
		err = client.IndexCoverage(ctx, db, *maxRecords, progress)
	} else if *all {
		err = client.IndexAll(ctx, db, *maxRecords, progress)
	} else {
		err = client.IndexArticle(ctx, db, *limit, *articleID, progress)
	}
	if err != nil {
		log.Fatal(err)
	}
	count, err := client.Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("RAG 建库完成，当前集合 %d 段；请检查 docker stats 后再扩大规模", count)
}
