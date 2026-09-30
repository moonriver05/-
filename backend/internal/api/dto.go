package api

import (
	"encoding/json"
	"time"

	"mathwiki/internal/model"
)

type sectionResponse struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	ContentHTML string `json:"contentHtml,omitempty"`
	Level       int    `json:"level,omitempty"`
	Position    int    `json:"position"`
	Formula     string `json:"formula,omitempty"`
}

type articleResponse struct {
	Published       bool              `json:"published"`
	ID              uint              `json:"id"`
	PageID          int64             `json:"pageId"`
	Slug            string            `json:"slug"`
	Title           string            `json:"title"`
	Excerpt         string            `json:"excerpt"`
	Category        string            `json:"category"`
	Tags            []string          `json:"tags"`
	Kind            string            `json:"kind"`
	Difficulty      string            `json:"difficulty"`
	ReadMinutes     int               `json:"readMinutes"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	Saved           bool              `json:"saved"`
	Sections        []sectionResponse `json:"sections"`
	SourceURL       string            `json:"sourceUrl"`
	License         string            `json:"license"`
	ReviewStatus    string            `json:"reviewStatus"`
	ContentVersion  int               `json:"contentVersion,omitempty"`
	ContentWarnings []string          `json:"contentWarnings,omitempty"`
}

type annotationResponse struct {
	UserID    uint      `json:"userId"`
	Status    string    `json:"status"`
	ID        uint      `json:"id"`
	ArticleID uint      `json:"articleId"`
	SectionID *uint     `json:"sectionId,omitempty"`
	Quote     string    `json:"quote"`
	Body      string    `json:"body"`
	Author    string    `json:"author"`
	Avatar    string    `json:"avatar,omitempty"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"createdAt"`
	Replies   int       `json:"replies"`
	Likes     int       `json:"likes"`
	Liked     bool      `json:"liked"`
}

func toArticleResponse(article model.Article, saved bool) articleResponse {
	result := articleResponse{
		Published: article.Published && !article.DeletedAt.Valid && article.ReviewStatus != "community_pending" && article.ReviewStatus != "rejected",
		ID:        article.ID, PageID: article.PageID, Slug: article.Slug, Title: article.Title,
		Excerpt: article.Lead, Category: article.Category, Tags: decodeList(article.Tags), Kind: article.Kind,
		Difficulty: article.Difficulty, ReadMinutes: article.ReadMinutes, UpdatedAt: article.UpdatedAt,
		Saved: saved, SourceURL: article.SourceURL, License: article.License, ReviewStatus: article.ReviewStatus,
		ContentVersion: article.ContentVersion, ContentWarnings: decodeList(article.ContentWarnings),
		Sections: make([]sectionResponse, 0, len(article.Sections)),
	}
	if result.Kind == "" {
		result.Kind = "概念"
	}
	for _, section := range article.Sections {
		if section.Hidden {
			continue
		}
		formula := ""
		formulae := decodeList(section.Formulae)
		if len(formulae) > 0 {
			formula = formulae[0]
		}
		result.Sections = append(result.Sections, sectionResponse{ID: section.ID, Title: section.Title, Content: section.Content, Formula: formula, ContentHTML: section.ContentHTML, Level: section.Level, Position: section.Position})
	}
	return result
}

func toAnnotationResponse(item model.Annotation) annotationResponse {
	author := item.User.Name
	if author == "" {
		author = "用户"
	}
	return annotationResponse{
		UserID: item.UserID, Status: item.Status,
		ID: item.ID, ArticleID: item.ArticleID, SectionID: item.SectionID, Quote: item.Quote,
		Body: item.Body, Author: author, Avatar: item.User.Avatar, Color: item.Color,
		CreatedAt: item.CreatedAt, Replies: len(item.Comments), Likes: item.Likes,
	}
}

func decodeList(value string) []string {
	result := []string{}
	if value == "" {
		return result
	}
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return []string{}
	}
	return result
}
