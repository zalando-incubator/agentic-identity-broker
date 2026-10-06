package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mpltypes "github.com/aws/aws-cryptographic-material-providers-library/releases/go/mpl/awscryptographymaterialproviderssmithygeneratedtypes"
	esdktypes "github.com/aws/aws-encryption-sdk/releases/go/encryption-sdk/awscryptographyencryptionsdksmithygeneratedtypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/branchkey"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
)

// TestNewAWSEncryptionWithBase64KEK tests adapter creation with base64-encoded KEK
func TestNewAWSEncryptionWithBase64KEK(t *testing.T) {
	// Setup: Create test KEK material (base64-encoded 32 bytes)
	// Note: Environment variable interpolation (${VAR_NAME}) is handled by config loader
	// This test passes the base64 key directly, as the adapter expects
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

	// Test: Create adapter with base64-encoded key directly
	adapter, _, err := NewAWSEncryption(testKEK, "", 0)

	// Verify: Adapter created successfully
	if err != nil {
		t.Fatalf("failed to create adapter with base64 KEK: %v", err)
	}
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}
}

// TestNewAWSEncryptionWithEmptyKEK tests adapter creation fails when empty base64 key provided
func TestNewAWSEncryptionWithEmptyKEK(t *testing.T) {
	// Test: Try to create adapter with empty key material
	adapter, _, err := NewAWSEncryption("", "", 0)

	// Verify: Error returned
	if err == nil {
		t.Fatal("expected error when key material is empty")
	}
	if adapter != nil {
		t.Fatal("adapter should be nil when key material is empty")
	}

	// Verify: Error is KEKUnavailable type
	var kekErr *encryption.EncryptionError
	if !isKEKUnavailableError(err) {
		t.Errorf("expected KEKUnavailable error, got: %v", err)
	}
	_ = kekErr // silence unused
}

// TestNewAWSEncryptionWithInvalidBase64Format tests adapter rejects invalid base64 KEK
func TestNewAWSEncryptionWithInvalidBase64Format(t *testing.T) {
	// Test: Try to create adapter with invalid base64 key
	adapter, _, err := NewAWSEncryption("not-valid-base64!!!", "", 0)

	// Verify: Error returned
	if err == nil {
		t.Fatal("expected error for invalid base64 KEK")
	}
	if adapter != nil {
		t.Fatal("adapter should be nil for invalid base64 KEK")
	}

	// Verify: Error is KEKUnavailable type
	if !isKEKUnavailableError(err) {
		t.Errorf("expected KEKUnavailable error, got: %v", err)
	}
}

// TestNewAWSEncryptionWithInvalidKEKLength tests adapter rejects KEK with wrong length
func TestNewAWSEncryptionWithInvalidKEKLength(t *testing.T) {
	// Test: Try to create adapter with 16-byte KEK (too short, need 32)
	// "SGVsbG8gV29ybGQgSGVsbG8gV29ybGQ=" is 16 bytes base64-encoded
	shortKEK := "SGVsbG8gV29ybGQgSGVsbG8gV29ybGQ="
	adapter, _, err := NewAWSEncryption(shortKEK, "", 0)

	// Verify: Error returned
	if err == nil {
		t.Fatal("expected error for 16-byte KEK (need 32)")
	}
	if adapter != nil {
		t.Fatal("adapter should be nil for invalid KEK length")
	}

	// Verify: Error is KEKUnavailable type
	if !isKEKUnavailableError(err) {
		t.Errorf("expected KEKUnavailable error, got: %v", err)
	}
}

// TestNewAWSEncryptionWithInvalidFormat tests adapter rejects invalid key material format
func TestNewAWSEncryptionWithInvalidFormat(t *testing.T) {
	tests := []struct {
		name        string
		keyMaterial string
	}{
		{"empty string", ""},
		{"invalid format", "some-invalid-key"},
		{"partial KMS ARN", "arn:aws:kms:"},
		{"partial env var", "${INCOMPLETE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, _, err := NewAWSEncryption(tt.keyMaterial, "", 0)

			if err == nil {
				t.Fatal("expected error for invalid format")
			}
			if adapter != nil {
				t.Fatal("adapter should be nil for invalid format")
			}

			if !isKEKUnavailableError(err) {
				t.Errorf("expected KEKUnavailable error, got: %v", err)
			}
		})
	}
}

// TestEncryptDecryptRoundtrip tests encryption and decryption of tokens
func TestEncryptDecryptRoundtrip(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_ROUNDTRIP", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_ROUNDTRIP environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_ROUNDTRIP") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("test-oauth2-token-value")
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Test: Encrypt
	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Verify: Ciphertext is not empty
	if len(ciphertext) == 0 {
		t.Fatal("ciphertext should not be empty")
	}

	// Verify: Ciphertext is different from plaintext
	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext should be different from plaintext")
	}

	// Test: Decrypt
	decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	// Verify: Decrypted matches original plaintext
	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted plaintext mismatch: expected %q, got %q", string(plaintext), string(decrypted))
	}
}

