package api

import (
	"testing"

	"mathwiki/internal/model"
)

func TestToArticleResponseDecodesChineseMetadata(t *testing.T) {
	article := model.Article{
		ID: 7, Title: "贝叶斯定理", Tags: `["概率论","条件概率"]`, Kind: "概念",
		Sections: []model.Section{{ID: 2, Title: "公式", Content: "后验概率", Formulae: `["P(A|B)=P(B|A)P(A)/P(B)"]`}},
	}
	result := toArticleResponse(article, true)
	if !result.Saved || len(result.Tags) != 2 || result.Tags[0] != "概率论" {
		t.Fatalf("unexpected article metadata: %#v", result)
	}
	if len(result.Sections) != 1 || result.Sections[0].Formula == "" {
		t.Fatalf("formula was not decoded: %#v", result.Sections)
	}
}

func TestQuoteBelongsToRequestedSection(t *testing.T) {
	sectionID := uint(11)
	article := model.Article{Sections: []model.Section{{ID: sectionID, Content: "所有边权必须非负。"}}}
	if !quoteBelongsToArticle(article, "边权必须非负", &sectionID) {
		t.Fatal("expected quote to be accepted")
	}
	otherSection := uint(12)
	if quoteBelongsToArticle(article, "边权必须非负", &otherSection) {
		t.Fatal("quote from a different section must be rejected")
	}
}

func TestAnnotationResponsePreservesTextAnchor(t *testing.T) {
	sectionID := uint(11)
	result := toAnnotationResponse(model.Annotation{
		ID: 4, SectionID: &sectionID, Quote: "相同句子", Prefix: "第二处：", Suffix: "下一句",
		StartOffset: 17, EndOffset: 21,
	})
	if result.SectionID == nil || *result.SectionID != sectionID || result.Prefix != "第二处：" || result.Suffix != "下一句" || result.StartOffset != 17 || result.EndOffset != 21 {
		t.Fatalf("annotation anchor was lost: %#v", result)
	}
}

func TestAnnotationQuoteAllowsRenderedWhitespaceOnly(t *testing.T) {
	if !containsAnnotationQuote("第一段。\n第二段。", "第一段。第二段。") || !containsAnnotationQuote("a b", "a\u00a0b") {
		t.Fatal("HTML paragraph/space differences should not reject a real selection")
	}
	if containsAnnotationQuote("边权必须非负。", "边权可以为负。") || containsAnnotationQuote("无空白", " \n ") {
		t.Fatal("changed words and whitespace-only selections must be rejected")
	}
}
