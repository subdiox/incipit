package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fsEntry is a sub-directory of the browsed path.
type fsEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// fsListing is a server-side directory listing (folders only).
type fsListing struct {
	Path    string    `json:"path"`
	Parent  string    `json:"parent"` // "" when at the filesystem root
	Entries []fsEntry `json:"entries"`
}

// handleBrowseFs lists sub-directories of a server-side path so the UI can offer
// a folder picker for the library location. It is allowed during first-run
// setup (no users yet) and, once set up, only to admins.
func (s *Server) handleBrowseFs(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count users")
		return
	}
	if n > 0 {
		if u, _ := s.authenticate(r); u == nil || !u.IsAdmin {
			writeError(w, http.StatusForbidden, "admin privileges required")
			return
		}
	}

	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		p = s.defaultBrowseStart()
	}
	abs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		writeError(w, http.StatusBadRequest, "not a directory")
		return
	}
	dirents, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read directory: "+err.Error())
		return
	}

	entries := []fsEntry{}
	for _, de := range dirents {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue // folders only, skip hidden
		}
		entries = append(entries, fsEntry{Name: de.Name(), Path: filepath.Join(abs, de.Name())})
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	parent := filepath.Dir(abs)
	if parent == abs {
		parent = "" // at the filesystem root
	}
	writeJSON(w, http.StatusOK, fsListing{Path: abs, Parent: parent, Entries: entries})
}

// defaultBrowseStart picks a friendly starting directory: the configured
// library, else the user's home directory, else the filesystem root.
func (s *Server) defaultBrowseStart() string {
	if a := s.lib(); a != nil && a.LibraryPath() != "" {
		if abs, err := filepath.Abs(a.LibraryPath()); err == nil {
			return abs
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return string(filepath.Separator)
}
