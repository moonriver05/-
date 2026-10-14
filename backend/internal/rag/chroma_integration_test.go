package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in real Chroma API test. Ark is simulated so this never charges an account.
func TestRealChromaProtocol(t *testing.T) {
	endpoint := os.Getenv("CHROMA_TEST_URL")
	if endpoint == "" {
		t.Skip("set CHROMA_TEST_URL for disposable Chroma integration test")
	}
	db, a, _ := fixture(t)
	ark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vector := make([]float32, Dimensions)
		vector[0] = 1
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"embedding": vector}})
	}))
	defer ark.Close()
	t.Setenv("RAG_ENABLED", "true")
	t.Setenv("ARK_API_KEY", "test-key")
	t.Setenv("ARK_BASE_URL", ark.URL)
	t.Setenv("CHROMA_URL", endpoint)
	t.Setenv("CHROMA_COLLECTION", "mathwiki_test_"+time.Now().Format("150405000"))
	c := FromEnv()
	if err := c.Index(context.Background(), db, 1000, nil); err != nil {
		t.Fatal(err)
	}
	if count, err := c.Count(context.Background()); err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err := c.Index(context.Background(), db, 1000, nil); err != nil {
		t.Fatal(err)
	}
	evidence, status := c.Retrieve(context.Background(), db, a, "定义是什么")
	if status.Mode != "chroma" || len(evidence) != 2 {
		t.Fatalf("real query failed: %+v %+v", status, evidence)
	}
}

func TestRealChromaThousandVectors(t *testing.T) {
	endpoint := os.Getenv("CHROMA_TEST_URL")
	if endpoint == "" {
		t.Skip("set CHROMA_TEST_URL for disposable Chroma capacity smoke test")
	}
	t.Setenv("CHROMA_URL", endpoint)
	t.Setenv("CHROMA_COLLECTION", "mathwiki_scale_"+time.Now().Format("150405000"))
	c := FromEnv()
	col, err := c.collection(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	var query []float32
	for start := 0; start < 1000; start += 25 {
		var ids []string
		var embeddings [][]float32
		var metadata []any
		for i := start; i < start+25; i++ {
			vector := make([]float32, Dimensions)
			var norm float64
			for j := range vector {
				v := math.Sin(float64((i + 1) * (j + 1)))
				vector[j] = float32(v)
				norm += v * v
			}
			for j := range vector {
				vector[j] /= float32(math.Sqrt(norm))
			}
			if i == 0 {
				query = vector
			}
			ids = append(ids, fmt.Sprintf("scale-%d", i))
			embeddings = append(embeddings, vector)
			metadata = append(metadata, map[string]any{"article_id": 1, "section_id": i + 1, "chunk": 0})
		}
		if err := c.request(context.Background(), "POST", c.records(col.ID, "upsert"), map[string]any{"ids": ids, "embeddings": embeddings, "metadatas": metadata}, false, nil); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := c.Count(context.Background()); err != nil || count != 1000 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	for i := 0; i < 20; i++ {
		ids, err := c.query(context.Background(), col, query, 0)
		if err != nil || len(ids) == 0 || ids[0] != "scale-0" {
			t.Fatalf("query %d: %v %v", i, ids, err)
		}
	}
}
