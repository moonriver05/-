package api

import (
	"net/http/httptest"
	"testing"
)

func TestSpeechOrigin(t *testing.T) {
	for _, item := range []struct {
		origin string
		want   bool
	}{
		{"https://wiki.example", true}, {"http://wiki.example", true}, {"https://evil.example", false}, {"", false}, {"null", false}, {"https://wiki.example.evil.example", false},
	} {
		request := httptest.NewRequest("GET", "https://wiki.example/api/v1/speech/stream", nil)
		request.Header.Set("Origin", item.origin)
		if speechOrigin(request) != item.want {
			t.Fatalf("origin %q", item.origin)
		}
	}
}
