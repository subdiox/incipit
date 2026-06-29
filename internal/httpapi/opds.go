package httpapi

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/calibre"
)

// Minimal OPDS 1.2 (Atom) catalog. Navigation feeds link to sub-catalogs;
// acquisition feeds list books with download + cover links.

const (
	opdsNav         = "application/atom+xml;profile=opds-catalog;kind=navigation"
	opdsAcquisition = "application/atom+xml;profile=opds-catalog;kind=acquisition"
)

type opdsFeed struct {
	XMLName   xml.Name    `xml:"feed"`
	Xmlns     string      `xml:"xmlns,attr"`
	XmlnsOPDS string      `xml:"xmlns:opds,attr"`
	ID        string      `xml:"id"`
	Title     string      `xml:"title"`
	Updated   string      `xml:"updated"`
	Links     []opdsLink  `xml:"link"`
	Entries   []opdsEntry `xml:"entry"`
}

type opdsLink struct {
	Rel   string `xml:"rel,attr,omitempty"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Title string `xml:"title,attr,omitempty"`
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
	feed := opdsFeed{
		ID:    "urn:incipit:root",
		Title: "Incipit Library",
		Links: []opdsLink{
			{Rel: "self", Href: "/opds", Type: opdsNav},
			{Rel: "start", Href: "/opds", Type: opdsNav},
			{Rel: "search", Href: "/opds/search?q={searchTerms}", Type: opdsAcquisition},
		},
		Entries: []opdsEntry{
			navEntry("urn:incipit:new", "Recently Added", "Newest books in the library", "/opds/new"),
			navEntry("urn:incipit:authors", "By Author", "Browse books by author", "/opds/authors"),
			navEntry("urn:incipit:series", "By Series", "Browse books by series", "/opds/series"),
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
	s.writeAcquisitionFeed(w, "urn:incipit:new", "Recently Added", "/opds/new", res.Books)
}

func (s *Server) handleOPDSSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	res, err := s.lib().ListBooks(r.Context(), calibre.ListOptions{Search: q, Limit: 100})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search")
		return
	}
	s.writeAcquisitionFeed(w, "urn:incipit:search", "Search: "+q, "/opds/search?q="+q, res.Books)
}

func (s *Server) handleOPDSAuthors(w http.ResponseWriter, r *http.Request) {
	authors, err := s.lib().Authors(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "authors")
		return
	}
	feed := opdsFeed{ID: "urn:incipit:authors", Title: "Authors",
		Links: []opdsLink{{Rel: "self", Href: "/opds/authors", Type: opdsNav}}}
	for _, a := range authors {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      fmt.Sprintf("urn:incipit:author:%d", a.ID),
			Title:   a.Name,
			Updated: time.Now().UTC().Format(time.RFC3339),
			Content: &opdsContent{Type: "text", Text: fmt.Sprintf("%d book(s)", a.Count)},
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
	s.writeAcquisitionFeed(w, fmt.Sprintf("urn:incipit:author:%d", id), "Author", fmt.Sprintf("/opds/authors/%d", id), res.Books)
}

func (s *Server) handleOPDSSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.lib().SeriesList(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "series")
		return
	}
	feed := opdsFeed{ID: "urn:incipit:series", Title: "Series",
		Links: []opdsLink{{Rel: "self", Href: "/opds/series", Type: opdsNav}}}
	for _, s2 := range series {
		feed.Entries = append(feed.Entries, opdsEntry{
			ID:      fmt.Sprintf("urn:incipit:series:%d", s2.ID),
			Title:   s2.Name,
			Updated: time.Now().UTC().Format(time.RFC3339),
			Content: &opdsContent{Type: "text", Text: fmt.Sprintf("%d book(s)", s2.Count)},
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
	s.writeAcquisitionFeed(w, fmt.Sprintf("urn:incipit:series:%d", id), "Series", fmt.Sprintf("/opds/series/%d", id), res.Books)
}

// writeAcquisitionFeed renders a list of books as an OPDS acquisition feed.
func (s *Server) writeAcquisitionFeed(w http.ResponseWriter, id, title, self string, books []calibre.Book) {
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
		feed.Entries = append(feed.Entries, entry)
	}
	writeOPDS(w, opdsAcquisition, feed)
}
