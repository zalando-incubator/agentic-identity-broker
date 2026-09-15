package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSPAHandlerServesAssetsWithoutEscapingStaticPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	staticPath := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(staticPath, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "assets", "app.js"), []byte("asset"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o600))

	handler := NewSPAHandler(staticPath, slog.New(slog.NewTextHandler(io.Discard, nil)))

	t.Run("serves a root-mounted asset", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, "asset", recorder.Body.String())
	})

	t.Run("does not serve a file outside the static path", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/%2e%2e/secret.txt", nil))

		require.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestSPAHandlerSetsSecurityHeaders(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	staticPath := filepath.Join(root, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(staticPath, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("index"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "assets", "app.js"), []byte("asset"), 0o600))

	handler := NewSPAHandler(staticPath, slog.New(slog.NewTextHandler(io.Discard, nil)))
	securityHeaders := map[string]string{
		"X-Frame-Options":         "DENY",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "frame-ancestors 'none'",
	}

	for _, test := range []struct {
		name string
		path string
	}{
		{name: "serves a static asset", path: "/assets/app.js"},
		{name: "falls back to the SPA entrypoint", path: "/delegations"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))

			require.Equal(t, http.StatusOK, recorder.Code)
			for header, expected := range securityHeaders {
				require.Equal(t, []string{expected}, recorder.Header().Values(header), header)
			}
		})
	}
}
