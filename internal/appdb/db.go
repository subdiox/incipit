// Package appdb is Incipit's own state database (app.db): users, sessions,
// shelves, reading progress, the CBZ page-list cache and settings. It is kept
// strictly separate from the Calibre metadata.db so the library stays portable
// and desktop-Calibre-compatible.
package appdb

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// Store wraps the app database and exposes all state operations.
type Store struct {
	db *sql.DB
}

// migrations are applied in order; each runs once and is recorded. This is a
// dependency-free migrator — simple and sufficient for app.db's own schema.
var migrations = []string{
	`CREATE TABLE users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
		password_hash TEXT NOT NULL DEFAULT '',
		is_admin      INTEGER NOT NULL DEFAULT 0,
		source        TEXT NOT NULL DEFAULT 'local',
		can_download  INTEGER NOT NULL DEFAULT 1,
		can_upload    INTEGER NOT NULL DEFAULT 0,
		can_edit      INTEGER NOT NULL DEFAULT 0,
		created_at    TEXT NOT NULL
	);
	CREATE TABLE sessions (
		id         TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT NOT NULL
	);
	CREATE INDEX idx_sessions_user ON sessions(user_id);
	CREATE TABLE shelves (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id    INTEGER NOT NULL,
		name       TEXT NOT NULL,
		is_public  INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL,
		UNIQUE(user_id, name)
	);
	CREATE TABLE shelf_books (
		shelf_id INTEGER NOT NULL,
		book_id  INTEGER NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		added_at TEXT NOT NULL,
		PRIMARY KEY(shelf_id, book_id)
	);
	CREATE TABLE read_progress (
		user_id     INTEGER NOT NULL,
		book_id     INTEGER NOT NULL,
		format      TEXT NOT NULL DEFAULT 'CBZ',
		page        INTEGER NOT NULL DEFAULT 0,
		total_pages INTEGER NOT NULL DEFAULT 0,
		updated_at  TEXT NOT NULL,
		PRIMARY KEY(user_id, book_id, format)
	);
	CREATE TABLE page_cache (
		book_id    INTEGER NOT NULL,
		format     TEXT NOT NULL,
		file_path  TEXT NOT NULL,
		pages_json TEXT NOT NULL,
		page_count INTEGER NOT NULL,
		mtime      INTEGER NOT NULL,
		size       INTEGER NOT NULL,
		scanned_at TEXT NOT NULL,
		PRIMARY KEY(book_id, format)
	);
	CREATE TABLE settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
}

// Open opens (creating if needed) the app database at path and runs migrations.
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(1)")
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping app.db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var current int
	if err := s.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return err
	}
	for i := current; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }
