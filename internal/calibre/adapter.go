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
	Limit       int
	Offset      int
	Sort        string // title|timestamp|pubdate|author|series|rating; default title
	Desc        bool
	Search      string
	AuthorID    int64
	SeriesID    int64
	TagIDs      []int64 // multiple tags are AND-combined (a book must have all)
	PublisherID int64
	Language    string
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
		clauses = append(clauses, `(b.title LIKE ? OR b.author_sort LIKE ? OR EXISTS (
			SELECT 1 FROM books_authors_link bal JOIN authors au ON au.id=bal.author
			WHERE bal.book=b.id AND au.name LIKE ?))`)
		args = append(args, like, like, like)
	}
	if opts.AuthorID > 0 {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM books_authors_link bal WHERE bal.book=b.id AND bal.author=?)")
		args = append(args, opts.AuthorID)
	}
	if opts.SeriesID > 0 {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM books_series_link bsl WHERE bsl.book=b.id AND bsl.series=?)")
		args = append(args, opts.SeriesID)
	}
	// Each selected tag adds its own EXISTS clause, so they AND together: a book
	// must carry every selected tag to match.
	for _, tid := range opts.TagIDs {
		if tid > 0 {
			clauses = append(clauses, "EXISTS (SELECT 1 FROM books_tags_link btl WHERE btl.book=b.id AND btl.tag=?)")
			args = append(args, tid)
		}
	}
	if opts.PublisherID > 0 {
		clauses = append(clauses, "EXISTS (SELECT 1 FROM books_publishers_link bpl WHERE bpl.book=b.id AND bpl.publisher=?)")
		args = append(args, opts.PublisherID)
	}
	if lang := strings.TrimSpace(opts.Language); lang != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM books_languages_link bll
			JOIN languages l ON l.id=bll.lang_code WHERE bll.book=b.id AND l.lang_code=?)`)
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
		author_sort, path, uuid, has_cover, last_modified
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
			&b.AuthorSort, &b.Path, &uuid, &b.HasCover, &lastMod); err != nil {
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
