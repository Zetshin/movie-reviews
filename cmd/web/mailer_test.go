package main

import (
	"bufio"
	"context"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// fakeSMTP is a minimal SMTP server for one connection. It records the
// envelope and message and can advertise STARTTLS without supporting it.
type fakeSMTP struct {
	ln         net.Listener
	offerTLS   bool
	from, rcpt string
	data       string
	done       chan struct{}
	sawQuit    bool
}

func newFakeSMTP(t *testing.T, offerTLS bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln, offerTLS: offerTLS, done: make(chan struct{})}
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *fakeSMTP) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	r := bufio.NewReader(conn)
	write := func(line string) { io.WriteString(conn, line+"\r\n") }
	write("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			if s.offerTLS {
				write("250-fake")
				write("250 STARTTLS")
			} else {
				write("250 fake")
			}
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			s.from = strings.TrimSpace(line[len("MAIL FROM:"):])
			write("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			s.rcpt = strings.TrimSpace(line[len("RCPT TO:"):])
			write("250 ok")
		case cmd == "DATA":
			write("354 go ahead")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.data = b.String()
			write("250 queued")
		case cmd == "QUIT":
			s.sawQuit = true
			write("221 bye")
			return
		default:
			write("502 not implemented")
		}
	}
}

func (s *fakeSMTP) mailer(requireTLS bool) *smtpResetMailer {
	host, port, _ := net.SplitHostPort(s.ln.Addr().String())
	return &smtpResetMailer{host: host, port: port, from: "no-reply@movieclubth.com", requireTLS: requireTLS,
		now: func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }}
}

func TestSendPasswordResetDeliversMessage(t *testing.T) {
	srv := newFakeSMTP(t, false)
	link := "https://movieclubth.com/user/reset-password?token=abc_DEF-123"
	if err := srv.mailer(false).SendPasswordReset(context.Background(), "User@Example.com", link); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-srv.done
	if srv.from != "<no-reply@movieclubth.com>" || srv.rcpt != "<User@Example.com>" {
		t.Fatalf("envelope from=%q rcpt=%q", srv.from, srv.rcpt)
	}
	if !srv.sawQuit {
		t.Error("client should end the session with QUIT")
	}

	msg, err := mail.ReadMessage(strings.NewReader(srv.data))
	if err != nil {
		t.Fatalf("message does not parse: %v", err)
	}
	for _, h := range []string{"From", "To", "Subject", "Date", "Message-ID", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"} {
		if msg.Header.Get(h) == "" {
			t.Errorf("missing header %s", h)
		}
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || !strings.Contains(subject, "MovieClub TH") || !strings.Contains(subject, "รีเซ็ตรหัสผ่าน") {
		t.Errorf("subject %q (%v)", subject, err)
	}
	if _, err := msg.Header.Date(); err != nil {
		t.Errorf("Date header: %v", err)
	}
	if !strings.HasSuffix(msg.Header.Get("Message-ID"), "@movieclubth.com>") {
		t.Errorf("Message-ID %q", msg.Header.Get("Message-ID"))
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	// The link must survive quoted-printable encoding intact.
	if !strings.Contains(string(body), link) {
		t.Errorf("body does not contain the reset link:\n%s", body)
	}
	if !strings.Contains(string(body), "30 นาที") {
		t.Error("body should explain the expiry in Thai")
	}
}

func TestSendPasswordResetRequiresTLS(t *testing.T) {
	srv := newFakeSMTP(t, false)
	err := srv.mailer(true).SendPasswordReset(context.Background(), "user@example.com", "https://x/y")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS error, got %v", err)
	}
}

func TestSendPasswordResetRejectsHeaderInjection(t *testing.T) {
	m := &smtpResetMailer{host: "127.0.0.1", port: "1", from: "no-reply@movieclubth.com"}
	for _, to := range []string{"a@example.com\r\nBcc: victim@example.com", "not an address"} {
		if err := m.SendPasswordReset(context.Background(), to, "https://x/y"); err == nil {
			t.Errorf("recipient %q should be rejected before connecting", to)
		}
	}
}

func TestSendPasswordResetHonoursContext(t *testing.T) {
	m := &smtpResetMailer{host: "127.0.0.1", port: "1", from: "no-reply@movieclubth.com",
		dial: func(ctx context.Context, _, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := m.SendPasswordReset(ctx, "user@example.com", "https://x/y"); err == nil {
		t.Fatal("expected a timeout error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("send did not stop when the context ended")
	}
}

func TestValidatePasswordResetConfigTLS(t *testing.T) {
	cfg := config{publicBaseURL: "https://movieclubth.com", secureCookie: true, smtpHost: "mail.movieclubth.com", smtpPort: "587",
		smtpUsername: "no-reply@movieclubth.com", smtpPassword: "x", mailFrom: "no-reply@movieclubth.com", smtpRequireTLS: false}
	if err := validatePasswordResetConfig(cfg); err == nil {
		t.Error("credentials without TLS must be rejected")
	}
	cfg.smtpRequireTLS = true
	if err := validatePasswordResetConfig(cfg); err != nil {
		t.Errorf("production config rejected: %v", err)
	}
	cfg.smtpHost = "mail.movieclubth.com:587"
	if err := validatePasswordResetConfig(cfg); err == nil {
		t.Error("host with port must be rejected")
	}
}
