package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchBoundsProtocolAndFiltersSources(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/tools/search" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Error("wrong search protocol")
		}
		var payload struct {
			Query    string `json:"query"`
			Question string `json:"ori_question"`
			Count    int    `json:"count"`
			Content  bool   `json:"need_content"`
		}
		if json.NewDecoder(r.Body).Decode(&payload) != nil || len([]rune(payload.Query)) != 128 || len([]rune(payload.Question)) != 128 || payload.Count != 3 || !payload.Content {
			t.Errorf("wrong request limits: %+v", payload)
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []any{
			map[string]string{"title": "非法链接", "url": "javascript:alert(1)", "content": "不要使用"},
			map[string]string{"url": "http://127.0.0.1/private", "content": "不要使用"},
			map[string]string{"url": "https://name:password@example.com/", "content": "不要使用"},
			map[string]string{"title": "课程原文", "url": "https://example.edu/course", "content": strings.Repeat("正文", 1500)},
			map[string]string{"title": "重复项", "url": "https://example.edu/course", "content": "重复"},
			map[string]string{"title": "摘要", "url": "https://example.edu/summary", "summary": "只有搜索摘要，不能冒充完整原文。"},
		}})
	}))
	defer server.Close()
	c := &Client{Enabled: true, Key: "test-only", BaseURL: server.URL + "/v1", HTTP: server.Client()}
	results, err := c.Search(context.Background(), strings.Repeat("查", 200), strings.Repeat("问", 200))
	if err != nil || calls != 1 || len(results) != 2 || len([]rune(results[0].Content)) != 1800 || results[0].SummaryOnly || !results[1].SummaryOnly {
		t.Fatalf("unexpected search: %+v, %v", results, err)
	}
}

func TestSearchFailuresAndCancellationDoNotExposeVendorBody(t *testing.T) {
	for _, body := range []string{`{"error":"secret-vendor-detail"}`, `invalid-json`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			c := &Client{Enabled: true, Key: "test-only", BaseURL: server.URL, HTTP: server.Client()}
			_, err := c.Search(context.Background(), "链式法则", "如何推导")
			if err == nil || strings.Contains(err.Error(), "secret-vendor-detail") {
				t.Fatalf("bad error: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := c.Search(ctx, "查询", "问题"); err == nil {
				t.Fatal("cancellation ignored")
			}
		})
	}
}

func TestSearchCredentialReuseOnlyForOfficialChatProvider(t *testing.T) {
	t.Setenv("DEEPSEEK_SEARCH_ENABLED", "true")
	t.Setenv("DEEPSEEK_SEARCH_BASE_URL", "")
	t.Setenv("DEEPSEEK_SEARCH_API_KEY", "")
	t.Setenv("LLM_API_KEY", "provider-test-key")
	t.Setenv("LLM_BASE_URL", "https://other-provider.example/v1")
	if FromEnv().Configured() {
		t.Fatal("third-party chat credential reused")
	}
	t.Setenv("LLM_BASE_URL", "https://api.deepseek.com/v1")
	if !FromEnv().Configured() {
		t.Fatal("official credential reuse unavailable")
	}
	t.Setenv("DEEPSEEK_SEARCH_BASE_URL", "https://search-proxy.example")
	if FromEnv().Configured() {
		t.Fatal("official chat credential sent to a search proxy")
	}
	t.Setenv("LLM_BASE_URL", "https://other-provider.example/v1")
	t.Setenv("DEEPSEEK_SEARCH_API_KEY", "separate-search-test-key")
	if !FromEnv().Configured() {
		t.Fatal("explicit search credential unavailable")
	}
	t.Setenv("DEEPSEEK_SEARCH_ENABLED", "false")
	if FromEnv().Configured() {
		t.Fatal("search must remain opt-in")
	}
}

func TestSearchDoesNotFollowAuthenticatedRedirects(t *testing.T) {
	forwarded := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true; fmt.Fprint(w, `{"results":[]}`) }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c := FromEnv()
	c.Enabled = true
	c.Key = "test-only"
	c.BaseURL = source.URL
	if _, err := c.Search(context.Background(), "查询", "问题"); err == nil || forwarded {
		t.Fatal("authenticated redirect was followed")
	}
}
