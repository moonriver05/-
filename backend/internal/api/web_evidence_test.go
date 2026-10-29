package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"mathwiki/internal/ai"
	"mathwiki/internal/websearch"
)

func TestWebSearchOptInFallbackAndSourceMetadataAcrossEndpoints(t *testing.T) {
	f := newSecurityFixture(t)
	f.s.aiGate.perMinute = 60
	searchCalls, chatCalls := 0, 0
	searchFails := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/tools/search" {
			searchCalls++
			if searchFails {
				w.WriteHeader(http.StatusPaymentRequired)
				return
			}
			fmt.Fprint(w, `{"results":[{"title":"大学课程","url":"https://example.edu/calculus","content":"链式法则用于计算复合函数的导数。"}]}`)
			return
		}
		chatCalls++
		var request struct {
			Stream   bool         `json:"stream"`
			Messages []ai.Message `json:"messages"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Error("invalid model request")
			return
		}
		if request.Stream {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"推导补充\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		prompt := ""
		for _, message := range request.Messages {
			prompt += message.Content
		}
		answer := ai.Explanation{Summary: "模型推导补充：应用链式法则。", Steps: []ai.Step{}, Citations: []ai.Citation{}}
		match := regexp.MustCompile(`\[S(\d+)｜大学课程｜网页正文节选\]`).FindStringSubmatch(prompt)
		if len(match) > 1 {
			answer.Citations = []ai.Citation{{Section: "S" + match[1], Quote: "链式法则用于计算复合函数的导数。", SourceURL: "https://forged.example"}}
		}
		encoded, _ := json.Marshal(answer)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(encoded)}, "finish_reason": "stop"}}})
	}))
	defer upstream.Close()
	f.s.explainer = ai.New(upstream.URL, "test-model-key", "deepseek-flash")
	f.s.webSearch = &websearch.Client{Enabled: true, Key: "test-search-key", BaseURL: upstream.URL, HTTP: upstream.Client()}
	body := map[string]any{"articleId": f.article.ID, "question": "链式法则如何推导", "answerMode": "balanced"}
	local := f.call("POST", "/explanations", body, f.cookie, 200)
	var answer ai.Explanation
	if json.Unmarshal(local.Body.Bytes(), &answer) != nil || answer.Grounding != "model" || searchCalls != 0 {
		t.Fatalf("search called without consent: %+v", answer)
	}
	body["webSearch"] = true
	web := f.call("POST", "/explanations", body, f.cookie, 200)
	if json.Unmarshal(web.Body.Bytes(), &answer) != nil || searchCalls != 1 || answer.WebSearch == nil || answer.WebSearch.Count != 1 || len(answer.Citations) != 1 || answer.Citations[0].SourceURL != "https://example.edu/calculus" {
		t.Fatalf("missing web evidence: %s", web.Body.String())
	}
	stream := f.call("POST", "/explanations/stream", body, f.cookie, 200)
	if searchCalls != 2 || !strings.Contains(stream.Body.String(), `"text":"searching"`) || !strings.Contains(stream.Body.String(), `"sourceType":"web"`) {
		t.Fatalf("stream search was not integrated: %s", stream.Body.String())
	}
	searchFails = true
	fallback := f.call("POST", "/explanations", body, f.cookie, 200)
	if json.Unmarshal(fallback.Body.Bytes(), &answer) != nil || answer.WebSearch.Status != "failed" || answer.Grounding != "model" {
		t.Fatalf("search failure did not fall back transparently: %s", fallback.Body.String())
	}
	f.s.webSearch.Enabled = false
	beforeSearch, beforeChat := searchCalls, chatCalls
	f.call("POST", "/explanations/stream", body, f.cookie, 400)
	if searchCalls != beforeSearch || chatCalls != beforeChat {
		t.Fatal("disabled search invoked upstream services")
	}
}
