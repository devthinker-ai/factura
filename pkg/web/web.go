// Package web serves the embedded React compliance console SPA.
//
// go:embed cannot use ".." paths, so Makefile syncs frontend/dist → pkg/web/dist
// before `go build`. A placeholder dist is committed so `go test ./...` works
// without npm; `make frontend` (or `make build` when sources are newer) replaces it.
package web

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const placeholderHint = "frontend not built — run make frontend"

// Handler serves the production embed at /app/ (caller mounts with StripPrefix).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("web: embed dist: " + err.Error())
	}
	return HandlerFromFS(sub)
}

// HandlerFromFS serves an SPA from sub (testable; used by Handler()).
// Exact asset paths are served from the FS; any other path falls back to
// index.html for client-side routing. If the embed is still the placeholder
// (no real index.html), returns 503 with a build hint.
func HandlerFromFS(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPlaceholder(sub) {
			http.Error(w, placeholderHint, http.StatusServiceUnavailable)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			serveIndex(w, sub)
			return
		}
		f, err := sub.Open(path)
		if err != nil {
			serveIndex(w, sub)
			return
		}
		stat, err := f.Stat()
		_ = f.Close()
		if err != nil || stat.IsDir() {
			serveIndex(w, sub)
			return
		}
		// Hashed Vite assets are immutable; HTML must revalidate.
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else if path == "index.html" || strings.HasSuffix(path, ".html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

func isPlaceholder(sub fs.FS) bool {
	_, err := sub.Open("index.html")
	if err != nil {
		return true
	}
	return false
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	f, err := sub.Open("index.html")
	if err != nil {
		http.Error(w, placeholderHint, http.StatusServiceUnavailable)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
}

// DistFS exposes the embed for tests that assert asset bytes.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
