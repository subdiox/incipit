package httpapi

import (
	"context"
	"log/slog"
	"net/http"
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
	s.indexTotal.Store(int64(len(ids)))
	s.indexDone.Store(0)
	slog.Info("page index: starting", "books", len(ids))

	const batch = 200
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
		if books, err := lib.BooksByIDs(ctx, ids[i:end]); err == nil {
			for j := range books {
				_, _ = s.cbzPagesCtx(ctx, &books[j])
			}
		}
		s.indexDone.Add(int64(end - i))
		time.Sleep(20 * time.Millisecond) // yield I/O between batches
	}
	slog.Info("page index: done", "books", len(ids))
}

type pageIndexStatus struct {
	Enabled bool  `json:"enabled"`
	Running bool  `json:"running"`
	Done    int64 `json:"done"`
	Total   int64 `json:"total"`
}

// handlePageIndexStatus reports background page-index progress (admin).
func (s *Server) handlePageIndexStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, pageIndexStatus{
		Enabled: s.pageFilterEnabled(r.Context()),
		Running: s.indexing.Load(),
		Done:    s.indexDone.Load(),
		Total:   s.indexTotal.Load(),
	})
}

// handleReindexPages starts a fresh page-count scan (admin), e.g. to pick up
// newly-added books. No-op if the filter is disabled or a run is in progress.
func (s *Server) handleReindexPages(w http.ResponseWriter, r *http.Request) {
	s.startPageIndex()
	writeJSON(w, http.StatusOK, pageIndexStatus{
		Enabled: s.pageFilterEnabled(r.Context()),
		Running: s.indexing.Load(),
		Done:    s.indexDone.Load(),
		Total:   s.indexTotal.Load(),
	})
}
