package gui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed web/dist/*
var webDistFS embed.FS

// WebHandler returns an http.Handler that serves embedded static assets
// and falls back to index.html for single-page application routing.
func WebHandler() http.Handler {
	sub, err := fs.Sub(webDistFS, "web/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if f, err := sub.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