// Both adapters use the same Dafny runtime globals, including the empty sequence
// modified while the signed ESDK messages are built.
func TestConcurrentTwoAdapterRoundTrips(t *testing.T) {
	const testKEK = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	first, _, err := NewAWSEncryption(testKEK, "", 0)
	require.NoError(t, err)
	second, _, err := NewAWSEncryption(testKEK, "", 0)
	require.NoError(t, err)

	const workers = 12
	start := make(chan struct{})
	failures := make(chan error, workers)
	var workersDone sync.WaitGroup
	for i := range workers {
		workersDone.Add(1)
		go func(i int) {
			defer workersDone.Done()
			<-start
			encryptor, decryptor := first, second
			if i%2 == 1 {
				encryptor, decryptor = second, first
			}
			plaintext := []byte(fmt.Sprintf("refresh-secret-%d", i))
			aad := map[string]string{"service_id": fmt.Sprintf("service-%d", i)}
			ciphertext, err := encryptor.Encrypt(context.Background(), plaintext, aad)
			if err != nil {
				failures <- fmt.Errorf("worker %d encrypt: %w", i, err)
				return
			}
			recovered, err := decryptor.Decrypt(context.Background(), ciphertext, aad)
			if err != nil {
				failures <- fmt.Errorf("worker %d decrypt: %w", i, err)
				return
			}
			if string(recovered) != string(plaintext) {
				failures <- fmt.Errorf("worker %d decrypted %q instead of %q", i, recovered, plaintext)
			}
		}(i)
	}
	close(start)
	workersDone.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}

// TestContextMismatchDetection tests that wrong context fails decryption
func TestContextMismatchDetection(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_CONTEXT", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_CONTEXT environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_CONTEXT") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("test-oauth2-token")

	// Encrypt with service_id "oauth2"
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}
	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Test: Try to decrypt with different service_id "github"
	wrongContext := map[string]string{
		"service_id": "github",
	}
	decrypted, err := adapter.Decrypt(ctx, ciphertext, wrongContext)

	// Verify: Decryption fails
	if err == nil {
		t.Fatal("expected decryption to fail with wrong context")
	}

	// Verify: Error is ContextMismatch type
	if !isContextMismatchError(err) {
		t.Errorf("expected ContextMismatch error, got: %v", err)
	}

	// Verify: Decrypted is nil or empty
	if len(decrypted) > 0 {
		t.Fatal("decrypted should be empty/nil on context mismatch")
	}
}

