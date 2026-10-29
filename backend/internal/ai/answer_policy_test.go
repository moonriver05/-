package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mathwiki/internal/model"
)

func TestBalancedExplanationCanFinishWithoutInventingCitations(t *testing.T) {
	for _, policy := range []string{BalancedAnswer, StrictAnswer} {
		t.Run(policy, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Stream   bool      `json:"stream"`
					Messages []Message `json:"messages"`
				}
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Error("invalid request")
					return
				}
				prompts := ""
				for _, message := range request.Messages {
					prompts += message.Content
				}
				if !strings.Contains(prompts, "数学排版") || (policy == BalancedAnswer && !strings.Contains(prompts, "推导补充")) {
					t.Error("missing answer policy")
				}
				if request.Stream {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"推导补充\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": `{"kind":"explanation","summary":"模型推导补充：先说明可微条件，再应用链式法则。","steps":[],"citations":[],"confidence":0.7}`}, "finish_reason": "stop"}}})
				}
			}))
			defer server.Close()
			client := New(server.URL, "test-only", "deepseek-flash")
			result, err := client.Stream(context.Background(), model.Article{}, "展开推导", "本科生", "公式推导", nil, func(StreamEvent) error { return nil }, policy)
			if policy == BalancedAnswer && (err != nil || result.AnswerMode != BalancedAnswer || result.Citations == nil) {
				t.Fatalf("balanced response blocked: %+v %v", result, err)
			}
			if policy == StrictAnswer && err == nil {
				t.Fatal("strict mode accepted uncited math")
			}
			_, err = client.Explain(context.Background(), model.Article{}, "展开推导", "本科生", "公式推导", policy)
			if (err != nil) != (policy == StrictAnswer) {
				t.Fatalf("sync policy mismatch: %v", err)
			}
		})
	}
}
