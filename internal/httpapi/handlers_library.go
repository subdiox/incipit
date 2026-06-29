package httpapi

import (
	"net/http"
	"strings"
)

// librarySettings is the admin-facing view of the configured Calibre library.
type librarySettings struct {
	Path       string `json:"path"`
	ReadOnly   bool   `json:"readOnly"`
	Configured bool   `json:"configured"`
}

func (s *Server) currentLibrarySettings() librarySettings {
	path := ""
	if a := s.lib(); a != nil {
		path = a.LibraryPath()
	}
	return librarySettings{
		Path:       path,
		ReadOnly:   s.cfg.ReadOnly,
		Configured: s.libraryConfigured(),
	}
}

// handleGetLibrary returns the current Calibre library path and mode.
func (s *Server) handleGetLibrary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.currentLibrarySettings())
}

type libraryUpdateBody struct {
	Path string `json:"path"`
}

// handleUpdateLibrary points Incipit at a different Calibre library, opening it
// (creating it when missing and not read-only) and swapping it in live.
func (s *Server) handleUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	var body libraryUpdateBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	path := strings.TrimSpace(body.Path)
	if path == "" {
		writeError(w, http.StatusBadRequest, "library path is required")
		return
	}
	if err := s.openLibrary(path); err != nil {
		writeError(w, http.StatusBadRequest, "cannot open library at that path: "+err.Error())
		return
	}
	if err := s.store.SetSetting(r.Context(), LibraryPathKey, path); err != nil {
		writeError(w, http.StatusInternalServerError, "save library path")
		return
	}
	writeJSON(w, http.StatusOK, s.currentLibrarySettings())
}
