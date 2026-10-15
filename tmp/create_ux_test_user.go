package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/Zetshin/movie-reviews/internal/models"
	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "user:pass@/movies?parseTime=true"
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil { panic(err) }
	defer db.Close()
	_, _ = db.Exec("DELETE FROM users WHERE email = ?", "codex-ux-test@invalid.example")
	if err := (&models.UserModel{DB: db}).Insert("UX Test User", "codex-ux-test@invalid.example", "UxTestPass123!"); err != nil { panic(err) }
	fmt.Println("created")
}
