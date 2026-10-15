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
}
