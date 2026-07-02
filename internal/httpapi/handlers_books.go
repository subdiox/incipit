package httpapi

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
	"incipit/internal/calibre"
	"incipit/internal/reader"
)

func (s *Server) bookFromURL(w http.ResponseWriter, r *http.Request) (*calibre.Book, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return nil, false
	}
	b, err := s.lib().GetBook(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return nil, false
	}
	return b, true
}

// handleListBooks returns a paginated, filterable, sortable book list.
func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := calibre.ListOptions{
		Search:      q.Get("search"),
		Sort:        q.Get("sort"),
		Desc:        q.Get("order") == "desc",
		AuthorID:    atoi64(q.Get("author")),
		SeriesID:    atoi64(q.Get("series")),
		TagIDs:      atoi64s(q["tag"]),    // repeated ?tag= → AND filter
		AnyTagIDs:   atoi64s(q["anyTag"]), // repeated ?anyTag= → OR group ("match any" collection)
		PublisherID: atoi64(q.Get("publisher")),
		Language:    q.Get("language"),
		Limit:       atoi(q.Get("limit")),
		Offset:      atoi(q.Get("offset")),
	}
	// The normal path lets metadata.db do the filtering, sorting and pagination.
	// Two things it can't express fall back to the "ID path": sorts ranked by
	// app.db data (view count, last-read) and the page-count filter (page counts
	// live in app.db). The ID path pulls the matching IDs (already SQL-sorted for
	// ordinary sorts), re-sorts / filters in Go, then hydrates one page.
	minPages := atoi(q.Get("minPages"))
	maxPages := atoi(q.Get("maxPages"))
	pageFiltered := (minPages > 0 || maxPages > 0) && s.pageFilterEnabled(r.Context())
	ranked := opts.Sort == "views" || opts.Sort == "lastread"
	if !ranked && !pageFiltered {
		res, err := s.lib().ListBooks(r.Context(), opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list books")
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}

	ids, err := s.lib().FilteredIDs(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list books")
		return
	}
	switch opts.Sort {
	case "views":
		views, err := s.store.AllBookViewCounts(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "view counts")
			return
		}
		sortIDsStable(ids, opts.Desc, func(a, b int64) bool { return views[a] > views[b] })
	case "lastread":
		last, err := s.store.AllBookLastRead(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "last read")
			return
		}
		sortIDsStable(ids, opts.Desc, func(a, b int64) bool { return last[a].After(last[b]) })
	}
	if pageFiltered {
		counts, err := s.store.AllPageCounts(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "page counts")
			return
		}
		ids = filterIDsByPages(ids, counts, minPages, maxPages)
	}
	s.writeBookPage(w, r, opts, ids)
}

// sortIDsStable sorts ids in place by a "a before b" comparator (descending
// intent); ascending inverts it. Stable, so equal keys keep FilteredIDs' order.
func sortIDsStable(ids []int64, desc bool, before func(a, b int64) bool) {
	sort.SliceStable(ids, func(i, j int) bool {
		if desc {
			return before(ids[i], ids[j])
		}
		return before(ids[j], ids[i])
	})
}

// filterIDsByPages keeps only books whose indexed page count is within
// [min, max] (0 = open bound). Books with no indexed count are dropped.
func filterIDsByPages(ids []int64, counts map[int64]int, min, max int) []int64 {
	var out []int64
	for _, id := range ids {
		c, ok := counts[id]
		if !ok || (min > 0 && c < min) || (max > 0 && c > max) {
			continue
		}
		out = append(out, id)
	}
	return out
}

// writeBookPage paginates an ordered ID list and hydrates just the page.
func (s *Server) writeBookPage(w http.ResponseWriter, r *http.Request, opts calibre.ListOptions, ids []int64) {
	total := len(ids)
	limit := opts.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	start := opts.Offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	books, err := s.lib().BooksByIDs(r.Context(), ids[start:end])
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load books")
		return
	}
	if books == nil {
		books = []calibre.Book{}
	}
	writeJSON(w, http.StatusOK, calibre.ListResult{Books: books, Total: total})
}

