package api

import (
	"mathwiki/internal/ai"
	"mathwiki/internal/rag"
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
