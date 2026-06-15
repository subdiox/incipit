package httpapi

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"incipit/internal/calibre"
	"incipit/internal/reader"
)

const maxUpload = 1 << 30 // 1 GiB

// handleAddBook imports an uploaded CBZ with metadata from a multipart form.
func (s *Server) handleAddBook(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.CanUpload {
		writeError(w, http.StatusForbidden, "upload not permitted")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload")
		return
	}
	defer r.MultipartForm.RemoveAll()

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	format := strings.ToUpper(strings.TrimPrefix(extOf(header.Filename), "."))
	if format != "CBZ" {
		writeError(w, http.StatusBadRequest, "only CBZ uploads are supported")
		return
	}

	// Persist to a temp file so we can both generate a cover and stream into the
	// library without buffering the whole archive in memory.
	tmp, err := os.CreateTemp("", "incipit-upload-*.cbz")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "temp file")
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		writeError(w, http.StatusInternalServerError, "save upload")
		return
	}
	tmp.Close()

	var cover []byte
	if data, err := s.reader.FirstPageJPEG(tmpName, 600); err == nil {
		cover = data
	} else if !errors.Is(err, reader.ErrPageOutOfRange) {
		// A malformed archive should be rejected early.
		writeError(w, http.StatusBadRequest, "not a valid CBZ archive")
		return
	}

	dataFile, err := os.Open(tmpName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reopen upload")
		return
	}
	defer dataFile.Close()

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, extOf(header.Filename))
	}

	in := calibre.AddBookInput{
		Title:       title,
		Authors:     splitList(r.FormValue("authors")),
		Series:      strings.TrimSpace(r.FormValue("series")),
		SeriesIndex: parseFloat(r.FormValue("seriesIndex")),
		Tags:        splitList(r.FormValue("tags")),
		Publisher:   strings.TrimSpace(r.FormValue("publisher")),
		Languages:   splitList(r.FormValue("languages")),
		Rating:      atoi(r.FormValue("rating")),
		Comments:    r.FormValue("comments"),
		Format:      "CBZ",
		Data:        dataFile,
		Cover:       cover,
	}
	if pd := r.FormValue("pubdate"); pd != "" {
		if t, err := time.Parse("2006-01-02", pd); err == nil {
			in.PubDate = t
		}
	}

	book, err := s.lib.AddBook(r.Context(), in)
	if err != nil {
		if errors.Is(err, calibre.ErrReadOnly) {
			writeError(w, http.StatusForbidden, "library is read-only")
			return
		}
		writeError(w, http.StatusInternalServerError, "add book: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, book)
}

type updateBookBody struct {
	Title       *string            `json:"title"`
	Authors     *[]string          `json:"authors"`
	Series      *string            `json:"series"`
	SeriesIndex *float64           `json:"seriesIndex"`
	Tags        *[]string          `json:"tags"`
	Publisher   *string            `json:"publisher"`
	Languages   *[]string          `json:"languages"`
	Rating      *int               `json:"rating"`
	Comments    *string            `json:"comments"`
	Identifiers *map[string]string `json:"identifiers"`
	PubDate     *string            `json:"pubdate"`
}

// handleUpdateBook edits a book's metadata.
func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.CanEdit {
		writeError(w, http.StatusForbidden, "editing not permitted")
		return
	}
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	var body updateBookBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in := calibre.UpdateBookInput{
		Title:       body.Title,
		Authors:     body.Authors,
		Series:      body.Series,
		SeriesIndex: body.SeriesIndex,
		Tags:        body.Tags,
		Publisher:   body.Publisher,
		Languages:   body.Languages,
		Rating:      body.Rating,
		Comments:    body.Comments,
		Identifiers: body.Identifiers,
	}
	if body.PubDate != nil {
		if t, err := time.Parse("2006-01-02", *body.PubDate); err == nil {
			in.PubDate = &t
		}
	}
	updated, err := s.lib.UpdateBook(r.Context(), b.ID, in)
	if err != nil {
		if errors.Is(err, calibre.ErrReadOnly) {
			writeError(w, http.StatusForbidden, "library is read-only")
			return
		}
		writeError(w, http.StatusInternalServerError, "update book: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteBook removes a book and its files.
func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.CanEdit {
		writeError(w, http.StatusForbidden, "deleting not permitted")
		return
	}
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	if err := s.lib.DeleteBook(r.Context(), b.ID); err != nil {
		if errors.Is(err, calibre.ErrReadOnly) {
			writeError(w, http.StatusForbidden, "library is read-only")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete book")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		return name[i:]
	}
	return ""
}