func TestRefreshSessionAADWithRawAWSKeyring(t *testing.T) {
	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	require.NoError(t, err)

	ctx := context.Background()
	plaintext := []byte("refresh-result-fixture")
	root := map[string]string{encryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440001"}
	ciphertext, err := adapter.Encrypt(ctx, plaintext, root)
	require.NoError(t, err)

	recovered, err := adapter.Decrypt(ctx, ciphertext, root)
	require.NoError(t, err)
	assert.Equal(t, plaintext, recovered)

	t.Run("SDK-returned public key metadata does not change routing", func(t *testing.T) {
		require.NoError(t, lockSDK(ctx))
		result, err := adapter.encryptionClient.Decrypt(ctx, esdktypes.DecryptInput{
			Ciphertext:        ciphertext,
			EncryptionContext: root,
			Keyring:           adapter.keyring,
		})
		unlockSDK()
		require.NoError(t, err)
		require.NotNil(t, result)
		if _, ok := result.EncryptionContext["aws-crypto-public-key"]; !ok {
			t.Fatal("SDK did not return signing public key metadata")
		}
		supplier := NewBranchKeyIdSupplier(branchkey.NewDefaultProvider())
		output, err := supplier.GetBranchKeyId(mpltypes.GetBranchKeyIdInput{EncryptionContext: result.EncryptionContext})
		require.NoError(t, err)
		require.NotNil(t, output)
		assert.Equal(t, "refresh_550e8400-e29b-41d4-a716-446655440001_branch_key", output.BranchKeyId)
	})

	for _, tt := range []struct {
		name    string
		context map[string]string
	}{
		{"another refresh session", map[string]string{encryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440002"}},
		{"another subject namespace", map[string]string{encryption.ContextKeyServiceID: "550e8400-e29b-41d4-a716-446655440001"}},
		{"missing root subject", map[string]string{}},
		{"mixed root subjects", map[string]string{
			encryption.ContextKeyRefreshSessionID: "550e8400-e29b-41d4-a716-446655440001",
			encryption.ContextKeyKID:              "kid-123",
		}},
		{"malformed root subject", map[string]string{encryption.ContextKeyRefreshSessionID: "not-a-uuid"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decrypted, err := adapter.Decrypt(ctx, ciphertext, tt.context)
			require.Error(t, err)
			if len(decrypted) != 0 {
				t.Fatal("rejected decryption returned plaintext")
			}
		})
	}

	t.Run("tampered ciphertext", func(t *testing.T) {
		tampered := append([]byte(nil), ciphertext...)
		tampered[len(tampered)-1] ^= 1
		decrypted, err := adapter.Decrypt(ctx, tampered, root)
		require.Error(t, err)
		if len(decrypted) != 0 {
			t.Fatal("tampered ciphertext returned plaintext")
		}
	})
}

// TestCancelledContextClassifiedAsKEKUnavailable verifies that a cancelled or
// deadline-exceeded context is returned as ErrorKindKEKUnavailable — not as
// ErrorKindContextMismatch.
//
// Regression guard: context.DeadlineExceeded and context.Canceled both produce
// error strings that contain the word "context" (e.g. "context deadline exceeded").
// Without the ctx.Err() guard these would be misclassified as AAD-mismatch errors,
// signalling "do not retry" instead of "transient, retry after backoff".
func TestCancelledContextClassifiedAsKEKUnavailable(t *testing.T) {
	const testKEK = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	adapter, _, err := NewAWSEncryption(testKEK, "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	plaintext := []byte("test-token")
	encryptionContext := map[string]string{"service_id": "oauth2"}

	// Produce valid ciphertext with a live context so decrypt tests have something to work with.
	ciphertext, err := adapter.Encrypt(context.Background(), plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("setup: encryption failed: %v", err)
	}

	tests := []struct {
		name string
		ctx  func() context.Context
	}{
		{
			name: "already cancelled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
		},
		{
			name: "already expired deadline",
			ctx: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 0)
				// cancel is called to release resources; the context is already expired.
				t.Cleanup(cancel)
				return ctx
			},
		},
	}

	for _, tt := range tests {
		t.Run("Encrypt/"+tt.name, func(t *testing.T) {
			_, encErr := adapter.Encrypt(tt.ctx(), plaintext, encryptionContext)
			if encErr == nil {
				t.Fatal("expected error, got nil")
			}
			if !isKEKUnavailableError(encErr) {
				t.Errorf("expected ErrorKindKEKUnavailable, got: %v (type %T)", encErr, encErr)
			}
		})

		t.Run("Decrypt/"+tt.name, func(t *testing.T) {
			_, decErr := adapter.Decrypt(tt.ctx(), ciphertext, encryptionContext)
			if decErr == nil {
				t.Fatal("expected error, got nil")
			}
			if !isKEKUnavailableError(decErr) {
				t.Errorf("expected ErrorKindKEKUnavailable, got: %v (type %T)", decErr, decErr)
			}
		})
	}
}

// waitingSDKContext reports when a request has reached the cancellable gate wait.
type waitingSDKContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *waitingSDKContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestQueuedSDKCallsRespectContext(t *testing.T) {
	plaintext := []byte("secret")
	aad := map[string]string{"service_id": "oauth2"}
	for _, operation := range []struct {
		name string
		call func(context.Context, *AWSAdapter) error
	}{
		{"Encrypt", func(ctx context.Context, adapter *AWSAdapter) error {
			_, err := adapter.Encrypt(ctx, plaintext, aad)
			return err
		}},
		{"Decrypt", func(ctx context.Context, adapter *AWSAdapter) error {
			_, err := adapter.Decrypt(ctx, []byte("non-empty ciphertext"), aad)
			return err
		}},
	} {
		t.Run(operation.name+"/caller_cancel", func(t *testing.T) {
			require.NoError(t, lockSDK(context.Background()))
			held := true
			defer func() {
				if held {
					unlockSDK()
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			watched := &waitingSDKContext{Context: ctx, waiting: make(chan struct{})}
			var sdkCalled atomic.Bool
			adapter := &AWSAdapter{encryptionClient: &testMockEncryptionSDKClient{
				onCall: func(context.Context) error {
					sdkCalled.Store(true)
					return errors.New("SDK entered while gate held")
				},
			}}
			finished := make(chan error, 1)
			go func() { finished <- operation.call(watched, adapter) }()
			select {
			case <-watched.waiting:
			case <-time.After(2 * time.Second):
				t.Fatal("queued operation did not reach the SDK gate")
			}
			cancel()
			select {
			case err := <-finished:
				require.ErrorIs(t, err, context.Canceled)
				assert.True(t, isKEKUnavailableError(err))
				assert.False(t, sdkCalled.Load(), "cancelled call entered the SDK")
			case <-time.After(2 * time.Second):
				t.Fatal("queued operation did not return before the gate was released")
			}
			unlockSDK()
			held = false
		})

		t.Run(operation.name+"/internal_timeout", func(t *testing.T) {
			require.NoError(t, lockSDK(context.Background()))
			held := true
			defer func() {
				if held {
					unlockSDK()
				}
			}()

			const timeout = 100 * time.Millisecond
			var sdkCalled atomic.Bool
			adapter := &AWSAdapter{
				dynamoDBTimeout: timeout,
				encryptionClient: &testMockEncryptionSDKClient{
					onCall: func(context.Context) error {
						sdkCalled.Store(true)
						return errors.New("SDK entered while gate held")
					},
				},
			}
			finished := make(chan error, 1)
			go func() { finished <- operation.call(context.Background(), adapter) }()
			select {
			case err := <-finished:
				require.Error(t, err)
				assert.True(t, isKEKUnavailableError(err))
				assert.Contains(t, err.Error(), "dynamodb_timeout="+timeout.String())
				assert.False(t, sdkCalled.Load(), "timed-out call entered the SDK")
			case <-time.After(2 * time.Second):
				t.Fatal("queued operation did not respect dynamodb_timeout")
			}
			unlockSDK()
			held = false
		})
	}
}

// TestUniqueEncryptionPerCall tests that same plaintext produces different ciphertexts
func TestUniqueEncryptionPerCall(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_UNIQUE", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_UNIQUE environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_UNIQUE") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("same-token-value")
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Test: Encrypt same plaintext twice
	ciphertext1, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("first encryption failed: %v", err)
	}

	ciphertext2, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("second encryption failed: %v", err)
	}

	// Verify: Ciphertexts are different (fresh DEK per encryption)
	if string(ciphertext1) == string(ciphertext2) {
		t.Fatal("ciphertexts should be different for same plaintext (unique DEK per call)")
	}

	// Verify: Both can be decrypted to same plaintext
	decrypted1, err := adapter.Decrypt(ctx, ciphertext1, encryptionContext)
	if err != nil {
		t.Fatalf("first decryption failed: %v", err)
	}

	decrypted2, err := adapter.Decrypt(ctx, ciphertext2, encryptionContext)
	if err != nil {
		t.Fatalf("second decryption failed: %v", err)
	}

	if string(decrypted1) != string(plaintext) {
		t.Errorf("first decrypted mismatch: expected %q, got %q", string(plaintext), string(decrypted1))
	}

	if string(decrypted2) != string(plaintext) {
		t.Errorf("second decrypted mismatch: expected %q, got %q", string(plaintext), string(decrypted2))
	}
}

// TestEncryptEmptyPlaintext tests encryption of empty plaintext
func TestEncryptEmptyPlaintext(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_EMPTY", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_EMPTY environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_EMPTY") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	emptyPlaintext := []byte{}
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Test: Encrypt empty plaintext
	ciphertext, err := adapter.Encrypt(ctx, emptyPlaintext, encryptionContext)

	// AWS SDK should handle empty plaintext (may succeed or fail depending on configuration)
	// At minimum, check that error handling doesn't panic
	if err != nil {
		t.Logf("encryption of empty plaintext returned error (expected): %v", err)
	} else {
		t.Logf("encryption of empty plaintext succeeded (ciphertext length: %d)", len(ciphertext))
	}
}

