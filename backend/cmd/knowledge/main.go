// knowledge exports a public, field-allowlisted snapshot, never a database dump.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mathwiki/internal/model"
)

const maxPart = 20 * 1024 * 1024

// Keep this schema explicit: adding a private field to a database model must not
// silently add it to the published dataset. Local row IDs are never exported.
type article struct {
	PageID          int64     `json:"pageId"`
	RevisionID      int64     `json:"revisionId"`
	Title           string    `json:"title"`
	SourceTitle     string    `json:"sourceTitle"`
	Slug            string    `json:"slug"`
	Lead            string    `json:"excerpt"`
	Category        string    `json:"category"`
	Categories      []string  `json:"categories"`
	Tags            []string  `json:"tags"`
	Kind            string    `json:"kind"`
	Difficulty      string    `json:"difficulty"`
	ReadMinutes     int       `json:"readMinutes"`
	Language        string    `json:"language"`
	SourceURL       string    `json:"sourceUrl"`
	License         string    `json:"license"`
	ReviewStatus    string    `json:"reviewStatus"`
	ContentVersion  int       `json:"contentVersion"`
	ContentWarnings []string  `json:"contentWarnings"`
	Sections        []section `json:"sections"`
}
type section struct {
	Position    int      `json:"position"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	ContentHTML string   `json:"contentHtml"`
	Level       int      `json:"level"`
	Formulae    []string `json:"formulae"`
}
type part struct {
	File     string `json:"file"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
	Articles int    `json:"articles"`
	Sections int    `json:"sections"`
}
type manifest struct {
	Version    int      `json:"version"`
	Articles   int      `json:"articles"`
	Sections   int      `json:"sections"`
	Categories []string `json:"categories"`
	Parts      []part   `json:"parts"`
}

func main() {
	mode := flag.String("mode", "verify", "export, verify, or import (empty content tables only)")
	dir := flag.String("dir", "../datasets/public-knowledge", "snapshot directory")
	flag.Parse()
	if err := run(*mode, *dir); err != nil {
		// Database/parse errors may contain credentials or content. Do not echo them.
		fmt.Fprintln(os.Stderr, "Knowledge operation failed:", err)
		os.Exit(1)
	}
}

func run(mode, dir string) error {
	if mode == "verify" {
		return walk(dir, nil)
	}
	if mode != "export" && mode != "import" {
		return errors.New("unknown mode")
	}
	if mode == "import" {
		if err := walk(dir, nil); err != nil {
			return err
		}
	}
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return errors.New("MYSQL_DSN must be configured explicitly")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return errors.New("database connection failed (details redacted)")
	}
	conn, err := db.DB()
	if err != nil {
		return errors.New("database handle unavailable")
	}
	defer conn.Close()
	if mode == "export" {
		return export(db, dir, maxPart)
	}
	return restore(db, dir)
}

func list(raw string) []string {
	values := []string{}
	_ = json.Unmarshal([]byte(raw), &values)
	if values == nil {
		return []string{}
	}
	return values
}

func publicQuery(db *gorm.DB) *gorm.DB {
	return db.Model(&model.Article{}).Where("published = ? AND review_status NOT IN ? AND page_id > 0 AND language = ? AND source_url LIKE ?", true, []string{"community_pending", "rejected"}, "zh-Hans", "https://zh.wikipedia.org/wiki/%")
}

