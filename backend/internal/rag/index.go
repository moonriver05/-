package rag

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"time"
)

// limit is the total prefix to index, not an unbounded new batch. Re-running skips
// identical existing IDs, avoiding duplicate records and repeated embedding fees.
func (c *Client) Index(ctx context.Context, db *gorm.DB, limit int, progress func(int, int)) error {
	return c.IndexArticle(ctx, db, limit, 0, progress)
}

// IndexArticle optionally scopes the bounded index pass to one public article.
// Existing collection IDs remain intact and identical chunks are still skipped.
func (c *Client) IndexArticle(ctx context.Context, db *gorm.DB, limit int, articleID uint, progress func(int, int)) error {
	if !c.Configured() {
		return errors.New("set RAG_ENABLED=true and ARK_API_KEY before indexing")
	}
	if limit < 1 || limit > 10000 {
		return errors.New("index limit must be between 1 and 10000")
	}
	col, err := c.collection(ctx, true)
	if err != nil {
		return err
	}
	var count int
	if err = c.request(ctx, "GET", c.records(col.ID, "count"), nil, false, &count); err != nil {
		return err
	}
	processed, added, lastID := 0, 0, uint(0)
	for processed < limit {
		var rows []row
		query := selectedRows(db.WithContext(ctx)).Where("sections.id > ?", lastID)
		if articleID > 0 {
			query = query.Where("sections.article_id = ?", articleID)
		}
		if err := query.Order("sections.id ASC").Limit(20).Scan(&rows).Error; err != nil {
			return err
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
			remaining := limit - processed
			if len(parts) > remaining {
				parts = parts[:remaining]
			}
			ids := make([]string, len(parts))
			for i, part := range parts {
				ids[i] = r.chunkID(i, part)
			}
			var existing struct {
				IDs []string `json:"ids"`
			}
			if err := c.request(ctx, "POST", c.records(col.ID, "get"), map[string]any{"ids": ids, "include": []string{}}, false, &existing); err != nil {
				return err
			}
			found := map[string]bool{}
			for _, id := range existing.IDs {
				found[id] = true
			}
			for i, part := range parts {
				if err := ctx.Err(); err != nil {
					return err
				}
				processed++
				if !found[ids[i]] {
					if count >= 10000 {
						return errors.New("collection reached 10000-record trial cap; evaluate memory before expanding")
					}
					vector, err := c.Embed(ctx, r.vectorText(part), false)
					if err != nil {
						return fmt.Errorf("section %d: %w", r.ID, err)
					}
					body := map[string]any{"ids": []string{ids[i]}, "embeddings": [][]float32{vector}, "metadatas": []any{map[string]any{"article_id": r.ArticleID, "section_id": r.ID, "chunk": i}}}
					if err := c.request(ctx, "POST", c.records(col.ID, "upsert"), body, false, nil); err != nil {
						return err
					}
					count++
					added++
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(200 * time.Millisecond):
					}
				}
				if processed%25 == 0 && progress != nil {
					progress(processed, added)
				}
			}
			if processed == limit {
				break
			}
		}
	}
	if progress != nil {
		progress(processed, added)
	}
	if articleID > 0 && processed == 0 {
		return errors.New("no indexable public sections for the requested article")
	}
	return nil
}
