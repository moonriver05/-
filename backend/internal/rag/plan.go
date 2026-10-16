package rag

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"unicode/utf8"
)

type Plan struct {
	Articles           int     `json:"articles"`
	Sections           int     `json:"sections"`
	Chunks             int     `json:"chunks"`
	CappedSections     int     `json:"cappedSections"`
	RawVectorMiB       float64 `json:"rawVectorMiB"`
	CoverageSectionIDs []uint  `json:"-"`
}

// Estimate scans bounded pages without embedding calls or collection writes.
// Its count follows the same 12000-rune-per-section limit as actual indexing.
func Estimate(ctx context.Context, db *gorm.DB) (Plan, error) {
	var plan Plan
	seen := map[uint]bool{}
	lastID := uint(0)
	for {
		var rows []row
		if err := selectedRows(db.WithContext(ctx)).Where("sections.id > ?", lastID).Order("sections.id ASC").Limit(40).Scan(&rows).Error; err != nil {
			return plan, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			lastID = r.ID
			parts := chunks(r.Content)
			if len(parts) == 0 {
				continue
			}
			if !seen[r.ArticleID] {
				plan.CoverageSectionIDs = append(plan.CoverageSectionIDs, r.ID)
			}
			seen[r.ArticleID] = true
			plan.Sections++
			plan.Chunks += len(parts)
			if utf8.RuneCountInString(r.Content) >= MaxSectionRunes {
				plan.CappedSections++
			}
		}
	}
	plan.Articles = len(seen)
	plan.RawVectorMiB = float64(plan.Chunks) * Dimensions * 4 / (1024 * 1024)
	return plan, nil
}

// Coverage adds one first valid chunk per article, rather than spending the
// entire budget on all chapters of the first few articles.
func (c *Client) IndexCoverage(ctx context.Context, db *gorm.DB, maxRecords int, progress func(int, int)) error {
	plan, err := Estimate(ctx, db)
	if err != nil {
		return err
	}
	if plan.Articles == 0 {
		return errors.New("no indexable public articles")
	}
	if plan.Articles > maxRecords {
		return fmt.Errorf("coverage needs at least %d vectors, above %d-record budget", plan.Articles, maxRecords)
	}
	return c.index(ctx, db, plan.Articles, 0, plan.CoverageSectionIDs, maxRecords, progress)
}

func (c *Client) IndexAll(ctx context.Context, db *gorm.DB, maxRecords int, progress func(int, int)) error {
	plan, err := Estimate(ctx, db)
	if err != nil {
		return err
	}
	if plan.Chunks == 0 {
		return errors.New("no indexable public chunks")
	}
	if plan.Chunks > maxRecords {
		return fmt.Errorf("all indexable text needs %d vectors (raw %.1f MiB before index overhead), above %d-record budget; inspect -plan before raising -max-records", plan.Chunks, plan.RawVectorMiB, maxRecords)
	}
	return c.index(ctx, db, plan.Chunks, 0, nil, maxRecords, progress)
}
