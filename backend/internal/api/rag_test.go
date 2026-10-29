package api

import (
	"mathwiki/internal/ai"
	"mathwiki/internal/rag"
	"mathwiki/internal/websearch"
	"testing"
)

func TestRAGCitationMustMatchOriginalAndServerMetadata(t *testing.T) {
	evidence := []rag.Evidence{{ArticleID: 3, SectionID: 17, Slug: "addition", ArticleTitle: "加法", SectionTitle: "定义", Content: "加法用于合并数量。"}}
	result := ai.Explanation{Mode: "model", Kind: "explanation", Citations: []ai.Citation{{Section: "S1", Quote: "加法用于合并数量。", ArticleSlug: "forged", SectionID: 999}, {Section: "S2", Quote: "不存在"}, {Section: "S1", Quote: "数学事实被编造"}}}
	if err := decorateEvidence(&result, evidence, rag.Status{Mode: "chroma", Count: 1}); err != nil {
		t.Fatal(err)
	}
	if len(result.Citations) != 1 || result.Citations[0].ArticleSlug != "addition" || result.Citations[0].SectionID != 17 || result.Retrieval.Mode != "chroma" {
		t.Fatalf("bad citations: %+v", result)
	}
	result.Citations = []ai.Citation{{Section: "S1", Quote: "无法在原文找到"}}
	if err := decorateEvidence(&result, evidence, rag.Status{}); err == nil {
		t.Fatal("fabricated citations accepted")
	}
	result.Kind = "conversation"
	result.Citations = nil
	if err := decorateEvidence(&result, evidence, rag.Status{}); err != nil {
		t.Fatal("casual chat must not require citations")
	}
}

func TestBalancedAnswerDropsForgedCitationsAndLabelsUncitedExplanations(t *testing.T) {
	answer := ai.Explanation{Mode: "model", Kind: "explanation", AnswerMode: ai.BalancedAnswer, Summary: "模型推导", Confidence: .99, Citations: []ai.Citation{{Section: "S1", Quote: "虚构引文", SourceURL: "javascript:alert(1)"}}}
	if err := decorateEvidence(&answer, nil, rag.Status{}); err != nil || len(answer.Citations) != 0 || answer.Grounding != "model" || answer.Confidence != 0 {
		t.Fatalf("uncited answer mislabeled: %+v %v", answer, err)
	}
}

func TestWebCitationUsesExactTextAndSearchOwnedURLs(t *testing.T) {
	evidence := []rag.Evidence{{ArticleID: 1, SectionID: 2, Content: "链式法则。"}, {ArticleTitle: "大学课程", SectionTitle: "网页正文节选", Content: "复合函数的导数可以用链式法则计算。"}}
	web := websearch.Result{Title: "大学课程", URL: "https://example.edu/course", Content: evidence[1].Content}
	answer := ai.Explanation{Mode: "model", AnswerMode: ai.BalancedAnswer, Citations: []ai.Citation{{Section: "S2", Quote: "复合函数的导数", SourceURL: "https://forged.example", ArticleID: 999}}}
	if err := decorateEvidence(&answer, evidence, rag.Status{}, web); err != nil {
		t.Fatal(err)
	}
	if len(answer.Citations) != 1 || answer.Citations[0].SourceURL != web.URL || answer.Citations[0].SourceType != "web" || answer.Citations[0].ArticleID != 0 || answer.Grounding != "mixed" {
		t.Fatalf("untrusted citation metadata: %+v", answer)
	}
}
