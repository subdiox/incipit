// Package web embeds the built frontend SPA and serves it with history-API
// fallback (unknown paths return index.html so client-side routing works).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFiles embed.FS

// FS returns the embedded dist/ filesystem.
func FS() (fs.FS, error) {
	return fs.Sub(distFiles, "dist")
}

// IndexHTML returns the embedded index.html (empty if the frontend isn't built).
func IndexHTML() []byte {
	sub, err := FS()
	if err != nil {
		return nil
	}
	b, _ := fs.ReadFile(sub, "index.html")
	return b
}

// Handler serves the SPA: real files are served directly with long-lived cache
// headers for hashed assets; any other path falls back to serveIndex (so the
// caller can inject the configured site title / Open Graph tags).
func Handler(serveIndex http.HandlerFunc) http.Handler {
	sub, err := FS()
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w, r)
			return
		}
		if info, err := fs.Stat(sub, p); err != nil || info.IsDir() {
			serveIndex(w, r)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
