package httpapi

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/calibre"
)

// Minimal OPDS 1.2 (Atom) catalog. Navigation feeds link to sub-catalogs;
// acquisition feeds list books with download + cover links.

const (
	opdsNav          = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcquisition  = "application/atom+xml;profile=opds-catalog;kind=acquisition"
	opdsOpenSearchCT = "application/opensearchdescription+xml"
)

type opdsFeed struct {
	XMLName   xml.Name    `xml:"feed"`
	Xmlns     string      `xml:"xmlns,attr"`
	XmlnsOPDS string      `xml:"xmlns:opds,attr"`
	XmlnsPSE  string      `xml:"xmlns:pse,attr"`
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Updated   string      `xml:"updated"`
	Links     []opdsLink  `xml:"link"`
	Entries   []opdsEntry `xml:"entry"`
}

type opdsLink struct {
	Rel string `xml:"rel,attr,omitempty"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
	// OPDS Page Streaming Extension: number of pages, on a stream link.
	PSECount int `xml:"pse:count,attr,omitempty"`
}

// opdsPSENamespace is the OPDS-PSE (page streaming) namespace; a stream link
// carries pse:count so comic readers can page through without downloading.
const opdsPSENamespace = "http://vaemendis.net/opds-pse/1.0"
const opdsPSEStreamRel = "http://vaemendis.net/opds-pse/stream"

// opdsL10n holds the OPDS catalog's user-facing strings for one language. Feeds
// are rendered in the authenticated user's UI language (Account → Language).
type opdsL10n struct {
	recentlyAdded, byAuthor, bySeries               string
	newestSummary, byAuthorSummary, bySeriesSummary string
	authors, series, author, searchTitle            string
	searchPrefix, booksFmt                          string
}

var opdsEN = opdsL10n{
	recentlyAdded: "Recently Added", byAuthor: "By Author", bySeries: "By Series",
	newestSummary: "Newest books in the library",
	byAuthorSummary: "Browse books by author", bySeriesSummary: "Browse books by series",
	authors: "Authors", series: "Series", author: "Author", searchTitle: "Search",
	searchPrefix: "Search: ", booksFmt: "%d book(s)",
}

var opdsJA = opdsL10n{
	recentlyAdded: "最近追加した本", byAuthor: "著者別", bySeries: "シリーズ別",
	newestSummary: "ライブラリの新着",
	byAuthorSummary: "著者で探す", bySeriesSummary: "シリーズで探す",
	authors: "著者", series: "シリーズ", author: "著者", searchTitle: "検索",
	searchPrefix: "検索: ", booksFmt: "%d冊",
}

// opdsL returns the localized strings for the authenticated user's language.
func opdsL(r *http.Request) opdsL10n {
	if u := currentUser(r); u != nil && u.Language == "ja" {
		return opdsJA
	}
	return opdsEN
}

type opdsEntry struct {
	ID      string       `xml:"id"`
	Title   string       `xml:"title"`
	Updated string       `xml:"updated"`
	Authors []opdsAuthor `xml:"author"`
	Content *opdsContent `xml:"content,omitempty"`
	Links   []opdsLink   `xml:"link"`
}

type opdsAuthor struct {
	Name string `xml:"name"`
}

type opdsContent struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

func writeOPDS(w http.ResponseWriter, kind string, feed opdsFeed) {
	feed.Xmlns = "http://www.w3.org/2005/Atom"
	feed.XmlnsOPDS = "http://opds-spec.org/2010/catalog"
	feed.XmlnsPSE = opdsPSENamespace
	if feed.Updated == "" {
		feed.Updated = time.Now().UTC().Format(time.RFC3339)
	}
	w.Header().Set("Content-Type", kind)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, xml.Header)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(feed)
}

func (s *Server) handleOPDSRoot(w http.ResponseWriter, r *http.Request) {
	l := opdsL(r)
	base := opdsBaseURL(r)
	feed := opdsFeed{
		ID:    "urn:incipit:root",
		Title: s.siteTitle(r.Context()),
		Links: []opdsLink{
			{Rel: "self", Href: "/opds", Type: opdsNav},
			{Rel: "start", Href: "/opds", Type: opdsNav},
			// Two search hints for maximum client coverage: an OpenSearch
			// description document, and a direct templated atom+xml link (which
			// some readers, e.g. Comic Share, use instead of parsing the OSD).
			{Rel: "search", Href: "/opds/opensearch.xml", Type: opdsOpenSearchCT, Title: l.searchTitle},
			{Rel: "search", Href: base + "/opds/search/{searchTerms}", Type: "application/atom+xml", Title: l.searchTitle},
		},
		Entries: []opdsEntry{
			navEntry("urn:incipit:new", l.recentlyAdded, l.newestSummary, "/opds/new"),
			navEntry("urn:incipit:authors", l.byAuthor, l.byAuthorSummary, "/opds/authors"),
			navEntry("urn:incipit:series", l.bySeries, l.bySeriesSummary, "/opds/series"),
		},
	}
	writeOPDS(w, opdsNav, feed)
}

func navEntry(id, title, summary, href string) opdsEntry {
	return opdsEntry{
		ID: id, Title: title, Updated: time.Now().UTC().Format(time.RFC3339),
		Content: &opdsContent{Type: "text", Text: summary},
		Links:   []opdsLink{{Rel: "subsection", Href: href, Type: opdsNav}},
	}
}

func (s *Server) handleOPDSNew(w http.ResponseWriter, r *http.Request) {
	res, err := s.lib().ListBooks(r.Context(), calibre.ListOptions{Sort: "timestamp", Desc: true, Limit: 50})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list books")
		return
	}
	s.writeAcquisitionFeed(w, r, "urn:incipit:new", opdsL(r).recentlyAdded, "/opds/new", res.Books)
}

// openSearchDescription is the OpenSearch 1.1 description document advertised by
// the catalog root so OPDS clients can discover the search template.
type openSearchDescription struct {
	XMLName        xml.Name        `xml:"http://a9.com/-/spec/opensearch/1.1/ OpenSearchDescription"`
	ShortName      string          `xml:"ShortName"`
	Description    string          `xml:"Description"`
	InputEncoding  string          `xml:"InputEncoding"`
	OutputEncoding string          `xml:"OutputEncoding"`
	URLs           []openSearchURL `xml:"Url"`
}

type openSearchURL struct {
	Type     string `xml:"type,attr"`
	Template string `xml:"template,attr"`
}

// opdsBaseURL reconstructs the externally-visible scheme+host (honouring the
// reverse proxy) so OpenSearch/PSE templates can be absolute, which stricter
// clients (e.g. Comic Share) require.
func opdsBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = p
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

func (s *Server) handleOPDSOpenSearch(w http.ResponseWriter, r *http.Request) {
	title := s.siteTitle(r.Context())
	base := opdsBaseURL(r)
	desc := openSearchDescription{
		ShortName:      title,
		Description:    "Search " + title,
		InputEncoding:  "UTF-8",
		OutputEncoding: "UTF-8",
		// calibre-web-style: path template, absolute URL, and a plain
		// application/atom+xml type. Some readers (Comic Share) reject the OPDS
		// profile media type or a query-string template.
		URLs: []openSearchURL{
			{Type: "application/atom+xml", Template: base + "/opds/search/{searchTerms}"},
		},
	}
	w.Header().Set("Content-Type", opdsOpenSearchCT)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, xml.Header)
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(desc)
}

// handleOPDSSearch handles the query-string form (/opds/search?q=…).
func (s *Server) handleOPDSSearch(w http.ResponseWriter, r *http.Request) {
	s.opdsSearch(w, r, r.URL.Query().Get("q"))
}

// handleOPDSSearchPath handles the calibre-web path form (/opds/search/{terms}),
// which is what some readers (Comic Share) generate from the OpenSearch template.
func (s *Server) handleOPDSSearchPath(w http.ResponseWriter, r *http.Request) {
	s.opdsSearch(w, r, chi.URLParam(r, "terms"))
}

func (s *Server) opdsSearch(w http.ResponseWriter, r *http.Request, q string) {
	q = strings.TrimSpace(q)
	res, err := s.lib().ListBooks(r.Context(), calibre.ListOptions{Search: q, Limit: 100})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search")
		return
	}
	s.writeAcquisitionFeed(w, r, "urn:incipit:search", opdsL(r).searchPrefix+q, "/opds/search/"+url.PathEscape(q), res.Books)
}

func (s *Server) handleOPDSAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := s.lib().Authors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authors")
		return
	}
	l := opdsL(r)
	feed := opdsFeed{ID: "urn:incipit:authors", Title: l.authors,
		Links: []opdsLink{{Rel: "self", Href: "/opds/authors", Type: opdsNav}}}
	for _, a := range authors {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      fmt.Sprintf("urn:incipit:author:%d", a.ID),
			Title:   a.Name,
			Updated: time.Now().UTC().Format(time.RFC3339),
			Content: &opdsContent{Type: "text", Text: fmt.Sprintf(l.booksFmt, a.Count)},
			Links:   []opdsLink{{Rel: "subsection", Href: fmt.Sprintf("/opds/authors/%d", a.ID), Type: opdsAcquisition}},
		})
	}
	writeOPDS(w, opdsNav, feed)
}

func (s *Server) handleOPDSAuthor(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	res, err := s.lib().ListBooks(r.Context(), calibre.ListOptions{AuthorID: id, Sort: "series", Limit: 500})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "author books")
		return
	}
	title := opdsL(r).author
	if len(res.Books) > 0 && len(res.Books[0].Authors) > 0 {
		title = res.Books[0].Authors[0].Name
	}
	s.writeAcquisitionFeed(w, r, fmt.Sprintf("urn:incipit:author:%d", id), title, fmt.Sprintf("/opds/authors/%d", id), res.Books)
}

func (s *Server) handleOPDSSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.lib().SeriesList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series")
		return
	}
	l := opdsL(r)
	feed := opdsFeed{ID: "urn:incipit:series", Title: l.series,
		Links: []opdsLink{{Rel: "self", Href: "/opds/series", Type: opdsNav}}}
	for _, s2 := range series {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      fmt.Sprintf("urn:incipit:series:%d", s2.ID),
			Title:   s2.Name,
			Updated: time.Now().UTC().Format(time.RFC3339),
			Content: &opdsContent{Type: "text", Text: fmt.Sprintf(l.booksFmt, s2.Count)},
			Links:   []opdsLink{{Rel: "subsection", Href: fmt.Sprintf("/opds/series/%d", s2.ID), Type: opdsAcquisition}},
		})
	}
	writeOPDS(w, opdsNav, feed)
}

func (s *Server) handleOPDSSeriesBooks(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	res, err := s.lib().ListBooks(r.Context(), calibre.ListOptions{SeriesID: id, Sort: "series", Limit: 500})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series books")
		return
	}
	title := opdsL(r).series
	if len(res.Books) > 0 && res.Books[0].Series != nil {
		title = res.Books[0].Series.Name
	}
	s.writeAcquisitionFeed(w, r, fmt.Sprintf("urn:incipit:series:%d", id), title, fmt.Sprintf("/opds/series/%d", id), res.Books)
}

// writeAcquisitionFeed renders a list of books as an OPDS acquisition feed. CBZ
// books also get an OPDS-PSE stream link so readers can page through without
// downloading the whole archive.
func (s *Server) writeAcquisitionFeed(w http.ResponseWriter, r *http.Request, id, title, self string, books []calibre.Book) {
	base := opdsBaseURL(r)
	feed := opdsFeed{
		ID: id, Title: title,
		Links: []opdsLink{
			{Rel: "self", Href: self, Type: opdsAcquisition},
			{Rel: "start", Href: "/opds", Type: opdsNav},
		},
	}
	for _, b := range books {
		entry := opdsEntry{
			ID:      fmt.Sprintf("urn:incipit:book:%d", b.ID),
			Title:   b.Title,
			Updated: b.LastModified.UTC().Format(time.RFC3339),
			Links: []opdsLink{
				{Rel: "http://opds-spec.org/acquisition", Href: fmt.Sprintf("/api/books/%d/file", b.ID), Type: "application/vnd.comicbook+zip"},
			},
		}
		if entry.Updated == "0001-01-01T00:00:00Z" {
			entry.Updated = time.Now().UTC().Format(time.RFC3339)
		}
		for _, a := range b.Authors {
			entry.Authors = append(entry.Authors, opdsAuthor{Name: a.Name})
		}
		if b.Comments != "" {
			entry.Content = &opdsContent{Type: "text", Text: b.Comments}
		}
		if b.HasCover {
			entry.Links = append(entry.Links,
				opdsLink{Rel: "http://opds-spec.org/image", Href: fmt.Sprintf("/api/books/%d/cover", b.ID), Type: "image/jpeg"},
				opdsLink{Rel: "http://opds-spec.org/image/thumbnail", Href: fmt.Sprintf("/api/books/%d/thumbnail", b.ID), Type: "image/jpeg"},
			)
		}
		// Page-streaming link: {pageNumber} is a template the reader fills in. The
		// URL is absolute and under /opds so readers (e.g. Comic Share) resolve it
		// against the catalog and send the same Basic-auth credentials. Page count
		// comes from the cached page list, so it's cheap after the first scan; a
		// book with no readable pages simply gets no stream link.
		if pages, err := s.cbzPages(r, &b); err == nil && len(pages) > 0 {
			entry.Links = append(entry.Links, opdsLink{
				Rel:      opdsPSEStreamRel,
				Href:     fmt.Sprintf("%s/opds/books/%d/page/{pageNumber}", base, b.ID),
				Type:     "image/jpeg",
				PSECount: len(pages),
			})
		}
		feed.Entries = append(feed.Entries, entry)
	}
	writeOPDS(w, opdsAcquisition, feed)
}
