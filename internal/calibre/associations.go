package calibre

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// associations captures the related metadata to (re)write for a book. Each
// set* flag gates whether that relation is touched; when set, existing links
// are cleared and the provided values re-linked (get-or-create as needed).
type associations struct {
	replace bool

	authors    []string
	setAuthors bool

	series    string
	setSeries bool

	tags    []string
	setTags bool

	// addTags appends tags without removing existing ones (union). Used when
	// re-enriching from an external source so user-added tags are preserved.
	addTags    []string
	setAddTags bool

	publisher    string
	setPublisher bool

	languages    []string
	setLanguages bool

	rating    int
	setRating bool

	comments    string
	setComments bool

	identifiers    map[string]string
	setIdentifiers bool
}

func (a *Adapter) applyAssociations(ctx context.Context, tx *sql.Tx, book int64, as associations) error {
	if as.setAuthors {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_authors_link WHERE book=?", book); err != nil {
			return err
		}
		for _, name := range as.authors {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			id, err := getOrCreateNamed(ctx, tx, "authors", "sort", name, AuthorSort(name))
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO books_authors_link (book, author) VALUES (?, ?)", book, id); err != nil {
				return err
			}
		}
	}

	if as.setSeries {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_series_link WHERE book=?", book); err != nil {
			return err
		}
		if s := strings.TrimSpace(as.series); s != "" {
			id, err := getOrCreateNamed(ctx, tx, "series", "sort", s, TitleSort(s))
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO books_series_link (book, series) VALUES (?, ?)", book, id); err != nil {
				return err
			}
		}
	}

	// setTags replaces (delete then insert); setAddTags only inserts (union),
	// preserving user-added tags. Both link via INSERT OR IGNORE.
	if as.setTags {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_tags_link WHERE book=?", book); err != nil {
			return err
		}
	}
	if as.setTags || as.setAddTags {
		for _, t := range append(append([]string{}, as.tags...), as.addTags...) {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			id, err := getOrCreateNamed(ctx, tx, "tags", "", t, "")
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO books_tags_link (book, tag) VALUES (?, ?)", book, id); err != nil {
				return err
			}
		}
	}

	if as.setPublisher {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_publishers_link WHERE book=?", book); err != nil {
			return err
		}
		if p := strings.TrimSpace(as.publisher); p != "" {
			id, err := getOrCreateNamed(ctx, tx, "publishers", "sort", p, p)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO books_publishers_link (book, publisher) VALUES (?, ?)", book, id); err != nil {
				return err
			}
		}
	}

	if as.setLanguages {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_languages_link WHERE book=?", book); err != nil {
			return err
		}
		for i, code := range as.languages {
			code = strings.TrimSpace(code)
			if code == "" {
				continue
			}
			id, err := getOrCreateLanguage(ctx, tx, code)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR IGNORE INTO books_languages_link (book, lang_code, item_order) VALUES (?, ?, ?)",
				book, id, i); err != nil {
				return err
			}
		}
	}

	if as.setRating {
		if _, err := tx.ExecContext(ctx, "DELETE FROM books_ratings_link WHERE book=?", book); err != nil {
			return err
		}
		if as.rating > 0 {
			id, err := getOrCreateRating(ctx, tx, as.rating)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO books_ratings_link (book, rating) VALUES (?, ?)", book, id); err != nil {
				return err
			}
		}
	}

	if as.setComments {
		if _, err := tx.ExecContext(ctx, "DELETE FROM comments WHERE book=?", book); err != nil {
			return err
		}
		if c := strings.TrimSpace(as.comments); c != "" {
			if _, err := tx.ExecContext(ctx,
				"INSERT INTO comments (book, text) VALUES (?, ?)", book, as.comments); err != nil {
				return err
			}
		}
	}

	if as.setIdentifiers {
		if _, err := tx.ExecContext(ctx, "DELETE FROM identifiers WHERE book=?", book); err != nil {
			return err
		}
		for typ, val := range as.identifiers {
			typ, val = strings.TrimSpace(typ), strings.TrimSpace(val)
			if typ == "" || val == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx,
				"INSERT OR REPLACE INTO identifiers (book, type, val) VALUES (?, ?, ?)", book, typ, val); err != nil {
				return err
			}
		}
	}

	return nil
}

// getOrCreateNamed returns the id of a row in a name-keyed table, inserting it
// (with an optional sort column) when absent. Table/column names are internal
// constants, never user input.
func getOrCreateNamed(ctx context.Context, tx *sql.Tx, table, sortCol, name, sort string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM "+table+" WHERE name=?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var res sql.Result
	if sortCol != "" {
		res, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (name, "+sortCol+") VALUES (?, ?)", name, sort)
	} else {
		res, err = tx.ExecContext(ctx, "INSERT INTO "+table+" (name) VALUES (?)", name)
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func getOrCreateLanguage(ctx context.Context, tx *sql.Tx, code string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM languages WHERE lang_code=?", code).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO languages (lang_code) VALUES (?)", code)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func getOrCreateRating(ctx context.Context, tx *sql.Tx, rating int) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM ratings WHERE rating=?", rating).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO ratings (rating) VALUES (?)", rating)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
