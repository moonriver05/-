package api

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type aiUsage struct {
	until  time.Time
	count  int
	active bool
}

// One gate covers both explanation endpoints for this API process. Admission is
// non-blocking: a full service rejects before retrieval/model calls incur cost.
// This is a single-instance guard; a multi-instance deployment needs shared quotas.
type aiGate struct {
	sync.Mutex
	users                          map[uint]aiUsage
	running, concurrent, perMinute int
	now                            func() time.Time
}

func boundedAISetting(name string, fallback, maximum int) int {
	n, err := strconv.Atoi(os.Getenv(name))
	if err != nil || n < 1 || n > maximum {
		return fallback
	}
	return n
}

func newAIGate() *aiGate {
	return &aiGate{users: make(map[uint]aiUsage), concurrent: boundedAISetting("AI_MAX_CONCURRENT", 2, 8), perMinute: boundedAISetting("AI_REQUESTS_PER_MINUTE", 6, 60), now: time.Now}
}

func (g *aiGate) admit(c *gin.Context) {
	id := userID(c)
	if id == 0 {
		c.AbortWithStatusJSON(401, gin.H{"error": "请先登录账户"})
		return
	}
	g.Lock()
	now := g.now()
	for id, usage := range g.users {
		if !usage.active && !now.Before(usage.until) {
			delete(g.users, id)
		}
	}
	usage := g.users[id]
	status, message, retry := 0, "", 2
	switch {
	case usage.active:
		status, message = 429, "当前账户已有回答正在生成，请等待或停止后再试"
	case now.Before(usage.until) && usage.count >= g.perMinute:
		status, message = 429, "提问过于频繁，请稍后再试"
		retry = int(usage.until.Sub(now).Seconds()) + 1
	case g.running >= g.concurrent || len(g.users) >= 10000:
		status, message = 503, "AI 服务正忙，请稍后再试"
	}
	if status != 0 {
		g.Unlock()
		c.Header("Retry-After", strconv.Itoa(retry))
		c.AbortWithStatusJSON(status, gin.H{"error": message})
		return
	}
	if !now.Before(usage.until) {
		usage.count = 0
		usage.until = now.Add(time.Minute)
	}
	usage.count++
	usage.active = true
	g.users[id] = usage
	g.running++
	g.Unlock()
	// Release on success, errors, client cancellation, and handler panics alike.
	defer func() {
		g.Lock()
		usage := g.users[id]
		usage.active = false
		g.users[id] = usage
		g.running--
		g.Unlock()
	}()
	c.Next()
}
