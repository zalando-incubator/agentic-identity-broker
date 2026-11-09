package middleware_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	adapterhttp "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHandler_ResolvesClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
		configure    func(t *testing.T, cfg *adapterhttp.ServerConfig)
		wantClientIP string
	}{
		{
			name:         "uses direct remote address when trusted proxy is disabled",
			remoteAddr:   "203.0.113.10:4567",
			wantClientIP: "203.0.113.10",
		},
		{
			name:         "uses right-most forwarded hop when trusted proxy is enabled",
			remoteAddr:   "10.0.0.5:9443",
			forwardedFor: "198.51.100.7, 203.0.113.44",
			configure: func(t *testing.T, cfg *adapterhttp.ServerConfig) {
				setNestedField(t, cfg, true, "RequestContext", "TrustedProxy", "Enabled")
				setNestedField(t, cfg, "X-Forwarded-For", "RequestContext", "TrustedProxy", "ForwardedHeader")
			},
			wantClientIP: "203.0.113.44",
		},
		{
			name:         "ignores spoofable left-most forwarded entry",
			remoteAddr:   "10.0.0.5:9443",
			forwardedFor: "198.51.100.7, 192.0.2.99, 203.0.113.44",
			configure: func(t *testing.T, cfg *adapterhttp.ServerConfig) {
				setNestedField(t, cfg, true, "RequestContext", "TrustedProxy", "Enabled")
				setNestedField(t, cfg, "X-Forwarded-For", "RequestContext", "TrustedProxy", "ForwardedHeader")
			},
			wantClientIP: "203.0.113.44",
		},
		{
			name:         "falls back to safe default when forwarded header is malformed",
			remoteAddr:   "10.0.0.5:9443",
			forwardedFor: "not-an-ip",
			configure: func(t *testing.T, cfg *adapterhttp.ServerConfig) {
				setNestedField(t, cfg, true, "RequestContext", "TrustedProxy", "Enabled")
				setNestedField(t, cfg, "X-Forwarded-For", "RequestContext", "TrustedProxy", "ForwardedHeader")
			},
			wantClientIP: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestServerConfig()
			if tt.configure != nil {
				tt.configure(t, &cfg)
			}

			var observed observedSecurityContext
			handler := adapterhttp.NewHandler(cfg, func(r chi.Router) {
				r.Get("/api/ip", func(w http.ResponseWriter, r *http.Request) {
					observed.sc, observed.ok = security.FromContext(r.Context())
					w.WriteHeader(http.StatusNoContent)
				})
			}, slog.New(slog.DiscardHandler))

			req := httptest.NewRequest(http.MethodGet, "/api/ip", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedFor)
			}
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusNoContent, res.Code, "client-ip capture must remain fail-open")
			require.True(t, observed.ok, "client-ip resolution must contribute to the request security context")
			assert.Equal(t, tt.wantClientIP, observed.sc.ClientIP)
		})
	}
}
