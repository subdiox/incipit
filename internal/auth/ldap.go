package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/go-ldap/ldap/v3"

	"incipit/internal/appdb"
)

// safeUsername restricts usernames used in DNs / filters, preventing LDAP/DN
// injection. Directory usernames are normally well within this set.
var safeUsername = regexp.MustCompile(`^[a-zA-Z0-9._@-]+$`)

// LDAPSettings configures search-and-bind LDAP authentication and user import.
// It is persisted (as JSON) in app.db so an admin can edit it at runtime.
type LDAPSettings struct {
	Enabled  bool   `json:"enabled"`
	URL      string `json:"url"`      // ldap://host:389 or ldaps://host:636
	StartTLS bool   `json:"startTLS"` // upgrade a plain ld:// connection to TLS

	// Service ("reader") account used to search the directory. Leave BindDN
	// empty to search anonymously.
	BindDN       string `json:"bindDN"`
	BindPassword string `json:"bindPassword"` // sensitive: never sent back to the UI

	// User search.
	BaseDN            string `json:"baseDN"`            // e.g. ou=people,dc=example,dc=com
	UserFilter        string `json:"userFilter"`        // contains %s for the username, e.g. (uid=%s)
	UsernameAttribute string `json:"usernameAttribute"` // attribute used as the Incipit username, e.g. uid

	// Members of this group are imported / provisioned as admins.
	AdminGroupDN string `json:"adminGroupDN"`

	// When set, only members of this group may log in (and are imported).
	// Empty = no group restriction. Membership is resolved the same way as the
	// admin group (member / uniqueMember / memberUid).
	LoginGroupDN string `json:"loginGroupDN"`
}

// withDefaults fills in conventional defaults for empty fields.
func (s LDAPSettings) withDefaults() LDAPSettings {
	if strings.TrimSpace(s.UserFilter) == "" {
		s.UserFilter = "(uid=%s)"
	}
	if strings.TrimSpace(s.UsernameAttribute) == "" {
		s.UsernameAttribute = "uid"
	}
	return s
}

// ImportResult summarizes an LDAP user import run.
type ImportResult struct {
	Scanned          int      `json:"scanned"`          // directory entries matched
	Created          int      `json:"created"`          // new Incipit accounts created
	Existing         int      `json:"existing"`         // already present, left unchanged
	CreatedUsernames []string `json:"createdUsernames"` // sample of newly created usernames
}

// LDAPManager holds the current LDAP settings (guarded for concurrent reads
// during auth and writes from the admin API) and performs auth/test/import.
// It implements ExternalAuthenticator.
type LDAPManager struct {
	mu sync.RWMutex
	s  LDAPSettings
}

// NewLDAPManager builds a manager with the given initial settings.
func NewLDAPManager(s LDAPSettings) *LDAPManager {
	return &LDAPManager{s: s.withDefaults()}
}

// Settings returns the current settings (including the bind password — callers
// that expose these to clients must redact it).
func (m *LDAPManager) Settings() LDAPSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s
}

// SetSettings replaces the live settings.
func (m *LDAPManager) SetSettings(s LDAPSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s = s.withDefaults()
}

// Enabled reports whether LDAP auth is currently on.
func (m *LDAPManager) Enabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.s.Enabled
}

// Authenticate finds the user by search, binds as them to verify the password,
// and reports admin-group membership. Implements ExternalAuthenticator.
func (m *LDAPManager) Authenticate(_ context.Context, username, password string) (bool, bool, error) {
	s := m.Settings()
	if !s.Enabled {
		return false, false, nil
	}
	if !safeUsername.MatchString(username) || password == "" {
		return false, false, nil
	}

	conn, err := dial(s)
	if err != nil {
		return false, false, fmt.Errorf("ldap dial: %w", err)
	}
	defer conn.Close()
	if err := bindService(conn, s); err != nil {
		return false, false, fmt.Errorf("ldap service bind: %w", err)
	}

	entry, err := findUser(conn, s, username)
	if err != nil {
		return false, false, fmt.Errorf("ldap user search: %w", err)
	}
	if entry == nil {
		return false, false, nil // unknown user
	}

	// Verify the password on a second connection so the service binding is kept
	// for the subsequent group lookup.
	vconn, err := dial(s)
	if err != nil {
		return false, false, fmt.Errorf("ldap dial: %w", err)
	}
	defer vconn.Close()
	if err := vconn.Bind(entry.DN, password); err != nil {
		if ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("ldap user bind: %w", err)
	}

	// Restrict login to the allowed group, when one is configured.
	if dn := strings.TrimSpace(s.LoginGroupDN); dn != "" {
		member, err := isMemberOfGroup(conn, dn, entry.DN, username)
		if err != nil {
			return false, false, err
		}
		if !member {
			return false, false, nil // authenticated, but not permitted to log in
		}
	}

	isAdmin, err := isMemberOfGroup(conn, s.AdminGroupDN, entry.DN, username)
	if err != nil {
		return false, false, err
	}
	return true, isAdmin, nil
}

