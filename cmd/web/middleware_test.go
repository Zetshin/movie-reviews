package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestApp(cfg config) *application {
	return &application{config: cfg, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestCommonHeaders(t *testing.T) {
	for _, secure := range []bool{false, true} {
		app := newTestApp(config{secureCookie: secure})
		rr := httptest.NewRecorder()
		app.commonHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).
			ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
			if rr.Header().Get(header) == "" {
				t.Errorf("secure=%v: missing %s", secure, header)
			}
		}
		if got := rr.Header().Get("Strict-Transport-Security") != ""; got != secure {
			t.Errorf("secure=%v: HSTS present = %v", secure, got)
		}
		if rr.Header().Get("Server") != "" {
			t.Errorf("Server header should not be set")
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		trustProxy bool
		forwarded  string
		realIP     string
		want       string
	}{
		{"direct", false, "", "", "192.0.2.1"},
		{"spoofed header ignored", false, "203.0.113.9", "203.0.113.8", "192.0.2.1"},
		{"trusted proxy", true, "203.0.113.9", "", "203.0.113.9"},
		{"last hop wins", true, "10.0.0.1, 203.0.113.9", "", "203.0.113.9"},
		{"x-real-ip preferred", true, "6.6.6.6, 203.0.113.9", "198.51.100.7", "198.51.100.7"},
		{"invalid x-real-ip ignored", true, "203.0.113.9", "not-an-ip", "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp(config{trustProxy: tt.trustProxy})
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "192.0.2.1:5555"
			if tt.forwarded != "" {
				r.Header.Set("X-Forwarded-For", tt.forwarded)
			}
			if tt.realIP != "" {
				r.Header.Set("X-Real-IP", tt.realIP)
			}
			if got := app.clientIP(r); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsHTTPS(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if newTestApp(config{}).isHTTPS(r) {
		t.Error("X-Forwarded-Proto must be ignored without -trust-proxy")
	}
	if !newTestApp(config{trustProxy: true}).isHTTPS(r) {
		t.Error("X-Forwarded-Proto should be honoured with -trust-proxy")
	}
}

func TestRateLimiter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(3, time.Minute)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !limiter.allow("a") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if limiter.allow("a") {
		t.Fatal("4th attempt within the window should be blocked")
	}
	if !limiter.allow("b") {
		t.Fatal("other clients must not share a bucket")
	}
	now = now.Add(time.Minute + time.Second)
	if !limiter.allow("a") {
		t.Fatal("window should reset")
	}
	// Expired clients are swept so the map does not grow forever.
	now = now.Add(3 * time.Minute)
	limiter.allow("c")
	if _, ok := limiter.clients["b"]; ok {
		t.Fatal("expired entries should be removed")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	app := newTestApp(config{})
	limiter := newRateLimiter(1, time.Minute)
	handler := app.rateLimit(limiter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	do := func(method string) int {
		r := httptest.NewRequest(method, "/user/login", nil)
		r.RemoteAddr = "192.0.2.1:1"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, r)
		return rr.Code
	}
	if code := do(http.MethodPost); code != http.StatusOK {
		t.Fatalf("first POST = %d", code)
	}
	if code := do(http.MethodPost); code != http.StatusTooManyRequests {
		t.Fatalf("second POST = %d, want 429", code)
	}
	if code := do(http.MethodGet); code != http.StatusOK {
		t.Fatalf("GET must not be rate limited, got %d", code)
	}
}

func TestStaticDirectoryListingDisabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uploads", "a.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := http.FileServer(neuteredFileSystem{http.Dir(dir)})

	for path, want := range map[string]int{
		"/uploads/":      http.StatusNotFound,
		"/uploads":       http.StatusNotFound,
		"/uploads/a.png": http.StatusOK,
		"/missing.png":   http.StatusNotFound,
	} {
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != want {
			t.Errorf("%s: status %d, want %d", path, rr.Code, want)
		}
	}
}

func TestHealthzWithoutDatabase(t *testing.T) {
	app := newTestApp(config{})
	rr := httptest.NewRecorder()
	app.healthz(rr, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
}

func TestRedirectLegacyPersonRoute(t *testing.T) {
	rr := httptest.NewRecorder()
	redirectLegacyPersonRoute(rr, httptest.NewRequest(http.MethodGet, "/persons/12?from_movie=3", nil))
	if rr.Code != http.StatusPermanentRedirect || rr.Header().Get("Location") != "/actors/12?from_movie=3" {
		t.Fatalf("got %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestValidCatalogImage(t *testing.T) {
	for value, want := range map[string]bool{
		"":                                 true,
		"/static/uploads/movies/a.png":     true,
		"https://image.tmdb.org/t/p/a.jpg": true,
		"http://example.com/a.jpg":         true,
		"javascript:alert(1)":              false,
		"data:image/png;base64,AAAA":       false,
		"//evil.example/a.png":             false,
		"https://user:pw@example.com/a":    false,
		"/a\r\nSet-Cookie: x":              false,
	} {
		if got := validCatalogImage(value); got != want {
			t.Errorf("validCatalogImage(%q) = %v, want %v", value, got, want)
		}
	}
}
