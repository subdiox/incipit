package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INCIPIT_CONFIG", dir)
	t.Setenv("INCIPIT_LIBRARY", filepath.Join(dir, "lib"))
	t.Setenv("INCIPIT_READONLY", "true")
	t.Setenv("INCIPIT_LDAP_ENABLED", "true")
	t.Setenv("INCIPIT_LDAP_URL", "ldap://example.com")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("default addr = %q", cfg.Addr)
	}
	if !cfg.ReadOnly {
		t.Error("expected ReadOnly")
	}
	if !cfg.LDAP.Enabled || cfg.LDAP.URL != "ldap://example.com" {
		t.Errorf("ldap = %+v", cfg.LDAP)
	}
	if cfg.LDAP.UserFilter != "(uid=%s)" {
		t.Errorf("default user filter = %q", cfg.LDAP.UserFilter)
	}
	if cfg.AppDBPath() != filepath.Join(dir, "app.db") {
		t.Errorf("app db path = %q", cfg.AppDBPath())
	}
	if cfg.MetadataDBPath() != filepath.Join(dir, "lib", "metadata.db") {
		t.Errorf("metadata path = %q", cfg.MetadataDBPath())
	}
	// Cache dir was created.
	if _, err := os.Stat(cfg.CacheDir); err != nil {
		t.Errorf("cache dir not created: %v", err)
	}
}

func TestSessionSecretPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INCIPIT_CONFIG", dir)
	t.Setenv("INCIPIT_SESSION_SECRET", "") // force generated-and-persisted path

	c1, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c1.SessionSecret) < 32 {
		t.Fatalf("secret too short: %d", len(c1.SessionSecret))
	}
	if string(c1.SessionSecret) != string(c2.SessionSecret) {
		t.Error("session secret not stable across loads")
	}
}

func TestSessionSecretFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INCIPIT_CONFIG", dir)
	t.Setenv("INCIPIT_SESSION_SECRET", "explicit-secret-value")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(cfg.SessionSecret) != "explicit-secret-value" {
		t.Errorf("secret = %q", cfg.SessionSecret)
	}
}
