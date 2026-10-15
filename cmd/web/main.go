package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
	"github.com/alexedwards/scs/mysqlstore"
	"github.com/alexedwards/scs/v2"
	"github.com/go-playground/form/v4"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// config holds runtime settings. Every flag can also be set with an
// environment variable so the same binary works locally and in Docker.
type config struct {
	addr         string
	dsn          string
	tls          bool
	tlsCert      string
	tlsKey       string
	secureCookie bool
	trustProxy   bool
	migrate      bool
}

type application struct {
	config         config
	logger         *slog.Logger
	db             *sql.DB
	movies         *models.MovieModel
	users          *models.UserModel
	templateCache  map[string]*template.Template
	formDecoder    *form.Decoder
	sessionManager *scs.SessionManager
	reviews        *models.ReviewModel
	persons        *models.PersonModel
	moviePersons   *models.MoviePersonModel
	tmdb           *tmdb.Client
	heroCache      heroCache
	authLimiter    *rateLimiter
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Error("could not load .env", "error", err)
		os.Exit(1)
	}

	var cfg config
	flag.StringVar(&cfg.addr, "addr", envString("ADDR", ":4000"), "HTTP network address (env ADDR)")
	flag.StringVar(&cfg.dsn, "dsn", envString("DB_DSN", "user:pass@/movies?parseTime=true"), "MySQL data source name (env DB_DSN)")
	flag.BoolVar(&cfg.tls, "tls", envBool("TLS_ENABLED", true), "Serve HTTPS directly using the certificate below (env TLS_ENABLED)")
	flag.StringVar(&cfg.tlsCert, "tls-cert", envString("TLS_CERT_FILE", "./tls/cert.pem"), "TLS certificate file (env TLS_CERT_FILE)")
	flag.StringVar(&cfg.tlsKey, "tls-key", envString("TLS_KEY_FILE", "./tls/key.pem"), "TLS private key file (env TLS_KEY_FILE)")
	flag.BoolVar(&cfg.secureCookie, "secure-cookie", envBool("SECURE_COOKIE", true), "Mark cookies Secure and send HSTS; disable only for local HTTP testing (env SECURE_COOKIE)")
	flag.BoolVar(&cfg.trustProxy, "trust-proxy", envBool("TRUST_PROXY", false), "Trust X-Forwarded-For/Proto from a reverse proxy (env TRUST_PROXY)")
	flag.BoolVar(&cfg.migrate, "migrate", envBool("DB_MIGRATE", false), "Apply database migrations before starting (env DB_MIGRATE)")
	importTMDB := flag.Int("import-tmdb", 0, "Import 1-20 now-playing movies from TMDB, then exit without starting the server")
	importPeople := flag.Bool("import-person-details", false, "Fill details for imported TMDB people, then exit without starting the server")
	importTrending := flag.Int("import-trending-people", 0, "Import 1-20 weekly trending people for Home, then exit")
	migrateMetadata := flag.Bool("migrate-catalog-metadata", false, "Add required catalog and review columns, then exit unless syncing")
	syncMetadata := flag.Bool("sync-tmdb-metadata", false, "Refresh cast order and character names for imported movies, then exit")
	healthcheck := flag.Bool("healthcheck", false, "Probe the local /healthz endpoint and exit 0 when healthy (for Docker HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthcheck(cfg))
	}

	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		logger.Error("TMDB_API_KEY is missing")
		os.Exit(1)
	}
	if *importTMDB < 0 || *importTMDB > 20 {
		logger.Error("import-tmdb must be between 1 and 20, or 0 to start the server normally")
		os.Exit(1)
	}
	if *importTrending < 0 || *importTrending > 20 {
		logger.Error("import-trending-people must be between 1 and 20, or 0 to skip")
		os.Exit(1)
	}

	db, err := openDB(cfg.dsn)
	if err != nil {
		// The DSN contains the password, so never log it.
		logger.Error("could not connect to the database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if cfg.migrate {
		applied, err := models.Migrate(db)
		if err != nil {
			logger.Error("database migration failed", "error", err)
			os.Exit(1)
		}
		logger.Info("database schema ready", "applied", applied)
	}

	if *migrateMetadata {
		if err := models.EnsureCatalogMetadataSchema(db); err != nil {
			logger.Error("catalog migration failed", "error", err)
			os.Exit(1)
		}
		logger.Info("catalog metadata schema ready")
		if !*syncMetadata {
			return
		}
	}

	templateCache, err := newTemplateCache()
	if err != nil {
		logger.Error("could not parse templates", "error", err)
		os.Exit(1)
	}

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.Cookie.Secure = cfg.secureCookie
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode

	app := &application{
		config:         cfg,
		logger:         logger,
		db:             db,
		movies:         &models.MovieModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    form.NewDecoder(),
		sessionManager: sessionManager,
		reviews:        &models.ReviewModel{DB: db},
		persons:        &models.PersonModel{DB: db},
		moviePersons:   &models.MoviePersonModel{DB: db},
		tmdb:           tmdb.NewClient(apiKey),
		authLimiter:    newRateLimiter(10, time.Minute),
	}

	if *syncMetadata {
		if err := app.syncTMDBMetadata(); err != nil {
			logger.Error("catalog metadata sync failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if *importTMDB > 0 {
		if err := app.importNowPlayingMovies(*importTMDB); err != nil {
			logger.Error("TMDB starter import failed", "error", err)
			os.Exit(1)
		}
		if !*importPeople && *importTrending == 0 {
			return
		}
	}
	if *importPeople {
		if err := app.importPersonDetails(); err != nil {
			logger.Error("TMDB person import failed", "error", err)
			os.Exit(1)
		}
		if *importTrending == 0 {
			return
		}
	}
	if *importTrending > 0 {
		if err := app.importTrendingPeople(*importTrending); err != nil {
			logger.Error("TMDB trending people import failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := app.serve(); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// serve runs the HTTP server until SIGINT/SIGTERM, then drains in-flight
// requests so container restarts and deploys do not cut users off.
func (app *application) serve() error {
	srv := &http.Server{
		Addr:     app.config.addr,
		Handler:  app.routes(),
		ErrorLog: slog.NewLogLogger(app.logger.Handler(), slog.LevelError),
		TLSConfig: &tls.Config{
			MinVersion:       tls.VersionTLS12,
			CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
		},
		IdleTimeout:       time.Minute,
		ReadHeaderTimeout: 5 * time.Second,
		// Uploads up to 12 MB need more than a few seconds on slow links.
		ReadTimeout: 30 * time.Second,
		// Admin TMDB imports wait up to 7 seconds per upstream call.
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	shutdownErr := make(chan error, 1)
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		s := <-quit
		app.logger.Info("shutting down server", "signal", s.String())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		shutdownErr <- srv.Shutdown(ctx)
	}()

	app.logger.Info("starting server", "addr", srv.Addr, "tls", app.config.tls)
	var err error
	if app.config.tls {
		err = srv.ListenAndServeTLS(app.config.tlsCert, app.config.tlsKey)
	} else {
		err = srv.ListenAndServe()
	}
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if err := <-shutdownErr; err != nil {
		return err
	}
	app.logger.Info("server stopped cleanly")
	return nil
}

// runHealthcheck lets the distroless image (which has no curl or shell)
// check itself: the binary calls its own /healthz endpoint.
func runHealthcheck(cfg config) int {
	scheme := "http"
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.tls {
		scheme = "https"
		// Local loopback probe of our own certificate, which may be self-signed.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	host := cfg.addr
	if len(host) > 0 && host[0] == ':' {
		host = "127.0.0.1" + host
	}
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}
	resp, err := client.Get(fmt.Sprintf("%s://%s/healthz", scheme, host))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck failed:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck failed: status", resp.StatusCode)
		return 1
	}
	return 0
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(time.Hour)

	// MySQL can take a while to accept connections when containers start
	// together, so retry for up to a minute before giving up.
	var pingErr error
	for attempt := 0; attempt < 30; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pingErr = db.PingContext(ctx)
		cancel()
		if pingErr == nil {
			return db, nil
		}
		time.Sleep(2 * time.Second)
	}
	db.Close()
	return nil, pingErr
}
