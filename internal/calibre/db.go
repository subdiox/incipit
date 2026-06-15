package calibre

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Schema returns the embedded Calibre schema DDL. Exposed for test fixtures.
func Schema() string { return schemaSQL }

// openDB opens metadata.db with the custom SQL functions registered and sane
// pragmas. When readOnly is true the database is opened in read-only mode and
// WAL is not forced (so it works on a read-only mount).
func openDB(path string, readOnly bool) (*sql.DB, error) {
	registerSQLFunctions()

	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(0)") // Calibre does not rely on FK enforcement
	if readOnly {
		q.Set("mode", "ro")
	}
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open metadata.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping metadata.db: %w", err)
	}
	if !readOnly {
		// WAL lets readers proceed during the single serialized writer.
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable WAL: %w", err)
		}
	}
	return db, nil
}

// EnsureLibrary makes sure libraryPath exists and contains a metadata.db with
// the Calibre schema. A fresh database also gets a library_id row, mirroring
// what Calibre writes when it creates a new library. Returns true if a new
// database was created.
func EnsureLibrary(libraryPath string) (created bool, err error) {
	if err := os.MkdirAll(libraryPath, 0o755); err != nil {
		return false, fmt.Errorf("create library dir: %w", err)
	}
	dbPath := filepath.Join(libraryPath, "metadata.db")
	if _, statErr := os.Stat(dbPath); statErr == nil {
		return false, nil
	}

	db, err := openDB(dbPath, false)
	if err != nil {
		return false, err
	}
	defer db.Close()

	if _, err := db.Exec(schemaSQL); err != nil {
		return false, fmt.Errorf("apply schema: %w", err)
	}
	if _, err := db.Exec("INSERT INTO library_id (uuid) VALUES (?)", UUID4()); err != nil {
		return false, fmt.Errorf("seed library_id: %w", err)
	}
	return true, nil
}

// applySchema runs the embedded schema against an already-open database. Used
// by tests to build fixtures.
func applySchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schemaSQL)
	return err
}
