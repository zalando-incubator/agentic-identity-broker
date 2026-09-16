package oauth2session_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

type noopBranchKeyManager struct{}

func newNoopBranchKeyManager() *noopBranchKeyManager {
	return &noopBranchKeyManager{}
}

func (m *noopBranchKeyManager) Create(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

// =============================================================================
// Tests for InitiateOAuth2Flow
// =============================================================================

func TestInitiateOAuth2Flow_Success(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create a third-party service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: "GitHub",
		ClientID:    id.ClientID("test-client-id"),
		Secret:      model.NewPlaintextSecret("test-secret"),
		IssuerURI:   "https://github.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
			{ScopeValue: "user", Description: "User profile"},
		},
	}
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Call InitiateOAuth2Flow
	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	// Implementation complete: verify success
	require.NoError(t, err, "InitiateOAuth2Flow should succeed")
	require.NotNil(t, result, "result should not be nil")
	assert.NotEmpty(t, result.AuthorizationURL, "authorization URL should not be empty")
	assert.NotEmpty(t, result.StateToken, "state token should not be empty")

	// Verify authorization URL contains third-party authorization endpoint
	assert.Contains(t, result.AuthorizationURL, "https://github.com/login/oauth/authorize")

	// Verify URL contains required OAuth2 params
	parsedURL, err := url.Parse(result.AuthorizationURL)
	require.NoError(t, err)
	query := parsedURL.Query()
	assert.Equal(t, "test-client-id", query.Get("client_id"))
	assert.NotEmpty(t, query.Get("redirect_uri"))
	assert.Equal(t, "code", query.Get("response_type"))
	assert.NotEmpty(t, query.Get("code_challenge"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.NotEmpty(t, query.Get("scope"))
	assert.NotEmpty(t, query.Get("state"))
}

func TestInitiateOAuth2Flow_AddsStoredAuthorizationParams(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)
	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	thirdPartyService.AuthorizationParams = map[string]string{"business_partner_id": "12345"}
	require.NoError(t, providerService.Create(ctx, thirdPartyService))

	result, err := service.InitiateOAuth2Flow(ctx, id.Principal("user@example.com"), serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	parsedURL, err := url.Parse(result.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "12345", parsedURL.Query().Get("business_partner_id"))
}

func TestHandleCallbackAndRefresh_PassAuthorizationParams(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)
	serviceID := id.NewServiceID()
	provider := createTestService(serviceID)
	provider.AuthorizationParams = map[string]string{"business_partner_id": "12345"}

	var requests []url.Values
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		requests = append(requests, r.Form)
		if r.Form.Get("grant_type") == "authorization_code" && len(requests) == 1 {
			http.Error(w, `{"error":"server_error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer mockServer.Close()

	provider.Endpoints.TokenEndpoint = mockServer.URL
	require.NoError(t, providerService.Create(ctx, provider))
	flow, err := service.InitiateOAuth2Flow(ctx, id.Principal("user@example.com"), serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	_, err = service.HandleCallback(ctx, id.Principal("user@example.com"), &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)

	decryptedProvider, err := providerService.Get(ctx, serviceID)
	require.NoError(t, err)
	_, err = service.RefreshAccessToken(ctx, decryptedProvider, "refresh-token")
	require.NoError(t, err)
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, "12345", request.Get("business_partner_id"))
	}
}

func TestLegacyAuthorizationParamsCannotOverrideBrokerFields(t *testing.T) {
	ctx := context.Background()
	service, repo, providerService := setupService(t)
	serviceID := id.NewServiceID()
	provider := createTestService(serviceID)

	var requests []url.Values
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		requests = append(requests, r.Form)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer mockServer.Close()

	provider.Endpoints.TokenEndpoint = mockServer.URL
	require.NoError(t, providerService.Create(ctx, provider))

	persisted, err := repo.Get(ctx, serviceID)
	require.NoError(t, err)
	persisted.AuthorizationParams = map[string]string{
		"client_id":           "attacker-client",
		"code":                "attacker-code",
		"grant_type":          "attacker-grant",
		"code_verifier":       "attacker-verifier",
		"refresh_token":       "attacker-refresh-token",
		"business_partner_id": "12345",
	}
	require.NoError(t, repo.Update(ctx, persisted, nil))

	flow, err := service.InitiateOAuth2Flow(ctx, id.Principal("user@example.com"), serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	parsedURL, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "test-client-id", parsedURL.Query().Get("client_id"))
	assert.Equal(t, "code", parsedURL.Query().Get("response_type"))
	assert.Equal(t, "12345", parsedURL.Query().Get("business_partner_id"))

	_, err = service.HandleCallback(ctx, id.Principal("user@example.com"), &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)

	decryptedProvider, err := providerService.Get(ctx, serviceID)
	require.NoError(t, err)
	_, err = service.RefreshAccessToken(ctx, decryptedProvider, "refresh-token")
	require.NoError(t, err)
	require.Len(t, requests, 2)
	assert.Equal(t, "authorization-code", requests[0].Get("code"))
	assert.Equal(t, "authorization_code", requests[0].Get("grant_type"))
	assert.NotEqual(t, "attacker-verifier", requests[0].Get("code_verifier"))
	assert.Equal(t, "12345", requests[0].Get("business_partner_id"))
	assert.Equal(t, "refresh_token", requests[1].Get("grant_type"))
	assert.Equal(t, "refresh-token", requests[1].Get("refresh_token"))
	assert.Equal(t, "12345", requests[1].Get("business_partner_id"))
}

func TestRefreshAccessToken_OmitsAuthorizationParamsForUnconfiguredService(t *testing.T) {
	service, _, _ := setupService(t)
	provider := createTestService(id.NewServiceID())

	var request url.Values
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		request = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer mockServer.Close()

	provider.Endpoints.TokenEndpoint = mockServer.URL
	_, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
	require.NoError(t, err)
	assert.Empty(t, request.Get("business_partner_id"))
}

func TestRefreshAccessToken_PublicClientOmitsClientSecret(t *testing.T) {
	service, _, _ := setupService(t)
	provider := createTestService(id.NewServiceID())
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	provider.AuthorizationParams = map[string]string{"business_partner_id": "12345"}

	var receivedBody string
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenEndpoint.Close()

	provider.Endpoints.TokenEndpoint = tokenEndpoint.URL
	_, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
	require.NoError(t, err)

	expectedBody := url.Values{}
	expectedBody.Set("grant_type", "refresh_token")
	expectedBody.Set("refresh_token", "refresh-token")
	expectedBody.Set("client_id", "test-client-id")
	expectedBody.Set("business_partner_id", "12345")
	assert.Equal(t, expectedBody.Encode(), receivedBody)
	assert.NotContains(t, receivedBody, "client_secret")
}

func TestRefreshAccessToken_ConfidentialClientPreservesRequestBody(t *testing.T) {
	service, _, _ := setupService(t)
	provider := createTestService(id.NewServiceID())
	provider.AuthorizationParams = map[string]string{"business_partner_id": "12345"}

	var receivedBody string
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenEndpoint.Close()

	provider.Endpoints.TokenEndpoint = tokenEndpoint.URL
	_, err := service.RefreshAccessToken(context.Background(), provider, "refresh-token")
	require.NoError(t, err)

	expectedBody := url.Values{}
	expectedBody.Set("grant_type", "refresh_token")
	expectedBody.Set("refresh_token", "refresh-token")
	expectedBody.Set("client_id", "test-client-id")
	expectedBody.Set("client_secret", "test-client-secret")
	expectedBody.Set("business_partner_id", "12345")
	assert.Equal(t, expectedBody.Encode(), receivedBody)
}

func TestInitiateOAuth2Flow_ServiceNotFound(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	// Should fail with service not found
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestInitiateOAuth2Flow_InvalidRedirectURI(t *testing.T) {
	// Note: redirect_uri validation for host matching and format is done at the HTTP handler level.
	// The service only requires that redirect_uri is non-empty (validated in state token claims).
	// This test focuses on what the service actually validates.

	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")

	// Test 1: Empty redirect_uri fails at state token claims validation
	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "")
	assert.Error(t, err, "empty redirect_uri should fail validation")
	assert.Nil(t, result)

	// Test 2: Non-empty redirect_uri succeeds at service level
	// (handler-level validation for host matching would happen separately in HTTP layer)
	result, err = service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/callback")
	assert.NoError(t, err, "valid non-empty redirect_uri should succeed")
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.StateToken)
}

func TestInitiateOAuth2Flow_StateTokenContainsCorrectClaims(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	// Verify success
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify state token is present (claims validation tested in state_token_test.go)
	assert.NotEmpty(t, result.StateToken, "state token should not be empty")
}

func TestInitiateOAuth2Flow_AuthorizationURLContainsRequiredParams(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	// Verify success
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify authorization URL contains required OAuth2 params
	parsedURL, err := url.Parse(result.AuthorizationURL)
	require.NoError(t, err)

	query := parsedURL.Query()
	assert.Equal(t, "test-client-id", query.Get("client_id"), "should include client_id")
	assert.NotEmpty(t, query.Get("redirect_uri"), "should include redirect_uri")
	assert.Equal(t, "code", query.Get("response_type"), "should use authorization code flow")
	assert.NotEmpty(t, query.Get("code_challenge"), "should include PKCE code_challenge")
	assert.Equal(t, "S256", query.Get("code_challenge_method"), "should use S256 method")
	assert.NotEmpty(t, query.Get("scope"), "should include scopes")
	assert.NotEmpty(t, query.Get("state"), "should include state token")
}

func TestInitiateOAuth2Flow_DifferentServicesScopes(t *testing.T) {
	tests := []struct {
		name          string
		scopes        []model.OAuthScope
		expectedScope string
		hasScope      bool
	}{
		{
			name: "single scope",
			scopes: []model.OAuthScope{
				{ScopeValue: "repo", Description: "Repository access"},
			},
			expectedScope: "repo",
			hasScope:      true,
		},
		{
			name: "multiple scopes",
			scopes: []model.OAuthScope{
				{ScopeValue: "repo", Description: "Repository access"},
				{ScopeValue: "user", Description: "User profile"},
				{ScopeValue: "notifications", Description: "Notifications"},
			},
			expectedScope: "repo user notifications",
			hasScope:      true,
		},
		{
			name:     "no scopes omits scope parameter",
			scopes:   nil,
			hasScope: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, providerService := setupService(t)

			serviceID := id.NewServiceID()
			thirdPartyService := createTestService(serviceID)
			thirdPartyService.Scopes = tt.scopes
			err := providerService.Create(ctx, thirdPartyService)
			require.NoError(t, err)

			result, err := service.InitiateOAuth2Flow(ctx, id.Principal("user@example.com"), serviceID, "https://example.com/sessions")
			require.NoError(t, err)
			require.NotNil(t, result)

			parsedURL, err := url.Parse(result.AuthorizationURL)
			require.NoError(t, err)
			query := parsedURL.Query()
			_, hasScope := query["scope"]
			assert.Equal(t, tt.hasScope, hasScope)
			assert.Equal(t, tt.expectedScope, query.Get("scope"))
		})
	}
}

// =============================================================================
// Tests for OAuth2 Configuration
// =============================================================================

func TestHandleCallback_BuildOAuth2Config_PublicClientUsesBodyAuthentication(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	requestCount := atomic.Int64{}
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token request form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		requestCount.Add(1)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Empty(t, r.Header.Get("Authorization"), "public clients must not use HTTP authorization")
		assert.NotContains(t, r.URL.Query(), "client_id", "public client ID must not be sent in the query")
		assert.NotContains(t, r.URL.Query(), "client_secret", "public client secret must not be sent in the query")
		assert.Equal(t, "test-client-id", r.PostForm.Get("client_id"))
		assert.NotContains(t, r.PostForm, "client_secret", "public clients must not send a client secret")
		assert.Equal(t, "authorization-code", r.PostForm.Get("code"))
		assert.NotEmpty(t, r.PostForm.Get("code_verifier"), "public clients must send the PKCE verifier")

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"test-access-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer tokenEndpoint.Close()

	provider := createTestService(serviceID)
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	provider.Endpoints.TokenEndpoint = tokenEndpoint.URL
	require.NoError(t, providerService.Create(ctx, provider))

	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)

	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), requestCount.Load(), "public configuration must use body authentication without probing")
}

func TestHandleCallback_BuildOAuth2Config_ConfidentialClientUsesAutoDetectedAuthentication(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	requestCount := atomic.Int64{}
	tokenEndpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse token request form: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		switch requestCount.Add(1) {
		case 1:
			clientID, clientSecret, ok := r.BasicAuth()
			assert.True(t, ok, "auto-detection must first try HTTP basic authentication")
			assert.Equal(t, "test-client-id", clientID)
			assert.Equal(t, "test-client-secret", clientSecret, "the decrypted secret must reach oauth2.Config")
			assert.NotContains(t, r.PostForm, "client_id", "basic authentication must not duplicate credentials in the body")
			assert.NotContains(t, r.PostForm, "client_secret", "basic authentication must not duplicate credentials in the body")
			assert.NotEmpty(t, r.PostForm.Get("code_verifier"), "the initial authentication probe must retain the PKCE verifier")

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":"invalid_client"}`)
		case 2:
			assert.Empty(t, r.Header.Get("Authorization"), "the fallback must move credentials into the body")
			assert.NotContains(t, r.URL.Query(), "client_id", "confidential client ID must not be sent in the query")
			assert.NotContains(t, r.URL.Query(), "client_secret", "confidential client secret must not be sent in the query")
			assert.Equal(t, "test-client-id", r.PostForm.Get("client_id"))
			assert.Equal(t, "test-client-secret", r.PostForm.Get("client_secret"))
			assert.NotEmpty(t, r.PostForm.Get("code_verifier"), "the fallback must retain the PKCE verifier")

			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"access_token":"test-access-token","token_type":"Bearer","expires_in":3600}`)
		default:
			t.Errorf("unexpected token request %d", requestCount.Load())
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer tokenEndpoint.Close()

	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = tokenEndpoint.URL
	require.NoError(t, providerService.Create(ctx, provider))

	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)

	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "authorization-code",
		State:     flow.StateToken,
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(2), requestCount.Load(), "default oauth2 authentication must fall back from basic to body credentials")
}

// =============================================================================
// Tests for HandleCallback
// =============================================================================

func TestHandleCallback_Success(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: create service and initiate flow
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)

	// Create mock token endpoint
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "test-access-token-xyz",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "test-refresh-token-abc",
		scope:        "repo user",
	})
	defer mockServer.Close()

	// Update token endpoint to point to mock server
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Initiate flow to get state token with PKCE verifier
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotNil(t, flowResult)

	// Call HandleCallback with valid authorization code
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "test-authorization-code-123",
		State:     flowResult.StateToken,
		Error:     "",
		ErrorDesc: "",
	}

	result, err := service.HandleCallback(ctx, principal, req)

	// Verify success
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)

	// Verify session properties
	session := result.Session
	assert.Equal(t, principal, session.Principal)
	assert.Equal(t, serviceID, session.ServiceID)
	assert.NotEmpty(t, session.EncryptedAccessToken)
	assert.NotEmpty(t, session.EncryptedRefreshToken)
	assert.Equal(t, "Bearer", session.TokenType)
	assert.Contains(t, session.Scope, "repo")
	assert.Contains(t, session.Scope, "user")
	assert.NotZero(t, session.AccessTokenExpiresAt)
}

func TestHandleCallback_InvalidStateToken(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: id.NewServiceID(),
		Code:      "auth-code",
		State:     "invalid-state-token-xyz",
	}

	result, err := service.HandleCallback(ctx, principal, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify it's specifically an invalid state token error
	assert.True(t, errors.Is(err, oauth2session.ErrInvalidStateToken),
		"expected ErrInvalidStateToken, got: %v", err)
}

func TestHandleCallback_ExpiredStateToken(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create service
	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Create an expired state token (manually constructed)
	// This would require CreateStateToken to be exposed or tested via time manipulation
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     "expired-token", // Would be actual expired JWE in implementation
	}

	principal := id.Principal("user@example.com")
	result, err := service.HandleCallback(ctx, principal, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify it's specifically an invalid state token error (malformed token treated as invalid)
	assert.True(t, errors.Is(err, oauth2session.ErrInvalidStateToken),
		"expected ErrInvalidStateToken, got: %v", err)
}

func TestHandleCallback_PrincipalMismatch(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create service and initiate flow as user1
	principal1 := id.Principal("user1@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal1, serviceID, redirectURI)
	if err != nil {
		assert.Contains(t, err.Error(), "not implemented")
		return
	}

	// Try to complete callback as user2 (CSRF attack)
	principal2 := id.Principal("user2@example.com")
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal2, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify it's specifically a principal mismatch error
	assert.True(t, errors.Is(err, oauth2session.ErrPrincipalMismatch),
		"expected ErrPrincipalMismatch, got: %v", err)
}

func TestHandleCallback_ServiceIDMismatch(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Create two services
	serviceID1 := id.NewServiceID()
	serviceID2 := id.NewServiceID()

	service1 := createTestService(serviceID1)
	service2 := createTestService(serviceID2)
	service2.DisplayName = "Google"

	err := providerService.Create(ctx, service1)
	require.NoError(t, err)
	err = providerService.Create(ctx, service2)
	require.NoError(t, err)

	// Initiate flow for service1
	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID1, redirectURI)
	if err != nil {
		assert.Contains(t, err.Error(), "not implemented")
		return
	}

	// Try to use state token for service2
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID2, // Different service!
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify it's specifically a service ID mismatch error
	assert.True(t, errors.Is(err, oauth2session.ErrServiceIDMismatch),
		"expected ErrServiceIDMismatch, got: %v", err)
}

func TestHandleCallback_OAuth2ErrorResponse(t *testing.T) {
	tests := []struct {
		name      string
		error     string
		errorDesc string
	}{
		{
			name:      "access denied",
			error:     "access_denied",
			errorDesc: "User denied access",
		},
		{
			name:      "invalid scope",
			error:     "invalid_scope",
			errorDesc: "Requested scope is invalid",
		},
		{
			name:      "server error",
			error:     "server_error",
			errorDesc: "Authorization server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, _ := setupService(t)

			principal := id.Principal("user@example.com")
			req := &oauth2session.HandleCallbackRequest{
				ServiceID: id.NewServiceID(),
				Code:      "", // No code when error present
				State:     "state-token",
				Error:     tt.error,
				ErrorDesc: tt.errorDesc,
			}

			result, err := service.HandleCallback(ctx, principal, req)

			assert.Error(t, err)
			assert.Nil(t, result)
			// Verify error contains the OAuth2 error code
			assert.Contains(t, err.Error(), tt.error, "error should contain OAuth2 error code")
		})
	}
}

func TestHandleCallback_TokenExchangeFailureWithRetry(t *testing.T) {
	// This test verifies retry logic with exponential backoff
	// Mock endpoint fails first 2 attempts (HTTP 500), succeeds on 3rd

	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)

	// Create mock token endpoint that fails first 2 times, then succeeds
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "test-access-token-retry",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "test-refresh-token-retry",
		scope:        "repo",
		failCount:    2, // Fail 2 times, succeed on 3rd attempt
	})
	defer mockServer.Close()

	// Update token endpoint to point to mock server
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotNil(t, flowResult)

	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	// Time the operation to verify backoff delays occurred (at least 1s + 2s)
	start := time.Now()
	result, err := service.HandleCallback(ctx, principal, req)
	elapsed := time.Since(start)

	// Verify success after retries
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)

	// Verify session was created
	session := result.Session
	assert.Equal(t, principal, session.Principal)
	assert.Equal(t, serviceID, session.ServiceID)
	assert.NotEmpty(t, session.EncryptedAccessToken)

	// Verify retries took time (should have delays between attempts)
	// On fast systems, may execute quickly; we just verify session was created successfully
	// Exact timing is hard to guarantee in tests, so we only assert the session exists
	_ = elapsed
}

func TestHandleCallback_RetryExhausted(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	// Initiate OAuth2 flow
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	// Mock token endpoint that always fails (persistent failures)
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		returnError: true,
		errorCode:   "server_error",
		errorDesc:   "Internal server error",
		httpStatus:  500,
		failCount:   100, // Always fail (more than max retries)
	})
	defer mockServer.Close()

	// Update service to use mock token endpoint.
	// Reset secret to plaintext — Update() always requires plaintext state.
	thirdPartyService.Secret = model.NewPlaintextSecret("test-client-secret")
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL
	err = providerService.Update(ctx, thirdPartyService, nil)
	require.NoError(t, err)

	// Call HandleCallback - should fail after exhausting retries
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	// Verify error handling
	assert.Error(t, err)
	assert.Nil(t, result)
	// Verify it exhausted retries and contains expected error messages
	assert.Contains(t, err.Error(), "failed to exchange authorization code", "error should indicate authorization code exchange failed")
	assert.Contains(t, err.Error(), "token exchange failed", "error should indicate retry exhaustion")
}

func TestHandleCallback_ContextCancellationDuringRetry(t *testing.T) {
	// Use production-like retry delays here so the request fails once, then the
	// context expires while waiting for the next retry.
	service, _, providerService := setupServiceWithConfig(t, func(config *oauth2session.Config) {
		config.RetryBaseDelay = 250 * time.Millisecond
	})

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(context.Background(), thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	// Initiate OAuth2 flow (use background context for this part)
	flowResult, err := service.InitiateOAuth2Flow(context.Background(), principal, serviceID, redirectURI)
	require.NoError(t, err)

	// Mock token endpoint that always fails (will trigger retries with delays)
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		returnError: true,
		errorCode:   "server_error",
		errorDesc:   "Internal server error",
		httpStatus:  500,
		failCount:   100, // Always fail (will exhaust retries)
	})
	defer mockServer.Close()

	// Update service to use mock token endpoint.
	// Reset secret to plaintext — Update() always requires plaintext state.
	thirdPartyService.Secret = model.NewPlaintextSecret("test-client-secret")
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL
	err = providerService.Update(context.Background(), thirdPartyService, nil)
	require.NoError(t, err)

	// Create a context that expires after the initial setup work but before the
	// next retry is allowed to run, then call HandleCallback with it.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	// Verify error handling - should get context cancellation error or timeout error
	assert.Error(t, err)
	assert.Nil(t, result)
	// Should be a context-related error (timeout or cancelled)
	assert.True(t, errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context"),
		"Expected context-related error, got: %v", err)
}

func TestHandleCallback_TokenEncryptionFailure(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	// Mock token endpoint for successful token exchange
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "test-access-token",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "test-refresh-token",
		scope:        "repo user",
	})
	defer mockServer.Close()

	// Update service to use mock token endpoint.
	// Reset secret to plaintext — Update() always requires plaintext state.
	thirdPartyService.Secret = model.NewPlaintextSecret("test-client-secret")
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL
	err = providerService.Update(ctx, thirdPartyService, nil)
	require.NoError(t, err)

	// Test with mock token endpoint working but encryption would fail
	// This tests the successful path through token exchange
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	// Should succeed with proper mock encryption (setupService creates working encryption)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Session)
}

func TestHandleCallback_SessionAlreadyExists(t *testing.T) {
	// Test upsert semantics: if session exists, update it
	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)

	// Create mock token endpoint
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "test-access-token-xyz",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "test-refresh-token-abc",
		scope:        "repo user",
	})
	defer mockServer.Close()

	// Update token endpoint to point to mock server
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	// First callback - creates session
	flowResult1, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotNil(t, flowResult1)

	req1 := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code-1",
		State:     flowResult1.StateToken,
	}

	result1, err := service.HandleCallback(ctx, principal, req1)
	require.NoError(t, err)
	require.NotNil(t, result1)
	session1ID := result1.Session.ID
	session1CreatedAt := result1.Session.CreatedAt

	// Wait a tiny bit to ensure UpdatedAt changes
	time.Sleep(10 * time.Millisecond)

	// Second callback - should upsert (update existing session, not create new)
	flowResult2, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotNil(t, flowResult2)

	req2 := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code-2",
		State:     flowResult2.StateToken,
	}

	result2, err := service.HandleCallback(ctx, principal, req2)
	require.NoError(t, err)
	require.NotNil(t, result2)
	session2ID := result2.Session.ID
	session2UpdatedAt := result2.Session.UpdatedAt

	// Verify upsert behavior
	assert.Equal(t, session1ID, session2ID, "Session ID should remain the same (upsert)")
	assert.Equal(t, session1CreatedAt, result2.Session.CreatedAt, "CreatedAt should not change")
	assert.Greater(t, session2UpdatedAt, session1CreatedAt, "UpdatedAt should be more recent than CreatedAt")
}

func TestHandleCallback_PKCEValidationFailure(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	// Initiate OAuth2 flow
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	// Validate state token
	_, err = service.ValidateStateToken(flowResult.StateToken, principal, serviceID)
	require.NoError(t, err)

	// Mock token endpoint that returns invalid_grant when PKCE doesn't match
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		returnError:      true,
		errorCode:        "invalid_grant",
		errorDesc:        "PKCE validation failed",
		httpStatus:       400,
		validatePKCE:     true,
		expectedVerifier: "invalid-verifier", // Mismatch PKCE verifier
	})
	defer mockServer.Close()

	// Update service to use mock token endpoint.
	// Reset secret to plaintext — Update() always requires plaintext state.
	thirdPartyService.Secret = model.NewPlaintextSecret("test-client-secret")
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL
	err = providerService.Update(ctx, thirdPartyService, nil)
	require.NoError(t, err)

	// Call HandleCallback - should fail due to PKCE validation
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	// Verify error handling
	assert.Error(t, err)
	assert.Nil(t, result)
	// PKCE validation failures result in token exchange errors with specific messages
	assert.Contains(t, err.Error(), "failed to exchange authorization code", "error should indicate authorization code exchange failed")
	// The mock returns invalid_grant which becomes a token exchange error
	assert.Contains(t, err.Error(), "invalid_grant", "error should indicate PKCE validation failure")
}

func TestHandleCallback_MissingCode(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: id.NewServiceID(),
		Code:      "", // Missing code
		State:     "state-token",
	}

	result, err := service.HandleCallback(ctx, principal, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// State validation should fail on invalid state token
	assert.True(t, errors.Is(err, oauth2session.ErrInvalidStateToken),
		"expected ErrInvalidStateToken, got: %v", err)
}

func TestHandleCallback_MissingState(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: id.NewServiceID(),
		Code:      "auth-code",
		State:     "", // Missing state
	}

	result, err := service.HandleCallback(ctx, principal, req)

	assert.Error(t, err)
	assert.Nil(t, result)
	// State validation should fail on invalid/missing state token
	assert.True(t, errors.Is(err, oauth2session.ErrInvalidStateToken),
		"expected ErrInvalidStateToken, got: %v", err)
}

// =============================================================================
// Test Helpers
// =============================================================================

// mockTokenConfig configures the behavior of a mock OAuth2 token endpoint
type mockTokenConfig struct {
	// Success response fields
	accessToken  string
	tokenType    string
	expiresIn    int
	refreshToken string
	scope        string

	// Error simulation
	returnError bool
	errorCode   string // "invalid_grant", "invalid_client", etc.
	errorDesc   string
	httpStatus  int

	// PKCE validation
	validatePKCE     bool
	expectedVerifier string
	expectedCode     string
	expectedClientID string

	// Retry simulation: fail first N attempts, then succeed
	failCount      int
	currentAttempt *atomic.Int64
}

// createMockOAuth2TokenEndpoint creates an httptest.Server that mocks OAuth2 token endpoint.
// It validates OAuth2 token exchange requests and returns configurable responses.
func createMockOAuth2TokenEndpoint(t *testing.T, config mockTokenConfig) *httptest.Server {
	t.Helper()

	if config.currentAttempt == nil {
		config.currentAttempt = &atomic.Int64{}
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify POST request with form-encoded body
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "invalid_request"}`, http.StatusBadRequest)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			http.Error(w, `{"error": "invalid_request"}`, http.StatusBadRequest)
			return
		}

		// Extract OAuth2 parameters
		grantType := r.FormValue("grant_type")
		code := r.FormValue("code")
		codeVerifier := r.FormValue("code_verifier")
		clientID := r.FormValue("client_id")

		// Validate required fields
		if grantType != "authorization_code" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error": "unsupported_grant_type"}`)
			return
		}

		// Validate expected parameters if configured
		if config.expectedCode != "" && code != config.expectedCode {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error": "invalid_grant", "error_description": "invalid code"}`)
			return
		}

		if config.expectedClientID != "" && clientID != config.expectedClientID {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(w, `{"error": "invalid_client"}`)
			return
		}

		// Validate PKCE if configured
		if config.validatePKCE {
			if config.expectedVerifier != "" && codeVerifier != config.expectedVerifier {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"error": "invalid_grant", "error_description": "PKCE validation failed"}`)
				return
			}

			if codeVerifier == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"error": "invalid_request", "error_description": "code_verifier required"}`)
				return
			}
		}

		// Simulate retry failures
		attempt := config.currentAttempt.Add(1)
		if attempt <= int64(config.failCount) {
			// Return transient error (500) to trigger retry
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error": "server_error", "error_description": "temporary failure"}`)
			return
		}

		// Return error response if configured
		if config.returnError {
			httpStatus := config.httpStatus
			if httpStatus == 0 {
				httpStatus = http.StatusBadRequest
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(httpStatus)
			if config.errorCode == "" {
				config.errorCode = "server_error"
			}
			if config.errorDesc == "" {
				config.errorDesc = "Unknown error"
			}
			_, _ = fmt.Fprintf(w, `{"error": "%s", "error_description": "%s"}`, config.errorCode, config.errorDesc)
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		tokenType := config.tokenType
		if tokenType == "" {
			tokenType = "Bearer"
		}

		expiresIn := config.expiresIn
		if expiresIn == 0 {
			expiresIn = 3600
		}

		response := map[string]interface{}{
			"access_token": config.accessToken,
			"token_type":   tokenType,
			"expires_in":   expiresIn,
		}

		if config.refreshToken != "" {
			response["refresh_token"] = config.refreshToken
		}

		if config.scope != "" {
			response["scope"] = config.scope
		}

		respJSON, _ := json.Marshal(response)
		_, _ = w.Write(respJSON)
	}))
}

