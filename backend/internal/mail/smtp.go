// Package mail sends registration codes over authenticated, encrypted SMTP only.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Sender interface {
	Configured() bool
	SendCode(context.Context, string, string) error
}
type SMTP struct{ Host, Port, Username, Password, From, TLSMode string }

func FromEnv() *SMTP {
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "465"
	}
	mode := os.Getenv("SMTP_TLS_MODE")
	if mode == "" {
		mode = "tls"
	}
	return &SMTP{Host: strings.TrimSpace(os.Getenv("SMTP_HOST")), Port: port, Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD"), From: strings.TrimSpace(os.Getenv("SMTP_FROM")), TLSMode: mode}
}
func (s *SMTP) Configured() bool {
	from, err := mail.ParseAddress(s.From)
	port, e := strconv.Atoi(s.Port)
	return err == nil && from.Address == s.From && !strings.ContainsAny(s.From, "\r\n") && e == nil && port > 0 && port <= 65535 && s.Host != "" && s.Username != "" && s.Password != "" && (s.TLSMode == "tls" || s.TLSMode == "starttls")
}
func (s *SMTP) SendCode(ctx context.Context, email, code string) error {
	return s.sendCode(ctx, email, code, false)
}
func (s *SMTP) SendResetCode(ctx context.Context, email, code string) error {
	return s.sendCode(ctx, email, code, true)
}
func (s *SMTP) sendCode(ctx context.Context, email, code string, reset bool) error {
	if !s.Configured() {
		return errors.New("SMTP is not configured")
	}
	recipient, err := mail.ParseAddress(email)
	if err != nil || recipient.Address != email || strings.ContainsAny(email, "\r\n") {
		return errors.New("invalid recipient")
	}
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		return errors.New("invalid code")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	if s.TLSMode == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err = secure.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support STARTTLS")
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if err = client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
		return err
	}
	if err = client.Mail(s.From); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := fmt.Sprintf("From: MathWiki LAB <%s>\r\nTo: %s\r\nSubject: MathWiki registration verification code\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\nDate: %s\r\n\r\n你的 MathWiki 注册验证码是：%s\r\n\r\n验证码 10 分钟内有效，仅用于注册，请勿转发给他人。\r\n如果不是你本人操作，请忽略此邮件。\r\n", s.From, email, time.Now().Format(time.RFC1123Z), code)
	if reset {
		message = strings.ReplaceAll(message, "registration verification", "password reset")
		message = strings.ReplaceAll(message, "注册", "重置密码")
	}
	if _, err = writer.Write([]byte(message)); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	// Successful DATA acknowledgement means accepted; a later QUIT failure must not invalidate the code.
	_ = client.Quit()
	return nil
}
