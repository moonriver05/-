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
		{"本科生", "直觉 + 严谨", []string{"本轮用户明确提出", "下拉框默认选择", "我是小学生，用简单短句回答", "若草稿仍然过难", "summary、steps"}},
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
				if !payload.Stream && !strings.Contains(prompt.String(), "我是小学生，请用简单的短句解释它如何工作？") {
					t.Error("verification lost the user's current reading-level override")
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
			_, err := New(server.URL, "test-key", "deepseek-flash").Stream(context.Background(), model.Article{}, "我是小学生，请用简单的短句解释它如何工作？", tc.audience, tc.style, []Message{{Role: "user", Content: "旧问题"}, {Role: "assistant", Content: "此前的专业讲解"}}, func(StreamEvent) error { return nil })
			if err != nil || calls != 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestSynchronousVerificationRetainsCurrentPreferenceOverride(t *testing.T) {
	question := "我是小学生，请用简单短句讲解，不要公式"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var payload struct {
			Messages []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		var prompt strings.Builder
		for _, message := range payload.Messages {
			prompt.WriteString(message.Content)
		}
		if !strings.Contains(prompt.String(), question) || !strings.Contains(prompt.String(), "本轮用户明确提出") {
			t.Errorf("call %d lost the current preference override", calls)
		}
		answer := `{"summary":"把两堆苹果放在一起。","steps":[],"citations":[{"section":"S1","quote":"合并数量"}],"checks":[],"confidence":0.8}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	_, err := New(server.URL, "test-key", "deepseek-flash").Explain(context.Background(), model.Article{}, question, "本科生", "公式推导")
	if err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
