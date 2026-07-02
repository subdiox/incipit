package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/metadata"
)

// Metadata previews let the uploader fetch and review cmoa matches (and their
// covers) before committing, then commit by token without re-fetching.
const (
	previewTTL  = 30 * time.Minute
	maxPreviews = 1000
)

type previewEntry struct {
	meta    *metadata.Meta
	cover   []byte // re-encoded JPEG, may be nil when cmoa has no cover
	expires time.Time
}

// previewStore is an in-memory, TTL'd cache of fetched metadata keyed by an
// opaque token. It is intentionally ephemeral: a server restart just means the
// user re-previews.
type previewStore struct {
	mu      sync.Mutex
	entries map[string]*previewEntry
}

func newPreviewStore() *previewStore {
	return &previewStore{entries: make(map[string]*previewEntry)}
}

func (p *previewStore) put(token string, e *previewEntry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.entries) >= maxPreviews {
		now := time.Now()
		for k, v := range p.entries {
			if now.After(v.expires) {
				delete(p.entries, k)
			}
		}
	}
	p.entries[token] = e
}

func (p *previewStore) get(token string) (*previewEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[token]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(p.entries, token)
		return nil, false
	}
	return e, true
}

type metaPreviewBody struct {
	Query       string `json:"query"`
	Genre       string `json:"genre"`
	MetaAdd     string `json:"metaAdd"`
	MetaExclude string `json:"metaExclude"`
}

type metaPreviewResponse struct {
	Matched     bool     `json:"matched"`
	Token       string   `json:"token,omitempty"`
	Title       string   `json:"title,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Series      string   `json:"series,omitempty"`
	SeriesIndex float64  `json:"seriesIndex,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Pubdate     string   `json:"pubdate,omitempty"`
	Rating      int      `json:"rating,omitempty"`
	Comments    string   `json:"comments,omitempty"`
	HasCover    bool     `json:"hasCover"`
}

// handleMetadataPreview fetches a cmoa match for one query and caches it (with
// its cover) under a token, without creating a book. Returns matched:false when
// cmoa has no hit, so the client can let the user edit the query and retry.
func (s *Server) handleMetadataPreview(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil || !u.CanUpload {
		writeError(w, http.StatusForbidden, "upload not permitted")
		return
	}
	var body metaPreviewBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	query := strings.TrimSpace(body.Query)
	if query == "" {
		writeError(w, http.StatusBadRequest, "query is required")
		return
	}

	m, err := s.meta.Fetch(r.Context(), query, body.Genre,
		strings.TrimSpace(body.MetaAdd), strings.TrimSpace(body.MetaExclude))
	if err != nil {
		writeError(w, http.StatusBadGateway, "metadata lookup failed: "+err.Error())
		return
	}
	if m == nil {
		writeJSON(w, http.StatusOK, metaPreviewResponse{Matched: false})
		return
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token")
		return
	}
	var cover []byte
	if m.CoverURL != "" {
		cover = s.coverFromURL(r.Context(), m.CoverURL)
	}
	s.previews.put(token, &previewEntry{meta: m, cover: cover, expires: time.Now().Add(previewTTL)})

	resp := metaPreviewResponse{
		Matched:     true,
		Token:       token,
		Title:       m.Title,
		Authors:     m.Authors,
		Series:      m.Series,
		SeriesIndex: m.SeriesIndex,
		Tags:        m.TagsWithCategory(),
		Publisher:   m.Publisher,
		Rating:      m.Rating,
		Comments:    m.Comments,
		HasCover:    len(cover) > 0,
	}
	if !m.PubDate.IsZero() {
		resp.Pubdate = m.PubDate.Format("2006-01-02")
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleMetadataPreviewCover serves a cached preview cover so the review UI can
// show the official cover before the book exists.
func (s *Server) handleMetadataPreviewCover(w http.ResponseWriter, r *http.Request) {
	e, ok := s.previews.get(chi.URLParam(r, "token"))
	if !ok || len(e.cover) == 0 {
		writeError(w, http.StatusNotFound, "no preview cover")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=600")
	w.Write(e.cover)
}
