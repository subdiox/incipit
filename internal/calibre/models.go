package calibre

import (
	"strings"
	"time"
)

// Book is a fully-hydrated book record assembled from metadata.db.
type Book struct {
	ID           int64             `json:"id"`
	Title        string            `json:"title"`
	Sort         string            `json:"sort"`
	Timestamp    time.Time         `json:"timestamp"`
	PubDate      time.Time         `json:"pubdate"`
	SeriesIndex  float64           `json:"seriesIndex"`
	AuthorSort   string            `json:"authorSort"`
	Path         string            `json:"path"`
	UUID         string            `json:"uuid"`
	HasCover     bool              `json:"hasCover"`
	LastModified time.Time         `json:"lastModified"`
	Authors      []Author          `json:"authors"`
	Series       *Series           `json:"series,omitempty"`
	Tags         []Tag             `json:"tags"`
	Publisher    *Publisher        `json:"publisher,omitempty"`
	Languages    []string          `json:"languages"`
	Rating       int               `json:"rating"` // 0..10, where 10 == 5 stars
	Identifiers  map[string]string `json:"identifiers"`
	Comments     string            `json:"comments"`
	Formats      []Format          `json:"formats"`
}

// Author of a book.
type Author struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort string `json:"sort"`
}

// Series a book belongs to.
type Series struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort string `json:"sort"`
}

// Tag attached to a book.
type Tag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Publisher of a book.
type Publisher struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Sort string `json:"sort"`
}

// Format is a stored file for a book (e.g. CBZ). Name is the on-disk basename
// without extension, matching Calibre's data.name column.
type Format struct {
	Format string `json:"format"`
	Size   int64  `json:"size"`
	Name   string `json:"name"`
}

// FormatFile returns the on-disk filename for a format, e.g. "Title - Author.cbz".
func (f Format) FormatFile() string {
	return f.Name + "." + strings.ToLower(f.Format)
}

// Facet is a browseable category value with a count of books.
type Facet struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// calibreTimeLayouts are the formats Calibre uses for its TIMESTAMP columns.
var calibreTimeLayouts = []string{
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05.999999+00:00",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// parseCalibreTime leniently parses a Calibre timestamp string.
func parseCalibreTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range calibreTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// formatCalibreTime renders a time in Calibre's canonical TIMESTAMP format.
func formatCalibreTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.999999-07:00")
}
