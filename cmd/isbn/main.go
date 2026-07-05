// Command isbn backfills each book's ISBN (and derived Amazon ASIN) from
// コミックシーモア, attaching them to Calibre's identifiers table. Unlike the
// category backfill, ISBN is a property of the individual volume, so it looks up
// one cmoa page PER BOOK (with its volume) rather than per series.
//
// It is HARM-FREE: it only ADDS the "isbn"/"amazon" identifiers, never touching
// title, authors or any existing field. To avoid attaching a WRONG ISBN (which
// would later masquerade as a confident key), it searches comic genres only and
// requires a precision match — the normalized work title must agree, and when
// both sides list an author, at least one author must overlap (this rejects the
// "same title, different author" trap).
//
// Resumable (skips books that already have an ISBN), rate-limited, and safe to
// run while incipit serves the same library (WAL + busy_timeout).
//
//	go run ./cmd/isbn -library /path [-dry-run] [-limit 50] [-genre comic]
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"incipit/internal/calibre"
	"incipit/internal/metadata"
)

func main() {
	lib := flag.String("library", os.Getenv("INCIPIT_LIBRARY"), "Calibre library path")
	genre := flag.String("genre", "comic", "cmoa search genre key (comic|all|shonen|…); comic keeps precision high")
	conc := flag.Int("concurrency", 3, "concurrent cmoa fetches")
	delay := flag.Duration("delay", 300*time.Millisecond, "cool-down after each book (politeness)")
	limit := flag.Int("limit", 0, "process only the first N books (0 = all)")
	dry := flag.Bool("dry-run", false, "don't write, just report")
	resume := flag.Bool("resume", true, "skip books that already carry an ISBN identifier")
	flag.Parse()
	if *lib == "" {
		log.Fatal("library path required (set INCIPIT_LIBRARY or -library)")
	}

	a, err := calibre.Open(*lib, false)
	if err != nil {
		log.Fatalf("open library: %v", err)
	}
	defer a.Close()
	client := metadata.NewClient()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ids, err := a.FilteredIDs(ctx, calibre.ListOptions{})
	if err != nil {
		log.Fatalf("list ids: %v", err)
	}
	todo := ids
	if *resume {
		done, err := a.BooksWithIdentifier(ctx, "isbn")
		if err != nil {
			log.Fatalf("resume scan: %v", err)
		}
		todo = todo[:0]
		for _, id := range ids {
			if !done[id] {
				todo = append(todo, id)
			}
		}
		log.Printf("resume: %d books total, %d already have an ISBN, %d to process", len(ids), len(done), len(todo))
	}
	if *limit > 0 && len(todo) > *limit {
		todo = todo[:*limit]
	}
	log.Printf("books to process: %d (concurrency=%d delay=%s dry=%v genre=%q)",
		len(todo), *conc, *delay, *dry, *genre)

	var attached, nomatch, mismatch, noisbn, errs, processed int64

	start := time.Now()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	go func() {
		for range tick.C {
			p := atomic.LoadInt64(&processed)
			rate := float64(p) / time.Since(start).Seconds()
			eta := time.Duration(0)
			if rate > 0 {
				eta = time.Duration(float64(len(todo)-int(p))/rate) * time.Second
			}
			log.Printf("progress %d/%d  attached=%d mismatch=%d nomatch=%d noisbn=%d err=%d  %.1f/s  eta=%s",
				p, len(todo), atomic.LoadInt64(&attached), atomic.LoadInt64(&mismatch),
				atomic.LoadInt64(&nomatch), atomic.LoadInt64(&noisbn), atomic.LoadInt64(&errs),
				rate, eta.Round(time.Second))
		}
	}()

	sem := make(chan struct{}, *conc)
	var wg sync.WaitGroup
	for _, id := range todo {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			defer func() { <-sem }()

			b, err := a.GetBook(ctx, id)
			if err != nil || b == nil {
				atomic.AddInt64(&errs, 1)
				return
			}
			meta, err := client.Fetch(ctx, b.Title, *genre, "", "")
			atomic.AddInt64(&processed, 1)
			switch {
			case ctx.Err() != nil:
				return
			case err != nil:
				atomic.AddInt64(&errs, 1)
			case meta == nil:
				atomic.AddInt64(&nomatch, 1)
			case !matches(b, meta):
				atomic.AddInt64(&mismatch, 1)
			case meta.ISBN == "":
				atomic.AddInt64(&noisbn, 1)
			default:
				if *dry {
					log.Printf("[dry] %-40.40s -> isbn=%s asin=%s", b.Title, meta.ISBN, metadata.ISBNToASIN(meta.ISBN))
				} else if err := attach(ctx, a, b, meta.ISBN); err != nil {
					atomic.AddInt64(&errs, 1)
					log.Printf("write %d %q: %v", id, b.Title, err)
					return
				}
				atomic.AddInt64(&attached, 1)
			}
			select {
			case <-ctx.Done():
			case <-time.After(*delay):
			}
		}(id)
	}
	wg.Wait()

	log.Printf("done: processed=%d attached=%d mismatch=%d nomatch=%d noisbn=%d err=%d in %s",
		processed, attached, mismatch, nomatch, noisbn, errs, time.Since(start).Round(time.Second))
	if ctx.Err() != nil {
		log.Print("interrupted — rerun to continue (resumes: books with an ISBN are skipped)")
	}
}

