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
	var comic struct {
		ID int64 `json:"id"`
	}
	decodeBody(t, h.uploadCBZ("OPDS Comic", makeCBZBytes(t, 3)), &comic)

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
	// OPDS-PSE page streaming: namespace + a stream link with pse:count (the CBZ
	// has 3 pages). The literal "pse:count" prefix must survive XML encoding.
	if !strings.Contains(string(body), `xmlns:pse="http://vaemendis.net/opds-pse/1.0"`) ||
		!strings.Contains(string(body), `rel="http://vaemendis.net/opds-pse/stream"`) ||
		!strings.Contains(string(body), `pse:count="3"`) ||
		!strings.Contains(string(body), "/opds/books/") ||
		!strings.Contains(string(body), "/page/{pageNumber}") {
		t.Errorf("opds feed missing PSE stream link:\n%s", body)
	}

	// The PSE page endpoint (under /opds, Basic auth) returns an image.
	req, _ = http.NewRequest(http.MethodGet, h.server.URL+"/opds/books/"+strconv.FormatInt(comic.ID, 10)+"/page/0", nil)
	req.SetBasicAuth("admin", "supersecret")
	resp = h.raw(req)
	pageCT := resp.Header.Get("Content-Type")
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(pageCT, "image/") {
		t.Errorf("opds pse page = %d type=%q", resp.StatusCode, pageCT)
	}

	// Root feed advertises the OpenSearch description (search discovery).
	req, _ = http.NewRequest(http.MethodGet, h.server.URL+"/opds", nil)
	req.SetBasicAuth("admin", "supersecret")
	resp = h.raw(req)
	root, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(root), `rel="search"`) ||
		!strings.Contains(string(root), "/opds/opensearch.xml") {
		t.Errorf("opds root missing opensearch search link:\n%s", root)
	}

	// The OpenSearch description declares an absolute, path-based template.
	req, _ = http.NewRequest(http.MethodGet, h.server.URL+"/opds/opensearch.xml", nil)
	req.SetBasicAuth("admin", "supersecret")
	resp = h.raw(req)
	osd, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(osd), "OpenSearchDescription") ||
		!strings.Contains(string(osd), "/opds/search/{searchTerms}") {
		t.Errorf("opensearch description malformed:\n%s", osd)
	}

	// Both search forms return the matching book: query-string and calibre-web
	// path (the form Comic Share generates from the template).
	for _, path := range []string{"/opds/search?q=OPDS", "/opds/search/OPDS"} {
		req, _ = http.NewRequest(http.MethodGet, h.server.URL+path, nil)
		req.SetBasicAuth("admin", "supersecret")
		resp = h.raw(req)
		sb, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(sb), "OPDS Comic") {
			t.Errorf("opds search %q missing result:\n%s", path, sb)
		}
	}

	// OPDS is localized to the authenticated user's language.
	h.putJSON("/api/auth/me", map[string]string{"language": "ja"}).Body.Close()
	req, _ = http.NewRequest(http.MethodGet, h.server.URL+"/opds", nil)
	req.SetBasicAuth("admin", "supersecret")
	resp = h.raw(req)
	ja, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(ja), "著者別") || !strings.Contains(string(ja), "シリーズ別") {
		t.Errorf("opds root not localized to ja:\n%s", ja)
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

// TestShelfPrivacy verifies that a PRIVATE shelf is strictly owner-only: no
// other account — not even another admin — can see it, its name, its owner, or
// its contents, and the endpoints 404 (never 403) so the shelf's existence
// stays hidden. PUBLIC shelves remain world-readable.
func TestShelfPrivacy(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "owner", Password: "supersecret"}).Body.Close()

	var priv, pub appdb.Shelf
	decodeBody(t, h.postJSON("/api/shelves", createShelfBody{Name: "秘密の本棚", IsPublic: false}), &priv)
	decodeBody(t, h.postJSON("/api/shelves", createShelfBody{Name: "公開本棚", IsPublic: true}), &pub)

	// A second admin — the exact scenario that leaked before (every account in
	// the deployment was an admin).
	if resp := h.postJSON("/api/admin/users", createUserBody{Username: "other", Password: "otherpass", IsAdmin: true}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create other admin = %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	// Also make a plain non-admin user to cover the ordinary case.
	if resp := h.postJSON("/api/admin/users", createUserBody{Username: "plain", Password: "plainpass"}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("create plain user = %d", resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	status := func(method, path string) int {
		resp := h.do(method, path, nil, "")
		resp.Body.Close()
		return resp.StatusCode
	}

	// From each OTHER account, the private shelf must be invisible (404 on every
	// read and write), while the public shelf stays readable.
	for _, who := range []credentials{{Username: "other", Password: "otherpass"}, {Username: "plain", Password: "plainpass"}} {
		h.do(http.MethodPost, "/api/auth/logout", nil, "").Body.Close()
		if resp := h.postJSON("/api/auth/login", who); resp.StatusCode != http.StatusOK {
			t.Fatalf("login %s = %d", who.Username, resp.StatusCode)
		} else {
			resp.Body.Close()
		}

		for _, suffix := range []string{"", "/books", "/contents"} {
			if got := status(http.MethodGet, shelfPath(priv.ID, suffix)); got != http.StatusNotFound {
				t.Errorf("%s GET private%s = %d, want 404 (must not leak name/owner/existence)", who.Username, suffix, got)
			}
		}
		// Mutations on the private shelf are likewise masked as 404.
		if got := status(http.MethodDelete, shelfPath(priv.ID, "")); got != http.StatusNotFound {
			t.Errorf("%s DELETE private = %d, want 404", who.Username, got)
		}
		if got := status(http.MethodGet, shelfPath(pub.ID, "/contents")); got != http.StatusOK {
			t.Errorf("%s GET public/contents = %d, want 200 (public stays shareable)", who.Username, got)
		}
	}

	// The owner still has full access to their own private shelf.
	h.do(http.MethodPost, "/api/auth/logout", nil, "").Body.Close()
	h.postJSON("/api/auth/login", credentials{Username: "owner", Password: "supersecret"}).Body.Close()
	if got := status(http.MethodGet, shelfPath(priv.ID, "/contents")); got != http.StatusOK {
		t.Errorf("owner GET own private/contents = %d, want 200", got)
	}
}

// path helpers

func TestPageCountFilter(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()
	var small, big struct {
		ID int64 `json:"id"`
	}
	decodeBody(t, h.uploadCBZ("Small", makeCBZBytes(t, 3)), &small)
	decodeBody(t, h.uploadCBZ("Big", makeCBZBytes(t, 10)), &big)

	// Populate the page-count cache (as the reader / background index would).
	h.do(http.MethodGet, bookPath(small.ID, "/pages"), nil, "").Body.Close()
	h.do(http.MethodGet, bookPath(big.ID, "/pages"), nil, "").Body.Close()

	// Filter is inert until enabled.
	var res struct {
		Total int `json:"total"`
		Books []struct {
			Title string `json:"title"`
		} `json:"books"`
	}
	decodeBody(t, h.do(http.MethodGet, "/api/books?minPages=5", nil, ""), &res)
	if res.Total != 2 {
		t.Errorf("filter disabled should ignore minPages, got total=%d", res.Total)
	}

	pf := true
	h.putJSON("/api/admin/site", siteUpdateBody{Title: "x", PageFilter: &pf}).Body.Close()

	decodeBody(t, h.do(http.MethodGet, "/api/books?minPages=5", nil, ""), &res)
	if res.Total != 1 || res.Books[0].Title != "Big" {
		t.Errorf("minPages=5 => %+v, want [Big]", res)
	}
	decodeBody(t, h.do(http.MethodGet, "/api/books?maxPages=5", nil, ""), &res)
	if res.Total != 1 || res.Books[0].Title != "Small" {
		t.Errorf("maxPages=5 => %+v, want [Small]", res)
	}
	decodeBody(t, h.do(http.MethodGet, "/api/books?minPages=3&maxPages=10", nil, ""), &res)
	if res.Total != 2 {
		t.Errorf("minPages=3&maxPages=10 => total=%d, want 2", res.Total)
	}
}

// TestGroupedRecentlyRead covers the series-grouped view under the per-user
// "recently read" sort: it must return grouped units (not a flat list, which
// rendered as an empty library), and reading any single volume must rank the
// whole series to the top.
func TestGroupedRecentlyRead(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()

	upload := func(title string) calibre.Book {
		resp := h.uploadCBZ(title, makeCBZBytes(t, 5))
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("upload %q = %d", title, resp.StatusCode)
		}
		var b calibre.Book
		decodeBody(t, resp, &b)
		return b
	}
	setSeries := func(id int64, name string, idx float64) {
		resp := h.putJSON(bookPath(id, ""), updateBookBody{Series: &name, SeriesIndex: &idx})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("set series = %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Two volumes of one series + one standalone book => 2 grouped units.
	vol1 := upload("Saga Vol 1")
	vol2 := upload("Saga Vol 2")
	upload("Standalone")
	const seriesName = "Saga"
	setSeries(vol1.ID, seriesName, 1)
	setSeries(vol2.ID, seriesName, 2)

	groupedLastRead := func() calibre.GroupedResult {
		var res calibre.GroupedResult
		decodeBody(t, h.do(http.MethodGet, "/api/books?group=series&sort=lastread&order=desc", nil, ""), &res)
		return res
	}

	// Regression: grouped + lastread returned a flat ListResult (no units), which
	// the grouped UI rendered as "library empty".
	res := groupedLastRead()
	if len(res.Units) == 0 {
		t.Fatalf("grouped+lastread returned no units (series-grouped recently-read showed empty)")
	}
	if res.Total != 2 {
		t.Errorf("total units = %d, want 2 (series + standalone)", res.Total)
	}

	// Reading one volume ranks the whole series to the top.
	resp := h.putJSON(bookPath(vol2.ID, "/progress"), progressBody{Page: 3, TotalPages: 5})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set progress = %d", resp.StatusCode)
	}
	resp.Body.Close()

	res = groupedLastRead()
	if len(res.Units) == 0 || res.Units[0].Kind != "series" || res.Units[0].Series == nil {
		t.Fatalf("first unit = %+v, want the read series first", res.Units)
	}
	if res.Units[0].Series.Name != seriesName || res.Units[0].Series.BookCount != 2 {
		t.Errorf("top series = %+v, want %q with 2 volumes", res.Units[0].Series, seriesName)
	}
}

func bookPath(id int64, suffix string) string {
	return "/api/books/" + strconv.FormatInt(id, 10) + suffix
}
func shelfPath(id int64, suffix string) string {
	return "/api/shelves/" + strconv.FormatInt(id, 10) + suffix
}
func shelfBookPath(shelfID, bookID int64) string {
	return "/api/shelves/" + strconv.FormatInt(shelfID, 10) + "/books/" + strconv.FormatInt(bookID, 10)
}
