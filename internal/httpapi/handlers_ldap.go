package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"incipit/internal/auth"
)

// LDAPSettingKey is the app.db settings key under which LDAP config is stored.
const LDAPSettingKey = "ldap"

// ldapSettingsResponse mirrors auth.LDAPSettings but never exposes the bind
// password; a boolean reports whether one is stored.
type ldapSettingsResponse struct {
	Enabled           bool   `json:"enabled"`
	URL               string `json:"url"`
	StartTLS          bool   `json:"startTLS"`
	BindDN            string `json:"bindDN"`
	BindPasswordSet   bool   `json:"bindPasswordSet"`
	BaseDN            string `json:"baseDN"`
	UserFilter        string `json:"userFilter"`
	UsernameAttribute string `json:"usernameAttribute"`
	AdminGroupDN      string `json:"adminGroupDN"`
	LoginGroupDN      string `json:"loginGroupDN"`
}

func toLDAPResponse(s auth.LDAPSettings) ldapSettingsResponse {
	return ldapSettingsResponse{
		Enabled:           s.Enabled,
		URL:               s.URL,
		StartTLS:          s.StartTLS,
		BindDN:            s.BindDN,
		BindPasswordSet:   s.BindPassword != "",
		BaseDN:            s.BaseDN,
		UserFilter:        s.UserFilter,
		UsernameAttribute: s.UsernameAttribute,
		AdminGroupDN:      s.AdminGroupDN,
		LoginGroupDN:      s.LoginGroupDN,
	}
}

// handleGetLDAP returns the current LDAP settings (password redacted).
func (s *Server) handleGetLDAP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toLDAPResponse(s.ldap.Settings()))
}

// ldapUpdateBody is the editable settings payload. BindPassword is a pointer so
// an omitted/empty value keeps the stored password (only a non-empty value
// replaces it).
type ldapUpdateBody struct {
	Enabled           bool    `json:"enabled"`
	URL               string  `json:"url"`
	StartTLS          bool    `json:"startTLS"`
	BindDN            string  `json:"bindDN"`
	BindPassword      *string `json:"bindPassword"`
	BaseDN            string  `json:"baseDN"`
	UserFilter        string  `json:"userFilter"`
	UsernameAttribute string  `json:"usernameAttribute"`
	AdminGroupDN      string  `json:"adminGroupDN"`
	LoginGroupDN      string  `json:"loginGroupDN"`
}

// handleUpdateLDAP validates, persists and hot-swaps the LDAP settings.
func (s *Server) handleUpdateLDAP(w http.ResponseWriter, r *http.Request) {
	var body ldapUpdateBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	current := s.ldap.Settings()
	next := auth.LDAPSettings{
		Enabled:           body.Enabled,
		URL:               strings.TrimSpace(body.URL),
		StartTLS:          body.StartTLS,
		BindDN:            strings.TrimSpace(body.BindDN),
		BindPassword:      current.BindPassword,
		BaseDN:            strings.TrimSpace(body.BaseDN),
		UserFilter:        strings.TrimSpace(body.UserFilter),
		UsernameAttribute: strings.TrimSpace(body.UsernameAttribute),
		AdminGroupDN:      strings.TrimSpace(body.AdminGroupDN),
		LoginGroupDN:      strings.TrimSpace(body.LoginGroupDN),
	}
	// Only replace the password when a non-empty value is supplied.
	if body.BindPassword != nil && *body.BindPassword != "" {
		next.BindPassword = *body.BindPassword
	}
	// Clearing the bind DN drops the (now unusable) stored password.
	if next.BindDN == "" {
		next.BindPassword = ""
	}

	if body.Enabled {
		if next.URL == "" {
			writeError(w, http.StatusBadRequest, "server URL is required when LDAP is enabled")
			return
		}
		if next.UserFilter != "" && !strings.Contains(next.UserFilter, "%s") {
			writeError(w, http.StatusBadRequest, "user filter must contain %s for the username")
			return
		}
	}

	if err := s.persistLDAP(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, "save settings")
		return
	}
	writeJSON(w, http.StatusOK, toLDAPResponse(s.ldap.Settings()))
}

// persistLDAP writes settings to app.db then swaps them into the live manager.
func (s *Server) persistLDAP(ctx context.Context, next auth.LDAPSettings) error {
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := s.store.SetSetting(ctx, LDAPSettingKey, string(raw)); err != nil {
		return err
	}
	s.ldap.SetSettings(next)
	return nil
}

// handleTestLDAP dials the directory and binds the service account.
func (s *Server) handleTestLDAP(w http.ResponseWriter, r *http.Request) {
	if err := s.ldap.TestConnection(r.Context()); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleImportLDAP imports directory users as Incipit accounts.
func (s *Server) handleImportLDAP(w http.ResponseWriter, r *http.Request) {
	result, err := s.ldap.ImportUsers(r.Context(), s.store)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