// TestConnection dials, optionally binds the service account, and confirms the
// base DN is reachable. Used by the admin "Test" button.
func (m *LDAPManager) TestConnection(_ context.Context) error {
	s := m.Settings()
	if !s.Enabled {
		return errors.New("ldap is disabled")
	}
	if strings.TrimSpace(s.URL) == "" {
		return errors.New("server URL is required")
	}
	conn, err := dial(s)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	if err := bindService(conn, s); err != nil {
		return fmt.Errorf("bind: %w", err)
	}
	if strings.TrimSpace(s.BaseDN) != "" {
		req := ldap.NewSearchRequest(s.BaseDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
			1, 0, false, "(objectClass=*)", []string{"dn"}, nil)
		if _, err := conn.Search(req); err != nil {
			return fmt.Errorf("search base DN: %w", err)
		}
	}
	return nil
}

// ImportUsers searches the directory and creates an Incipit account for every
// matching user that does not already exist. Existing accounts are left
// untouched so admin permission overrides persist. New users get admin from
// the configured admin group.
func (m *LDAPManager) ImportUsers(ctx context.Context, store *appdb.Store) (*ImportResult, error) {
	s := m.Settings()
	if !s.Enabled {
		return nil, errors.New("ldap is disabled")
	}
	if strings.TrimSpace(s.BaseDN) == "" {
		return nil, errors.New("base DN is required to import users")
	}

	conn, err := dial(s)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	if err := bindService(conn, s); err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}

	adminDNs, adminUIDs, err := groupMembers(conn, s.AdminGroupDN)
	if err != nil {
		return nil, err
	}

	// When a login group is configured, only its members are imported.
	var loginDNs, loginUIDs map[string]bool
	if strings.TrimSpace(s.LoginGroupDN) != "" {
		loginDNs, loginUIDs, err = groupMembers(conn, s.LoginGroupDN)
		if err != nil {
			return nil, err
		}
	}

	// Enumerate users: reuse the login filter with '*' in place of the username.
	filter := strings.ReplaceAll(s.UserFilter, "%s", "*")
	if strings.TrimSpace(filter) == "" {
		filter = "(objectClass=person)"
	}
	req := ldap.NewSearchRequest(s.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, filter, []string{"dn", s.UsernameAttribute}, nil)
	res, err := conn.SearchWithPaging(req, 500)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}

	result := &ImportResult{}
	for _, entry := range res.Entries {
		username := strings.TrimSpace(entry.GetAttributeValue(s.UsernameAttribute))
		if username == "" || !safeUsername.MatchString(username) {
			continue
		}
		// Skip users outside the configured login group.
		if loginDNs != nil && !loginDNs[strings.ToLower(entry.DN)] && !loginUIDs[username] {
			continue
		}
		result.Scanned++

		if _, err := store.GetUserByUsername(ctx, username); err == nil {
			result.Existing++
			continue
		} else if !errors.Is(err, appdb.ErrNotFound) {
			return nil, err
		}

		isAdmin := adminDNs[strings.ToLower(entry.DN)] || adminUIDs[username]
		if _, err := store.CreateUser(ctx, appdb.User{
			Username:    username,
			Source:      appdb.SourceLDAP,
			IsAdmin:     isAdmin,
			CanDownload: true,
			CanUpload:   isAdmin,
			CanEdit:     isAdmin,
		}); err != nil {
			return nil, fmt.Errorf("create user %q: %w", username, err)
		}
		result.Created++
		if len(result.CreatedUsernames) < 100 {
			result.CreatedUsernames = append(result.CreatedUsernames, username)
		}
	}
	return result, nil
}

