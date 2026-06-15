package calibre

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	dir := t.TempDir()
	a, err := Open(dir, false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func addSample(t *testing.T, a *Adapter, title string, authors []string) *Book {
	t.Helper()
	b, err := a.AddBook(context.Background(), AddBookInput{
		Title:       title,
		Authors:     authors,
		Series:      "Foundation",
		SeriesIndex: 2,
		Tags:        []string{"scifi", "classic"},
		Publisher:   "Gnome Press",
		Languages:   []string{"eng"},
		Rating:      8,
		Comments:    "A great comic.",
		Identifiers: map[string]string{"isbn": "9780553293357"},
		PubDate:     time.Date(1951, 1, 1, 0, 0, 0, 0, time.UTC),
		Format:      "CBZ",
		Data:        bytes.NewReader([]byte("PK\x03\x04fake-cbz-bytes")),
		Cover:       []byte("\xff\xd8\xff\xe0fake-jpeg"),
	})
	if err != nil {
		t.Fatalf("AddBook(%q): %v", title, err)
	}
	return b
}

func TestAddBookCreatesFilesAndMetadata(t *testing.T) {
	a := newTestAdapter(t)
	b := addSample(t, a, "The Caves of Steel", []string{"Isaac Asimov"})

	if b.ID == 0 || b.UUID == "" {
		t.Fatalf("expected id and uuid, got id=%d uuid=%q", b.ID, b.UUID)
	}
	if b.Sort != "Caves of Steel, The" {
		t.Errorf("sort = %q, want %q", b.Sort, "Caves of Steel, The")
	}
	if b.AuthorSort != "Asimov, Isaac" {
		t.Errorf("author_sort = %q, want %q", b.AuthorSort, "Asimov, Isaac")
	}

	// Folder layout: <Author>/<Title> (id)/
	wantRel := "Isaac Asimov/The Caves of Steel (" + itoa(b.ID) + ")"
	if b.Path != wantRel {
		t.Errorf("path = %q, want %q", b.Path, wantRel)
	}
	folder := a.BookFolder(b)
	mustExist(t, filepath.Join(folder, "The Caves of Steel - Isaac Asimov.cbz"))
	mustExist(t, filepath.Join(folder, "cover.jpg"))
	opf := filepath.Join(folder, "metadata.opf")
	mustExist(t, opf)

	opfBytes, _ := os.ReadFile(opf)
	if !bytes.Contains(opfBytes, []byte("The Caves of Steel")) ||
		!bytes.Contains(opfBytes, []byte(b.UUID)) {
		t.Errorf("opf missing title/uuid:\n%s", opfBytes)
	}

	// Hydration.
	if len(b.Authors) != 1 || b.Authors[0].Name != "Isaac Asimov" {
		t.Errorf("authors = %+v", b.Authors)
	}
	if b.Series == nil || b.Series.Name != "Foundation" || b.SeriesIndex != 2 {
		t.Errorf("series = %+v idx=%v", b.Series, b.SeriesIndex)
	}
	if len(b.Tags) != 2 {
		t.Errorf("tags = %+v", b.Tags)
	}
	if b.Publisher == nil || b.Publisher.Name != "Gnome Press" {
		t.Errorf("publisher = %+v", b.Publisher)
	}
	if b.Rating != 8 {
		t.Errorf("rating = %d", b.Rating)
	}
	if b.Identifiers["isbn"] != "9780553293357" {
		t.Errorf("identifiers = %+v", b.Identifiers)
	}
	if len(b.Formats) != 1 || b.Formats[0].Format != "CBZ" || b.Formats[0].Size == 0 {
		t.Errorf("formats = %+v", b.Formats)
	}
	if !b.HasCover {
		t.Error("expected has_cover")
	}
}

func TestListBooksEmptyReturnsNonNilSlice(t *testing.T) {
	a := newTestAdapter(t)
	res, err := a.ListBooks(context.Background(), ListOptions{})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("total = %d, want 0", res.Total)
	}
	// Must be a non-nil empty slice so it serializes as [] (not null), which
	// would crash clients doing books.map / books.length.
	if res.Books == nil {
		t.Fatal("Books is nil; would serialize as JSON null")
	}
	if len(res.Books) != 0 {
		t.Errorf("len = %d, want 0", len(res.Books))
	}
}

