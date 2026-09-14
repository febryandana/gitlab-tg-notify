// Package store persists the mapping between a GitLab issue and the
// Telegram message that first announced it (PRD §9).
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/febryandana/gitlab-tg-notify/migrations"
)

// Open opens (creating if needed) the SQLite database at path and applies
// every migration in migrations.FS, in filename order.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %q: %w", path, err)
	}

	// SQLite only usefully supports one writer at a time; a single
	// connection avoids "database is locked" errors under modernc's driver.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database %q: %w", path, err)
	}

	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func runMigrations(db *sql.DB) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}
		if _, err := db.Exec(string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %q: %w", name, err)
		}
	}

	return nil
}
