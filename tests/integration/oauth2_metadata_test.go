package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/enduser"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2MetadataEndpoint_ReturnsValidJSON tests metadata endpoint returns valid JSON
func TestOAuth2MetadataEndpoint_ReturnsValidJSON(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Verify valid JSON response
	var metadata ports.MetadataResponse
	body, _ := io.ReadAll(w.Body)
	err := json.Unmarshal(body, &metadata)
	require.NoError(t, err, "response should be valid JSON")
}

// TestOAuth2MetadataEndpoint_RFC8414Schema tests metadata conforms to RFC 8414
func TestOAuth2MetadataEndpoint_RFC8414Schema(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code", "refresh_token"},
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var metadata ports.MetadataResponse
	body, _ := io.ReadAll(w.Body)
	_ = json.Unmarshal(body, &metadata)

	// RFC 8414 required fields
	assert.NotEmpty(t, metadata.Issuer, "issuer required")
	assert.NotEmpty(t, metadata.AuthorizationEndpoint, "authorization_endpoint required")
	assert.NotEmpty(t, metadata.TokenEndpoint, "token_endpoint required")
	assert.NotEmpty(t, metadata.ResponseTypesSupported, "response_types_supported required")
	assert.NotEmpty(t, metadata.GrantTypesSupported, "grant_types_supported required")

	// Verify values are correct
	assert.Equal(t, "https://broker.example.com", metadata.Issuer)
	assert.Equal(t, "https://broker.example.com/oauth2/authorize", metadata.AuthorizationEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/token", metadata.TokenEndpoint)
}

// TestOAuth2MetadataEndpoint_HTTPStatus tests HTTP 200 status for successful request
func TestOAuth2MetadataEndpoint_HTTPStatus(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "metadata endpoint should return 200 OK")
}

// TestOAuth2MetadataEndpoint_ContentType tests Content-Type header
func TestOAuth2MetadataEndpoint_ContentType(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	assert.Equal(t, "application/json", contentType, "Content-Type must be application/json")
}

// TestOAuth2MetadataEndpoint_IsPublic tests metadata endpoint is public (no authentication)
func TestOAuth2MetadataEndpoint_IsPublic(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	// Request without any authentication headers
	req := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Should succeed without authentication
	assert.Equal(t, http.StatusOK, w.Code, "metadata endpoint should be public (no auth required)")
}

// TestOAuth2MetadataEndpoint_MultipleRequests tests consistent responses across multiple requests
func TestOAuth2MetadataEndpoint_MultipleRequests(t *testing.T) {
	agentRepo := newInMemoryAgentRepo()
	grantRepo := newInMemoryGrantRepo()

	svc := oauth2.NewAuthorizationService(grantRepo, memory.NewInMemoryUserSessionRepository(), oauth2.NewAgentClientResolver(agentRepo, nil), &oauth2.OAuth2Config{
		ModeStrategy:              oauth2.NewProxyModeStrategy(),
		UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
		UpstreamTokenEndpoint:     "https://auth.example.com/token",
		PublicURL:                 "https://broker.example.com",
		SupportedResponseTypes:    []string{"code"},
		SupportedGrantTypes:       []string{"authorization_code"},
	}, nil, newIntegrationSessionTokenSvc(), testAuthorizationClock{now: time.Now()})

	handler := &enduser.OAuth2MetadataHandler{
		Service: svc,
	}

	var metadata1, metadata2 ports.MetadataResponse

	// First request
	req1 := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	body1, _ := io.ReadAll(w1.Body)
	_ = json.Unmarshal(body1, &metadata1)

	// Second request
	req2 := httptest.NewRequest("GET", "https://broker.example.com/.well-known/oauth-authorization-server", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	body2, _ := io.ReadAll(w2.Body)
	_ = json.Unmarshal(body2, &metadata2)

	// Verify consistent responses
	assert.Equal(t, metadata1.Issuer, metadata2.Issuer)
	assert.Equal(t, metadata1.AuthorizationEndpoint, metadata2.AuthorizationEndpoint)
	assert.Equal(t, metadata1.TokenEndpoint, metadata2.TokenEndpoint)
}
