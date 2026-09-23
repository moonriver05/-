package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mathwiki/internal/model"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	t.Cleanup(func() { conn.Close() })
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func fixture(t *testing.T, db *gorm.DB, id int64, change func(*model.Article)) model.Article {
	t.Helper()
	a := model.Article{PageID: id, RevisionID: 123, Title: "公开数学", SourceTitle: "公開數學", Slug: strings.Repeat("a", int(id)), Lead: "摘要", Category: "线性代数", Categories: `["数学"]`, Tags: `["教学"]`, Kind: "概念", Difficulty: "入门", Language: "zh-Hans", SourceURL: "https://zh.wikipedia.org/wiki/数学", License: "CC BY-SA 4.0", ReviewStatus: "machine_selected", Published: true, RawWikitext: "PRIVATE_RAW_CANARY", PlainText: "PRIVATE_OLD_TEXT_CANARY", Sections: []model.Section{{Position: 1, Title: "定义", Content: "完整正文", ContentHTML: "<p>完整正文</p>", Formulae: `["x=1"]`}, {Position: 2, Title: "旧版", Content: "PRIVATE_HIDDEN_CANARY", Hidden: true}}}
	if change != nil {
		change(&a)
	}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	// GORM's default tags are not used to decide which records are public.
	if !a.Published {
		if err := db.Model(&a).Update("published", false).Error; err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func TestPublicExportAndRestore(t *testing.T) {
	db := database(t)
	fixture(t, db, 1, nil)
	fixture(t, db, 2, nil)
	fixture(t, db, 3, func(a *model.Article) { a.ReviewStatus = "community_pending"; a.Title = "PRIVATE_PENDING_CANARY" })
	fixture(t, db, 4, func(a *model.Article) { a.ReviewStatus = "rejected" })
	fixture(t, db, 5, func(a *model.Article) { a.Published = false })
	deleted := fixture(t, db, 6, nil)
	if err := db.Delete(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	fixture(t, db, 7, func(a *model.Article) { a.SourceURL = "https://example.com/private" })
	fixture(t, db, 8, func(a *model.Article) { a.PageID = -8 })
	if err := db.Create(&model.User{Name: "PRIVATE_USER_CANARY", PasswordHash: "PRIVATE_HASH_CANARY"}).Error; err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "snapshot")
	if err := export(db, dir, 700); err != nil {
		t.Fatal(err)
	}
	var m manifest
	data, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	json.Unmarshal(data, &m)
	if m.Articles != 2 || m.Sections != 2 || len(m.Parts) != 2 {
		t.Fatalf("unexpected counts: %+v", m)
	}
	for _, p := range m.Parts {
		data, _ := os.ReadFile(filepath.Join(dir, p.File))
		if strings.Contains(string(data), "PRIVATE_") {
			t.Fatal("private canary leaked")
		}
	}
	if err := export(db, dir, maxPart); err == nil {
		t.Fatal("must not overwrite export")
	}
	destination := database(t)
	if err := restore(destination, dir); err != nil {
		t.Fatal(err)
	}
	var count int64
	destination.Model(&model.Article{}).Count(&count)
	if count != 2 {
		t.Fatal(count)
	}
	destination.Model(&model.Section{}).Count(&count)
	if count != 2 {
		t.Fatal(count)
	}
	destination.Model(&model.User{}).Count(&count)
	if count != 0 {
		t.Fatal("imported a user")
	}
	var a model.Article
	destination.Preload("Sections").First(&a)
	if !a.Published || a.PlainText != "完整正文" || a.Sections[0].ContentHTML != "<p>完整正文</p>" || a.RawWikitext != "" {
		t.Fatal("content round trip failed")
	}
	if err := restore(destination, dir); err == nil {
		t.Fatal("must refuse non-empty database")
	}
	// Source was not changed by export.
	db.Unscoped().Model(&model.Article{}).Count(&count)
	if count != 8 {
		t.Fatal("source was modified")
	}
}

func TestRejectTamperingAndPrivateFields(t *testing.T) {
	db := database(t)
	fixture(t, db, 1, nil)
	dir := filepath.Join(t.TempDir(), "snapshot")
	if err := export(db, dir, maxPart); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "articles-001.jsonl")
	data, _ := os.ReadFile(path)
	data = append([]byte(`{"passwordHash":"DO_NOT_EXPORT",`), data[1:]...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := walk(dir, nil); err == nil {
		t.Fatal("tampering not detected")
	}
	// Even with an updated checksum, unknown/private fields must be rejected.
	manifestPath := filepath.Join(dir, "manifest.json")
	md, _ := os.ReadFile(manifestPath)
	var m manifest
	json.Unmarshal(md, &m)
	sum := sha256.Sum256(data)
	m.Parts[0].Bytes = len(data)
	m.Parts[0].SHA256 = hex.EncodeToString(sum[:])
	md, _ = json.Marshal(m)
	os.WriteFile(manifestPath, md, 0600)
	if err := walk(dir, nil); err == nil {
		t.Fatal("private schema field not rejected")
	}
	destination := database(t)
	if err := restore(destination, dir); err == nil {
		t.Fatal("invalid snapshot imported")
	}
	var n int64
	destination.Model(&model.Article{}).Count(&n)
	if n != 0 {
		t.Fatal("failed import not rolled back")
	}
}

// Opt-in full snapshot round trip. All writes go to a temporary SQLite database,
// never to MYSQL_DSN or the running application's database.
func TestRepositorySnapshotRoundTrip(t *testing.T) {
	if os.Getenv("KNOWLEDGE_SNAPSHOT_TEST") != "1" {
		t.Skip("set KNOWLEDGE_SNAPSHOT_TEST=1 for the full dataset import test")
	}
	dir := filepath.Join("..", "..", "..", "datasets", "public-knowledge")
	if err := walk(dir, nil); err != nil {
		t.Fatal(err)
	}
	db := database(t)
	if err := restore(db, dir); err != nil {
		t.Fatal(err)
	}
	var users int64
	db.Model(&model.User{}).Count(&users)
	if users != 0 {
		t.Fatal("snapshot must not create accounts")
	}
	copyDir := filepath.Join(t.TempDir(), "roundtrip")
	if err := export(db, copyDir, maxPart); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	copy, err := os.ReadFile(filepath.Join(copyDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Matching all part checksums proves every public field survived import.
	var before, after manifest
	json.Unmarshal(original, &before)
	json.Unmarshal(copy, &after)
	left, _ := json.Marshal(before)
	right, _ := json.Marshal(after)
	if string(left) != string(right) {
		t.Fatal("snapshot round trip changed public content")
	}
}
