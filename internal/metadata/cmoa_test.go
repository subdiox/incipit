package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitTitleVolume(t *testing.T) {
	cases := []struct {
		in   string
		work string
		vol  int
		ok   bool
	}{
		{"作品名 30", "作品名", 30, true},
		{"作品名　30", "作品名", 30, true}, // full-width space
		{"作品名（30）", "作品名", 30, true},
		{"作品名(30)", "作品名", 30, true},
		{"ONE PIECE 105", "ONE PIECE", 105, true},
		{"巻数なし作品", "巻数なし作品", 0, false},
	}
	for _, c := range cases {
		work, vol, ok := splitTitleVolume(c.in)
		if work != c.work || vol != c.vol || ok != c.ok {
			t.Errorf("splitTitleVolume(%q) = (%q,%d,%v); want (%q,%d,%v)",
				c.in, work, vol, ok, c.work, c.vol, c.ok)
		}
	}
}

func TestExtractVolume(t *testing.T) {
	cases := []struct {
		in  string
		vol int
		ok  bool
	}{
		{"ReLIFE 1巻【フルカラー】", 1, true},
		{"作品名 30", 30, true},
		{"作品名（5）", 5, true},
		{"第3巻から第10巻まで", 10, true}, // last N巻 wins
		{"巻数なし", 0, false},
	}
	for _, c := range cases {
		vol, ok := extractVolume(c.in)
		if vol != c.vol || ok != c.ok {
			t.Errorf("extractVolume(%q) = (%d,%v); want (%d,%v)", c.in, vol, ok, c.vol, c.ok)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	cases := map[string]string{
		"天は赤い河のほとり　1":   "天は赤い河のほとり 01",
		"ONE PIECE　105": "ONE PIECE 105",
		"巻数なし作品":        "巻数なし作品",
	}
	for in, want := range cases {
		if got := normalizeTitle(in); got != want {
			t.Errorf("normalizeTitle(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSearchTokens(t *testing.T) {
	got := searchTokens("NARUTO―ナルト―")
	want := []string{"NARUTO", "ナルト"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("searchTokens = %v; want %v", got, want)
	}
	// The prolonged-sound mark ー(U+30FC) is part of a word, not a separator.
	if got := searchTokens("ソードアート・オンライン"); len(got) != 1 {
		t.Errorf("middle dot and long-sound mark must not split, got %v", got)
	}
	// ASCII hyphen splits.
	if got := searchTokens("BORUTO-ボルト-"); len(got) != 2 {
		t.Errorf("expected 2 tokens for hyphen split, got %v", got)
	}
}

func TestGenreIDsDefault(t *testing.T) {
	if ids := genreIDs("does-not-exist"); len(ids) != 1 || ids[0] != nil {
		t.Errorf("unknown genre should default to [nil], got %v", ids)
	}
	if ids := genreIDs("comic"); len(ids) != len(comicGenreIDs) {
		t.Errorf("comic should fan out, got %d ids", len(ids))
	}
}

const searchHTML = `<!doctype html><html><body>
<div class="search_result_box_right_sec1"><p><a href="/title/12345/">ReLIFE</a></p></div>
</body></html>`

const bookHTML = `<!doctype html><html><head>
<title>ReLIFE 3巻【フルカラー】（最新刊）｜無料漫画ならコミックシーモア｜著者名</title>
</head><body>
<nav class="brCramb">
  <a href="/">コミックシーモアTOP</a>
  <a href="/genre/13/">青年マンガ</a>
  <a href="/publisher/comico/">comico</a>
  <a href="/title/12345/">ReLIFE【フルカラー】</a>
</nav>
<div class="category_line">
  <div class="category_line_f_l_l">配信開始日</div>
  <div class="category_line_f_r_l"><span class="margin_r5">：</span>2020年5月1日</div>
</div>
<div class="category_line">
  <div class="category_line_f_l_l">出版社</div>
  <div class="category_line_f_r_l"><a href="/publisher/comico/">comico</a></div>
</div>
<div class="title_details_author_name"><a href="#">夜宵草</a><a href="#">夜宵草</a></div>
<div id="comic_description"><p>あらすじ一行目。<br>二行目<ruby>本文<rt>ほんぶん</rt></ruby>。</p></div>
<div class="genre_detail"><a href="#">青春</a><a href="#">広告掲載中</a><a href="#">青春</a><a href="#">学園</a></div>
<div class="reviewArea">（4.5） 投稿数850件</div>
<img class="title_big_thum" src="//cover.example/relife3.jpg">
</body></html>`

func newTestClient(t *testing.T) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/result/"):
			w.Write([]byte(searchHTML))
		case strings.HasPrefix(r.URL.Path, "/title/12345/"):
			w.Write([]byte(bookHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), Root: srv.URL}, srv
}

func TestFetch(t *testing.T) {
	c, _ := newTestClient(t)
	m, err := c.Fetch(context.Background(), "ReLIFE 3", "seinen", "", "")
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if m == nil {
		t.Fatal("expected a match, got nil")
	}
	if m.Title != "ReLIFE【フルカラー】 03" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Series != "ReLIFE【フルカラー】" {
		t.Errorf("Series = %q", m.Series)
	}
	if m.SeriesIndex != 3 {
		t.Errorf("SeriesIndex = %v", m.SeriesIndex)
	}
	if len(m.Authors) != 1 || m.Authors[0] != "夜宵草" {
		t.Errorf("Authors = %v (want one, deduped)", m.Authors)
	}
	if m.Publisher != "comico" {
		t.Errorf("Publisher = %q", m.Publisher)
	}
	if m.PubDate.Format("2006-01-02") != "2020-05-01" {
		t.Errorf("PubDate = %v", m.PubDate)
	}
	// <br> preserved as a line break; ruby flattened to its base text.
	if m.Comments != "あらすじ一行目。<br>二行目本文ほんぶん。" {
		t.Errorf("Comments = %q", m.Comments)
	}
	// "広告掲載中" excluded; "青春" deduped → ["青春","学園"].
	if strings.Join(m.Tags, ",") != "青春,学園" {
		t.Errorf("Tags = %v", m.Tags)
	}
	if m.Rating != 9 { // 4.5 stars × 2
		t.Errorf("Rating = %d; want 9", m.Rating)
	}
	if m.CoverURL != "https://cover.example/relife3.jpg" {
		t.Errorf("CoverURL = %q", m.CoverURL)
	}
}

func TestFetchNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Search page with no result block.
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`<html><body>no result</body></html>`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), Root: srv.URL}
	m, err := c.Fetch(context.Background(), "存在しない作品 1", "seinen", "", "")
	if err != nil {
		t.Fatalf("no-match should not error, got %v", err)
	}
	if m != nil {
		t.Errorf("expected nil meta, got %+v", m)
	}
}
