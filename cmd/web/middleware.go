package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/justinas/nosurf"
)

func (app *application) commonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' https: blob:; style-src 'self' fonts.googleapis.com; font-src fonts.gstatic.com; "+
				"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "deny")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if app.config.secureCookie {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		app.logger.Info("received request",
			"ip", app.clientIP(r),
			"proto", r.Proto,
			"method", r.Method,
			"uri", r.URL.RequestURI(),
		)
		next.ServeHTTP(w, r)
	})
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if pv := recover(); pv != nil {
				// http.ErrAbortHandler is used to abort a response on purpose.
				if pv == http.ErrAbortHandler {
					panic(pv)
				}
				w.Header().Set("Connection", "close")
				app.serverError(w, r, fmt.Errorf("%v", pv))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !app.isAuthenticated(r) {
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

		isAdmin, err := app.users.IsAdmin(id)
		if err != nil {
			// A session can outlive its user (for example after the account is
			// deleted). Treat that as "not an admin" instead of a server error.
			if errors.Is(err, models.ErrNoRecord) {
				app.clientError(w, http.StatusForbidden)
				return
			}
			app.serverError(w, r, err)
			return
		}

		if !isAdmin {
			app.clientError(w, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) preventCSRF(next http.Handler) http.Handler {
	csrfHandler := nosurf.New(next)
	csrfHandler.SetBaseCookie(http.Cookie{
		HttpOnly: true,
		Path:     "/",
		Secure:   app.config.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
	// nosurf assumes HTTPS by default and then requires a Referer on every
	// POST. When TLS ends at a reverse proxy we rely on X-Forwarded-Proto,
	// which is only trusted when -trust-proxy is set.
	csrfHandler.SetIsTLSFunc(app.isHTTPS)
	return csrfHandler
}

// isHTTPS reports whether the client reached us over HTTPS, either directly
// or through a trusted TLS-terminating reverse proxy.
func (app *application) isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return app.config.trustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP returns the address used for logging and rate limiting. Proxy
// headers are ignored unless the operator has opted in with -trust-proxy,
// otherwise any client could spoof its address.
func (app *application) clientIP(r *http.Request) string {
	if app.config.trustProxy {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			// The proxy appends the address it saw as the last entry.
			parts := strings.Split(forwarded, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Apply this before CSRF middleware can parse an uploaded movie image.
func limitMovieUpload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
		next.ServeHTTP(w, r)
	})
}

// Limit the body before CSRF middleware parses a multipart form.
func limitAvatarUpload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
		next.ServeHTTP(w, r)
	})
}

// limitFormBody caps ordinary (non-upload) form submissions at 1 MB.
func limitFormBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}

// rateLimiter is a small fixed-window limiter keyed by client IP. It protects
// login and signup from password guessing and bcrypt-driven CPU exhaustion.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]*rateWindow
	now     func() time.Time
}

type rateWindow struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, clients: make(map[string]*rateWindow), now: time.Now}
}

// allow records one attempt for key and reports whether it is permitted.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	// Drop expired entries so the map cannot grow without bound.
	if len(l.clients) > 10000 {
		for k, entry := range l.clients {
			if now.After(entry.reset) {
				delete(l.clients, k)
			}
		}
	}
	entry, ok := l.clients[key]
	if !ok || now.After(entry.reset) {
		l.clients[key] = &rateWindow{count: 1, reset: now.Add(l.window)}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	return true
}

func (app *application) rateLimit(limiter *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && !limiter.allow(app.clientIP(r)) {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(limiter.window.Seconds())))
				app.clientError(w, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// neuteredFileSystem serves files but never directory listings, so upload
// folders cannot be enumerated.
type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if s.IsDir() {
		f.Close()
		// http.FileServer maps fs.ErrNotExist to a plain 404.
		return nil, fs.ErrNotExist
	}
	return f, nil
}
