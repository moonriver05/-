package api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPublishedCommunityArticleCanReceiveDatedSupplements(t *testing.T) {
	f := newSecurityFixture(t)
	input := gin.H{
		"title": "整体代换", "category": "微积分", "kind": "例题",
		"difficulty": "入门", "content": "这是第一道题。",
	}
	var article articleResponse
	created := f.call("POST", "/articles", input, f.adminCookie, 201)
	if err := json.Unmarshal(created.Body.Bytes(), &article); err != nil {
		t.Fatal(err)
	}
	adminPath := fmt.Sprintf("/admin/articles/%d", article.ID)
	supplementPath := adminPath + "/supplements"
	body := gin.H{"title": "第二道题", "content": `求 \(u\ge2\) 时的值。`, "supplementedAt": "2000-01-01"}
	f.call("POST", supplementPath, body, nil, 401)
	f.call("POST", supplementPath, body, f.cookie, 403)
	f.call("POST", supplementPath, body, f.adminCookie, 404)
	f.call("PATCH", adminPath, gin.H{"status": "approved", "expectedStatus": "community_pending"}, f.adminCookie, 200)
	f.call("POST", supplementPath, gin.H{"title": " ", "content": "正文"}, f.adminCookie, 400)
	f.call("POST", fmt.Sprintf("/admin/articles/%d/supplements", f.article.ID), body, f.adminCookie, 404)

	var appended struct {
		Section   sectionResponse
		UpdatedAt time.Time
	}
	response := f.call("POST", supplementPath, body, f.adminCookie, 201)
	if err := json.Unmarshal(response.Body.Bytes(), &appended); err != nil {
		t.Fatal(err)
	}
	if appended.Section.SupplementedAt == nil || !appended.Section.SupplementedAt.Equal(appended.UpdatedAt) || appended.UpdatedAt.Year() == 2000 {
		t.Fatalf("server date missing or overridden: %s", response.Body.String())
	}
	f.call("POST", supplementPath, gin.H{"title": "第三道题", "content": "这是第三道题。"}, f.adminCookie, 201)

	var published articleResponse
	response = f.call("GET", "/articles/"+article.Slug, nil, nil, 200)
	if err := json.Unmarshal(response.Body.Bytes(), &published); err != nil {
		t.Fatal(err)
	}
	if published.ID != article.ID || len(published.Sections) != 3 ||
		published.Sections[0].ID != article.Sections[0].ID || published.Sections[0].Content != input["content"] ||
		published.Sections[0].SupplementedAt != nil || published.Sections[1].ID != appended.Section.ID ||
		published.Sections[1].Content != body["content"] || published.Sections[2].Title != "第三道题" ||
		published.Sections[2].SupplementedAt == nil || !published.UpdatedAt.Equal(*published.Sections[2].SupplementedAt) {
		t.Fatalf("append changed the original or lost order/date: %s", response.Body.String())
	}
	f.call("DELETE", adminPath, nil, f.adminCookie, 200)
	f.call("POST", supplementPath, body, f.adminCookie, 404)
}
