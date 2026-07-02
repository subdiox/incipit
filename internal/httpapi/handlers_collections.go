package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
)

type collectionBody struct {
	Name     string  `json:"name"`
	TagIDs   []int64 `json:"tagIds"`
	MatchAny bool    `json:"matchAny"`
	Position int     `json:"position"`
}

// handleListCollections returns all admin-defined collections (visible to every user, for
// the nav under Library).
func (s *Server) handleListCollections(w http.ResponseWriter, r *http.Request) {
	collections, err := s.store.ListCollections(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list collections")
		return
	}
	writeJSON(w, http.StatusOK, collections)
}

func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	var body collectionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	collection, err := s.store.CreateCollection(r.Context(), name, body.TagIDs, body.MatchAny)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create collection")
		return
	}
	writeJSON(w, http.StatusOK, collection)
}

func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	var body collectionBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := s.store.UpdateCollection(r.Context(), id, name, body.TagIDs, body.MatchAny, body.Position); err != nil {
		if errors.Is(err, appdb.ErrNotFound) {
			writeError(w, http.StatusNotFound, "collection not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "update collection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderCollections(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.ReorderCollections(r.Context(), body.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, "reorder collections")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err := s.store.DeleteCollection(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete collection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
