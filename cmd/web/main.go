package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
)

type application struct {
	logger *slog.Logger
}

func main() {
	addr := flag.String("addr", ":4000", "HTTP network address")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)) //ให้ log เป็นแบบโครงสร้าง พวกนี้สามารถ filter หาได้ง่ายเพราะเป็น key-value
	app := &application{
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", app.home)
	mux.HandleFunc("GET /profile", profile)
	mux.HandleFunc("GET /movies/", moviesShow)
	mux.HandleFunc("GET /movies/{movieID}", movieDetail)             //แสดลงรายละเอียดหนัง มีรีวิวคร่าวๆอยู่ด้วบ
	mux.HandleFunc("GET /movies/{movieID}/review", movieReview)      //แสดงรีวิวทั้งหมด
	mux.HandleFunc("POST /movies/{movieID}/review", movieReviewPost) //ส่งรีวิว

	logger.Info("starting server", slog.String("addr", *addr))

	err := http.ListenAndServe(*addr, mux)
	logger.Error(err.Error())
	os.Exit(1)
}
