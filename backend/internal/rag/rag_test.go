package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"mathwiki/internal/model"
)

func fixture(t *testing.T) (*gorm.DB, model.Article, model.Article) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rag.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.Article{}, &model.Section{}); err != nil {
		t.Fatal(err)
	}
	create := func(page int64, status string, hidden bool) model.Article {
		a := model.Article{PageID: page, Title: fmt.Sprintf("条目%d", page), Slug: fmt.Sprintf("article-%d", page), Published: true, Language: "zh-Hans", ReviewStatus: status, Sections: []model.Section{{Title: "定义", Content: fmt.Sprintf("条目%d的公开定义。", page), Hidden: hidden}}}
		if err := db.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		return a
	}
	a, b := create(1, "approved", false), create(2, "machine_selected", false)
	create(3, "community_pending", false)
	create(4, "rejected", false)
	create(5, "approved", true)
	deleted := create(6, "approved", false)
	db.Delete(&deleted)
	return db, a, b
}

func TestIndexRetrieveIdempotencyAndLiveVisibility(t *testing.T) {
	db, a, b := fixture(t)
	records := map[string]bool{}
	embeddingCalls := 0
	var lastQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				t.Error(err)
			}
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/embeddings/multimodal"):
			embeddingCalls++
			if r.Header.Get("Authorization") != "Bearer test-key" || p["model"] != Model || p["dimensions"] != float64(1024) {
				t.Error("bad Ark protocol")
			}
			inputs := p["input"].([]any)
			if len(inputs) != 1 {
				t.Error("each text must be a separate embedding request")
			}
			if _, ok := p["instructions"]; ok {
				lastQuery = inputs[0].(map[string]any)["text"].(string)
			}
			vector := make([]float32, 1024)
			vector[0] = 2
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"embedding": vector}})
		case strings.HasSuffix(r.URL.Path, "/count"):
			json.NewEncoder(w).Encode(len(records))
		case strings.HasSuffix(r.URL.Path, "/get"):
			var ids []string
			for _, id := range p["ids"].([]any) {
				if records[id.(string)] {
					ids = append(ids, id.(string))
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"ids": ids})
		case strings.HasSuffix(r.URL.Path, "/upsert"):
			vectors := p["embeddings"].([]any)
			v := vectors[0].([]any)
			if len(v) != 1024 || v[0] != float64(1) {
				t.Error("vectors must be normalized")
			}
			if _, ok := p["documents"]; ok {
				t.Error("full documents must stay in MySQL")
			}
			for _, id := range p["ids"].([]any) {
				records[id.(string)] = true
			}
			w.Write([]byte("{}"))
		case strings.HasSuffix(r.URL.Path, "/query"):
			if _, ok := p["query_embeddings"]; !ok {
				t.Error("missing explicit query vector")
			}
			ids := []string{chunkID(a.Sections[0].ID, 0, a.Title+"\n"+a.Sections[0].Title+"\n"+a.Sections[0].Content), chunkID(b.Sections[0].ID, 0, b.Title+"\n"+b.Sections[0].Title+"\n"+b.Sections[0].Content)}
			if _, local := p["where"]; local {
				ids = ids[:1]
			}
			ds := make([]float64, len(ids))
			for i := range ds {
				ds[i] = .2
			}
			json.NewEncoder(w).Encode(map[string]any{"ids": [][]string{ids}, "distances": [][]float64{ds}})
		default:
			if r.Method == "POST" {
				hnsw := p["configuration"].(map[string]any)["hnsw"].(map[string]any)
				if hnsw["space"] != "cosine" {
					t.Error("wrong distance")
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"id": "collection-id", "metadata": map[string]any{"rag_model": Model, "rag_dimensions": 1024, "rag_format": "chunks800_overlap100_v1"}})
		}
	}))
	defer server.Close()
	t.Setenv("RAG_ENABLED", "true")
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_BASE_URL", server.URL)
	t.Setenv("CHROMA_URL", server.URL)
	c := FromEnv()
	if err := c.IndexArticle(context.Background(), db, 1000, b.ID, nil); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || embeddingCalls != 1 {
		t.Fatal("targeted pass must only index the chosen article")
	}
	if err := c.IndexArticle(context.Background(), db, 1000, b.ID, nil); err != nil {
		t.Fatal(err)
	}
	if embeddingCalls != 1 {
		t.Fatal("targeted rerun incurred embedding charges")
	}
	for _, privateID := range []uint{3, 4, 5, 6, 999} {
		if err := c.IndexArticle(context.Background(), db, 1000, privateID, nil); err == nil {
			t.Fatalf("article %d must not be indexed", privateID)
		}
	}
	if embeddingCalls != 1 {
		t.Fatal("private/hidden/deleted article incurred embedding charges")
	}
	if err := c.Index(context.Background(), db, 1000, nil); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || embeddingCalls != 2 {
		t.Fatalf("indexed private/hidden/deleted records: count=%d calls=%d", len(records), embeddingCalls)
	}
	if err := c.Index(context.Background(), db, 1000, nil); err != nil {
		t.Fatal(err)
	}
	if embeddingCalls != 2 {
		t.Fatal("rerun incurred embedding charges")
	}
	evidence, status := c.Retrieve(context.Background(), db, a, "定义是什么")
	if status.Mode != "chroma" || len(evidence) != 2 || evidence[1].ArticleID != b.ID || lastQuery == "" {
		t.Fatalf("bad retrieval: %+v %+v", status, evidence)
	}
	// Even if a stale vector still ranks highly, withdrawal must take effect immediately.
	db.Model(&b).Update("review_status", "rejected")
	evidence, status = c.Retrieve(context.Background(), db, a, "定义是什么")
	if status.Mode != "chroma" || len(evidence) != 1 || evidence[0].ArticleID != a.ID {
		t.Fatal("rejected article leaked")
	}
	db.Model(&model.Section{}).Where("id = ?", a.Sections[0].ID).Update("content", "已经更新，旧片段不再有效")
	evidence, status = c.Retrieve(context.Background(), db, a, "定义是什么")
	if status.Mode != "current_article" || status.Reason != "no_match" {
		t.Fatal("changed chunk must not be retrieved")
	}
	if err := c.Index(context.Background(), db, 10001, nil); err == nil {
		t.Fatal("trial cap not enforced")
	}
}

