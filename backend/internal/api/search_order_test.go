package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"mathwiki/internal/ai"
	"mathwiki/internal/model"
)

func TestSearchTitlePriorityBeforePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "search.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	seq := int64(0)
	add := func(title, lead, tags, body, source, status string, updated time.Time) model.Article {
		seq++
		a := model.Article{PageID: seq, Title: title, SourceTitle: source, Lead: lead, Tags: tags, PlainText: body, Slug: fmt.Sprint(seq), Language: "zh-Hans", Category: "搜索测试", Kind: "概念", Published: true, ReviewStatus: status, UpdatedAt: updated}
		if err := db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		return a
	}
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := old.Add(24 * time.Hour)
	for i := 0; i < 60; i++ {
		add(fmt.Sprintf("正文命中-%02d", i), "", "[]", strings.Repeat("梯度下降 ", 200), "", "machine_selected", newer)
	}
	add("摘要命中", "梯度下降", "[]", "", "", "machine_selected", newer)
	add("标签命中", "", `["梯度下降"]`, "", "", "machine_selected", old)
	add("随机梯度下降", "", "[]", "", "", "machine_selected", old)
	add("梯度下降法", "", "[]", "", "", "machine_selected", old)
	exact := add("梯度下降", "", "[]", "", "", "machine_selected", old)
	add("原名命中", "", "[]", "", "梯度下降", "machine_selected", newer)
	add("梯度下降", "", "[]", "", "", "community_pending", newer)
	add("梯度下降", "", "[]", "", "", "rejected", newer)
	deleted := add("梯度下降", "", "[]", "", "", "approved", newer)
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	add("A_B%算法", "", "[]", "", "", "approved", old)
	add("DIJKSTRA", "", "[]", "", "", "approved", old)
	router := New(db, nil, ai.New("", "", "demo"), "mock").Router()
	get := func(path string) ([]articleResponse, int) {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1"+path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %s", path, w.Body.String())
		}
		if strings.Contains(path, "format=page") {
			var result struct {
				Items []articleResponse
				Total int
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			return result.Items, result.Total
		}
		var result []articleResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result, len(result)
	}
	q := url.QueryEscape("梯度下降")
	for _, path := range []string{"/articles?format=page&q=" + q + "&limit=2", "/articles?format=page&sort=relevance&q=" + q + "&limit=2", "/search?q=" + q + "&limit=2"} {
		items, _ := get(path)
		if len(items) != 2 || items[0].ID != exact.ID || items[1].Title != "梯度下降法" {
			t.Fatalf("title priority lost: %s / %+v", path, items)
		}
	}
	items, total := get("/articles?format=page&q=" + q + "&limit=2&page=2")
	if total != 3 || len(items) != 1 || items[0].Title != "随机梯度下降" {
		t.Fatalf("pagination/count incorrect: %d %+v", total, items)
	}
	items, _ = get("/articles?format=page&q=" + q + "&limit=100")
	want := []string{"梯度下降", "梯度下降法", "随机梯度下降"}
	for i, title := range want {
		if items[i].Title != title {
			t.Fatalf("rank %d got %s want %s", i, items[i].Title, title)
		}
	}
	for _, query := range []string{"%", "_", "a_b%", " dijkstra "} {
		items, total := get("/articles?format=page&q=" + url.QueryEscape(query))
		if total != 1 || len(items) != 1 {
			t.Fatalf("literal/case search %q: %d", query, total)
		}
	}
	items, total = get("/articles?format=page&q=" + url.QueryEscape("' OR 1=1 --"))
	if total != 0 || len(items) != 0 {
		t.Fatal("search escaped its filters")
	}
	_, total = get("/articles?format=page&q=" + q + "&kind=算法")
	if total != 0 {
		t.Fatal("kind filter lost")
	}
	_, total = get("/articles?format=page&q=" + q + "&category=" + url.QueryEscape("不匹配分类"))
	if total != 0 {
		t.Fatal("category filter lost")
	}
	_, total = get("/articles?format=page&q=" + q + "&collection=favorites")
	if total != 0 {
		t.Fatal("collection filter lost")
	}
	items, _ = get("/articles?format=page&limit=1")
	if items[0].Title != "正文命中-00" {
		t.Fatal("empty search changed default update order")
	}
	items, _ = get("/articles?format=page&q=" + q + "&sort=updated&limit=1")
	if items[0].Title != "随机梯度下降" {
		t.Fatal("explicit date sort ignored")
	}
}
