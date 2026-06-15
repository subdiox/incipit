// Package config loads Incipit's runtime configuration from the environment
// (12-factor). Every setting has a sane default so the binary boots with zero
// configuration for local development.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the fully-resolved runtime configuration.
type Config struct {
	// Addr is the TCP listen address, e.g. ":8080".
	Addr string

	// LibraryPath is the Calibre library directory containing metadata.db and
	// the per-book folders.
	LibraryPath string

	// ConfigDir holds Incipit's own state: app.db, the image cache and any
	// generated secrets. It is never part of the Calibre library so the
	// library stays portable.
	ConfigDir string

	// ReadOnly disables every write path to the Calibre library. Useful when
	// the library is shared with a desktop Calibre instance.
	ReadOnly bool

	// SessionSecret signs session cookies. Generated and persisted on first
	// run when not supplied.
	SessionSecret []byte

	// SecureCookies marks session cookies Secure (HTTPS only).
	SecureCookies bool

	// LDAP holds optional LDAP authentication settings.
	LDAP LDAPConfig

	// ProxyAuth holds optional reverse-proxy header authentication settings.
	ProxyAuth ProxyAuthConfig

	// CacheDir is derived from ConfigDir; where resized images live.
	CacheDir string
}

// LDAPConfig configures bind-based LDAP authentication.
type LDAPConfig struct {
	Enabled      bool
	URL          string // ldap://host:389 or ldaps://host:636
	BindDN       string // template with %s for the username, e.g. uid=%s,ou=users,dc=example,dc=com
	BaseDN       string
	UserFilter   string // e.g. (uid=%s)
	AdminGroupDN string // members of this group become admins
	StartTLS     bool
}

// ProxyAuthConfig trusts an upstream reverse proxy to authenticate users and
// pass the username in a header. Only enable behind a trusted proxy.
type ProxyAuthConfig struct {
	Enabled     bool
	UserHeader  string // e.g. X-Authenticated-User
	AdminHeader string // optional header whose presence/value grants admin
	AutoCreate  bool   // create a local user record on first sight
}

// Load resolves configuration from the environment, creating ConfigDir and the
// cache directory and persisting a generated session secret when needed.
func Load() (*Config, error) {
	c := &Config{
		Addr:          envOr("INCIPIT_ADDR", ":8080"),
		LibraryPath:   envOr("INCIPIT_LIBRARY", "/library"),
		ConfigDir:     envOr("INCIPIT_CONFIG", "/config"),
		ReadOnly:      envBool("INCIPIT_READONLY", false),
		SecureCookies: envBool("INCIPIT_SECURE_COOKIES", false),
		LDAP: LDAPConfig{
			Enabled:      envBool("INCIPIT_LDAP_ENABLED", false),
			URL:          os.Getenv("INCIPIT_LDAP_URL"),
			BindDN:       os.Getenv("INCIPIT_LDAP_BIND_DN"),
			BaseDN:       os.Getenv("INCIPIT_LDAP_BASE_DN"),
			UserFilter:   envOr("INCIPIT_LDAP_USER_FILTER", "(uid=%s)"),
			AdminGroupDN: os.Getenv("INCIPIT_LDAP_ADMIN_GROUP_DN"),
			StartTLS:     envBool("INCIPIT_LDAP_STARTTLS", false),
		},
		ProxyAuth: ProxyAuthConfig{
			Enabled:     envBool("INCIPIT_PROXY_AUTH_ENABLED", false),
			UserHeader:  envOr("INCIPIT_PROXY_AUTH_HEADER", "X-Authenticated-User"),
			AdminHeader: os.Getenv("INCIPIT_PROXY_AUTH_ADMIN_HEADER"),
			AutoCreate:  envBool("INCIPIT_PROXY_AUTH_AUTOCREATE", true),
		},
	}

	if err := os.MkdirAll(c.ConfigDir, 0o755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	c.CacheDir = filepath.Join(c.ConfigDir, "cache")
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}

	secret, err := resolveSessionSecret(c.ConfigDir)
	if err != nil {
		return nil, err
	}
	c.SessionSecret = secret

	return c, nil
}

// AppDBPath is the path to Incipit's own SQLite database.
func (c *Config) AppDBPath() string { return filepath.Join(c.ConfigDir, "app.db") }

// MetadataDBPath is the path to the Calibre metadata database.
func (c *Config) MetadataDBPath() string { return filepath.Join(c.LibraryPath, "metadata.db") }

// resolveSessionSecret reads INCIPIT_SESSION_SECRET, else loads/generates a
// persisted secret under ConfigDir so sessions survive restarts.
func resolveSessionSecret(configDir string) ([]byte, error) {
	if env := os.Getenv("INCIPIT_SESSION_SECRET"); env != "" {
		return []byte(env), nil
	}
	path := filepath.Join(configDir, "session.key")
	if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
		decoded, derr := hex.DecodeString(strings.TrimSpace(string(b)))
		if derr == nil && len(decoded) >= 32 {
			return decoded, nil
		}
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate session secret: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(secret)), 0o600); err != nil {
		return nil, fmt.Errorf("persist session secret: %w", err)
	}
	return secret, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
