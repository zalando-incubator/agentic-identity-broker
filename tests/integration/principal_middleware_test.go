package integration

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrincipalMiddlewareExtraction tests end-to-end principal extraction through chi router.
func TestPrincipalMiddlewareExtraction(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	// Create test router with chi
	router := chi.NewRouter()

	// Configure authentication for the router
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
	}

	// Apply optional principal middleware globally
	router.Use(middleware.OptionalPrincipalMiddleware(authConfig, nil, logger))

	// Create protected route that requires principal
	router.Post("/protected", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal.FromContext(r.Context())
		if !ok {
			http.Error(w, "No principal", http.StatusUnauthorized)
			return
		}

		resp := map[string]string{"principal": p}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Create public route that works with or without principal
	router.Get("/public", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal.FromContext(r.Context())
		resp := map[string]interface{}{
			"hasPrincipal": ok,
			"principal":    p,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	tests := []struct {
		name              string
		method            string
		path              string
		headerName        string
		headerValue       string
		expectedStatus    int
		expectedPrincipal string
	}{
		{
			name:              "protected route with valid principal",
			method:            http.MethodPost,
			path:              "/protected",
			headerName:        "X-Remote-User",
			headerValue:       "alice@example.com",
			expectedStatus:    http.StatusOK,
			expectedPrincipal: "alice@example.com",
		},
		{
			name:              "protected route without principal header",
			method:            http.MethodPost,
			path:              "/protected",
			headerName:        "",
			headerValue:       "",
			expectedStatus:    http.StatusUnauthorized,
			expectedPrincipal: "",
		},
		{
			name:              "public route with principal",
			method:            http.MethodGet,
			path:              "/public",
			headerName:        "X-Remote-User",
			headerValue:       "bob",
			expectedStatus:    http.StatusOK,
			expectedPrincipal: "bob",
		},
		{
			name:              "public route without principal",
			method:            http.MethodGet,
			path:              "/public",
			headerName:        "",
			headerValue:       "",
			expectedStatus:    http.StatusOK,
			expectedPrincipal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testServer.URL + tt.path
			req, err := http.NewRequest(tt.method, url, nil)
			require.NoError(t, err)

			if tt.headerValue != "" {
				req.Header.Set(tt.headerName, tt.headerValue)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == http.StatusOK {
				var respBody map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&respBody)
				require.NoError(t, err)

				if tt.expectedPrincipal != "" {
					assert.Equal(t, tt.expectedPrincipal, respBody["principal"])
				}
			}
		})
	}
}

// TestPrincipalMiddlewareValidation tests principal validation in chi routes.
func TestPrincipalMiddlewareValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
	}

	router := chi.NewRouter()

	// Create a protected group that requires principal
	router.Group(func(r chi.Router) {
		r.Use(middleware.RequirePrincipalMiddleware(authConfig, nil, logger))

		r.Get("/admin/users", func(w http.ResponseWriter, r *http.Request) {
			p, _ := principal.FromContext(r.Context())
			resp := map[string]string{"authenticatedAs": p}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		})
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	tests := []struct {
		name           string
		headerValue    string
		expectedStatus int
		description    string
	}{
		{
			name:           "valid principal",
			headerValue:    "admin@example.com",
			expectedStatus: http.StatusOK,
			description:    "Should allow valid principal",
		},
		{
			name:           "missing header",
			headerValue:    "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject missing principal",
		},
		{
			name:           "empty header value",
			headerValue:    "",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject empty principal",
		},
		{
			name:           "whitespace only",
			headerValue:    "   ",
			expectedStatus: http.StatusUnauthorized,
			description:    "Should reject whitespace-only principal",
		},
		{
			name:           "too long principal",
			headerValue:    strings.Repeat("a", 201),
			expectedStatus: http.StatusBadRequest,
			description:    "Should reject principal exceeding 200 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testServer.URL + "/admin/users"
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			if tt.headerValue != "" {
				req.Header.Set("X-Remote-User", tt.headerValue)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode, tt.description)

			if tt.expectedStatus != http.StatusOK {
				// Verify error response format
				var errResp map[string]string
				err := json.NewDecoder(resp.Body).Decode(&errResp)
				require.NoError(t, err)
				assert.NotEmpty(t, errResp["error"])
			}
		})
	}
}

