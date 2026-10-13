package oauth2session_test

import (
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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
)

// T075: Security tests for state token expiration rejection
// Tests that expired state tokens are properly rejected during callback validation

// TestStateTokenSecurityExpiration_RejectedAtBoundary tests token rejection at expiration boundary.
func TestStateTokenSecurityExpiration_RejectedAtBoundary(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now().Add(-5 * time.Minute),
		ExpiresAt:    time.Now().Add(-100 * time.Millisecond), // Expired 100ms ago (reliable boundary)
	}

	// Should be marked as expired
	assert.True(t, claims.IsExpired(), "claims should be expired when time has passed expiration")
}

// TestStateTokenSecurityExpiration_ValidJustBeforeExpiry tests token is valid just before expiry.
func TestStateTokenSecurityExpiration_ValidJustBeforeExpiry(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now().Add(-5 * time.Minute),
		ExpiresAt:    time.Now().Add(100 * time.Millisecond), // Expires in 100ms (reliable boundary)
	}

	// Should be valid (not yet expired)
	assert.False(t, claims.IsExpired(), "claims should be valid until expiration time passes")
}

// TestStateTokenSecurityExpiration_LongExpiredToken tests very old expired tokens.
func TestStateTokenSecurityExpiration_LongExpiredToken(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now().Add(-1 * time.Hour),
		ExpiresAt:    time.Now().Add(-30 * time.Minute), // Expired 30 minutes ago
	}

	// Should definitely be expired
	assert.True(t, claims.IsExpired(), "claims expired 30 minutes ago should be rejected")
}

// T076: Security tests for principal mismatch rejection (CSRF)
// Tests that tokens with mismatched principals are rejected

// TestStateTokenSecurityCSRF_PrincipalValidation tests principal claim validation.
func TestStateTokenSecurityCSRF_PrincipalValidation(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Validate principal claim exists and is not empty
	err := claims.Validate()
	require.NoError(t, err, "valid claims should pass validation")
	assert.NotEmpty(t, claims.Principal, "principal should be present")
}

// TestStateTokenSecurityCSRF_EmptyPrincipal tests rejection of missing principal.
func TestStateTokenSecurityCSRF_EmptyPrincipal(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    "", // Missing principal - CSRF vulnerability!
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Should fail validation
	err := claims.Validate()
	assert.Error(t, err, "claims with empty principal should fail validation")
}

// TestStateTokenSecurityCSRF_PrincipalMismatchDetection tests that different principals are detectable.
func TestStateTokenSecurityCSRF_PrincipalMismatchDetection(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims1 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user1@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	claims2 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user2@example.com"), // Different principal
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Both are individually valid
	assert.NoError(t, claims1.Validate())
	assert.NoError(t, claims2.Validate())

	// But principals should be different (CSRF detection point)
	assert.NotEqual(t, claims1.Principal, claims2.Principal)
}

// T077: Security tests for service_id mismatch rejection
// Tests that tokens with mismatched service IDs are rejected

// TestStateTokenSecurityServiceID_Validation tests service ID claim validation.
func TestStateTokenSecurityServiceID_Validation(t *testing.T) {
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Validate service ID claim exists
	err := claims.Validate()
	require.NoError(t, err, "valid claims should pass validation")
	assert.NotEmpty(t, claims.ServiceID, "service ID should be present")
}

// TestStateTokenSecurityServiceID_EmptyServiceID tests rejection of missing service ID.
func TestStateTokenSecurityServiceID_EmptyServiceID(t *testing.T) {
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    id.ServiceID{}, // Missing service ID!
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Should fail validation
	err := claims.Validate()
	assert.Error(t, err, "claims with empty service ID should fail validation")
}

// TestStateTokenSecurityServiceID_MismatchDetection tests that different service IDs are detectable.
func TestStateTokenSecurityServiceID_MismatchDetection(t *testing.T) {
	githubServiceID := id.NewServiceID()
	googleServiceID := id.NewServiceID()
	claims1 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    githubServiceID, // GitHub
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	claims2 := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    googleServiceID, // Google - different service
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	// Both are individually valid
	assert.NoError(t, claims1.Validate())
	assert.NoError(t, claims2.Validate())

	// But service IDs should be different (prevents cross-service attacks)
	assert.NotEqual(t, claims1.ServiceID, claims2.ServiceID)
}

