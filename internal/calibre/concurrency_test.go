package calibre

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestConcurrentAddBook verifies the serialized writer keeps metadata.db and the
// filesystem consistent under concurrent imports. Run with -race.
func TestConcurrentAddBook(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()

	const n = 25
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := a.AddBook(ctx, AddBookInput{
				Title:   fmt.Sprintf("Comic %d", i),
				Authors: []string{fmt.Sprintf("Author %d", i%3)},
				Format:  "CBZ",
				Data:    bytes.NewReader([]byte("PK\x03\x04data")),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent AddBook: %v", err)
		}
	}

	res, err := a.ListBooks(ctx, ListOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != n {
		t.Fatalf("expected %d books, got %d", n, res.Total)
	}

	// Every book has a unique, non-empty path and exactly one CBZ format.
	seen := map[string]bool{}
	for _, b := range res.Books {
		if b.Path == "" || seen[b.Path] {
			t.Fatalf("bad/duplicate path %q", b.Path)
		}
		seen[b.Path] = true
		if len(b.Formats) != 1 {
			t.Errorf("book %d formats = %+v", b.ID, b.Formats)
		}
	}
}
