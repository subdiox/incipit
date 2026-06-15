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

// Handler serves the SPA: real files are served directly with long-lived cache
// headers for hashed assets; any other path falls back to index.html.
func Handler() http.Handler {
	sub, err := FS()
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	index, _ := fs.ReadFile(sub, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveIndex(w, index)
			return
		}
		if info, err := fs.Stat(sub, p); err != nil || info.IsDir() {
			serveIndex(w, index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if len(index) == 0 {
		index = []byte("<!doctype html><title>Incipit</title><p>Frontend not built.</p>")
	}
	w.Write(index)
}
