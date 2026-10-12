package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domjwtauth "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestLogger creates a test logger that discards all output.
func createTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer([]byte{}), nil))
}

// testAuthConfig creates a test authentication config with the specified header name.
func testAuthConfig(headerName string) ports.AuthenticationConfig {
	return ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: headerName,
		},
	}
}

// TestRequirePrincipalMiddleware_ValidPrincipal tests that valid principals are extracted and added to context.
func TestRequirePrincipalMiddleware_ValidPrincipal(t *testing.T) {
	tests := []struct {
		name               string
		headerName         string
		headerValue        string
		expectedPrincipal  string
		expectedStatusCode int
	}{
		{
			name:               "simple username",
			headerName:         "X-Remote-User",
			headerValue:        "alice",
			expectedPrincipal:  "alice",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "email address",
			headerName:         "X-Remote-User",
			headerValue:        "alice@example.com",
			expectedPrincipal:  "alice@example.com",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "leading and trailing whitespace trimmed",
			headerName:         "X-Remote-User",
			headerValue:        "   alice   ",
			expectedPrincipal:  "alice",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "unicode characters preserved",
			headerName:         "X-Remote-User",
			headerValue:        "用户",
			expectedPrincipal:  "用户",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "emoji preserved",
			headerName:         "X-Remote-User",
			headerValue:        "🚀rocket",
			expectedPrincipal:  "🚀rocket",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "complex email with special chars",
			headerName:         "X-Authenticated-User",
			headerValue:        "user+tag@example.co.uk",
			expectedPrincipal:  "user+tag@example.co.uk",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "custom header name",
			headerName:         "Remote-User",
			headerValue:        "bob",
			expectedPrincipal:  "bob",
			expectedStatusCode: http.StatusOK,
		},
		{
			name:               "200 character principal (at max)",
			headerName:         "X-Remote-User",
			headerValue:        strings.Repeat("a", 200),
			expectedPrincipal:  strings.Repeat("a", 200),
			expectedStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig(tt.headerName)

			// Create a test handler that retrieves the principal from context
			handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p, ok := principal.FromContext(r.Context())
				assert.True(t, ok, "principal should be in context")
				assert.Equal(t, tt.expectedPrincipal, p)
				w.WriteHeader(http.StatusOK)
			}))

			// Create request with header
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(tt.headerName, tt.headerValue)

			// Execute middleware
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Verify response
			assert.Equal(t, tt.expectedStatusCode, rr.Code)
		})
	}
}

// TestRequirePrincipalMiddleware_MissingPrincipal tests that missing principals return 401.
func TestRequirePrincipalMiddleware_MissingPrincipal(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		headerSet  bool
		headerVal  string
	}{
		{
			name:       "header not set",
			headerName: "X-Remote-User",
			headerSet:  false,
			headerVal:  "",
		},
		{
			name:       "header empty string",
			headerName: "X-Remote-User",
			headerSet:  true,
			headerVal:  "",
		},
		{
			name:       "header whitespace only",
			headerName: "X-Remote-User",
			headerSet:  true,
			headerVal:  "   ",
		},
		{
			name:       "header tabs and newlines",
			headerName: "X-Remote-User",
			headerSet:  true,
			headerVal:  "\t\n  \t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig(tt.headerName)

			handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.headerSet {
				req.Header.Set(tt.headerName, tt.headerVal)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Should return 401 Unauthorized
			assert.Equal(t, http.StatusUnauthorized, rr.Code)

			// Verify response is valid JSON with error message
			var errResp ErrorResponse
			err := json.Unmarshal(rr.Body.Bytes(), &errResp)
			require.NoError(t, err)
			assert.Equal(t, "missing or empty principal", errResp.Error)
		})
	}
}

// TestRequirePrincipalMiddleware_PrincipalTooLong tests that principals exceeding max length return 400.
func TestRequirePrincipalMiddleware_PrincipalTooLong(t *testing.T) {
	tests := []struct {
		name               string
		principalLength    int
		expectedStatusCode int
	}{
		{
			name:               "201 characters",
			principalLength:    201,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "500 characters",
			principalLength:    500,
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:               "1000 characters",
			principalLength:    1000,
			expectedStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig("X-Remote-User")

			handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			// Create principal of specified length
			longPrincipal := strings.Repeat("a", tt.principalLength)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-Remote-User", longPrincipal)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Should return 400 Bad Request
			assert.Equal(t, tt.expectedStatusCode, rr.Code)

			// Verify response is valid JSON with error message
			var errResp ErrorResponse
			err := json.Unmarshal(rr.Body.Bytes(), &errResp)
			require.NoError(t, err)
			assert.Contains(t, errResp.Error, "exceeds maximum length")
			assert.Contains(t, errResp.Error, "200")
		})
	}
}

