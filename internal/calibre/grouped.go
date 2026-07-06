package calibre

import (
	"context"
	"fmt"
)

// SeriesCard is a series collapsed to a single tile: its name, how many volumes
// the library holds, and the latest volume for the thumbnail.
type SeriesCard struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
	Cover     *Book  `json:"cover,omitempty"` // latest volume, for the thumbnail
}

// GroupUnit is one grid cell in the grouped view: either a whole series or a
// standalone (series-less) book.
type GroupUnit struct {
	Kind   string      `json:"kind"` // "series" | "book"
	Book   *Book       `json:"book,omitempty"`
	Series *SeriesCard `json:"series,omitempty"`
}

// GroupedResult is a page of grouped units plus the total unit count.
type GroupedResult struct {
	Units []GroupUnit `json:"units"`
	Total int         `json:"total"`
}

// groupedSortExprs returns the per-unit sort expressions for the series subquery
// (an aggregate over the series' volumes) and the standalone-book subquery, plus
// the direction. Sorts that can't be expressed here (views/lastread, per-user)
// fall back to name order.
func groupedSortExprs(opts ListOptions) (seriesExpr, bookExpr, dir string) {
	dir = "ASC"
	if opts.Desc {
		dir = "DESC"
	}
	const rating = "(SELECT r.rating FROM books_ratings_link brl JOIN ratings r ON r.id=brl.rating WHERE brl.book=b.id)"
	switch opts.Sort {
	case "timestamp":
		return "MAX(b.timestamp)", "b.timestamp", dir
	case "pubdate":
		return "MAX(b.pubdate)", "b.pubdate", dir
	case "rating":
		return "MAX(" + rating + ")", rating, dir
	case "author":
		return "MIN(b.author_sort)", "b.author_sort", dir
	default: // title / series / views / lastread → by name
		return "s.sort", "b.sort", dir
	}
}

// ListGrouped returns a page of the library with each series collapsed to one
// tile (thumbnail = latest volume, count = volumes held) and standalone books
// shown individually. The same filter as ListBooks applies: a series appears
// when any of its volumes match. Sorting is over the unit (e.g. a series sorts by
// its most-recent volume for "recently added").
func (a *Adapter) ListGrouped(ctx context.Context, opts ListOptions) (*GroupedResult, error) {
	where, wargs := a.buildFilters(opts)
	const notInSeries = "NOT EXISTS (SELECT 1 FROM books_series_link l WHERE l.book=b.id)"
	soloWhere := " WHERE " + notInSeries
	if where != "" {
		soloWhere = where + " AND " + notInSeries
	}

	// Total units = distinct matching series + matching standalone books.
	countQ := "SELECT (SELECT COUNT(DISTINCT bsl.series) FROM books b JOIN books_series_link bsl ON bsl.book=b.id" + where + ")" +
		" + (SELECT COUNT(*) FROM books b" + soloWhere + ")"
	countArgs := append(append([]any{}, wargs...), wargs...)
	var total int
	if err := a.db.QueryRowContext(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count grouped: %w", err)
	}

	seriesSort, bookSort, dir := groupedSortExprs(opts)
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	// Series and standalone units share a sort value column so the page can be
	// ordered and sliced in one query. Book ids and series ids can collide, so the
	// kind is kept alongside the key.
	unionQ := fmt.Sprintf(`SELECT kind, key FROM (
		SELECT 'series' AS kind, s.id AS key, %s AS sortval
		FROM books b JOIN books_series_link bsl ON bsl.book=b.id JOIN series s ON s.id=bsl.series%s
		GROUP BY s.id
		UNION ALL
		SELECT 'book' AS kind, b.id AS key, %s AS sortval
		FROM books b%s
	) ORDER BY sortval %s, kind %s, key %s LIMIT ? OFFSET ?`,
		seriesSort, where, bookSort, soloWhere, dir, dir, dir)
	args := append(append([]any{}, wargs...), wargs...)
	args = append(args, limit, opts.Offset)

	rows, err := a.db.QueryContext(ctx, unionQ, args...)
	if err != nil {
		return nil, fmt.Errorf("list grouped: %w", err)
	}
	type unit struct {
		kind string
		key  int64
	}
	var order []unit
	var seriesIDs, bookIDs []int64
	for rows.Next() {
		var u unit
		if err := rows.Scan(&u.kind, &u.key); err != nil {
			rows.Close()
			return nil, err
		}
		order = append(order, u)
		if u.kind == "series" {
			seriesIDs = append(seriesIDs, u.key)
		} else {
			bookIDs = append(bookIDs, u.key)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	summaries, err := a.SeriesSummaries(ctx, seriesIDs)
	if err != nil {
		return nil, err
	}

	// Hydrate every book we need to show: standalone books + each series' latest
	// volume (its cover).
	need := append([]int64{}, bookIDs...)
	for _, sid := range seriesIDs {
		if sm, ok := summaries[sid]; ok && sm.LastBookID != 0 {
			need = append(need, sm.LastBookID)
		}
	}
	byID := map[int64]*Book{}
	if len(need) > 0 {
		bs, err := a.BooksByIDs(ctx, need)
		if err != nil {
			return nil, err
		}
		for i := range bs {
			byID[bs[i].ID] = &bs[i]
		}
	}

	units := make([]GroupUnit, 0, len(order))
	for _, u := range order {
		if u.kind == "book" {
			if b := byID[u.key]; b != nil {
				units = append(units, GroupUnit{Kind: "book", Book: b})
			}
			continue
		}
		sm := summaries[u.key]
		card := &SeriesCard{ID: u.key, Name: sm.Name, BookCount: sm.BookCount}
		if sm.LastBookID != 0 {
			card.Cover = byID[sm.LastBookID]
		}
		units = append(units, GroupUnit{Kind: "series", Series: card})
	}
	return &GroupedResult{Units: units, Total: total}, nil
}
