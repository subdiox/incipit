package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"incipit/internal/appdb"
	"incipit/internal/auth"
	"incipit/internal/calibre"
	"incipit/internal/config"
	"incipit/internal/reader"
	"incipit/web"
)

// Server holds the HTTP dependencies and builds the router.
type Server struct {
	cfg      *config.Config
	lib      *calibre.Adapter
	store    *appdb.Store
	auth     *auth.Service
	reader   *reader.Service
	proxyCfg auth.ProxyConfig
	limiter  *rateLimiter
}

// New constructs a Server.
func New(cfg *config.Config, lib *calibre.Adapter, store *appdb.Store, authSvc *auth.Service, rd *reader.Service) *Server {
	return &Server{
		cfg:    cfg,
		lib:    lib,
		store:  store,
		auth:   authSvc,
		reader: rd,
		proxyCfg: auth.ProxyConfig{
			Enabled:     cfg.ProxyAuth.Enabled,
			UserHeader:  cfg.ProxyAuth.UserHeader,
			AdminHeader: cfg.ProxyAuth.AdminHeader,
			AutoCreate:  cfg.ProxyAuth.AutoCreate,
		},
		limiter: newRateLimiter(10, time.Minute),
	}
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
		r.Post("/auth/login", s.handleLogin)

		// Authenticated endpoints.
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Use(s.csrfProtect)

			r.Get("/auth/me", s.handleMe)
			r.Post("/auth/logout", s.handleLogout)

			r.Get("/books", s.handleListBooks)
			r.Post("/books", s.handleAddBook)
			r.Get("/books/{id}", s.handleGetBook)
			r.Put("/books/{id}", s.handleUpdateBook)
			r.Delete("/books/{id}", s.handleDeleteBook)
			r.Get("/books/{id}/cover", s.handleCover)
			r.Get("/books/{id}/thumbnail", s.handleThumbnail)
			r.Get("/books/{id}/file", s.handleDownload)
			r.Get("/books/{id}/pages", s.handlePageList)
			r.Get("/books/{id}/pages/{n}", s.handlePage)
			r.Get("/books/{id}/progress", s.handleGetProgress)
			r.Put("/books/{id}/progress", s.handleSetProgress)

			r.Get("/authors", s.handleAuthors)
			r.Get("/series", s.handleSeries)
			r.Get("/tags", s.handleTags)
			r.Get("/publishers", s.handlePublishers)
			r.Get("/languages", s.handleLanguages)
			r.Get("/stats", s.handleStats)

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
		r.Get("/search", s.handleOPDSSearch)
	})

	r.Get("/healthz", s.handleHealth)

	// SPA fallback for everything else.
	r.Handle("/*", s.spaHandler())
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// spaHandler serves the embedded frontend.
func (s *Server) spaHandler() http.Handler {
	return web.Handler()
}
