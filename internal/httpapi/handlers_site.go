package httpapi

import (
	"context"
	"net/http"
	"strings"
)

// SiteTitleKey is the app.db settings key holding the admin-configurable site
// title shown in the UI (sidebar, login, browser tab) and the OPDS feed.
const SiteTitleKey = "site_title"
const defaultSiteTitle = "Incipit"

// siteTitle returns the configured site title, or the default when unset.
func (s *Server) siteTitle(ctx context.Context) string {
	if v, _ := s.store.GetSetting(ctx, SiteTitleKey); strings.TrimSpace(v) != "" {
		return v
	}
	return defaultSiteTitle
}

// handleGetSite returns public site configuration (no auth) so the SPA can
// render the title on the login screen before sign-in.
func (s *Server) handleGetSite(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"title": s.siteTitle(r.Context())})
}

type siteUpdateBody struct {
	Title string `json:"title"`
}

// handleUpdateSite sets the site title (admin only).
func (s *Server) handleUpdateSite(w http.ResponseWriter, r *http.Request) {
	var body siteUpdateBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	if len([]rune(title)) > 80 {
		writeError(w, http.StatusBadRequest, "title is too long")
		return
	}
	if err := s.store.SetSetting(r.Context(), SiteTitleKey, title); err != nil {
		writeError(w, http.StatusInternalServerError, "save title")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"title": title})
}
