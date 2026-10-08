package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mathwiki/internal/model"
)

func TestDeepSeekExplanationTwoPasses(t *testing.T) {
	calls := 0
	answer := `{"summary":"解释","steps":[],"citations":[{"section":"S1","quote":"定义"}],"confidence":0.8,"checks":["核验引用"]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/chat/completions" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("incorrect request routing or authentication")
		}
		var p map[string]any
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Fatal(err)
		}
		thinking, _ := p["thinking"].(map[string]any)
		if p["model"] != "deepseek-flash" || thinking["type"] != "disabled" || p["max_tokens"] != float64(4096) {
			t.Errorf("unexpected model options: %v", p)
		}
		if calls == 2 && !strings.Contains(p["messages"].([]any)[1].(map[string]any)["content"].(string), answer) {
			t.Error("verification must receive the draft")
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	got, err := New(server.URL, "test-key", "deepseek-flash").Explain(context.Background(), model.Article{}, "问题", "本科生", "直觉")
	if err != nil || got.Mode != "model" || calls != 2 {
		t.Fatalf("result=%+v err=%v calls=%d", got, err, calls)
	}
}

func TestCompletionRejectsUnusableResponses(t *testing.T) {
	for _, body := range []string{`{"choices":[]}`, `{"choices":[{"message":{"content":""}}]}`, `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
			defer server.Close()
			if _, err := New(server.URL, "test-key", "deepseek-flash").complete(context.Background(), "system", "prompt"); err == nil {
				t.Fatal("expected incomplete response error")
			}
		})
	}
}
