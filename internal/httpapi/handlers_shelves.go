package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
	"incipit/internal/calibre"
)

// handleListShelves returns shelves visible to the user.
func (s *Server) handleListShelves(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	shelves, err := s.store.ListShelves(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list shelves")
		return
	}
	if shelves == nil {
		shelves = []appdb.Shelf{}
	}
	writeJSON(w, http.StatusOK, shelves)
}

type createShelfBody struct {
	Name     string `json:"name"`
	IsPublic bool   `json:"isPublic"`
}

// handleCreateShelf creates a shelf owned by the user.
func (s *Server) handleCreateShelf(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var body createShelfBody
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	sh, err := s.store.CreateShelf(r.Context(), appdb.Shelf{
		UserID: u.ID, Name: strings.TrimSpace(body.Name), IsPublic: body.IsPublic,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "shelf already exists")
		return
	}
	writeJSON(w, http.StatusCreated, sh)
}

// shelfFromURL loads a shelf and verifies the user may modify it.
func (s *Server) shelfFromURL(w http.ResponseWriter, r *http.Request, requireOwner bool) (*appdb.Shelf, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid shelf id")
		return nil, false
	}
	sh, err := s.store.GetShelf(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "shelf not found")
		return nil, false
	}
	u := currentUser(r)
	if requireOwner && sh.UserID != u.ID && !u.IsAdmin {
		writeError(w, http.StatusForbidden, "not your shelf")
		return nil, false
	}
	if !requireOwner && sh.UserID != u.ID && !sh.IsPublic && !u.IsAdmin {
		writeError(w, http.StatusForbidden, "not visible")
		return nil, false
	}
	return sh, true
}

// handleDeleteShelf deletes a shelf the user owns.
func (s *Server) handleDeleteShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	if err := s.store.DeleteShelf(r.Context(), sh.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete shelf")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleShelfBooks returns the hydrated books on a shelf.
func (s *Server) handleShelfBooks(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, false)
	if !ok {
		return
	}
	ids, err := s.store.ShelfBookIDs(r.Context(), sh.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "shelf books")
		return
	}
	books := make([]calibre.Book, 0, len(ids))
	for _, id := range ids {
		if b, err := s.lib().GetBook(r.Context(), id); err == nil {
			books = append(books, *b)
		}
	}
	writeJSON(w, http.StatusOK, calibre.ListResult{Books: books, Total: len(books)})
}

func (s *Server) bookIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "bookId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return 0, false
	}
	return id, true
}

// handleAddToShelf adds a book to a shelf.
func (s *Server) handleAddToShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	bookID, ok := s.bookIDParam(w, r)
	if !ok {
		return
	}
	if _, err := s.lib().GetBook(r.Context(), bookID); err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}
	if err := s.store.AddBookToShelf(r.Context(), sh.ID, bookID); err != nil {
		writeError(w, http.StatusInternalServerError, "add to shelf")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// handleRemoveFromShelf removes a book from a shelf.
func (s *Server) handleRemoveFromShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	bookID, ok := s.bookIDParam(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveBookFromShelf(r.Context(), sh.ID, bookID); err != nil {
		writeError(w, http.StatusInternalServerError, "remove from shelf")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
