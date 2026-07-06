package calibre

import (
	"context"
	"fmt"
)

// SeriesCard is a series collapsed to a single tile: its name, how many volumes
// the library holds, and the first volume for the thumbnail.
type SeriesCard struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	BookCount int    `json:"bookCount"`
	Cover     *Book  `json:"cover,omitempty"` // first volume, for the thumbnail
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

// GroupRef references one grouped unit — a whole series ("series") or a
// standalone book ("book"), by id. It carries a unit ordering computed outside
// the SQL query (e.g. a Go-side per-user ranking) into HydrateUnits.
type GroupRef struct {
	Kind string
	Key  int64
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
// tile (thumbnail = first volume, count = volumes held) and standalone books
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
	var order []GroupRef
	for rows.Next() {
		var ref GroupRef
		if err := rows.Scan(&ref.Kind, &ref.Key); err != nil {
			rows.Close()
			return nil, err
		}
		order = append(order, ref)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	return a.HydrateUnits(ctx, order, total)
}

// HydrateUnits resolves an ordered list of unit references into a page of
// GroupUnits (series cards + standalone books), preserving order; total is the
// full unit count for the caller's pagination. It lets a caller that computed
// the ordering itself — e.g. the per-user "recently read" / "most viewed" sorts,
// which rank in Go over app.db data metadata.db can't see — reuse the same
// hydration as ListGrouped.
func (a *Adapter) HydrateUnits(ctx context.Context, order []GroupRef, total int) (*GroupedResult, error) {
	var seriesIDs, bookIDs []int64
	for _, ref := range order {
		if ref.Kind == "series" {
			seriesIDs = append(seriesIDs, ref.Key)
		} else {
			bookIDs = append(bookIDs, ref.Key)
		}
	}

	summaries, err := a.SeriesSummaries(ctx, seriesIDs)
	if err != nil {
		return nil, err
	}

	// Hydrate every book we need to show: standalone books + each series' first
	// volume (its cover).
	need := append([]int64{}, bookIDs...)
	for _, sid := range seriesIDs {
		if sm, ok := summaries[sid]; ok && sm.FirstBookID != 0 {
			need = append(need, sm.FirstBookID)
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
	for _, ref := range order {
		if ref.Kind == "book" {
			if b := byID[ref.Key]; b != nil {
				units = append(units, GroupUnit{Kind: "book", Book: b})
			}
			continue
		}
		sm, ok := summaries[ref.Key]
		if !ok {
			continue // series no longer present in the library
		}
		card := &SeriesCard{ID: ref.Key, Name: sm.Name, BookCount: sm.BookCount}
		if sm.FirstBookID != 0 {
			card.Cover = byID[sm.FirstBookID]
		}
		units = append(units, GroupUnit{Kind: "series", Series: card})
	}
	return &GroupedResult{Units: units, Total: total}, nil
}

// AllBookSeries maps every book that belongs to a series to its series id, in a
// single scan of the link table (books not in a series are absent). It mirrors
// the whole-table app.db loaders (AllBookLastRead/AllBookViewCounts) so the
// Go-side grouped ranking can group a large filtered id list into series without
// a giant per-id IN clause.
func (a *Adapter) AllBookSeries(ctx context.Context) (map[int64]int64, error) {
	rows, err := a.db.QueryContext(ctx, "SELECT book, series FROM books_series_link")
	if err != nil {
		return nil, fmt.Errorf("all book series: %w", err)
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var book, series int64
		if err := rows.Scan(&book, &series); err != nil {
			return nil, err
		}
		out[book] = series
	}
	return out, rows.Err()
}
