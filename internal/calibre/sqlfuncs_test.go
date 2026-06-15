package calibre

import (
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"
)

func TestTitleSort(t *testing.T) {
	cases := map[string]string{
		"The Hobbit":          "Hobbit, The",
		"A Game of Thrones":   "Game of Thrones, A",
		"An Inconvenient Sea": "Inconvenient Sea, An",
		"Dune":                "Dune",
		"  The Stand ":        "Stand, The",
		"The":                 "The",      // article with no remainder is left alone
		"Theodore":            "Theodore", // not an article boundary
	}
	for in, want := range cases {
		if got := TitleSort(in); got != want {
			t.Errorf("TitleSort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAuthorSort(t *testing.T) {
	cases := map[string]string{
		"George Orwell":         "Orwell, George",
		"Brandon Sanderson":     "Sanderson, Brandon",
		"Plato":                 "Plato",
		"Orwell, George":        "Orwell, George", // already sorted
		"Martin Luther King Jr": "King Jr, Martin Luther",
		"J. R. R. Tolkien":      "Tolkien, J. R. R.",
	}
	for in, want := range cases {
		if got := AuthorSort(in); got != want {
			t.Errorf("AuthorSort(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUUID4Format(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		u := UUID4()
		if !re.MatchString(u) {
			t.Fatalf("UUID4() = %q, not a valid v4 uuid", u)
		}
		if seen[u] {
			t.Fatalf("UUID4() collision on %q", u)
		}
		seen[u] = true
	}
}

// TestInsertTriggersFireCustomFunctions is the critical clean-room guarantee:
// inserting a book row must succeed (the trigger calls title_sort + uuid4) and
// populate sort/uuid automatically, exactly as Calibre does.
func TestInsertTriggersFireCustomFunctions(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureLibrary(dir); err != nil {
		t.Fatalf("EnsureLibrary: %v", err)
	}
	db, err := openDB(filepath.Join(dir, "metadata.db"), false)
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()

	res, err := db.Exec("INSERT INTO books (title, author_sort, path) VALUES (?, ?, ?)",
		"The Lord of the Rings", "Tolkien, J.R.R.", "")
	if err != nil {
		t.Fatalf("insert book: %v", err)
	}
	id, _ := res.LastInsertId()

	var sortVal, uuidVal sql.NullString
	if err := db.QueryRow("SELECT sort, uuid FROM books WHERE id=?", id).Scan(&sortVal, &uuidVal); err != nil {
		t.Fatalf("query book: %v", err)
	}
	if sortVal.String != "Lord of the Rings, The" {
		t.Errorf("trigger sort = %q, want %q", sortVal.String, "Lord of the Rings, The")
	}
	if !regexp.MustCompile(`^[0-9a-f-]{36}$`).MatchString(uuidVal.String) {
		t.Errorf("trigger uuid = %q, want a uuid", uuidVal.String)
	}

	// Updating the title must re-derive sort via books_update_trg.
	if _, err := db.Exec("UPDATE books SET title=? WHERE id=?", "A New Title", id); err != nil {
		t.Fatalf("update title: %v", err)
	}
	if err := db.QueryRow("SELECT sort FROM books WHERE id=?", id).Scan(&sortVal); err != nil {
		t.Fatalf("re-query: %v", err)
	}
	if sortVal.String != "New Title, A" {
		t.Errorf("after update sort = %q, want %q", sortVal.String, "New Title, A")
	}
}
