// Package metadata fetches book metadata from external sources by title, so an
// uploaded file named "Series 03" can be auto-enriched (authors, publisher,
// pubdate, description, tags, rating, cover) without manual entry.
//
// The only source today is コミックシーモア (cmoa.jp). This is a clean-room Go
// port of the original Python "ookamura" uploader: we read cmoa's public HTML,
// not any private API. Genre filtering avoids matching a same-named work in the
// wrong category — see the genre notes below and GenreChoices.
package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// ErrFetch signals that talking to cmoa failed even after retries. It is
// distinct from "no match": a successful fetch with no result returns (nil, nil)
// so the caller can fall back to filename-derived metadata instead of failing.
var ErrFetch = errors.New("metadata: cmoa fetch failed")

const (
	defaultRoot  = "https://www.cmoa.jp"
	searchPath   = "/search/result/?header_word="
	userAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	acceptHeader = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"
)

// Genre is one selectable search category. IDs is the ordered list of cmoa
// genre_ids to try; a single nil entry means "no genre filter" (search all).
type Genre struct {
	Key   string
	Label string
	IDs   []*int
}

func iptr(i int) *int { return &i }

// comicGenreIDs: cmoa has no single "all manga" id, so the "comic" choice tries
// the manga genres in order and takes the first hit (shonen→seinen→shojo→josei→
// BL→TL→harlequin→adult→manga-mag).
var comicGenreIDs = []*int{iptr(12), iptr(13), iptr(20), iptr(2), iptr(24), iptr(34), iptr(21), iptr(11), iptr(28)}

// GenreChoices is the single source of truth for search categories. The
// frontend mirrors these keys; keep them in sync. Specifying a precise genre
// (e.g. "seinen") instead of "all"/"comic" avoids pulling a same-named work's
// metadata from the wrong category.
var GenreChoices = []Genre{
	{"all", "全ジャンル", []*int{nil}},
	{"comic", "コミック (全マンガを順引き)", comicGenreIDs},
	{"shonen", "少年マンガ", []*int{iptr(12)}},
	{"seinen", "青年マンガ", []*int{iptr(13)}},
	{"shojo", "少女マンガ", []*int{iptr(20)}},
	{"josei", "女性マンガ", []*int{iptr(2)}},
	{"bl", "BLマンガ", []*int{iptr(24)}},
	{"tl", "TLマンガ", []*int{iptr(34)}},
	{"harlequin", "ハーレクインコミックス", []*int{iptr(21)}},
	{"adult", "アダルトマンガ", []*int{iptr(11)}},
	{"mangamag", "マンガ雑誌", []*int{iptr(28)}},
	{"novel", "小説・実用書", []*int{iptr(1000)}},
	{"ranobe", "ライトノベル", []*int{iptr(1013)}},
	{"magazine", "雑誌", []*int{iptr(9925)}},
	{"photo", "写真集", []*int{iptr(9923)}},
}

// genreIDs returns the genre_id sequence for a type key, defaulting to "no
// filter" (all) when the key is unknown.
func genreIDs(key string) []*int {
	for _, g := range GenreChoices {
		if g.Key == key {
			return g.IDs
		}
	}
	return []*int{nil}
}

// excludedTags are non-genre promotional tags dropped from the result.
var excludedTags = map[string]bool{"広告掲載中": true}

// Meta is the normalized metadata extracted for one book.
type Meta struct {
	Title       string
	Series      string
	SeriesIndex float64
	Authors     []string
	Publisher   string
	PubDate     time.Time // zero when unknown
	Comments    string
	Tags        []string
	Category    string // cmoa top-page category, exactly as cmoa labels it (e.g. 少年マンガ); "" if unknown
	Rating      int    // 0-10 (Calibre scale, 2 per star); 0 when unknown
	Languages   []string
	CoverURL    string
}

// topCategory maps a cmoa genre_id to its top-page category label, taken
// verbatim from the single-genre GenreChoices — cmoa's own katakana labels
// (少年マンガ, 青年マンガ, …). Used to read a book's category off its breadcrumb
// (see parseBookPage → Meta.Category).
var topCategory = func() map[int]string {
	m := map[int]string{}
	for _, g := range GenreChoices {
		if len(g.IDs) == 1 && g.IDs[0] != nil {
			m[*g.IDs[0]] = g.Label
		}
	}
	return m
}()

