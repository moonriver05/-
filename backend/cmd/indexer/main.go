package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"mathwiki/internal/config"
	"mathwiki/internal/model"
)

type document struct {
	PageID     int64     `json:"pageId"`
	Title      string    `json:"title"`
	Lead       string    `json:"lead"`
	Category   string    `json:"category"`
	Tags       []string  `json:"tags"`
	Difficulty string    `json:"difficulty"`
	Language   string    `json:"language"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Sections   []section `json:"sections"`
}
type section struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

func main() {
	cfg := config.Load()
	db, err := gorm.Open(mysql.Open(cfg.MySQLDSN), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	if err := ensureIndex(client, cfg); err != nil {
		log.Fatal(err)
	}
	var offset int
	for {
		var articles []model.Article
		if err := db.Preload("Sections", func(tx *gorm.DB) *gorm.DB { return tx.Order("position ASC") }).Where("published = ? AND language = ?", true, "zh-Hans").Order("id").Limit(200).Offset(offset).Find(&articles).Error; err != nil {
			log.Fatal(err)
		}
		if len(articles) == 0 {
			break
		}
		if err := bulk(client, cfg, articles); err != nil {
			log.Fatal(err)
		}
		offset += len(articles)
		log.Printf("已建立 %d 篇中文条目索引", offset)
	}
	request(client, http.MethodPost, fmt.Sprintf("%s/%s/_refresh", strings.TrimRight(cfg.ElasticsearchURL, "/"), cfg.ElasticsearchIndex), nil)
	log.Printf("Elasticsearch 中文索引完成，共 %d 篇", offset)
}

func ensureIndex(client *http.Client, cfg config.Config) error {
	base := strings.TrimRight(cfg.ElasticsearchURL, "/")
	url := fmt.Sprintf("%s/%s", base, cfg.ElasticsearchIndex)
	res, err := client.Head(url)
	if err == nil && res.Body != nil {
		res.Body.Close()
	}
	if err == nil && res.StatusCode == http.StatusOK {
		return nil
	}
	path := filepath.Join("..", "infrastructure", "elasticsearch", "index-template.json")
	data, err := os.ReadFile(path)
	if err != nil {
		data, err = os.ReadFile(filepath.Join("infrastructure", "elasticsearch", "index-template.json"))
	}
	if err != nil {
		return err
	}
	_, err = request(client, http.MethodPut, url, data)
	return err
}

func bulk(client *http.Client, cfg config.Config, articles []model.Article) error {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, article := range articles {
		var tags []string
		_ = json.Unmarshal([]byte(article.Tags), &tags)
		doc := document{PageID: article.PageID, Title: article.Title, Lead: article.Lead, Category: article.Category, Tags: tags, Difficulty: article.Difficulty, Language: article.Language, UpdatedAt: article.UpdatedAt}
		for _, item := range article.Sections {
			doc.Sections = append(doc.Sections, section{Title: item.Title, Content: item.Content})
		}
		_ = encoder.Encode(map[string]any{"index": map[string]any{"_index": cfg.ElasticsearchIndex, "_id": article.PageID}})
		_ = encoder.Encode(doc)
	}
	_, err := request(client, http.MethodPost, strings.TrimRight(cfg.ElasticsearchURL, "/")+"/_bulk", buffer.Bytes())
	return err
}

func request(client *http.Client, method, url string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("Elasticsearch %s: %s", res.Status, string(body))
	}
	return body, nil
}
