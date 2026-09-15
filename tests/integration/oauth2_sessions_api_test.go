package integration_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/oauth2_sessions"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestListSessions_Success(t *testing.T) {
	// Setup
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	ctx := context.Background()

	principal := "user@example.com"
	serviceUUID := id.NewServiceID()

	// Create service first
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
	}
	err := serviceRepo.Create(ctx, service)
	assert.NoError(t, err)

	// Create a session
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo", "user"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	// Create service with dependencies
	service2 := createOAuth2SessionService(
		t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil, // encryption not needed for this test
		nil, // jweKey not needed for this test
		oauth2session.DefaultConfig(),
	)

	// Create handler
	handler := oauth2_sessions.NewHandler(service2)
	router := setupTestRouter(handler)

	// Make request
	req := httptest.NewRequest("GET", "/api/third-party/sessions", nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Sessions []map[string]interface{} `json:"sessions"`
		} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Len(t, resp.Data.Sessions, 1)

	// Verify session data
	sessionData := resp.Data.Sessions[0]
	assert.Equal(t, session.ID.String(), sessionData["id"])
	assert.Equal(t, serviceUUID.String(), sessionData["service_id"])
	assert.Equal(t, "GitHub", sessionData["service_display_name"])
	assert.Equal(t, "Bearer", sessionData["token_type"])
}

func TestListSessions_MissingPrincipal(t *testing.T) {
	// Setup
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	grantRepo := memory.NewUserGrantRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()

	service := createOAuth2SessionService(
		t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)

	// Make request without X-Remote-User header
	req := httptest.NewRequest("GET", "/api/third-party/sessions", nil)
	w := httptest.NewRecorder()

	handler.ListSessions(w, req)

	// Verify response
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
}

