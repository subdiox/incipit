package auth

import (
	"context"
	"errors"
	"strings"

	"incipit/internal/appdb"
)

// ErrInvalidCredentials is returned when authentication fails.
var ErrInvalidCredentials = errors.New("auth: invalid credentials")

// ExternalAuthenticator authenticates a username/password against an external
// directory (e.g. LDAP) and reports whether the user should be an admin.
type ExternalAuthenticator interface {
	Authenticate(ctx context.Context, username, password string) (ok bool, isAdmin bool, err error)
}

// Service performs login against local accounts and an optional external
// directory, provisioning local records for external users on first sight.
type Service struct {
	store    *appdb.Store
	external ExternalAuthenticator // nil when no external auth is configured
}

// NewService builds a login service. external may be nil.
func NewService(store *appdb.Store, external ExternalAuthenticator) *Service {
	return &Service{store: store, external: external}
}

// Login authenticates a user and returns the resolved account.
//
// A local account with a password is verified locally. Otherwise, when an
// external authenticator is configured, credentials are checked against it and
// a local record (source=ldap) is created or updated to reflect admin status.
func (s *Service) Login(ctx context.Context, username, password string) (*appdb.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	existing, err := s.store.GetUserByUsername(ctx, username)
	switch {
	case err == nil && existing.Source == appdb.SourceLocal && existing.PasswordHash != "":
		ok, verr := VerifyPassword(existing.PasswordHash, password)
		if verr != nil {
			return nil, verr
		}
		if !ok {
			return nil, ErrInvalidCredentials
		}
		return existing, nil
	case err != nil && !errors.Is(err, appdb.ErrNotFound):
		return nil, err
	}

	// Fall through to external authentication.
	if s.external == nil {
		return nil, ErrInvalidCredentials
	}
	ok, isAdmin, err := s.external.Authenticate(ctx, username, password)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidCredentials
	}
	return s.provisionExternal(ctx, username, appdb.SourceLDAP, isAdmin)
}

// provisionExternal creates the local record mirroring an external user on
// first sight, returning the resolved account. Existing accounts are returned
// as-is: the directory sets initial admin status, but an Incipit admin's later
// permission overrides are preserved across logins and imports.
func (s *Service) provisionExternal(ctx context.Context, username string, source appdb.UserSource, isAdmin bool) (*appdb.User, error) {
	existing, err := s.store.GetUserByUsername(ctx, username)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, appdb.ErrNotFound) {
		return nil, err
	}
	return s.store.CreateUser(ctx, appdb.User{
		Username:    username,
		Source:      source,
		IsAdmin:     isAdmin,
		CanDownload: true,
		CanUpload:   isAdmin,
		CanEdit:     isAdmin,
	})
}

// ProxyConfig configures reverse-proxy header authentication.
type ProxyConfig struct {
	Enabled     bool
	UserHeader  string
	AdminHeader string
	AutoCreate  bool
}

// ResolveProxyUser resolves the user identified by a trusted upstream proxy from
// request headers. Returns (nil, nil) when no user header is present. Only call
// this when the request is known to come from a trusted proxy.
func (s *Service) ResolveProxyUser(ctx context.Context, cfg ProxyConfig, header func(string) string) (*appdb.User, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	username := strings.TrimSpace(header(cfg.UserHeader))
	if username == "" {
		return nil, nil
	}
	isAdmin := false
	if cfg.AdminHeader != "" && strings.TrimSpace(header(cfg.AdminHeader)) != "" {
		isAdmin = true
	}

	existing, err := s.store.GetUserByUsername(ctx, username)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, appdb.ErrNotFound) {
		return nil, err
	}
	if !cfg.AutoCreate {
		return nil, ErrInvalidCredentials
	}
	return s.provisionExternal(ctx, username, appdb.SourceProxy, isAdmin)
}