// TestRequirePrincipalMiddleware_HandlerNotCalled tests that handler is not called when principal is rejected.
func TestRequirePrincipalMiddleware_HandlerNotCalled(t *testing.T) {
	logger := createTestLogger()
	authConfig := testAuthConfig("X-Remote-User")

	handlerCalled := false
	handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Don't set header - should trigger 401

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.False(t, handlerCalled, "handler should not be called when principal is missing")
}

// TestOptionalPrincipalMiddleware_ValidPrincipal tests that valid principals are extracted in optional mode.
func TestOptionalPrincipalMiddleware_ValidPrincipal(t *testing.T) {
	tests := []struct {
		name                string
		headerName          string
		headerValue         string
		expectedPrincipal   string
		shouldHavePrincipal bool
	}{
		{
			name:                "simple username",
			headerName:          "X-Remote-User",
			headerValue:         "alice",
			expectedPrincipal:   "alice",
			shouldHavePrincipal: true,
		},
		{
			name:                "with whitespace trimming",
			headerName:          "X-Remote-User",
			headerValue:         "  bob  ",
			expectedPrincipal:   "bob",
			shouldHavePrincipal: true,
		},
		{
			name:                "unicode preserved",
			headerName:          "X-Remote-User",
			headerValue:         "用户",
			expectedPrincipal:   "用户",
			shouldHavePrincipal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig(tt.headerName)

			var capturedPrincipal string
			var capturedOk bool

			handler := OptionalPrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPrincipal, capturedOk = principal.FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(tt.headerName, tt.headerValue)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.True(t, capturedOk)
			assert.Equal(t, tt.expectedPrincipal, capturedPrincipal)
		})
	}
}

// TestOptionalPrincipalMiddleware_MissingPrincipal tests that missing principals don't reject requests.
func TestOptionalPrincipalMiddleware_MissingPrincipal(t *testing.T) {
	tests := []struct {
		name       string
		headerName string
		headerSet  bool
		headerVal  string
	}{
		{
			name:       "header not set",
			headerName: "X-Remote-User",
			headerSet:  false,
			headerVal:  "",
		},
		{
			name:       "header empty",
			headerName: "X-Remote-User",
			headerSet:  true,
			headerVal:  "",
		},
		{
			name:       "header whitespace only",
			headerName: "X-Remote-User",
			headerSet:  true,
			headerVal:  "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig(tt.headerName)

			handlerCalled := false
			var capturedPrincipal string
			var capturedOk bool

			handler := OptionalPrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				capturedPrincipal, capturedOk = principal.FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.headerSet {
				req.Header.Set(tt.headerName, tt.headerVal)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Should return 200 OK and call handler
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.True(t, handlerCalled, "handler should be called even without principal")
			assert.False(t, capturedOk, "principal should not be in context")
			assert.Empty(t, capturedPrincipal)
		})
	}
}

// TestOptionalPrincipalMiddleware_PrincipalTooLong tests that long principals are ignored in optional mode.
func TestOptionalPrincipalMiddleware_PrincipalTooLong(t *testing.T) {
	logger := createTestLogger()
	authConfig := testAuthConfig("X-Remote-User")

	handlerCalled := false
	var capturedPrincipal string
	var capturedOk bool

	handler := OptionalPrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		capturedPrincipal, capturedOk = principal.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Create principal exceeding max length
	longPrincipal := strings.Repeat("a", 201)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", longPrincipal)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should return 200 OK and call handler (optional means no rejection)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, handlerCalled)
	assert.False(t, capturedOk, "principal should not be in context (too long)")
	assert.Empty(t, capturedPrincipal)
}

// TestOptionalPrincipalMiddleware_NoRejection tests that optional middleware never rejects requests.
func TestOptionalPrincipalMiddleware_NoRejection(t *testing.T) {
	testCases := []struct {
		name      string
		condition func(req *http.Request)
	}{
		{
			name: "no header",
			condition: func(req *http.Request) {
				// Don't set any header
			},
		},
		{
			name: "empty header",
			condition: func(req *http.Request) {
				req.Header.Set("X-Remote-User", "")
			},
		},
		{
			name: "very long principal",
			condition: func(req *http.Request) {
				req.Header.Set("X-Remote-User", strings.Repeat("x", 1000))
			},
		},
		{
			name: "invalid utf-8",
			condition: func(req *http.Request) {
				req.Header.Set("X-Remote-User", "invalid\xFF\xFE")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logger := createTestLogger()
			authConfig := testAuthConfig("X-Remote-User")

			handler := OptionalPrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			tc.condition(req)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Should always return 200 OK
			assert.Equal(t, http.StatusOK, rr.Code, "optional middleware should never reject requests")
		})
	}
}