// LoginEligibility reports, per username, whether that user currently satisfies
// the login-group restriction. It returns (nil, nil) when LDAP is disabled or no
// login group is set — i.e. everyone is eligible. Used to flag LDAP accounts in
// the admin user list that can no longer sign in (fell out of the login group).
func (m *LDAPManager) LoginEligibility(_ context.Context, usernames []string) (map[string]bool, error) {
	s := m.Settings()
	if !s.Enabled || strings.TrimSpace(s.LoginGroupDN) == "" {
		return nil, nil // no restriction in effect
	}
	if len(usernames) == 0 {
		return map[string]bool{}, nil
	}

	conn, err := dial(s)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()
	if err := bindService(conn, s); err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}

	loginDNs, loginUIDs, err := groupMembers(conn, s.LoginGroupDN)
	if err != nil {
		return nil, err
	}

	// Resolve each username's DN so DN- and uid-based groups both work.
	filter := strings.ReplaceAll(s.UserFilter, "%s", "*")
	if strings.TrimSpace(filter) == "" {
		filter = "(objectClass=person)"
	}
	req := ldap.NewSearchRequest(s.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false, filter, []string{"dn", s.UsernameAttribute}, nil)
	res, err := conn.SearchWithPaging(req, 500)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}

	want := make(map[string]bool, len(usernames))
	for _, u := range usernames {
		want[u] = true
	}
	out := make(map[string]bool, len(usernames))
	for _, entry := range res.Entries {
		username := strings.TrimSpace(entry.GetAttributeValue(s.UsernameAttribute))
		if username == "" || !want[username] {
			continue
		}
		out[username] = loginUIDs[username] || loginDNs[strings.ToLower(entry.DN)]
	}
	// Usernames not present in the directory cannot log in via LDAP.
	for _, u := range usernames {
		if _, ok := out[u]; !ok {
			out[u] = false
		}
	}
	return out, nil
}

// --- helpers ---

func dial(s LDAPSettings) (*ldap.Conn, error) {
	conn, err := ldap.DialURL(s.URL)
	if err != nil {
		return nil, err
	}
	if s.StartTLS {
		if err := conn.StartTLS(&tls.Config{ServerName: serverName(s.URL)}); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// bindService binds the configured service account, or stays anonymous when no
// BindDN is set.
func bindService(conn *ldap.Conn, s LDAPSettings) error {
	if strings.TrimSpace(s.BindDN) == "" {
		return nil
	}
	return conn.Bind(s.BindDN, s.BindPassword)
}

// findUser searches BaseDN for the single user matching the configured filter.
func findUser(conn *ldap.Conn, s LDAPSettings, username string) (*ldap.Entry, error) {
	if strings.TrimSpace(s.BaseDN) == "" {
		return nil, errors.New("base DN is not configured")
	}
	filter := strings.ReplaceAll(s.UserFilter, "%s", ldap.EscapeFilter(username))
	req := ldap.NewSearchRequest(s.BaseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		2, 0, false, filter, []string{"dn", s.UsernameAttribute}, nil)
	res, err := conn.Search(req)
	if err != nil {
		return nil, err
	}
	if len(res.Entries) != 1 {
		return nil, nil // zero or ambiguous => treat as not found
	}
	return res.Entries[0], nil
}

// isMemberOfGroup reports whether a user (by DN or username) belongs to the
// given group DN. Handles groupOfNames/uniqueMember and posixGroup. An empty
// groupDN reports false (no group => not a member).
func isMemberOfGroup(conn *ldap.Conn, groupDN, userDN, username string) (bool, error) {
	if strings.TrimSpace(groupDN) == "" {
		return false, nil
	}
	filter := fmt.Sprintf("(|(member=%s)(uniqueMember=%s)(memberUid=%s))",
		ldap.EscapeFilter(userDN), ldap.EscapeFilter(userDN), ldap.EscapeFilter(username))
	req := ldap.NewSearchRequest(groupDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, filter, []string{"dn"}, nil)
	res, err := conn.Search(req)
	if err != nil {
		var ldapErr *ldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldap.LDAPResultNoSuchObject {
			return false, nil
		}
		return false, fmt.Errorf("ldap group search: %w", err)
	}
	return len(res.Entries) > 0, nil
}

// groupMembers reads a group's membership once, returning lowercase member DNs
// and memberUid usernames for fast lookup during import. An empty groupDN
// returns empty maps.
func groupMembers(conn *ldap.Conn, groupDN string) (dns map[string]bool, uids map[string]bool, err error) {
	dns, uids = map[string]bool{}, map[string]bool{}
	if strings.TrimSpace(groupDN) == "" {
		return dns, uids, nil
	}
	req := ldap.NewSearchRequest(groupDN, ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, 0, false, "(objectClass=*)", []string{"member", "uniqueMember", "memberUid"}, nil)
	res, err := conn.Search(req)
	if err != nil {
		var ldapErr *ldap.Error
		if errors.As(err, &ldapErr) && ldapErr.ResultCode == ldap.LDAPResultNoSuchObject {
			return dns, uids, nil
		}
		return nil, nil, fmt.Errorf("ldap group: %w", err)
	}
	for _, e := range res.Entries {
		for _, dn := range append(e.GetAttributeValues("member"), e.GetAttributeValues("uniqueMember")...) {
			dns[strings.ToLower(dn)] = true
		}
		for _, uid := range e.GetAttributeValues("memberUid") {
			uids[uid] = true
		}
	}
	return dns, uids, nil
}

func serverName(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "ldaps://"), "ldap://")
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return s
}