// reGenreID pulls the numeric genre id out of a genre link (/search/genre/<id>/).
var reGenreID = regexp.MustCompile(`genre/(\d+)`)

// reRankSuffix strips a trailing "(9位)" / "（9位）" ranking off a category label.
var reRankSuffix = regexp.MustCompile(`[\s\x{3000}]*[(（]\d+位[)）][\s\x{3000}]*$`)

// CategoryTagPrefix namespaces the cmoa top-page category as a Calibre tag
// (e.g. "ジャンル:少年マンガ"), keeping it distinct from the fine genre tags.
const CategoryTagPrefix = "ジャンル:"

// CategoryTag returns the namespaced category tag, or "" when the category is
// unknown.
func (m *Meta) CategoryTag() string {
	if m.Category == "" {
		return ""
	}
	return CategoryTagPrefix + m.Category
}

// TagsWithCategory returns the fine genre tags with the category tag prepended
// when known (deduped) — the full tag set to persist for a fetched book.
func (m *Meta) TagsWithCategory() []string {
	ct := m.CategoryTag()
	if ct == "" {
		return m.Tags
	}
	out := make([]string, 0, len(m.Tags)+1)
	out = append(out, ct)
	for _, t := range m.Tags {
		if t != ct {
			out = append(out, t)
		}
	}
	return out
}

// Client fetches metadata from cmoa. Root is overridable for tests.
type Client struct {
	HTTP *http.Client
	Root string
}

// NewClient returns a Client with a sane timeout pointed at cmoa.jp.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, Root: defaultRoot}
}

