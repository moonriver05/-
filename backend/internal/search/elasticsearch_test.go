package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTitleFirstQuery(t *testing.T) {
	query := titleFirstQuery(`A*B?C\D`, 30)
	root := query["query"].(map[string]any)["bool"].(map[string]any)
	dismax := root["must"].(map[string]any)["dis_max"].(map[string]any)
	if dismax["tie_breaker"] != 0 {
		t.Fatal("tiers must not add scores")
	}
	tiers := dismax["queries"].([]any)
	if len(tiers) != 3 {
		t.Fatal("only title tiers are allowed")
	}
	previous := 1000
	for _, item := range tiers {
		tier := item.(map[string]any)["constant_score"].(map[string]any)
		score := tier["boost"].(int)
		if score >= previous {
			t.Fatal("title/summary/body ordering violated")
		}
		previous = score
	}
	filter := tiers[2].(map[string]any)["constant_score"].(map[string]any)["filter"].(map[string]any)
	value := filter["wildcard"].(map[string]any)["title"].(map[string]any)["value"]
	if value != `*A\*B\?C\\D*` {
		t.Fatalf("wildcard not escaped: %v", value)
	}
	data, _ := json.Marshal(query)
	if strings.Contains(string(data), "fuzziness") || !strings.Contains(string(data), "zh-Hans") {
		t.Fatal("unexpected recall behavior")
	}
}

func TestSearchSendsRankedQueryAndKeepsResponseOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/knowledge/_search" || r.Method != "POST" {
			t.Error("unexpected request")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["size"] != float64(2) || body["sort"] == nil {
			t.Error("missing limit or sorting")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":{"hits":[{"_score":500,"_source":{"pageId":2,"title":"梯度下降"}},{"_score":100,"_source":{"pageId":1,"title":"机器学习"}}]}}`))
	}))
	defer server.Close()
	hits, err := New(server.URL, "knowledge").Search(context.Background(), "梯度下降", 2)
	if err != nil || len(hits) != 2 || hits[0].PageID != 2 || hits[0].Score != 500 {
		t.Fatalf("unexpected hits: %+v / %v", hits, err)
	}
}
