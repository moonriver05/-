package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mathwiki/internal/model"
)

type Step struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}
type Citation struct {
	Section string `json:"section"`
	Quote   string `json:"quote"`
}
type Explanation struct {
	Mode       string     `json:"mode"`
	Kind       string     `json:"kind,omitempty"`
	Summary    string     `json:"summary"`
	Steps      []Step     `json:"steps"`
	Citations  []Citation `json:"citations"`
	Confidence float64    `json:"confidence"`
	Checks     []string   `json:"checks"`
}

type Explainer struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func New(baseURL, apiKey, modelName string) *Explainer {
	return &Explainer{strings.TrimRight(baseURL, "/"), apiKey, modelName, &http.Client{Timeout: 90 * time.Second}}
}

func (e *Explainer) Model() string    { return e.model }
func (e *Explainer) Configured() bool { return e.apiKey != "" }

func (e *Explainer) Explain(ctx context.Context, article model.Article, question, audience, style string) (Explanation, error) {
	if e.apiKey == "" {
		return fallback(article, question, audience), nil
	}
	sources := buildSources(article.Sections)
	draftPrompt := fmt.Sprintf(`你是中文数学教学助手。只能根据“证据章节”回答，不能补充证据中没有的事实。
读者水平：%s
解释方式：%s
问题：%s

证据章节：
%s

输出严格 JSON，不要 Markdown：
{"summary":"一句话答案","steps":[{"title":"步骤标题","content":"解释"}],"citations":[{"section":"章节编号与标题","quote":"证据中的短句"}],"confidence":0到1之间的数字,"checks":["已检查事项"]}`, audience, style, question, sources)
	draftPrompt += "\n" + teachingGuidance(audience, style)
	draft, err := e.complete(ctx, "你负责生成有出处、易理解的中文数学解释。", draftPrompt)
	if err != nil {
		return Explanation{}, err
	}

	verifyPrompt := fmt.Sprintf(`请核验下面的数学解释。逐项检查定义、前提、公式、复杂度和引用是否能由证据支持；删除或改正无依据内容。

证据章节：
%s

待核验解释：
%s

输出与待核验解释完全相同字段的严格 JSON。checks 必须列出具体完成的核验，confidence 对证据覆盖程度评分。`, sources, draft)
	verifyPrompt += "\n" + teachingGuidance(audience, style)
	verified, err := e.complete(ctx, "你是严谨的中文数学内容核验员，不允许保留无法由给定证据支持的结论。", verifyPrompt)
	if err != nil {
		return Explanation{}, err
	}
	var result Explanation
	if err := json.Unmarshal(extractJSON(verified), &result); err != nil {
		return Explanation{}, fmt.Errorf("解析核验结果: %w", err)
	}
	if len(result.Citations) == 0 {
		return Explanation{}, errors.New("核验结果没有可回溯引用")
	}
	result.Mode = "model"
	return result, nil
}

func buildSources(sections []model.Section) string {
	var builder strings.Builder
	for i, section := range sections {
		if section.Hidden || section.Position >= 1000000 || strings.TrimSpace(section.Content) == "" {
			continue
		}
		content := section.Content
		if len([]rune(content)) > 1800 {
			content = string([]rune(content)[:1800])
		}
		fmt.Fprintf(&builder, "[S%d｜%s]\n%s\n\n", i+1, section.Title, content)
	}
	return builder.String()
}

func (e *Explainer) complete(ctx context.Context, system, prompt string) (string, error) {
	payload := map[string]any{
		"model": e.model, "temperature": 0.2,
		"max_tokens":      4096,
		"response_format": map[string]string{"type": "json_object"},
		"messages":        []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": prompt}},
	}
	// DeepSeek defaults to thinking mode; keep the two-pass explanation responsive.
	if strings.HasPrefix(e.model, "deepseek-") {
		payload["thinking"] = map[string]string{"type": "disabled"}
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return "", fmt.Errorf("LLM status %d: %s", res.StatusCode, body)
	}
	var response struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return "", err
	}
	if len(response.Choices) == 0 || strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return "", errors.New("LLM 未返回内容")
	}
	if response.Choices[0].FinishReason == "length" {
		return "", errors.New("模型输出达到长度限制，请缩小问题范围后重试")
	}
	return response.Choices[0].Message.Content, nil
}

func extractJSON(text string) []byte {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return []byte(strings.TrimSpace(text))
}

func fallback(article model.Article, question, audience string) Explanation {
	section := "当前条目"
	quote := article.Lead
	if len(article.Sections) > 0 {
		section = "S1｜" + article.Sections[0].Title
		quote = article.Sections[0].Content
	}
	runes := []rune(quote)
	if len(runes) > 70 {
		quote = string(runes[:70]) + "……"
	}
	return Explanation{
		Mode:      "demo",
		Summary:   fmt.Sprintf("这是面向%s的本地演示解释：%s", audience, article.Lead),
		Steps:     []Step{{Title: "从定义开始", Content: "先明确题目中的对象、已知条件和需要得到的结论。"}, {Title: "联系当前词条", Content: "把问题对应到正文中的定义或算法步骤，再逐项验证前提。"}},
		Citations: []Citation{{Section: section, Quote: quote}}, Confidence: .72,
		Checks: []string{"已限制为当前中文词条内容", "已附加章节引用", "当前为未配置模型密钥时的演示回答"},
	}
}