// TestDecryptEmptyCiphertext tests that empty ciphertext fails
func TestDecryptEmptyCiphertext(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_EMPTY_CIPHER", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_EMPTY_CIPHER environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_EMPTY_CIPHER") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	emptyCiphertext := []byte{}
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Test: Try to decrypt empty ciphertext
	decrypted, err := adapter.Decrypt(ctx, emptyCiphertext, encryptionContext)

	// Verify: Decryption fails
	if err == nil {
		t.Fatal("expected decryption to fail with empty ciphertext")
	}

	// Verify: Decrypted is nil
	if decrypted != nil {
		t.Fatal("decrypted should be nil for empty ciphertext")
	}

	// Verify: Error is DecryptionFailed type
	if !isDecryptionFailedError(err) {
		t.Errorf("expected DecryptionFailed error, got: %v", err)
	}
}

// TestTamperedCiphertextDetection tests that tampered ciphertext fails
func TestTamperedCiphertextDetection(t *testing.T) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("TEST_KEK_TAMPER", testKEK); err != nil {
		t.Fatalf("failed to set TEST_KEK_TAMPER environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEK_TAMPER") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("test-token-value")
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Encrypt
	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	// Test: Tamper with first byte of ciphertext
	if len(ciphertext) > 0 {
		tamperedCiphertext := make([]byte, len(ciphertext))
		copy(tamperedCiphertext, ciphertext)
		tamperedCiphertext[0] = tamperedCiphertext[0] ^ 0xFF // Flip all bits
		ciphertext = tamperedCiphertext
	}

	// Test: Try to decrypt tampered ciphertext
	decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)

	// Verify: Decryption fails (authentication tag verification)
	if err == nil {
		t.Fatal("expected decryption to fail for tampered ciphertext")
	}

	// Verify: Error is IntegrityViolation or DecryptionFailed type
	if !isIntegrityViolationError(err) && !isDecryptionFailedError(err) {
		t.Errorf("expected IntegrityViolation or DecryptionFailed error, got: %v", err)
	}

	// Verify: Decrypted is nil
	if len(decrypted) > 0 {
		t.Fatal("decrypted should be empty for tampered ciphertext")
	}
}

// Helper functions for error type checking

func isKEKUnavailableError(err error) bool {
	encErr, ok := err.(*encryption.EncryptionError)
	return ok && encErr.Kind == encryption.ErrorKindKEKUnavailable
}

func isContextMismatchError(err error) bool {
	encErr, ok := err.(*encryption.EncryptionError)
	return ok && encErr.Kind == encryption.ErrorKindContextMismatch
}

func isIntegrityViolationError(err error) bool {
	encErr, ok := err.(*encryption.EncryptionError)
	return ok && encErr.Kind == encryption.ErrorKindIntegrityViolation
}