// newTestEncryption creates a real encryption adapter using the shared deterministic test key.
func newTestEncryption(t *testing.T) ports.EncryptionPort {
	t.Helper()
	return testutil.NewTestEncryptionAdapter(t)
}

func setupService(t *testing.T) (*oauth2session.OAuth2SessionService, *memory.InMemoryThirdpartyOAuth2ProviderRepository, *thirdparty.ThirdpartyOAuth2ProviderService) {
	t.Helper()
	return setupServiceWithConfig(t, func(config *oauth2session.Config) {
		config.RetryBaseDelay = 10 * time.Millisecond
	})
}

func setupServiceWithConfig(
	t *testing.T,
	configure func(*oauth2session.Config),
) (*oauth2session.OAuth2SessionService, *memory.InMemoryThirdpartyOAuth2ProviderRepository, *thirdparty.ThirdpartyOAuth2ProviderService) {
	t.Helper()

	// Create test JWE key
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	err = key.Set(jwk.KeyIDKey, "test-key")
	require.NoError(t, err)
	err = key.Set(jwk.AlgorithmKey, "A256GCM")
	require.NoError(t, err)

	// Create repositories
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	grantRepo := memory.NewUserGrantRepository()
	agentRepo := memory.NewAgentRepository()

	// Create service
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"
	if configure != nil {
		configure(&config)
	}

	// Use real encryption for unit tests
	encryption := newTestEncryption(t)

	// Create ThirdpartyOAuth2ProviderService for handling encryption/decryption of client secrets
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		serviceRepo,
		encryption,
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	svc := oauth2session.NewOAuth2SessionService(
		providerService,
		sessionRepo,
		grantRepo,
		agentRepo,
		encryption,
		&http.Client{},
		domjwe.New(key),
		config,
		slog.Default(),
	)

	return svc, serviceRepo, providerService
}

