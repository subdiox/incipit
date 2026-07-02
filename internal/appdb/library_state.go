package appdb

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

// --- Shelves ---

// CreateShelf creates a new shelf for a user.
func (s *Store) CreateShelf(ctx context.Context, sh Shelf) (*Shelf, error) {
	sh.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `INSERT INTO shelves (user_id, name, is_public, created_at)
		VALUES (?, ?, ?, ?)`, sh.UserID, sh.Name, b2i(sh.IsPublic), sh.CreatedAt.Format(timeLayout))
	if err != nil {
		return nil, err
	}
	sh.ID, _ = res.LastInsertId()
	return &sh, nil
}

// ListShelves returns shelves visible to a user: their own plus public ones.
func (s *Store) ListShelves(ctx context.Context, userID int64) ([]Shelf, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sh.id, sh.user_id, sh.name, sh.is_public, sh.created_at,
		(SELECT COUNT(*) FROM shelf_books sb WHERE sb.shelf_id=sh.id)
		FROM shelves sh WHERE sh.user_id=? OR sh.is_public=1 ORDER BY sh.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Shelf
	for rows.Next() {
		var sh Shelf
		var pub int
		var created string
		if err := rows.Scan(&sh.ID, &sh.UserID, &sh.Name, &pub, &created, &sh.BookCount); err != nil {
			return nil, err
		}
		sh.IsPublic = pub != 0
		sh.CreatedAt, _ = time.Parse(timeLayout, created)
		out = append(out, sh)
	}
	return out, rows.Err()
}

// GetShelf returns a shelf by ID.
func (s *Store) GetShelf(ctx context.Context, id int64) (*Shelf, error) {
	var sh Shelf
	var pub int
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, name, is_public, created_at,
		(SELECT COUNT(*) FROM shelf_books sb WHERE sb.shelf_id=shelves.id)
		FROM shelves WHERE id=?`, id).Scan(&sh.ID, &sh.UserID, &sh.Name, &pub, &created, &sh.BookCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sh.IsPublic = pub != 0
	sh.CreatedAt, _ = time.Parse(timeLayout, created)
	return &sh, nil
}

// DeleteShelf removes a shelf and its membership rows.
func (s *Store) DeleteShelf(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM shelf_books WHERE shelf_id=?", id); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM shelves WHERE id=?", id); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// AddBookToShelf adds a Calibre book id to a shelf (idempotent).
func (s *Store) AddBookToShelf(ctx context.Context, shelfID, bookID int64) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO shelf_books (shelf_id, book_id, position, added_at)
		VALUES (?, ?, (SELECT COALESCE(MAX(position), 0)+1 FROM shelf_books WHERE shelf_id=?), ?)`,
		shelfID, bookID, shelfID, time.Now().UTC().Format(timeLayout))
	return err
}

// RemoveBookFromShelf removes a book from a shelf.
func (s *Store) RemoveBookFromShelf(ctx context.Context, shelfID, bookID int64) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM shelf_books WHERE shelf_id=? AND book_id=?", shelfID, bookID)
	return err
}

// ShelfBookIDs returns the ordered Calibre book ids on a shelf.
func (s *Store) ShelfBookIDs(ctx context.Context, shelfID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT book_id FROM shelf_books WHERE shelf_id=? ORDER BY position", shelfID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// --- Reading progress ---

// SetProgress upserts a user's reading position.
func (s *Store) SetProgress(ctx context.Context, p Progress) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO read_progress
		(user_id, book_id, format, page, total_pages, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, book_id, format) DO UPDATE SET
			page=excluded.page, total_pages=excluded.total_pages, updated_at=excluded.updated_at`,
		p.UserID, p.BookID, p.Format, p.Page, p.TotalPages, time.Now().UTC().Format(timeLayout))
	return err
}

// GetProgress returns a user's progress for a book/format, or ErrNotFound.
func (s *Store) GetProgress(ctx context.Context, userID, bookID int64, format string) (*Progress, error) {
	var p Progress
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT user_id, book_id, format, page, total_pages, updated_at
		FROM read_progress WHERE user_id=? AND book_id=? AND format=?`, userID, bookID, format).
		Scan(&p.UserID, &p.BookID, &p.Format, &p.Page, &p.TotalPages, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.UpdatedAt, _ = time.Parse(timeLayout, updated)
	return &p, nil
}

// Reading-list filters for ListReading.
const (
	ReadingAll        = "all"         // every entry
	ReadingInProgress = "in-progress" // started but not finished ("continue reading")
	ReadingFinished   = "finished"    // read to the last page
)