func export(db *gorm.DB, dir string, limit int) error {
	// Refuse overwriting any existing directory, including a previous snapshot.
	if err := os.Mkdir(dir, 0755); err != nil {
		return errors.New("export requires a new directory with an existing parent")
	}
	m := manifest{Version: 1}
	categories := map[string]bool{}
	var buffer bytes.Buffer
	p := part{}
	flush := func() error {
		if p.Articles == 0 {
			return nil
		}
		p.File = fmt.Sprintf("articles-%03d.jsonl", len(m.Parts)+1)
		p.Bytes = buffer.Len()
		sum := sha256.Sum256(buffer.Bytes())
		p.SHA256 = hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(dir, p.File), buffer.Bytes(), 0644); err != nil {
			return errors.New("cannot write snapshot part")
		}
		m.Parts = append(m.Parts, p)
		buffer.Reset()
		p = part{}
		return nil
	}
	// Read one consistent snapshot, without schema changes or writes to MySQL.
	err := db.Transaction(func(tx *gorm.DB) error {
		var last uint
		for {
			var rows []model.Article
			columns := strings.Split("id,page_id,revision_id,title,source_title,slug,lead,category,categories,tags,kind,difficulty,read_minutes,language,source_url,license,review_status,content_version,content_warnings", ",")
			if err := publicQuery(tx).Select(columns).Where("id > ?", last).Order("id").Limit(100).Find(&rows).Error; err != nil {
				return errors.New("article read failed")
			}
			if len(rows) == 0 {
				break
			}
			for _, row := range rows {
				last = row.ID
				a := article{PageID: row.PageID, RevisionID: row.RevisionID, Title: row.Title, SourceTitle: row.SourceTitle, Slug: row.Slug, Lead: row.Lead, Category: row.Category, Categories: list(row.Categories), Tags: list(row.Tags), Kind: row.Kind, Difficulty: row.Difficulty, ReadMinutes: row.ReadMinutes, Language: row.Language, SourceURL: row.SourceURL, License: row.License, ReviewStatus: row.ReviewStatus, ContentVersion: row.ContentVersion, ContentWarnings: list(row.ContentWarnings), Sections: []section{}}
				var sections []model.Section
				if err := tx.Select([]string{"position", "title", "content", "content_html", "level", "formulae"}).Where("article_id = ? AND hidden = ?", row.ID, false).Order("position,id").Find(&sections).Error; err != nil {
					return errors.New("section read failed")
				}
				for _, s := range sections {
					a.Sections = append(a.Sections, section{Position: s.Position, Title: s.Title, Content: s.Content, ContentHTML: s.ContentHTML, Level: s.Level, Formulae: list(s.Formulae)})
				}
				if err := validate(a); err != nil {
					return err
				}
				data, err := json.Marshal(a)
				if err != nil {
					return errors.New("article encoding failed")
				}
				data = append(data, '\n')
				if len(data) > limit {
					return errors.New("one article exceeds the part size limit")
				}
				if buffer.Len()+len(data) > limit {
					if err := flush(); err != nil {
						return err
					}
				}
				buffer.Write(data)
				p.Articles++
				p.Sections += len(a.Sections)
				m.Articles++
				m.Sections += len(a.Sections)
				categories[a.Category] = true
			}
		}
		return flush()
	})
	if err != nil {
		return err
	}
	if m.Articles == 0 {
		return errors.New("no public Wikipedia articles found")
	}
	for name := range categories {
		if name != "" {
			m.Categories = append(m.Categories, name)
		}
	}
	sort.Strings(m.Categories)
	data, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), append(data, '\n'), 0644); err != nil {
		return errors.New("cannot write manifest")
	}
	return walk(dir, nil)
}

func validate(a article) error {
	if a.PageID <= 0 || a.RevisionID <= 0 || a.Title == "" || a.Slug == "" || a.Language != "zh-Hans" || !strings.HasPrefix(a.SourceURL, "https://zh.wikipedia.org/wiki/") || a.License != "CC BY-SA 4.0" || (a.ReviewStatus != "machine_selected" && a.ReviewStatus != "approved") || len(a.Sections) == 0 {
		return errors.New("record is not eligible public Chinese Wikipedia content")
	}
	return nil
}

