package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-ldap/ldap/v3"

	"incipit/internal/config"
)

// safeUsername restricts usernames used to build a bind DN, preventing LDAP/DN
// injection. Directory usernames are normally well within this set.
var safeUsername = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)

// LDAPAuthenticator authenticates users by binding to an LDAP/AD server and
// optionally checking admin-group membership. Implements ExternalAuthenticator.
type LDAPAuthenticator struct {
	cfg config.LDAPConfig
}

// NewLDAPAuthenticator returns an authenticator for the given config, or nil if
// LDAP is disabled.
func NewLDAPAuthenticator(cfg config.LDAPConfig) *LDAPAuthenticator {
	if !cfg.Enabled {
		return nil
	}
	return &LDAPAuthenticator{cfg: cfg}
}

// Authenticate binds as the user and reports success and admin status.
func (l *LDAPAuthenticator) Authenticate(_ context.Context, username, password string) (bool, bool, error) {
	if !safeUsername.MatchString(username) {
		return false, false, nil
	}
	conn, err := l.dial()
	if err != nil {
		return false, false, fmt.Errorf("ldap dial: %w", err)
	}
	defer conn.Close()

	bindDN := fmt.Sprintf(l.cfg.BindDN, username)
	if err := conn.Bind(bindDN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("ldap bind: %w", err)
	}

	isAdmin, err := l.isAdmin(conn, bindDN, username)
	if err != nil {
		return false, false, err
	}
	return true, isAdmin, nil
}

func (l *LDAPAuthenticator) dial() (*ldap.Conn, error) {
	conn, err := ldap.DialURL(l.cfg.URL)
	if err != nil {
		return nil, err
	}
	if l.cfg.StartTLS {
		if err := conn.StartTLS(&tls.Config{ServerName: serverName(l.cfg.URL)}); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// isAdmin checks whether the bound user is a member of the configured admin
// group. Both groupOfNames (member=DN) and posixGroup (memberUid=uid) styles
// are checked.
func (l *LDAPAuthenticator) isAdmin(conn *ldap.Conn, bindDN, username string) (bool, error) {
	if l.cfg.AdminGroupDN == "" {
		return false, nil
	}
	filter := fmt.Sprintf("(|(member=%s)(uniqueMember=%s)(memberUid=%s))",
		ldap.EscapeFilter(bindDN), ldap.EscapeFilter(bindDN), ldap.EscapeFilter(username))
	req := ldap.NewSearchRequest(
		l.cfg.AdminGroupDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases, 1, 0, false,
		filter, []string{"dn"}, nil,
	)
	res, err := conn.Search(req)
	if err != nil {
		// A "no such object" on the group DN simply means not a member.
		var ldapErr *ldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldap.LDAPResultNoSuchObject {
			return false, nil
		}
		return false, fmt.Errorf("ldap admin search: %w", err)
	}
	return len(res.Entries) > 0, nil
}

func serverName(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "ldaps://"), "ldap://")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return s
}
