package main

import (
	"crypto/tls"
	"fmt"
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
	base, err := url.Parse(cfg.publicBaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.RawQuery != "" || base.Fragment != "" {
		return fmt.Errorf("PUBLIC_BASE_URL must be an absolute http(s) URL without a query or fragment")
	}
	if cfg.secureCookie && base.Scheme != "https" {
		return fmt.Errorf("PUBLIC_BASE_URL must use https when SECURE_COOKIE is true")
	}
	return nil
}

type passwordResetMailer interface {
	SendPasswordReset(to, resetURL string) error
}

type smtpResetMailer struct {
	host, port, username, password, from string
	requireTLS                           bool
}

func (m smtpResetMailer) SendPasswordReset(to, resetURL string) error {
	addr := net.JoinHostPort(m.host, m.port)
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		conn.Close()
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	} else if m.requireTLS {
		return fmt.Errorf("SMTP server does not offer STARTTLS")
	}
	if m.username != "" {
		if err := client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return err
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
	subject := "Reset your MovieClub TH password"
	body := "We received a request to reset your password.\r\n\r\n" + resetURL + "\r\n\r\nThis link expires in 30 minutes. If you did not request it, you can ignore this email.\r\n"
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", m.from, to, subject, body)
	if _, err := w.Write([]byte(message)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func newPasswordResetMailer(cfg config) passwordResetMailer {
	if strings.TrimSpace(cfg.smtpHost) == "" || strings.TrimSpace(cfg.mailFrom) == "" {
		return nil
	}
	return smtpResetMailer{host: cfg.smtpHost, port: cfg.smtpPort, username: cfg.smtpUsername, password: cfg.smtpPassword, from: cfg.mailFrom, requireTLS: cfg.smtpRequireTLS}
}