// handleGetBook returns one fully-hydrated book.
func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// handleCover serves the book's full cover.jpg.
func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	if !b.HasCover {
		writeError(w, http.StatusNotFound, "no cover")
		return
	}
	coverPath := filepath.Join(s.lib().BookFolder(b), "cover.jpg")
	f, err := os.Open(coverPath)
	if err != nil {
		writeError(w, http.StatusNotFound, "no cover")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cover stat")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "cover.jpg", info.ModTime(), f)
}

// handleThumbnail serves a small cover thumbnail, generating one from the cover
// or the first CBZ page when necessary.
func (s *Server) handleThumbnail(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	width := clampWidth(atoi(r.URL.Query().Get("w")), 400)

	// Prefer the stored cover.jpg (so cover edits — uploaded or fetched from
	// cmoa — actually show), falling back to the CBZ first page when a book has
	// no stored cover.
	if b.HasCover {
		coverPath := filepath.Join(s.lib().BookFolder(b), "cover.jpg")
		if raw, err := os.ReadFile(coverPath); err == nil {
			if data, err := s.reader.ResizeImageToJPEG(raw, width); err == nil {
				serveCachedBytes(w, r, "image/jpeg", data, "")
				return
			}
			// Undecodable cover.jpg: serve it as-is rather than failing.
			serveCachedBytes(w, r, "image/jpeg", raw, "")
			return
		}
	}
	if cbz, _, _, err := s.resolveCBZ(b); err == nil {
		if data, err := s.reader.FirstPageJPEG(cbz, width); err == nil {
			serveCachedBytes(w, r, "image/jpeg", data, "")
			return
		}
	}
	writeError(w, http.StatusNotFound, "no thumbnail")
}

// handleDownload streams the original CBZ file as an attachment.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.CanDownload {
		writeError(w, http.StatusForbidden, "download not permitted")
		return
	}
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	path, format, err := s.resolvePrimaryFile(b)
	if err != nil {
		writeError(w, http.StatusNotFound, "no downloadable file")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file missing")
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	filename := sanitizeFilename(b.Title) + "." + strings.ToLower(format)
	ct := formatContentType[format]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	http.ServeContent(w, r, filename, info.ModTime(), f)
}

// handleContent streams a book's primary file inline for in-browser reading
// (PDF/EPUB). Unlike handleDownload it requires only authentication, not the
// download permission, and uses an inline disposition + range support.
func (s *Server) handleContent(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	path, format, err := s.resolvePrimaryFile(b)
	if err != nil {
		writeError(w, http.StatusNotFound, "no readable file")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "file missing")
		return
	}
	defer f.Close()
	info, _ := f.Stat()
	ct := formatContentType[format]
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, "book."+strings.ToLower(format), info.ModTime(), f)
}

// resolvePrimaryFile returns the absolute path and format of a book's file
// (preferring CBZ when present, else the first available format).
func (s *Server) resolvePrimaryFile(b *calibre.Book) (path, format string, err error) {
	if len(b.Formats) == 0 {
		return "", "", os.ErrNotExist
	}
	chosen := b.Formats[0]
	for _, f := range b.Formats {
		if strings.EqualFold(f.Format, "CBZ") {
			chosen = f
			break
		}
	}
	p := filepath.Join(s.lib().BookFolder(b), chosen.FormatFile())
	if _, serr := os.Stat(p); serr != nil {
		return "", "", serr
	}
	return p, strings.ToUpper(chosen.Format), nil
}

