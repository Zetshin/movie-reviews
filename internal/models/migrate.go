package models

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate applies any embedded SQL migrations that have not run yet and
// returns how many were applied. A MySQL advisory lock makes it safe for
// several replicas to start at the same time.
func Migrate(db *sql.DB) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// GET_LOCK is per connection, so pin one connection for the whole run.
	conn, err := db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('movie_reviews_migrate', 60)`).Scan(&locked); err != nil {
		return 0, err
	}
	if !locked.Valid || locked.Int64 != 1 {
		return 0, fmt.Errorf("could not acquire migration lock")
	}
	defer conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK('movie_reviews_migrate')`)

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(255) PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return 0, err
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return 0, err
	}
	sort.Strings(names)

	applied := 0
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var exists int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return applied, err
		}
		if exists > 0 {
			continue
		}
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return applied, err
		}
		// MySQL DDL commits implicitly, so statements run one by one; each
		// migration is written to be idempotent instead of transactional.
		for _, stmt := range splitSQL(string(body)) {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return applied, fmt.Errorf("migration %s: %w", version, err)
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// splitSQL splits a migration file into statements. It removes full-line
// "--" comments and splits on semicolons that end a line, which is enough
// for the plain DDL used in this project (no procedures or string literals
// containing ";\n").
func splitSQL(body string) []string {
	var cleaned strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteByte('\n')
	}
	var statements []string
	for _, part := range strings.Split(cleaned.String(), ";\n") {
		if stmt := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}
