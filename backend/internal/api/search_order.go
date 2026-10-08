package api

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Escape LIKE metacharacters explicitly, identically on MySQL and SQLite.
func searchPattern(query string) string {
	return strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(strings.TrimSpace(query)))
}

func matchKnowledge(db *gorm.DB, query string) *gorm.DB {
	like := "%" + searchPattern(query) + "%"
	return db.Where("LOWER(title) LIKE ? ESCAPE '!'", like)
}

// Apply in SQL BEFORE pagination. GORM Order(clause.Expr) is ignored; use the
// OrderBy expression explicitly and include deterministic secondary ordering.
func orderKnowledge(db *gorm.DB, query string) *gorm.DB {
	exact := strings.ToLower(strings.TrimSpace(query))
	prefix := searchPattern(query) + "%"
	return db.Clauses(clause.OrderBy{Expression: clause.Expr{
		SQL:                "CASE WHEN LOWER(title) = ? THEN 0 WHEN LOWER(title) LIKE ? ESCAPE '!' THEN 1 ELSE 2 END ASC, updated_at DESC, id ASC",
		Vars:               []any{exact, prefix},
		WithoutParentheses: true,
	}})
}
