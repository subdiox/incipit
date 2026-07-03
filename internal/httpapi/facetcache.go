package httpapi

import (
	"context"
	"net/http"
	"sync"
	"time"

	"incipit/internal/calibre"
)

// defaultFacetLimit is how many "most-used" entries the default (no-search) facet
// list returns.
const defaultFacetLimit = 40

// facetCacheTTL is how long a cached default facet list is served before a
// background refresh is triggered.
const facetCacheTTL = 10 * time.Minute

type facetEntry struct {
	data       []calibre.Facet
	at         time.Time
	refreshing bool
}

// facetCache memoises the default (most-used) facet lists. Computing them ranks
// every author/tag by book count — seconds of CPU on a 100k+-row category — so
// it must never run inline per request. A stale entry is served immediately
// while it refreshes in the background (stale-while-revalidate); only the very
// first miss computes inline.
type facetCache struct {
	mu sync.Mutex
	m  map[string]*facetEntry
}

func newFacetCache() *facetCache { return &facetCache{m: map[string]*facetEntry{}} }

func (c *facetCache) get(key string, load func() ([]calibre.Facet, error)) ([]calibre.Facet, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.m[key]
	if e != nil {
		if time.Since(e.at) < facetCacheTTL {
			return e.data, nil
		}
		// Stale: hand back the stale list now, refresh once in the background.
		if !e.refreshing {
			e.refreshing = true
			go c.refresh(key, load)
		}
		return e.data, nil
	}
	// First-ever miss: compute inline. Holding the lock serialises a concurrent
	// burst so only one request pays the cost.
	data, err := load()
	if err != nil {
		return nil, err
	}
	c.m[key] = &facetEntry{data: data, at: time.Now()}
	return data, nil
}

// clear drops all cached facet lists, so the next request recomputes. Called
// after a book write, since adds/edits/deletes change author/tag membership.
func (c *facetCache) clear() {
	c.mu.Lock()
	c.m = map[string]*facetEntry{}
	c.mu.Unlock()
}

func (c *facetCache) refresh(key string, load func() ([]calibre.Facet, error)) {
	data, err := load()
	c.mu.Lock()
	if e := c.m[key]; e != nil {
		if err == nil {
			e.data = data
			e.at = time.Now()
		}
		e.refreshing = false
	}
	c.mu.Unlock()
}

// warm precomputes an entry in the background (used at startup) so the first
// request is already served from cache.
func (c *facetCache) warm(key string, load func() ([]calibre.Facet, error)) {
	go func() { _, _ = c.get(key, load) }()
}

// serveFacet answers a searchable-facet request: the default (no search, no ids)
// list is served from the cache; a search or id-lookup runs live (both are fast).
func (s *Server) serveFacet(
	w http.ResponseWriter,
	r *http.Request,
	name string,
	search func(context.Context, calibre.FacetQuery) ([]calibre.Facet, error),
) {
	fq := parseFacetQuery(r)
	var facets []calibre.Facet
	var err error
	if fq.Search == "" && len(fq.IDs) == 0 {
		facets, err = s.facets.get(name, func() ([]calibre.Facet, error) {
			return search(context.Background(), calibre.FacetQuery{Limit: defaultFacetLimit})
		})
	} else {
		facets, err = search(r.Context(), fq)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load facets")
		return
	}
	if facets == nil {
		facets = []calibre.Facet{}
	}
	writeJSON(w, http.StatusOK, facets)
}

// warmFacets precomputes the default author/tag lists in the background.
func (s *Server) warmFacets() {
	if s.lib() == nil {
		return
	}
	s.facets.warm("tags", func() ([]calibre.Facet, error) {
		return s.lib().TagsSearch(context.Background(), calibre.FacetQuery{Limit: defaultFacetLimit})
	})
	s.facets.warm("authors", func() ([]calibre.Facet, error) {
		return s.lib().AuthorsSearch(context.Background(), calibre.FacetQuery{Limit: defaultFacetLimit})
	})
}