func (c *Client) root() string {
	if c.Root != "" {
		return c.Root
	}
	return defaultRoot
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// httpGet GETs url with linear-backoff retries on transport errors.
//
// requireOK=true  : only HTTP 200 succeeds; other statuses retry, then ErrFetch.
// requireOK=false : any reachable response is returned regardless of status.
//
//	cmoa returns 404 for "no result in this genre"; we want to parse that body
//	to move on to the next genre, so the search step uses requireOK=false.
//
// Only a persistently failing transport yields ErrFetch.
func (c *Client) httpGet(ctx context.Context, url string, requireOK bool) (*goquery.Document, error) {
	const retries = 3
	var last string
	for attempt := 0; attempt < retries; attempt++ {
		doc, status, err := c.tryGet(ctx, url)
		if err != nil {
			last = err.Error()
		} else if !requireOK || status == http.StatusOK {
			return doc, nil
		} else {
			last = fmt.Sprintf("HTTP %d", status)
		}
		if attempt < retries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 600 * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("%w: %s: %s", ErrFetch, url, last)
}

func (c *Client) tryGet(ctx context.Context, url string) (*goquery.Document, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("Accept-Language", "ja,en-US;q=0.9,en;q=0.8")
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	doc, err := goquery.NewDocumentFromReader(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return doc, res.StatusCode, nil
}

// dashChars are hyphen/dash/long-bar variants treated as search-term separators:
// cmoa hits more reliably when "NARUTO―ナルト―" is searched as "NARUTO" AND
// "ナルト". The prolonged-sound mark ー(U+30FC) is part of a word, not included.
const dashChars = "-‐‑‒–—―−─━－"

var dashReplacer = func() *strings.Replacer {
	pairs := make([]string, 0, len([]rune(dashChars))*2)
	for _, r := range dashChars {
		pairs = append(pairs, string(r), " ")
	}
	return strings.NewReplacer(pairs...)
}()

var wsSplit = regexp.MustCompile(`[\s\x{3000}]+`)

// searchTokens splits a query on dashes/long-bars into AND tokens (drops empty).
func searchTokens(word string) []string {
	s := dashReplacer.Replace(word)
	var out []string
	for _, t := range wsSplit.Split(s, -1) {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// findBookURL searches the given genre (nil = all) for series and returns the
// first hit's book URL, or "" when there is no match.
func (c *Client) findBookURL(ctx context.Context, series string, genreID *int, add, exclude string) (string, error) {
	tokens := searchTokens(series)
	parts := make([]string, 0, len(tokens)+2)
	if len(tokens) > 0 {
		for _, t := range tokens {
			parts = append(parts, url.QueryEscape(t))
		}
	} else {
		parts = append(parts, url.QueryEscape(series))
	}
	if add != "" {
		parts = append(parts, url.QueryEscape(add))
	}
	if exclude != "" {
		parts = append(parts, "-"+url.QueryEscape(exclude))
	}
	word := strings.Join(parts, "+")
	searchURL := c.root() + searchPath + word
	if genreID != nil {
		searchURL += "&genre_id=" + strconv.Itoa(*genreID)
	}
	// Allow 404 (= "no result") so we can parse the body and try the next genre.
	doc, err := c.httpGet(ctx, searchURL, false)
	if err != nil {
		return "", err
	}
	a := doc.Find(".search_result_box_right_sec1 > p > a").First()
	if a.Length() == 0 {
		return "", nil
	}
	href, _ := a.Attr("href")
	if href == "" {
		return "", nil
	}
	return c.root() + href, nil
}

var (
	reTitleVolume   = regexp.MustCompile(`^(.+\S) (\d+)`)
	reSpaceVolume   = regexp.MustCompile(`^(.*\S)[\s\x{3000}]+(\d+)[\s\x{3000}]*$`)
	reBracketVolume = regexp.MustCompile(`^(.*\S)[\s\x{3000}]*[（(](\d+)[）)][\s\x{3000}]*$`)
	reNkan          = regexp.MustCompile(`(\d+)[\s\x{3000}]*巻`)
	reLatest        = regexp.MustCompile(`[\s\x{3000}]*[（(][\s\x{3000}]*最新刊[\s\x{3000}]*[）)][\s\x{3000}]*$`)
	reTitleSplit    = regexp.MustCompile(`[｜|]`)
	reReviewScore   = regexp.MustCompile(`[（(]\s*(\d+(?:\.\d+)?)\s*[）)]`)
)

// splitTitleVolume separates a trailing volume number from a title, handling
// "作品名 30" / "作品名　30" (half/full-width space) and "作品名（30）" / "作品名(30)".
func splitTitleVolume(title string) (work string, vol int, ok bool) {
	if m := reSpaceVolume.FindStringSubmatch(title); m != nil {
		v, _ := strconv.Atoi(m[2])
		return m[1], v, true
	}
	if m := reBracketVolume.FindStringSubmatch(title); m != nil {
		v, _ := strconv.Atoi(m[2])
		return m[1], v, true
	}
	return title, 0, false
}

// extractVolume pulls a volume number from a cmoa title string. It prefers a
// trailing number, else the last "N巻" (e.g. "ReLIFE 1巻【フルカラー】").
func extractVolume(title string) (int, bool) {
	if _, v, ok := splitTitleVolume(title); ok {
		return v, true
	}
	ms := reNkan.FindAllStringSubmatch(title, -1)
	if len(ms) > 0 {
		v, _ := strconv.Atoi(ms[len(ms)-1][1])
		return v, true
	}
	return 0, false
}

// formatVolume renders a volume for display: zero-padded to 2 digits, 3+ as-is.
func formatVolume(vol int) string { return fmt.Sprintf("%02d", vol) }

// normalizeTitle normalizes the volume notation in a cmoa title (half-width
// space separator, 2-digit zero pad). Titles without a volume are unchanged.
func normalizeTitle(title string) string {
	work, vol, ok := splitTitleVolume(title)
	if ok {
		return fmt.Sprintf("%s %s", work, formatVolume(vol))
	}
	return title
}

// Fetch looks up metadata for a title in the given genre type. add/exclude
// refine the cmoa search (extra/excluded word). It returns (nil, nil) when
// nothing matched, or ErrFetch when cmoa was unreachable.
func (c *Client) Fetch(ctx context.Context, title, genreType, add, exclude string) (*Meta, error) {
	series := title
	volume := 0
	hasVolume := false
	if m := reTitleVolume.FindStringSubmatch(title); m != nil {
		series = m[1]
		volume, _ = strconv.Atoi(m[2])
		hasVolume = true
	}

	var bookURL string
	for _, gid := range genreIDs(genreType) {
		u, err := c.findBookURL(ctx, series, gid, add, exclude)
		if err != nil {
			return nil, err // transport failure: distinct from "no match"
		}
		if u != "" {
			bookURL = u
			break
		}
	}
	if bookURL == "" {
		return nil, nil
	}
	if hasVolume {
		bookURL += fmt.Sprintf("vol/%d", volume)
	}
	doc, err := c.httpGet(ctx, bookURL, true)
	if err != nil {
		return nil, err
	}
	return parseBookPage(doc, series, volume, hasVolume), nil
}

// parseBookPage extracts metadata from a cmoa book page document.
func parseBookPage(doc *goquery.Document, series string, volume int, hasVolume bool) *Meta {
	meta := &Meta{Languages: []string{"日本語"}, Series: series}
	if hasVolume {
		meta.SeriesIndex = float64(volume)
	} else {
		meta.SeriesIndex = 1
	}

	// Per-row info table (出版社, 配信開始日, …), keyed by the left label cell.
	infos := map[string]*goquery.Selection{}
	doc.Find(".category_line").Each(func(_ int, e *goquery.Selection) {
		key := "tags2"
		if l := e.Find(".category_line_f_l_l"); l.Length() > 0 {
			key = strings.TrimSpace(l.Text())
		}
		if r := e.Find(".category_line_f_r_l"); r.Length() > 0 {
			infos[key] = r
		}
	})

	// Top-page category from the "ジャンル" info row, e.g.
	//   ジャンル ： <a href="/search/genre/13/">青年マンガ(9位)</a>
	// The link is the category (green nav category); the trailing "(N位)" is the
	// book's ranking within it, which we drop. Prefer the canonical label from
	// the genre_id, falling back to the link text with the ranking stripped.
	if r, ok := infos["ジャンル"]; ok {
		if a := r.Find("a").First(); a.Length() > 0 {
			if href, ok := a.Attr("href"); ok {
				if m := reGenreID.FindStringSubmatch(href); m != nil {
					if id, _ := strconv.Atoi(m[1]); topCategory[id] != "" {
						meta.Category = topCategory[id]
					}
				}
			}
			if meta.Category == "" {
				meta.Category = strings.TrimSpace(reRankSuffix.ReplaceAllString(a.Text(), ""))
			}
		}
	}

	if r, ok := infos["配信開始日"]; ok {
		// cmoa renders month/day without zero-padding ("2018年7月4日"), so use the
		// non-padded layout (Go's 1/2), which also accepts padded values.
		if t, err := time.Parse("2006年1月2日", firstText(r)); err == nil {
			meta.PubDate = t
		}
	}

	// Authors: cmoa may list several (原作/作画). Preserve order, drop dupes
	// (PC/mobile blocks repeat them). Calibre treats authors joined by " & ".
	seenAuthor := map[string]bool{}
	doc.Find(".title_details_author_name > a").Each(func(_ int, a *goquery.Selection) {
		name := strings.TrimSpace(a.Text())
		if name != "" && !seenAuthor[name] {
			seenAuthor[name] = true
			meta.Authors = append(meta.Authors, name)
		}
	})

	if d := doc.Find("#comic_description > p").First(); d.Length() > 0 {
		// Preserve cmoa's line breaks as <br> (Calibre comments are HTML) instead
		// of collapsing them, so the description reads the way it does on cmoa.
		meta.Comments = htmlWithBreaks(d)
	}

	// Genre tags. The page duplicates .genre_detail for PC/mobile; preserve
	// order, drop dupes, and skip non-genre promo tags.
	seenTag := map[string]bool{}
	doc.Find(".genre_detail > a").Each(func(_ int, a *goquery.Selection) {
		t := strings.TrimSpace(a.Text())
		if t != "" && !excludedTags[t] && !seenTag[t] {
			seenTag[t] = true
			meta.Tags = append(meta.Tags, t)
		}
	})

	// Rating: the aggregate review score lives in .reviewArea, e.g.
	// "（4.5） 投稿数850件". Take the value in parentheses (so the post count is
	// not picked up), round to the nearest 0.5 star, store on Calibre's 0-10
	// scale (×2). .title_details_review_point_star is for related works.
	if el := doc.Find(".reviewArea").First(); el.Length() > 0 {
		if m := reReviewScore.FindStringSubmatch(el.Text()); m != nil {
			if pt, err := strconv.ParseFloat(m[1], 64); err == nil && pt >= 0 {
				// Round to the nearest 0.5 star and store on Calibre's ×2 scale,
				// so 4.5 stars → 9. int(pt*2+0.5) does both at once.
				meta.Rating = int(pt*2 + 0.5)
				if meta.Rating > 10 {
					meta.Rating = 10
				}
			}
		}
	}

	if src, ok := doc.Find(".title_big_thum").First().Attr("src"); ok && src != "" {
		meta.CoverURL = withScheme(src)
	}

	if r, ok := infos["出版社"]; ok {
		if a := r.Find("a").First(); a.Length() > 0 {
			meta.Publisher = strings.TrimSpace(a.Text())
		}
	}

	// Series name: the breadcrumb's last link, e.g.
	//   TOP > 少年・青年マンガ > 青年マンガ > comico > ReLIFE【フルカラー】
	// The current volume "ReLIFE 1巻【フルカラー】" is plain text, not a link.
	seriesFromBC := ""
	doc.Find(".brCramb a").Each(func(_ int, a *goquery.Selection) {
		t := strings.TrimSpace(a.Text())
		if t != "" && !strings.Contains(t, "コミックシーモア") {
			seriesFromBC = t
		}
	})

	// Title from the page <title>: "作品名 巻数｜…｜著者"; take the part before ｜
	// and strip a trailing "（最新刊）".
	cmoaTitle := ""
	if pt := doc.Find("title").First(); pt.Length() > 0 {
		cmoaTitle = strings.TrimSpace(reTitleSplit.Split(strings.TrimSpace(pt.Text()), -1)[0])
		cmoaTitle = strings.TrimSpace(reLatest.ReplaceAllString(cmoaTitle, ""))
	}

	switch {
	case cmoaTitle != "" && seriesFromBC != "":
		// Series from breadcrumb, volume normalized from the cmoa title.
		meta.Series = seriesFromBC
		vol, ok := extractVolume(cmoaTitle)
		if !ok && hasVolume {
			vol, ok = volume, true
		}
		if ok {
			meta.Title = fmt.Sprintf("%s %s", seriesFromBC, formatVolume(vol))
		} else {
			meta.Title = seriesFromBC
		}
	case cmoaTitle != "":
		work, _, _ := splitTitleVolume(cmoaTitle)
		meta.Title = normalizeTitle(cmoaTitle)
		meta.Series = work
	case !hasVolume:
		meta.Title = series
		if seriesFromBC != "" {
			meta.Series = seriesFromBC
		}
	default:
		meta.Title = fmt.Sprintf("%s %s", series, formatVolume(volume))
		if seriesFromBC != "" {
			meta.Series = seriesFromBC
		}
	}
	return meta
}

// FetchCover downloads a cover image (raw bytes) from coverURL.
func (c *Client) FetchCover(ctx context.Context, coverURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coverURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	res, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: cover HTTP %d", ErrFetch, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, 16<<20))
}

// firstText returns the element's own direct text (not descendants'), trimmed.
// htmlWithBreaks renders a selection's content as minimal, safe HTML: text is
// escaped and <br> is kept as a line break, while other inline tags (ruby, span,
// …) are flattened to their text. Used for the description so line breaks survive
// into the stored Calibre comment (which is HTML).
func htmlWithBreaks(sel *goquery.Selection) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch {
		case n.Type == html.TextNode:
			b.WriteString(html.EscapeString(n.Data))
		case n.Type == html.ElementNode && n.Data == "br":
			b.WriteString("<br>")
		default:
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
	}
	for _, node := range sel.Nodes {
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	return strings.TrimSpace(b.String())
}

func firstText(sel *goquery.Selection) string {
	var b strings.Builder
	for _, n := range sel.Nodes {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.TextNode {
				b.WriteString(child.Data)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// withScheme prefixes a scheme-relative "//host/..." URL with https:.
func withScheme(src string) string {
	if strings.HasPrefix(src, "//") {
		return "https:" + src
	}
	return src
}
