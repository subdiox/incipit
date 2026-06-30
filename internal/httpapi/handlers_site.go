package httpapi

import (
	"context"
	"html"
	"net/http"
	"regexp"
	"strings"
)

// SiteTitleKey is the app.db settings key holding the admin-configurable site
// title shown in the UI (sidebar, login, browser tab) and the OPDS feed.
const SiteTitleKey = "site_title"
const defaultSiteTitle = "Incipit"

// titleRe matches the static <title> element in the built index.html.
var titleRe = regexp.MustCompile(`<title>[^<]*</title>`)

// renderIndex rewrites index.html's <title> to the configured site title and
// adds Open Graph / Twitter tags, so pasted links preview with the right name
// (crawlers read the static HTML and never run the SPA's client-side update).
func renderIndex(raw []byte, title string) []byte {
	esc := html.EscapeString(title)
	block := "<title>" + esc + "</title>" +
		`<meta property="og:title" content="` + esc + `">` +
		`<meta property="og:site_name" content="` + esc + `">` +
		`<meta property="og:type" content="website">` +
		`<meta name="twitter:card" content="summary">`
	if titleRe.Match(raw) {
		return titleRe.ReplaceAllLiteral(raw, []byte(block))
	}
	return raw
}

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