func isDecryptionFailedError(err error) bool {
	encErr, ok := err.(*encryption.EncryptionError)
	return ok && encErr.Kind == encryption.ErrorKindDecryptionFailed
}

// testMockEncryptionSDKClient calls onCall with the context received from the SDK call
// and returns the resulting error. Tests configure onCall to either wait for the context
// to expire (dynamodb_timeout path) or cancel the parent context (context_error path).
type testMockEncryptionSDKClient struct {
	onCall func(ctx context.Context) error
}

func (m *testMockEncryptionSDKClient) Encrypt(ctx context.Context, _ esdktypes.EncryptInput) (*esdktypes.EncryptOutput, error) {
	return nil, m.onCall(ctx)
}

func (m *testMockEncryptionSDKClient) Decrypt(ctx context.Context, _ esdktypes.DecryptInput) (*esdktypes.DecryptOutput, error) {
	return nil, m.onCall(ctx)
}

// TestEncryptDecryptPostCallContextExpiry guards the post-SDK-call classification branch
// (lines ~304-335 for Encrypt, ~499-530 for Decrypt).  The pre-check path is already
// covered by TestCancelledContextClassifiedAsKEKUnavailable; these tests cover expiry that
// occurs DURING the SDK call so the pre-check was not triggered.
//
// dynamodb_timeout: a non-zero dynamoDBTimeout causes Encrypt/Decrypt to wrap the caller's
// context with context.WithTimeoutCause.  The mock blocks on the derived ctx.Done() so the
// adapter's own internal timeout fires mid-call.  The error message must include the
// configured duration, proving the real internal-timeout path was exercised.
//
// context_error: no internal timeout; the mock cancels the parent context and returns an
// error to drive the non-sentinel context_error branch.
func TestEncryptDecryptPostCallContextExpiry(t *testing.T) {
	const testKEK = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	const internalTimeout = 1 * time.Millisecond

	// Produce a valid ciphertext for the Decrypt sub-tests.
	realAdapter, _, err := NewAWSEncryption(testKEK, "", 0)
	require.NoError(t, err)

	encCtx := map[string]string{"service_id": "test-svc"}
	plaintext := []byte("test-token")

	ciphertext, err := realAdapter.Encrypt(context.Background(), plaintext, encCtx)
	require.NoError(t, err)

	t.Run("Encrypt/dynamodb_timeout", func(t *testing.T) {
		mock := &testMockEncryptionSDKClient{
			onCall: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return errors.New("test guard: context did not expire — WithTimeoutCause not applied")
				}
			},
		}
		adapter := &AWSAdapter{encryptionClient: mock, dynamoDBTimeout: internalTimeout}

		_, encErr := adapter.Encrypt(context.Background(), plaintext, encCtx)
		require.Error(t, encErr)
		assert.True(t, isKEKUnavailableError(encErr), "expected KEKUnavailable, got %v (type %T)", encErr, encErr)
		assert.Contains(t, encErr.Error(), "dynamodb_timeout="+internalTimeout.String())
	})

	t.Run("Encrypt/context_error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mock := &testMockEncryptionSDKClient{
			onCall: func(_ context.Context) error {
				cancel()
				return errors.New("mock error after parent cancel")
			},
		}
		adapter := &AWSAdapter{encryptionClient: mock, dynamoDBTimeout: time.Minute}

		_, encErr := adapter.Encrypt(ctx, plaintext, encCtx)
		require.Error(t, encErr)
		assert.True(t, isKEKUnavailableError(encErr), "expected KEKUnavailable, got %v (type %T)", encErr, encErr)
		assert.NotContains(t, encErr.Error(), "dynamodb_timeout")
	})

	t.Run("Decrypt/dynamodb_timeout", func(t *testing.T) {
		mock := &testMockEncryptionSDKClient{
			onCall: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return errors.New("test guard: context did not expire — WithTimeoutCause not applied")
				}
			},
		}
		adapter := &AWSAdapter{encryptionClient: mock, dynamoDBTimeout: internalTimeout}

		_, decErr := adapter.Decrypt(context.Background(), ciphertext, encCtx)
		require.Error(t, decErr)
		assert.True(t, isKEKUnavailableError(decErr), "expected KEKUnavailable, got %v (type %T)", decErr, decErr)
		assert.Contains(t, decErr.Error(), "dynamodb_timeout="+internalTimeout.String())
	})

	t.Run("Decrypt/context_error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mock := &testMockEncryptionSDKClient{
			onCall: func(_ context.Context) error {
				cancel()
				return errors.New("mock error after parent cancel")
			},
		}
		adapter := &AWSAdapter{encryptionClient: mock, dynamoDBTimeout: time.Minute}

		_, decErr := adapter.Decrypt(ctx, ciphertext, encCtx)
		require.Error(t, decErr)
		assert.True(t, isKEKUnavailableError(decErr), "expected KEKUnavailable, got %v (type %T)", decErr, decErr)
		assert.NotContains(t, decErr.Error(), "dynamodb_timeout")
	})
}

