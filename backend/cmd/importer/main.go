package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mathwiki/internal/config"
	"mathwiki/internal/model"
)

type inputSection struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	Position    *int   `json:"position"`
	Level       int    `json:"level"`
	ContentHTML string `json:"contentHtml"`
}
type inputArticle struct {
	PageID          int64          `json:"pageId"`
	RevisionID      int64          `json:"revisionId"`
	Title           string         `json:"title"`
	SourceTitle     string         `json:"sourceTitle"`
	SourceURL       string         `json:"sourceUrl"`
	PlainText       string         `json:"plainText"`
	RawWikitext     string         `json:"rawWikitext"`
	License         string         `json:"license"`
	Roots           []string       `json:"roots"`
	Categories      []string       `json:"categories"`
	Sections        []inputSection `json:"sections"`
	TeachingScore   int            `json:"teachingScore"`
	ContentVersion  int            `json:"contentVersion"`
	ContentWarnings []string       `json:"contentWarnings"`
}

var blankLine = regexp.MustCompile(`\r?\n\s*\r?\n`)

func main() {
	defaultPath := `E:\MathWikiData\exports\core-articles.jsonl`
	path := flag.String("input", defaultPath, "最终中文核心条目 JSONL")
	flag.Parse()
	cfg := config.Load()
	db, err := gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		log.Fatal(err)
	}
	file, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 128*1024), 32*1024*1024)
	count := 0
	for scanner.Scan() {
		var input inputArticle
		if err := json.Unmarshal(scanner.Bytes(), &input); err != nil {
			log.Fatalf("第 %d 行格式错误: %v", count+1, err)
		}
		if err := upsert(db, input); err != nil {
			log.Fatalf("导入 %s 失败: %v", input.Title, err)
		}
		count++
		if count%250 == 0 {
			log.Printf("已导入 %d 篇简体中文核心条目", count)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("完成：%d 篇简体中文核心教学条目", count)
}

func upsert(db *gorm.DB, input inputArticle) error {
	categories, _ := json.Marshal(input.Categories)
	tags := input.Categories
	if len(tags) > 8 {
		tags = tags[:8]
	}
	tagsJSON, _ := json.Marshal(tags)
	category := "数学与算法"
	if len(input.Roots) > 0 {
		category = normalizeCategory(input.Roots[0])
	}
	article := model.Article{PageID: input.PageID, RevisionID: input.RevisionID, Title: input.Title, SourceTitle: input.SourceTitle, Slug: fmt.Sprintf("zh-%d", input.PageID), Lead: lead(input.PlainText), PlainText: input.PlainText, RawWikitext: input.RawWikitext, Category: category, Categories: string(categories), Tags: string(tagsJSON), Kind: kind(input), Difficulty: difficulty(input), ReadMinutes: max(1, utf8.RuneCountInString(input.PlainText)/400), Language: "zh-Hans", SourceURL: input.SourceURL, License: input.License, ReviewStatus: "machine_selected", Published: true}
	article.ContentVersion = input.ContentVersion
	warnings, _ := json.Marshal(input.ContentWarnings)
	article.ContentWarnings = string(warnings)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "page_id"}}, DoUpdates: clause.AssignmentColumns([]string{"revision_id", "title", "source_title", "lead", "plain_text", "raw_wikitext", "category", "categories", "tags", "kind", "difficulty", "read_minutes", "source_url", "license", "review_status", "published", "updated_at", "content_version", "content_warnings"})}).Create(&article).Error; err != nil {
			return err
		}
		if err := tx.Where("page_id = ?", input.PageID).First(&article).Error; err != nil {
			return err
		}
		if err := tx.Where("article_id = ?", article.ID).Delete(&model.Section{}).Error; err != nil {
			return err
		}
		sections := make([]model.Section, 0, len(input.Sections))
		for i, section := range input.Sections {
			if strings.TrimSpace(section.Content) != "" || strings.TrimSpace(section.ContentHTML) != "" {
				position := i + 1
				if section.Position != nil {
					position = *section.Position
				}
				sections = append(sections, model.Section{ArticleID: article.ID, Position: position, Title: section.Title, Content: section.Content, ContentHTML: section.ContentHTML, Level: section.Level, Formulae: "[]"})
			}
		}
		if len(sections) == 0 && strings.TrimSpace(input.PlainText) != "" {
			sections = append(sections, model.Section{ArticleID: article.ID, Position: 1, Title: "概述", Content: input.PlainText, Formulae: "[]"})
		}
		if len(sections) > 0 {
			return tx.CreateInBatches(&sections, 100).Error
		}
		return nil
	})
}

func lead(text string) string {
	parts := blankLine.Split(strings.TrimSpace(text), 2)
	value := parts[0]
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240]) + "……"
	}
	return value
}
func difficulty(article inputArticle) string {
	size := utf8.RuneCountInString(article.PlainText)
	if article.TeachingScore > 180 || size > 10000 {
		return "高级"
	}
	if size > 3500 || len(article.Sections) > 7 {
		return "进阶"
	}
	return "入门"
}
func kind(article inputArticle) string {
	joined := article.Title + " " + strings.Join(article.Categories, " ")
	if strings.Contains(joined, "例题") || strings.Contains(joined, "习题") {
		return "例题"
	}
	if strings.Contains(joined, "算法") || strings.Contains(joined, "排序") || strings.Contains(joined, "搜索") {
		return "算法"
	}
	return "概念"
}
func normalizeCategory(value string) string {
	replacements := map[string]string{"幾何學": "几何学", "線性代數": "线性代数", "離散數學": "离散数学", "機器學習": "机器学习", "數值分析": "数值计算", "機率論": "概率与统计"}
	if converted, ok := replacements[value]; ok {
		return converted
	}
	return value
}