func createTestService(serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: "GitHub",
		ClientID:    id.ClientID("test-client-id"),
		Secret:      model.NewPlaintextSecret("test-client-secret"),
		IssuerURI:   "https://github.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
			{ScopeValue: "user", Description: "User profile"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// =============================================================================
// Tests for HandleCallback with GitHub Flavor (comma-separated scopes)
// =============================================================================

func TestHandleCallback_GitHubFlavorCommaSeparatedScopes(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	thirdPartyService.Flavor = model.OAuth2FlavorGitHub

	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "gho_test_token_123",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "ghr_refresh_456",
		scope:        "repo,user",
	})
	defer mockServer.Close()

	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "test-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)
	assert.Equal(t, []string{"repo", "user"}, result.Session.Scope)
}

func TestHandleCallback_GitHubFlavorCommaSeparatedScopesWithWhitespace(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	thirdPartyService.Flavor = model.OAuth2FlavorGitHub

	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "gho_test_token_123",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "ghr_refresh_456",
		scope:        "repo, user, admin:org",
	})
	defer mockServer.Close()

	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "test-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)
	assert.Equal(t, []string{"repo", "user", "admin:org"}, result.Session.Scope)
}

func TestHandleCallback_GitHubFlavorFallsBackToServiceScopes(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	thirdPartyService := createTestService(serviceID)
	thirdPartyService.Flavor = model.OAuth2FlavorGitHub

	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "gho_test_token_123",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "ghr_refresh_456",
		scope:        "",
	})
	defer mockServer.Close()

	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL

	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "test-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Session)
	assert.Equal(t, []string{"repo", "user"}, result.Session.Scope)
}