func TestListSessions_EmptySessions(t *testing.T) {
	// Setup
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	grantRepo := memory.NewUserGrantRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()

	service := createOAuth2SessionService(
		t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make request for user with no sessions
	req := httptest.NewRequest("GET", "/api/third-party/sessions", nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Sessions []interface{} `json:"sessions"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Empty(t, resp.Data.Sessions)
}

// =============================================================================
// Tests for Authorize Endpoint (InitiateFlow)
// =============================================================================

func TestAuthorizeEndpoint_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create third-party service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
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
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create JWE key for state tokens
	jweKey := createTestJWEKey(t)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	service := createOAuth2SessionService(
		t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil, // encryption not needed for authorize endpoint
		jweKey,
		config,
	)

	handler := oauth2_sessions.NewHandler(service)

	// Create router and register routes
	router := setupTestRouter(handler)

	// Make request
	principal := "user@example.com"
	redirectURI := "https://broker.example.com/sessions"
	reqURL := "/api/third-party/" + serviceUUID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(redirectURI)

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	req.Host = "broker.example.com"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify: Status 302 Redirect
	assert.Equal(t, http.StatusFound, w.Code)

	// Verify: Location header contains authorization URL
	location := w.Header().Get("Location")
	assert.NotEmpty(t, location)
	assert.Contains(t, location, "https://github.com/login/oauth/authorize")

	// Verify: Location contains required OAuth2 parameters
	authURL, err := url.Parse(location)
	assert.NoError(t, err)
	query := authURL.Query()

	// Verify state parameter (JWE token)
	assert.NotEmpty(t, query.Get("state"))

	// Verify PKCE parameters
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.NotEmpty(t, query.Get("code_challenge"))

	// Verify other OAuth2 parameters
	assert.Equal(t, "test-client-id", query.Get("client_id"))
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Contains(t, query.Get("scope"), "repo")
}

func TestAuthorizeEndpoint_MissingPrincipal(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	service := createOAuth2SessionService(
		t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make request without X-Remote-User header
	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/oauth2/authorize?redirect_uri=https://example.com"

	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
}

func TestAuthorizeEndpoint_ServiceNotFound(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	jweKey := createTestJWEKey(t)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		config,
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make request for non-existent service
	principal := "user@example.com"
	serviceID := "00000000-0000-0000-0000-000000000000"
	reqURL := "/api/third-party/" + serviceID + "/oauth2/authorize?redirect_uri=https://broker.example.com/callback"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 404 Not Found
	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "service_not_found", resp["error"])
}

func TestAuthorizeEndpoint_InvalidRedirectURI(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
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
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		config,
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	tests := []struct {
		name        string
		redirectURI string
	}{
		{
			name:        "different host",
			redirectURI: "https://evil.com/callback",
		},
		{
			name:        "same host different scheme",
			redirectURI: "http://broker.example.com/callback",
		},
	}

	principal := "user@example.com"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqURL := "/api/third-party/" + serviceUUID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(tt.redirectURI)
			req := httptest.NewRequest("GET", reqURL, nil)
			req.Header.Set("X-Remote-User", principal)
			req.Host = "broker.example.com" // Set the request host
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp map[string]string
			err = json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, "invalid_redirect_uri", resp["error"])
		})
	}
}

func TestAuthorizeEndpoint_VerifyAuthorizationURL(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
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
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		config,
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	redirectURI := "https://broker.example.com/sessions"
	reqURL := "/api/third-party/" + serviceUUID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(redirectURI)

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	req.Host = "broker.example.com"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Verify status and location
	assert.Equal(t, http.StatusFound, w.Code)
	location := w.Header().Get("Location")
	assert.NotEmpty(t, location)

	// Parse and verify authorization URL
	authURL, err := url.Parse(location)
	assert.NoError(t, err)
	assert.Equal(t, "github.com", authURL.Host)
	assert.Equal(t, "/login/oauth/authorize", authURL.Path)

	query := authURL.Query()
	assert.Equal(t, "test-client-id", query.Get("client_id"))
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Contains(t, query.Get("redirect_uri"), "/oauth2/callback")
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
	assert.NotEmpty(t, query.Get("code_challenge"))
	assert.NotEmpty(t, query.Get("state"))

	// Verify scopes
	scope := query.Get("scope")
	assert.Contains(t, scope, "repo")
	assert.Contains(t, scope, "user")
}

func TestAuthorizeEndpoint_StateTokenCanBeDecrypted(t *testing.T) {
	// This test verifies the state token in the authorization URL can be decrypted
	// and contains the correct claims
	// TDD: Will be implemented along with the handler
}

func TestAuthorizeEndpoint_PKCEPresent(t *testing.T) {
	// This test specifically verifies PKCE parameters are present in the authorization URL
	// TDD: Will be implemented along with the handler
}

// =============================================================================
// Tests for Callback Endpoint (HandleCallback)
// =============================================================================

func TestCallbackEndpoint_Success(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
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
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil, // Will need mock encryption for full test
		jweKey,
		config,
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// For TDD, we'll simulate having a state token
	// In real scenario, this comes from InitiateFlow
	principal := "user@example.com"
	stateToken := "dummy-state-token"

	// Make callback request
	url := "/api/third-party/" + serviceUUID.String() + "/oauth2/callback?code=auth-code-123&state=" + stateToken

	req := httptest.NewRequest("GET", url, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Will fail until handler is implemented
	// When implemented, verify:
	// - Status: 302 Redirect (to redirect_uri)
	// - Session created in database
	// - Session has encrypted tokens
	// - Redirect location includes success indicator
}

func TestCallbackEndpoint_MissingPrincipal(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/oauth2/callback?code=auth-code&state=state-token"

	req := httptest.NewRequest("GET", reqURL, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should return 401 Unauthorized
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var resp map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, "unauthorized", resp["error"])
}

func TestCallbackEndpoint_InvalidStateToken(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	jweKey := createTestJWEKey(t)

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/oauth2/callback?code=auth-code&state=invalid-token-xyz"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should redirect with error (invalid state token)
	assert.Equal(t, http.StatusFound, w.Code)

	location := w.Header().Get("Location")
	assert.True(t, strings.HasPrefix(location, "/sessions?"), "unexpected callback redirect: %s", location)
	assert.Contains(t, location, "error=invalid_state")
}

func TestCallbackEndpoint_ExpiredStateToken(t *testing.T) {
	// This test will create an expired state token and verify it's rejected
	// TDD: Will be implemented along with state token creation/validation
}

func TestCallbackEndpoint_PrincipalMismatch(t *testing.T) {
	// This test verifies CSRF protection:
	// - User1 initiates flow
	// - User2 tries to complete callback with User1's state token
	// - Should fail with 403 Forbidden
	// TDD: Will be implemented along with handler
}

func TestCallbackEndpoint_OAuth2Error(t *testing.T) {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionRepo := memory.NewInMemoryUserSessionRepository()
			serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
			grantRepo := memory.NewUserGrantRepository()
			jweKey := createTestJWEKey(t)

			service := createOAuth2SessionService(t,
				serviceRepo,
				sessionRepo,
				grantRepo,
				nil,
				jweKey,
				oauth2session.DefaultConfig(),
			)

			handler := oauth2_sessions.NewHandler(service)
			router := setupTestRouter(handler)

			principal := "user@example.com"
			serviceID := uuid.New().String()
			// Build URL with properly encoded query parameters
			params := url.Values{}
			params.Set("error", tt.error)
			params.Set("error_description", tt.errorDesc)
			params.Set("state", "state-token")
			reqURL := "/api/third-party/" + serviceID + "/oauth2/callback?" + params.Encode()

			req := httptest.NewRequest("GET", reqURL, nil)
			req.Header.Set("X-Remote-User", principal)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// Should return 302 redirect with error in query params
			assert.Equal(t, http.StatusFound, w.Code)

			location := w.Header().Get("Location")
			assert.True(t, strings.HasPrefix(location, "/sessions?"), "unexpected callback redirect: %s", location)
			assert.Contains(t, location, "error="+tt.error)
			assert.Contains(t, location, url.QueryEscape(tt.errorDesc))
		})
	}
}

func TestCallbackEndpoint_MissingCode(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	jweKey := createTestJWEKey(t)

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/oauth2/callback?state=state-token"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should redirect with error (missing code)
	assert.Equal(t, http.StatusFound, w.Code)

	location := w.Header().Get("Location")
	assert.True(t, strings.HasPrefix(location, "/sessions?"), "unexpected callback redirect: %s", location)
	assert.Contains(t, location, "error=invalid_callback")
}

func TestCallbackEndpoint_MissingState(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/oauth2/callback?code=auth-code"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should redirect with error (missing state)
	assert.Equal(t, http.StatusFound, w.Code)

	location := w.Header().Get("Location")
	assert.True(t, strings.HasPrefix(location, "/sessions?"), "unexpected callback redirect: %s", location)
	assert.Contains(t, location, "error=invalid_callback")
}

func TestCallbackEndpoint_SessionStoredInDatabase(t *testing.T) {
	// This test verifies that after successful callback:
	// - Session is stored in database
	// - Tokens are encrypted
	// - Session can be retrieved
	// TDD: Will be implemented with full flow
}

func TestCallbackEndpoint_RedirectLocationValid(t *testing.T) {
	// This test verifies the redirect Location header after callback:
	// - Should redirect to redirect_uri from state token
	// - Should include success/error indicators
	// TDD: Will be implemented with handler
}

func TestCallbackEndpoint_TokenExchangeWithMockThirdParty(t *testing.T) {
	// This test will mock the third-party token endpoint using httptest.Server
	// and verify the token exchange flow works correctly
	// TDD: Will be implemented with handler
}

// =============================================================================
// Tests for DELETE /api/third-party/{serviceId}/session (T065)
// =============================================================================

func TestDeleteSession_SuccessfullyTerminatesSessionWithStatusOK(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	// Create service
	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make DELETE request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 200 OK
	// - Body: success response or empty
	// - Session is deleted from database
}

func TestDeleteSession_ReturnsNotFoundWhenSessionDoesntExist(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	jweKey := createTestJWEKey(t)

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	serviceID := "00000000-0000-0000-0000-000000000000"
	reqURL := "/api/third-party/" + serviceID + "/session"

	req := httptest.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 404 Not Found
	// - Body contains error message
}

func TestDeleteSession_ReturnsUnauthorizedWhenXRemoteUserMissing(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/session"

	req := httptest.NewRequest("DELETE", reqURL, nil)
	// No X-Remote-User header
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 401 Unauthorized
	// - Body contains error message
}

func TestDeleteSession_ReturnsForbiddenWhenPrincipalDoesntMatch(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session for user1
	owner := "owner@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(owner),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Try to delete as different user
	attacker := "attacker@example.com"
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"

	req := httptest.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-Remote-User", attacker)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 403 Forbidden
	// - Body contains error message
	// - Session is NOT deleted
}

func TestDeleteSession_VerifiesSessionDeletedFromDatabase(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make DELETE request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Session is deleted from database
	// - Subsequent FindByPrincipalAndService returns nil
}

func TestDeleteSession_VerifiesTokensDeleted(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session with encrypted tokens
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             serviceUUID,
		EncryptedAccessToken:  []byte("encrypted-access-token"),
		EncryptedRefreshToken: []byte("encrypted-refresh-token"),
		TokenType:             "Bearer",
		Scope:                 []string{"repo"},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make DELETE request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("DELETE", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Encrypted tokens are deleted (session no longer exists)
	// - No sensitive data remains in storage
}

func TestRefreshSession_ReturnsRefreshedSummary(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	principal := "user@example.com"
	serviceUUID := id.NewServiceID()

	var requests []url.Values
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		requests = append(requests, r.Form)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh-token","scope":"repo"}`))
	}))
	defer mockServer.Close()

	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     mockServer.URL,
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "repo", Description: "Repository access"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, serviceRepo.Create(ctx, thirdPartyService))

	expiry := time.Now().Add(time.Hour)
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             serviceUUID,
		EncryptedAccessToken:  encryptSecretForTest(t, serviceUUID.String(), "old-access-token"),
		EncryptedRefreshToken: encryptSecretForTest(t, serviceUUID.String(), "old-refresh-token"),
		TokenType:             "Bearer",
		Scope:                 []string{"repo"},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
		AccessTokenExpiresAt:  &expiry,
	}
	require.NoError(t, sessionRepo.Create(ctx, session))

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		createTestJWEKey(t),
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+serviceUUID.String()+"/session/refresh", nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, serviceUUID.String(), resp.Data["service_id"])
	assert.Equal(t, true, resp.Data["has_refresh_token"])
	require.Len(t, requests, 1)
	assert.Equal(t, "refresh_token", requests[0].Get("grant_type"))
	assert.Equal(t, "old-refresh-token", requests[0].Get("refresh_token"))
}

