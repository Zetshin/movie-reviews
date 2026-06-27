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

	logger.Info("starting server", slog.String("addr", *addr))
	err := http.ListenAndServe(*addr, app.routes())
	logger.Error(err.Error())
	os.Exit(1)
}
