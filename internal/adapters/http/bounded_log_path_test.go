package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogsBoundPath(t *testing.T) {
	const limit = 2048
	const marker = "...[truncated]"
	prefix := strings.Repeat("a", limit-len(marker)-1)
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"empty", "", ""},
		{"ordinary", "/api/resource", "/api/resource"},
		{"invalid oversized UTF-8", strings.Repeat("\x80", limit+1), marker},
		{"at limit", strings.Repeat("a", limit), strings.Repeat("a", limit)},
		{"over limit", strings.Repeat("a", limit+1), strings.Repeat("a", limit-len(marker)) + marker},
		{"multibyte at cut", prefix + "é" + strings.Repeat("a", 20), prefix + marker},
		{"four-byte rune at cut", prefix + "🌍" + strings.Repeat("a", 20), prefix + marker},
		{"multibyte at input limit", strings.Repeat("a", limit-1) + "é", strings.Repeat("a", limit-len(marker)) + marker},
		{"escaped at limit", strings.Repeat("\n", limit/2), strings.Repeat(`\n`, limit/2)},
		{"escaping exceeds limit", strings.Repeat("\n", limit/2+1), strings.Repeat(`\n`, (limit-len(marker))/2) + marker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mw := range []struct {
				name  string
				wrap  func(*slog.Logger) func(http.Handler) http.Handler
				panic bool
			}{
				{"access", func(l *slog.Logger) func(http.Handler) http.Handler { return LoggingMiddleware(l) }, false},
				{"recovery", func(l *slog.Logger) func(http.Handler) http.Handler { return RecoveryMiddleware(l, false) }, true},
				{"oauth2 audit", httpmiddleware.OAuth2AuditMiddleware, false},
			} {
				t.Run(mw.name, func(t *testing.T) {
					var buf bytes.Buffer
					logger := slog.New(slog.NewJSONHandler(&buf, nil))
					handler := mw.wrap(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						if mw.panic {
							panic("test panic")
						}
						w.WriteHeader(http.StatusNoContent)
					}))
					req := httptest.NewRequest(http.MethodGet, "/api/resource?code=synthetic-query-secret", nil)
					req.URL.Path = tc.path
					req.Header.Set("Authorization", "Bearer synthetic-bearer-secret")
					rr := httptest.NewRecorder()

					handler.ServeHTTP(rr, req)

					var record map[string]any
					require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record))
					path, ok := record["path"].(string)
					require.True(t, ok)
					assert.Equal(t, tc.want, path)
					assert.LessOrEqual(t, len(path), limit)
					assert.True(t, utf8.ValidString(path))
					assert.Equal(t, tc.path, req.URL.Path, "logging must not change routing input")
					assert.NotContains(t, buf.String(), "synthetic-query-secret")
					assert.NotContains(t, buf.String(), "synthetic-bearer-secret")
					if mw.panic {
						assert.Equal(t, http.StatusInternalServerError, rr.Code)
					} else {
						assert.Equal(t, http.StatusNoContent, rr.Code)
					}
				})
			}
		})
	}
}
