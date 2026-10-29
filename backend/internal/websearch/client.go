// Package websearch adds optional DeepSeek Search results without fetching
// arbitrary URLs from the application server or storing web pages in Chroma.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Result struct {
	Title, URL, Content string
	SummaryOnly         bool
}

type Client struct {
	Enabled      bool
	Key, BaseURL string
	HTTP         *http.Client
}

func FromEnv() *Client {
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("DEEPSEEK_SEARCH_ENABLED")))
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("DEEPSEEK_SEARCH_BASE_URL")), "/")
	if base == "" {
		base = "https://api.deepseek.com"
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_SEARCH_API_KEY"))
	llmBase := strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/")
	// Only reuse between official endpoints; proxies need an explicit credential.
	officialSearch := base == "https://api.deepseek.com" || base == "https://api.deepseek.com/v1"
	if key == "" && officialSearch && (llmBase == "" || llmBase == "https://api.deepseek.com" || llmBase == "https://api.deepseek.com/v1") {
		key = strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	}
	return &Client{Enabled: enabled, Key: key, BaseURL: base, HTTP: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) Configured() bool { return c != nil && c.Enabled && c.Key != "" }

func clip(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

func validSource(value string) bool {
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsMulticast()) {
		return false
	}
	return true
}

func (c *Client) Search(ctx context.Context, query, question string) ([]Result, error) {
	if !c.Configured() {
		return nil, errors.New("web search unavailable")
	}
	query = clip(strings.Join(strings.Fields(query), " "), 128)
	if query == "" {
		return nil, errors.New("empty search query")
	}
	payload, _ := json.Marshal(map[string]any{"query": query, "ori_question": clip(question, 128), "count": 3, "need_content": true})
	base := strings.TrimSuffix(strings.TrimRight(c.BaseURL, "/"), "/v1")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/tools/search", bytes.NewReader(payload))
	if err != nil {
		return nil, errors.New("web search unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+c.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("web search unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("web search unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		return nil, errors.New("web search response too large")
	}
	var response struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
			Summary string `json:"summary"`
		} `json:"results"`
	}
	if json.Unmarshal(data, &response) != nil || response.Results == nil {
		return nil, errors.New("web search response invalid")
	}
	results := []Result{}
	seen := map[string]bool{}
	for _, item := range response.Results {
		if len(results) >= 3 {
			break
		}
		if !validSource(item.URL) || seen[item.URL] {
			continue
		}
		body := strings.TrimSpace(item.Content)
		summaryOnly := body == ""
		if summaryOnly {
			body = strings.TrimSpace(item.Summary)
		}
		if body == "" {
			continue
		}
		title := clip(item.Title, 180)
		if title == "" {
			title = "网页资料"
		}
		results = append(results, Result{Title: title, URL: item.URL, Content: clip(body, 1800), SummaryOnly: summaryOnly})
		seen[item.URL] = true
	}
	return results, nil
}