func TestEncryptDecryptTimeoutErrorChainPreservesSDKError(t *testing.T) {
	const internalTimeout = 1 * time.Millisecond
	plaintext := []byte("test-token")
	ciphertext := []byte("not-empty")
	encryptionContext := map[string]string{"service_id": "test-svc"}

	t.Run("Encrypt", func(t *testing.T) {
		sdkErr := errors.New("mock sdk timeout after context expiry")
		adapter := &AWSAdapter{
			encryptionClient: &testMockEncryptionSDKClient{
				onCall: func(ctx context.Context) error {
					<-ctx.Done()
					return sdkErr
				},
			},
			dynamoDBTimeout: internalTimeout,
		}

		_, err := adapter.Encrypt(context.Background(), plaintext, encryptionContext)
		require.Error(t, err)
		assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
		assert.ErrorIs(t, err, sdkErr)
		assert.False(t, errors.Is(err, context.DeadlineExceeded), "timeout path should preserve the SDK error, not context.DeadlineExceeded")
	})

	t.Run("Decrypt", func(t *testing.T) {
		sdkErr := errors.New("mock sdk timeout after context expiry")
		adapter := &AWSAdapter{
			encryptionClient: &testMockEncryptionSDKClient{
				onCall: func(ctx context.Context) error {
					<-ctx.Done()
					return sdkErr
				},
			},
			dynamoDBTimeout: internalTimeout,
		}

		_, err := adapter.Decrypt(context.Background(), ciphertext, encryptionContext)
		require.Error(t, err)
		assert.True(t, isKEKUnavailableError(err), "expected KEKUnavailable, got %v (type %T)", err, err)
		assert.ErrorIs(t, err, sdkErr)
		assert.False(t, errors.Is(err, context.DeadlineExceeded), "timeout path should preserve the SDK error, not context.DeadlineExceeded")
	})
}

// BenchmarkEncrypt benchmarks encryption performance
func BenchmarkEncrypt(b *testing.B) {
	// Setup: Create adapter with env var KEK
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("BENCH_KEK", testKEK); err != nil {
		b.Fatalf("failed to set BENCH_KEK environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("BENCH_KEK") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		b.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("test-oauth2-access-token-value-1234567890")
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
		if err != nil {
			b.Fatalf("encryption failed: %v", err)
		}
	}
}

// BenchmarkDecrypt benchmarks decryption performance
func BenchmarkDecrypt(b *testing.B) {
	// Setup: Create adapter and prepare ciphertext
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("BENCH_KEK_DECRYPT", testKEK); err != nil {
		b.Fatalf("failed to set BENCH_KEK_DECRYPT environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("BENCH_KEK_DECRYPT") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		b.Fatalf("failed to create adapter: %v", err)
	}

	ctx := context.Background()
	plaintext := []byte("test-oauth2-access-token-value-1234567890")
	encryptionContext := map[string]string{
		"service_id": "oauth2",
	}

	// Prepare ciphertext
	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		b.Fatalf("encryption failed: %v", err)
	}

	b.ResetTimer()
	for b.Loop() {
		_, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
		if err != nil {
			b.Fatalf("decryption failed: %v", err)
		}
	}
}

// TestKMSHierarchicalKeyringAdapterCreation tests adapter creation with KMS ARN
// This tests the hierarchical keyring implementation (not env var KEK)
func TestKMSHierarchicalKeyringAdapterCreation(t *testing.T) {
	// Test with invalid KMS ARN (will fail on accessibility check in test env)
	// This validates the hierarchical keyring initialization flow
	invalidKMSARN := "arn:aws:kms:us-west-2:123456789012:key/invalid-test-key"

	adapter, _, err := NewAWSEncryption(invalidKMSARN, "", 0)

	// Expected to fail due to invalid KMS key in test environment
	if err == nil {
		t.Fatal("expected error for invalid KMS ARN in test environment")
	}

	if adapter != nil {
		t.Fatal("adapter should be nil when KMS key is not accessible")
	}

	// Verify error is KEKUnavailable type
	if !isKEKUnavailableError(err) {
		t.Errorf("expected KEKUnavailable error, got: %v", err)
	}
}

// TestKMSARNValidation tests that KMS ARN format validation works
func TestKMSARNValidation(t *testing.T) {
	tests := []struct {
		name        string
		keyMaterial string
		shouldFail  bool
	}{
		{
			name:        "valid_kms_arn_format",
			keyMaterial: "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012",
			shouldFail:  true, // Will fail on KMS accessibility in test, not format
		},
		{
			name:        "kms_alias_arn_format",
			keyMaterial: "arn:aws:kms:us-east-1:123456789012:alias/my-encryption-key",
			shouldFail:  true, // Will fail on KMS accessibility in test, not format
		},
		{
			name:        "invalid_kms_arn_format",
			keyMaterial: "arn:aws:s3:::my-bucket", // S3 ARN, not KMS
			shouldFail:  true,                     // Will fail on ARN format validation
		},
		{
			name:        "malformed_arn",
			keyMaterial: "not-an-arn",
			shouldFail:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, _, err := NewAWSEncryption(tt.keyMaterial, "", 0)

			if !tt.shouldFail && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if tt.shouldFail && err == nil {
				t.Errorf("expected error, got nil")
			}

			if adapter != nil && !tt.shouldFail {
				// Only expect a valid adapter if we expected success
				if adapter.keyring == nil {
					t.Error("adapter keyring should not be nil")
				}
				if adapter.encryptionClient == nil {
					t.Error("adapter encryptionClient should not be nil")
				}
			}
		})
	}
}