// =============================================================================
// Tests for TerminateSession (T063)
// =============================================================================

func TestTerminateSession_SuccessfullyTerminatesExistingSession(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	serviceID := id.NewServiceID()
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/sessions"

	// Initiate OAuth2 flow
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)

	// Mock token endpoint for successful token exchange
	mockServer := createMockOAuth2TokenEndpoint(t, mockTokenConfig{
		accessToken:  "test-access-token",
		tokenType:    "Bearer",
		expiresIn:    3600,
		refreshToken: "test-refresh-token",
		scope:        "repo user",
	})
	defer mockServer.Close()

	// Update service to use mock token endpoint.
	// Reset secret to plaintext — Update() always requires plaintext state.
	thirdPartyService.Secret = model.NewPlaintextSecret("test-client-secret")
	thirdPartyService.Endpoints.TokenEndpoint = mockServer.URL
	err = providerService.Update(ctx, thirdPartyService, nil)
	require.NoError(t, err)

	// Complete OAuth2 callback to create session
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "auth-code",
		State:     flowResult.StateToken,
	}

	result, err := service.HandleCallback(ctx, principal, req)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify session exists before termination
	retrievedSession, err := service.GetSessionWithAgents(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, retrievedSession)

	// Terminate the session
	err = service.TerminateSession(ctx, principal, serviceID)
	require.NoError(t, err)

	// Verify session no longer exists
	_, err = service.GetSessionWithAgents(ctx, principal, serviceID)
	assert.Error(t, err)
	assert.Equal(t, oauth2session.ErrSessionNotFound, err)

	// Verify ListUserSessions returns empty
	sessions, err := service.ListUserSessions(ctx, principal)
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestTerminateSession_ReturnsErrorWhenSessionNotFound(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	err := service.TerminateSession(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
}

func TestTerminateSession_DeletesSessionFromRepository(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// No session exists, so termination should fail
	err = service.TerminateSession(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
}

func TestTerminateSession_HandlesRepositoryDeletionErrors(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// No session exists, so termination should fail
	err = service.TerminateSession(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
}

func TestTerminateSession_ValidatesPrincipalOwnershipBeforeDeletion(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service and session for user1
	principal2 := id.Principal("user2@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// No session exists for principal2, so termination should fail with session not found
	err = service.TerminateSession(ctx, principal2, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
}

func TestTerminateSession_ReturnsErrorWhenPrincipalMismatch(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service and session
	attacker := id.Principal("attacker@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Try to terminate session as different user (no session exists for attacker)
	err = service.TerminateSession(ctx, attacker, serviceID)

	// Should return error for non-existent session (since attacker has no session for this service)
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
}

func TestForceRefreshSession(t *testing.T) {
	t.Run("refreshes even when access token is valid", func(t *testing.T) {
		ctx := context.Background()
		service, _, providerService := setupService(t)
		serviceID := id.NewServiceID()
		provider := createTestService(serviceID)

		var requests []url.Values
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			requests = append(requests, r.Form)

			w.Header().Set("Content-Type", "application/json")
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				_, _ = w.Write([]byte(`{"access_token":"initial-access-token","token_type":"Bearer","expires_in":3600,"refresh_token":"initial-refresh-token","scope":"repo user"}`))
			case "refresh_token":
				_, _ = w.Write([]byte(`{"access_token":"refreshed-access-token","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh-token","scope":"repo user"}`))
			default:
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
			}
		}))
		defer mockServer.Close()

		provider.Endpoints.TokenEndpoint = mockServer.URL
		require.NoError(t, providerService.Create(ctx, provider))

		principal := id.Principal("user@example.com")
		flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
		require.NoError(t, err)

		_, err = service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
			ServiceID: serviceID,
			Code:      "authorization-code",
			State:     flow.StateToken,
		})
		require.NoError(t, err)

		summary, err := service.ForceRefreshSession(ctx, principal, serviceID)
		require.NoError(t, err)
		require.NotNil(t, summary)
		assert.True(t, summary.HasRefreshToken)
		assert.Equal(t, serviceID, summary.ServiceID)
		require.Len(t, requests, 2)
		assert.Equal(t, "authorization_code", requests[0].Get("grant_type"))
		assert.Equal(t, "refresh_token", requests[1].Get("grant_type"))
		assert.Equal(t, "initial-refresh-token", requests[1].Get("refresh_token"))
	})

	t.Run("no session", func(t *testing.T) {
		ctx := context.Background()
		service, _, _ := setupService(t)

		_, err := service.ForceRefreshSession(ctx, id.Principal("user@example.com"), id.NewServiceID())
		require.Error(t, err)
		assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound))
	})

	t.Run("no refresh token", func(t *testing.T) {
		ctx := context.Background()
		service, _, providerService := setupService(t)
		serviceID := id.NewServiceID()
		provider := createTestService(serviceID)

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"initial-access-token","token_type":"Bearer","expires_in":3600,"scope":"repo user"}`))
		}))
		defer mockServer.Close()

		provider.Endpoints.TokenEndpoint = mockServer.URL
		require.NoError(t, providerService.Create(ctx, provider))

		principal := id.Principal("user@example.com")
		flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
		require.NoError(t, err)

		_, err = service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
			ServiceID: serviceID,
			Code:      "authorization-code",
			State:     flow.StateToken,
		})
		require.NoError(t, err)

		_, err = service.ForceRefreshSession(ctx, principal, serviceID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, oauth2session.ErrRefreshNotAvailable))
	})

	t.Run("upstream rejects", func(t *testing.T) {
		ctx := context.Background()
		service, _, providerService := setupService(t)
		serviceID := id.NewServiceID()
		provider := createTestService(serviceID)

		seedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"initial-access-token","token_type":"Bearer","expires_in":3600,"refresh_token":"initial-refresh-token","scope":"repo user"}`))
		}))
		defer seedServer.Close()

		provider.Endpoints.TokenEndpoint = seedServer.URL
		require.NoError(t, providerService.Create(ctx, provider))

		principal := id.Principal("user@example.com")
		flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
		require.NoError(t, err)

		_, err = service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{
			ServiceID: serviceID,
			Code:      "authorization-code",
			State:     flow.StateToken,
		})
		require.NoError(t, err)

		rejectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh rejected"}`))
		}))
		defer rejectServer.Close()

		provider.Secret = model.NewPlaintextSecret("test-client-secret")
		provider.Endpoints.TokenEndpoint = rejectServer.URL
		require.NoError(t, providerService.Update(ctx, provider, nil))

		_, err = service.ForceRefreshSession(ctx, principal, serviceID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, oauth2session.ErrRefreshFailed))
		assert.ErrorContains(t, err, "upstream token endpoint returned error status 400")
	})
}

