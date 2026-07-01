package httpapi

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
	"incipit/internal/auth"
	"incipit/internal/calibre"
	"incipit/internal/config"
	"incipit/internal/metadata"
	"incipit/internal/reader"
	"incipit/web"
)

// LibraryPathKey is the app.db settings key holding the Calibre library path
// chosen during first-run setup (when INCIPIT_LIBRARY is not set).
const LibraryPathKey = "library_path"

// Server holds the HTTP dependencies and builds the router.
type Server struct {
	cfg      *config.Config
	libPtr   atomic.Pointer[calibre.Adapter] // set at boot or during setup
	store    *appdb.Store
	auth     *auth.Service
	reader   *reader.Service
	ldap     *auth.LDAPManager
	proxyCfg auth.ProxyConfig
	limiter  *rateLimiter
	meta     *metadata.Client
	previews *previewStore
}

// New constructs a Server. lib may be nil when the library has not been
// configured yet (it is then opened during first-run setup).
func New(cfg *config.Config, lib *calibre.Adapter, store *appdb.Store, authSvc *auth.Service, rd *reader.Service, ldap *auth.LDAPManager) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		auth:   authSvc,
		reader: rd,
		ldap:   ldap,
		proxyCfg: auth.ProxyConfig{
			Enabled:     cfg.ProxyAuth.Enabled,
			UserHeader:  cfg.ProxyAuth.UserHeader,
			AdminHeader: cfg.ProxyAuth.AdminHeader,
			AutoCreate:  cfg.ProxyAuth.AutoCreate,
		},
		limiter:  newRateLimiter(10, time.Minute),
		meta:     metadata.NewClient(),
		previews: newPreviewStore(),
	}
	if lib != nil {
		s.libPtr.Store(lib)
	}
	return s
}

// lib returns the current Calibre adapter, or nil if the library has not been
// configured yet.
func (s *Server) lib() *calibre.Adapter { return s.libPtr.Load() }

// libraryConfigured reports whether a Calibre library is open.
func (s *Server) libraryConfigured() bool { return s.libPtr.Load() != nil }

// openLibrary opens the Calibre library at path and makes it the live one,
// closing the previously-open adapter (if any) to release its handle.
func (s *Server) openLibrary(path string) error {
	a, err := calibre.Open(path, s.cfg.ReadOnly)
	if err != nil {
		return err
	}
	if old := s.libPtr.Swap(a); old != nil {
		_ = old.Close()
	}
	return nil
}

// Router builds the complete HTTP handler.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(recoverer)
	r.Use(requestLogger)

	r.Route("/api", func(r chi.Router) {
		// Unauthenticated endpoints.
		r.Get("/setup/status", s.handleSetupStatus)
		r.Post("/setup", s.handleSetup)
		r.Get("/site", s.handleGetSite)
		r.Get("/fs", s.handleBrowseFs) // setup-or-admin gated inside
		r.Post("/auth/login", s.handleLogin)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.csrfProtect)

			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)
			r.Post("/auth/logout", s.handleLogout)

			r.Get("/metadata/genres", s.handleMetadataGenres)
			r.Post("/metadata/preview", s.handleMetadataPreview)
			r.Get("/metadata/preview/{token}/cover", s.handleMetadataPreviewCover)

			r.Get("/books", s.handleListBooks)
			r.Post("/books", s.handleAddBook)
			r.Get("/books/{id}", s.handleGetBook)
			r.Put("/books/{id}", s.handleUpdateBook)
			r.Delete("/books/{id}", s.handleDeleteBook)
			r.Get("/books/{id}/cover", s.handleCover)
			r.Put("/books/{id}/cover", s.handleSetCover)
			r.Get("/books/{id}/thumbnail", s.handleThumbnail)
			r.Get("/books/{id}/file", s.handleDownload)
			r.Get("/books/{id}/content", s.handleContent)
			r.Get("/books/{id}/pages", s.handlePageList)
			r.Get("/books/{id}/pages/{n}", s.handlePage)
			r.Get("/books/{id}/progress", s.handleGetProgress)
			r.Put("/books/{id}/progress", s.handleSetProgress)
			r.Delete("/books/{id}/progress", s.handleResetProgress)
			r.Get("/books/{id}/views", s.handleBookViews)
			r.Post("/books/{id}/views", s.handleRecordView)

			// Reading history (per-user).
			r.Get("/me/reading", s.handleMyReading)

			r.Get("/authors", s.handleAuthors)
			r.Get("/series", s.handleSeries)
			r.Get("/tags", s.handleTags)
			r.Get("/publishers", s.handlePublishers)
			r.Get("/languages", s.handleLanguages)
			r.Get("/stats", s.handleStats)

			// Admin-defined library panes (saved tag filters); the list is
			// visible to everyone, mutations are admin-only (below).
			r.Get("/panes", s.handleListPanes)

			r.Get("/shelves", s.handleListShelves)
			r.Post("/shelves", s.handleCreateShelf)
			r.Delete("/shelves/{id}", s.handleDeleteShelf)
			r.Get("/shelves/{id}/books", s.handleShelfBooks)
			r.Post("/shelves/{id}/books/{bookId}", s.handleAddToShelf)
			r.Delete("/shelves/{id}/books/{bookId}", s.handleRemoveFromShelf)

			r.Route("/admin", func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Put("/site", s.handleUpdateSite)

				r.Post("/panes", s.handleCreatePane)
				r.Put("/panes/{id}", s.handleUpdatePane)
				r.Delete("/panes/{id}", s.handleDeletePane)

				r.Get("/library", s.handleGetLibrary)
				r.Put("/library", s.handleUpdateLibrary)

				r.Get("/ldap", s.handleGetLDAP)
				r.Put("/ldap", s.handleUpdateLDAP)
				r.Post("/ldap/test", s.handleTestLDAP)
				r.Post("/ldap/import", s.handleImportLDAP)
			})
		})
	})

	// OPDS catalog (HTTP Basic auth, for external reader apps).
	r.Route("/opds", func(r chi.Router) {
		r.Use(s.basicAuth)
		r.Get("/", s.handleOPDSRoot)
		r.Get("/new", s.handleOPDSNew)
		r.Get("/authors", s.handleOPDSAuthors)
		r.Get("/authors/{id}", s.handleOPDSAuthor)
		r.Get("/series", s.handleOPDSSeries)
		r.Get("/series/{id}", s.handleOPDSSeriesBooks)
		r.Get("/opensearch.xml", s.handleOPDSOpenSearch)
		r.Get("/search", s.handleOPDSSearch)
		r.Get("/search/{terms}", s.handleOPDSSearchPath)
	})

	r.Get("/healthz", s.handleHealth)

	// SPA fallback for everything else.
	r.Handle("/*", s.spaHandler())
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// spaHandler serves the embedded frontend, injecting the configured site title
// and Open Graph tags into index.html so link previews show the right name.
func (s *Server) spaHandler() http.Handler {
	raw := web.IndexHTML()
	return web.Handler(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if len(raw) == 0 {
			w.Write([]byte("<!doctype html><title>Incipit</title><p>Frontend not built.</p>"))
			return
		}
		w.Write(renderIndex(raw, s.siteTitle(r.Context())))
	})
}
