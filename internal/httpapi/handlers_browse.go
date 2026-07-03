package httpapi

import (
	"context"
	"net/http"
	"strings"

	"incipit/internal/calibre"
)

// parseFacetQuery reads the search/ids/limit params for a searchable facet.
func parseFacetQuery(r *http.Request) calibre.FacetQuery {
	q := r.URL.Query()
	fq := calibre.FacetQuery{Search: q.Get("q"), Limit: atoi(q.Get("limit"))}
	if ids := strings.TrimSpace(q.Get("ids")); ids != "" {
		fq.IDs = atoi64s(strings.Split(ids, ","))
	}
	return fq
}

func (s *Server) facetHandler(load func(*calibre.Adapter, *http.Request) ([]calibre.Facet, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		facets, err := load(s.lib(), r)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load facets")
			return
		}
		if facets == nil {
			facets = []calibre.Facet{}
		}
		writeJSON(w, http.StatusOK, facets)
	}
}

func (s *Server) handleAuthors(w http.ResponseWriter, r *http.Request) {
	s.serveFacet(w, r, "authors", func(ctx context.Context, fq calibre.FacetQuery) ([]calibre.Facet, error) {
		return s.lib().AuthorsSearch(ctx, fq)
	})
}

func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.SeriesList(r.Context())
	})(w, r)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	s.serveFacet(w, r, "tags", func(ctx context.Context, fq calibre.FacetQuery) ([]calibre.Facet, error) {
		return s.lib().TagsSearch(ctx, fq)
	})
}

func (s *Server) handlePublishers(w http.ResponseWriter, r *http.Request) {
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.Publishers(r.Context())
	})(w, r)
}

func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.Languages(r.Context())
	})(w, r)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.lib().Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
