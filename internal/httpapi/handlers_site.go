package httpapi

import (
	"context"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// SiteTitleKey is the app.db settings key holding the admin-configurable site
// title shown in the UI (sidebar, login, browser tab) and the OPDS feed.
const SiteTitleKey = "site_title"
const defaultSiteTitle = "Incipit"

// PopularityKey toggles the favorites/popularity feature for this library
// instance: the ♥ count badge on cards, the popularity sort option and the
// detail-page count. Off by default; an admin turns it on for a library whose
// books carry a favorites count (the book_favorites table is populated).
const PopularityKey = "popularity"

func (s *Server) popularityEnabled(ctx context.Context) bool {
	v, _ := s.store.GetSetting(ctx, PopularityKey)
	return v == "true"
}

// HomeFilterTagsKey / HomeFilterExcludeTagsKey hold the admin-configured base tag
// filter always applied to the home ("/") library view: a CSV of Calibre tag IDs
// the home library is scoped to (include, AND) and one it hides (exclude, NOT).
// It is a display scope, not access control — books stay reachable via direct
// links, collections and OPDS.
const HomeFilterTagsKey = "home_filter_tags"
const HomeFilterExcludeTagsKey = "home_filter_exclude_tags"

// HomeFilterMatchAnyKey toggles how the home include tags combine: "true" = any
// (OR), otherwise all (AND). Mirrors a collection's match mode.
const HomeFilterMatchAnyKey = "home_filter_match_any"

func (s *Server) homeFilterMatchAny(ctx context.Context) bool {
	v, _ := s.store.GetSetting(ctx, HomeFilterMatchAnyKey)
	return v == "true"
}

func csvToIDs(s string) []int64 {
	ids := []int64{} // non-nil so it marshals as [] not null
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if n, err := strconv.ParseInt(p, 10, 64); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	return ids
}

func idsToCSV(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	return strings.Join(parts, ",")
}

// homeFilterTagIDs returns the configured home-library include and exclude tag IDs.
func (s *Server) homeFilterTagIDs(ctx context.Context) (include, exclude []int64) {
	iv, _ := s.store.GetSetting(ctx, HomeFilterTagsKey)
	ev, _ := s.store.GetSetting(ctx, HomeFilterExcludeTagsKey)
	return csvToIDs(iv), csvToIDs(ev)
}

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
	inc, exc := s.homeFilterTagIDs(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"title":           s.siteTitle(r.Context()),
		"pageFilter":      s.pageFilterEnabled(r.Context()),
		"popularity":      s.popularityEnabled(r.Context()),
		"homeTags":        inc,
		"homeExcludeTags": exc,
		"homeMatchAny":    s.homeFilterMatchAny(r.Context()),
	})
}

type siteUpdateBody struct {
	Title           string   `json:"title"`
	PageFilter      *bool    `json:"pageFilter"`
	Popularity      *bool    `json:"popularity"`
	HomeTags        *[]int64 `json:"homeTags"`
	HomeExcludeTags *[]int64 `json:"homeExcludeTags"`
	HomeMatchAny    *bool    `json:"homeMatchAny"`
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
	if body.Popularity != nil {
		val := "false"
		if *body.Popularity {
			val = "true"
		}
		if err := s.store.SetSetting(r.Context(), PopularityKey, val); err != nil {
			writeError(w, http.StatusInternalServerError, "save popularity")
			return
		}
	}
	if body.HomeTags != nil {
		if err := s.store.SetSetting(r.Context(), HomeFilterTagsKey, idsToCSV(*body.HomeTags)); err != nil {
			writeError(w, http.StatusInternalServerError, "save home filter")
			return
		}
	}
	if body.HomeExcludeTags != nil {
		if err := s.store.SetSetting(r.Context(), HomeFilterExcludeTagsKey, idsToCSV(*body.HomeExcludeTags)); err != nil {
			writeError(w, http.StatusInternalServerError, "save home filter")
			return
		}
	}
	if body.HomeMatchAny != nil {
		val := "false"
		if *body.HomeMatchAny {
			val = "true"
		}
		if err := s.store.SetSetting(r.Context(), HomeFilterMatchAnyKey, val); err != nil {
			writeError(w, http.StatusInternalServerError, "save home filter")
			return
		}
	}
	inc, exc := s.homeFilterTagIDs(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"title":           title,
		"pageFilter":      s.pageFilterEnabled(r.Context()),
		"popularity":      s.popularityEnabled(r.Context()),
		"homeTags":        inc,
		"homeExcludeTags": exc,
		"homeMatchAny":    s.homeFilterMatchAny(r.Context()),
	})
}
