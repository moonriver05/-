package mail

import (
	"context"
	"testing"
)

func TestSMTPConfigurationFailsClosed(t *testing.T) {
	valid := SMTP{Host: "smtp.example.com", Port: "465", Username: "sender@example.com", Password: "test-only", From: "sender@example.com", TLSMode: "tls"}
	if !valid.Configured() {
		t.Fatal("valid TLS configuration rejected")
	}
	starttls := valid
	starttls.Port = "587"
	starttls.TLSMode = "starttls"
	if !starttls.Configured() {
		t.Fatal("STARTTLS configuration rejected")
	}
	for _, edit := range []func(*SMTP){func(s *SMTP) { s.Password = "" }, func(s *SMTP) { s.Host = "" }, func(s *SMTP) { s.Port = "0" }, func(s *SMTP) { s.Port = "65536" }, func(s *SMTP) { s.TLSMode = "none" }, func(s *SMTP) { s.From = "sender@example.com\r\nBcc: other@example.com" }} {
		s := valid
		edit(&s)
		if s.Configured() {
			t.Fatal("unsafe configuration accepted")
		}
		if s.SendCode(context.Background(), "recipient@example.com", "123456") == nil {
			t.Fatal("unconfigured send succeeded")
		}
	}
	if valid.SendCode(context.Background(), "victim@example.com\r\nBcc: other@example.com", "123456") == nil {
		t.Fatal("header injection accepted")
	}
	if valid.SendCode(context.Background(), "recipient@example.com", "not-a-code") == nil {
		t.Fatal("invalid code accepted")
	}
}