func TestRefreshSession_ReturnsNotFoundWhenSessionDoesntExist(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		createTestJWEKey(t),
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	serviceID := id.NewServiceID()
	router := setupTestRouter(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+serviceID.String()+"/session/refresh", nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "session not found")
}

func TestRefreshSession_ReturnsConflictWhenRefreshTokenUnavailable(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	principal := "user@example.com"
	serviceUUID := id.NewServiceID()
	expiry := time.Now().Add(time.Hour)

	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: encryptSecretForTest(t, serviceUUID.String(), "old-access-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
		AccessTokenExpiresAt: &expiry,
	}
	require.NoError(t, sessionRepo.Create(ctx, session))

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		createTestJWEKey(t),
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+serviceUUID.String()+"/session/refresh", nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "refresh_unavailable")
}

func TestRefreshSession_ReturnsNotFoundForCrossPrincipalRequest(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	serviceUUID := id.NewServiceID()

	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal("owner@example.com"),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: encryptSecretForTest(t, serviceUUID.String(), "old-access-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	require.NoError(t, sessionRepo.Create(ctx, session))

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		createTestJWEKey(t),
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+serviceUUID.String()+"/session/refresh", nil)
	req.Header.Set("X-Remote-User", "attacker@example.com")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "session not found")
}

func TestRefreshSession_ReturnsBadGatewayWhenProviderRefreshFails(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	principal := "user@example.com"
	serviceUUID := id.NewServiceID()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server_error","error_description":"refresh failed"}`))
	}))
	defer mockServer.Close()

	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     mockServer.URL,
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "repo", Description: "Repository access"}},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, serviceRepo.Create(ctx, thirdPartyService))

	expiry := time.Now().Add(time.Hour)
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             id.Principal(principal),
		ServiceID:             serviceUUID,
		EncryptedAccessToken:  encryptSecretForTest(t, serviceUUID.String(), "old-access-token"),
		EncryptedRefreshToken: encryptSecretForTest(t, serviceUUID.String(), "old-refresh-token"),
		TokenType:             "Bearer",
		Scope:                 []string{"repo"},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
		AccessTokenExpiresAt:  &expiry,
	}
	require.NoError(t, sessionRepo.Create(ctx, session))

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		createTestJWEKey(t),
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)
	req := httptest.NewRequest(http.MethodPost, "/api/third-party/"+serviceUUID.String()+"/session/refresh", nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "refresh_failed")
}

