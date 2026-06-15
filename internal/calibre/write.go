package calibre

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrReadOnly is returned by every write method when the adapter is read-only.
var ErrReadOnly = errors.New("calibre: library is read-only")

// AddBookInput describes a new book to import.
type AddBookInput struct {
	Title       string
	Authors     []string
	Series      string
	SeriesIndex float64
	Tags        []string
	Publisher   string
	Languages   []string
	Rating      int // 0..10
	Comments    string
	Identifiers map[string]string
	PubDate     time.Time
	Format      string    // e.g. "CBZ"
	Data        io.Reader // book file content
	Cover       []byte    // optional JPEG cover
}

// AddBook imports a new book: it inserts the metadata rows, creates the book
// folder, writes the file, optional cover and metadata.opf, all consistently.
func (a *Adapter) AddBook(ctx context.Context, in AddBookInput) (*Book, error) {
	if a.readOnly {
		return nil, ErrReadOnly
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("calibre: title is required")
	}
	if in.Format == "" || in.Data == nil {
		return nil, errors.New("calibre: format and data are required")
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	var createdFolder string
	defer func() {
		if !committed {
			_ = tx.Rollback()
			if createdFolder != "" {
				_ = os.RemoveAll(createdFolder)
			}
		}
	}()

	now := time.Now().UTC()
	pubdate := in.PubDate
	if pubdate.IsZero() {
		pubdate = now
	}
	authorSort := combineAuthorSort(in.Authors)

	res, err := tx.ExecContext(ctx, `INSERT INTO books
		(title, author_sort, path, series_index, timestamp, pubdate, last_modified)
		VALUES (?, ?, '', ?, ?, ?, ?)`,
		in.Title, authorSort, seriesIndexOrDefault(in.SeriesIndex),
		formatCalibreTime(now), formatCalibreTime(pubdate), formatCalibreTime(now))
	if err != nil {
		return nil, fmt.Errorf("insert book: %w", err)
	}
	id, _ := res.LastInsertId()

	var uuid string
	if err := tx.QueryRowContext(ctx, "SELECT uuid FROM books WHERE id=?", id).Scan(&uuid); err != nil {
		return nil, fmt.Errorf("read uuid: %w", err)
	}

	relPath, fileBase := bookRelPath(id, in.Title, in.Authors)
	if _, err := tx.ExecContext(ctx, "UPDATE books SET path=? WHERE id=?", relPath, id); err != nil {
		return nil, err
	}

	if err := a.applyAssociations(ctx, tx, id, associations{
		replace:        true,
		authors:        in.Authors,
		setAuthors:     true,
		series:         in.Series,
		setSeries:      true,
		tags:           in.Tags,
		setTags:        true,
		publisher:      in.Publisher,
		setPublisher:   true,
		languages:      in.Languages,
		setLanguages:   true,
		rating:         in.Rating,
		setRating:      true,
		comments:       in.Comments,
		setComments:    true,
		identifiers:    in.Identifiers,
		setIdentifiers: true,
	}); err != nil {
		return nil, err
	}

	// Filesystem: create folder and write the book file + cover. On failure we
	// only remove this book's folder (the author folder may be shared).
	folder := filepath.Join(a.libraryPath, filepath.FromSlash(relPath))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return nil, fmt.Errorf("create book folder: %w", err)
	}
	createdFolder = folder

	format := strings.ToUpper(in.Format)
	dataFile := filepath.Join(folder, fileBase+"."+strings.ToLower(format))
	size, err := writeReaderToFile(dataFile, in.Data)
	if err != nil {
		return nil, fmt.Errorf("write book file: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO data (book, format, uncompressed_size, name)
		VALUES (?, ?, ?, ?)`, id, format, size, fileBase); err != nil {
		return nil, err
	}

	hasCover := len(in.Cover) > 0
	if hasCover {
		if err := os.WriteFile(filepath.Join(folder, "cover.jpg"), in.Cover, 0o644); err != nil {
			return nil, fmt.Errorf("write cover: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE books SET has_cover=1 WHERE id=?", id); err != nil {
			return nil, err
		}
	}

	// metadata.opf for round-trip compatibility.
	opfBook := bookForOPF(id, uuid, in)
	opf, err := buildOPF(opfBook)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(folder, "metadata.opf"), opf, 0o644); err != nil {
		return nil, fmt.Errorf("write opf: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit add book: %w", err)
	}
	committed = true
	return a.GetBook(ctx, id)
}

// UpdateBookInput holds optional metadata changes. Nil fields are left
// unchanged; a non-nil pointer to a zero value clears the field.
type UpdateBookInput struct {
	Title       *string
	Authors     *[]string
	Series      *string
	SeriesIndex *float64
	Tags        *[]string
	Publisher   *string
	Languages   *[]string
	Rating      *int
	Comments    *string
	Identifiers *map[string]string
	PubDate     *time.Time
}

// UpdateBook applies metadata changes, moving the book folder and renaming its
// files when the title or primary author changes.
func (a *Adapter) UpdateBook(ctx context.Context, id int64, in UpdateBookInput) (*Book, error) {
	if a.readOnly {
		return nil, ErrReadOnly
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	existing, err := a.GetBook(ctx, id)
	if err != nil {
		return nil, err
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	committed := false
	var rollbackFS func()
	defer func() {
		if !committed {
			_ = tx.Rollback()
			if rollbackFS != nil {
				rollbackFS()
			}
		}
	}()

	// Resolve the effective title/authors for path computation.
	newTitle := existing.Title
	if in.Title != nil {
		newTitle = *in.Title
		if _, err := tx.ExecContext(ctx, "UPDATE books SET title=? WHERE id=?", newTitle, id); err != nil {
			return nil, err
		}
	}
	newAuthors := authorNames(existing.Authors)
	if in.Authors != nil {
		newAuthors = *in.Authors
		if _, err := tx.ExecContext(ctx, "UPDATE books SET author_sort=? WHERE id=?",
			combineAuthorSort(newAuthors), id); err != nil {
			return nil, err
		}
	}
	if in.SeriesIndex != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE books SET series_index=? WHERE id=?",
			seriesIndexOrDefault(*in.SeriesIndex), id); err != nil {
			return nil, err
		}
	}
	if in.PubDate != nil {
		if _, err := tx.ExecContext(ctx, "UPDATE books SET pubdate=? WHERE id=?",
			formatCalibreTime(*in.PubDate), id); err != nil {
			return nil, err
		}
	}

	assoc := associations{replace: true}
	apply := false
	if in.Authors != nil {
		assoc.authors, assoc.setAuthors, apply = *in.Authors, true, true
	}
	if in.Series != nil {
		assoc.series, assoc.setSeries = *in.Series, true
		apply = true
	}
	if in.Tags != nil {
		assoc.tags, assoc.setTags = *in.Tags, true
		apply = true
	}
	if in.Publisher != nil {
		assoc.publisher, assoc.setPublisher = *in.Publisher, true
		apply = true
	}
	if in.Languages != nil {
		assoc.languages, assoc.setLanguages = *in.Languages, true
		apply = true
	}
	if in.Rating != nil {
		assoc.rating, assoc.setRating = *in.Rating, true
		apply = true
	}
	if in.Comments != nil {
		assoc.comments, assoc.setComments = *in.Comments, true
		apply = true
	}
	if in.Identifiers != nil {
		assoc.identifiers, assoc.setIdentifiers = *in.Identifiers, true
		apply = true
	}
	if apply {
		if err := a.applyAssociations(ctx, tx, id, assoc); err != nil {
			return nil, err
		}
	}

	// Move folder / rename files if the path-determining fields changed.
	if in.Title != nil || in.Authors != nil {
		newRel, newBase := bookRelPath(id, newTitle, newAuthors)
		if newRel != existing.Path {
			rb, err := a.relocateBook(ctx, tx, existing, newRel, newBase)
			if err != nil {
				return nil, err
			}
			rollbackFS = rb
		}
	}

	if _, err := tx.ExecContext(ctx, "UPDATE books SET last_modified=? WHERE id=?",
		formatCalibreTime(time.Now().UTC()), id); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update: %w", err)
	}
	committed = true

	updated, err := a.GetBook(ctx, id)
	if err != nil {
		return nil, err
	}
	a.rewriteOPF(updated) // best-effort
	return updated, nil
}

// relocateBook performs the on-disk folder move and file renames for a changed
// path, updating books.path and data.name in the transaction. It returns a
// function that undoes the filesystem move should the transaction fail.
func (a *Adapter) relocateBook(ctx context.Context, tx *sql.Tx, existing *Book, newRel, newBase string) (func(), error) {
	oldFolder := filepath.Join(a.libraryPath, filepath.FromSlash(existing.Path))
	newFolder := filepath.Join(a.libraryPath, filepath.FromSlash(newRel))

	if err := os.MkdirAll(filepath.Dir(newFolder), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(oldFolder); err == nil {
		if err := os.Rename(oldFolder, newFolder); err != nil {
			return nil, fmt.Errorf("move book folder: %w", err)
		}
	} else if err := os.MkdirAll(newFolder, 0o755); err != nil {
		return nil, err
	}
	rollback := func() { _ = os.Rename(newFolder, oldFolder) }

	// Rename each format file to the new basename and update data.name.
	for _, f := range existing.Formats {
		oldFile := filepath.Join(newFolder, f.Name+"."+strings.ToLower(f.Format))
		newFile := filepath.Join(newFolder, newBase+"."+strings.ToLower(f.Format))
		if oldFile != newFile {
			if _, err := os.Stat(oldFile); err == nil {
				if err := os.Rename(oldFile, newFile); err != nil {
					rollback()
					return nil, fmt.Errorf("rename format file: %w", err)
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "UPDATE data SET name=? WHERE book=?", newBase, existing.ID); err != nil {
		rollback()
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE books SET path=? WHERE id=?", newRel, existing.ID); err != nil {
		rollback()
		return nil, err
	}
	return rollback, nil
}

// DeleteBook removes a book's metadata (cascade trigger) and its folder.
func (a *Adapter) DeleteBook(ctx context.Context, id int64) error {
	if a.readOnly {
		return ErrReadOnly
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	book, err := a.GetBook(ctx, id)
	if err != nil {
		return err
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM books WHERE id=?", id); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete book: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete: %w", err)
	}
	// Best-effort folder removal after the metadata is gone.
	_ = os.RemoveAll(a.BookFolder(book))
	return nil
}

// rewriteOPF regenerates metadata.opf for a book; failures are non-fatal.
func (a *Adapter) rewriteOPF(b *Book) {
	opf, err := buildOPF(b)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(a.BookFolder(b), "metadata.opf"), opf, 0o644)
}

func seriesIndexOrDefault(v float64) float64 {
	if v <= 0 {
		return 1.0
	}
	return v
}

func firstAuthor(authors []string) string {
	if len(authors) > 0 && strings.TrimSpace(authors[0]) != "" {
		return authors[0]
	}
	return "Unknown"
}

func authorNames(authors []Author) []string {
	out := make([]string, len(authors))
	for i, au := range authors {
		out[i] = au.Name
	}
	return out
}

func combineAuthorSort(authors []string) string {
	parts := make([]string, 0, len(authors))
	for _, name := range authors {
		if s := strings.TrimSpace(name); s != "" {
			parts = append(parts, AuthorSort(s))
		}
	}
	if len(parts) == 0 {
		return "Unknown"
	}
	return strings.Join(parts, " & ")
}

func bookForOPF(id int64, uuid string, in AddBookInput) *Book {
	b := &Book{
		ID:          id,
		Title:       in.Title,
		UUID:        uuid,
		Comments:    in.Comments,
		PubDate:     in.PubDate,
		Languages:   in.Languages,
		Identifiers: in.Identifiers,
	}
	for _, name := range in.Authors {
		b.Authors = append(b.Authors, Author{Name: name, Sort: AuthorSort(name)})
	}
	if strings.TrimSpace(in.Series) != "" {
		b.Series = &Series{Name: in.Series}
	}
	if strings.TrimSpace(in.Publisher) != "" {
		b.Publisher = &Publisher{Name: in.Publisher}
	}
	for _, t := range in.Tags {
		b.Tags = append(b.Tags, Tag{Name: t})
	}
	return b
}

func writeReaderToFile(path string, r io.Reader) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, r)
	if err != nil {
		return 0, err
	}
	return n, f.Sync()
}
