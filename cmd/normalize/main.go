// Command normalize rewrites metadata.db's text columns to Unicode NFC, fixing
// macOS-NFD values (e.g. a decomposed "ド") that won't match the NFC text users
// type into search. It normalizes books.title/sort/author_sort,
// series/authors/publishers/tags names+sort (merging NFD/NFC duplicates), and —
// with -comments — comments.text.
//
// It does NOT move or rename folders (books.path is left as-is), so it is safe
// to run on a large library without a filesystem storm. It is idempotent.
//
//	go run ./cmd/normalize -library /path [-dry-run] [-comments]
//
// Back up metadata.db and run it during a quiet period; incipit's own writes are
// serialized against it (WAL + busy_timeout), but a big commit briefly holds the
// write lock.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"incipit/internal/calibre"
)

func main() {
	lib := flag.String("library", os.Getenv("INCIPIT_LIBRARY"), "Calibre library path")
	dry := flag.Bool("dry-run", false, "scan and report, but don't write")
	comments := flag.Bool("comments", true, "also normalize comments.text")
	flag.Parse()
	if *lib == "" {
		log.Fatal("library path required (set INCIPIT_LIBRARY or -library)")
	}

	a, err := calibre.Open(*lib, false)
	if err != nil {
		log.Fatalf("open library: %v", err)
	}
	defer a.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	start := time.Now()
	log.Printf("normalize: scanning %s (dry=%v comments=%v)", *lib, *dry, *comments)
	rep, err := a.NormalizeText(ctx, *comments, *dry)
	if err != nil {
		log.Fatalf("normalize: %v", err)
	}

	tables := make([]string, 0, len(rep.Scanned))
	for t := range rep.Scanned {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	verb := "changed"
	if *dry {
		verb = "would change"
	}
	var totalChanged, totalMerged int
	for _, t := range tables {
		log.Printf("  %-11s scanned=%-7d %s=%-6d merged=%d", t, rep.Scanned[t], verb, rep.Changed[t], rep.Merged[t])
		totalChanged += rep.Changed[t]
		totalMerged += rep.Merged[t]
	}
	log.Printf("done: %s %d rows, merged %d duplicates in %s", verb, totalChanged, totalMerged, time.Since(start).Round(time.Millisecond))
	if *dry && (totalChanged > 0 || totalMerged > 0) {
		log.Print("dry-run — rerun without -dry-run to apply")
	}
}
