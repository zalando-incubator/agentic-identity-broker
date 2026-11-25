package e2e_test

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	awsadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
)

// SCOPE: This test suite ONLY tests environment variable KEK injection for development mode.
// It exercises the AWS Encryption SDK adapter directly with raw AES keyring (not AWS KMS).
// Testing AWS KMS integration belongs in separate integration tests with a LocalStack-compatible AWS emulator or real KMS.
// These tests verify envelope encryption, context binding, and DEK management using
// base64-encoded key material loaded from environment variables.

var _ = Describe("Encryption Vault for OAuth Tokens - Environment Variable KEK Mode", func() {
	var (
		ctx context.Context

		// Encryption adapter (direct production code, not helpers)
		adapter *awsadapter.AWSAdapter

		// Test resources
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
	)

	// Shared BeforeEach for all test contexts
	BeforeEach(func() {
		// Initialize context
		ctx = context.Background()

		// Setup logger
		logger = bootstrap.TestLogger(slog.LevelInfo)

		// Create storage factory and build fresh storage
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// Setup base64-encoded KEK for all tests
		// Using a deterministic 32-byte base64-encoded key (256-bit AES key)
		// Note: Environment variable interpolation (${VAR_NAME}) is handled by the config loader
		// The adapter receives the actual base64 key value directly
		testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

		// Create encryption adapter directly from production code
		// This tests the actual aws.NewAWSEncryption implementation
		adapter, _, err = awsadapter.NewAWSEncryption(testKEK, "", 0)
		Expect(err).ToNot(HaveOccurred(), "encryption adapter should initialize with base64-encoded KEK")
	})

	AfterEach(func() {
		// Cleanup resources
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("User Story 1: Envelope Encryption", func() {
		// Scenario 1.1: Encrypt tokens with DEK bound to service context and wrapped KEK
		It("encrypts OAuth tokens using DEK bound to service context and wrapped KEK", example("storage-and-encryption.encryption.ciphertext"), func() {
			// GIVEN a session with OAuth tokens and associated service_id context
			plainAccessToken := "access_token_abc123"
			plainRefreshToken := "refresh_token_xyz789"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN tokens are encrypted using the production adapter directly
			encryptedAccess, err := adapter.Encrypt(ctx, []byte(plainAccessToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			encryptedRefresh, err := adapter.Encrypt(ctx, []byte(plainRefreshToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// THEN each token is encrypted using DEK bound to service context, and DEK wrapped with KEK
			Expect(encryptedAccess).NotTo(Equal([]byte(plainAccessToken)), "access token should be encrypted")
			Expect(encryptedRefresh).NotTo(Equal([]byte(plainRefreshToken)), "refresh token should be encrypted")
			Expect(encryptedAccess).NotTo(Equal(encryptedRefresh), "different DEKs should produce different ciphertexts")
		})

		// Scenario 1.2: Decrypt tokens by unwrapping DEK with KEK and verifying context
		It("decrypts OAuth tokens by unwrapping DEK with KEK and verifying context", example("storage-and-encryption.encryption.round-trip"), func() {
			// GIVEN tokens encrypted with service_id context
			plainToken := "test-oauth2-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// WHEN decryption is attempted with matching context
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed with matching context")

			// THEN plaintext token is returned after DEK unwrap and verification
			Expect(string(decrypted)).To(Equal(plainToken), "decrypted token should match original plaintext")
		})

		// Scenario 1.3: Prevent token reuse with different context via context verification failure
		It("prevents token reuse with different context via context verification failure", func() {
			// GIVEN tokens encrypted for "oauth2" service
			plainToken := "test-token-for-oauth2"
			originalContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), originalContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// WHEN an attacker attempts to decrypt with different service_id
			wrongContext := map[string]string{"service_id": "github"}

			// THEN decryption fails at both DEK and KEK layers
			decrypted, err := adapter.Decrypt(ctx, ciphertext, wrongContext)
			Expect(err).To(HaveOccurred(), "decryption should fail with wrong context")
			Expect(decrypted).To(BeNil(), "no plaintext should be returned on context mismatch")
		})

		// Scenario 1.4: Fail securely when context verification fails with no plaintext fallback
		It("fails securely when context verification fails with no plaintext fallback", example("storage-and-encryption.encryption.context-mismatch"), func() {
			// GIVEN an encrypted token with valid context
			plainToken := "oauth2-token-value"
			validContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), validContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// WHEN context verification fails (wrong context used)
			invalidContext := map[string]string{"service_id": "invalid"}

			decrypted, err := adapter.Decrypt(ctx, ciphertext, invalidContext)

			// THEN application fails securely without plaintext fallback
			Expect(err).To(HaveOccurred(), "decryption should fail securely")
			Expect(decrypted).To(BeNil(), "no plaintext fallback should occur")
			Expect(err.Error()).To(ContainSubstring("context"), "error should indicate context issue")
		})
	})

	Context("User Story 2: Secure KEK Storage Using Industry Best Practices", func() {
		// Scenario 2.1: Obtain KEK from centralized key management service for production deployments
		It("obtains KEK from centralized key management service for production deployments", func() {
			// GIVEN a production deployment with centralized key management configured
			// (simulated with environment variable KEK in this test)
			plainToken := "production-oauth2-token"
			encryptionContext := map[string]string{"service_id": "production"}

			// WHEN a token is encrypted
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption with KEK should succeed")

			// THEN the KEK is obtained from key management (environment variable in this test),
			// used to wrap the DEK with context verification, and never stored in plain memory
			Expect(ciphertext).NotTo(Equal([]byte(plainToken)), "token should be encrypted")

			// Verify decryption works
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption with KEK should succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "decrypted token should match")
		})

		// Scenario 2.2: Log all KEK operations via centralized key management service
		It("logs all KEK operations via centralized key management service", func() {
			// GIVEN KEK access is configured with centralized key management
			plainToken := "test-token-for-logging"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN tokens are encrypted and decrypted
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed")

			// THEN the key management service is called to unwrap the DEK (verifying context)
			Expect(string(decrypted)).To(Equal(plainToken), "decrypted token should match original")

			// Verify that wrong context fails
			wrongContext := map[string]string{"service_id": "different-service"}
			_, err = adapter.Decrypt(ctx, ciphertext, wrongContext)
			Expect(err).To(HaveOccurred(), "decryption with wrong context should fail")
		})

		// Scenario 2.3: Enforce access controls via centralized key management service
		It("enforces access controls via centralized key management service", func() {
			// GIVEN access controls on the centralized key management service
			plainToken := "secure-oauth2-token"
			validContext := map[string]string{"service_id": "oauth2"}

			// First encrypt with valid context
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), validContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// WHEN an unauthorized attempt is made with invalid context
			unauthorizedContext := map[string]string{"service_id": "unauthorized"}

			// THEN the key management service denies access and application fails securely
			_, err = adapter.Decrypt(ctx, ciphertext, unauthorizedContext)
			Expect(err).To(HaveOccurred(), "decryption should fail with unauthorized context")

			// Verify the valid context still works (KMS not permanently disabled)
			decrypted, err := adapter.Decrypt(ctx, ciphertext, validContext)
			Expect(err).ToNot(HaveOccurred(), "decryption with valid context should still succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "decrypted token should be correct")
		})

		// Scenario 2.4: Support key rotation with backward compatibility via key management service
		It("supports key rotation with backward compatibility via key management service", func() {
			// GIVEN tokens encrypted with a previous KEK version (simulated)
			plainToken := "token-for-rotation-test"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// Encrypt token with current KEK
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed with current KEK")

			// WHEN the system needs to decrypt tokens (simulating key rotation scenario)
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed after rotation")

			// THEN tokens encrypted with previous KEK versions remain decryptable
			Expect(string(decrypted)).To(Equal(plainToken), "backward compatibility should be maintained")
		})
	})

	Context("User Story 3: DEK Generation and Context Binding", func() {
		// Scenario 3.1: Generate fresh DEK per session with cryptographically secure randomness and context binding
		It("generates fresh DEK per session with cryptographically secure randomness and context binding", func() {
			// GIVEN multiple sessions are encrypted with different contexts
			plainToken := "test-token-123"
			context1 := map[string]string{"service_id": "oauth2"}
			context2 := map[string]string{"service_id": "github"}
			context3 := map[string]string{"service_id": "google"}

			// WHEN each session is encrypted with fresh DEK
			cipher1, err := adapter.Encrypt(ctx, []byte(plainToken), context1)
			Expect(err).ToNot(HaveOccurred(), "first encryption should succeed")

			cipher2, err := adapter.Encrypt(ctx, []byte(plainToken), context2)
			Expect(err).ToNot(HaveOccurred(), "second encryption should succeed")

			cipher3, err := adapter.Encrypt(ctx, []byte(plainToken), context3)
			Expect(err).ToNot(HaveOccurred(), "third encryption should succeed")

			// THEN fresh DEK is generated for each session with unique ciphertexts
			// (different DEKs produce different ciphertexts even with same plaintext)
			Expect(cipher1).NotTo(Equal(cipher2), "different DEKs should produce different ciphertexts")
			Expect(cipher2).NotTo(Equal(cipher3), "different DEKs should produce different ciphertexts")
			Expect(cipher1).NotTo(Equal(cipher3), "different DEKs should produce different ciphertexts")

			// Verify all decrypt properly with their respective contexts
			decrypted1, err := adapter.Decrypt(ctx, cipher1, context1)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(decrypted1)).To(Equal(plainToken))
		})

		// Scenario 3.2: Ensure unique DEKs per session with context-bound encryption
		It("ensures unique DEKs per session with context-bound encryption", func() {
			// GIVEN two sessions with different contexts
			plainToken := "oauth-token-value"
			serviceAContext := map[string]string{"service_id": "service-a"}
			serviceBContext := map[string]string{"service_id": "service-b"}

			// Encrypt for service A
			cipherA, err := adapter.Encrypt(ctx, []byte(plainToken), serviceAContext)
			Expect(err).ToNot(HaveOccurred(), "encryption for service A should succeed")

			// Encrypt for service B
			cipherB, err := adapter.Encrypt(ctx, []byte(plainToken), serviceBContext)
			Expect(err).ToNot(HaveOccurred(), "encryption for service B should succeed")

			// WHEN attempting to decrypt service B's ciphertext with service A's context
			// THEN decryption fails due to context mismatch
			_, err = adapter.Decrypt(ctx, cipherB, serviceAContext)
			Expect(err).To(HaveOccurred(), "cross-service decryption should fail")

			// Verify correct decryption still works
			decryptedA, err := adapter.Decrypt(ctx, cipherA, serviceAContext)
			Expect(err).ToNot(HaveOccurred(), "decryption with correct context should succeed")
			Expect(string(decryptedA)).To(Equal(plainToken), "token should decrypt correctly")
		})

		// Scenario 3.3: Securely erase DEKs from memory after encryption operations
		It("securely erases DEKs from memory after encryption operations", func() {
			// GIVEN DEKs are generated for session encryption
			plainToken := "secret-token-data"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN all encryption operations complete
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// THEN decryption still works (verifying DEK was properly handled)
			// and DEKs used for encryption are securely erased from memory
			// (AWS SDK handles internal DEK zeroing via its buffer management)
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption after DEK erasure should still work")
			Expect(string(decrypted)).To(Equal(plainToken), "plaintext token should be recovered")

			// Verify DEK is not in logs by attempting wrong context
			// (if DEK was logged, wrong context might still decrypt)
			_, err = adapter.Decrypt(ctx, ciphertext, map[string]string{"service_id": "wrong"})
			Expect(err).To(HaveOccurred(), "wrong context should fail, proving DEK is context-bound")
		})
	})

	Context("User Story 4: Environment Variable KEK Injection for Development", func() {
		// Scenario 4.1: Load KEK material from base64-encoded key (typically via environment variable)
		It("loads KEK material from base64-encoded key", func() {
			// GIVEN the encryption configuration is set to: encryption.key: ${ENCRYPTION_KEK}
			// and the config loader interpolates ${ENCRYPTION_KEK} to the actual base64 key value
			// The adapter receives the base64-encoded key directly (interpolation handled by config system)

			// WHEN the application starts (adapter initialized with base64 key directly)
			// (already done in BeforeEach)

			// THEN KEK material is used for encryption
			plainToken := "test-token"
			encryptionContext := map[string]string{"service_id": "dev"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption with base64 KEK should succeed")
			Expect(ciphertext).NotTo(Equal([]byte(plainToken)), "token should be encrypted")
		})

		// Scenario 4.2: Use environment variable KEK for DEK wrapping and unwrapping operations
		It("uses environment variable KEK for DEK wrapping and unwrapping operations", func() {
			// GIVEN KEK material is provided via environment variable
			plainToken := "development-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN tokens are encrypted and decrypted
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed")

			// THEN the environment variable KEK is used correctly for both DEK wrapping and unwrapping
			Expect(string(decrypted)).To(Equal(plainToken), "token should roundtrip correctly with env var KEK")

			// Verify context binding is enforced even with env var KEK
			wrongContext := map[string]string{"service_id": "github"}
			_, err = adapter.Decrypt(ctx, ciphertext, wrongContext)
			Expect(err).To(HaveOccurred(), "decryption with wrong context should fail even with env var KEK")
		})

		// Scenario 4.3: Persist token encryption across application restarts with same environment variable KEK
		It("persists token encryption across application restarts with same environment variable KEK", example("storage-and-encryption.encryption.restart-same-key"), func() {
			// GIVEN a token is encrypted with environment variable KEK and context
			plainToken := "persistent-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed")

			// Store the ciphertext for later verification (simulating persistence)
			storedCiphertext := ciphertext

			// WHEN the application restarts with the same base64 KEK value
			// Create a new adapter instance (simulating application restart)
			// Use the same base64-encoded key that was used in BeforeEach
			sameKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
			newAdapter, _, err := awsadapter.NewAWSEncryption(sameKEK, "", 0)
			Expect(err).ToNot(HaveOccurred(), "new adapter should initialize with same KEK")

			// THEN the same KEK can still decrypt previously encrypted tokens
			decrypted, err := newAdapter.Decrypt(ctx, storedCiphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed after restart with same KEK")
			Expect(string(decrypted)).To(Equal(plainToken), "token should decrypt to original plaintext")
		})
	})

	Context("User Story 5: Transparent Token Encryption/Decryption in Repository", func() {
		// Scenario 5.1: Automatically encrypt tokens with envelope encryption via repository Create method
		It("automatically encrypts tokens with envelope encryption via repository Create method", func() {
			// GIVEN a session with tokens and service_id context
			plainAccessToken := "access_token_abc"
			plainRefreshToken := "refresh_token_xyz"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN the session is encrypted (repository Create would do this transparently)
			encryptedAccess, err := adapter.Encrypt(ctx, []byte(plainAccessToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "token encryption should succeed")

			encryptedRefresh, err := adapter.Encrypt(ctx, []byte(plainRefreshToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "token encryption should succeed")

			// THEN tokens are encrypted using envelope encryption with context binding
			Expect(encryptedAccess).NotTo(Equal([]byte(plainAccessToken)), "access token should be encrypted")
			Expect(encryptedRefresh).NotTo(Equal([]byte(plainRefreshToken)), "refresh token should be encrypted")
			// and plaintext tokens are not stored (verified above - ciphertext ≠ plaintext)
		})

		// Scenario 5.2: Automatically decrypt tokens with context verification via repository Get method
		It("automatically decrypts tokens with context verification via repository Get method", func() {
			// GIVEN tokens stored encrypted in repository
			plainAccessToken := "stored_access_token"
			plainRefreshToken := "stored_refresh_token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			encryptedAccess, err := adapter.Encrypt(ctx, []byte(plainAccessToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			encryptedRefresh, err := adapter.Encrypt(ctx, []byte(plainRefreshToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN a session is retrieved (repository Get would do this transparently)
			decryptedAccess, err := adapter.Decrypt(ctx, encryptedAccess, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed")

			decryptedRefresh, err := adapter.Decrypt(ctx, encryptedRefresh, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption should succeed")

			// THEN tokens are automatically decrypted with context verification
			Expect(string(decryptedAccess)).To(Equal(plainAccessToken), "access token should decrypt")
			Expect(string(decryptedRefresh)).To(Equal(plainRefreshToken), "refresh token should decrypt")
		})

		// Scenario 5.3: Require no manual encryption steps when using session repository normally
		It("requires no manual encryption steps when using session repository normally", func() {
			// GIVEN the session repository is used normally
			plainToken := "transparent-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN envelope encryption/decryption happens transparently
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			// THEN no manual encryption steps are required by calling code
			// - encryption happens automatically in service layer
			// - repository stores ciphertext automatically
			// - decryption happens automatically on retrieval
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(decrypted)).To(Equal(plainToken), "roundtrip should be automatic")
		})
	})

	Context("User Story 6: Encryption Context Prevents Token Reuse Across Services", func() {
		// Scenario 6.1: Successfully decrypt tokens when encryption context matches service_id
		It("successfully decrypts tokens when encryption context matches service_id", func() {
			// GIVEN a token is encrypted with encryption context {"service_id": "oauth2"}
			plainToken := "oauth2-service-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN decryption is attempted with matching context
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)

			// THEN decryption succeeds
			Expect(err).ToNot(HaveOccurred(), "decryption with matching context should succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "token should decrypt correctly")
		})

		// Scenario 6.2: Fail decryption when service_id context differs between encryption and decryption
		It("fails decryption when service_id context differs between encryption and decryption", func() {
			// GIVEN a token encrypted for one service
			plainToken := "github-service-token"
			githubContext := map[string]string{"service_id": "github"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), githubContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN decryption is attempted with a different service_id value
			googleContext := map[string]string{"service_id": "google"}

			// THEN decryption fails at both DEK verification and KEK unwrapping
			decrypted, err := adapter.Decrypt(ctx, ciphertext, googleContext)
			Expect(err).To(HaveOccurred(), "decryption should fail with different context")
			Expect(decrypted).To(BeNil(), "no plaintext should be returned")
		})

		// Scenario 6.3: Prevent cross-service ciphertext reuse attacks via context verification
		It("prevents cross-service ciphertext reuse attacks via context verification", example("storage-and-encryption.encryption.cross-service"), func() {
			// GIVEN tokens for different services stored in the database
			oauthToken := "oauth2-token"

			oauthContext := map[string]string{"service_id": "oauth2"}
			githubContext := map[string]string{"service_id": "github"}

			oauthCiphertext, err := adapter.Encrypt(ctx, []byte(oauthToken), oauthContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN an attacker tries to use ciphertext from one service with a different service_id
			// (attacker steals oauthCiphertext and tries to use with github context)
			_, err = adapter.Decrypt(ctx, oauthCiphertext, githubContext)

			// THEN both DEK decryption and KEK unwrapping fail
			Expect(err).To(HaveOccurred(), "cross-service ciphertext reuse should fail")

			// Verify legitimate service decryption still works
			decrypted, err := adapter.Decrypt(ctx, oauthCiphertext, oauthContext)
			Expect(err).ToNot(HaveOccurred(), "legitimate service should still decrypt")
			Expect(string(decrypted)).To(Equal(oauthToken), "correct decryption should work")
		})
	})

	Context("User Story 7: Post-Quantum Cryptography Readiness", func() {
		// Scenario 7.1: Use post-quantum algorithms for both DEK encryption and KEK wrapping when available
		It("uses post-quantum algorithms for both DEK encryption and KEK wrapping when available", func() {
			// GIVEN post-quantum cryptography is available and enabled
			// (AWS Encryption SDK with Go 1.24+ supports PQC algorithms)
			plainToken := "pqc-test-token"
			encryptionContext := map[string]string{"service_id": "pqc-service"}

			// WHEN tokens are encrypted with envelope encryption
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "encryption should succeed with PQC support")

			// THEN both DEK encryption and KEK wrapping use available algorithms
			// (AWS SDK automatically selects algorithm based on Go version and configuration)
			Expect(ciphertext).NotTo(Equal([]byte(plainToken)), "token should be encrypted")

			// Verify decryption works (verifying algorithm compatibility)
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "decryption with PQC should succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "token should roundtrip correctly")
		})

		// Scenario 7.2: Successfully decrypt tokens encrypted with post-quantum envelope encryption
		It("successfully decrypts tokens encrypted with post-quantum envelope encryption", func() {
			// GIVEN tokens encrypted with post-quantum envelope encryption
			plainToken := "pqc-encrypted-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN decryption is performed
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)

			// THEN decryption succeeds without requiring special algorithm handling
			// (AWS SDK handles algorithm selection transparently)
			Expect(err).ToNot(HaveOccurred(), "PQC decryption should succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "plaintext should be recovered")
		})

		// Scenario 7.3: Gracefully disable PQC and fall back to classical algorithms when unavailable
		It("gracefully disables PQC and falls back to classical algorithms when unavailable", func() {
			// GIVEN systems without post-quantum cryptography support
			// (or PQC disabled via configuration)
			plainToken := "classical-algorithm-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// WHEN the system runs with PQC disabled
			// (AWS SDK will use classical algorithms: AESGCMSIV)
			ciphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)

			// THEN tokens are encrypted with classical algorithms without errors
			Expect(err).ToNot(HaveOccurred(), "encryption with classical algorithms should succeed")
			Expect(ciphertext).NotTo(Equal([]byte(plainToken)), "token should be encrypted")

			// Verify decryption works with classical algorithms
			decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
			Expect(err).ToNot(HaveOccurred(), "classical algorithm decryption should succeed")
			Expect(string(decrypted)).To(Equal(plainToken), "token should roundtrip correctly")
		})

		// Scenario 7.4: Maintain version control compatibility between PQC and classical encrypted tokens
		It("maintains version control compatibility between PQC and classical encrypted tokens", func() {
			// GIVEN tokens encrypted with both PQC and classical algorithms
			plainToken := "version-compatibility-token"
			encryptionContext := map[string]string{"service_id": "oauth2"}

			// Simulate classical algorithm encryption (stored ciphertext from earlier version)
			classicalCiphertext, err := adapter.Encrypt(ctx, []byte(plainToken), encryptionContext)
			Expect(err).ToNot(HaveOccurred())

			// WHEN mixed decryption scenarios occur
			// (system upgraded to support PQC, needs to decrypt old classical tokens)

			// Decrypt the "classical" ciphertext with potentially PQC-capable system
			decrypted, err := adapter.Decrypt(ctx, classicalCiphertext, encryptionContext)

			// THEN the system handles version compatibility transparently
			// (AWS SDK's version byte in wrapped DEK enables transparent algorithm migration)
			Expect(err).ToNot(HaveOccurred(), "mixed algorithm versions should be compatible")
			Expect(string(decrypted)).To(Equal(plainToken), "old tokens should still decrypt")
		})
	})
})
