package db

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Migrate applies any migrations that haven't been recorded in schema_migrations.
//
// `dir` is a filesystem path to a directory of .sql files. Migrations are applied
// in lexical filename order, each inside its own transaction, and recorded by
// filename. This is deliberately thin — no down-migrations, no checksums.
// If a migration partially fails you restore from backup, not rewind.
//
// Production: /app/migrations (copied by Dockerfile).
// Dev (go run ./cmd/wunest): ./migrations relative to the CWD.
// Override with MIGRATIONS_DIR env var.
func Migrate(ctx context.Context, pg *Postgres, dir string) error {
	if dir == "" {
		if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
			dir = env
		} else if _, err := os.Stat("/app/migrations"); err == nil {
			dir = "/app/migrations"
		} else {
			dir = "./migrations"
		}
	}

	// 1. Ensure tracking table.
	if _, err := pg.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// 2. Enumerate migrations on disk.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("list %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	// 3. Load set of already-applied names.
	applied := make(map[string]struct{})
	rows, err := pg.Query(ctx, `SELECT name FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("select applied: %w", err)
	}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return fmt.Errorf("scan applied: %w", err)
		}
		applied[n] = struct{}{}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate applied: %w", err)
	}

	// 4. Apply the pending ones.
	pending := 0
	for _, name := range names {
		if _, ok := applied[name]; ok {
			continue
		}
		pending++
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		start := time.Now()
		if err := applyOne(ctx, pg, name, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		slog.Info("migration applied", "name", name, "dur", time.Since(start))
	}
	if pending == 0 {
		slog.Info("migrations up to date", "count", len(applied))
	}
	return nil
}

// applyOne runs one migration file in a transaction, then records it.
func applyOne(ctx context.Context, pg *Postgres, name, body string) error {
	tx, err := pg.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck — Rollback after Commit is a no-op.

	if _, err := tx.Exec(ctx, body); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (name) VALUES ($1) ON CONFLICT DO NOTHING`,
		name,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
