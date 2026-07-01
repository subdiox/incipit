package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
)

type paneBody struct {
	Name     string  `json:"name"`
	TagIDs   []int64 `json:"tagIds"`
	MatchAny bool    `json:"matchAny"`
	Position int     `json:"position"`
}

// handleListPanes returns all admin-defined panes (visible to every user, for
// the nav under Library).
func (s *Server) handleListPanes(w http.ResponseWriter, r *http.Request) {
	panes, err := s.store.ListPanes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list panes")
		return
	}
	writeJSON(w, http.StatusOK, panes)
}

func (s *Server) handleCreatePane(w http.ResponseWriter, r *http.Request) {
	var body paneBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	pane, err := s.store.CreatePane(r.Context(), name, body.TagIDs, body.MatchAny)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create pane")
		return
	}
	writeJSON(w, http.StatusOK, pane)
}

func (s *Server) handleUpdatePane(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body paneBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.store.UpdatePane(r.Context(), id, name, body.TagIDs, body.MatchAny, body.Position); err != nil {
		if errors.Is(err, appdb.ErrNotFound) {
			writeError(w, http.StatusNotFound, "pane not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "update pane")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeletePane(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.store.DeletePane(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete pane")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
