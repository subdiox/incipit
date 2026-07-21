package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"incipit/internal/appdb"
	"incipit/internal/calibre"
)

// Seed weights turn the user's own activity into a taste profile. Favorites are
// the strongest, explicit signal; a finished read is strong; an in-progress read
// scales with how far in they are. Read weights decay with age (recencyTauDays)
// so recent taste dominates. Favorites don't decay (they're a standing choice).
const (
	seedFavorite       = 1.0
	seedFavoriteSeries = 0.7 // per volume of a favorited series
	seedFinished       = 0.8
	seedInProgressMin  = 0.3
	seedInProgressMax  = 0.7
	recencyTauDays     = 90.0

	defaultRecommendLimit = 24  // home shelf: a single scrollable row
	maxRecommendLimit     = 500 // dedicated "For You" page: the full ranked set
	excludeReadScan       = 500 // cap on recent reads scanned for taste seeds (exclusion is uncapped, see ReadBookIDs)

	// Precompute settings: how many recs to cache per user, and when to warm the
	// whole cache once after boot. There is no periodic refresh — the cache is
	// updated per-user, event-driven, when a favorites/history change schedules a
	// background recompute (see markRecommendationsStale).
	recommendPrecomputeLimit = maxRecommendLimit
	recommendWarmDelay       = 30 * time.Second

	// recommendDebounceDelay coalesces a burst of activity changes for one user
	// (turning pages upserts progress on every page) into a single recompute once
	// the activity settles — long enough that engaged reading doesn't re-score the
	// library between page turns, short enough to feel prompt after finishing.
	recommendDebounceDelay = 8 * time.Second
)

// recommendItem is a suggested book plus why it was suggested (for the
// "because you like …" caption). ReasonKind is "author" | "series" | "tag".
type recommendItem struct {
	Book       calibre.Book `json:"book"`
	ReasonKind string       `json:"reasonKind"`
	ReasonName string       `json:"reasonName"`
}

// handleRecommended serves the current user's precomputed recommendations from
// the cache, so the response is instant even on a large library. The cache is
// refreshed in the background a few seconds after their favorites or reading
// history change (markRecommendationsStale), so a just-read book drops off on the
// next load. Empty (200 with []) when the feature is off or the user has no cached
// recs yet, so the UI hides the section.
func (s *Server) handleRecommended(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !s.recommendationsEnabled(ctx) {
		writeJSON(w, http.StatusOK, []recommendItem{})
		return
	}
	u := currentUser(r)

	limit := atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = defaultRecommendLimit
	}
	if limit > maxRecommendLimit {
		limit = maxRecommendLimit
	}

	cached, err := s.store.CachedRecommendations(ctx, u.ID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recommendations")
		return
	}
	ids := make([]int64, 0, len(cached))
	for _, c := range cached {
		ids = append(ids, c.BookID)
	}
	books, err := s.lib().BooksByIDs(ctx, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load books")
		return
	}
	byID := make(map[int64]calibre.Book, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}
	items := []recommendItem{}
	for _, c := range cached {
		b, ok := byID[c.BookID]
		if !ok {
			continue // book deleted since it was cached
		}
		items = append(items, recommendItem{Book: b, ReasonKind: c.ReasonKind, ReasonName: c.ReasonName})
	}
	writeJSON(w, http.StatusOK, items)
}

