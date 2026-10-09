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

func TestStreamDeliversDraftBeforeVerificationAndPassesHistory(t *testing.T) {
	var events []StreamEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Stream   bool      `json:"stream"`
			Messages []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Error(err)
			return
		}
		if p.Stream {
			if len(p.Messages) != 4 || p.Messages[1].Content != "上轮问题" || p.Messages[2].Content != "上轮回答" || p.Messages[3].Content != "第二步呢？" {
				t.Errorf("history missing: %+v", p.Messages)
			}
			if strings.Contains(p.Messages[0].Content, "隐藏证据") {
				t.Error("hidden evidence leaked")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, text := range []string{"第二步", "是合并。"} {
				raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": text}, "finish_reason": nil}}})
				fmt.Fprintf(w, "data: %s\n\n", raw)
				w.(http.Flusher).Flush()
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		} else {
			if len(events) != 4 || events[1].Type != "delta" || events[2].Text != "是合并。" || events[3].Text != "verifying" {
				t.Errorf("draft must arrive before verification: %+v", events)
			}
			answer := `{"summary":"合并","steps":[],"citations":[{"section":"S1","quote":"定义"}],"checks":[],"confidence":0.8}`
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": answer}, "finish_reason": "stop"}}})
		}
	}))
	defer server.Close()
	article := model.Article{Title: "加法", Sections: []model.Section{{Title: "定义", Content: "定义"}, {Hidden: true, Content: "隐藏证据"}}}
	result, err := New(server.URL, "test-key", "deepseek-flash").Stream(context.Background(), article, "第二步呢？", "本科生", "直觉", []Message{{Role: "user", Content: "上轮问题"}, {Role: "assistant", Content: "上轮回答"}}, func(event StreamEvent) error { events = append(events, event); return nil })
	if err != nil || result.Mode != "model" || len(events) != 5 || events[4].Type != "done" {
		t.Fatalf("result=%+v err=%v events=%+v", result, err, events)
	}
}

func TestStreamRejectsTruncatedOrDisconnectedUpstream(t *testing.T) {
	for _, body := range []string{`data: {"choices":[{"delta":{"content":"半句"}}]}` + "\n\n", `data: {"choices":[{"delta":{"content":"半句"},"finish_reason":"length"}]}` + "\n\ndata: [DONE]\n\n"} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			_, err := New(server.URL, "test-key", "deepseek-flash").streamCompletion(context.Background(), nil, func(string) error { return nil })
			if err == nil {
				t.Fatal("must reject incomplete response")
			}
		})
	}
}

func TestStreamCancellationDoesNotVerify(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"开始\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	_, err := New(server.URL, "test-key", "deepseek-flash").streamCompletion(ctx, nil, func(string) error { cancel(); return ctx.Err() })
	if err == nil {
		t.Fatal("expected cancellation")
	}
}
