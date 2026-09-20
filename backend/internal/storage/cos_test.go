package storage

import (
	"strings"
	"testing"
)

func TestPublicCOSConfig(t *testing.T) {
	for k, v := range map[string]string{"COS_BUCKET": "test-bucket-1234567890", "COS_REGION": "ap-guangzhou", "COS_SECRET_ID": "test-id", "COS_SECRET_KEY": "test-secret", "COS_SESSION_TOKEN": ""} {
		t.Setenv(k, v)
	}
	s := FromEnv()
	if !s.Configured() {
		t.Fatal("valid config rejected")
	}
	key := "4/" + strings.Repeat("a", 32) + ".jpg"
	result, err := s.PublicURL(key)
	if err != nil || result != "https://test-bucket-1234567890.cos.ap-guangzhou.myqcloud.com/mathwiki/avatars/"+key {
		t.Fatal("wrong public URL")
	}
	if strings.Contains(result, "?") || strings.Contains(result, "test-secret") {
		t.Fatal("public URL should not contain credentials or a signature")
	}
	for _, key := range []string{"../secret", "https://evil.example/a.jpg", "1/test.svg", "1/a.jpg?x=y"} {
		if _, err = s.PublicURL(key); err == nil {
			t.Fatal("unsafe object key accepted")
		}
	}
	for _, field := range []string{"COS_BUCKET", "COS_REGION", "COS_SECRET_ID", "COS_SECRET_KEY"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv(field, "")
			if FromEnv().Configured() {
				t.Fatal("incomplete config accepted")
			}
		})
	}
	t.Setenv("COS_REGION", "ap-guangzhou/evil")
	if FromEnv().Configured() {
		t.Fatal("unsafe region accepted")
	}
}
