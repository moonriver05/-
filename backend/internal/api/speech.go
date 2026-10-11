package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"mathwiki/internal/speech"
)

func speechOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (origin.Scheme == "https" || origin.Scheme == "http") && origin.Host == r.Host && origin.User == nil
}

func (s *Server) speechRateLimit(c *gin.Context) {
	s.limiter.Lock()
	defer s.limiter.Unlock()
	now := time.Now()
	for key, value := range s.limiter.entries {
		if now.After(value.Until) {
			delete(s.limiter.entries, key)
		}
	}
	key := fmt.Sprintf("speech:%d", userID(c))
	entry := s.limiter.entries[key]
	if entry.Count >= 10 {
		c.Header("Retry-After", strconv.Itoa(int(time.Until(entry.Until).Seconds())+1))
		c.AbortWithStatusJSON(429, gin.H{"error": "语音输入过于频繁，请一分钟后再试"})
		return
	}
	if entry.Count == 0 {
		entry.Until = now.Add(time.Minute)
	}
	entry.Count++
	s.limiter.entries[key] = entry
}

func (s *Server) speechStream(c *gin.Context) {
	if !speechOrigin(c.Request) {
		c.JSON(403, gin.H{"error": "语音请求来源无效"})
		return
	}
	if !s.speech.Configured() {
		c.JSON(503, gin.H{"error": "语音服务尚未配置"})
		return
	}
	// Bound simultaneous streams and memory on the small production server.
	select {
	case s.speechSlots <- struct{}{}:
		defer func() { <-s.speechSlots }()
	default:
		c.JSON(429, gin.H{"error": "语音服务繁忙，请稍后重试"})
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: speechOrigin, ReadBufferSize: 8192, WriteBufferSize: 8192}
	browser, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer browser.Close()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 75*time.Second)
	defer cancel()
	cloud, err := s.speech.Open(ctx, fmt.Sprintf("mathwiki-%d", userID(c)))
	if err != nil {
		_ = browser.WriteJSON(gin.H{"event": "error", "error": err.Error()})
		return
	}
	defer cloud.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		browser.Close()
		cloud.Close()
	}()
	defer close(done)
	browser.SetReadLimit(speech.MaxFrameBytes)
	deadline, _ := ctx.Deadline()
	browser.SetReadDeadline(deadline)
	browser.SetWriteDeadline(deadline)
	_ = browser.WriteJSON(gin.H{"event": "ready"})
	// Only this reader writes to the browser after ready, avoiding concurrent writes.
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		defer cancel()
		for {
			_, frame, err := cloud.ReadMessage()
			if err != nil {
				_ = browser.WriteJSON(gin.H{"event": "error", "error": "语音连接中断或超时，请重试"})
				return
			}
			text, final, err := speech.Parse(frame)
			if err != nil {
				_ = browser.WriteJSON(gin.H{"event": "error", "error": err.Error()})
				return
			}
			if text != "" || final {
				event := "transcript"
				if final {
					event = "done"
				}
				if browser.WriteJSON(gin.H{"event": event, "text": text}) != nil || final {
					return
				}
			}
		}
	}()
	total := 0
	var pending []byte
	for {
		kind, payload, err := browser.ReadMessage()
		if err != nil {
			break
		}
		if kind == websocket.TextMessage && string(payload) == "stop" {
			_ = cloud.WriteMessage(websocket.BinaryMessage, speech.Frame(2, true, pending))
			select {
			case <-finished:
			case <-ctx.Done():
			}
			break
		}
		if kind != websocket.BinaryMessage || len(payload) == 0 || len(payload)%2 != 0 || total+len(payload) > speech.MaxPCMBytes {
			break
		}
		total += len(payload)
		if len(pending) > 0 && cloud.WriteMessage(websocket.BinaryMessage, speech.Frame(2, false, pending)) != nil {
			break
		}
		pending = payload
	}
	cancel()
	browser.Close()
	cloud.Close()
	<-finished
}
