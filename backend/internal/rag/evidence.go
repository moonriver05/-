package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"mathwiki/internal/model"
)

const MaxSectionRunes = 12000
const MaxEvidence = 5

type Evidence struct {
	ArticleID, SectionID                      uint
	Slug, ArticleTitle, SectionTitle, Content string
}
type Status struct {
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
	Count  int    `json:"count"`
}
type row struct {
	ID, ArticleID                      uint
	ArticleTitle, Slug, Title, Content string
}

func publicSections(db *gorm.DB) *gorm.DB {
	return db.Table("sections").Joins("JOIN articles ON articles.id = sections.article_id").Where("articles.deleted_at IS NULL AND articles.published = ? AND articles.language = ? AND articles.review_status NOT IN ? AND sections.hidden = ? AND sections.position < ?", true, "zh-Hans", []string{"community_pending", "rejected"}, false, 1000000)
}
func selectedRows(db *gorm.DB) *gorm.DB {
	return publicSections(db).Select("sections.id, sections.article_id, articles.title AS article_title, articles.slug, SUBSTR(sections.title, 1, 255) AS title, SUBSTR(sections.content, 1, 12000) AS content")
}
func chunks(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > MaxSectionRunes {
		runes = runes[:MaxSectionRunes]
	}
	var result []string
	for start := 0; start < len(runes); start += 700 {
		end := start + 800
		if end > len(runes) {
			end = len(runes)
		}
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			result = append(result, part)
		}
		if end == len(runes) {
			break
		}
	}
	return result
}
func chunkID(section uint, index int, text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("s%d-c%d-%x", section, index, sum[:8])
}
func (r row) vectorText(content string) string {
	return r.ArticleTitle + "\n" + r.Title + "\n" + content
}
func (r row) chunkID(index int, content string) string {
	return chunkID(r.ID, index, r.vectorText(content))
}
func (r row) evidence(content string) Evidence {
	return Evidence{r.ArticleID, r.ID, r.Slug, r.ArticleTitle, r.Title, content}
}

// Fallback is question-aware, bounded current-article evidence, not whole-article prompting.
func Current(article model.Article, question string) []Evidence {
	type scored struct {
		Evidence
		score int
	}
	var candidates []scored
	qr := []rune(strings.ToLower(question))
	var terms []string
	for i := 0; i+1 < len(qr) && len(terms) < 100; i++ {
		term := strings.TrimSpace(string(qr[i : i+2]))
		if len([]rune(term)) == 2 {
			terms = append(terms, term)
		}
	}
	for sectionIndex, section := range article.Sections {
		if sectionIndex >= 60 {
			break
		}
		if section.Hidden || section.Position >= 1000000 {
			continue
		}
		for _, content := range chunks(section.Content) {
			score := 0
			text := strings.ToLower(section.Title + " " + content)
			for _, term := range terms {
				if strings.Contains(text, term) {
					score++
				}
			}
			candidates = append(candidates, scored{Evidence{article.ID, section.ID, article.Slug, article.Title, section.Title, content}, score})
			sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
			if len(candidates) > MaxEvidence {
				candidates = candidates[:MaxEvidence]
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	var result []Evidence
	for i, candidate := range candidates {
		if i == MaxEvidence {
			break
		}
		result = append(result, candidate.Evidence)
	}
	return result
}

// Chroma stores IDs/vectors only. Re-read current public content before any model call;
// deleted, rejected, hidden and changed chunks cannot leak through a stale index.
func (c *Client) Retrieve(ctx context.Context, db *gorm.DB, article model.Article, question string) ([]Evidence, Status) {
	fallback := Current(article, question)
	fail := func(reason string) ([]Evidence, Status) {
		return fallback, Status{"current_article", reason, len(fallback)}
	}
	if !c.Configured() {
		return fail("disabled")
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return fail("busy")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	col, err := c.collection(ctx, false)
	if err != nil {
		return fail("unavailable")
	}
	vector, err := c.Embed(ctx, article.Title+"\n"+question, true)
	if err != nil {
		return fail("unavailable")
	}
	local, err := c.query(ctx, col, vector, article.ID)
	if err != nil {
		return fail("unavailable")
	}
	global, err := c.query(ctx, col, vector, 0)
	if err != nil {
		return fail("unavailable")
	}
	ids := append(local, global...)
	sections := make([]uint, 0, len(ids))
	for _, id := range ids {
		parts := strings.Split(id, "-")
		if len(parts) != 3 {
			continue
		}
		n, e := strconv.ParseUint(strings.TrimPrefix(parts[0], "s"), 10, 32)
		if e == nil {
			sections = append(sections, uint(n))
		}
	}
	if len(sections) == 0 {
		return fail("no_match")
	}
	var rows []row
	if err := selectedRows(db.WithContext(ctx)).Where("sections.id IN ?", sections).Limit(30).Scan(&rows).Error; err != nil {
		return fail("unavailable")
	}
	valid := map[string]Evidence{}
	for _, r := range rows {
		for i, part := range chunks(r.Content) {
			valid[r.chunkID(i, part)] = r.evidence(part)
		}
	}
	var result []Evidence
	seen := map[string]bool{}
	for _, id := range ids {
		if evidence, ok := valid[id]; ok && !seen[id] {
			result = append(result, evidence)
			seen[id] = true
			if len(result) == MaxEvidence {
				break
			}
		}
	}
	if len(result) == 0 {
		return fail("no_match")
	}
	return result, Status{"chroma", "", len(result)}
}

func SourceArticle(article model.Article, evidence []Evidence) model.Article {
	article.Sections = nil
	for _, e := range evidence {
		article.Sections = append(article.Sections, model.Section{ID: e.SectionID, ArticleID: e.ArticleID, Title: e.ArticleTitle + "｜" + e.SectionTitle, Content: e.Content})
	}
	return article
}
