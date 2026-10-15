package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/alexedwards/scs/v2/memstore"
	"github.com/go-playground/form/v4"
)

type recordingMailer struct {
	mu   sync.Mutex
	sent []string
}

func (m *recordingMailer) SendPasswordReset(_ context.Context, to, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, to)
	return nil
}

func newResetTestApp(mailer passwordResetMailer) *application {
	sm := scs.New()
	sm.Store = memstore.New()
	return &application{
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		sessionManager: sm,
		formDecoder:    form.NewDecoder(),
		passwordMailer: mailer,
		resetLimiter:   newRateLimiter(3, time.Hour),
		mailSlots:      make(chan struct{}, 4),
	}
}

func postForgot(app *application, email string) *httptest.ResponseRecorder {
	body := url.Values{"email": {email}}.Encode()
	r := httptest.NewRequest(http.MethodPost, "/user/forgot-password", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	app.sessionManager.LoadAndSave(http.HandlerFunc(app.forgotPasswordPost)).ServeHTTP(rr, r)
	return rr
}

// Without SMTP no token is created (app.users is nil here, so touching the
// database would panic) and the visitor gets the usual neutral answer.
func TestForgotPasswordWithoutMailer(t *testing.T) {
	app := newResetTestApp(nil)
	rr := postForgot(app, "user@example.com")
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/user/login" {
		t.Fatalf("got %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

// Once the per-address limit is reached the request is answered the same
// way without touching the database or sending email.
func TestForgotPasswordPerAddressLimit(t *testing.T) {
	mailer := &recordingMailer{}
	app := newResetTestApp(mailer)
	for i := 0; i < 3; i++ {
		app.resetLimiter.allow("victim@example.com")
	}
	rr := postForgot(app, "Victim@Example.com")
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("got %d", rr.Code)
	}
	if len(mailer.sent) != 0 {
		t.Fatalf("no email expected, got %v", mailer.sent)
	}
}

func TestSendResetEmailCapsConcurrency(t *testing.T) {
	block := make(chan struct{})
	mailer := &blockingMailer{release: block}
	app := newResetTestApp(mailer)
	app.mailSlots = make(chan struct{}, 2)
	for i := 0; i < 5; i++ {
		app.sendResetEmail("user@example.com", "https://x/y")
	}
	close(block)
	app.background.Wait()
	if got := mailer.count(); got != 2 {
		t.Fatalf("expected 2 sends with 2 slots, got %d", got)
	}
}

type blockingMailer struct {
	release chan struct{}
	mu      sync.Mutex
	n       int
}

func (m *blockingMailer) SendPasswordReset(context.Context, string, string) error {
	<-m.release
	m.mu.Lock()
	m.n++
	m.mu.Unlock()
	return nil
}

func (m *blockingMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.n
}
