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

func TestListMultiTagAND(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()
	add := func(title string, tags []string) {
		if _, err := a.AddBook(ctx, AddBookInput{
			Title: title, Authors: []string{"X"}, Tags: tags, Format: "CBZ",
			Data: bytes.NewReader([]byte("PK\x03\x04z")),
		}); err != nil {
			t.Fatalf("AddBook(%q): %v", title, err)
		}
	}
	add("AB", []string{"a", "b"})
	add("BC", []string{"b", "c"})
	add("ABC", []string{"a", "b", "c"})

	tagID := map[string]int64{}
	tags, _ := a.Tags(ctx)
	for _, f := range tags {
		tagID[f.Name] = f.ID
	}

	// Single tag b matches all three.
	if res, _ := a.ListBooks(ctx, ListOptions{TagIDs: []int64{tagID["b"]}}); res.Total != 3 {
		t.Errorf("tag b => %d, want 3", res.Total)
	}
	// a AND b matches AB and ABC (not BC).
	res, _ := a.ListBooks(ctx, ListOptions{TagIDs: []int64{tagID["a"], tagID["b"]}, Sort: "title"})
	if res.Total != 2 || res.Books[0].Title != "AB" || res.Books[1].Title != "ABC" {
		t.Errorf("a AND b => %+v", titles(res.Books))
	}
	// a AND b AND c matches only ABC.
	if res, _ := a.ListBooks(ctx, ListOptions{TagIDs: []int64{tagID["a"], tagID["b"], tagID["c"]}}); res.Total != 1 || res.Books[0].Title != "ABC" {
		t.Errorf("a AND b AND c => %+v", titles(res.Books))
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

// TestRelocateRollbackRestoresFilesAndFolder verifies that when a relocate is
// rolled back (e.g. the surrounding tx fails to commit), the filesystem is
// restored completely — folder back in place AND files under their old names —
// so disk stays consistent with the rolled-back DB.
func TestRelocateRollbackRestoresFilesAndFolder(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()
	b := addSample(t, a, "Old Title", []string{"Jane Doe"})

	oldFolder := filepath.Join(a.libraryPath, filepath.FromSlash(b.Path))
	if len(b.Formats) == 0 {
		t.Fatal("sample book has no formats")
	}
	f := b.Formats[0]
	oldFile := filepath.Join(oldFolder, f.Name+"."+strings.ToLower(f.Format))
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("precondition: old file missing: %v", err)
	}

	newRel, newBase := bookRelPath(b.ID, "Brand New Title", []string{"Jane Doe"})
	if newRel == b.Path {
		t.Fatal("expected the new title to change the path")
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := a.relocateBook(ctx, tx, b, newRel, newBase)
	if err != nil {
		t.Fatalf("relocateBook: %v", err)
	}
	// Simulate a commit failure: undo the filesystem move, then the DB tx.
	rb()
	_ = tx.Rollback()

	if _, err := os.Stat(oldFile); err != nil {
		t.Errorf("old file not restored to its original name/location: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldFolder, newBase+".cbz")); err == nil {
		t.Error("a renamed file was left behind in the old folder")
	}
	if _, err := os.Stat(filepath.Join(a.libraryPath, filepath.FromSlash(newRel))); !os.IsNotExist(err) {
		t.Errorf("new folder should not exist after rollback (stat err=%v)", err)
	}
	got, err := a.GetBook(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != b.Path {
		t.Errorf("DB path = %q, want %q (tx should have rolled back)", got.Path, b.Path)
	}
}

// TestUpdateBookAddTagsPreservesExisting verifies AddTags unions with existing
// tags (no delete), so a cmoa re-enrich keeps user-added tags.
func TestUpdateBookAddTagsPreservesExisting(t *testing.T) {
	a := newTestAdapter(t)
	ctx := context.Background()
	b := addSample(t, a, "Tagged", []string{"Jane Doe"}) // tags: scifi, classic

	add := []string{"漫画BANK", "scifi"} // new tag + a duplicate of an existing one
	updated, err := a.UpdateBook(ctx, b.ID, UpdateBookInput{AddTags: &add})
	if err != nil {
		t.Fatalf("UpdateBook: %v", err)
	}

	got := map[string]bool{}
	for _, tg := range updated.Tags {
		got[tg.Name] = true
	}
	for _, want := range []string{"scifi", "classic", "漫画BANK"} {
		if !got[want] {
			t.Errorf("tag %q missing after AddTags; got %v", want, updated.Tags)
		}
	}
	if len(updated.Tags) != 3 {
		t.Errorf("expected 3 unique tags (union, no dupes), got %d: %v", len(updated.Tags), updated.Tags)
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
