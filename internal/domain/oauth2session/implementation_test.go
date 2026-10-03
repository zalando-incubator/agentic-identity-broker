package oauth2session_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
)

// setupImplementedService creates a service with full encryption support for testing the implementation.
func setupImplementedService(t *testing.T) (*oauth2session.OAuth2SessionService, *thirdparty.ThirdpartyOAuth2ProviderService) {
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
	encryption := newTestEncryption(t)

	// Create service
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

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
		sessionRepo,
		grantRepo,
		agentRepo,
		encryption,
		&http.Client{},
		domjwe.New(key),
		config,
		slog.Default(),
	)

	return svc, providerService
}

func TestImplementation_InitiateOAuth2Flow_Success(t *testing.T) {
	ctx := context.Background()
	service, providerService := setupImplementedService(t)

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
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Call InitiateOAuth2Flow
	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	// Verify success
	require.NoError(t, err, "InitiateOAuth2Flow should succeed")
	require.NotNil(t, result, "result should not be nil")
	assert.NotEmpty(t, result.AuthorizationURL, "authorization URL should not be empty")
	assert.NotEmpty(t, result.StateToken, "state token should not be empty")

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

func TestImplementation_StateTokenValidation_Success(t *testing.T) {
	ctx := context.Background()
	service, providerService := setupImplementedService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	// Create service
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Initiate flow to get state token
	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Validate state token
	claims, err := service.ValidateStateToken(result.StateToken, principal, serviceID)
	require.NoError(t, err, "ValidateStateToken should succeed")
	require.NotNil(t, claims, "claims should not be nil")

	// Verify claims
	assert.Equal(t, principal, claims.Principal, "principal should match")
	assert.Equal(t, serviceID, claims.ServiceID, "service ID should match")
	assert.Equal(t, redirectURI, claims.RedirectURI, "redirect URI should match")
	assert.NotEmpty(t, claims.PKCEVerifier, "PKCE verifier should not be empty")
	assert.False(t, claims.IssuedAt.IsZero(), "issued at should be set")
	assert.False(t, claims.ExpiresAt.IsZero(), "expires at should be set")
	assert.False(t, claims.IsExpired(), "token should not be expired")
}

func TestImplementation_StateTokenValidation_PrincipalMismatch(t *testing.T) {
	ctx := context.Background()
	service, providerService := setupImplementedService(t)

	principal1 := id.Principal("user1@example.com")
	principal2 := id.Principal("user2@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	// Create service
	thirdPartyService := createTestService(serviceID)
	err := providerService.Create(ctx, thirdPartyService)
	require.NoError(t, err)

	// Initiate flow as principal1
	result, err := service.InitiateOAuth2Flow(ctx, principal1, serviceID, redirectURI)
	require.NoError(t, err)

	// Try to validate as principal2 (CSRF attack)
	_, err = service.ValidateStateToken(result.StateToken, principal2, serviceID)
	assert.ErrorIs(t, err, oauth2session.ErrPrincipalMismatch, "should return ErrPrincipalMismatch")
}

func TestImplementation_StateTokenValidation_ServiceIDMismatch(t *testing.T) {
	ctx := context.Background()
	service, providerService := setupImplementedService(t)

	principal := id.Principal("user@example.com")
	serviceID1 := id.NewServiceID()
	serviceID2 := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	// Create two services
	service1 := createTestService(serviceID1)
	service2 := createTestService(serviceID2)
	service2.DisplayName = "Google"
	err := providerService.Create(ctx, service1)
	require.NoError(t, err)
	err = providerService.Create(ctx, service2)
	require.NoError(t, err)

	// Initiate flow for service1
	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID1, redirectURI)
	require.NoError(t, err)

	// Try to validate with service2 (wrong service)
	_, err = service.ValidateStateToken(result.StateToken, principal, serviceID2)
	assert.Error(t, err, "should return error on service ID mismatch")
	assert.Contains(t, err.Error(), "service_id mismatch", "error should indicate service ID mismatch")
}

func TestImplementation_StateTokenValidation_InvalidToken(t *testing.T) {
	service, _ := setupImplementedService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	// Try to validate invalid token
	_, err := service.ValidateStateToken("invalid-token-xyz", principal, serviceID)
	assert.Error(t, err, "should return error for invalid token")
	assert.ErrorIs(t, err, oauth2session.ErrInvalidStateToken, "should return ErrInvalidStateToken")
}

func TestImplementation_InitiateOAuth2Flow_ServiceNotFound(t *testing.T) {
	ctx := context.Background()
	service, _ := setupImplementedService(t)

	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	redirectURI := "https://example.com/sessions"

	result, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, redirectURI)

	assert.Error(t, err, "should return error for non-existent service")
	assert.Nil(t, result, "result should be nil on error")
	assert.Contains(t, err.Error(), "service not found", "error should indicate service not found")
}

func TestImplementation_StateToken_RoundTrip(t *testing.T) {
	service, _ := setupImplementedService(t)

	// Create claims
	now := time.Now()
	originalClaims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		ServiceID:    id.NewServiceID(),
		PKCEVerifier: "test-verifier-1234567890",
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}

	// Encrypt to token
	token, err := service.CreateStateToken(originalClaims)
	require.NoError(t, err, "CreateStateToken should succeed")
	assert.NotEmpty(t, token, "token should not be empty")

	// Decrypt and validate
	decryptedClaims, err := service.ValidateStateToken(token, originalClaims.Principal, originalClaims.ServiceID)
	require.NoError(t, err, "ValidateStateToken should succeed")
	require.NotNil(t, decryptedClaims, "decrypted claims should not be nil")

	// Verify all claims match
	assert.Equal(t, originalClaims.Principal, decryptedClaims.Principal)
	assert.Equal(t, originalClaims.ServiceID, decryptedClaims.ServiceID)
	assert.Equal(t, originalClaims.PKCEVerifier, decryptedClaims.PKCEVerifier)
	assert.Equal(t, originalClaims.RedirectURI, decryptedClaims.RedirectURI)
	assert.True(t, originalClaims.IssuedAt.Equal(decryptedClaims.IssuedAt))
	assert.True(t, originalClaims.ExpiresAt.Equal(decryptedClaims.ExpiresAt))
}

func TestOAuth2SessionService_ServiceAuthorizeURL(t *testing.T) {
	service, _ := setupImplementedService(t) // CallbackBaseURL = "https://broker.example.com"

	tests := []struct {
		name      string
		serviceID id.ServiceID
		expected  string
	}{
		{
			name:      "produces correct authorize URL for a service",
			serviceID: id.MustParseServiceID("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
			expected:  "https://broker.example.com/api/third-party/f47ac10b-58cc-4372-a567-0e02b2c3d479/oauth2/authorize",
		},
		{
			name:      "different service ID produces different URL",
			serviceID: id.MustParseServiceID("00000000-0000-0000-0000-000000000001"),
			expected:  "https://broker.example.com/api/third-party/00000000-0000-0000-0000-000000000001/oauth2/authorize",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, service.ServiceAuthorizeURL(tt.serviceID))
		})
	}
}
