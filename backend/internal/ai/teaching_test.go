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

func TestTeachingRequirementsReachGenerationAndVerification(t *testing.T) {
	for _, tc := range []struct {
		audience, style string
		requirements    []string
	}{
		{"小学生", "生活类比", []string{"只掌握基础算术", "具体生活情景", "类比的局限"}},
		{"专业学习者", "直觉 + 严谨", []string{"形式化定义", "成立条件", "区分直觉与严格结论"}},
	} {
		t.Run(tc.audience+tc.style, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var payload struct {
					Stream   bool      `json:"stream"`
					Messages []Message `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
					return
				}
				var prompt strings.Builder
				for _, m := range payload.Messages {
					prompt.WriteString(m.Content)
				}
				for _, requirement := range tc.requirements {
					if !strings.Contains(prompt.String(), requirement) {
						t.Errorf("call %d lost teaching requirement %s", calls, requirement)
					}
				}
				if payload.Stream {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"示例回答\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				} else {
					answer := `{"kind":"explanation","summary":"示例回答","steps":[],"citations":[{"section":"S1","quote":"定义"}],"checks":[]}`
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
				}
			}))
			defer server.Close()
			_, err := New(server.URL, "test-key", "deepseek-flash").Stream(context.Background(), model.Article{}, "它如何工作？", tc.audience, tc.style, []Message{{Role: "user", Content: "旧问题"}, {Role: "assistant", Content: "此前的专业讲解"}}, func(StreamEvent) error { return nil })
			if err != nil || calls != 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
