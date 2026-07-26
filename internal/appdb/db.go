// Package appdb is Incipit's own state database (app.db): users, sessions,
// shelves, reading progress, the CBZ page-list cache and settings. It is kept
// strictly separate from the Calibre metadata.db so the library stays portable
// and desktop-Calibre-compatible.
package appdb

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strconv"

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
	// Per-account UI language preference ('en' | 'ja').
	`ALTER TABLE users ADD COLUMN language TEXT NOT NULL DEFAULT 'en';`,
	// Per-account library page-size preference.
	`ALTER TABLE users ADD COLUMN page_size INTEGER NOT NULL DEFAULT 36;`,
	// Aggregate, anonymized per-book view counter (incremented when a book is
	// opened in the reader). Backs the "view count" sort and detail display.
	`CREATE TABLE book_views (
		book_id     INTEGER PRIMARY KEY,
		views       INTEGER NOT NULL DEFAULT 0,
		last_viewed TEXT NOT NULL
	);`,
	// Admin-defined library panes: a saved tag filter (AND) shown as its own nav
	// entry under Library. tag_ids is a CSV of Calibre tag IDs.
	`CREATE TABLE panes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		name       TEXT NOT NULL,
		tag_ids    TEXT NOT NULL DEFAULT '',
		position   INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	);`,
	// A pane can OR its tags ("match any") instead of the default AND
	// ("match all"). 0 = all (AND), 1 = any (OR).
	`ALTER TABLE panes ADD COLUMN match_any INTEGER NOT NULL DEFAULT 0;`,
	// The "pane" concept was renamed to "collection" (user-facing and in code);
	// the table keeps its columns and just changes name.
	`ALTER TABLE panes RENAME TO collections;`,
	// A collection can also EXCLUDE tags (books carrying any of these are hidden);
	// exclude_tag_ids is a CSV of Calibre tag IDs, like tag_ids.
	`ALTER TABLE collections ADD COLUMN exclude_tag_ids TEXT NOT NULL DEFAULT '';`,
	// Every user has a built-in private "Favorites" shelf (is_default=1) that can't
	// be deleted; backfill one for existing users.
	`ALTER TABLE shelves ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;
	 INSERT OR IGNORE INTO shelves (user_id, name, is_public, is_default, created_at)
	   SELECT id, 'Favorite', 0, 1, strftime('%Y-%m-%dT%H:%M:%SZ','now') FROM users;`,
	// A shelf can hold whole series (by Calibre series id), not just individual
	// books — kept as the series identity so it shows as one entry that expands
	// to its volumes.
	`CREATE TABLE shelf_series (
		shelf_id  INTEGER NOT NULL,
		series_id INTEGER NOT NULL,
		position  INTEGER NOT NULL DEFAULT 0,
		added_at  TEXT NOT NULL,
		PRIMARY KEY(shelf_id, series_id)
	);`,
	// Per-user library view preferences (shared across all pages): the sort
	// field, its direction, and whether to group volumes into series tiles.
	`ALTER TABLE users ADD COLUMN sort TEXT NOT NULL DEFAULT 'timestamp';`,
	`ALTER TABLE users ADD COLUMN sort_order TEXT NOT NULL DEFAULT 'desc';`,
	`ALTER TABLE users ADD COLUMN group_series INTEGER NOT NULL DEFAULT 1;`,
	// A collection can pin a fixed sort (and direction): when set, opening it
	// forces this order and the sort control is hidden, so e.g. a "weekly
	// popular" collection always shows in popularity order. Empty = inherit the
	// viewer's own global sort preference (the previous behaviour).
	`ALTER TABLE collections ADD COLUMN sort TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE collections ADD COLUMN sort_order TEXT NOT NULL DEFAULT '';`,
	// Per-account home-page toggles: whether to show the "Recommended for you" and
	// "Continue reading" shelves on the home landing. Both on by default.
	`ALTER TABLE users ADD COLUMN show_recommended INTEGER NOT NULL DEFAULT 1;`,
	`ALTER TABLE users ADD COLUMN show_history INTEGER NOT NULL DEFAULT 1;`,
	// Precomputed per-user recommendations, refreshed hourly by a background job
	// so the endpoint serves instantly instead of scoring the whole library on
	// each request. rank is the 0-based display order; reason_* is the resolved
	// "because you like …" trait.
	`CREATE TABLE rec_cache (
		user_id     INTEGER NOT NULL,
		rank        INTEGER NOT NULL,
		book_id     INTEGER NOT NULL,
		score       REAL NOT NULL DEFAULT 0,
		reason_kind TEXT NOT NULL DEFAULT '',
		reason_name TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, rank)
	);`,
}

// defaultCacheMB / maxConns mirror the metadata.db tuning: SQLite's stock 2 MiB
// page cache is far too small once the database is hundreds of MB, and the cache
// is per-connection so the pool has to be bounded.
const (
	defaultCacheMB = 256
	maxConns       = 8
)

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

// Open opens (creating if needed) the app database at path and runs migrations.
func Open(path string) (*Store, error) {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(1)")
	// app.db grows with the library too (page_cache alone is hundreds of MB on a
	// 300k-book shelf), so it gets the same page cache as metadata.db. Read from
	// the environment directly rather than importing calibre/config: app.db is
	// deliberately independent of both.
	if p := cachePragma(); p != "" {
		q.Add("_pragma", p)
	}
	dsn := "file:" + path + "?" + q.Encode()

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open app.db: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
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
