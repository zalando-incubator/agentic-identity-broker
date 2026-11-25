package fixtures

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"time"

	"github.com/google/uuid"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

// TestSessionData represents test data for OAuth2 sessions with known values
// for encryption vault testing.
type TestSessionData struct {
	Principal     string
	ServiceID     uuid.UUID
	AccessToken   string
	RefreshToken  string
	EncryptionCtx map[string]string
}

// TestServices returns test service IDs for encryption vault testing
func TestServices() map[string]uuid.UUID {
	return map[string]uuid.UUID{
		"oauth2": uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef"),
		"github": uuid.MustParse("12345678-9abc-def0-1234-56789abcdef0"),
		"google": uuid.MustParse("23456789-abcd-ef01-2345-6789abcdef01"),
	}
}

// TestSessionWithService creates a test session with known values for a specific service
func TestSessionWithService(serviceName string) *TestSessionData {
	services := TestServices()
	serviceID := services[serviceName]

	return &TestSessionData{
		Principal:    "test-user@example.com",
		ServiceID:    serviceID,
		AccessToken:  "test-access-token-" + serviceName + "-" + generateRandomSuffix(8),
		RefreshToken: "test-refresh-token-" + serviceName + "-" + generateRandomSuffix(8),
		EncryptionCtx: map[string]string{
			"service_id": serviceID.String(),
		},
	}
}

// TestSessionForPrincipal creates a test session for a specific principal
func TestSessionForPrincipal(principal, serviceName string) *TestSessionData {
	services := TestServices()
	serviceID := services[serviceName]

	return &TestSessionData{
		Principal:    principal,
		ServiceID:    serviceID,
		AccessToken:  "test-access-token-" + principal + "-" + serviceName + "-" + generateRandomSuffix(8),
		RefreshToken: "test-refresh-token-" + principal + "-" + serviceName + "-" + generateRandomSuffix(8),
		EncryptionCtx: map[string]string{
			"service_id": serviceID.String(),
		},
	}
}

// GetSessionData returns the session data for use in tests
// Conversion to domain objects will be handled by test helpers in Phase 4-10
func (tsd *TestSessionData) GetSessionData() (string, uuid.UUID, string, string, map[string]string) {
	return tsd.Principal, tsd.ServiceID, tsd.AccessToken, tsd.RefreshToken, tsd.EncryptionCtx
}

// TestKEKMaterial provides base64-encoded KEK material for testing environment variable injection
func TestKEKMaterial() string {
	// Generate 32 bytes of random KEK material for AES-256
	kekBytes := make([]byte, 32)
	_, err := rand.Read(kekBytes)
	if err != nil {
		panic("Failed to generate test KEK material: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(kekBytes)
}

// TestKEKMaterialDeterministic returns deterministic KEK material for tests
// that need the same KEK across test runs.
// Delegates to testutil.TestKEKBase64 — single source of truth, no independent encoding.
func TestKEKMaterialDeterministic() string {
	return testutil.TestKEKBase64
}

// TestEncryptionContexts provides various encryption contexts for testing
func TestEncryptionContexts() map[string]map[string]string {
	services := TestServices()
	return map[string]map[string]string{
		"oauth2":          {"service_id": services["oauth2"].String()},
		"github":          {"service_id": services["github"].String()},
		"google":          {"service_id": services["google"].String()},
		"wrong_context":   {"service_id": "wrong_service"},
		"empty_context":   {},
		"invalid_context": {"wrong_key": "wrong_value"},
	}
}

// TestTokenPairs provides known token pairs for encryption testing
func TestTokenPairs() map[string][2]string {
	return map[string][2]string{
		"oauth2": {
			"gho_test_oauth2_access_token_123456789abcdef",
			"gho_test_oauth2_refresh_token_abcdef123456789",
		},
		"github": {
			"ghp_test_github_access_token_987654321fedcba",
			"ghr_test_github_refresh_token_fedcba987654321",
		},
		"google": {
			"ya29.test_google_access_token_456789abcdef123",
			"1//test_google_refresh_token_123456789abcdef",
		},
	}
}

// EncryptedToken encrypts a token string using the deterministic test key with service_id context.
// Use this for EncryptedAccessToken and EncryptedRefreshToken fields in storage.UserSession fixtures
// that are stored directly in the storage layer (bypassing the domain service).
// The serviceID must match the session's ServiceID field.
func EncryptedToken(serviceID, token string) []byte {
	adapter, _, err := awsencryption.NewAWSEncryption(TestKEKMaterialDeterministic(), "", 0, slog.New(slog.DiscardHandler))
	if err != nil {
		panic("fixtures.EncryptedToken: failed to create encryption adapter: " + err.Error())
	}
	ciphertext, err := adapter.Encrypt(context.Background(), []byte(token), map[string]string{"service_id": serviceID})
	if err != nil {
		panic("fixtures.EncryptedToken: failed to encrypt: " + err.Error())
	}
	return ciphertext
}

// EncryptedSecret creates a properly encrypted model.Secret using the deterministic test key.
// The secret is encrypted with the same key used by DefaultOAuth2Config(), ensuring that
// services stored via testStorage.Services().Create() can be decrypted by the application.
//
// Use this in fixtures that return entities stored directly in the storage layer (bypassing
// the domain service Create method). The serviceID must match the entity's ID field.
func EncryptedSecret(serviceID, plaintext string) model.Secret {
	adapter, _, err := awsencryption.NewAWSEncryption(TestKEKMaterialDeterministic(), "", 0, slog.New(slog.DiscardHandler))
	if err != nil {
		panic("fixtures.EncryptedSecret: failed to create encryption adapter: " + err.Error())
	}
	ciphertext, err := adapter.Encrypt(context.Background(), []byte(plaintext), map[string]string{"service_id": serviceID})
	if err != nil {
		panic("fixtures.EncryptedSecret: failed to encrypt: " + err.Error())
	}
	return model.NewEncryptedSecret(ciphertext)
}

// generateRandomSuffix generates a random suffix for test data uniqueness
func generateRandomSuffix(length int) string {
	bytes := make([]byte, length)
	_, err := rand.Read(bytes)
	if err != nil {
		// Fallback to timestamp-based suffix if rand fails
		return time.Now().Format("150405")
	}
	return base64.RawURLEncoding.EncodeToString(bytes)[:length]
}
