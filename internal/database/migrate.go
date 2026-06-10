package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func RunMigrations(db *pgxpool.Pool) error {
	ctx := context.Background()

	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	rows, err := db.Query(ctx, "SELECT filename FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	applied := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan applied migration: %w", err)
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate applied migrations: %w", err)
	}
	rows.Close()

	files, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	// Only include up migrations (exclude *.down.sql)
	var upFiles []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".down.sql") {
			upFiles = append(upFiles, f)
		}
	}
	sort.Strings(upFiles)

	for _, file := range upFiles {
		name := filepath.Base(file)
		if applied[name] {
			slog.Debug("migration skipped", "file", name)
			continue
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction for %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", name, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)", name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", name, err)
		}

		slog.Info("migration applied", "file", name)
	}

	return nil
}

func RollbackLastMigration(db *pgxpool.Pool) error {
	ctx := context.Background()

	var filename string
	err := db.QueryRow(ctx,
		"SELECT filename FROM schema_migrations ORDER BY applied_at DESC, filename DESC LIMIT 1",
	).Scan(&filename)
	if err != nil {
		return fmt.Errorf("no migrations to roll back")
	}

	base := strings.TrimSuffix(filename, ".sql")
	downFile := filepath.Join("migrations", base+".down.sql")

	content, err := os.ReadFile(downFile)
	if err != nil {
		return fmt.Errorf("down migration not found for %s: %w", filename, err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("execute down migration %s: %w", filename, err)
	}

	if _, err := tx.Exec(ctx,
		"DELETE FROM schema_migrations WHERE filename = $1", filename); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("remove migration record for %s: %w", filename, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rollback: %w", err)
	}

	slog.Info("migration rolled back", "file", filename)
	return nil
}
