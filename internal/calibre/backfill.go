package calibre

import (
	"context"
	"strings"
)

// SeriesRef is a series and its book ids, used by bulk backfills.
type SeriesRef struct {
	ID    int64
	Name  string
	Books []int64
}

// StandaloneRef is a book with no series (its own backfill unit).
type StandaloneRef struct {
	ID    int64
	Title string
}

// ListSeries returns every series with its member book ids, ordered by id. It's
// the unit for a category backfill: cmoa's category is a property of the work,
// so one lookup per series tags all its volumes.
func (a *Adapter) ListSeries(ctx context.Context) ([]SeriesRef, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT s.id, s.name, bsl.book
		FROM series s
		JOIN books_series_link bsl ON bsl.series = s.id
		ORDER BY s.id, bsl.book`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SeriesRef
	byID := map[int64]int{} // series id → index in out
	for rows.Next() {
		var sid, book int64
		var name string
		if err := rows.Scan(&sid, &name, &book); err != nil {
			return nil, err
		}
		i, ok := byID[sid]
		if !ok {
			i = len(out)
			byID[sid] = i
			out = append(out, SeriesRef{ID: sid, Name: name})
		}
		out[i].Books = append(out[i].Books, book)
	}
	return out, rows.Err()
}

// ListStandaloneBooks returns books that belong to no series, ordered by id.
func (a *Adapter) ListStandaloneBooks(ctx context.Context) ([]StandaloneRef, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT b.id, b.title FROM books b
		WHERE NOT EXISTS (SELECT 1 FROM books_series_link l WHERE l.book = b.id)
		ORDER BY b.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StandaloneRef
	for rows.Next() {
		var r StandaloneRef
		if err := rows.Scan(&r.ID, &r.Title); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// BooksWithTagPrefix returns the set of book ids that already carry any tag
// starting with prefix (e.g. "ジャンル:"). Used to make a backfill resumable —
// units whose books are already tagged can be skipped.
func (a *Adapter) BooksWithTagPrefix(ctx context.Context, prefix string) (map[int64]bool, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT DISTINCT l.book FROM books_tags_link l
		JOIN tags t ON t.id = l.tag
		WHERE t.name LIKE ? ESCAPE '\'`, escapeLike(prefix)+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		seen[id] = true
	}
	return seen, rows.Err()
}

// AddTagToBooks links tagName to every bookID in a single transaction, creating
// the tag if needed and keeping any existing links (INSERT OR IGNORE). It does
// NOT rewrite per-book metadata.opf — it's for bulk backfills, not user edits.
func (a *Adapter) AddTagToBooks(ctx context.Context, tagName string, bookIDs []int64) error {
	if a.readOnly {
		return ErrReadOnly
	}
	if strings.TrimSpace(tagName) == "" || len(bookIDs) == 0 {
		return nil
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	tagID, err := getOrCreateNamed(ctx, tx, "tags", "", tagName, "")
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT OR IGNORE INTO books_tags_link (book, tag) VALUES (?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range bookIDs {
		if _, err := stmt.ExecContext(ctx, id, tagID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// escapeLike escapes LIKE wildcards so a literal prefix matches literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
