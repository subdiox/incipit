package appdb

import (
	"context"
	"testing"
)

// A new account must not be pinned to a language: the UI reads an empty value
// as "never chosen" and follows the browser instead.
func TestCreateUserLeavesLanguageUnset(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	u, err := s.CreateUser(ctx, User{Username: "alice"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Language != "" {
		t.Fatalf("returned Language = %q, want empty", u.Language)
	}

	got, err := s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.Language != "" {
		t.Fatalf("stored Language = %q, want empty", got.Language)
	}

	if err := s.SetUserLanguage(ctx, u.ID, "ja"); err != nil {
		t.Fatalf("SetUserLanguage: %v", err)
	}
	got, err = s.GetUserByUsername(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.Language != "ja" {
		t.Fatalf("stored Language = %q, want %q", got.Language, "ja")
	}
}

// An explicitly passed language still wins over the unset default.
func TestCreateUserKeepsExplicitLanguage(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser(context.Background(), User{Username: "bob", Language: "ja"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Language != "ja" {
		t.Fatalf("Language = %q, want %q", u.Language, "ja")
	}
}
