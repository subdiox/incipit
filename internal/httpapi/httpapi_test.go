package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"incipit/internal/appdb"
	"incipit/internal/auth"
	"incipit/internal/calibre"
	"incipit/internal/config"
	"incipit/internal/reader"
)

type harness struct {
	t      *testing.T
	server *httptest.Server
	client *http.Client
	srv    *Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	cfgDir := filepath.Join(dir, "config")
	cacheDir := filepath.Join(cfgDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	lib, err := calibre.Open(libDir, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lib.Close() })
	store, err := appdb.Open(filepath.Join(cfgDir, "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	cfg := &config.Config{
		LibraryPath:   libDir,
		ConfigDir:     cfgDir,
		CacheDir:      cacheDir,
		SessionSecret: []byte("test-secret-test-secret-test-secret"),
	}
	ldapMgr := auth.NewLDAPManager(auth.LDAPSettings{})
	srv := New(cfg, lib, store, auth.NewService(store, ldapMgr), reader.NewService(cacheDir), ldapMgr)
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(ts.Close)

	jar, _ := cookiejar.New(nil)
	return &harness{t: t, server: ts, client: &http.Client{Jar: jar}, srv: srv}
}

// do performs a request, attaching the CSRF header for unsafe methods.
func (h *harness) do(method, path string, body io.Reader, contentType string) *http.Response {
	h.t.Helper()
	req, err := http.NewRequest(method, h.server.URL+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet && method != http.MethodHead {
		if tok := h.csrf(); tok != "" {
			req.Header.Set(csrfHeader, tok)
		}
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// raw performs a prepared request, failing the test on transport error.
func (h *harness) raw(req *http.Request) *http.Response {
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	return resp
}

func (h *harness) csrf() string {
	u, _ := url.Parse(h.server.URL)
	for _, c := range h.client.Jar.Cookies(u) {
		if c.Name == csrfCookie {
			return c.Value
		}
	}
	return ""
}

func (h *harness) postJSON(path string, v any) *http.Response {
	b, _ := json.Marshal(v)
	return h.do(http.MethodPost, path, bytes.NewReader(b), "application/json")
}

func (h *harness) putJSON(path string, v any) *http.Response {
	b, _ := json.Marshal(v)
	return h.do(http.MethodPut, path, bytes.NewReader(b), "application/json")
}

func decodeBody(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func makeCBZBytes(t *testing.T, pages int) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 1; i <= pages; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 300, 400))
		for x := 0; x < 300; x++ {
			for y := 0; y < 400; y++ {
				img.Set(x, y, color.RGBA{uint8(i * 20), uint8(x), uint8(y), 255})
			}
		}
		w, _ := zw.Create(pageName(i))
		png.Encode(w, img)
	}
	zw.Close()
	return buf.Bytes()
}

func pageName(i int) string {
	if i < 10 {
		return "page0" + string(rune('0'+i)) + ".png"
	}
	return "page" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + ".png"
}

func (h *harness) uploadCBZ(title string, data []byte) *http.Response {
	h.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("title", title)
	mw.WriteField("authors", "Test Author")
	mw.WriteField("tags", "test, comic")
	fw, _ := mw.CreateFormFile("file", "comic.cbz")
	fw.Write(data)
	mw.Close()
	return h.do(http.MethodPost, "/api/books", &body, mw.FormDataContentType())
}

func TestFullAPIFlow(t *testing.T) {
	h := newHarness(t)

	// 1. Setup is needed initially.
	var status struct{ NeedsSetup bool }
	decodeBody(t, h.do(http.MethodGet, "/api/setup/status", nil, ""), &status)
	if !status.NeedsSetup {
		t.Fatal("expected needsSetup=true")
	}

	// 2. First-run admin creation logs us in.
	resp := h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("setup status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. /me reflects the admin.
	var me appdb.User
	decodeBody(t, h.do(http.MethodGet, "/api/auth/me", nil, ""), &me)
	if me.Username != "admin" || !me.IsAdmin {
		t.Fatalf("me = %+v", me)
	}

	// 4. Upload a CBZ.
	resp = h.uploadCBZ("My First Comic", makeCBZBytes(t, 5))
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, b)
	}
	var book calibre.Book
	decodeBody(t, resp, &book)
	if book.ID == 0 || book.Title != "My First Comic" || len(book.Authors) != 1 {
		t.Fatalf("uploaded book = %+v", book)
	}
	if !book.HasCover {
		t.Error("expected cover generated from first page")
	}

	// 5. It appears in listings.
	var list calibre.ListResult
	decodeBody(t, h.do(http.MethodGet, "/api/books?sort=title", nil, ""), &list)
	if list.Total != 1 {
		t.Fatalf("list total = %d", list.Total)
	}

	// 6. Page list + page rendering.
	var pl struct {
		Count int      `json:"count"`
		Pages []string `json:"pages"`
	}
	decodeBody(t, h.do(http.MethodGet, bookPath(book.ID, "/pages"), nil, ""), &pl)
	if pl.Count != 5 {
		t.Fatalf("page count = %d", pl.Count)
	}
	resp = h.do(http.MethodGet, bookPath(book.ID, "/pages/0?w=150"), nil, "")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "image/") {
		t.Fatalf("page render: status=%d type=%s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()

	// 7. Thumbnail.
	resp = h.do(http.MethodGet, bookPath(book.ID, "/thumbnail"), nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("thumbnail status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 8. Reading progress round-trip.
	resp = h.putJSON(bookPath(book.ID, "/progress"), progressBody{Page: 3, TotalPages: 5})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set progress = %d", resp.StatusCode)
	}
	resp.Body.Close()
	var prog appdb.Progress
	decodeBody(t, h.do(http.MethodGet, bookPath(book.ID, "/progress"), nil, ""), &prog)
	if prog.Page != 3 {
		t.Errorf("progress page = %d", prog.Page)
	}

	// 9. Edit metadata.
	newTitle := "Renamed Comic"
	resp = h.putJSON(bookPath(book.ID, ""), updateBookBody{Title: &newTitle})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update = %d", resp.StatusCode)
	}
	var updated calibre.Book
	decodeBody(t, resp, &updated)
	if updated.Title != newTitle {
		t.Errorf("updated title = %q", updated.Title)
	}

	// 10. Shelves.
	resp = h.postJSON("/api/shelves", createShelfBody{Name: "Faves"})
	var shelf appdb.Shelf
	decodeBody(t, resp, &shelf)
	resp = h.do(http.MethodPost, shelfBookPath(shelf.ID, book.ID), nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add to shelf = %d", resp.StatusCode)
	}
	resp.Body.Close()
	var shelfBooks calibre.ListResult
	decodeBody(t, h.do(http.MethodGet, shelfPath(shelf.ID, "/books"), nil, ""), &shelfBooks)
	if shelfBooks.Total != 1 {
		t.Errorf("shelf books = %d", shelfBooks.Total)
	}

	// 11. Admin: create another user.
	resp = h.postJSON("/api/admin/users", createUserBody{Username: "reader", Password: "readerpass", CanDownload: true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user = %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 12. Facets reflect the upload.
	var authors []calibre.Facet
	decodeBody(t, h.do(http.MethodGet, "/api/authors", nil, ""), &authors)
	if len(authors) != 1 || authors[0].Count != 1 {
		t.Errorf("authors = %+v", authors)
	}

	// 13. Logout invalidates the session.
	resp = h.do(http.MethodPost, "/api/auth/logout", nil, "")
	resp.Body.Close()
	resp = h.do(http.MethodGet, "/api/auth/me", nil, "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("after logout /me = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestOPDSFeeds(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()
	h.uploadCBZ("OPDS Comic", makeCBZBytes(t, 3)).Body.Close()

	// OPDS requires Basic auth.
	req, _ := http.NewRequest(http.MethodGet, h.server.URL+"/opds/new", nil)
	resp := h.raw(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("opds without auth = %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, h.server.URL+"/opds/new", nil)
	req.SetBasicAuth("admin", "supersecret")
	resp = h.raw(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("opds new = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "OPDS Comic") ||
		!strings.Contains(string(body), "http://opds-spec.org/acquisition") {
		t.Errorf("opds feed missing entry/acquisition:\n%s", body)
	}
}

func TestCSRFAndAuthEnforcement(t *testing.T) {
	h := newHarness(t)

	// Unauthenticated mutation is rejected.
	resp := h.postJSON("/api/shelves", createShelfBody{Name: "x"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth shelf = %d", resp.StatusCode)
	}
	resp.Body.Close()

	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()

	// Authenticated but missing CSRF header => 403.
	req, _ := http.NewRequest(http.MethodPost, h.server.URL+"/api/shelves",
		strings.NewReader(`{"name":"y"}`))
	req.Header.Set("Content-Type", "application/json")
	resp = h.raw(req) // no CSRF header
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("missing CSRF = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()

	// Non-admin cannot reach admin routes.
	h.postJSON("/api/admin/users", createUserBody{Username: "reader", Password: "readerpass"}).Body.Close()
	h.do(http.MethodPost, "/api/auth/logout", nil, "").Body.Close()
	h.postJSON("/api/auth/login", credentials{Username: "reader", Password: "readerpass"}).Body.Close()
	resp = h.do(http.MethodGet, "/api/admin/users", nil, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("non-admin admin route = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

// path helpers

func bookPath(id int64, suffix string) string {
	return "/api/books/" + strconv.FormatInt(id, 10) + suffix
}
func shelfPath(id int64, suffix string) string {
	return "/api/shelves/" + strconv.FormatInt(id, 10) + suffix
}
func shelfBookPath(shelfID, bookID int64) string {
	return "/api/shelves/" + strconv.FormatInt(shelfID, 10) + "/books/" + strconv.FormatInt(bookID, 10)
}
