package httpapi

import (
	"net/http"

	"incipit/internal/calibre"
)

func (s *Server) facetHandler(load func(*calibre.Adapter, *http.Request) ([]calibre.Facet, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		facets, err := load(s.lib, r)
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
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.Authors(r.Context())
	})(w, r)
}

func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.SeriesList(r.Context())
	})(w, r)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	s.facetHandler(func(a *calibre.Adapter, r *http.Request) ([]calibre.Facet, error) {
		return a.Tags(r.Context())
	})(w, r)
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
	stats, err := s.lib.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