// =============================================================================
// Tests for GET /api/third-party/{serviceId}/session (T066)
// =============================================================================

func TestGetSession_ReturnsSessionDetailsWithDependentAgentList(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	// TODO: Create grants with delegated tokens for this service

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make GET request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 200 OK
	// - Body contains session + dependent_agents list
}

func TestGetSession_ReturnsNotFoundWhenSessionDoesntExist(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()
	jweKey := createTestJWEKey(t)

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	principal := "user@example.com"
	serviceID := "00000000-0000-0000-0000-000000000000"
	reqURL := "/api/third-party/" + serviceID + "/session"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 404 Not Found
	// - Body contains error message
}

func TestGetSession_ReturnsUnauthorizedWhenXRemoteUserMissing(t *testing.T) {
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		nil,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	serviceID := uuid.New().String()
	reqURL := "/api/third-party/" + serviceID + "/session"

	req := httptest.NewRequest("GET", reqURL, nil)
	// No X-Remote-User header
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 401 Unauthorized
	// - Body contains error message
}

func TestGetSession_ReturnsForbiddenWhenPrincipalDoesntMatch(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session for user1
	owner := "owner@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(owner),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Try to get session as different user
	attacker := "attacker@example.com"
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"

	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", attacker)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 403 Forbidden
	// - Body contains error message
}