func TestUnavailableCancellationAndDimensionMismatch(t *testing.T) {
	db, a, _ := fixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "embeddings") {
			w.Write([]byte(`{"data":{"embedding":[1,2]}}`))
			return
		}
		w.WriteHeader(500)
		w.Write([]byte("secret vendor payload"))
	}))
	defer server.Close()
	t.Setenv("RAG_ENABLED", "true")
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_BASE_URL", server.URL)
	t.Setenv("CHROMA_URL", server.URL)
	c := FromEnv()
	if _, err := c.Embed(context.Background(), "问题", true); err == nil || !strings.Contains(err.Error(), "dimension") {
		t.Fatal("wrong dimensions accepted")
	}
	evidence, status := c.Retrieve(context.Background(), db, a, "定义")
	if status.Reason != "unavailable" || len(evidence) != 1 {
		t.Fatal("fallback missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Embed(ctx, "问题", true); err == nil || strings.Contains(err.Error(), "test-key") {
		t.Fatal("cancellation/error sanitization failed")
	}
	c.slots <- struct{}{}
	c.slots <- struct{}{}
	_, status = c.Retrieve(context.Background(), db, a, "问题")
	if status.Reason != "busy" {
		t.Fatal("concurrency must be bounded")
	}
}

func TestChunksAndFallbackAreBoundedAndQuestionAware(t *testing.T) {
	parts := chunks(strings.Repeat("数", 20000))
	if len(parts) > 18 {
		t.Fatal("unbounded section")
	}
	for _, part := range parts {
		if len([]rune(part)) > 800 {
			t.Fatal("unbounded chunk")
		}
	}
	article := model.Article{ID: 1, Title: "数学"}
	for i := 0; i < 10; i++ {
		article.Sections = append(article.Sections, model.Section{ID: uint(i + 1), Title: "概念", Content: "普通定义"})
	}
	article.Sections = append(article.Sections, model.Section{ID: 99, Title: "负数", Content: "负数不能满足该算法的前提"}, model.Section{ID: 100, Title: "隐藏", Content: "负数", Hidden: true})
	evidence := Current(article, "为什么不能有负数")
	if len(evidence) != 5 || evidence[0].SectionID != 99 {
		t.Fatal("question-aware selection failed")
	}
}
