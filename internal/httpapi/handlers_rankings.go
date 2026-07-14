package httpapi

import (
	"net/http"

	"incipit/internal/calibre"
)

// handleListRankings returns the configured ranking lists (tab order, labels and
// current counts) for the SPA's Rankings section. Empty when the feature is off
// or the library has no rankings, so the section simply doesn't render.
func (s *Server) handleListRankings(w http.ResponseWriter, r *http.Request) {
	if !s.rankingsEnabled(r.Context()) {
		writeJSON(w, http.StatusOK, []calibre.RankingList{})
		return
	}
	lists, err := s.lib().RankingLists(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rankings")
		return
	}
	if lists == nil {
		lists = []calibre.RankingList{}
	}
	writeJSON(w, http.StatusOK, lists)
}
