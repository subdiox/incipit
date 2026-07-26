package calibre

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// Adapter is the read/write gateway to a Calibre library. All writes are
// serialized through writeMu so the library has a single logical writer, which
// keeps metadata.db and the on-disk folders consistent.
type Adapter struct {
	db          *sql.DB
	libraryPath string
	readOnly    bool
	writeMu     sync.Mutex
}

// Open connects to the Calibre library at libraryPath. The library (and a fresh
// metadata.db) is created if missing, unless readOnly is set.
func Open(libraryPath string, readOnly bool) (*Adapter, error) {
	if !readOnly {
		if _, err := EnsureLibrary(libraryPath); err != nil {
			return nil, err
		}
	}
	db, err := openDB(filepath.Join(libraryPath, "metadata.db"), readOnly)
	if err != nil {
		return nil, err
	}
	if !readOnly {
		// Side table for an optional per-book favorites/popularity count
		// imported from a book's source. Kept out of the standard Calibre
		// tables; empty for libraries that never populate it. Created on every
		// open so existing libraries pick it up (EnsureLibrary only runs on
		// first create).
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS book_favorites (
			book INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
			favorites INTEGER NOT NULL DEFAULT 0)`); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure book_favorites: %w", err)
		}
		// Side tables for optional externally-curated ranking lists (e.g. a
		// source's per-window popularity charts). Purely generic: `ranking_lists`
		// is self-describing (an opaque key + a human label + tab order) and
		// `book_rankings` holds each list's books in explicit rank order. An
		// importer populates them; libraries that never do (leaving no rows) get
		// no ranking UI. Kept out of the standard Calibre tables, created on every
		// writable open so existing libraries pick them up.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS ranking_lists (
			key        TEXT PRIMARY KEY,
			label      TEXT NOT NULL,
			position   INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT)`); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure ranking_lists: %w", err)
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS book_rankings (
			list TEXT NOT NULL,
			rank INTEGER NOT NULL,
			book INTEGER NOT NULL REFERENCES books(id) ON DELETE CASCADE,
			PRIMARY KEY (list, book))`); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure book_rankings: %w", err)
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS book_rankings_list_rank
			ON book_rankings(list, rank)`); err != nil {
			db.Close()
			return nil, fmt.Errorf("ensure book_rankings index: %w", err)
		}
	}
	return &Adapter{db: db, libraryPath: libraryPath, readOnly: readOnly}, nil
}

// Close releases the database handle.
func (a *Adapter) Close() error { return a.db.Close() }

// LibraryPath returns the on-disk library root.
func (a *Adapter) LibraryPath() string { return a.libraryPath }

// ReadOnly reports whether writes are disabled.
func (a *Adapter) ReadOnly() bool { return a.readOnly }

// BookFolder returns the absolute path to a book's folder.
func (a *Adapter) BookFolder(b *Book) string {
	return filepath.Join(a.libraryPath, filepath.FromSlash(b.Path))
}

// ListOptions controls a book listing query.
type ListOptions struct {
	Limit         int
	Offset        int
	Sort          string // title|timestamp|pubdate|author|series|rating; default title
	Desc          bool
	Search        string
	AuthorID      int64
	SeriesID      int64
	TagIDs        []int64 // multiple tags are AND-combined (a book must have all)
	AnyTagIDs     []int64 // OR-combined as one group (a book must have at least one)
	ExcludeTagIDs []int64 // a book carrying ANY of these is excluded (NOT EXISTS)
	PublisherID   int64
	Language      string
}

// ListResult is a page of books plus the total matching count.
type ListResult struct {
	Books []Book `json:"books"`
	Total int    `json:"total"`
}

var sortColumns = map[string]string{
	"title":     "b.sort",
	"timestamp": "b.timestamp",
	"pubdate":   "b.pubdate",
	"author":    "b.author_sort",
	"series":    "b.series_index",
	"rating":    "(SELECT r.rating FROM books_ratings_link brl JOIN ratings r ON r.id=brl.rating WHERE brl.book=b.id)",
	"favorites": "(SELECT favorites FROM book_favorites WHERE book=b.id)",
}