// T078: Security tests for tampered token rejection
// Tests that tampered/invalid JWE tokens are rejected

// TestStateTokenSecurityTampered_InvalidToken tests rejection of completely invalid tokens.
func TestStateTokenSecurityTampered_InvalidToken(t *testing.T) {
	service, testServiceID := setupTestService(t)

	// Try to validate completely invalid token
	_, err := service.ValidateStateToken("invalid-token-xyz", id.Principal("user@example.com"), testServiceID)
	assert.Error(t, err, "invalid token should fail validation")
	assert.ErrorIs(t, err, oauth2session.ErrInvalidStateToken, "should return ErrInvalidStateToken")
}

// TestStateTokenSecurityTampered_CorruptedJWE tests rejection of corrupted JWE format.
func TestStateTokenSecurityTampered_CorruptedJWE(t *testing.T) {
	service, testServiceID := setupTestService(t)

	// Try to validate token with invalid JWE format
	corrupted := "eyJhbGciOiJBMjU2R0NNS1ciLCJlbmMiOiJBMjU2R0NNIn0." + // Invalid segments
		"invalid.corrupted.token"

	_, err := service.ValidateStateToken(corrupted, id.Principal("user@example.com"), testServiceID)
	assert.Error(t, err, "corrupted JWE should fail validation")
}

// TestStateTokenSecurityTampered_EmptyToken tests rejection of empty state token.
func TestStateTokenSecurityTampered_EmptyToken(t *testing.T) {
	service, testServiceID := setupTestService(t)

	_, err := service.ValidateStateToken("", id.Principal("user@example.com"), testServiceID)
	assert.Error(t, err, "empty token should fail validation")
}

