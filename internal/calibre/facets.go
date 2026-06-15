package calibre

import (
	"context"
	"database/sql"
	"fmt"
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

func (a *Adapter) facets(ctx context.Context, query string) ([]Facet, error) {
	rows, err := a.db.QueryContext(ctx, query)
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
