package httpapi

import (
	"context"
	"log/slog"
	"time"

	"incipit/internal/calibre"
)

// PageFilterKey is the app.db settings key toggling the page-count filter. When
// on, the library can be filtered by min/max page count and a background index
// keeps every CBZ's page count in app.db.
const PageFilterKey = "page_filter"

func (s *Server) pageFilterEnabled(ctx context.Context) bool {
	v, _ := s.store.GetSetting(ctx, PageFilterKey)
	return v == "true"
}

// startPageIndex kicks off the background page-count index if the filter is
// enabled and a library is open. Safe to call repeatedly — a run in progress is
// not duplicated.
func (s *Server) startPageIndex() {
	if s.lib() == nil || !s.pageFilterEnabled(context.Background()) {
		return
	}
	go s.indexPageCounts(context.Background())
}

// indexPageCounts scans every CBZ's page count into app.db (via cbzPagesCtx,
// which caches). It's incremental: already-cached books are a quick lookup, so
// re-runs and restarts are cheap. Serial + lightly throttled to avoid saturating
// disk on the first pass over a large library.
func (s *Server) indexPageCounts(ctx context.Context) {
	if !s.indexing.CompareAndSwap(false, true) {
		return // already running
	}
	defer s.indexing.Store(false)

	lib := s.lib()
	if lib == nil {
		return
	}
	ids, err := lib.FilteredIDs(ctx, calibre.ListOptions{})
	if err != nil {
		slog.Error("page index: list ids", "err", err)
		return
	}
	slog.Info("page index: starting", "books", len(ids))

	const batch = 200
	scanned := 0
	for i := 0; i < len(ids); i += batch {
		select {
		case <-ctx.Done():
			return
		default:
		}
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		books, err := lib.BooksByIDs(ctx, ids[i:end])
		if err != nil {
			continue
		}
		for j := range books {
			if _, err := s.cbzPagesCtx(ctx, &books[j]); err == nil {
				scanned++
			}
		}
		time.Sleep(20 * time.Millisecond) // yield I/O between batches
	}
	slog.Info("page index: done", "indexed", scanned)
}