// attach adds the isbn identifier (plus the derived Amazon ASIN) to the book,
// preserving any identifiers it already has. It never changes another field.
func attach(ctx context.Context, a *calibre.Adapter, b *calibre.Book, isbn string) error {
	ids := map[string]string{}
	for k, v := range b.Identifiers {
		ids[k] = v
	}
	ids["isbn"] = isbn
	if asin := metadata.ISBNToASIN(isbn); asin != "" {
		ids["amazon"] = asin
	}
	_, err := a.UpdateBook(ctx, b.ID, calibre.UpdateBookInput{Identifiers: &ids})
	return err
}

// matches is the precision guard: the book's normalized work title must equal
// the cmoa hit's title or series, and — when both list an author — at least one
// author must overlap. This keeps a same-titled but different work (esp. a
// different author) from getting the wrong ISBN.
func matches(b *calibre.Book, m *metadata.Meta) bool {
	bk := titleKey(b.Title)
	if bk == "" || (bk != titleKey(m.Title) && bk != titleKey(m.Series)) {
		return false
	}
	if len(b.Authors) > 0 && len(m.Authors) > 0 {
		return authorsOverlap(b.Authors, m.Authors)
	}
	return true
}

func authorsOverlap(bookAuthors []calibre.Author, metaAuthors []string) bool {
	set := make(map[string]bool, len(bookAuthors))
	for _, a := range bookAuthors {
		if k := authorKey(a.Name); k != "" {
			set[k] = true
		}
	}
	for _, name := range metaAuthors {
		if set[authorKey(name)] {
			return true
		}
	}
	return false
}

var (
	reBracket  = regexp.MustCompile(`[【（(\[［].*?[】）)\]］]`)
	reTrailVol = regexp.MustCompile(`[\s]*[0-9]+\s*巻?\s*$`)
)

// titleKey normalizes a title for comparison: fold full-width to half-width,
// drop bracketed decorations (【フルカラー】 等) and a trailing volume number,
// remove whitespace, lowercase.
func titleKey(s string) string {
	s = foldWidth(s)
	s = reBracket.ReplaceAllString(s, "")
	s = reTrailVol.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return strings.ToLower(strings.TrimSpace(s))
}

func authorKey(s string) string {
	s = foldWidth(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	return strings.ToLower(s)
}

// foldWidth maps full-width ASCII (！-～) and the ideographic space to their
// half-width forms, so "ＮＡＲＵＴＯ" and "NARUTO" compare equal.
func foldWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			return r - 0xFEE0
		case r == 0x3000:
			return ' '
		}
		return r
	}, s)
}