// ListReading returns a user's reading entries, most recently read first,
// filtered by status (ReadingAll / ReadingInProgress / ReadingFinished). A book
// is "finished" once its position reaches the last page; "in progress" covers
// unknown-length or not-yet-final positions.
func (s *Store) ListReading(ctx context.Context, userID int64, status string, limit int) ([]Progress, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT user_id, book_id, format, page, total_pages, updated_at
		FROM read_progress WHERE user_id=?`
	switch status {
	case ReadingInProgress:
		// Not finished: unknown length, or not yet at the final page.
		q += ` AND (total_pages=0 OR page < total_pages-1)`
	case ReadingFinished:
		// At/after the last page (requires a known length).
		q += ` AND (total_pages>0 AND page >= total_pages-1)`
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Progress
	for rows.Next() {
		var p Progress
		var updated string
		if err := rows.Scan(&p.UserID, &p.BookID, &p.Format, &p.Page, &p.TotalPages, &updated); err != nil {
			return nil, err
		}
		p.UpdatedAt, _ = time.Parse(timeLayout, updated)
		out = append(out, p)
	}
	return out, rows.Err()
}

// IncrementBookViews bumps a book's aggregate view counter (library-wide,
// anonymized — no per-user record) and returns the new total. Called when a book
// is opened in the reader.
func (s *Store) IncrementBookViews(ctx context.Context, bookID int64) (int64, error) {
	now := time.Now().UTC().Format(timeLayout)
	if _, err := s.db.ExecContext(ctx, `INSERT INTO book_views (book_id, views, last_viewed)
		VALUES (?, 1, ?)
		ON CONFLICT(book_id) DO UPDATE SET views = views + 1, last_viewed = excluded.last_viewed`,
		bookID, now); err != nil {
		return 0, err
	}
	var v int64
	err := s.db.QueryRowContext(ctx, "SELECT views FROM book_views WHERE book_id=?", bookID).Scan(&v)
	return v, err
}

// BookViewCount returns a single book's total view count (0 if never viewed).
func (s *Store) BookViewCount(ctx context.Context, bookID int64) (int64, error) {
	var v int64
	err := s.db.QueryRowContext(ctx, "SELECT views FROM book_views WHERE book_id=?", bookID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// AllBookViewCounts returns every book's view count, keyed by book ID. Books
// never viewed are absent (treat as 0). Used to rank the library by views.
func (s *Store) AllBookViewCounts(ctx context.Context) (map[int64]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT book_id, views FROM book_views")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var id, v int64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, err
		}
		out[id] = v
	}
	return out, rows.Err()
}

// AllBookLastRead returns each book's most recent read time across all users,
// keyed by book ID (books never read are absent). Used to rank the library by
// "recently read". Max is computed in Go by parsing timestamps rather than via
// SQL MAX(text), since RFC3339Nano trims trailing fractional zeros and so does
// not sort lexicographically.
func (s *Store) AllBookLastRead(ctx context.Context) (map[int64]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT book_id, updated_at FROM read_progress")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]time.Time{}
	for rows.Next() {
		var id int64
		var ts string
		if err := rows.Scan(&id, &ts); err != nil {
			return nil, err
		}
		t, perr := time.Parse(timeLayout, ts)
		if perr != nil {
			continue
		}
		if cur, ok := out[id]; !ok || t.After(cur) {
			out[id] = t
		}
	}
	return out, rows.Err()
}

// DeleteProgress removes a user's reading position for a book (all formats),
// resetting it to unread.
func (s *Store) DeleteProgress(ctx context.Context, userID, bookID int64) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM read_progress WHERE user_id=? AND book_id=?", userID, bookID)
	return err
}

// --- Page-list cache ---

// GetPageCache returns the cached page list if present and still valid for the
// given mtime/size, otherwise ErrNotFound.
func (s *Store) GetPageCache(ctx context.Context, bookID int64, format string, mtime, size int64) (*PageCacheEntry, error) {
	var e PageCacheEntry
	var pagesJSON, scanned string
	err := s.db.QueryRowContext(ctx, `SELECT book_id, format, file_path, pages_json, page_count, mtime, size, scanned_at
		FROM page_cache WHERE book_id=? AND format=?`, bookID, format).
		Scan(&e.BookID, &e.Format, &e.FilePath, &pagesJSON, &e.PageCount, &e.MTime, &e.Size, &scanned)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if e.MTime != mtime || e.Size != size {
		return nil, ErrNotFound // stale
	}
	// Names are base64-encoded so non-UTF-8 (e.g. Shift-JIS) ZIP entry names
	// round-trip losslessly. Any entry that isn't base64 is a pre-fix (plain
	// JSON) cache row: treat it as a miss so it re-scans in the new format.
	var encoded []string
	if err := json.Unmarshal([]byte(pagesJSON), &encoded); err != nil {
		return nil, ErrNotFound
	}
	e.Pages = make([]string, len(encoded))
	for i, s := range encoded {
		b, derr := base64.StdEncoding.DecodeString(s)
		if derr != nil {
			return nil, ErrNotFound
		}
		e.Pages[i] = string(b)
	}
	e.ScannedAt, _ = time.Parse(timeLayout, scanned)
	return &e, nil
}

// PutPageCache stores/updates a page list cache entry.
func (s *Store) PutPageCache(ctx context.Context, e PageCacheEntry) error {
	// base64-encode each name so non-UTF-8 bytes survive (json.Marshal would
	// otherwise replace them with U+FFFD, breaking page lookup).
	encoded := make([]string, len(e.Pages))
	for i, p := range e.Pages {
		encoded[i] = base64.StdEncoding.EncodeToString([]byte(p))
	}
	pagesJSON, err := json.Marshal(encoded)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO page_cache
		(book_id, format, file_path, pages_json, page_count, mtime, size, scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(book_id, format) DO UPDATE SET
			file_path=excluded.file_path, pages_json=excluded.pages_json,
			page_count=excluded.page_count, mtime=excluded.mtime, size=excluded.size,
			scanned_at=excluded.scanned_at`,
		e.BookID, e.Format, e.FilePath, string(pagesJSON), e.PageCount, e.MTime, e.Size,
		time.Now().UTC().Format(timeLayout))
	return err
}

// --- Settings ---

// AllPageCounts returns every indexed book's CBZ page count, keyed by book ID.
// Only books whose page list has been scanned (page_cache) are present. Used by
// the page-count filter.
func (s *Store) AllPageCounts(ctx context.Context) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT book_id, page_count FROM page_cache WHERE format='CBZ'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// GetSetting returns a setting value or "" if unset.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// SetSetting upserts a setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