func TestListSearchAndFacets(t *testing.T) {
	a := newTestAdapter(t)
	addSample(t, a, "The Caves of Steel", []string{"Isaac Asimov"})
	addSample(t, a, "Foundation", []string{"Isaac Asimov"})
	addSample(t, a, "Dune", []string{"Frank Herbert"})

	ctx := context.Background()
	res, err := a.ListBooks(ctx, ListOptions{Sort: "title"})
	if err != nil {
		t.Fatalf("ListBooks: %v", err)
	}
	if res.Total != 3 || len(res.Books) != 3 {
		t.Fatalf("total=%d len=%d", res.Total, len(res.Books))
	}
	// Sorted by title sort key: "Caves...", "Dune", "Foundation".
	if res.Books[0].Title != "The Caves of Steel" || res.Books[1].Title != "Dune" {
		t.Errorf("sort order wrong: %q, %q", res.Books[0].Title, res.Books[1].Title)
	}

	// Search by author.
	res, _ = a.ListBooks(ctx, ListOptions{Search: "Herbert"})
	if res.Total != 1 || res.Books[0].Title != "Dune" {
		t.Errorf("search Herbert => %+v", titles(res.Books))
	}

	// Facets.
	authors, _ := a.Authors(ctx)
	if len(authors) != 2 {
		t.Errorf("authors facet = %+v", authors)
	}
	for _, f := range authors {
		if f.Name == "Isaac Asimov" && f.Count != 2 {
			t.Errorf("Asimov count = %d, want 2", f.Count)
		}
	}

	// Filter by author id.
	var asimovID int64
	for _, f := range authors {
		if f.Name == "Isaac Asimov" {
			asimovID = f.ID
		}
	}
	res, _ = a.ListBooks(ctx, ListOptions{AuthorID: asimovID})
	if res.Total != 2 {
		t.Errorf("filter by author => total %d", res.Total)
	}

	stats, _ := a.Stats(ctx)
	if stats.Books != 3 || stats.Authors != 2 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestUpdateBookMovesFolder(t *testing.T) {
	a := newTestAdapter(t)
	b := addSample(t, a, "Old Title", []string{"Isaac Asimov"})
	oldFolder := a.BookFolder(b)
	mustExist(t, oldFolder)

	ctx := context.Background()
	newTitle := "Brand New Title"
	newAuthors := []string{"Arthur C. Clarke"}
	updated, err := a.UpdateBook(ctx, b.ID, UpdateBookInput{
		Title:   &newTitle,
		Authors: &newAuthors,
	})
	if err != nil {
		t.Fatalf("UpdateBook: %v", err)
	}
	if updated.Title != newTitle || updated.Sort != "Brand New Title" {
		t.Errorf("title/sort = %q/%q", updated.Title, updated.Sort)
	}
	if updated.AuthorSort != "Clarke, Arthur C." {
		t.Errorf("author_sort = %q", updated.AuthorSort)
	}

	// Old folder gone, new folder present with renamed file.
	if _, err := os.Stat(oldFolder); !os.IsNotExist(err) {
		t.Errorf("old folder still exists: %v", err)
	}
	newFolder := a.BookFolder(updated)
	if !strings.Contains(newFolder, "Arthur C. Clarke") {
		t.Errorf("new folder = %q", newFolder)
	}
	mustExist(t, filepath.Join(newFolder, "Brand New Title - Arthur C. Clarke.cbz"))

	// Partial update: only change rating, folder stays.
	r := 4
	again, err := a.UpdateBook(ctx, b.ID, UpdateBookInput{Rating: &r})
	if err != nil {
		t.Fatalf("partial update: %v", err)
	}
	if again.Rating != 4 || again.Title != newTitle {
		t.Errorf("partial update clobbered fields: rating=%d title=%q", again.Rating, again.Title)
	}
	if again.Path != updated.Path {
		t.Errorf("path changed on partial update: %q -> %q", updated.Path, again.Path)
	}
}

func TestDeleteBook(t *testing.T) {
	a := newTestAdapter(t)
	b := addSample(t, a, "Doomed", []string{"Nobody"})
	folder := a.BookFolder(b)
	mustExist(t, folder)

	ctx := context.Background()
	if err := a.DeleteBook(ctx, b.ID); err != nil {
		t.Fatalf("DeleteBook: %v", err)
	}
	if _, err := a.GetBook(ctx, b.ID); err == nil {
		t.Error("expected book gone")
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("folder not removed: %v", err)
	}
	// Cascade: no orphan rows.
	var n int
	a.db.QueryRow("SELECT COUNT(*) FROM books_authors_link WHERE book=?", b.ID).Scan(&n)
	if n != 0 {
		t.Errorf("cascade left %d author links", n)
	}
}

func TestReadOnlyRejectsWrites(t *testing.T) {
	dir := t.TempDir()
	// Bootstrap a library first, then reopen read-only.
	a, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	ro, err := Open(dir, true)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer ro.Close()
	_, err = ro.AddBook(context.Background(), AddBookInput{Title: "x", Format: "CBZ", Data: bytes.NewReader([]byte("x"))})
	if err != ErrReadOnly {
		t.Errorf("expected ErrReadOnly, got %v", err)
	}
}

// helpers

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected to exist: %s (%v)", path, err)
	}
}

func titles(bs []Book) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Title
	}
	return out
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