// TestWriteErrorJSON tests the error response formatting.
func TestWriteErrorJSON(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		errorMessage   string
		expectedStatus int
	}{
		{
			name:           "unauthorized",
			statusCode:     http.StatusUnauthorized,
			errorMessage:   "missing or empty principal",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "bad request",
			statusCode:     http.StatusBadRequest,
			errorMessage:   "principal exceeds maximum length of 200 characters",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeErrorJSON(rr, tt.statusCode, tt.errorMessage)

			// Verify status code
			assert.Equal(t, tt.expectedStatus, rr.Code)

			// Verify content type
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			// Verify JSON response
			var resp ErrorResponse
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			require.NoError(t, err)
			assert.Equal(t, tt.errorMessage, resp.Error)
		})
	}
}

// --- JWT Backward Compatibility Tests (Phase 4: US5) ---

// mockJWTAuthenticator implements domjwtauth.JWTAuthenticator for testing.
type mockJWTAuthenticator struct {
	result *domjwtauth.AuthResult
	err    error
}

func (m *mockJWTAuthenticator) Authenticate(_ context.Context, _ string) (*domjwtauth.AuthResult, error) {
	return m.result, m.err
}

// TestRequirePrincipalMiddleware_PlainHeaderOnlyConfig tests that plain-header-only
// config works unchanged when no JWT block is configured (US5 backward compatibility).
func TestRequirePrincipalMiddleware_PlainHeaderOnlyConfig(t *testing.T) {
	logger := createTestLogger()
	// No JWT config — plain-header only
	authConfig := testAuthConfig("X-Remote-User")

	var capturedPrincipal string
	handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal, _ = principal.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "alice@example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "alice@example.com", capturedPrincipal)
}

// TestRequirePrincipalMiddleware_JWTPrefersOverPlainHeader tests that JWT is preferred
// over plain header when both are configured and JWT header is present.
func TestRequirePrincipalMiddleware_JWTPrefersOverPlainHeader(t *testing.T) {
	logger := createTestLogger()
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
		JWT: &ports.JWTConfig{
			HeaderName: "Authorization",
		},
	}

	mockAuth := &mockJWTAuthenticator{
		result: &domjwtauth.AuthResult{
			Principal: "jwt-user@example.com",
			Claims:    map[string]interface{}{"sub": "jwt-user@example.com"},
		},
	}

	var capturedPrincipal string
	handler := RequirePrincipalMiddleware(authConfig, mockAuth, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal, _ = principal.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")
	req.Header.Set("X-Remote-User", "header-user@example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "jwt-user@example.com", capturedPrincipal, "JWT should be preferred over plain header")
}

// TestRequirePrincipalMiddleware_RejectsWhenJWTConfiguredAndJWTHeaderAbsent tests that middleware
// rejects with 401 when JWT is configured but JWT header is absent (fail-closed, no fallback).
func TestRequirePrincipalMiddleware_RejectsWhenJWTConfiguredAndJWTHeaderAbsent(t *testing.T) {
	logger := createTestLogger()
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
		JWT: &ports.JWTConfig{
			HeaderName: "Authorization",
		},
	}

	mockAuth := &mockJWTAuthenticator{
		result: &domjwtauth.AuthResult{
			Principal: "jwt-user@example.com",
			Claims:    map[string]interface{}{"sub": "jwt-user@example.com"},
		},
	}

	var capturedPrincipal string
	handler := RequirePrincipalMiddleware(authConfig, mockAuth, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPrincipal, _ = principal.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	// Only set plain header, no Authorization/JWT header
	req.Header.Set("X-Remote-User", "header-user@example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "should reject with 401 when JWT is configured but JWT header is absent")
	assert.Equal(t, "", capturedPrincipal, "handler should not set a principal when JWT is absent")
}

// TestRequirePrincipalMiddleware_PanicsWhenJWTConfiguredButAuthenticatorNil ensures the middleware
// factory panics at startup when JWT is configured but no JWTAuthenticator is injected.
func TestRequirePrincipalMiddleware_PanicsWhenJWTConfiguredButAuthenticatorNil(t *testing.T) {
	logger := createTestLogger()
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
		JWT: &ports.JWTConfig{
			HeaderName: "Authorization",
		},
	}

	require.Panics(t, func() {
		_ = RequirePrincipalMiddleware(authConfig, nil, logger)
	}, "middleware factory should panic when JWT is configured but JWTAuthenticator is nil")
}

func TestOptionalPrincipalMiddleware_PanicsWhenJWTConfiguredButAuthenticatorNil(t *testing.T) {
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
		JWT:     &ports.JWTConfig{HeaderName: "X-Userinfo"},
	}
	require.Panics(t, func() {
		_ = OptionalPrincipalMiddleware(authConfig, nil, createTestLogger())
	}, "configured JWT authentication must not fall back to the plain principal header")
}

