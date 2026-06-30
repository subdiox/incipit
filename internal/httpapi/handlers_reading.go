package httpapi

import (
	"net/http"
	"time"

	"incipit/internal/calibre"
)

// readingItem is a book paired with the current user's reading position, used
// for the "continue reading" / history lists.
type readingItem struct {
	Book       calibre.Book `json:"book"`
	Page       int          `json:"page"`
	TotalPages int          `json:"totalPages"`
	UpdatedAt  time.Time    `json:"updatedAt"`
}

// handleMyReading returns the current user's reading entries, most recently read
// first. ?status=all returns the full history; otherwise only unfinished books
// ("continue reading"). Entries whose book has since been deleted are dropped.
func (s *Server) handleMyReading(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	inProgress := r.URL.Query().Get("status") != "all"
	prog, err := s.store.ListReading(r.Context(), u.ID, inProgress, atoi(r.URL.Query().Get("limit")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list reading")
		return
	}
	ids := make([]int64, 0, len(prog))
	for _, p := range prog {
		ids = append(ids, p.BookID)
	}
	books, err := s.lib().BooksByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load books")
		return
	}
	byID := make(map[int64]calibre.Book, len(books))
	for _, b := range books {
		byID[b.ID] = b
	}
	items := []readingItem{}
	for _, p := range prog {
		b, ok := byID[p.BookID]
		if !ok {
			continue // book deleted since it was read
		}
		items = append(items, readingItem{Book: b, Page: p.Page, TotalPages: p.TotalPages, UpdatedAt: p.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, items)
}

// handleRecentlyRead returns books recently read across the library, anonymized
// (the current user is excluded and no per-user information is exposed).
func (s *Server) handleRecentlyRead(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	ids, err := s.store.RecentlyReadBookIDs(r.Context(), u.ID, atoi(r.URL.Query().Get("limit")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "recently read")
		return
	}
	books, err := s.lib().BooksByIDs(r.Context(), ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load books")
		return
	}
	if books == nil {
		books = []calibre.Book{}
	}
	writeJSON(w, http.StatusOK, books)
}

// handleResetProgress clears the current user's reading position for a book.
func (s *Server) handleResetProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	u := currentUser(r)
	if err := s.store.DeleteProgress(r.Context(), u.ID, b.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "reset progress")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
