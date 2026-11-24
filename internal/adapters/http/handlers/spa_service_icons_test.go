package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSPAServiceIconPolicy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>Broker</title>"), 0600))
	handler := NewSPAHandler(dir, slog.New(slog.DiscardHandler))
	for _, path := range []string{"/", "/sessions", "/agents/example"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, "DENY", response.Header().Get("X-Frame-Options"))
			assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
			policy := response.Header().Get("Content-Security-Policy")
			assert.Equal(t, "frame-ancestors 'none'; object-src 'none'", policy)
			assert.NotContains(t, policy, "img-src")
			assert.NotContains(t, policy, "default-src")
		})
	}
}
