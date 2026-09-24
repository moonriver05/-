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
	return db.Where("(LOWER(title) LIKE ? ESCAPE '!' OR LOWER(source_title) LIKE ? ESCAPE '!' OR LOWER(`lead`) LIKE ? ESCAPE '!' OR LOWER(tags) LIKE ? ESCAPE '!' OR LOWER(plain_text) LIKE ? ESCAPE '!')", like, like, like, like, like)
}

// Apply in SQL BEFORE pagination. GORM Order(clause.Expr) is ignored; use the
// OrderBy expression explicitly and include deterministic secondary ordering.
func orderKnowledge(db *gorm.DB, query string) *gorm.DB {
	exact := strings.ToLower(strings.TrimSpace(query))
	prefix := searchPattern(query) + "%"
	contains := "%" + searchPattern(query) + "%"
	return db.Clauses(clause.OrderBy{Expression: clause.Expr{
		SQL:                "CASE WHEN LOWER(title) = ? THEN 0 WHEN LOWER(source_title) = ? OR LOWER(title) LIKE ? ESCAPE '!' THEN 1 WHEN LOWER(title) LIKE ? ESCAPE '!' OR LOWER(source_title) LIKE ? ESCAPE '!' THEN 2 WHEN LOWER(`lead`) LIKE ? ESCAPE '!' OR LOWER(tags) LIKE ? ESCAPE '!' THEN 3 ELSE 4 END ASC, updated_at DESC, id ASC",
		Vars:               []any{exact, exact, prefix, contains, contains, contains, contains},
		WithoutParentheses: true,
	}})
}
