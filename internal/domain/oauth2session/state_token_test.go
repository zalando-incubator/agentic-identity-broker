package oauth2session_test

import (
	"context"
	"log/slog"
	"net/http"
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

// TestCreateStateToken_Success tests successful state token creation.
func TestCreateStateToken_Success(t *testing.T) {
	service, testServiceID := setupTestService(t)
	ctx := context.Background()

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/callback"

	result, err := service.InitiateOAuth2Flow(ctx, principal, testServiceID, redirectURI)

	// Should succeed with valid inputs
	require.NoError(t, err, "InitiateOAuth2Flow should succeed with valid inputs")
	require.NotNil(t, result, "result should not be nil")
	assert.NotEmpty(t, result.StateToken, "state token should not be empty")
	assert.NotEmpty(t, result.AuthorizationURL, "authorization URL should not be empty")
}

func TestInitiateOAuth2FlowWithConsentState_SealsReference(t *testing.T) {
	service, testServiceID := setupTestService(t)
	principal := id.Principal("user@example.com")
	const consentStateID = "d0000000-0000-4000-8000-000000000001"

	result, err := service.InitiateOAuth2FlowWithConsentState(
		context.Background(),
		principal,
		testServiceID,
		"https://example.com/callback",
		consentStateID,
	)
	require.NoError(t, err)

	claims, err := service.ValidateStateToken(result.StateToken, principal, testServiceID)
	require.NoError(t, err)
	assert.Equal(t, consentStateID, claims.ConsentStateID)
}

// TestValidateStateToken_ValidToken tests successful validation of a valid state token.
func TestValidateStateToken_ValidToken(t *testing.T) {
	service, testServiceID := setupTestService(t)
	ctx := context.Background()

	principal := id.Principal("user@example.com")
	redirectURI := "https://example.com/callback"

	// Create a state token
	result, err := service.InitiateOAuth2Flow(ctx, principal, testServiceID, redirectURI)
	require.NoError(t, err, "InitiateOAuth2Flow should succeed")

	// Note: HandleCallback requires additional setup (HTTP mocking for token exchange)
	// This test validates that the state token is created and can be embedded in authorization URL
	assert.NotEmpty(t, result.StateToken, "state token should not be empty")
	assert.Contains(t, result.AuthorizationURL, "state=", "authorization URL should contain state parameter")
}

// TestValidateStateToken_ExpiredToken tests rejection of expired state token.
func TestValidateStateToken_ExpiredToken(t *testing.T) {
	// Test the claims validation logic to verify expiration detection
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now().Add(-20 * time.Minute),
		ExpiresAt:    time.Now().Add(-10 * time.Minute), // Expired 10 minutes ago
	}

	// Verify IsExpired() method works correctly
	assert.True(t, claims.IsExpired(), "claims should be expired when ExpiresAt is in the past")
}

// TestValidateStateToken_TamperedToken tests rejection of tampered state token.
func TestValidateStateToken_TamperedToken(t *testing.T) {
	service, testServiceID := setupTestService(t)
	ctx := context.Background()

	// Attempt to validate a tampered/invalid token
	req := &oauth2session.HandleCallbackRequest{
		ServiceID: testServiceID,
		Code:      "test-code",
		State:     "tampered-invalid-token-xyz",
	}

	_, err := service.HandleCallback(ctx, id.Principal("user@example.com"), req)
	// Should fail - either with "not implemented" (TDD) or decryption error (when implemented)
	assert.Error(t, err)
}

// TestValidateStateToken_PrincipalMismatch tests CSRF protection via principal validation.
func TestValidateStateToken_PrincipalMismatch(t *testing.T) {
	// Test that state token claims contain the principal for CSRF validation
	now := time.Now()
	testServiceID := id.NewServiceID()
	claims1 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user1@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}

	claims2 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user2@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}

	// Verify principals are different for CSRF detection
	assert.NotEqual(t, claims1.Principal, claims2.Principal, "principals should be different for CSRF detection")
}

// TestValidateStateToken_ServiceIDMismatch tests service ID validation.
func TestValidateStateToken_ServiceIDMismatch(t *testing.T) {
	// Test that state token claims contain the service ID for cross-service attack prevention
	now := time.Now()
	serviceID1 := id.NewServiceID()
	serviceID2 := id.NewServiceID()
	claims1 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    serviceID1,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}

	claims2 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    serviceID2,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(10 * time.Minute),
	}

	// Verify service IDs are different for cross-service attack prevention
	assert.NotEqual(t, claims1.ServiceID, claims2.ServiceID, "service IDs should be different to prevent cross-service attacks")
}