func strict(data []byte, dest any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return errors.New("invalid snapshot schema (details redacted)")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func walk(dir string, visit func(article) error) error {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return errors.New("manifest unavailable")
	}
	var m manifest
	if err := strict(data, &m); err != nil {
		return err
	}
	if m.Version != 1 || m.Articles < 1 || len(m.Parts) == 0 {
		return errors.New("invalid manifest")
	}
	articleCount, sectionCount := 0, 0
	ids := map[int64]bool{}
	slugs := map[string]bool{}
	categories := map[string]bool{}
	for i, p := range m.Parts {
		if p.File != fmt.Sprintf("articles-%03d.jsonl", i+1) || p.Bytes > maxPart || p.Bytes < 1 {
			return errors.New("invalid part name or size")
		}
		path := filepath.Join(dir, p.File)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() != int64(p.Bytes) {
			return errors.New("part missing or invalid size/type")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return errors.New("cannot read part")
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != p.SHA256 {
			return errors.New("part checksum mismatch")
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), maxPart)
		ac, sc := 0, 0
		for scanner.Scan() {
			var a article
			if err := strict(scanner.Bytes(), &a); err != nil {
				return err
			}
			if err := validate(a); err != nil {
				return err
			}
			if ids[a.PageID] || slugs[a.Slug] {
				return errors.New("duplicate article identity")
			}
			ids[a.PageID] = true
			slugs[a.Slug] = true
			categories[a.Category] = true
			if visit != nil {
				if err := visit(a); err != nil {
					return err
				}
			}
			ac++
			sc += len(a.Sections)
		}
		if scanner.Err() != nil {
			return errors.New("cannot parse part")
		}
		if ac != p.Articles || sc != p.Sections {
			return errors.New("part counts mismatch")
		}
		articleCount += ac
		sectionCount += sc
	}
	if articleCount != m.Articles || sectionCount != m.Sections {
		return errors.New("total counts mismatch")
	}
	names := []string{}
	for name := range categories {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if strings.Join(names, "\x00") != strings.Join(m.Categories, "\x00") {
		return errors.New("category manifest mismatch")
	}
	fmt.Printf("Verified public knowledge: %d articles, %d sections, %d parts\n", articleCount, sectionCount, len(m.Parts))
	return nil
}

func jsonList(values []string) string { data, _ := json.Marshal(values); return string(data) }

func restore(db *gorm.DB, dir string) error {
	// No automatic schema migration of user/session tables; no upserts or deletes.
	if err := db.AutoMigrate(&model.Article{}, &model.Section{}, &model.Category{}); err != nil {
		return errors.New("content schema migration failed")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{&model.Article{}, &model.Section{}} {
			var n int64
			if err := tx.Unscoped().Model(table).Count(&n).Error; err != nil {
				return errors.New("cannot check destination")
			}
			if n != 0 {
				return errors.New("destination content tables must be empty; existing data will not be overwritten")
			}
		}
		categories := map[string]bool{}
		err := walk(dir, func(a article) error {
			texts := []string{}
			row := model.Article{PageID: a.PageID, RevisionID: a.RevisionID, Title: a.Title, SourceTitle: a.SourceTitle, Slug: a.Slug, Lead: a.Lead, Category: a.Category, Categories: jsonList(a.Categories), Tags: jsonList(a.Tags), Kind: a.Kind, Difficulty: a.Difficulty, ReadMinutes: a.ReadMinutes, Language: a.Language, SourceURL: a.SourceURL, License: a.License, ReviewStatus: a.ReviewStatus, ContentVersion: a.ContentVersion, ContentWarnings: jsonList(a.ContentWarnings), Published: true}
			for _, s := range a.Sections {
				texts = append(texts, s.Content)
				row.Sections = append(row.Sections, model.Section{Position: s.Position, Title: s.Title, Content: s.Content, ContentHTML: s.ContentHTML, Level: s.Level, Formulae: jsonList(s.Formulae)})
			}
			row.PlainText = strings.Join(texts, "\n\n")
			if err := tx.Create(&row).Error; err != nil {
				return errors.New("article import failed; transaction rolled back")
			}
			categories[a.Category] = true
			return nil
		})
		if err != nil {
			return err
		}
		for name := range categories {
			if name == "" {
				continue
			}
			var category model.Category
			if err := tx.Where("name = ?", name).FirstOrCreate(&category, model.Category{Name: name}).Error; err != nil {
				return errors.New("category import failed")
			}
		}
		return nil
	})
}
