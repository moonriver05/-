package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"mathwiki/internal/model"
)

func TestAIGateQuotaAcrossEndpointsAndWindowReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := newAIGate()
	g.perMinute = 2
	now := time.Now()
	g.now = func() time.Time { return now }
	r := gin.New()
	for _, path := range []string{"/sync", "/stream"} {
		r.POST(path, func(c *gin.Context) { c.Set("user", model.User{ID: 1}) }, g.admit, func(c *gin.Context) { c.Status(204) })
	}
	for i, path := range []string{"/sync", "/stream", "/sync"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		want := 204
		if i == 2 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("request %d: %d", i, w.Code)
		}
		if i == 2 && w.Header().Get("Retry-After") == "" {
			t.Fatal("missing retry delay")
		}
	}
	now = now.Add(time.Minute)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/sync", nil))
	if w.Code != 204 || g.running != 0 {
		t.Fatal("window reset or slot release failed")
	}
}

func TestAIGateRejectsConcurrentWorkWithoutConsumingQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := newAIGate()
	g.concurrent = 1
	r := gin.New()
	r.POST("/:id", func(c *gin.Context) {
		id := uint(1)
		if c.Param("id") == "other" {
			id = 2
		}
		c.Set("user", model.User{ID: id})
	}, g.admit, func(c *gin.Context) {
		if c.Param("id") == "first" {
			for _, tc := range []struct {
				path string
				code int
			}{{"same", 429}, {"other", 503}} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest("POST", "/"+tc.path, nil))
				if w.Code != tc.code {
					t.Fatalf("%s: %d", tc.path, w.Code)
				}
			}
			if _, ok := g.users[2]; ok {
				t.Fatal("busy rejection consumed another user's quota")
			}
		}
		c.Status(204)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/first", nil))
	if w.Code != 204 || g.running != 0 || g.users[1].active {
		t.Fatal("active slot leaked")
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/other", nil))
	if w.Code != 204 {
		t.Fatal("completed request did not release slot")
	}
}

func TestAIGateReleasesAfterPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g := newAIGate()
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/", func(c *gin.Context) { c.Set("user", model.User{ID: 1}) }, g.admit, func(c *gin.Context) { panic("test failure") })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || g.running != 0 || g.users[1].active {
		t.Fatal("panic leaked admission slot")
	}
}
