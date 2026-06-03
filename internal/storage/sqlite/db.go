// Package sqlite implements OTM storage using an embedded SQLite database.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// DB wraps an open SQLite database and implements FlowWriter, TrafficQueryStore, and IdentityStore.
type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path, applies the schema, and returns a DB.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Single writer goroutine is the caller's responsibility; one connection is enough.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.db.Close()
}

// Ping checks that the database is reachable and writable.
func (d *DB) Ping(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, "SELECT 1")
	return err
}