// TestPrincipalMiddlewareUnicodeSupport tests that unicode principals are properly handled.
func TestPrincipalMiddlewareUnicodeSupport(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.OptionalPrincipalMiddleware(authConfig, nil, logger))

	router.Get("/user", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal.FromContext(r.Context())
		resp := map[string]interface{}{
			"found":     ok,
			"principal": p,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	tests := []struct {
		name      string
		principal string
	}{
		{
			name:      "simple ascii",
			principal: "alice",
		},
		{
			name:      "email address",
			principal: "user@example.com",
		},
		{
			name:      "chinese characters",
			principal: "用户",
		},
		{
			name:      "emoji",
			principal: "🚀rocket",
		},
		{
			name:      "mixed unicode",
			principal: "user_用户_🎉",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testServer.URL + "/user"
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			req.Header.Set("X-Remote-User", tt.principal)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&respBody)
			require.NoError(t, err)

			found, ok := respBody["found"].(bool)
			require.True(t, ok)
			assert.True(t, found)
			assert.Equal(t, tt.principal, respBody["principal"], "Unicode principal should be preserved")
		})
	}
}

// TestPrincipalMiddlewareWhitespaceHandling tests whitespace trimming in chi routes.
func TestPrincipalMiddlewareWhitespaceHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.OptionalPrincipalMiddleware(authConfig, nil, logger))

	router.Get("/principal", func(w http.ResponseWriter, r *http.Request) {
		p, ok := principal.FromContext(r.Context())
		resp := map[string]interface{}{
			"found":     ok,
			"principal": p,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	tests := []struct {
		name              string
		rawPrincipal      string
		expectedPrincipal string
	}{
		{
			name:              "leading spaces",
			rawPrincipal:      "   alice",
			expectedPrincipal: "alice",
		},
		{
			name:              "trailing spaces",
			rawPrincipal:      "alice   ",
			expectedPrincipal: "alice",
		},
		{
			name:              "leading and trailing spaces",
			rawPrincipal:      "   alice   ",
			expectedPrincipal: "alice",
		},
		{
			name:              "tabs and spaces",
			rawPrincipal:      "\t  alice  \t",
			expectedPrincipal: "alice",
		},
		{
			name:              "no whitespace",
			rawPrincipal:      "alice",
			expectedPrincipal: "alice",
		},
		{
			name:              "internal spaces preserved",
			rawPrincipal:      "  alice bob  ",
			expectedPrincipal: "alice bob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testServer.URL + "/principal"
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			req.Header.Set("X-Remote-User", tt.rawPrincipal)

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var respBody map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&respBody)
			require.NoError(t, err)

			found, ok := respBody["found"].(bool)
			require.True(t, ok)
			assert.True(t, found)
			assert.Equal(t, tt.expectedPrincipal, respBody["principal"])
		})
	}
}

// TestPrincipalMiddlewareCustomHeader tests custom header configuration.
func TestPrincipalMiddlewareCustomHeader(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Custom-User",
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.RequirePrincipalMiddleware(authConfig, nil, logger))

	router.Get("/verify", func(w http.ResponseWriter, r *http.Request) {
		p, _ := principal.FromContext(r.Context())
		resp := map[string]string{"principal": p}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	testServer := httptest.NewServer(router)
	defer testServer.Close()

	tests := []struct {
		name           string
		headerName     string
		headerValue    string
		expectedStatus int
		expectedFound  bool
	}{
		{
			name:           "correct header name",
			headerName:     "X-Custom-User",
			headerValue:    "alice",
			expectedStatus: http.StatusOK,
			expectedFound:  true,
		},
		{
			name:           "wrong header name (X-Remote-User)",
			headerName:     "X-Remote-User",
			headerValue:    "alice",
			expectedStatus: http.StatusUnauthorized,
			expectedFound:  false,
		},
		{
			name:           "wrong header name (Authorization)",
			headerName:     "Authorization",
			headerValue:    "Bearer token",
			expectedStatus: http.StatusUnauthorized,
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := testServer.URL + "/verify"
			req, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			if tt.headerValue != "" {
				req.Header.Set(tt.headerName, tt.headerValue)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}