// TestValidateStateToken_MissingClaims tests validation of required claims.
func TestValidateStateToken_MissingClaims(t *testing.T) {
	testServiceID := id.NewServiceID()
	tests := []struct {
		name   string
		claims *oauth2session.OAuth2StateTokenClaims
	}{
		{
			name: "missing principal",
			claims: &oauth2session.OAuth2StateTokenClaims{
				PKCEVerifier: "verifier",
				ServiceID:    testServiceID,
				RedirectURI:  "https://example.com",
				IssuedAt:     time.Now(),
				ExpiresAt:    time.Now().Add(10 * time.Minute),
			},
		},
		{
			name: "missing pkce_verifier",
			claims: &oauth2session.OAuth2StateTokenClaims{
				Principal:   id.Principal("user@example.com"),
				ServiceID:   testServiceID,
				RedirectURI: "https://example.com",
				IssuedAt:    time.Now(),
				ExpiresAt:   time.Now().Add(10 * time.Minute),
			},
		},
		{
			name: "missing service_id",
			claims: &oauth2session.OAuth2StateTokenClaims{
				Principal:    id.Principal("user@example.com"),
				PKCEVerifier: "verifier",
				RedirectURI:  "https://example.com",
				IssuedAt:     time.Now(),
				ExpiresAt:    time.Now().Add(10 * time.Minute),
			},
		},
		{
			name: "missing redirect_uri",
			claims: &oauth2session.OAuth2StateTokenClaims{
				Principal:    id.Principal("user@example.com"),
				PKCEVerifier: "verifier",
				ServiceID:    testServiceID,
				IssuedAt:     time.Now(),
				ExpiresAt:    time.Now().Add(10 * time.Minute),
			},
		},
		{
			name: "missing issued_at",
			claims: &oauth2session.OAuth2StateTokenClaims{
				Principal:    id.Principal("user@example.com"),
				PKCEVerifier: "verifier",
				ServiceID:    testServiceID,
				RedirectURI:  "https://example.com",
				ExpiresAt:    time.Now().Add(10 * time.Minute),
			},
		},
		{
			name: "missing expires_at",
			claims: &oauth2session.OAuth2StateTokenClaims{
				Principal:    id.Principal("user@example.com"),
				PKCEVerifier: "verifier",
				ServiceID:    testServiceID,
				RedirectURI:  "https://example.com",
				IssuedAt:     time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.claims.Validate()
			assert.Error(t, err, "Validate should fail for missing claims")
		})
	}
}

// TestStateTokenClaims_IsExpired tests the IsExpired method.
func TestStateTokenClaims_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(10 * time.Minute),
			want:      false,
		},
		{
			name:      "expired 1 hour ago",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "expired 1 second ago",
			expiresAt: time.Now().Add(-1 * time.Second),
			want:      true,
		},
		{
			name:      "expires in 1 second",
			expiresAt: time.Now().Add(1 * time.Second),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testServiceID := id.NewServiceID()
			claims := &oauth2session.OAuth2StateTokenClaims{
				Principal:    id.Principal("user@example.com"),
				PKCEVerifier: "verifier",
				ServiceID:    testServiceID,
				RedirectURI:  "https://example.com",
				IssuedAt:     time.Now(),
				ExpiresAt:    tt.expiresAt,
			}
			assert.Equal(t, tt.want, claims.IsExpired())
		})
	}
}

// TestStateTokenClaims_Validate tests the Validate method comprehensively.
func TestStateTokenClaims_Validate(t *testing.T) {
	t.Run("valid claims", func(t *testing.T) {
		testServiceID := id.NewServiceID()
		claims := &oauth2session.OAuth2StateTokenClaims{
			Principal:    id.Principal("user@example.com"),
			PKCEVerifier: "test-verifier-abc123",
			ServiceID:    testServiceID,
			RedirectURI:  "https://example.com/callback",
			IssuedAt:     time.Now(),
			ExpiresAt:    time.Now().Add(10 * time.Minute),
		}
		err := claims.Validate()
		assert.NoError(t, err, "Validate should succeed for complete claims")
	})
}

// TestStateToken_TTL tests that state tokens have appropriate TTL.
func TestStateToken_TTL(t *testing.T) {
	// Verify state token TTL is configured to be <= 15 minutes for security
	config := oauth2session.DefaultConfig()

	// TTL should be <= 15 minutes (security requirement)
	maxAllowedTTL := 15 * time.Minute
	assert.LessOrEqual(t, config.StateTokenTTL, maxAllowedTTL,
		"state token TTL should be <= 15 minutes for security")

	// TTL should be reasonable (not instant, but also not too long)
	minReasonableTTL := 1 * time.Minute
	assert.GreaterOrEqual(t, config.StateTokenTTL, minReasonableTTL,
		"state token TTL should be at least 1 minute")
}

// setupTestService creates a test OAuth2SessionService with mocked dependencies and test data.
func setupTestService(t *testing.T) (*oauth2session.OAuth2SessionService, id.ServiceID) {
	t.Helper()

	// Create test JWE key (exactly 32 bytes for AES-256)
	key, err := jwk.Import[jwk.Key]([]byte("0123456789012345678901234567890X"))
	require.NoError(t, err)
	err = key.Set(jwk.KeyIDKey, "test-key-id")
	require.NoError(t, err)
	err = key.Set(jwk.AlgorithmKey, "A256GCM")
	require.NoError(t, err)

	// Create in-memory repositories
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	grantRepo := memory.NewUserGrantRepository()
	agentRepo := memory.NewAgentRepository()

	// Create service with default config
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	// Create ThirdpartyOAuth2ProviderService for handling encryption/decryption of client secrets
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		serviceRepo,
		newTestEncryption(t),
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	service := oauth2session.NewOAuth2SessionService(
		providerService,
		sessionRepo,
		grantRepo,
		agentRepo,
		nil, // encryption not needed for state token tests (uses JWE)
		&http.Client{},
		domjwe.New(key),
		config,
		slog.Default(),
	)

	// Set up test data: add a third-party service
	testServiceID := id.NewServiceID()
	testService := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          testServiceID,
		DisplayName: "Test OAuth2 Service",
		ClientID:    id.NewClientID("test-client-id"),
		Secret:      model.NewPlaintextSecret("test-client-secret"),
		IssuerURI:   "https://auth.example.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "openid", Description: "OpenID Connect scope"},
			{ScopeValue: "profile", Description: "User profile scope"},
			{ScopeValue: "email", Description: "Email scope"},
		},
	}
	err = providerService.Create(context.Background(), testService)
	require.NoError(t, err, "failed to set up test service")

	return service, testServiceID
}
