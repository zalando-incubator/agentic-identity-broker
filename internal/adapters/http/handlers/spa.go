package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SPAHandler serves a Single Page Application with History API fallback.
// Routes starting with /api/ are ignored (handled by API routes).
// All other routes serve the SPA's index.html to enable client-side routing.
type SPAHandler struct {
	staticPath string
	logger     *slog.Logger
}

// NewSPAHandler creates a new SPA handler.
// staticPath: path to the directory containing the SPA files (e.g., "./dist")
func NewSPAHandler(staticPath string, logger *slog.Logger) *SPAHandler {
	return &SPAHandler{
		staticPath: staticPath,
		logger:     logger,
	}
}

// ServeHTTP implements http.Handler interface.
// Serves static files with History API fallback for client-side routing.
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")

	// API routes should not be handled by SPA
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	filePath := strings.TrimPrefix(r.URL.Path, "/")
	for _, segment := range strings.Split(filePath, "/") {
		if segment == ".." {
			http.NotFound(w, r)
			return
		}
	}
	path := filepath.Join(h.staticPath, filePath)

	// Check if the file exists
	fileInfo, err := os.Stat(path) // #nosec G703 -- request path rejects parent segments before static-root resolution.

	// If the file doesn't exist or is a directory, serve index.html (History API fallback)
	if err != nil || fileInfo.IsDir() {
		// History API fallback: serve the SPA entrypoint for client-side routes.
		indexPath := filepath.Join(h.staticPath, "index.html")

		// Check if index.html exists
		if _, err := os.Stat(indexPath); err != nil {
			h.logger.Error("SPA index.html not found",
				"path", indexPath,
				"error", err)
			http.Error(w, "SPA not configured", http.StatusNotFound)
			return
		}

		// Serve index.html
		http.ServeFile(w, r, indexPath)
		return
	}

	// File exists, serve it directly
	http.ServeFile(w, r, path) // #nosec G703 -- request path rejects parent segments before static-root resolution.
}
