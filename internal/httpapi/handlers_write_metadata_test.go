package httpapi

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"incipit/internal/calibre"
	"incipit/internal/metadata"
)

// mockCmoa serves canned search/book/cover responses so the upload-with-metadata
// path can be exercised without touching the network.
func mockCmoa(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/result/"):
			// Only "ReLIFE" matches; anything else is a no-result 404.
			if strings.Contains(r.URL.RawQuery, "ReLIFE") {
				w.Write([]byte(`<div class="search_result_box_right_sec1"><p>` +
					`<a href="/title/12345/">ReLIFE</a></p></div>`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`<html><body>no result</body></html>`))
		case strings.HasPrefix(r.URL.Path, "/title/"):
			fmt.Fprintf(w, `<!doctype html><html><head>
<title>ReLIFE 3巻【フルカラー】｜無料漫画ならコミックシーモア｜夜宵草</title></head><body>
<nav class="brCramb"><a href="/">コミックシーモアTOP</a>
  <a href="/g/">青年マンガ</a><a href="/t/">ReLIFE【フルカラー】</a></nav>
<div class="category_line"><div class="category_line_f_l_l">配信開始日</div>
  <div class="category_line_f_r_l">2020年05月01日</div></div>
<div class="category_line"><div class="category_line_f_l_l">出版社</div>
  <div class="category_line_f_r_l"><a href="/p/">comico</a></div></div>
<div class="title_details_author_name"><a href="#">夜宵草</a></div>
<div id="comic_description"><p>あらすじ本文。</p></div>
<div class="genre_detail"><a href="#">青春</a><a href="#">広告掲載中</a></div>
<div class="reviewArea">（4.5） 投稿数850件</div>
<img class="title_big_thum" src="%s/cover.jpg"></body></html>`, srv.URL)
		case r.URL.Path == "/cover.jpg":
			// Distinctive 1×1 JPEG so the test can confirm the cmoa cover (not the
			// 300×400 first page) was stored.
			img := image.NewRGBA(image.Rect(0, 0, 1, 1))
			img.Set(0, 0, color.RGBA{255, 0, 0, 255})
			w.Header().Set("Content-Type", "image/jpeg")
			jpeg.Encode(w, img, nil)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// uploadCBZMeta posts a CBZ with the auto-fetch-metadata options set.
func (h *harness) uploadCBZMeta(title, genre string, data []byte) *http.Response {
	h.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("title", title)
	mw.WriteField("fetchMeta", "true")
	mw.WriteField("genre", genre)
	fw, _ := mw.CreateFormFile("file", "comic.cbz")
	fw.Write(data)
	mw.Close()
	return h.do(http.MethodPost, "/api/books", &body, mw.FormDataContentType())
}

func TestUploadWithMetadataFetch(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()

	cmoa := mockCmoa(t)
	h.srv.meta = &metadata.Client{HTTP: cmoa.Client(), Root: cmoa.URL}

	// Matched: cmoa metadata + official cover overlay the filename.
	resp := h.uploadCBZMeta("ReLIFE 3", "seinen", makeCBZBytes(t, 3))
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("X-Metadata-Matched"); got == "false" {
		t.Errorf("expected a match, got X-Metadata-Matched=%q", got)
	}
	var book calibre.Book
	decodeBody(t, resp, &book)

	if book.Title != "ReLIFE【フルカラー】 03" {
		t.Errorf("Title = %q", book.Title)
	}
	if book.Series == nil || book.Series.Name != "ReLIFE【フルカラー】" {
		t.Errorf("Series = %+v", book.Series)
	}
	if len(book.Authors) != 1 || book.Authors[0].Name != "夜宵草" {
		t.Errorf("Authors = %+v", book.Authors)
	}
	if book.Publisher == nil || book.Publisher.Name != "comico" {
		t.Errorf("Publisher = %+v", book.Publisher)
	}
	if book.Rating != 9 {
		t.Errorf("Rating = %d; want 9", book.Rating)
	}
	if book.PubDate.Format("2006-01-02") != "2020-05-01" {
		t.Errorf("PubDate = %v", book.PubDate)
	}
	if len(book.Tags) != 1 || book.Tags[0].Name != "青春" {
		t.Errorf("Tags = %+v (promo tag should be dropped)", book.Tags)
	}
	if !book.HasCover {
		t.Fatal("expected a cover")
	}
	// The stored cover must be the 1×1 cmoa cover, not the 300×400 first page.
	cresp := h.do(http.MethodGet, bookPath(book.ID, "/cover"), nil, "")
	defer cresp.Body.Close()
	cfg, _, err := image.DecodeConfig(cresp.Body)
	if err != nil {
		t.Fatalf("decode cover: %v", err)
	}
	if cfg.Width != 1 || cfg.Height != 1 {
		t.Errorf("cover is %dx%d; expected the 1x1 cmoa cover", cfg.Width, cfg.Height)
	}
}

func TestUploadWithMetadataNoMatch(t *testing.T) {
	h := newHarness(t)
	h.postJSON("/api/setup", credentials{Username: "admin", Password: "supersecret"}).Body.Close()

	cmoa := mockCmoa(t)
	h.srv.meta = &metadata.Client{HTTP: cmoa.Client(), Root: cmoa.URL}

	// No match → upload still succeeds with the filename, flagged via header.
	resp := h.uploadCBZMeta("存在しない作品 4", "seinen", makeCBZBytes(t, 2))
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload status = %d: %s", resp.StatusCode, b)
	}
	if got := resp.Header.Get("X-Metadata-Matched"); got != "false" {
		t.Errorf("expected X-Metadata-Matched=false, got %q", got)
	}
	var book calibre.Book
	decodeBody(t, resp, &book)
	if book.Title != "存在しない作品 4" {
		t.Errorf("Title = %q; want the filename-derived title", book.Title)
	}
	// First-page cover still applies (300x400 from makeCBZBytes).
	if !book.HasCover {
		t.Error("expected first-page fallback cover")
	}
}
