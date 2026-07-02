// Command categorize backfills each book's cmoa top-page category as a
// "ジャンル:<category>" tag. Category is a property of the work, so it looks up
// one unit per series (+ one per standalone book) and tags every volume.
//
// It is resumable (skips units already tagged), rate-limited (be polite to
// cmoa), and safe to run while incipit serves the same library (WAL +
// busy_timeout). Fine genres (バトル・アクション, アニメ化, …) are left untouched.
//
// Backfill everything (unfiltered cmoa search):
//
//	go run ./cmd/categorize -library /path [-dry-run] [-limit 30]
//
// Re-crawl a mis-assigned category — e.g. CBZ manga wrongly tagged
// ジャンル:ライトノベル (an unfiltered search matched the light-novel edition) —
// searching only the comic genres and replacing the tag:
//
//	go run ./cmd/categorize -library /path -recrawl ライトノベル -genre comic
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"incipit/internal/calibre"
	"incipit/internal/metadata"
)

type unit struct {
	name  string
	books []int64
}

func main() {
	lib := flag.String("library", os.Getenv("INCIPIT_LIBRARY"), "Calibre library path")
	prefix := flag.String("prefix", metadata.CategoryTagPrefix, "tag prefix for the category")
	genre := flag.String("genre", "all", "cmoa search genre key (all|comic|shonen|…)")
	recrawl := flag.String("recrawl", "", "re-crawl mode: fix books already tagged <prefix><this> (e.g. ライトノベル)")
	format := flag.String("format", "CBZ", "recrawl: only books with a file of this format")
	conc := flag.Int("concurrency", 3, "concurrent cmoa fetches")
	delay := flag.Duration("delay", 400*time.Millisecond, "cool-down after each unit (politeness)")
	limit := flag.Int("limit", 0, "process only the first N units (0 = all)")
	dry := flag.Bool("dry-run", false, "don't write tags, just report")
	resume := flag.Bool("resume", true, "backfill: skip units whose books already carry a prefix tag")
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

	var units []unit
	oldTag := "" // recrawl: the mis-assigned tag to replace
	if *recrawl != "" {
		oldTag = *prefix + *recrawl
		books, err := a.BooksWithTagAndFormat(ctx, oldTag, *format)
		if err != nil {
			log.Fatalf("recrawl scan: %v", err)
		}
		units = groupUnits(books)
		log.Printf("recrawl: %q (%s) → %d books in %d units, searching genre=%q",
			oldTag, *format, len(books), len(units), *genre)
	} else {
		units, err = backfillUnits(ctx, a, *prefix, *resume)
		if err != nil {
			log.Fatalf("build units: %v", err)
		}
	}
	if *limit > 0 && len(units) > *limit {
		units = units[:*limit]
	}
	log.Printf("units to process: %d (concurrency=%d delay=%s dry=%v prefix=%q genre=%q)",
		len(units), *conc, *delay, *dry, *prefix, *genre)

	var tagged, nomatch, nocat, errs, processed, unchanged int64
	cats := map[string]int64{}
	var catsMu sync.Mutex

	start := time.Now()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	go func() {
		for range tick.C {
			p := atomic.LoadInt64(&processed)
			rate := float64(p) / time.Since(start).Seconds()
			eta := time.Duration(0)
			if rate > 0 {
				eta = time.Duration(float64(len(units)-int(p))/rate) * time.Second
			}
			log.Printf("progress %d/%d  tagged=%d unchanged=%d nomatch=%d nocat=%d err=%d  %.1f/s  eta=%s",
				p, len(units), atomic.LoadInt64(&tagged), atomic.LoadInt64(&unchanged),
				atomic.LoadInt64(&nomatch), atomic.LoadInt64(&nocat), atomic.LoadInt64(&errs),
				rate, eta.Round(time.Second))
		}
	}()

	sem := make(chan struct{}, *conc)
	var wg sync.WaitGroup
	for _, u := range units {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(u unit) {
			defer wg.Done()
			defer func() { <-sem }()

			meta, err := client.Fetch(ctx, u.name, *genre, "", "")
			atomic.AddInt64(&processed, 1)
			switch {
			case ctx.Err() != nil:
				return
			case err != nil:
				atomic.AddInt64(&errs, 1)
			case meta == nil:
				atomic.AddInt64(&nomatch, 1)
			case meta.Category == "":
				atomic.AddInt64(&nocat, 1)
			default:
				newTag := *prefix + meta.Category
				if oldTag != "" && newTag == oldTag {
					// Comic search still yielded the same category — leave as-is.
					atomic.AddInt64(&unchanged, 1)
					return
				}
				catsMu.Lock()
				cats[meta.Category]++
				catsMu.Unlock()
				if *dry {
					if oldTag != "" {
						log.Printf("[dry] %-36.36s  %s → %s (%d vol)", u.name, oldTag, newTag, len(u.books))
					} else {
						log.Printf("[dry] %-40.40s -> %s (%d vol)", u.name, newTag, len(u.books))
					}
				} else {
					if oldTag != "" {
						if err := a.RemoveTagFromBooks(ctx, oldTag, u.books); err != nil {
							atomic.AddInt64(&errs, 1)
							log.Printf("remove %q from %q: %v", oldTag, u.name, err)
							return
						}
					}
					if err := a.AddTagToBooks(ctx, newTag, u.books); err != nil {
						atomic.AddInt64(&errs, 1)
						log.Printf("write %q: %v", u.name, err)
						return
					}
				}
				atomic.AddInt64(&tagged, 1)
			}
			select {
			case <-ctx.Done():
			case <-time.After(*delay):
			}
		}(u)
	}
	wg.Wait()

	log.Printf("done: processed=%d tagged=%d unchanged=%d nomatch=%d nocat=%d err=%d in %s",
		processed, tagged, unchanged, nomatch, nocat, errs, time.Since(start).Round(time.Second))
	catsMu.Lock()
	for c, n := range cats {
		fmt.Printf("  %-16s %d\n", c, n)
	}
	catsMu.Unlock()
	if ctx.Err() != nil {
		log.Print("interrupted — rerun to continue (backfill: -resume; recrawl reprocesses only remaining)")
	}
}