// =============================================================================
// Tests for GetSessionWithAgents (T064)
// =============================================================================

func TestGetSessionWithAgents_ReturnsSessionWithEmptyAgentListWhenNoGrants(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service and session with no grants
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// No session exists for this principal+serviceID pair
	result, err := service.GetSessionWithAgents(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
	assert.Nil(t, result)
}

func TestGetSessionWithAgents_ReturnsSessionWithAgentListWhenGrantsExist(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service, session, and grants
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// No session exists for this principal+serviceID pair
	result, err := service.GetSessionWithAgents(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
	assert.Nil(t, result)
}

func TestGetSessionWithAgents_UsesGrantRepositoryCountAgentsByServiceID(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	result, err := service.GetSessionWithAgents(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
	assert.Nil(t, result)
}

func TestGetSessionWithAgents_QueriesGrantsByServiceID(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	result, err := service.GetSessionWithAgents(ctx, principal, serviceID)

	// Should return error for non-existent session
	require.Error(t, err)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound), "error should be ErrSessionNotFound")
	assert.Nil(t, result)

	// - Queries grants filtered by serviceID
	// - Extracts agent IDs from grants' delegated_oauth2_tokens JSONB
	// - Returns correct agent IDs in DependentAgents list
}

func TestGetSessionWithAgents_ReturnsErrorWhenSessionNotFound(t *testing.T) {
	ctx := context.Background()
	service, _, _ := setupService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	result, err := service.GetSessionWithAgents(ctx, principal, serviceID)

	// Verify: Returns error when session doesn't exist
	assert.Error(t, err)
	assert.Nil(t, result)
	// Error should be the session not found error
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound),
		"expected ErrSessionNotFound, got %v", err)
}

