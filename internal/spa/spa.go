// Package spa serves the Vue SPA bundle embedded into the Go binary.
//
// The bundle is produced by `npm run build` into frontend/dist and then
// copied into internal/spa/dist during the Docker multi-stage build
// (see Dockerfile). A placeholder index.html is committed to git so local
// `go build` without a frontend works and `go:embed` has something to find.
//
// The handler serves static assets by path and falls back to index.html for
// any unknown route, enabling HTML5 history-mode routing on the client.
package spa

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA.
//
// Behaviour:
//   - GET /assets/... and other known static paths are served with the
//     appropriate Content-Type and long-cache headers.
//   - Any other GET falls back to index.html (SPA history-mode).
//   - Non-GET requests get 404 (API routes should be mounted before this).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Panicking here is deliberate — the binary was built without the
		// dist directory, which is a build error we want to surface loudly.
		panic("spa: fs.Sub(dist): " + err.Error())
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		panic("spa: read index.html: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}

		// Normalize path: drop leading slash.
		path := strings.TrimPrefix(r.URL.Path, "/")

		if path == "" {
			serveIndex(w, index)
			return
		}

		// Check whether the asset actually exists in the bundle.
		if info, err := fs.Stat(sub, path); err == nil && !info.IsDir() {
			// Long-cache hashed assets (vite output: /assets/app-abc123.js).
			if strings.HasPrefix(path, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Asset paths that don't exist must 404 — if we fall back to index.html
		// here, the browser receives HTML with a JS/CSS Content-Type guess and
		// blocks it under strict MIME checking, leading to endless retries of
		// stale hashed filenames after a deploy. Let the 404 propagate so the
		// client reloads index.html on the next navigation and picks up fresh
		// asset hashes.
		if strings.HasPrefix(path, "assets/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: let the client router handle unknown routes.
		serveIndex(w, index)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	_, _ = w.Write(index)
}
