// Rebuild display content without deleting articles, annotations or section IDs.
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
	"mathwiki/internal/config"
	"mathwiki/internal/model"
)

type renderedSection struct {
	Position    int    `json:"position"`
	Title       string `json:"title"`
	Level       int    `json:"level"`
	Content     string `json:"content"`
	ContentHTML string `json:"contentHtml"`
}
type renderedArticle struct {
	PageID         int64             `json:"pageId"`
	Excerpt        string            `json:"excerpt"`
	PlainText      string            `json:"plainText"`
	ContentVersion int               `json:"contentVersion"`
	Warnings       []string          `json:"warnings"`
	Sections       []renderedSection `json:"sections"`
}

var templateMarker = regexp.MustCompile(`〔模板待展开：[^〕]*〕`)

func preview(input renderedArticle) string {
	for _, section := range input.Sections {
		if section.Position != 0 {
			continue
		}
		text := strings.TrimSpace(templateMarker.ReplaceAllString(section.Content, ""))
		if text == "" {
			continue
		}
		text = strings.Join(strings.Fields(strings.SplitN(text, "\n\n", 2)[0]), " ")
		runes := []rune(text)
		if len(runes) > 160 {
			return string(runes[:160]) + "…"
		}
		return text
	}
	return templateMarker.ReplaceAllString(input.Excerpt, "")
}

func repair(db *gorm.DB, input renderedArticle) error {
	if input.PageID <= 0 || input.ContentVersion != 2 || len(input.Sections) == 0 {
		return fmt.Errorf("invalid rendered article %d", input.PageID)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var article model.Article
		if err := tx.Where("page_id = ?", input.PageID).First(&article).Error; err != nil {
			return err
		}
		var existing []model.Section
		if err := tx.Where("article_id = ?", article.ID).Find(&existing).Error; err != nil {
			return err
		}
		byPosition := make(map[int]uint)
		for _, section := range existing {
			byPosition[section.Position] = section.ID
		}
		// Retain obsolete rows (and their annotation anchors); just hide them in the reader.
		if err := tx.Model(&model.Section{}).Where("article_id = ?", article.ID).Update("hidden", true).Error; err != nil {
			return err
		}
		for _, section := range input.Sections {
			if id, ok := byPosition[section.Position]; ok {
				if err := tx.Model(&model.Section{}).Where("id = ?", id).Updates(map[string]any{"title": section.Title, "content": section.Content, "content_html": section.ContentHTML, "level": section.Level, "hidden": false}).Error; err != nil {
					return err
				}
			} else {
				row := model.Section{ArticleID: article.ID, Position: section.Position, Title: section.Title, Content: section.Content, ContentHTML: section.ContentHTML, Level: section.Level, Formulae: "[]"}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
		}
		warnings, _ := json.Marshal(input.Warnings)
		return tx.Model(&article).Updates(map[string]any{"lead": preview(input), "plain_text": input.PlainText, "content_version": input.ContentVersion, "content_warnings": string(warnings), "read_minutes": max(1, utf8.RuneCountInString(input.PlainText)/400)}).Error
	})
}

func main() {
	path := flag.String("input", "", "Rendered JSONL produced by scripts/render_wiki.py")
	flag.Parse()
	if *path == "" {
		log.Fatal("--input is required")
	}
	file, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()
	db, err := gorm.Open(mysql.Open(config.Load().MySQLDSN), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	if err = model.Migrate(db); err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 128*1024), 64*1024*1024)
	count := 0
	for scanner.Scan() {
		var input renderedArticle
		if err = json.Unmarshal(scanner.Bytes(), &input); err != nil {
			log.Fatal(err)
		}
		if err = repair(db, input); err != nil {
			log.Fatalf("page %d: %v", input.PageID, err)
		}
		count++
		if count%250 == 0 {
			log.Printf("Repaired %d articles", count)
		}
	}
	if err = scanner.Err(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Completed: %d articles; existing section IDs preserved", count)
}
