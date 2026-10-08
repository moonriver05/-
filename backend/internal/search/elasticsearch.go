package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	index   string
	http    *http.Client
}

type Hit struct {
	PageID     int64    `json:"pageId"`
	Title      string   `json:"title"`
	Excerpt    string   `json:"excerpt"`
	Category   string   `json:"category"`
	Tags       []string `json:"tags"`
	Difficulty string   `json:"difficulty"`
	Score      float64  `json:"score"`
}

func New(baseURL, index string) *Client {
	return &Client{strings.TrimRight(baseURL, "/"), index, &http.Client{Timeout: 800 * time.Millisecond}}
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	body := titleFirstQuery(strings.TrimSpace(query), limit)
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%s/_search", c.baseURL, c.index), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("elasticsearch status %d", res.StatusCode)
	}
	var raw struct {
		Hits struct {
			Hits []struct {
				Score  float64 `json:"_score"`
				Source Hit     `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, err
	}
	result := make([]Hit, 0, len(raw.Hits.Hits))
	for _, item := range raw.Hits.Hits {
		item.Source.Score = item.Score
		result = append(result, item.Source)
	}
	return result, nil
}

// Match only literal titles: exact, prefix, then substring.
func titleFirstQuery(query string, limit int) map[string]any {
	tier := func(filter map[string]any, score int) map[string]any {
		return map[string]any{"constant_score": map[string]any{"filter": filter, "boost": score}}
	}
	keyword := func(kind, value string) map[string]any {
		return map[string]any{kind: map[string]any{"title": map[string]any{"value": value, "case_insensitive": true}}}
	}
	wildcard := "*" + strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`).Replace(query) + "*"
	return map[string]any{
		"size": limit,
		"query": map[string]any{"bool": map[string]any{
			"filter": map[string]any{"term": map[string]any{"language": "zh-Hans"}},
			"must": map[string]any{"dis_max": map[string]any{"tie_breaker": 0, "queries": []any{
				tier(keyword("term", query), 500),
				tier(keyword("prefix", query), 400),
				tier(keyword("wildcard", wildcard), 350),
			}}},
		}},
		"sort": []any{map[string]any{"_score": "desc"}, map[string]any{"updatedAt": "desc"}, map[string]any{"pageId": "asc"}},
	}
}
