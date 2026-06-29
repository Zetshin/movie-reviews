package main

import (
	"database/sql"
	"flag"
	"log/slog"
	"net/http"
	"os"

	"github.com/Zetshin/movie-reviews/internal/models"

	_ "github.com/go-sql-driver/mysql"
)

type application struct {
	logger *slog.Logger
	movies *models.MovieModel
}

func main() {
	addr := flag.String("addr", ":4000", "HTTP network address")
	dsn := flag.String("dsn", "user:pass@/movies?parseTime=true", "MySQL data source name")
	flag.Parse() //รับค่าเริ่มต้นจาก user ตอนรันโปรแกรม

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)) //ให้ log เป็นแบบโครงสร้าง พวกนี้สามารถ filter หาได้ง่ายเพราะเป็น key-value
	db, err := openDB(*dsn)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	// We also defer a call to db.Close(), so that the connection pool is closed
	// before the main() function exits.
	defer db.Close()

	app := &application{
		logger: logger,
		movies: &models.MovieModel{DB: db},
	}

	logger.Info("starting server", slog.String("addr", *addr))
	err = http.ListenAndServe(*addr, app.routes())
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
