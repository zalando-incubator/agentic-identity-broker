package enduser

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2MetadataHandler_ServeHTTP_SuccessfulMetadata tests successful metadata generation
func TestOAuth2MetadataHandler_ServeHTTP_SuccessfulMetadata(t *testing.T) {
	svc := oauth2.NewAuthorizationService(consent.NewService(nil, nil, newMockGrantRepo(), nil, nil, nil, ledgerfixture.NewRecorder()), &noopSessionRepository{}, oauth2.NewAgentClientResolver(newMockAgentRepo(), nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}, nil, newTestSessionTokenSvc(), ledgerfixture.NewRecorder())

	handler := &OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var metadata ports.MetadataResponse
	body, _ := io.ReadAll(w.Body)
	err := json.Unmarshal(body, &metadata)
	require.NoError(t, err)

	assert.Equal(t, "https://broker.example.com", metadata.Issuer)
	assert.Equal(t, "https://broker.example.com/oauth2/authorize", metadata.AuthorizationEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/token", metadata.TokenEndpoint)
	assert.NotEmpty(t, metadata.ResponseTypesSupported)
	assert.NotEmpty(t, metadata.GrantTypesSupported)
}

// TestOAuth2MetadataHandler_ServeHTTP_JSONEncoding tests proper JSON encoding
func TestOAuth2MetadataHandler_ServeHTTP_JSONEncoding(t *testing.T) {
	svc := oauth2.NewAuthorizationService(consent.NewService(nil, nil, newMockGrantRepo(), nil, nil, nil, ledgerfixture.NewRecorder()), &noopSessionRepository{}, oauth2.NewAgentClientResolver(newMockAgentRepo(), nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code", "refresh_token"},
	}, nil, newTestSessionTokenSvc(), ledgerfixture.NewRecorder())

	handler := &OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body, _ := io.ReadAll(w.Body)

	// Verify valid JSON
	var result map[string]interface{}
	err := json.Unmarshal(body, &result)
	require.NoError(t, err)

	// Verify key fields present
	assert.NotNil(t, result["issuer"])
	assert.NotNil(t, result["authorization_endpoint"])
	assert.NotNil(t, result["token_endpoint"])
	assert.NotNil(t, result["response_types_supported"])
	assert.NotNil(t, result["grant_types_supported"])
}

// TestOAuth2MetadataHandler_ServeHTTP_CacheHeaders tests appropriate cache headers
func TestOAuth2MetadataHandler_ServeHTTP_CacheHeaders(t *testing.T) {
	svc := oauth2.NewAuthorizationService(consent.NewService(nil, nil, newMockGrantRepo(), nil, nil, nil, ledgerfixture.NewRecorder()), &noopSessionRepository{}, oauth2.NewAgentClientResolver(newMockAgentRepo(), nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
	}, nil, newTestSessionTokenSvc(), ledgerfixture.NewRecorder())

	handler := &OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Metadata endpoints can be cached since configuration changes rarely
	assert.NotEmpty(t, w.Header().Get("Cache-Control"))
}

// TestOAuth2MetadataHandler_ServeHTTP_AllowGETOnly tests only GET method allowed
func TestOAuth2MetadataHandler_ServeHTTP_AllowGETOnly(t *testing.T) {
	svc := oauth2.NewAuthorizationService(consent.NewService(nil, nil, newMockGrantRepo(), nil, nil, nil, ledgerfixture.NewRecorder()), &noopSessionRepository{}, oauth2.NewAgentClientResolver(newMockAgentRepo(), nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		PublicURL:                 "https://broker.example.com",
	}, nil, newTestSessionTokenSvc(), ledgerfixture.NewRecorder())

	handler := &OAuth2MetadataHandler{
		Service: svc,
	}

	methods := []string{"POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "https://broker.example.com/.well-known/oauth-authorization-server", nil)
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		})
	}
}

// TestOAuth2MetadataHandler_ServeHTTP_PublicEndpoint tests endpoint is public (no auth required)
func TestOAuth2MetadataHandler_ServeHTTP_PublicEndpoint(t *testing.T) {
	svc := oauth2.NewAuthorizationService(consent.NewService(nil, nil, newMockGrantRepo(), nil, nil, nil, ledgerfixture.NewRecorder()), &noopSessionRepository{}, oauth2.NewAgentClientResolver(newMockAgentRepo(), nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
	}, nil, newTestSessionTokenSvc(), ledgerfixture.NewRecorder())

	handler := &OAuth2MetadataHandler{
		Service: svc,
	}

	// Request without X-Remote-User header (no auth)
	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should succeed even without authentication
	assert.Equal(t, http.StatusOK, w.Code)
}
