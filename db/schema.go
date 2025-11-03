package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO)
)

// Open opens (or creates) the SQLite DB with pragmatic defaults for web apps.
// Call this once and share the *sql.DB (e.g., via your Application struct).
func Open(path string) (*sql.DB, error) {
	// Ensure parent dir exists (e.g., ./data)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	// Pragmas: foreign keys on, WAL, reasonable sync + busy timeout.
	dsn := path +
		"?_pragma=foreign_keys(ON)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// SQLite is happiest with a very small pool.
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)

	// Quick ping with timeout so startup fails fast if path is bad.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return db, nil
}

// ApplyMigrations runs every *.sql file in dir in lexicographic order.
// Files can contain multiple statements. This is idempotent if your SQL uses IF NOT EXISTS.
func ApplyMigrations(ctx context.Context, db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// If the directory doesn't exist, consider that a configuration error.
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("migrations dir not found: %s", dir)
		}
		return err
	}

	// Collect *.sql files
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".sql" {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("no .sql files found in %s", dir)
	}

	sort.Strings(files)

	// Execute each file in its own transaction.
	for _, f := range files {
		sqlBytes, readErr := os.ReadFile(f)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", f, readErr)
		}

		tx, beginErr := db.BeginTx(ctx, &sql.TxOptions{})
		if beginErr != nil {
			return fmt.Errorf("begin tx for %s: %w", f, beginErr)
		}

		if _, execErr := tx.ExecContext(ctx, string(sqlBytes)); execErr != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec %s: %w", f, execErr)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return fmt.Errorf("commit %s: %w", f, commitErr)
		}
	}

	return nil
}