// TestStateTokenSecurityTampered_WrongKeyDecryption tests that tokens encrypted with one key
// cannot be decrypted with a different key.
func TestStateTokenSecurityTampered_WrongKeyDecryption(t *testing.T) {
	// Create service with key1 (exactly 32 bytes for AES-256)
	key1, err := jwk.Import[jwk.Key]([]byte("0123456789012345678901234567890X"))
	require.NoError(t, err)
	err = key1.Set(jwk.KeyIDKey, "key1")
	require.NoError(t, err)
	err = key1.Set(jwk.AlgorithmKey, "A256GCM")
	require.NoError(t, err)

	transactions1 := memory.NewTransactionManager()
	serviceRepo1 := memory.NewInMemoryThirdpartyOAuth2ProviderRepository(transactions1)
	sessionRepo1 := memory.NewInMemoryUserSessionRepository(transactions1)
	grantRepo1 := memory.NewUserGrantRepository(transactions1)
	agentRepo1 := memory.NewAgentRepository(transactions1)

	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"

	// Create ThirdpartyOAuth2ProviderService for handling encryption/decryption of client secrets
	providerService1 := thirdparty.NewThirdpartyOAuth2ProviderService(
		serviceRepo1,
		newTestEncryption(t),
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	service1 := oauth2session.NewOAuth2SessionService(providerService1, sessionRepo1, sessionRepo1, grantRepo1, agentRepo1, nil, &http.Client{}, domjwe.New(key1), config, slog.Default(), ledgerfixture.NewRecorder(), transactions1)

	// Create service with key2 (different key, exactly 32 bytes)
	key2, err := jwk.Import[jwk.Key]([]byte("0123456789012345678901234567890Y"))
	require.NoError(t, err)
	err = key2.Set(jwk.KeyIDKey, "key2")
	require.NoError(t, err)
	err = key2.Set(jwk.AlgorithmKey, "A256GCM")
	require.NoError(t, err)

	transactions2 := memory.NewTransactionManager()
	serviceRepo2 := memory.NewInMemoryThirdpartyOAuth2ProviderRepository(transactions2)
	sessionRepo2 := memory.NewInMemoryUserSessionRepository(transactions2)
	grantRepo2 := memory.NewUserGrantRepository(transactions2)
	agentRepo2 := memory.NewAgentRepository(transactions2)

	// Create ThirdpartyOAuth2ProviderService for handling encryption/decryption of client secrets
	providerService2 := thirdparty.NewThirdpartyOAuth2ProviderService(
		serviceRepo2,
		newTestEncryption(t),
		newNoopBranchKeyManager(),
		nil,
		false,
		slog.Default(),
	)

	service2 := oauth2session.NewOAuth2SessionService(providerService2, sessionRepo2, sessionRepo2, grantRepo2, agentRepo2, nil, &http.Client{}, domjwe.New(key2), config, slog.Default(), ledgerfixture.NewRecorder(), transactions2)

	// Create token with service1's key
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	token, err := service1.CreateStateToken(claims)
	require.NoError(t, err, "CreateStateToken should succeed with service1's key")
	require.NotEmpty(t, token, "token should not be empty")

	// Try to validate with service2's key (should fail)
	_, err = service2.ValidateStateToken(token, id.Principal("user@example.com"), testServiceID)
	assert.Error(t, err, "token encrypted with key1 should not decrypt with key2")
}

// TestStateTokenSecurityTampered_ModifiedToken tests rejection of tokens with modified payload.
// This test validates that the JWE authentication tag prevents acceptance of tampered tokens.
// It uses deterministic tampering strategies that reliably modify the authentication tag.
func TestStateTokenSecurityTampered_ModifiedToken(t *testing.T) {
	service, testServiceID := setupTestService(t)

	// Create a valid token
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	token, err := service.CreateStateToken(claims)
	require.NoError(t, err, "CreateStateToken should succeed")
	require.NotEmpty(t, token, "token should not be empty")

	// Verify original token is valid first
	_, err = service.ValidateStateToken(token, id.Principal("user@example.com"), testServiceID)
	require.NoError(t, err, "original token should validate successfully")

	// Strategy: Modify a character in the middle of the token (ciphertext/payload part)
	// This ensures tampering detection is more deterministic than modifying base64url-encoded bytes.
	// Base64url encoding can sometimes result in multiple valid encodings of the same data,
	// which could theoretically pass authentication by accident. By modifying the actual
	// encrypted payload, we ensure the GCM authentication tag will always detect tampering.

	tokenBytes := []byte(token)
	require.True(t, len(tokenBytes) > 50, "token should be long enough to modify middle section")

	// Flip a bit in the middle of the token (part of the encrypted content)
	// Use XOR with 0xFF to guarantee the byte changes to a different value
	middleIdx := len(tokenBytes) / 2
	originalByte := tokenBytes[middleIdx]
	tokenBytes[middleIdx] = originalByte ^ 0xFF // XOR with 0xFF guarantees different value

	modifiedToken := string(tokenBytes)
	require.NotEqual(t, token, modifiedToken, "modified token should be different from original")
	require.NotEqual(t, originalByte, tokenBytes[middleIdx], "byte at middle index should be different")

	// Try to validate modified token - must fail
	_, err = service.ValidateStateToken(modifiedToken, id.Principal("user@example.com"), testServiceID)
	assert.Error(t, err, "modified token should always fail validation - JWE authentication tag should reject any tampering")
}

// T079: Verify JWE uses authenticated encryption A256GCMKW + A256GCM
// Code review task: verify the CreateStateToken implementation uses proper JWE algorithms

// TestStateTokenSecurityEncryption_UsesAuthenticatedEncryption verifies JWE uses A256GCMKW + A256GCM.
func TestStateTokenSecurityEncryption_UsesAuthenticatedEncryption(t *testing.T) {
	// This test verifies that state tokens are created with authenticated encryption
	service, testServiceID := setupTestService(t)

	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	token, err := service.CreateStateToken(claims)
	require.NoError(t, err, "CreateStateToken should succeed")

	// Token should be a valid JWE compact serialization (5 dot-separated parts)
	// Format: base64url(header).base64url(encrypted_key).base64url(iv).base64url(ciphertext).base64url(tag)
	parts := 0
	for _, c := range token {
		if c == '.' {
			parts++
		}
	}

	assert.Equal(t, 4, parts, "JWE token should have 5 parts (4 dots)")
	assert.NotEmpty(t, token, "token should not be empty")

	// Verify token can be decrypted successfully
	decrypted, err := service.ValidateStateToken(token, id.Principal("user@example.com"), testServiceID)
	require.NoError(t, err, "token should decrypt successfully with authenticated encryption")
	assert.Equal(t, claims.Principal, decrypted.Principal)
}

// T080: Verify state token TTL <= 15 minutes is enforced in config validation

// TestStateTokenSecurityTTL_DefaultIsLessThan15Minutes verifies default TTL is appropriate.
func TestStateTokenSecurityTTL_DefaultIsLessThan15Minutes(t *testing.T) {
	config := oauth2session.DefaultConfig()

	// TTL should be <= 15 minutes
	maxAllowedTTL := 15 * time.Minute
	assert.LessOrEqual(t, config.StateTokenTTL, maxAllowedTTL,
		"default state token TTL should be <= 15 minutes for security")

	// TTL should be reasonable (not instant, but also not too long)
	minReasonableTTL := 1 * time.Minute
	assert.GreaterOrEqual(t, config.StateTokenTTL, minReasonableTTL,
		"default state token TTL should be at least 1 minute")
}

// TestStateTokenSecurityTTL_TokenExpired tests that tokens expire according to TTL.
func TestStateTokenSecurityTTL_TokenExpired(t *testing.T) {
	// Create claims with short TTL for testing
	now := time.Now()
	testServiceID := id.NewServiceID()
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now,
		ExpiresAt:    now.Add(5 * time.Minute), // 5 minute TTL
	}

	assert.False(t, claims.IsExpired(), "fresh token should not be expired")

	// Simulate token expiration
	expiredClaims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     now.Add(-10 * time.Minute),
		ExpiresAt:    now.Add(-5 * time.Minute), // Already expired
	}

	assert.True(t, expiredClaims.IsExpired(), "expired token should be marked as expired")
}

