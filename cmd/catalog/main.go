// Command catalog exports the Calibre library's catalog (目録) to CSV — one row
// per book, with the fields most useful for an inventory: title, authors,
// series + volume, publisher, publication date, rating, tags, languages,
// identifiers (ISBN/Amazon), format + size, and the on-disk path.
//
// It opens the library READ-ONLY and never touches metadata.db, so it is safe
// to run at any time, even while incipit is serving the same library.
//
//	go run ./cmd/catalog -library /path -o catalog.csv
//	INCIPIT_LIBRARY=./library go run ./cmd/catalog > catalog.csv
//
// The output is UTF-8 with a BOM by default so Excel opens Japanese titles
// correctly; pass -bom=false for a clean stream to feed another program.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"log"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"incipit/internal/calibre"
)

// hydrateChunk is how many books we fully load per batch. Hydration issues one
// IN(...) query per relation, so batching keeps those placeholder lists (and
// memory) bounded on large libraries while still amortizing round-trips.
const hydrateChunk = 500

func main() {
	lib := flag.String("library", os.Getenv("INCIPIT_LIBRARY"), "Calibre library path")
	out := flag.String("o", "", "output CSV file (default: stdout)")
	bom := flag.Bool("bom", true, "prepend a UTF-8 BOM so Excel reads Japanese text correctly")
	flag.Parse()
	if *lib == "" {
		log.Fatal("library path required (set INCIPIT_LIBRARY or -library)")
	}

	a, err := calibre.Open(*lib, true) // read-only: catalog export never writes
	if err != nil {
		log.Fatalf("open library: %v", err)
	}
	defer a.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ids, err := a.FilteredIDs(ctx, calibre.ListOptions{})
	if err != nil {
		log.Fatalf("list books: %v", err)
	}

	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			log.Fatalf("create %s: %v", *out, err)
		}
		defer f.Close()
		w = f
	}
	if *bom {
		if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			log.Fatalf("write BOM: %v", err)
		}
	}

	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		log.Fatalf("write header: %v", err)
	}

	var n int
	for start := 0; start < len(ids); start += hydrateChunk {
		if err := ctx.Err(); err != nil {
			log.Fatalf("aborted: %v", err)
		}
		end := start + hydrateChunk
		if end > len(ids) {
			end = len(ids)
		}
		books, err := a.BooksByIDs(ctx, ids[start:end])
		if err != nil {
			log.Fatalf("load books %d..%d: %v", start, end, err)
		}
		for i := range books {
			if err := cw.Write(row(&books[i])); err != nil {
				log.Fatalf("write row: %v", err)
			}
			n++
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		log.Fatalf("flush csv: %v", err)
	}
	log.Printf("exported %d book(s)", n)
}

var header = []string{
	"id", "title", "title_sort", "authors", "author_sort",
	"series", "series_index", "publisher", "pubdate", "added",
	"rating", "tags", "languages", "isbn", "amazon", "identifiers",
	"formats", "size_bytes", "path", "uuid", "comments",
}

// row renders one book as a CSV record aligned with header.
func row(b *calibre.Book) []string {
	return []string{
		strconv.FormatInt(b.ID, 10),
		b.Title,
		b.Sort,
		authorNames(b.Authors),
		b.AuthorSort,
		seriesName(b.Series),
		seriesIndex(b.SeriesIndex, b.Series),
		publisherName(b.Publisher),
		dateOnly(b),
		date(b.Timestamp),
		stars(b.Rating),
		tagNames(b.Tags),
		strings.Join(b.Languages, ", "),
		b.Identifiers["isbn"],
		b.Identifiers["amazon"],
		identifiers(b.Identifiers),
		formats(b.Formats),
		strconv.FormatInt(totalSize(b.Formats), 10),
		b.Path,
		b.UUID,
		oneLine(b.Comments),
	}
}

func authorNames(as []calibre.Author) string {
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = a.Name
	}
	return strings.Join(names, " & ") // Calibre's author separator
}

func tagNames(ts []calibre.Tag) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

func seriesName(s *calibre.Series) string {
	if s == nil {
		return ""
	}
	return s.Name
}

// seriesIndex renders the volume number, but only for books that are actually
// in a series — a standalone book keeps Calibre's default index of 1.0, which
// would be meaningless (and misleading) in the catalog.
func seriesIndex(idx float64, s *calibre.Series) string {
	if s == nil {
		return ""
	}
	return strconv.FormatFloat(idx, 'f', -1, 64)
}

func publisherName(p *calibre.Publisher) string {
	if p == nil {
		return ""
	}
	return p.Name
}

// stars converts Calibre's 0..10 rating (×2 scale) to a star value, blank when
// unrated.
func stars(r int) string {
	if r <= 0 {
		return ""
	}
	return strconv.FormatFloat(float64(r)/2, 'f', -1, 64)
}

// dateOnly renders the publication date as YYYY-MM-DD, blank for Calibre's
// "undefined" sentinel or an unset date.
func dateOnly(b *calibre.Book) string {
	t := b.PubDate
	if t.IsZero() || t.Year() <= 101 {
		return ""
	}
	return t.Format("2006-01-02")
}

func date(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// identifiers renders every identifier as "type:value" pairs, sorted for a
// stable column across runs.
func identifiers(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + ":" + m[k]
	}
	return strings.Join(parts, "; ")
}

func formats(fs []calibre.Format) string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = f.Format
	}
	return strings.Join(names, ", ")
}

func totalSize(fs []calibre.Format) int64 {
	var n int64
	for _, f := range fs {
		n += f.Size
	}
	return n
}

// oneLine collapses newlines so an HTML/multiline comment stays within a single
// CSV cell (csv already quotes, but a linebreak inside a field trips up naive
// spreadsheet importers).
func oneLine(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}