// TestRequirePrincipalMiddleware_RejectInvalidJWTNoFallback tests fail-closed behavior:
// when JWT header is present but invalid, reject with 401 even when plain header is present (FR-013).
func TestRequirePrincipalMiddleware_RejectInvalidJWTNoFallback(t *testing.T) {
	logger := createTestLogger()
	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
		JWT: &ports.JWTConfig{
			HeaderName: "Authorization",
		},
	}

	mockAuth := &mockJWTAuthenticator{
		err: domjwtauth.ErrInvalidSignature,
	}

	handlerCalled := false
	handler := RequirePrincipalMiddleware(authConfig, mockAuth, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	req.Header.Set("X-Remote-User", "fallback-user@example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code, "should reject with 401 when JWT is invalid")
	assert.False(t, handlerCalled, "handler should not be called when JWT is invalid")

	var errResp ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	require.NoError(t, err)
	assert.Contains(t, errResp.Error, "signature")
}

// TestRequirePrincipalMiddleware_JWTSetsProfile tests that JWT auth sets PrincipalProfile in context.
func TestRequirePrincipalMiddleware_JWTSetsProfile(t *testing.T) {
	logger := createTestLogger()
	email := "alice@corp.com"
	pictureURL := "https://cdn.example.com/alice.jpg"
	displayName := "Alice Smith"

	authConfig := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
		JWT: &ports.JWTConfig{
			HeaderName: "Authorization",
		},
	}

	mockAuth := &mockJWTAuthenticator{
		result: &domjwtauth.AuthResult{
			Principal:   "alice@example.com",
			DisplayName: &displayName,
			Email:       &email,
			PictureURL:  &pictureURL,
			Claims:      map[string]interface{}{"sub": "alice@example.com"},
		},
	}

	var capturedProfile principal.PrincipalProfile
	var profileOk bool
	handler := RequirePrincipalMiddleware(authConfig, mockAuth, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProfile, profileOk = principal.ProfileFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer some.jwt.token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, profileOk, "profile should be in context")
	assert.Equal(t, "alice@example.com", capturedProfile.Principal())
	assert.Equal(t, "Alice Smith", capturedProfile.DisplayName())
	assert.NotNil(t, capturedProfile.Email())
	assert.Equal(t, "alice@corp.com", *capturedProfile.Email())
	assert.NotNil(t, capturedProfile.PictureURL())
	assert.Equal(t, "https://cdn.example.com/alice.jpg", *capturedProfile.PictureURL())
}

// TestRequirePrincipalMiddleware_PlainHeaderSetsBasicProfile tests that plain header mode
// sets a basic PrincipalProfile with display name = principal.
func TestRequirePrincipalMiddleware_PlainHeaderSetsBasicProfile(t *testing.T) {
	logger := createTestLogger()
	authConfig := testAuthConfig("X-Remote-User")

	var capturedProfile principal.PrincipalProfile
	var profileOk bool
	handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProfile, profileOk = principal.ProfileFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "alice@example.com")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.True(t, profileOk, "profile should be in context for plain header")
	assert.Equal(t, "alice@example.com", capturedProfile.Principal())
	assert.Equal(t, "alice@example.com", capturedProfile.DisplayName(), "display name should default to principal")
	assert.Nil(t, capturedProfile.Email(), "email should be nil for plain header")
	assert.Nil(t, capturedProfile.PictureURL(), "picture URL should be nil for plain header")
}

// BenchmarkRequirePrincipalMiddleware benchmarks the performance of RequirePrincipalMiddleware.
func BenchmarkRequirePrincipalMiddleware(b *testing.B) {
	logger := createTestLogger()
	authConfig := testAuthConfig("X-Remote-User")

	handler := RequirePrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "alice@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// BenchmarkOptionalPrincipalMiddleware benchmarks the performance of OptionalPrincipalMiddleware.
func BenchmarkOptionalPrincipalMiddleware(b *testing.B) {
	logger := createTestLogger()
	authConfig := testAuthConfig("X-Remote-User")

	handler := OptionalPrincipalMiddleware(authConfig, nil, logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Remote-User", "alice@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}
