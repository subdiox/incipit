package calibre

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// facetQuery returns the SQL for a category facet with book counts, filtered to
// a name LIKE search when search is non-empty.
func facetQuery(table, linkTable, linkCol string, hasSort bool) string {
	sortExpr := "''"
	if hasSort {
		sortExpr = "COALESCE(c.sort, c.name)"
	}
	return fmt.Sprintf(`SELECT c.id, c.name, %s, COUNT(l.book)
		FROM %s c JOIN %s l ON l.%s = c.id
		GROUP BY c.id ORDER BY %s COLLATE NOCASE`, sortExpr, table, linkTable, linkCol, sortExpr)
}

// Authors returns all authors with at least one book, sorted.
func (a *Adapter) Authors(ctx context.Context) ([]Facet, error) {
	return a.facets(ctx, facetQuery("authors", "books_authors_link", "author", true))
}

// Series returns all series with at least one book, sorted.
func (a *Adapter) SeriesList(ctx context.Context) ([]Facet, error) {
	return a.facets(ctx, facetQuery("series", "books_series_link", "series", true))
}

// Tags returns all tags with at least one book, sorted.
func (a *Adapter) Tags(ctx context.Context) ([]Facet, error) {
	return a.facets(ctx, facetQuery("tags", "books_tags_link", "tag", false))
}

// FacetQuery filters a facet listing so huge categories (a 100k+-tag library)
// need not be shipped whole: search by name and cap to Limit, or resolve a
// specific set of IDs (for rendering already-selected chips).
type FacetQuery struct {
	Search string
	IDs    []int64
	Limit  int
}

// likeEscape escapes LIKE wildcards so a search term is matched literally.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// facetsSearch returns a category's facets filtered by fq: either the given IDs,
// or the most-used matches for a name search (capped). Ordered by book count
// desc so the most relevant/popular entries surface first.
func (a *Adapter) facetsSearch(ctx context.Context, table, linkTable, linkCol string, hasSort bool, fq FacetQuery) ([]Facet, error) {
	sortExpr := "''"
	if hasSort {
		sortExpr = "COALESCE(c.sort, c.name)"
	}
	var where, limit string
	var args []any
	if len(fq.IDs) > 0 {
		ph := make([]string, len(fq.IDs))
		for i, id := range fq.IDs {
			ph[i] = "?"
			args = append(args, id)
		}
		where = "WHERE c.id IN (" + strings.Join(ph, ",") + ")"
	} else {
		if s := strings.TrimSpace(fq.Search); s != "" {
			where = `WHERE c.name LIKE ? ESCAPE '\'`
			args = append(args, "%"+likeEscape(s)+"%")
		}
		n := fq.Limit
		if n <= 0 {
			n = 40
		}
		if n > 500 {
			n = 500
		}
		limit = fmt.Sprintf("LIMIT %d", n)
	}
	q := fmt.Sprintf(`SELECT c.id, c.name, %s, COUNT(l.book)
		FROM %s c JOIN %s l ON l.%s = c.id
		%s GROUP BY c.id ORDER BY COUNT(l.book) DESC, c.name COLLATE NOCASE %s`,
		sortExpr, table, linkTable, linkCol, where, limit)
	return a.facets(ctx, q, args...)
}

// TagsSearch returns tags matching fq (search+cap, or by IDs).
func (a *Adapter) TagsSearch(ctx context.Context, fq FacetQuery) ([]Facet, error) {
	return a.facetsSearch(ctx, "tags", "books_tags_link", "tag", false, fq)
}

// AuthorsSearch returns authors matching fq (search+cap, or by IDs).
func (a *Adapter) AuthorsSearch(ctx context.Context, fq FacetQuery) ([]Facet, error) {
	return a.facetsSearch(ctx, "authors", "books_authors_link", "author", true, fq)
}

// Publishers returns all publishers with at least one book, sorted.
func (a *Adapter) Publishers(ctx context.Context) ([]Facet, error) {
	return a.facets(ctx, facetQuery("publishers", "books_publishers_link", "publisher", true))
}

// Languages returns all languages with at least one book.
func (a *Adapter) Languages(ctx context.Context) ([]Facet, error) {
	return a.facets(ctx, `SELECT l.id, l.lang_code, l.lang_code, COUNT(bll.book)
		FROM languages l JOIN books_languages_link bll ON bll.lang_code = l.id
		GROUP BY l.id ORDER BY l.lang_code`)
}

func (a *Adapter) facets(ctx context.Context, query string, args ...any) ([]Facet, error) {
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("facets: %w", err)
	}
	defer rows.Close()
	out := []Facet{}
	for rows.Next() {
		var f Facet
		var sortIgnored sql.NullString
		if err := rows.Scan(&f.ID, &f.Name, &sortIgnored, &f.Count); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Stats summarises the library.
type Stats struct {
	Books      int `json:"books"`
	Authors    int `json:"authors"`
	Series     int `json:"series"`
	Tags       int `json:"tags"`
	Publishers int `json:"publishers"`
}

// Stats returns library-wide counts.
func (a *Adapter) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	q := `SELECT
		(SELECT COUNT(*) FROM books),
		(SELECT COUNT(*) FROM authors),
		(SELECT COUNT(*) FROM series),
		(SELECT COUNT(*) FROM tags),
		(SELECT COUNT(*) FROM publishers)`
	err := a.db.QueryRowContext(ctx, q).Scan(&s.Books, &s.Authors, &s.Series, &s.Tags, &s.Publishers)
	return s, err
}