func TestGetSession_IncludesAgentCountInResponse(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	// TODO: Create 2 grants with delegated tokens for this service

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make GET request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 200 OK
	// - Response JSON includes "dependent_agent_count": 2
}

func TestGetSession_ValidatesJSONStructureMatchesSessionWithAgents(t *testing.T) {
	ctx := context.Background()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	grantRepo := memory.NewUserGrantRepository()

	// Create service
	serviceUUID := id.NewServiceID()
	thirdPartyService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceUUID,
		DisplayName: "GitHub",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewEncryptedSecret(encryptSecretForTest(t, serviceUUID.String(), "test-secret")),
		IssuerURI:   "https://github.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := serviceRepo.Create(ctx, thirdPartyService)
	assert.NoError(t, err)

	// Create session
	principal := "user@example.com"
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal(principal),
		ServiceID:            serviceUUID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"repo"},
		InitiatedAt:          time.Now(),
		CreatedAt:            time.Now(),
	}
	err = sessionRepo.Create(ctx, session)
	assert.NoError(t, err)

	jweKey := createTestJWEKey(t)
	service := createOAuth2SessionService(t,
		serviceRepo,
		sessionRepo,
		grantRepo,
		nil,
		jweKey,
		oauth2session.DefaultConfig(),
	)

	handler := oauth2_sessions.NewHandler(service)
	router := setupTestRouter(handler)

	// Make GET request
	reqURL := "/api/third-party/" + serviceUUID.String() + "/session"
	req := httptest.NewRequest("GET", reqURL, nil)
	req.Header.Set("X-Remote-User", principal)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// TDD: Expected to fail until handler is implemented
	// When implemented, verify:
	// - Status: 200 OK
	// - Response has fields: session (object), dependent_agents (array)
	// - Session object has: id, service_id, principal, token_type, scope, initiated_at
	// - dependent_agents is array of strings (agent IDs)
}

