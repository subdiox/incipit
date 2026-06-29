package auth

import (
	"context"
	"testing"
)

func TestLDAPManagerDefaults(t *testing.T) {
	m := NewLDAPManager(LDAPSettings{}) // all empty
	s := m.Settings()
	if s.UserFilter != "(uid=%s)" {
		t.Errorf("default user filter = %q", s.UserFilter)
	}
	if s.UsernameAttribute != "uid" {
		t.Errorf("default username attribute = %q", s.UsernameAttribute)
	}
	if m.Enabled() {
		t.Error("empty settings should be disabled")
	}
}

func TestLDAPManagerDisabledAuth(t *testing.T) {
	m := NewLDAPManager(LDAPSettings{Enabled: false, URL: "ldap://example.invalid"})
	// Disabled: never dials, never errors, never authenticates.
	ok, admin, err := m.Authenticate(context.Background(), "alice", "pw")
	if ok || admin || err != nil {
		t.Fatalf("disabled auth = (%v,%v,%v), want (false,false,nil)", ok, admin, err)
	}
	if err := m.TestConnection(context.Background()); err == nil {
		t.Error("TestConnection on disabled manager should error")
	}
}

func TestLDAPManagerRejectsUnsafeUsername(t *testing.T) {
	// Enabled but a malicious username must be rejected before any dial.
	m := NewLDAPManager(LDAPSettings{Enabled: true, URL: "ldap://example.invalid", BaseDN: "dc=x"})
	ok, _, err := m.Authenticate(context.Background(), "a)(uid=*", "pw")
	if ok || err != nil {
		t.Fatalf("unsafe username = (%v,%v), want (false,nil)", ok, err)
	}
}

func TestLDAPSettingsHotSwap(t *testing.T) {
	m := NewLDAPManager(LDAPSettings{})
	m.SetSettings(LDAPSettings{Enabled: true, URL: "ldap://h", BindDN: "cn=r", BindPassword: "pw"})
	s := m.Settings()
	if !s.Enabled || s.BindPassword != "pw" || s.UsernameAttribute != "uid" {
		t.Errorf("hot-swapped settings = %+v", s)
	}
}
