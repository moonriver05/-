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