// TestHierarchicalKeyringWithEnvVarFallback tests that env var KEK still works as fallback
func TestHierarchicalKeyringWithEnvVarFallback(t *testing.T) {
	// Test that environment variable KEK path still works (non-hierarchical)
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("FALLBACK_KEK", testKEK); err != nil {
		t.Fatalf("failed to set FALLBACK_KEK environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("FALLBACK_KEK") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)

	// Should succeed with env var KEK
	if err != nil {
		t.Fatalf("failed to create adapter with env var KEK: %v", err)
	}

	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}

	if adapter.keyring == nil {
		t.Error("adapter keyring should not be nil")
	}

	if adapter.encryptionClient == nil {
		t.Error("adapter encryptionClient should not be nil")
	}

	// Test that encryption/decryption still works with env var KEK
	ctx := context.Background()
	plaintext := []byte("test-oauth2-token")
	encryptionContext := map[string]string{"service_id": "oauth2"}

	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Fatal("ciphertext should not be empty")
	}

	decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted plaintext does not match: expected %q, got %q", string(plaintext), string(decrypted))
	}
}

// TestAdapterInterfaceImplementation tests that AWSAdapter implements EncryptionPort
func TestAdapterInterfaceImplementation(t *testing.T) {
	// Setup
	testKEK := "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	if err := os.Setenv("INTERFACE_KEK", testKEK); err != nil {
		t.Fatalf("failed to set INTERFACE_KEK environment variable: %v", err)
	}
	defer func() { _ = os.Unsetenv("INTERFACE_KEK") }()

	adapter, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	// Verify adapter was created and has required fields
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}

	if adapter.keyring == nil {
		t.Error("adapter keyring should not be nil")
	}

	if adapter.encryptionClient == nil {
		t.Error("adapter encryptionClient should not be nil")
	}

	// Test that the methods work
	ctx := context.Background()
	plaintext := []byte("test-data")
	encryptionContext := map[string]string{"key": "value"}

	// Should be able to call Encrypt
	ciphertext, err := adapter.Encrypt(ctx, plaintext, encryptionContext)
	if err != nil {
		t.Fatalf("Encrypt method failed: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Error("Encrypt should produce non-empty ciphertext")
	}

	// Should be able to call Decrypt
	decrypted, err := adapter.Decrypt(ctx, ciphertext, encryptionContext)
	if err != nil {
		t.Fatalf("Decrypt method failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decrypt should return original plaintext: expected %q, got %q", string(plaintext), string(decrypted))
	}
}

// TestEncryptContextClassification verifies that the ctx.Err() guard correctly
// distinguishes a caller-supplied deadline from the adapter's own dynamoDBTimeout.
//
// Regression guard: the previous implementation used
//
//	errors.Is(ctxErr, context.DeadlineExceeded) && dynamoDBTimeout > 0
//
// which fires for ANY DeadlineExceeded, including the caller's — misattributing a
// caller timeout as a DynamoDB misconfiguration. The fix uses context.Cause, which
// is only set to errDynamoDBTimeout when our internal timer fires.
func TestEncryptContextClassification(t *testing.T) {
	const testKEK = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	base, err := newAdapterWithBase64KEK(testKEK)
	require.NoError(t, err)
	adapter := &AWSAdapter{
		encryptionClient: base.encryptionClient,
		keyring:          base.keyring,
		dynamoDBTimeout:  5 * time.Second,
	}
	encCtx := map[string]string{"service_id": "12345678-1234-1234-1234-123456789012"}
	plaintext := []byte("test")

	t.Run("parent_deadline_classified_as_context_error", func(t *testing.T) {
		// context.WithDeadline with a past time creates a DeadlineExceeded context
		// synchronously — no sleep needed.  The old code would wrongly emit
		// dynamodb_timeout here because DeadlineExceeded && dynamoDBTimeout>0 is true.
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
		defer cancel()

		_, err := adapter.Encrypt(ctx, plaintext, encCtx)

		require.Error(t, err)
		var encErr *encryption.EncryptionError
		require.ErrorAs(t, err, &encErr)
		assert.Equal(t, encryption.ErrorKindKEKUnavailable, encErr.Kind)
		assert.Contains(t, encErr.Message, "cancelled")
		assert.NotContains(t, encErr.Message, "dynamodb_timeout=")
	})

	t.Run("pre_cancelled_with_internal_sentinel_classified_as_context_error", func(t *testing.T) {
		// The adapter creates its own WithTimeoutCause context immediately before the pre-call
		// ctx.Err() guard, so a pre-cancelled caller context never represents an internal
		// dynamodb_timeout expiry in production. Even if the caller uses our sentinel as a
		// cancellation cause, the pre-call fast path should stay on the generic context_error
		// classification.
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errDynamoDBTimeout)

		_, err := adapter.Encrypt(ctx, plaintext, encCtx)

		require.Error(t, err)
		var encErr *encryption.EncryptionError
		require.ErrorAs(t, err, &encErr)
		assert.Equal(t, encryption.ErrorKindKEKUnavailable, encErr.Kind)
		assert.Contains(t, encErr.Message, "cancelled")
		assert.NotContains(t, encErr.Message, "dynamodb_timeout=")
	})
}

