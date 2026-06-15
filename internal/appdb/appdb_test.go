package appdb

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUserLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if n, _ := s.CountUsers(ctx); n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}
	u, err := s.CreateUser(ctx, User{Username: "Alice", PasswordHash: "h", IsAdmin: true, CanEdit: true})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 || u.Source != SourceLocal {
		t.Fatalf("bad user: %+v", u)
	}

	// Case-insensitive username lookup.
	got, err := s.GetUserByUsername(ctx, "alice")
	if err != nil || got.ID != u.ID || !got.IsAdmin || !got.CanEdit {
		t.Fatalf("GetUserByUsername: %+v err=%v", got, err)
	}

	// Duplicate username rejected.
	if _, err := s.CreateUser(ctx, User{Username: "alice"}); err == nil {
		t.Error("expected duplicate username error")
	}

	got.CanUpload = true
	got.IsAdmin = false
	if err := s.UpdateUser(ctx, got); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	reload, _ := s.GetUser(ctx, u.ID)
	if !reload.CanUpload || reload.IsAdmin {
		t.Errorf("update not persisted: %+v", reload)
	}

	if err := s.DeleteUser(ctx, u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := s.GetUser(ctx, u.ID); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, User{Username: "bob"})

	valid := Session{ID: "tok1", UserID: u.ID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateSession(ctx, valid); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := s.GetSession(ctx, "tok1")
	if err != nil || got.UserID != u.ID {
		t.Fatalf("GetSession: %+v err=%v", got, err)
	}

	// Expired session is rejected and purged.
	expired := Session{ID: "tok2", UserID: u.ID, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	s.CreateSession(ctx, expired)
	if _, err := s.GetSession(ctx, "tok2"); err != ErrNotFound {
		t.Errorf("expected expired => ErrNotFound, got %v", err)
	}

	s.DeleteSession(ctx, "tok1")
	if _, err := s.GetSession(ctx, "tok1"); err != ErrNotFound {
		t.Errorf("expected deleted => ErrNotFound, got %v", err)
	}
}

func TestShelvesAndProgress(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, User{Username: "carol"})

	sh, err := s.CreateShelf(ctx, Shelf{UserID: u.ID, Name: "Favorites"})
	if err != nil {
		t.Fatalf("CreateShelf: %v", err)
	}
	for _, id := range []int64{10, 20, 30} {
		if err := s.AddBookToShelf(ctx, sh.ID, id); err != nil {
			t.Fatalf("AddBookToShelf: %v", err)
		}
	}
	s.AddBookToShelf(ctx, sh.ID, 20) // idempotent
	ids, _ := s.ShelfBookIDs(ctx, sh.ID)
	if len(ids) != 3 {
		t.Fatalf("shelf ids = %v", ids)
	}
	shelves, _ := s.ListShelves(ctx, u.ID)
	if len(shelves) != 1 || shelves[0].BookCount != 3 {
		t.Fatalf("ListShelves = %+v", shelves)
	}
	s.RemoveBookFromShelf(ctx, sh.ID, 20)
	ids, _ = s.ShelfBookIDs(ctx, sh.ID)
	if len(ids) != 2 {
		t.Errorf("after remove ids = %v", ids)
	}

	// Progress upsert.
	if err := s.SetProgress(ctx, Progress{UserID: u.ID, BookID: 10, Format: "CBZ", Page: 5, TotalPages: 100}); err != nil {
		t.Fatalf("SetProgress: %v", err)
	}
	s.SetProgress(ctx, Progress{UserID: u.ID, BookID: 10, Format: "CBZ", Page: 42, TotalPages: 100})
	p, err := s.GetProgress(ctx, u.ID, 10, "CBZ")
	if err != nil || p.Page != 42 {
		t.Fatalf("GetProgress = %+v err=%v", p, err)
	}
}

func TestPageCacheValidityAndSettings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	entry := PageCacheEntry{
		BookID: 1, Format: "CBZ", FilePath: "/x.cbz",
		Pages: []string{"001.jpg", "002.jpg"}, PageCount: 2, MTime: 1000, Size: 500,
	}
	if err := s.PutPageCache(ctx, entry); err != nil {
		t.Fatalf("PutPageCache: %v", err)
	}
	got, err := s.GetPageCache(ctx, 1, "CBZ", 1000, 500)
	if err != nil || got.PageCount != 2 || len(got.Pages) != 2 {
		t.Fatalf("GetPageCache = %+v err=%v", got, err)
	}
	// Stale on mtime change.
	if _, err := s.GetPageCache(ctx, 1, "CBZ", 2000, 500); err != ErrNotFound {
		t.Errorf("expected stale => ErrNotFound, got %v", err)
	}

	if v, _ := s.GetSetting(ctx, "missing"); v != "" {
		t.Errorf("missing setting = %q", v)
	}
	s.SetSetting(ctx, "theme", "dark")
	s.SetSetting(ctx, "theme", "light")
	if v, _ := s.GetSetting(ctx, "theme"); v != "light" {
		t.Errorf("setting = %q", v)
	}
}
