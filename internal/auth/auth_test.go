package auth

import (
	"context"
	"path/filepath"
	"testing"

	"incipit/internal/appdb"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || hash[:9] != "$argon2id" {
		t.Fatalf("unexpected hash: %q", hash)
	}
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Errorf("verify correct: ok=%v err=%v", ok, err)
	}
	ok, _ = VerifyPassword(hash, "wrong")
	if ok {
		t.Error("verify wrong password succeeded")
	}
	// Two hashes of the same password differ (random salt).
	hash2, _ := HashPassword("correct horse battery staple")
	if hash == hash2 {
		t.Error("expected distinct salts")
	}
	if _, err := VerifyPassword("not-a-hash", "x"); err == nil {
		t.Error("expected error on malformed hash")
	}
}

func newAuthStore(t *testing.T) *appdb.Store {
	t.Helper()
	s, err := appdb.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestLoginLocal(t *testing.T) {
	store := newAuthStore(t)
	ctx := context.Background()
	hash, _ := HashPassword("s3cret")
	store.CreateUser(ctx, appdb.User{Username: "alice", PasswordHash: hash, Source: appdb.SourceLocal})

	svc := NewService(store, nil)
	u, err := svc.Login(ctx, "alice", "s3cret")
	if err != nil || u.Username != "alice" {
		t.Fatalf("login ok case: %+v err=%v", u, err)
	}
	if _, err := svc.Login(ctx, "alice", "nope"); err != ErrInvalidCredentials {
		t.Errorf("bad password => %v", err)
	}
	if _, err := svc.Login(ctx, "ghost", "x"); err != ErrInvalidCredentials {
		t.Errorf("unknown user => %v", err)
	}
}

// fakeExternal is a stand-in for LDAP in tests.
type fakeExternal struct {
	okUser    string
	okPass    string
	adminUser string
}

func (f fakeExternal) Authenticate(_ context.Context, u, p string) (bool, bool, error) {
	if u == f.okUser && p == f.okPass {
		return true, u == f.adminUser, nil
	}
	return false, false, nil
}

func TestLoginExternalProvisions(t *testing.T) {
	store := newAuthStore(t)
	ctx := context.Background()
	svc := NewService(store, fakeExternal{okUser: "bob", okPass: "ldap-pw", adminUser: "bob"})

	u, err := svc.Login(ctx, "bob", "ldap-pw")
	if err != nil {
		t.Fatalf("external login: %v", err)
	}
	if u.Source != appdb.SourceLDAP || !u.IsAdmin {
		t.Errorf("provisioned user = %+v", u)
	}
	// Provisioned exactly once.
	if _, err := svc.Login(ctx, "bob", "ldap-pw"); err != nil {
		t.Fatalf("second login: %v", err)
	}
	users, _ := store.ListUsers(ctx)
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
	if _, err := svc.Login(ctx, "bob", "wrong"); err != ErrInvalidCredentials {
		t.Errorf("external bad pw => %v", err)
	}
}

func TestProxyResolve(t *testing.T) {
	store := newAuthStore(t)
	ctx := context.Background()
	svc := NewService(store, nil)
	cfg := ProxyConfig{Enabled: true, UserHeader: "X-User", AdminHeader: "X-Admin", AutoCreate: true}

	headers := map[string]string{"X-User": "carol", "X-Admin": "1"}
	get := func(k string) string { return headers[k] }

	u, err := svc.ResolveProxyUser(ctx, cfg, get)
	if err != nil || u == nil || u.Source != appdb.SourceProxy || !u.IsAdmin {
		t.Fatalf("proxy resolve = %+v err=%v", u, err)
	}

	// No header => no user, no error.
	delete(headers, "X-User")
	u, err = svc.ResolveProxyUser(ctx, cfg, get)
	if err != nil || u != nil {
		t.Errorf("missing header => %+v err=%v", u, err)
	}

	// Disabled => nil.
	u, _ = svc.ResolveProxyUser(ctx, ProxyConfig{Enabled: false}, get)
	if u != nil {
		t.Error("disabled proxy returned a user")
	}
}
