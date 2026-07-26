package calibre

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Schema returns the embedded Calibre schema DDL. Exposed for test fixtures.
func Schema() string { return schemaSQL }

// defaultCacheMB is SQLite's per-connection page cache, in MiB. SQLite's own
// default is 2 MiB, which on a large library (a 600 MB metadata.db) means every
// listing re-reads its b-tree pages from the filesystem — brutal when the
// library sits on a compressed/large-record filesystem (ZFS with lz4 and a 128K
// recordsize turns each 4K page read into a 128K decompress). Holding the hot
// index pages in-process removes those reads entirely.
const defaultCacheMB = 256

// maxConns bounds the connection pool. The page cache is per-connection, so an
// unbounded pool would multiply cacheMB by however many concurrent queries the
// server happens to run.
const maxConns = 8

// cachePragma returns the cache_size pragma (negative = KiB) for
// INCIPIT_DB_CACHE_MB, or "" when set to 0 (keep SQLite's default).
func cachePragma() string {
	mb := defaultCacheMB
	if v := os.Getenv("INCIPIT_DB_CACHE_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			mb = n
		}
	}
	if mb == 0 {
		return ""
	}
	return "cache_size(-" + strconv.Itoa(mb*1024) + ")"
}

// openDB opens metadata.db with the custom SQL functions registered and sane
// pragmas. When readOnly is true the database is opened in read-only mode and
// WAL is not forced (so it works on a read-only mount).
func openDB(path string, readOnly bool) (*sql.DB, error) {
	registerSQLFunctions()

	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(0)") // Calibre does not rely on FK enforcement
	if p := cachePragma(); p != "" {
		q.Add("_pragma", p)
	}
	if readOnly {
		q.Set("mode", "ro")
	}
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open metadata.db: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
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