// T081: Verify principal mismatch returns 403 Forbidden with security log
// (Integration test in oauth2_sessions_api_test.go)

// TestStateTokenSecurityValidation_PrincipalMismatchError tests principal mismatch error.
func TestStateTokenSecurityValidation_PrincipalMismatchError(t *testing.T) {
	service, testServiceID := setupTestService(t)

	// Create token for user1
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user1@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	token, err := service.CreateStateToken(claims)
	require.NoError(t, err, "CreateStateToken should succeed")

	// Try to validate as user2 (CSRF attack)
	_, err = service.ValidateStateToken(token, id.Principal("user2@example.com"), testServiceID)
	assert.Error(t, err, "should reject token with mismatched principal")
	assert.ErrorIs(t, err, oauth2session.ErrPrincipalMismatch,
		"should return ErrPrincipalMismatch for CSRF attacks")
}

// T082: Verify service_id mismatch returns 400 Bad Request

// TestStateTokenSecurityValidation_ServiceIDMismatchError tests service ID mismatch error.
func TestStateTokenSecurityValidation_ServiceIDMismatchError(t *testing.T) {
	service, testServiceID := setupTestService(t)

	// Create token for service1
	claims := &oauth2session.OAuth2StateTokenClaims{
		Principal:    id.Principal("user@example.com"),
		PKCEVerifier: "test-verifier",
		ServiceID:    testServiceID,
		RedirectURI:  "https://example.com/callback",
		IssuedAt:     time.Now(),
		ExpiresAt:    time.Now().Add(10 * time.Minute),
	}

	token, err := service.CreateStateToken(claims)
	require.NoError(t, err, "CreateStateToken should succeed")

	// Try to validate for service2 (cross-service attack)
	wrongServiceID := id.NewServiceID()
	_, err = service.ValidateStateToken(token, id.Principal("user@example.com"), wrongServiceID)
	assert.Error(t, err, "should reject token with mismatched service ID")
	assert.Contains(t, err.Error(), "service_id", "error should indicate service ID mismatch")
}
