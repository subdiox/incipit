package httpapi

import (
	"errors"
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
	// Every user has a built-in Favorites shelf; create it on demand.
	if err := s.store.EnsureFavoritesShelf(r.Context(), u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "ensure favorites")
		return
	}
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

type updateShelfBody struct {
	Name     *string `json:"name"`
	IsPublic *bool   `json:"isPublic"`
}

// handleUpdateShelf renames a shelf and/or changes its visibility (public vs
// private) after creation.
func (s *Server) handleUpdateShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	var body updateShelfBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := sh.Name
	// The built-in Favorites shelf keeps its (localized) name; only its
	// visibility can change.
	if body.Name != nil && !sh.IsDefault {
		name = strings.TrimSpace(*body.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
	}
	isPublic := sh.IsPublic
	if body.IsPublic != nil {
		isPublic = *body.IsPublic
	}
	// The built-in Favorites shelf is always private.
	if sh.IsDefault {
		isPublic = false
	}
	if err := s.store.UpdateShelf(r.Context(), sh.ID, name, isPublic); err != nil {
		if errors.Is(err, appdb.ErrNotFound) {
			writeError(w, http.StatusNotFound, "shelf not found")
			return
		}
		writeError(w, http.StatusConflict, "shelf name already exists")
		return
	}
	updated, err := s.store.GetShelf(r.Context(), sh.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load shelf")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteShelf deletes a shelf the user owns.
func (s *Server) handleDeleteShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	if sh.IsDefault {
		writeError(w, http.StatusForbidden, "the favorites shelf cannot be deleted")
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

type shelfSeriesCard struct {
	ID        int64         `json:"id"`
	Name      string        `json:"name"`
	BookCount int           `json:"bookCount"`
	Cover     *calibre.Book `json:"cover,omitempty"` // first volume, for the card thumbnail
}

type shelfContents struct {
	Series []shelfSeriesCard `json:"series"`
	Books  []calibre.Book    `json:"books"`
}

// handleShelfContents returns a shelf's whole-series entries (as cards that
// expand to their volumes) plus its individual books.
func (s *Server) handleShelfContents(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, false)
	if !ok {
		return
	}
	bookIDs, err := s.store.ShelfBookIDs(r.Context(), sh.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "shelf books")
		return
	}
	books, err := s.lib().BooksByIDs(r.Context(), bookIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load books")
		return
	}
	if books == nil {
		books = []calibre.Book{}
	}

	seriesIDs, err := s.store.ShelfSeriesIDs(r.Context(), sh.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "shelf series")
		return
	}
	sums, err := s.lib().SeriesSummaries(r.Context(), seriesIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series summaries")
		return
	}
	var coverIDs []int64
	for _, sm := range sums {
		if sm.FirstBookID > 0 {
			coverIDs = append(coverIDs, sm.FirstBookID)
		}
	}
	coverBooks, _ := s.lib().BooksByIDs(r.Context(), coverIDs)
	coverByID := make(map[int64]calibre.Book, len(coverBooks))
	for i := range coverBooks {
		coverByID[coverBooks[i].ID] = coverBooks[i]
	}
	cards := make([]shelfSeriesCard, 0, len(seriesIDs))
	for _, sid := range seriesIDs {
		sm, ok := sums[sid]
		if !ok {
			continue // series no longer in the library
		}
		card := shelfSeriesCard{ID: sid, Name: sm.Name, BookCount: sm.BookCount}
		if b, ok := coverByID[sm.FirstBookID]; ok {
			bc := b
			card.Cover = &bc
		}
		cards = append(cards, card)
	}
	writeJSON(w, http.StatusOK, shelfContents{Series: cards, Books: books})
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

func (s *Server) seriesIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "seriesId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid series id")
		return 0, false
	}
	return id, true
}

// handleAddSeriesToShelf adds a whole series to a shelf.
func (s *Server) handleAddSeriesToShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	seriesID, ok := s.seriesIDParam(w, r)
	if !ok {
		return
	}
	if err := s.store.AddSeriesToShelf(r.Context(), sh.ID, seriesID); err != nil {
		writeError(w, http.StatusInternalServerError, "add series to shelf")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "added"})
}

// handleRemoveSeriesFromShelf removes a series from a shelf.
func (s *Server) handleRemoveSeriesFromShelf(w http.ResponseWriter, r *http.Request) {
	sh, ok := s.shelfFromURL(w, r, true)
	if !ok {
		return
	}
	seriesID, ok := s.seriesIDParam(w, r)
	if !ok {
		return
	}
	if err := s.store.RemoveSeriesFromShelf(r.Context(), sh.ID, seriesID); err != nil {
		writeError(w, http.StatusInternalServerError, "remove series from shelf")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
