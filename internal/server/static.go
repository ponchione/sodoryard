package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

// staticHandler serves embedded frontend files with SPA fallback.
// Non-API requests that don't match a static file get index.html.
func staticHandler(logger *slog.Logger, frontendFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(frontendFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't serve static files for API routes.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the exact file.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(frontendFS, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: serve index.html for unknown paths (client-side routing).
		logger.Debug("spa fallback", "path", r.URL.Path)
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