// handlePageList returns the page count (and names) of the CBZ, using the cache.
func (s *Server) handlePageList(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	pages, err := s.cbzPages(r, b)
	if err != nil {
		writeError(w, http.StatusNotFound, "no readable pages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(pages), "pages": pages})
}

// handlePage serves a single CBZ page image, optionally resized (?w=).
func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	n, err := strconv.Atoi(chi.URLParam(r, "n"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid page number")
		return
	}
	cbz, mtime, _, err := s.resolveCBZ(b)
	if err != nil {
		writeError(w, http.StatusNotFound, "no readable file")
		return
	}
	pages, err := s.cbzPages(r, b)
	if err != nil {
		writeError(w, http.StatusNotFound, "no readable pages")
		return
	}
	width := clampWidth(atoi(r.URL.Query().Get("w")), 0)
	page, err := s.reader.RenderPage(cbz, pages, n, width, mtime)
	if err != nil {
		if errors.Is(err, reader.ErrPageOutOfRange) {
			writeError(w, http.StatusNotFound, "page out of range")
			return
		}
		writeError(w, http.StatusInternalServerError, "render page")
		return
	}
	serveCachedBytes(w, r, page.ContentType, page.Data, page.ETag)
}

// handleGetProgress returns the user's reading position.
func (s *Server) handleGetProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	u := currentUser(r)
	p, err := s.store.GetProgress(r.Context(), u.ID, b.ID, "CBZ")
	if err != nil {
		if errors.Is(err, appdb.ErrNotFound) {
			writeJSON(w, http.StatusOK, appdb.Progress{BookID: b.ID, Format: "CBZ"})
			return
		}
		writeError(w, http.StatusInternalServerError, "get progress")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type progressBody struct {
	Page       int `json:"page"`
	TotalPages int `json:"totalPages"`
}

// handleSetProgress upserts the user's reading position.
func (s *Server) handleSetProgress(w http.ResponseWriter, r *http.Request) {
	b, ok := s.bookFromURL(w, r)
	if !ok {
		return
	}
	var body progressBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	u := currentUser(r)
	if err := s.store.SetProgress(r.Context(), appdb.Progress{
		UserID: u.ID, BookID: b.ID, Format: "CBZ", Page: body.Page, TotalPages: body.TotalPages,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save progress")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- helpers ---

// resolveCBZ returns the absolute path, mtime and size of a book's CBZ file.
func (s *Server) resolveCBZ(b *calibre.Book) (path string, mtime, size int64, err error) {
	for _, f := range b.Formats {
		if strings.EqualFold(f.Format, "CBZ") {
			p := filepath.Join(s.lib().BookFolder(b), f.FormatFile())
			info, serr := os.Stat(p)
			if serr != nil {
				return "", 0, 0, serr
			}
			return p, info.ModTime().UnixNano(), info.Size(), nil
		}
	}
	return "", 0, 0, os.ErrNotExist
}

// cbzPages returns the CBZ's ordered page names, consulting and refreshing the
// page-list cache.
func (s *Server) cbzPages(r *http.Request, b *calibre.Book) ([]string, error) {
	return s.cbzPagesCtx(r.Context(), b)
}

// cbzPagesCtx returns a book's CBZ page list, from the app.db cache when valid
// (by mtime/size) or by scanning the archive's central directory once. The scan
// result (including page count) is cached, which is what the page-count index
// and filter rely on.
func (s *Server) cbzPagesCtx(ctx context.Context, b *calibre.Book) ([]string, error) {
	cbz, mtime, size, err := s.resolveCBZ(b)
	if err != nil {
		return nil, err
	}
	if e, err := s.store.GetPageCache(ctx, b.ID, "CBZ", mtime, size); err == nil {
		return e.Pages, nil
	}
	pages, err := reader.Pages(cbz)
	if err != nil {
		return nil, err
	}
	_ = s.store.PutPageCache(ctx, appdb.PageCacheEntry{
		BookID: b.ID, Format: "CBZ", FilePath: cbz, Pages: pages,
		PageCount: len(pages), MTime: mtime, Size: size, ScannedAt: time.Now(),
	})
	return pages, nil
}

func serveCachedBytes(w http.ResponseWriter, r *http.Request, contentType string, data []byte, etag string) {
	if etag != "" {
		etagHeader := `"` + etag + `"`
		w.Header().Set("ETag", etagHeader)
		if match := r.Header.Get("If-None-Match"); match == etagHeader {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Write(data)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}

// atoi64s parses a list of query values into int64s, dropping non-positive ones.
func atoi64s(ss []string) []int64 {
	var out []int64
	for _, s := range ss {
		if n := atoi64(s); n > 0 {
			out = append(out, n)
		}
	}
	return out
}

func clampWidth(w, def int) int {
	if w <= 0 {
		return def
	}
	if w > 4000 {
		return 4000
	}
	return w
}

func sanitizeFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 0x20 {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "book"
	}
	return s
}
