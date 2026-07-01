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
// render the title on the login screen and know which optional features (e.g.
// the page-count filter) are enabled.
func (s *Server) handleGetSite(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"title":      s.siteTitle(r.Context()),
		"pageFilter": s.pageFilterEnabled(r.Context()),
	})
}

type siteUpdateBody struct {
	Title      string `json:"title"`
	PageFilter *bool  `json:"pageFilter"`
}

// handleUpdateSite sets the site title and options (admin only).
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
	if body.PageFilter != nil {
		val := "false"
		if *body.PageFilter {
			val = "true"
		}
		if err := s.store.SetSetting(r.Context(), PageFilterKey, val); err != nil {
			writeError(w, http.StatusInternalServerError, "save page filter")
			return
		}
		if *body.PageFilter {
			s.startPageIndex() // begin/resume indexing when enabled
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title":      title,
		"pageFilter": s.pageFilterEnabled(r.Context()),
	})
}