// recommendForUser builds a user's taste seeds from their favorites and reading
// history and returns freshly scored suggestions. nil (not an error) when the
// user has no activity to seed from.
func (s *Server) recommendForUser(ctx context.Context, userID int64, limit int) ([]calibre.Recommendation, error) {
	seeds, exclude, err := s.recommendSeeds(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(seeds) == 0 {
		return nil, nil
	}
	return s.lib().Recommend(ctx, seeds, exclude, limit)
}

// recommendSeeds gathers a user's taste seeds (book id → weight) and the set of
// books to exclude from suggestions (everything they've already read or shelved).
func (s *Server) recommendSeeds(ctx context.Context, userID int64) (map[int64]float64, map[int64]bool, error) {
	seeds := map[int64]float64{}
	exclude := map[int64]bool{}
	addSeed := func(id int64, wgt float64) {
		if id <= 0 || wgt <= 0 {
			return
		}
		if cur, ok := seeds[id]; !ok || wgt > cur {
			seeds[id] = wgt // strongest signal for a book wins
		}
	}

	// Exclude every book the user has ever opened (uncapped), so a read book is
	// never suggested — even for heavy readers with more than excludeReadScan
	// reads, whose older reads wouldn't appear in the seed scan below.
	readIDs, err := s.store.ReadBookIDs(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	for _, id := range readIDs {
		exclude[id] = true
	}

	// Reading history seeds: the most recent finished/in-progress reads,
	// recency-decayed (bounded to excludeReadScan for the taste profile; full
	// exclusion is handled above).
	reads, err := s.store.ListReading(ctx, userID, appdb.ReadingAll, excludeReadScan)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	for _, p := range reads {
		exclude[p.BookID] = true
		base := seedInProgressMin
		switch {
		case p.TotalPages > 0 && p.Page >= p.TotalPages-1:
			base = seedFinished
		case p.TotalPages > 0:
			frac := float64(p.Page) / float64(p.TotalPages)
			base = seedInProgressMin + (seedInProgressMax-seedInProgressMin)*frac
		}
		addSeed(p.BookID, base*recencyDecay(now, p.UpdatedAt))
	}

	// Favorites: the built-in Favorites shelf's books and series (expanded to
	// their volumes). Also excluded — they already have these.
	if err := s.store.EnsureFavoritesShelf(ctx, userID); err != nil {
		return nil, nil, err
	}
	favID, err := s.store.FavoritesShelfID(ctx, userID)
	if err != nil && !errors.Is(err, appdb.ErrNotFound) {
		return nil, nil, err
	}
	if err == nil {
		favBooks, _ := s.store.ShelfBookIDs(ctx, favID)
		for _, id := range favBooks {
			exclude[id] = true
			addSeed(id, seedFavorite)
		}
		if favSeries, _ := s.store.ShelfSeriesIDs(ctx, favID); len(favSeries) > 0 {
			vols, err := s.lib().BookIDsInSeries(ctx, favSeries)
			if err != nil {
				return nil, nil, err
			}
			for _, id := range vols {
				exclude[id] = true
				addSeed(id, seedFavoriteSeries)
			}
		}
	}
	return seeds, exclude, nil
}

// --- Precompute ---

// startRecommendationWarm warms the recommendation cache once shortly after boot
// so the endpoint serves from cache immediately (the cache also survives restarts
// in app.db). After this, refreshes are event-driven per user — see
// markRecommendationsStale — rather than on a periodic sweep.
func (s *Server) startRecommendationWarm() {
	go func() {
		timer := time.NewTimer(recommendWarmDelay)
		defer timer.Stop()
		<-timer.C
		s.refreshAllRecommendations(context.Background())
	}()
}

// markRecommendationsStale schedules a background recompute of one user's
// recommendations after their favorites or reading history changed. It is
// debounced per user: repeated calls within recommendDebounceDelay collapse into
// a single run once the activity settles (page turns upsert progress on every
// page), and the recompute never runs in the request path — scoring a large
// library takes seconds. A no-op when the library isn't ready or recommendations
// are disabled.
func (s *Server) markRecommendationsStale(userID int64) {
	if userID <= 0 || !s.libraryConfigured() || !s.recommendationsEnabled(context.Background()) {
		return
	}
	s.recDebounceMu.Lock()
	defer s.recDebounceMu.Unlock()
	if t, ok := s.recDebounce[userID]; ok {
		t.Reset(recommendDebounceDelay) // coalesce: push the pending run later
		return
	}
	s.recDebounce[userID] = time.AfterFunc(recommendDebounceDelay, func() {
		s.recDebounceMu.Lock()
		delete(s.recDebounce, userID)
		s.recDebounceMu.Unlock()
		s.computeUserRecommendations(context.Background(), userID)
	})
}

// refreshAllRecommendations recomputes and caches recommendations for every user
// with activity. A no-op when the feature is off, the library isn't ready, or a
// run is already in progress.
func (s *Server) refreshAllRecommendations(ctx context.Context) {
	if !s.libraryConfigured() || !s.recommendationsEnabled(ctx) {
		return
	}
	if !s.recomputing.CompareAndSwap(false, true) {
		return
	}
	defer s.recomputing.Store(false)

	users, err := s.store.UsersWithActivity(ctx)
	if err != nil {
		slog.Error("recommend: list users", "err", err)
		return
	}
	for _, uid := range users {
		s.computeUserRecommendations(ctx, uid)
	}
	slog.Info("recommend: cache refreshed", "users", len(users))
}

// computeUserRecommendations scores and caches one user's recommendations,
// guarded so the warm sweep and an event-driven refresh can't compute the same
// user at once.
func (s *Server) computeUserRecommendations(ctx context.Context, userID int64) {
	if _, busy := s.recInFlight.LoadOrStore(userID, struct{}{}); busy {
		return
	}
	defer s.recInFlight.Delete(userID)

	recs, err := s.recommendForUser(ctx, userID, recommendPrecomputeLimit)
	if err != nil {
		slog.Error("recommend: compute", "user", userID, "err", err)
		return
	}
	cached := make([]appdb.CachedRec, 0, len(recs))
	for _, r := range recs {
		cached = append(cached, appdb.CachedRec{
			BookID: r.BookID, Score: r.Score, ReasonKind: r.ReasonKind, ReasonName: r.ReasonName,
		})
	}
	if err := s.store.ReplaceRecommendations(ctx, userID, cached); err != nil {
		slog.Error("recommend: store", "user", userID, "err", err)
	}
}

// recencyDecay is an exponential decay factor in [0,1] for a read that last
// happened at t: 1 today, ~0.37 after recencyTauDays. An unknown/zero time gives
// full weight rather than dropping the seed.
func recencyDecay(now, t time.Time) float64 {
	if t.IsZero() {
		return 1
	}
	days := now.Sub(t).Hours() / 24
	if days <= 0 {
		return 1
	}
	return math.Exp(-days / recencyTauDays)
}