// backfillUnits builds one unit per series plus each standalone book, optionally
// skipping units already carrying a prefix tag (resume).
func backfillUnits(ctx context.Context, a *calibre.Adapter, prefix string, resume bool) ([]unit, error) {
	series, err := a.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	standalone, err := a.ListStandaloneBooks(ctx)
	if err != nil {
		return nil, err
	}
	units := make([]unit, 0, len(series)+len(standalone))
	for _, s := range series {
		units = append(units, unit{name: s.Name, books: s.Books})
	}
	for _, b := range standalone {
		units = append(units, unit{name: b.Title, books: []int64{b.ID}})
	}
	total := len(units)
	if resume {
		done, err := a.BooksWithTagPrefix(ctx, prefix)
		if err != nil {
			return nil, err
		}
		kept := units[:0]
		for _, u := range units {
			if len(u.books) > 0 && done[u.books[0]] {
				continue
			}
			kept = append(kept, u)
		}
		units = kept
	}
	log.Printf("backfill: %d units total, %d to process", total, len(units))
	return units, nil
}

// groupUnits folds tagged books into units: one per series, standalone books
// (series id 0) each on their own. The series name (or book title) is the query.
func groupUnits(books []calibre.TaggedBook) []unit {
	var units []unit
	bySeries := map[int64]int{}
	for _, b := range books {
		if b.SeriesID == 0 {
			units = append(units, unit{name: b.Title, books: []int64{b.ID}})
			continue
		}
		if i, ok := bySeries[b.SeriesID]; ok {
			units[i].books = append(units[i].books, b.ID)
			continue
		}
		bySeries[b.SeriesID] = len(units)
		units = append(units, unit{name: b.SeriesName, books: []int64{b.ID}})
	}
	return units
}