// =============================================================================
// Test Helpers
// =============================================================================

// setupTestRouter creates a chi router with OAuth2 session routes registered.
func setupTestRouter(handler *oauth2_sessions.Handler) *chi.Mux {
	router := chi.NewRouter()
	// Add principal middleware to extract X-Remote-User header from the request
	config := ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{
			PrincipalHeaderName: "X-Remote-User",
		},
	}
	router.Use(middleware.OptionalPrincipalMiddleware(config, nil, slog.Default()))
	// Mount routes under /api to match production setup
	router.Route("/api", func(r chi.Router) {
		handler.RegisterRoutes(r)
	})
	return router
}

// newTestEncryption creates a real encryption adapter using a deterministic test key.
func newTestEncryption(t *testing.T) ports.EncryptionPort {
	t.Helper()
	adapter, _, err := awsencryption.NewAWSEncryption("ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA=", "", 0)
	require.NoError(t, err)
	return adapter
}

func encryptSecretForTest(t *testing.T, serviceID, secret string) []byte {
	t.Helper()
	enc := newTestEncryption(t)
	ciphertext, err := enc.Encrypt(context.Background(), []byte(secret), map[string]string{"service_id": serviceID})
	require.NoError(t, err)
	return ciphertext
}

func createOAuth2SessionService(
	t *testing.T,
	serviceRepo ports.ThirdpartyOAuth2ProviderRepository,
	sessionRepo ports.UserSessionRepository,
	grantRepo ports.UserGrantRepository,
	encryption ports.EncryptionPort,
	jweKey jwk.Key,
	config oauth2session.Config,
) *oauth2session.OAuth2SessionService {
	t.Helper()

	agentRepo := memory.NewAgentRepository()

	// Create ServiceManager for handling encryption/decryption of client secrets
	// Use test encryption if not provided
	if encryption == nil {
		encryption = newTestEncryption(t)
	}
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		serviceRepo,
		encryption,
		&encryptionnoop.BranchKeyManager{},
		nil,
		false,
		slog.Default(),
	)

	return oauth2session.NewOAuth2SessionService(
		providerService,
		sessionRepo,
		grantRepo,
		agentRepo,
		encryption,
		&http.Client{},
		domjwe.New(jweKey),
		config,
		slog.Default(),
	)
}

func createTestJWEKey(t *testing.T) jwk.Key {
	t.Helper()

	// Create a symmetric key for JWE
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	if err != nil {
		t.Fatalf("failed to create JWE key: %v", err)
	}

	// Set key metadata
	if err := key.Set(jwk.KeyIDKey, "test-key"); err != nil {
		t.Fatalf("failed to set key ID: %v", err)
	}
	if err := key.Set(jwk.AlgorithmKey, "A256GCM"); err != nil {
		t.Fatalf("failed to set algorithm: %v", err)
	}

	return key
}
