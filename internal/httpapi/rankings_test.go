package httpapi

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"incipit/internal/calibre"
)

// TestRankingsAPI covers the rankings HTTP surface end-to-end: the feature is
// gated off by default, an admin toggle turns it on, /api/rankings lists the
// configured lists in tab order with counts, and /api/books?ranking=<key> serves
// the books in explicit rank order.
func TestRankingsAPI(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()

	var a, b, c struct {
		ID int64 `json:"id"`
	}
	decodeBody(t, h.uploadCBZ("Alpha", makeCBZBytes(t, 2)), &a)
	decodeBody(t, h.uploadCBZ("Beta", makeCBZBytes(t, 2)), &b)
	decodeBody(t, h.uploadCBZ("Gamma", makeCBZBytes(t, 2)), &c)

	// Seed the ranking side tables directly, exactly as the external importer
	// (crawler) does — a separate connection to the same metadata.db.
	metaPath := filepath.Join(h.srv.lib().LibraryPath(), "metadata.db")
	db, err := sql.Open("sqlite", metaPath)
	if err != nil {
		t.Fatalf("open metadata.db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO ranking_lists (key,label,position) VALUES
		('popular-week','週間人気',1), ('popular-today','今日の人気',0)`); err != nil {
		t.Fatalf("seed ranking_lists: %v", err)
	}
	// popular-today: Gamma(1), Alpha(2). popular-week: Beta(1).
	if _, err := db.Exec(`INSERT INTO book_rankings (list,rank,book) VALUES
		('popular-today',1,?), ('popular-today',2,?), ('popular-week',1,?)`,
		c.ID, a.ID, b.ID); err != nil {
		t.Fatalf("seed book_rankings: %v", err)
	}

	// Gated off: /api/rankings is empty and ?ranking= is ignored (feature off).
	var lists []calibre.RankingList
	decodeBody(t, h.do(http.MethodGet, "/api/rankings", nil, ""), &lists)
	if len(lists) != 0 {
		t.Fatalf("rankings should be empty while disabled, got %+v", lists)
	}
	var res calibre.ListResult
	decodeBody(t, h.do(http.MethodGet, "/api/books?ranking=popular-today", nil, ""), &res)
	if res.Total != 3 {
		t.Fatalf("ranking ignored while disabled should list all 3, got total=%d", res.Total)
	}

	// Enable the feature.
	on := true
	h.putJSON("/api/admin/site", siteUpdateBody{Title: "x", Rankings: &on}).Body.Close()

	// /api/rankings now lists both, in position order, with counts.
	decodeBody(t, h.do(http.MethodGet, "/api/rankings", nil, ""), &lists)
	if len(lists) != 2 {
		t.Fatalf("want 2 lists, got %+v", lists)
	}
	if lists[0].Key != "popular-today" || lists[0].Count != 2 {
		t.Fatalf("list[0] = %+v, want popular-today count 2", lists[0])
	}
	if lists[1].Key != "popular-week" || lists[1].Count != 1 {
		t.Fatalf("list[1] = %+v, want popular-week count 1", lists[1])
	}

	// ?ranking= serves the books in explicit rank order.
	decodeBody(t, h.do(http.MethodGet, "/api/books?ranking=popular-today", nil, ""), &res)
	if res.Total != 2 || len(res.Books) != 2 {
		t.Fatalf("ranking total = %d, len = %d, want 2", res.Total, len(res.Books))
	}
	if res.Books[0].ID != c.ID || res.Books[1].ID != a.ID {
		t.Fatalf("ranking order = [%d %d], want [%d %d]", res.Books[0].ID, res.Books[1].ID, c.ID, a.ID)
	}
}
