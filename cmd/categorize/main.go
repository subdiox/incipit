// Command categorize backfills each book's cmoa top-page category as a
// "ジャンル:<category>" tag. Category is a property of the work, so it looks up
// one unit per series (+ one per standalone book) and tags every volume.
//
// It is resumable (skips units already tagged), rate-limited (be polite to
// cmoa), and safe to run while incipit serves the same library (WAL +
// busy_timeout). Fine genres (バトル・アクション, アニメ化, …) are left untouched.
//
//	go run ./cmd/categorize -library /path/to/library [-dry-run] [-limit 30]
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
	prefix := flag.String("prefix", "ジャンル:", "tag prefix for the category")
	conc := flag.Int("concurrency", 3, "concurrent cmoa fetches")
	delay := flag.Duration("delay", 400*time.Millisecond, "cool-down after each unit (politeness)")
	limit := flag.Int("limit", 0, "process only the first N units (0 = all)")
	dry := flag.Bool("dry-run", false, "don't write tags, just report")
	resume := flag.Bool("resume", true, "skip units whose books already carry a prefix tag")
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

	// Build the work list: one unit per series, plus each standalone book.
	series, err := a.ListSeries(ctx)
	if err != nil {
		log.Fatalf("list series: %v", err)
	}
	standalone, err := a.ListStandaloneBooks(ctx)
	if err != nil {
		log.Fatalf("list standalone: %v", err)
	}
	units := make([]unit, 0, len(series)+len(standalone))
	for _, s := range series {
		units = append(units, unit{name: s.Name, books: s.Books})
	}
	for _, b := range standalone {
		units = append(units, unit{name: b.Title, books: []int64{b.ID}})
	}
	total := len(units)

	if *resume {
		done, err := a.BooksWithTagPrefix(ctx, *prefix)
		if err != nil {
			log.Fatalf("resume scan: %v", err)
		}
		kept := units[:0]
		for _, u := range units {
			// A unit's tag write is atomic, so its first book being tagged means
			// the whole unit is done.
			if len(u.books) > 0 && done[u.books[0]] {
				continue
			}
			kept = append(kept, u)
		}
		units = kept
	}
	if *limit > 0 && len(units) > *limit {
		units = units[:*limit]
	}

	log.Printf("units: %d total, %d to process (concurrency=%d delay=%s dry=%v prefix=%q)",
		total, len(units), *conc, *delay, *dry, *prefix)

	var tagged, nomatch, nocat, errs, processed int64
	cats := map[string]int64{}
	var catsMu sync.Mutex

	// Progress ticker.
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
			log.Printf("progress %d/%d  tagged=%d nomatch=%d nocat=%d err=%d  %.1f/s  eta=%s",
				p, len(units), atomic.LoadInt64(&tagged), atomic.LoadInt64(&nomatch),
				atomic.LoadInt64(&nocat), atomic.LoadInt64(&errs), rate, eta.Round(time.Second))
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

			meta, err := client.Fetch(ctx, u.name, "all", "", "")
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
				tag := *prefix + meta.Category
				catsMu.Lock()
				cats[meta.Category]++
				catsMu.Unlock()
				if *dry {
					log.Printf("[dry] %-40.40s -> %s (%d vol)", u.name, tag, len(u.books))
				} else if err := a.AddTagToBooks(ctx, tag, u.books); err != nil {
					atomic.AddInt64(&errs, 1)
					log.Printf("write %q: %v", u.name, err)
					return
				}
				atomic.AddInt64(&tagged, 1)
			}
			// Politeness cool-down, holding the concurrency slot.
			select {
			case <-ctx.Done():
			case <-time.After(*delay):
			}
		}(u)
	}
	wg.Wait()

	log.Printf("done: processed=%d tagged=%d nomatch=%d nocat=%d err=%d in %s",
		processed, tagged, nomatch, nocat, errs, time.Since(start).Round(time.Second))
	// Category distribution.
	catsMu.Lock()
	for c, n := range cats {
		fmt.Printf("  %-16s %d\n", c, n)
	}
	catsMu.Unlock()
	if ctx.Err() != nil {
		log.Print("interrupted — rerun with -resume to continue")
	}
}
