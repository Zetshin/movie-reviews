package models

import (
	"io/fs"
	"strings"
	"testing"
)

func TestSplitSQL(t *testing.T) {
	got := splitSQL("-- comment\nCREATE TABLE a (\n  id INT -- trailing\n);\r\n\nCREATE TABLE b (id INT);\n")
	if len(got) != 2 {
		t.Fatalf("got %d statements: %q", len(got), got)
	}
	if !strings.HasPrefix(got[0], "CREATE TABLE a") || got[1] != "CREATE TABLE b (id INT)" {
		t.Fatalf("unexpected statements: %q", got)
	}
}

func TestAddColumnRX(t *testing.T) {
	m := addColumnRX.FindStringSubmatch("ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE")
	if m == nil || m[1] != "users" || m[2] != "is_admin" || m[3] != "BOOLEAN NOT NULL DEFAULT FALSE" {
		t.Fatalf("unexpected match: %q", m)
	}
	if addColumnRX.MatchString("ALTER TABLE users ADD COLUMN is_admin BOOLEAN") {
		t.Fatal("plain ADD COLUMN must run unchanged")
	}

	m = createIndexRX.FindStringSubmatch("CREATE UNIQUE INDEX IF NOT EXISTS movies_uc_tmdb_id ON movies (tmdb_id)")
	if m == nil || m[1] == "" || m[2] != "movies_uc_tmdb_id" || m[3] != "movies" || m[4] != "(tmdb_id)" {
		t.Fatalf("unexpected index match: %q", m)
	}
	if createIndexRX.MatchString("CREATE INDEX i ON t (c)") {
		t.Fatal("plain CREATE INDEX must run unchanged")
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil || len(names) == 0 {
		t.Fatalf("no embedded migrations: %v", err)
	}
	body, err := migrationFiles.ReadFile(names[0])
	if err != nil {
		t.Fatal(err)
	}
	statements := splitSQL(string(body))
	// Every table the models query must be created by the initial migration.
	for _, table := range []string{"users", "movies", "persons", "movie_persons", "reviews", "review_likes", "sessions"} {
		found := false
		for _, stmt := range statements {
			if strings.HasPrefix(stmt, "CREATE TABLE IF NOT EXISTS "+table+" ") {
				found = true
			}
		}
		if !found {
			t.Errorf("table %s is not created", table)
		}
	}
	resetMigration, err := migrationFiles.ReadFile("migrations/0003_password_resets.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resetMigration), "CREATE TABLE IF NOT EXISTS password_reset_tokens") || !strings.Contains(string(resetMigration), "auth_version") {
		t.Fatal("password reset migration is incomplete")
	}
	reviewMigration, err := migrationFiles.ReadFile("migrations/0004_unique_reviews.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reviewMigration), "reviews_movie_user_uc") || !strings.Contains(string(reviewMigration), "DELETE older") {
		t.Fatal("unique review migration is incomplete")
	}
}
