package models

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
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
			if err := execMigrationStatement(ctx, conn, stmt); err != nil {
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

// addColumnRX matches "ALTER TABLE t ADD COLUMN IF NOT EXISTS c <definition>".
var addColumnRX = regexp.MustCompile(`(?is)^ALTER\s+TABLE\s+` + "`?" + `(\w+)` + "`?" + `\s+ADD\s+COLUMN\s+IF\s+NOT\s+EXISTS\s+` + "`?" + `(\w+)` + "`?" + `\s+(.+)$`)

// createIndexRX matches "CREATE [UNIQUE] INDEX IF NOT EXISTS i ON t (cols)".
var createIndexRX = regexp.MustCompile(`(?is)^CREATE\s+(UNIQUE\s+)?INDEX\s+IF\s+NOT\s+EXISTS\s+` + "`?" + `(\w+)` + "`?" + `\s+ON\s+` + "`?" + `(\w+)` + "`?" + `\s*(\(.+\))$`)

// execMigrationStatement runs one statement. MySQL 8 does not support
// "ADD COLUMN IF NOT EXISTS" or "CREATE INDEX IF NOT EXISTS" (MariaDB does),
// so those forms are emulated with information_schema checks to keep
// migrations portable between both.
func execMigrationStatement(ctx context.Context, conn *sql.Conn, stmt string) error {
	if m := addColumnRX.FindStringSubmatch(stmt); m != nil {
		table, column, definition := m[1], m[2], m[3]
		var exists int
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&exists)
		if err != nil || exists > 0 {
			return err
		}
		_, err = conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, column, definition))
		return err
	}
	if m := createIndexRX.FindStringSubmatch(stmt); m != nil {
		unique, index, table, columns := m[1], m[2], m[3], m[4]
		// Skip when an index with this name already exists.
		var exists int
		err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, table, index).Scan(&exists)
		if err != nil || exists > 0 {
			return err
		}
		_, err = conn.ExecContext(ctx, fmt.Sprintf("CREATE %sINDEX `%s` ON `%s` %s", strings.ToUpper(unique), index, table, columns))
		return err
	}
	_, err := conn.ExecContext(ctx, stmt)
	return err
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