// TestDecryptContextClassification mirrors TestEncryptContextClassification for Decrypt.
func TestDecryptContextClassification(t *testing.T) {
	const testKEK = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	base, err := newAdapterWithBase64KEK(testKEK)
	require.NoError(t, err)
	adapter := &AWSAdapter{
		encryptionClient: base.encryptionClient,
		keyring:          base.keyring,
		dynamoDBTimeout:  5 * time.Second,
	}
	encCtx := map[string]string{"service_id": "12345678-1234-1234-1234-123456789012"}
	// Non-empty to pass the length guard; actual content irrelevant — context fires first.
	fakeCiphertext := []byte("not-empty")

	t.Run("parent_deadline_classified_as_context_error", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
		defer cancel()

		_, err := adapter.Decrypt(ctx, fakeCiphertext, encCtx)

		require.Error(t, err)
		var encErr *encryption.EncryptionError
		require.ErrorAs(t, err, &encErr)
		assert.Equal(t, encryption.ErrorKindKEKUnavailable, encErr.Kind)
		assert.Contains(t, encErr.Message, "cancelled")
		assert.NotContains(t, encErr.Message, "dynamodb_timeout=")
	})

	t.Run("pre_cancelled_with_internal_sentinel_classified_as_context_error", func(t *testing.T) {
		ctx, cancel := context.WithCancelCause(context.Background())
		cancel(errDynamoDBTimeout)

		_, err := adapter.Decrypt(ctx, fakeCiphertext, encCtx)

		require.Error(t, err)
		var encErr *encryption.EncryptionError
		require.ErrorAs(t, err, &encErr)
		assert.Equal(t, encryption.ErrorKindKEKUnavailable, encErr.Kind)
		assert.Contains(t, encErr.Message, "cancelled")
		assert.NotContains(t, encErr.Message, "dynamodb_timeout=")
	})
}

// T045 [US2] Unit test: Plaintext KEK never logged
// This test verifies that KEK material is never included in error messages or logs
func TestPlaintextKEKNeverLogged(t *testing.T) {
	// Test various error scenarios and verify KEK is not in error messages
	tests := []struct {
		name        string
		keyMaterial string
		shouldFail  bool
	}{
		{"invalid base64", "not-valid-base64!!!", true},
		{"too short KEK", "AQIDBAUGBwgJCgsMDQ4PEA==", true}, // 16 bytes binary data, need 32
		{"empty KEK", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := NewAWSEncryption(tt.keyMaterial, "", 0)

			if tt.shouldFail {
				if err == nil {
					t.Fatalf("expected error for %s", tt.name)
				}

				// Verify error message does NOT contain the KEK material
				errMsg := err.Error()
				if tt.keyMaterial != "" && len(tt.keyMaterial) > 10 {
					// Check that no part of the KEK appears in the error
					if strings.Contains(errMsg, tt.keyMaterial) {
						t.Errorf("error message should not contain KEK material, got: %s", errMsg)
					}
				}

				// Verify error message is sanitized (contains generic messages only)
				// Accept various error message patterns that don't leak KEK material
				sanitized := false
				for _, keyword := range []string{
					"invalid", "unavailable", "failed", "base64", "length",
					"required", "must be", "bytes", "material",
				} {
					if strings.Contains(errMsg, keyword) {
						sanitized = true
						break
					}
				}
				if !sanitized {
					t.Errorf("error message should contain generic error info: %s", errMsg)
				}
			}
		})
	}
}

func TestSDKAbortDoesNotPreventSubsequentEncryption(t *testing.T) {
	base, _, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0)
	require.NoError(t, err)
	defer func() {
		select {
		case <-sdkGate:
		default:
		}
	}()
	broken := &AWSAdapter{encryptionClient: &testMockEncryptionSDKClient{onCall: func(context.Context) error {
		panic("SDK aborted the request")
	}}, keyring: base.keyring}
	aad := map[string]string{"service_id": "recovered-request"}
	require.Panics(t, func() { _, _ = broken.Encrypt(t.Context(), []byte("discarded"), aad) })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	plaintext := []byte("subsequent valid request")
	ciphertext, err := base.Encrypt(ctx, plaintext, aad)
	require.NoError(t, err, "a recovered request abort must not strand the process-wide SDK gate")
	recovered, err := base.Decrypt(ctx, ciphertext, aad)
	require.NoError(t, err)
	require.Equal(t, plaintext, recovered)
}
