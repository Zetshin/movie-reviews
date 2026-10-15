package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func validatePasswordResetConfig(cfg config) error {
	host, from := strings.TrimSpace(cfg.smtpHost), strings.TrimSpace(cfg.mailFrom)
	if host == "" && from == "" {
		return nil
	}
	if host == "" || from == "" {
		return fmt.Errorf("SMTP_HOST and MAIL_FROM must be set together")
	}
	if strings.ContainsAny(host, " \r\n/:") {
		return fmt.Errorf("SMTP_HOST must be a bare hostname")
	}
	port, err := strconv.Atoi(cfg.smtpPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("SMTP_PORT must be between 1 and 65535")
	}
	address, err := mail.ParseAddress(from)
	if err != nil || address.Address != from {
		return fmt.Errorf("MAIL_FROM must be a plain email address")
	}
	if (cfg.smtpUsername == "") != (cfg.smtpPassword == "") {
		return fmt.Errorf("SMTP_USERNAME and SMTP_PASSWORD must be set together")
	}
	if cfg.smtpUsername != "" && !cfg.smtpRequireTLS && !isLoopbackHost(host) {
		// net/smtp refuses PLAIN auth without TLS anyway; fail at startup
		// instead of on the first reset request.
		return fmt.Errorf("SMTP_REQUIRE_TLS must be true when SMTP credentials are set")
	}
	base, err := url.Parse(cfg.publicBaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.Fragment != "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an absolute http(s) URL without a query or fragment")
	}
	if cfg.secureCookie && base.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL must use https when SECURE_COOKIE is true")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type passwordResetMailer interface {
	SendPasswordReset(ctx context.Context, to, resetURL string) error
}

type smtpResetMailer struct {
	host, port, username, password, from string
	requireTLS                           bool
	// Overridable in tests.
	dial      func(ctx context.Context, network, addr string) (net.Conn, error)
	tlsConfig *tls.Config
	now       func() time.Time
}

func newPasswordResetMailer(cfg config) passwordResetMailer {
	if strings.TrimSpace(cfg.smtpHost) == "" || strings.TrimSpace(cfg.mailFrom) == "" {
		return nil
	}
	return &smtpResetMailer{
		host:       strings.TrimSpace(cfg.smtpHost),
		port:       cfg.smtpPort,
		username:   cfg.smtpUsername,
		password:   cfg.smtpPassword,
		from:       strings.TrimSpace(cfg.mailFrom),
		requireTLS: cfg.smtpRequireTLS,
	}
}

// SendPasswordReset delivers one reset email. The whole SMTP conversation is
// bounded by ctx and a 30 second deadline, so a stuck server cannot pile up
// goroutines.
func (m *smtpResetMailer) SendPasswordReset(ctx context.Context, to, resetURL string) error {
	// Defence in depth: the address is validated by the handler, but a CR/LF
	// here would allow header injection into the message.
	if strings.ContainsAny(to, "\r\n") {
		return errors.New("invalid recipient address")
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dial := m.dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	}
	conn, err := dial(ctx, "tcp", net.JoinHostPort(m.host, m.port))
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		cfg := m.tlsConfig
		if cfg == nil {
			cfg = &tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}
		}
		if err := client.StartTLS(cfg); err != nil {
			return err
		}
	} else if m.requireTLS {
		return errors.New("SMTP server does not offer STARTTLS")
	}
	if m.username != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return errors.New("SMTP server does not offer AUTH")
		}
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}
	if err := client.Mail(m.from); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(m.buildMessage(to, resetURL)); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// buildMessage renders a bilingual (Thai/English) plain-text email with the
// headers mail providers expect (Date, Message-ID), an RFC 2047 encoded
// subject and quoted-printable UTF-8 body.
func (m *smtpResetMailer) buildMessage(to, resetURL string) []byte {
	now := time.Now
	if m.now != nil {
		now = m.now
	}
	domain := m.from[strings.LastIndex(m.from, "@")+1:]
	var id [12]byte
	_, _ = rand.Read(id[:])

	body := "สวัสดีครับ\r\n\r\n" +
		"เราได้รับคำขอให้รีเซ็ตรหัสผ่านบัญชี MovieClub TH ของคุณ\r\n" +
		"คลิกลิงก์ด้านล่างเพื่อตั้งรหัสผ่านใหม่ (ลิงก์ใช้ได้ 30 นาทีและใช้ได้ครั้งเดียว):\r\n\r\n" +
		resetURL + "\r\n\r\n" +
		"หากคุณไม่ได้ขอรีเซ็ตรหัสผ่าน ไม่ต้องทำอะไร รหัสผ่านเดิมของคุณยังใช้ได้ตามปกติ\r\n\r\n" +
		"----\r\n\r\n" +
		"We received a request to reset your MovieClub TH password.\r\n" +
		"Open the link above to choose a new password. It expires in 30 minutes and can be used once.\r\n" +
		"If you did not request this, you can ignore this email.\r\n\r\n" +
		"MovieClub TH\r\n"

	var qp bytes.Buffer
	enc := quotedprintable.NewWriter(&qp)
	_, _ = enc.Write([]byte(body))
	_ = enc.Close()

	var b bytes.Buffer
	header := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	header("From", (&mail.Address{Name: "MovieClub TH", Address: m.from}).String())
	header("To", (&mail.Address{Address: to}).String())
	header("Subject", mime.BEncoding.Encode("UTF-8", "รีเซ็ตรหัสผ่าน MovieClub TH / Reset your password"))
	header("Date", now().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id[:])+"@"+domain+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `text/plain; charset="UTF-8"`)
	header("Content-Transfer-Encoding", "quoted-printable")
	header("Auto-Submitted", "auto-generated")
	b.WriteString("\r\n")
	b.Write(qp.Bytes())
	return b.Bytes()
}
