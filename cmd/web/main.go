package main

import (
	"crypto/tls"
	"database/sql"
	"errors"
	"flag"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time" // New import

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
	"github.com/alexedwards/scs/mysqlstore" // New import
	"github.com/alexedwards/scs/v2"         // New import
	"github.com/go-playground/form/v4"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type application struct {
	logger         *slog.Logger
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
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	err := godotenv.Load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Error("could not load .env", "error", err)
		os.Exit(1)
	}

	apiKey := os.Getenv("TMDB_API_KEY")
	if apiKey == "" {
		logger.Error("TMDB_API_KEY is missing")
		os.Exit(1)
	}
	addr := flag.String("addr", ":4000", "HTTP network address")
	tlsEnabled := flag.Bool("tls", true, "Serve HTTPS directly using ./tls/cert.pem and ./tls/key.pem")
	secureCookie := flag.Bool("secure-cookie", true, "Mark session cookies Secure (disable only for local HTTP testing)")
	importTMDB := flag.Int("import-tmdb", 0, "Import 1-20 now-playing movies from TMDB, then exit without starting the server")
	importPeople := flag.Bool("import-person-details", false, "Fill details for imported TMDB people, then exit without starting the server")
	importTrending := flag.Int("import-trending-people", 0, "Import 1-20 weekly trending people for Home, then exit")
	migrateMetadata := flag.Bool("migrate-catalog-metadata", false, "Add required catalog and review columns, then exit unless syncing")
	syncMetadata := flag.Bool("sync-tmdb-metadata", false, "Refresh cast order and character names for imported movies, then exit")
	dsn := flag.String("dsn", "user:pass@/movies?parseTime=true", "MySQL data source name")
	flag.Parse() //รับค่าเริ่มต้นจาก user ตอนรันโปรแกรม
	if *importTMDB < 0 || *importTMDB > 20 {
		logger.Error("import-tmdb must be between 1 and 20, or 0 to start the server normally")
		os.Exit(1)
	}
	if *importTrending < 0 || *importTrending > 20 {
		logger.Error("import-trending-people must be between 1 and 20, or 0 to skip")
		os.Exit(1)
	}

	//ให้ log เป็นแบบโครงสร้าง พวกนี้สามารถ filter หาได้ง่ายเพราะเป็น key-value
	db, err := openDB(*dsn)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	// We also defer a call to db.Close(), so that the connection pool is closed
	// before the main() function exits.
	defer db.Close()
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
		logger.Error(err.Error())
		os.Exit(1)
	}

	formDecoder := form.NewDecoder()

	sessionManager := scs.New()
	sessionManager.Store = mysqlstore.New(db)
	sessionManager.Lifetime = 12 * time.Hour

	sessionManager.Cookie.Secure = *secureCookie
	app := &application{
		logger:         logger,
		movies:         &models.MovieModel{DB: db},
		users:          &models.UserModel{DB: db},
		templateCache:  templateCache,
		formDecoder:    formDecoder,
		sessionManager: sessionManager,
		reviews:        &models.ReviewModel{DB: db},
		persons:        &models.PersonModel{DB: db},
		moviePersons:   &models.MoviePersonModel{DB: db},
		tmdb:           tmdb.NewClient(apiKey),
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

	tlsConfig := &tls.Config{
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}
	srv := &http.Server{
		Addr:    *addr,
		Handler: app.routes(),
		// Create a *log.Logger from our structured logger handler, which writes
		// log entries at the Error level, and assign it to the ErrorLog field. If
		// you would prefer to log the server errors at Warn level instead, you
		// could pass slog.LevelWarn as the final parameter.
		ErrorLog:     slog.NewLogLogger(logger.Handler(), slog.LevelError),
		TLSConfig:    tlsConfig,
		IdleTimeout:  time.Minute,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	logger.Info("starting server", "addr", srv.Addr)
	if *tlsEnabled {
		err = srv.ListenAndServeTLS("./tls/cert.pem", "./tls/key.pem")
	} else {
		err = srv.ListenAndServe()
	}
	logger.Error(err.Error())
	os.Exit(1)
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
