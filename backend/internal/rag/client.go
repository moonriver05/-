package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const Dimensions = 1024
const Model = "doubao-embedding-vision-251215"

type Client struct {
	Enabled                                   bool
	Key, ArkURL, Model, ChromaURL, Collection string
	HTTP                                      *http.Client
	slots                                     chan struct{}
}

func FromEnv() *Client {
	env := func(key, fallback string) string {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return fallback
	}
	enabled, _ := strconv.ParseBool(env("RAG_ENABLED", "false"))
	return &Client{Enabled: enabled, Key: env("ARK_API_KEY", ""), ArkURL: strings.TrimRight(env("ARK_BASE_URL", "https://ark.cn-beijing.volces.com/api/v3"), "/"), Model: env("ARK_EMBEDDING_MODEL", Model), ChromaURL: strings.TrimRight(env("CHROMA_URL", "http://127.0.0.1:8000"), "/"), Collection: env("CHROMA_COLLECTION", "mathwiki_rag_1024_v1"), HTTP: &http.Client{Timeout: 20 * time.Second}, slots: make(chan struct{}, 2)}
}

func (c *Client) Configured() bool { return c != nil && c.Enabled && c.Key != "" }

// Never include vendor bodies, URLs or credentials in errors exposed to logs/users.
func (c *Client) request(ctx context.Context, method, endpoint string, body any, auth bool, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return errors.New("invalid RAG endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("RAG service unavailable or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("RAG service returned HTTP %d", res.StatusCode)
	}
	if result == nil {
		return nil
	}
	payload, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(payload) > 1<<20 {
		return errors.New("RAG response too large or unreadable")
	}
	if err := json.Unmarshal(payload, result); err != nil {
		return errors.New("invalid RAG response")
	}
	return nil
}

func (c *Client) Embed(ctx context.Context, text string, query bool) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("empty embedding input")
	}
	body := map[string]any{"model": c.Model, "dimensions": Dimensions, "encoding_format": "float", "input": []any{map[string]string{"type": "text", "text": text}}}
	if query {
		body["instructions"] = "Target_modality: text.\nInstruction:根据这个问题，找到能回答这个问题的相关数学或算法知识段落\nQuery:"
	}
	var response struct {
		Data struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.request(ctx, "POST", c.ArkURL+"/embeddings/multimodal", body, true, &response); err != nil {
		return nil, err
	}
	vector := response.Data.Embedding
	if len(vector) != Dimensions {
		return nil, errors.New("embedding dimension mismatch; expected 1024")
	}
	var sum float64
	for _, v := range vector {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, errors.New("invalid embedding value")
		}
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return nil, errors.New("zero embedding")
	}
	norm := math.Sqrt(sum)
	for i := range vector {
		vector[i] = float32(float64(vector[i]) / norm)
	}
	return vector, nil
}

type collection struct {
	ID       string         `json:"id"`
	Metadata map[string]any `json:"metadata"`
}

func (c *Client) collections() string {
	return c.ChromaURL + "/api/v2/tenants/default_tenant/databases/default_database/collections"
}
func (c *Client) collection(ctx context.Context, create bool) (collection, error) {
	var result collection
	method, endpoint := "GET", c.collections()+"/"+url.PathEscape(c.Collection)
	var body any
	if create {
		method, endpoint = "POST", c.collections()
		body = map[string]any{"name": c.Collection, "get_or_create": true, "metadata": map[string]any{"rag_model": c.Model, "rag_dimensions": Dimensions, "rag_format": "chunks800_overlap100_v1"}, "configuration": map[string]any{"hnsw": map[string]any{"space": "cosine"}}}
	}
	err := c.request(ctx, method, endpoint, body, false, &result)
	if err == nil && (result.ID == "" || result.Metadata["rag_model"] != c.Model || result.Metadata["rag_dimensions"] != float64(Dimensions) || result.Metadata["rag_format"] != "chunks800_overlap100_v1") {
		err = errors.New("collection model/dimensions/format mismatch; use a new collection")
	}
	return result, err
}
func (c *Client) records(id, action string) string {
	return c.collections() + "/" + url.PathEscape(id) + "/" + action
}
func (c *Client) Count(ctx context.Context) (int, error) {
	col, err := c.collection(ctx, false)
	if err != nil {
		return 0, err
	}
	var count int
	err = c.request(ctx, "GET", c.records(col.ID, "count"), nil, false, &count)
	return count, err
}
func (c *Client) query(ctx context.Context, col collection, vector []float32, articleID uint) ([]string, error) {
	body := map[string]any{"query_embeddings": [][]float32{vector}, "n_results": 15, "include": []string{"distances"}}
	if articleID > 0 {
		body["where"] = map[string]any{"article_id": articleID}
		body["n_results"] = 3
	}
	var response struct {
		IDs       [][]string  `json:"ids"`
		Distances [][]float64 `json:"distances"`
	}
	if err := c.request(ctx, "POST", c.records(col.ID, "query"), body, false, &response); err != nil {
		return nil, err
	}
	if len(response.IDs) != 1 || len(response.Distances) != 1 || len(response.IDs[0]) != len(response.Distances[0]) || len(response.IDs[0]) > 15 {
		return nil, errors.New("invalid Chroma query response")
	}
	var ids []string
	for i, id := range response.IDs[0] {
		d := response.Distances[0][i]
		if !math.IsNaN(d) && !math.IsInf(d, 0) && d <= .65 {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
