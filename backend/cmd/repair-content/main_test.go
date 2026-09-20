package main

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"mathwiki/internal/model"
)

func TestRepairPreservesAnchorsAndIsIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "repair.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err = model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	article := model.Article{PageID: 123, Slug: "zh-123", Title: "例子", Published: true, Tags: "[]", Categories: "[]"}
	if err = db.Create(&article).Error; err != nil {
		t.Fatal(err)
	}
	old := model.Section{ArticleID: article.ID, Position: 1, Title: "定义", Content: "旧正文", Formulae: "[]"}
	obsolete := model.Section{ArticleID: article.ID, Position: 2, Title: "分类杂讯", Content: "Category:数学", Formulae: "[]"}
	db.Create(&old)
	db.Create(&obsolete)
	user := model.User{ID: 1, Name: "学习者"}
	db.Create(&user)
	note := model.Annotation{ArticleID: article.ID, SectionID: &old.ID, UserID: 1, Quote: "旧正文", Body: "已有批注"}
	db.Create(&note)
	input := renderedArticle{PageID: 123, ContentVersion: 2, PlainText: "完整导言\n新正文", Excerpt: "完整导言", Sections: []renderedSection{{Position: 0, Title: "概述", Level: 1, Content: "完整导言", ContentHTML: "<p>完整导言</p>"}, {Position: 1, Title: "定义", Level: 2, Content: "新正文", ContentHTML: "<p>新正文</p>"}}}
	for i := 0; i < 2; i++ {
		if err = repair(db, input); err != nil {
			t.Fatal(err)
		}
	}
	var current model.Section
	db.First(&current, old.ID)
	if current.Hidden || current.Content != "新正文" {
		t.Fatalf("existing section not updated in place: %+v", current)
	}
	db.First(&current, old.ID)
	var preserved model.Annotation
	db.First(&preserved, note.ID)
	if preserved.SectionID == nil || *preserved.SectionID != old.ID || preserved.Body != "已有批注" {
		t.Fatal("annotation changed")
	}
	var count int64
	db.Model(&model.Section{}).Where("article_id = ?", article.ID).Count(&count)
	if count != 3 {
		t.Fatalf("duplicate or deleted section: %d", count)
	}
	db.First(&obsolete, obsolete.ID)
	if !obsolete.Hidden || obsolete.Content != "Category:数学" {
		t.Fatal("obsolete anchor should remain, hidden")
	}
}
