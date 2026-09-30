package routing_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

func TestSetupAdminRoutes_RequestSecurity(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		host        string
		contentType string
		origin      string
		fetchSite   string
		status      int
	}{
		{name: "non-browser JSON", host: "admin.example.com:14000", contentType: "application/json", status: http.StatusCreated},
		{name: "same-origin browser", host: "admin.example.com:14000", contentType: "application/json", origin: "https://admin.example.com:14000", fetchSite: "same-origin", status: http.StatusCreated},
		{name: "JSON parameters", host: "admin.example.com:14000", contentType: "application/json; charset=utf-8", status: http.StatusCreated},
		{name: "case-insensitive hostname", host: "ADMIN.EXAMPLE.COM:14000", contentType: "application/json", status: http.StatusCreated},
		{name: "cross-site browser", host: "admin.example.com:14000", contentType: "application/json", fetchSite: "cross-site", status: http.StatusForbidden},
		{name: "same-site browser", host: "admin.example.com:14000", contentType: "application/json", fetchSite: "same-site", status: http.StatusForbidden},
		{name: "legacy cross-origin browser", host: "admin.example.com:14000", contentType: "application/json", origin: "https://attacker.example", status: http.StatusForbidden},
		{name: "CORS does not bypass protection", host: "admin.example.com:14000", contentType: "application/json", origin: "https://console.example.com", fetchSite: "cross-site", status: http.StatusForbidden},
		{name: "null origin", host: "admin.example.com:14000", contentType: "application/json", origin: "null", status: http.StatusForbidden},
		{name: "text plain form", host: "admin.example.com:14000", contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		{name: "form encoded", host: "admin.example.com:14000", contentType: "application/x-www-form-urlencoded", status: http.StatusUnsupportedMediaType},
		{name: "missing content type", host: "admin.example.com:14000", status: http.StatusUnsupportedMediaType},
		{name: "malformed content type", host: "admin.example.com:14000", contentType: "application/json; charset", status: http.StatusUnsupportedMediaType},
		{name: "rebinding same-origin", host: "attacker.example:14000", contentType: "application/json", origin: "https://attacker.example:14000", fetchSite: "same-origin", status: http.StatusForbidden},
		{name: "wrong port", host: "admin.example.com:8000", contentType: "application/json", status: http.StatusForbidden},
		{name: "missing configured port", host: "admin.example.com", contentType: "application/json", status: http.StatusForbidden},
		{name: "multiple ports", host: "admin.example.com:14000:443", contentType: "application/json", status: http.StatusForbidden},
		{name: "hostname suffix", host: "admin.example.com.attacker.example:14000", contentType: "application/json", status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newAdminRouter(t, "https://admin.example.com:14000")
			req := httptest.NewRequest(http.MethodPost, "/api/agents", strings.NewReader(`{"display_name":"Security test agent","description":"Admin request security regression","permission_sets":[{"permission_set_id":"`+fixtures.PlaceholderPermissionSetID.String()+`","requirement_type":"optional"}]}`))
			req.Host = tc.host
			req.Header.Set("Content-Type", tc.contentType)
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			req.Header.Set("X-Forwarded-Host", "admin.example.com:14000")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())

			listReq := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
			listReq.Host = "admin.example.com:14000"
			listRec := httptest.NewRecorder()
			router.ServeHTTP(listRec, listReq)
			require.Equal(t, http.StatusOK, listRec.Code)
			var agents []struct {
				DisplayName string `json:"display_name"`
			}
			require.NoError(t, json.NewDecoder(listRec.Body).Decode(&agents))
			if tc.status == http.StatusCreated {
				require.Equal(t, "Security test agent", agents[0].DisplayName)
			} else {
				require.Empty(t, agents, "rejected request must not create an agent")
			}
		})
	}
}

func TestSetupAdminRoutes_RejectsNonJSONWritesAndBodies(t *testing.T) {
	t.Parallel()
	router := newAdminRouter(t, "https://admin.example.com:14000")
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodGet, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/agents", strings.NewReader(`{"display_name":"Rejected"}`))
			req.Host = "admin.example.com:14000"
			req.ContentLength = -1
			req.TransferEncoding = []string{"chunked"}
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
		})
	}
	for _, headers := range [][]string{{}, {"application/json", "text/plain"}} {
		req := httptest.NewRequest(http.MethodPost, "/api/agents", nil)
		req.Host = "admin.example.com:14000"
		for _, header := range headers {
			req.Header.Add("Content-Type", header)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	}
}

func TestSetupAdminRoutes_HostOnReadsAndHealth(t *testing.T) {
	t.Parallel()

	for _, publicURL := range []string{"https://admin.example.com", "https://admin.example.com:443", "http://[::1]:14000"} {
		t.Run(publicURL, func(t *testing.T) {
			router := newAdminRouter(t, publicURL)
			validHosts := []string{"admin.example.com", "admin.example.com:443"}
			if strings.Contains(publicURL, "[::1]") {
				validHosts = []string{"[::1]:14000"}
			}
			for _, host := range validHosts {
				req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
				req.Host = host
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code, host)
			}
			for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
				req := httptest.NewRequest(method, "/api/agents", nil)
				req.Host = "attacker.example"
				req.Header.Set("Origin", "https://console.example.com")
				req.Header.Set("Access-Control-Request-Method", "POST")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Equal(t, http.StatusForbidden, rec.Code)
			}
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.Host = "127.0.0.1"
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code)
		})
	}
}

func newAdminRouter(t *testing.T, publicURL string) http.Handler {
	t.Helper()
	cfg := fixtures.DefaultOAuth2Config()
	cfg.Server.Admin.PublicURL = publicURL
	cfg.Server.Admin.CORS.AllowedOrigins = []string{"https://console.example.com"}
	storage, err := storageadapter.NewAdapter(&cfg.Storage)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close(context.Background())) })
	require.NoError(t, storage.PermissionSets().Create(context.Background(), &domainstorage.PermissionSet{
		ID: fixtures.PlaceholderPermissionSetID, Name: "Security test permission set",
		ServiceScopes: []domainstorage.ServiceScope{{ServiceID: fixtures.PlaceholderServiceID, RequirementType: domainstorage.RequirementTypeOptional}},
	}))
	staticPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(staticPath, "index.html"), []byte("<!doctype html><title>test</title>"), 0o600))
	application, err := app.NewBuilder().WithConfig(cfg).WithStorage(storage).
		WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithStaticWebResourcesPath(staticPath).WithJWKSPublisher(&mockJWKSPublisher{}).Build()
	require.NoError(t, err)
	t.Cleanup(func() {
		if application.Shutdown != nil {
			require.NoError(t, application.Shutdown(context.Background()))
		}
	})
	router := chi.NewRouter()
	routing.SetupAdminRoutes(router, application.AdminHandlers, routing.AdminRouteConfig{
		CORS: cfg.Server.Admin.CORS, PublicURL: publicURL,
	})
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return router
}