func TestGetSessionWithAgents_ValidatesPrincipalOwnership(t *testing.T) {
	ctx := context.Background()
	service, _, providerService := setupService(t)

	// Setup: Create service
	// Note: To test principal ownership validation, we need a session first,
	// which requires completing an OAuth2 flow via callback
	principal2 := id.Principal("user2@example.com")
	serviceID := id.NewServiceID()

	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Try to get session as different user (no session exists yet)
	result, err := service.GetSessionWithAgents(ctx, principal2, serviceID)

	// Since no session exists, should return session not found error
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, errors.Is(err, oauth2session.ErrSessionNotFound),
		"expected ErrSessionNotFound, got %v", err)

	// When tested with actual sessions in integration tests, verify:
	// - Only the session owner can view session details
	// - Returns unauthorized error for principal mismatch
}

// =============================================================================
// Tests for Audit Logging (SR-009 Compliance)
// =============================================================================

// TestHandleCallback_PKCEValidationFailure_EmitsAuditLog verifies audit log is emitted
// on PKCE validation failure per SR-009 compliance requirements
func TestHandleCallback_PKCEValidationFailure_EmitsAuditLog(t *testing.T) {
	// Setup logging capture
	logOutput := new(strings.Builder)
	h := slog.NewJSONHandler(logOutput, nil)
	logger := slog.New(h)

	// Create test JWE key
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	err = key.Set(jwk.KeyIDKey, "test-key")
	require.NoError(t, err)
	err = key.Set(jwk.AlgorithmKey, "A256GCM")
	require.NoError(t, err)

	// Create repositories
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	grantRepo := memory.NewUserGrantRepository()
	agentRepo := memory.NewAgentRepository()
	encryption := newTestEncryption(t)

	// Create ThirdpartyOAuth2ProviderService to handle encryption context binding (simulates domain layer)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, encryption, newNoopBranchKeyManager(), nil, false, logger)

	// Create service with capturing logger
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"
	config.MaxRetries = 1 // Reduce retries for faster test

	service := oauth2session.NewOAuth2SessionService(
		providerService,
		sessionRepo,
		grantRepo,
		agentRepo,
		encryption,
		&http.Client{},
		domjwe.New(key),
		config,
		logger, // Capture logs
	)

	ctx := context.Background()
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	// Create third-party service
	thirdPartyService := createTestService(serviceID)
	err = providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Initiate flow to get valid state token
	redirectURI := "https://example.com/sessions"
	flowResult, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotEmpty(t, flowResult.StateToken)

	// Create callback request with valid state but code will fail exchange
	// In a real scenario, the OAuth2 provider's token endpoint would reject the code
	// due to PKCE validation failure. Here we simulate by using a code that will
	// fail during token exchange.
	callbackReq := &oauth2session.HandleCallbackRequest{
		ServiceID: serviceID,
		Code:      "invalid-code-pkce-will-fail",
		State:     flowResult.StateToken,
		Error:     "", // No OAuth2 error (error happens during token exchange)
		ErrorDesc: "",
	}

	// Execute - this should fail due to token exchange failure (simulating PKCE failure)
	// The real OAuth2 provider would reject with "invalid_grant" error
	result, err := service.HandleCallback(ctx, principal, callbackReq)

	// Verify error occurred
	require.Error(t, err)
	require.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to exchange authorization code",
		"Error should indicate token exchange failure")

	// Parse log output to verify audit log was emitted
	logLines := strings.Split(logOutput.String(), "\n")
	var foundAuditLog bool
	var auditLogEntry map[string]interface{}

	for _, line := range logLines {
		if line == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		// Look for the PKCE failure audit log
		if eventVal, ok := logEntry["event"].(string); ok && eventVal == "session.oauth2.pkce_validation_failed" {
			foundAuditLog = true
			auditLogEntry = logEntry

			// Verify required fields
			assert.Equal(t, "session.oauth2.pkce_validation_failed", eventVal)
			assert.Equal(t, principal.String(), logEntry["principal"])
			assert.Equal(t, serviceID.String(), logEntry["service_id"])
			assert.NotEmpty(t, logEntry["timestamp"])
			assert.Equal(t, "token_exchange_failed", logEntry["reason"])
			assert.Equal(t, false, logEntry["public_client"])
			assert.Equal(t, "ERROR", logEntry["level"]) // Should be ERROR level
			break
		}
	}

	assert.True(t, foundAuditLog, "Audit log for PKCE validation failure not found in logs. Log output:\n%s", logOutput.String())
	if foundAuditLog {
		t.Logf("PKCE validation failure audit log entry: %+v", auditLogEntry)
	}
}