// ListBooks returns a page of books matching opts, fully hydrated.
func (a *Adapter) ListBooks(ctx context.Context, opts ListOptions) (*ListResult, error) {
	where, wargs := a.buildFilters(opts)

	countQ := "SELECT COUNT(*) FROM books b" + where
	var total int
	if err := a.db.QueryRowContext(ctx, countQ, wargs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count books: %w", err)
	}

	col := sortColumns[opts.Sort]
	if col == "" {
		col = "b.sort"
	}
	dir := "ASC"
	if opts.Desc {
		dir = "DESC"
	}
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	listQ := fmt.Sprintf(
		"SELECT b.id FROM books b%s ORDER BY %s %s, b.id %s LIMIT ? OFFSET ?",
		where, col, dir, dir,
	)
	args := append(append([]any{}, wargs...), limit, opts.Offset)

	rows, err := a.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, fmt.Errorf("list books: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	books, err := a.loadBooks(ctx, ids)
	if err != nil {
		return nil, err
	}
	if books == nil {
		books = []Book{} // serialize as [] not null so clients can map safely
	}
	return &ListResult{Books: books, Total: total}, nil
}

// buildFilters constructs the WHERE clause and args for a listing query.
func (a *Adapter) buildFilters(opts ListOptions) (string, []any) {
	var clauses []string
	var args []any

	if s := strings.TrimSpace(opts.Search); s != "" {
		like := "%" + s + "%"
		// Search across every user-visible field: title, author, series (name +
		// volume number), tags, publisher and the description/comments.
		//
		// Each linked category matches names first, then resolves books through the
		// link table's category index:
		//   book IN (SELECT book FROM link WHERE cat IN (SELECT id FROM cat WHERE name LIKE ?))
		// This is critical on a large library: the naive JOIN form makes the planner
		// SCAN the whole link table (millions of rows, ~1s), whereas matching the
		// (few) category ids first keeps the whole search ~0.25s on 300k books.
		clauses = append(clauses, `(
			b.title LIKE ?
			OR b.author_sort LIKE ?
			OR CAST(b.series_index AS TEXT) LIKE ?
			OR b.id IN (SELECT book FROM books_authors_link WHERE author IN (SELECT id FROM authors WHERE name LIKE ?))
			OR b.id IN (SELECT book FROM books_series_link WHERE series IN (SELECT id FROM series WHERE name LIKE ?))
			OR b.id IN (SELECT book FROM books_tags_link WHERE tag IN (SELECT id FROM tags WHERE name LIKE ?))
			OR b.id IN (SELECT book FROM books_publishers_link WHERE publisher IN (SELECT id FROM publishers WHERE name LIKE ?))
			OR b.id IN (SELECT book FROM comments WHERE text LIKE ?)
		)`)
		args = append(args, like, like, like, like, like, like, like, like)
	}
	// Positive category filters use the same `b.id IN (SELECT book FROM link
	// WHERE cat=?)` form as the search clause above, for the same reason. The
	// equivalent correlated `EXISTS (SELECT 1 FROM link WHERE link.book=b.id AND
	// link.cat=?)` reads more naturally but is a trap at this size: the planner
	// drives it by SCANning all of `books` (through a covering index, in *sort*
	// order) and probing the link table once per book, so the work is proportional
	// to the whole library and every probe is a random read. The IN form inverts
	// that — it drives from the category index and touches only the matching books.
	// Measured on a 300k-book library (tag with ~97k books, filtered listing):
	// ~48s for the EXISTS form vs ~1s for this one.
	if opts.AuthorID > 0 {
		clauses = append(clauses, "b.id IN (SELECT book FROM books_authors_link WHERE author=?)")
		args = append(args, opts.AuthorID)
	}
	if opts.SeriesID > 0 {
		clauses = append(clauses, "b.id IN (SELECT book FROM books_series_link WHERE series=?)")
		args = append(args, opts.SeriesID)
	}
	// Each selected tag adds its own IN clause, so they AND together: a book must
	// carry every selected tag to match.
	for _, tid := range opts.TagIDs {
		if tid > 0 {
			clauses = append(clauses, "b.id IN (SELECT book FROM books_tags_link WHERE tag=?)")
			args = append(args, tid)
		}
	}
	// AnyTagIDs form a single OR group (a book needs at least one), which then
	// ANDs with the other clauses. Used by "match any" collections.
	var anyEx []string
	for _, tid := range opts.AnyTagIDs {
		if tid > 0 {
			anyEx = append(anyEx, "b.id IN (SELECT book FROM books_tags_link WHERE tag=?)")
			args = append(args, tid)
		}
	}
	if len(anyEx) > 0 {
		clauses = append(clauses, "("+strings.Join(anyEx, " OR ")+")")
	}
	// Exclude tags: each adds a NOT EXISTS clause, so a book carrying any of them
	// is filtered out. Used by collection / home-library exclude filters. This one
	// stays correlated: a negative filter can never drive the scan (it selects
	// most of the library), so there is nothing to invert.
	for _, tid := range opts.ExcludeTagIDs {
		if tid > 0 {
			clauses = append(clauses, "NOT EXISTS (SELECT 1 FROM books_tags_link btl WHERE btl.book=b.id AND btl.tag=?)")
			args = append(args, tid)
		}
	}
	if opts.PublisherID > 0 {
		clauses = append(clauses, "b.id IN (SELECT book FROM books_publishers_link WHERE publisher=?)")
		args = append(args, opts.PublisherID)
	}
	if lang := strings.TrimSpace(opts.Language); lang != "" {
		clauses = append(clauses, `b.id IN (SELECT bll.book FROM books_languages_link bll
			JOIN languages l ON l.id=bll.lang_code WHERE l.lang_code=?)`)
		args = append(args, lang)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// GetBook returns a single fully-hydrated book, or sql.ErrNoRows if absent.
func (a *Adapter) GetBook(ctx context.Context, id int64) (*Book, error) {
	books, err := a.loadBooks(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	if len(books) == 0 {
		return nil, sql.ErrNoRows
	}
	return &books[0], nil
}

// FilteredIDs returns the IDs of every book matching opts' filters (search,
// author, series, tag, publisher, language), ignoring sort/limit/offset, in
// newest-first order. It lets callers rank/paginate the matching set by data
// that lives outside metadata.db (e.g. app.db view counts) without mixing the
// two databases.
func (a *Adapter) FilteredIDs(ctx context.Context, opts ListOptions) ([]int64, error) {
	where, wargs := a.buildFilters(opts)
	// Order by the requested column when it's an SQL-sortable one, so callers
	// that only filter/paginate in Go still get the right order. Unknown sorts
	// (e.g. app.db-ranked "views") fall back to newest-first, which those callers
	// re-sort anyway.
	orderBy := " ORDER BY b.timestamp DESC, b.id DESC"
	if col, ok := sortColumns[opts.Sort]; ok {
		dir := "ASC"
		if opts.Desc {
			dir = "DESC"
		}
		orderBy = fmt.Sprintf(" ORDER BY %s %s, b.id %s", col, dir, dir)
	}
	q := "SELECT b.id FROM books b" + where + orderBy
	rows, err := a.db.QueryContext(ctx, q, wargs...)
	if err != nil {
		return nil, fmt.Errorf("filtered ids: %w", err)
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

// RankingList describes one externally-curated ranking (a self-describing entry
// from the ranking_lists side table). Count is the number of books currently in
// the list.
type RankingList struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// RankingLists returns the configured ranking lists in tab order, each with its
// current book count. Empty (not an error) when the library has no rankings —
// including when the side tables don't exist yet (a library no importer has ever
// written), so ordinary read-only libraries are unaffected.
func (a *Adapter) RankingLists(ctx context.Context) ([]RankingList, error) {
	if !a.tableExists(ctx, "ranking_lists") {
		return nil, nil
	}
	// Only surface lists that actually have books; join against book_rankings so
	// an empty or stale list header never shows an empty tab.
	q := `SELECT rl.key, rl.label, COUNT(br.book)
		FROM ranking_lists rl
		JOIN book_rankings br ON br.list = rl.key
		GROUP BY rl.key, rl.label, rl.position
		ORDER BY rl.position, rl.key`
	rows, err := a.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("ranking lists: %w", err)
	}
	defer rows.Close()
	var out []RankingList
	for rows.Next() {
		var rl RankingList
		if err := rows.Scan(&rl.Key, &rl.Label, &rl.Count); err != nil {
			return nil, err
		}
		out = append(out, rl)
	}
	return out, rows.Err()
}

// RankedBookIDs returns the book IDs of one ranking list in explicit rank order.
// Books that were deleted since the list was written are skipped by the join, so
// the sequence compacts but stays in rank order. Empty when the list is unknown
// or the side tables don't exist.
func (a *Adapter) RankedBookIDs(ctx context.Context, key string) ([]int64, error) {
	if key == "" || !a.tableExists(ctx, "book_rankings") {
		return nil, nil
	}
	q := `SELECT br.book FROM book_rankings br
		JOIN books b ON b.id = br.book
		WHERE br.list = ?
		ORDER BY br.rank`
	rows, err := a.db.QueryContext(ctx, q, key)
	if err != nil {
		return nil, fmt.Errorf("ranked book ids: %w", err)
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

// tableExists reports whether a table is present, so ranking reads degrade to
// "no rankings" on libraries whose side tables were never created (e.g. a
// read-only library no importer has written).
func (a *Adapter) tableExists(ctx context.Context, name string) bool {
	var n int
	err := a.db.QueryRowContext(ctx,
		"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&n)
	return err == nil
}

// BooksByIDs hydrates the given book IDs, preserving their order and silently
// skipping IDs that no longer exist (e.g. a book deleted since it was read).
// Used to turn an ordered list of IDs (reading history, recently read) into
// full book records.
func (a *Adapter) BooksByIDs(ctx context.Context, ids []int64) ([]Book, error) {
	return a.loadBooks(ctx, ids)
}

// loadBooks hydrates the given book IDs, preserving their order.
func (a *Adapter) loadBooks(ctx context.Context, ids []int64) ([]Book, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	in := placeholders(len(ids))
	idArgs := toAnySlice(ids)

	q := fmt.Sprintf(`SELECT id, title, sort, timestamp, pubdate, series_index,
		author_sort, path, uuid, has_cover, last_modified,
		COALESCE((SELECT favorites FROM book_favorites WHERE book=books.id), 0)
		FROM books WHERE id IN (%s)`, in)
	rows, err := a.db.QueryContext(ctx, q, idArgs...)
	if err != nil {
		return nil, fmt.Errorf("load books: %w", err)
	}
	defer rows.Close()

	byID := map[int64]*Book{}
	for rows.Next() {
		var b Book
		var sortS, ts, pub, lastMod sql.NullString
		var uuid sql.NullString
		if err := rows.Scan(&b.ID, &b.Title, &sortS, &ts, &pub, &b.SeriesIndex,
			&b.AuthorSort, &b.Path, &uuid, &b.HasCover, &lastMod, &b.Favorites); err != nil {
			return nil, err
		}
		b.Sort = sortS.String
		b.UUID = uuid.String
		b.Timestamp = parseCalibreTime(ts.String)
		b.PubDate = parseCalibreTime(pub.String)
		b.LastModified = parseCalibreTime(lastMod.String)
		b.Authors = []Author{}
		b.Tags = []Tag{}
		b.Languages = []string{}
		b.Identifiers = map[string]string{}
		b.Formats = []Format{}
		byID[b.ID] = &b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := a.attachRelations(ctx, byID, in, idArgs); err != nil {
		return nil, err
	}

	out := make([]Book, 0, len(ids))
	for _, id := range ids {
		if b, ok := byID[id]; ok {
			out = append(out, *b)
		}
	}
	return out, nil
}

// attachRelations batch-loads every related entity for the given books.
func (a *Adapter) attachRelations(ctx context.Context, byID map[int64]*Book, in string, idArgs []any) error {
	// Authors (ordered by link id for stable display order).
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT bal.book, au.id, au.name, au.sort
		FROM books_authors_link bal JOIN authors au ON au.id=bal.author
		WHERE bal.book IN (%s) ORDER BY bal.id`, in), idArgs, func(s scanner) error {
		var book int64
		var au Author
		var sort sql.NullString
		if err := s.Scan(&book, &au.ID, &au.Name, &sort); err != nil {
			return err
		}
		au.Sort = sort.String
		if b := byID[book]; b != nil {
			b.Authors = append(b.Authors, au)
		}
		return nil
	}); err != nil {
		return err
	}

	// Series.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT bsl.book, s.id, s.name, s.sort
		FROM books_series_link bsl JOIN series s ON s.id=bsl.series
		WHERE bsl.book IN (%s)`, in), idArgs, func(sc scanner) error {
		var book int64
		var ser Series
		var sort sql.NullString
		if err := sc.Scan(&book, &ser.ID, &ser.Name, &sort); err != nil {
			return err
		}
		ser.Sort = sort.String
		if b := byID[book]; b != nil {
			b.Series = &ser
		}
		return nil
	}); err != nil {
		return err
	}

	// Tags.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT btl.book, t.id, t.name
		FROM books_tags_link btl JOIN tags t ON t.id=btl.tag
		WHERE btl.book IN (%s) ORDER BY t.name`, in), idArgs, func(s scanner) error {
		var book int64
		var tag Tag
		if err := s.Scan(&book, &tag.ID, &tag.Name); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Tags = append(b.Tags, tag)
		}
		return nil
	}); err != nil {
		return err
	}

	// Publisher.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT bpl.book, p.id, p.name, p.sort
		FROM books_publishers_link bpl JOIN publishers p ON p.id=bpl.publisher
		WHERE bpl.book IN (%s)`, in), idArgs, func(sc scanner) error {
		var book int64
		var pub Publisher
		var sort sql.NullString
		if err := sc.Scan(&book, &pub.ID, &pub.Name, &sort); err != nil {
			return err
		}
		pub.Sort = sort.String
		if b := byID[book]; b != nil {
			b.Publisher = &pub
		}
		return nil
	}); err != nil {
		return err
	}

	// Languages (ordered).
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT bll.book, l.lang_code
		FROM books_languages_link bll JOIN languages l ON l.id=bll.lang_code
		WHERE bll.book IN (%s) ORDER BY bll.item_order`, in), idArgs, func(s scanner) error {
		var book int64
		var lang string
		if err := s.Scan(&book, &lang); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Languages = append(b.Languages, lang)
		}
		return nil
	}); err != nil {
		return err
	}

	// Ratings.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT brl.book, r.rating
		FROM books_ratings_link brl JOIN ratings r ON r.id=brl.rating
		WHERE brl.book IN (%s)`, in), idArgs, func(s scanner) error {
		var book int64
		var rating int
		if err := s.Scan(&book, &rating); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Rating = rating
		}
		return nil
	}); err != nil {
		return err
	}

	// Comments.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT book, text FROM comments
		WHERE book IN (%s)`, in), idArgs, func(s scanner) error {
		var book int64
		var text string
		if err := s.Scan(&book, &text); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Comments = text
		}
		return nil
	}); err != nil {
		return err
	}

	// Identifiers.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT book, type, val FROM identifiers
		WHERE book IN (%s)`, in), idArgs, func(s scanner) error {
		var book int64
		var typ, val string
		if err := s.Scan(&book, &typ, &val); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Identifiers[typ] = val
		}
		return nil
	}); err != nil {
		return err
	}

	// Formats.
	if err := a.eachRow(ctx, fmt.Sprintf(`SELECT book, format, uncompressed_size, name
		FROM data WHERE book IN (%s) ORDER BY format`, in), idArgs, func(s scanner) error {
		var book int64
		var f Format
		if err := s.Scan(&book, &f.Format, &f.Size, &f.Name); err != nil {
			return err
		}
		if b := byID[book]; b != nil {
			b.Formats = append(b.Formats, f)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// scanner abstracts *sql.Rows for the row-iteration helper.
type scanner interface{ Scan(dest ...any) error }

func (a *Adapter) eachRow(ctx context.Context, query string, args []any, fn func(scanner) error) error {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func toAnySlice(ids []int64) []any {
	out := make([]any, len(ids))
	for i, v := range ids {
		out[i] = v
	}
	return out
}
