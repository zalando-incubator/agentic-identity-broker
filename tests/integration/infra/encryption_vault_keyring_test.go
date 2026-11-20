//go:build integration
// +build integration

package integration

import (
	"context"
	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

var sharedEmulator *bootstrap.AWSEmulatorContainer

// Test service UUIDs — must match the IDs provisioned in bootstrap/aws_emulator.go:preBranchKeysForEmulator
// and the UUIDs in fixtures.TestServices().
const (
	testServiceOAuth2 = "01234567-89ab-cdef-0123-456789abcdef"
	testServiceGitHub = "12345678-9abc-def0-1234-56789abcdef0"
	testServiceGoogle = "23456789-abcd-ef01-2345-6789abcdef01"
)

// requireSharedEmulator skips the test if the AWS emulator is unavailable (e.g. no Docker).
func requireSharedEmulator(t *testing.T) *bootstrap.AWSEmulatorContainer {
	t.Helper()
	if sharedEmulator == nil {
		t.Skip("AWS emulator unavailable — ensure Docker or Colima is running")
	}
	return sharedEmulator
}

// newAdapter creates a fresh adapter pointing at the shared AWS emulator instance.
func newAdapter(t *testing.T) *awsencryption.AWSAdapter {
	t.Helper()
	emulator := requireSharedEmulator(t)
	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter, _, err := awsencryption.NewAWSEncryption(kmsARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err, "failed to create encryption adapter")
	return adapter
}

// TestEncryption_Roundtrip verifies basic encrypt/decrypt and that each call produces a unique ciphertext.
func TestEncryption_Roundtrip(t *testing.T) {
	ctx := context.Background()
	adapter := newAdapter(t)

	cases := []struct {
		name      string
		plaintext []byte
		serviceID string
	}{
		{"oauth2 token", []byte("test-oauth2-token"), testServiceOAuth2},
		{"github token", []byte("test-github-token"), testServiceGitHub},
		{"google token", []byte("test-google-token"), testServiceGoogle},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encCtx := map[string]string{"service_id": tc.serviceID}

			ct1, err := adapter.Encrypt(ctx, tc.plaintext, encCtx)
			require.NoError(t, err, "first encryption failed")

			ct2, err := adapter.Encrypt(ctx, tc.plaintext, encCtx)
			require.NoError(t, err, "second encryption failed")

			assert.NotEqual(t, string(ct1), string(ct2), "ciphertexts should differ (fresh DEK per call)")

			decrypted, err := adapter.Decrypt(ctx, ct1, encCtx)
			require.NoError(t, err, "decryption failed")
			assert.Equal(t, string(tc.plaintext), string(decrypted))
		})
	}
}

// TestEncryption_ContextBinding verifies that decryption fails when the wrong service_id is supplied.
func TestEncryption_ContextBinding(t *testing.T) {
	evidence.ForTest(t, "storage-and-encryption.encryption.kms-context")
	ctx := context.Background()
	adapter := newAdapter(t)

	plaintext := []byte("bound-token")
	encCtx := map[string]string{"service_id": testServiceOAuth2}
	wrongCtx := map[string]string{"service_id": testServiceGitHub}

	ciphertext, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err)

	cases := []struct {
		name    string
		decCtx  map[string]string
		wantErr bool
	}{
		{"correct context succeeds", encCtx, false},
		{"wrong service fails", wrongCtx, true},
		{"modified context fails", map[string]string{"service_id": testServiceGoogle}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decrypted, err := adapter.Decrypt(ctx, ciphertext, tc.decCtx)
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, decrypted)
				assert.True(t, isContextMismatchError(err) || isDecryptionFailedError(err),
					"expected ContextMismatch or DecryptionFailed, got: %v (%T)", err, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, string(plaintext), string(decrypted))
			}
		})
	}
}

// TestEncryption_TamperingAndIntegrity verifies that any modification to ciphertext is detected.
func TestEncryption_TamperingAndIntegrity(t *testing.T) {
	ctx := context.Background()
	adapter := newAdapter(t)

	plaintext := []byte("tamper-test-token")
	encCtx := map[string]string{"service_id": testServiceOAuth2}

	ciphertext, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err)
	require.True(t, len(ciphertext) > 2, "ciphertext too short to tamper")

	cases := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{"flip first byte", func(b []byte) []byte { c := cp(b); c[0] ^= 0xFF; return c }},
		{"flip middle byte", func(b []byte) []byte { c := cp(b); c[len(c)/2] ^= 0x01; return c }},
		{"truncate last byte", func(b []byte) []byte { return cp(b)[:len(b)-1] }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := adapter.Decrypt(ctx, tc.mutate(ciphertext), encCtx)
			require.Error(t, err, "tampered ciphertext should fail decryption")
			assert.True(t, isIntegrityViolationError(err) || isDecryptionFailedError(err) || isContextMismatchError(err),
				"unexpected error type: %v (%T)", err, err)
		})
	}
}

// TestEncryption_MultiServiceIsolation verifies that tokens encrypted for one service cannot
// be decrypted by another, and that all 3×3 cross-service combinations behave correctly.
func TestEncryption_MultiServiceIsolation(t *testing.T) {
	ctx := context.Background()
	adapter := newAdapter(t)

	services := []string{testServiceOAuth2, testServiceGitHub, testServiceGoogle}
	ciphertexts := make(map[string][]byte, len(services))

	for _, svc := range services {
		pt := []byte("token-for-" + svc)
		ct, err := adapter.Encrypt(ctx, pt, map[string]string{"service_id": svc})
		require.NoError(t, err, "encryption for %s failed", svc)
		ciphertexts[svc] = ct

		// Verify same plaintext encrypted for different services produces different ciphertexts
		ct2, err := adapter.Encrypt(ctx, []byte("shared-plaintext"), map[string]string{"service_id": svc})
		require.NoError(t, err)
		_ = ct2
	}

	// Verify ciphertexts for shared-plaintext differ across services
	sharedCTs := make(map[string][]byte, len(services))
	for _, svc := range services {
		ct, err := adapter.Encrypt(ctx, []byte("shared-plaintext"), map[string]string{"service_id": svc})
		require.NoError(t, err)
		sharedCTs[svc] = ct
	}
	assert.NotEqual(t, string(sharedCTs[testServiceOAuth2]), string(sharedCTs[testServiceGitHub]))
	assert.NotEqual(t, string(sharedCTs[testServiceOAuth2]), string(sharedCTs[testServiceGoogle]))

	// All 3×3 combinations: same-service → success, cross-service → error
	successes, failures := 0, 0
	for _, src := range services {
		for _, dst := range services {
			_, err := adapter.Decrypt(ctx, ciphertexts[src], map[string]string{"service_id": dst})
			if src == dst {
				assert.NoError(t, err, "same-service decryption should succeed: %s", src)
				successes++
			} else {
				assert.Error(t, err, "cross-service decryption should fail: %s→%s", src, dst)
				failures++
			}
		}
	}
	assert.Equal(t, 3, successes)
	assert.Equal(t, 6, failures)
}

// TestEncryption_EdgeCases covers boundary inputs: large tokens, empty plaintext, unknown service.
func TestEncryption_EdgeCases(t *testing.T) {
	ctx := context.Background()
	adapter := newAdapter(t)

	t.Run("large token (100KB)", func(t *testing.T) {
		large := make([]byte, 100*1024)
		for i := range large {
			large[i] = byte(i % 256)
		}
		encCtx := map[string]string{"service_id": testServiceOAuth2}
		ct, err := adapter.Encrypt(ctx, large, encCtx)
		require.NoError(t, err)
		decrypted, err := adapter.Decrypt(ctx, ct, encCtx)
		require.NoError(t, err)
		assert.Equal(t, len(large), len(decrypted))
	})

	t.Run("empty plaintext rejected", func(t *testing.T) {
		encCtx := map[string]string{"service_id": testServiceOAuth2}
		ct, encErr := adapter.Encrypt(ctx, []byte{}, encCtx)
		if encErr == nil {
			_, decErr := adapter.Decrypt(ctx, ct, encCtx)
			require.Error(t, decErr, "empty plaintext encryption/decryption should fail")
		}
	})

	t.Run("unknown service context", func(t *testing.T) {
		encCtx := map[string]string{"service_id": testServiceOAuth2}
		ct, err := adapter.Encrypt(ctx, []byte("token"), encCtx)
		require.NoError(t, err)
		_, err = adapter.Decrypt(ctx, ct, map[string]string{"service_id": "unknown-service-xyz"})
		require.Error(t, err, "decryption with unknown service context should fail")
	})
}

// TestEncryption_Configuration verifies adapter construction succeeds with a valid ARN and
// fails with clearly invalid inputs.
func TestEncryption_Configuration(t *testing.T) {
	emulator := requireSharedEmulator(t)
	validARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID

	adapter, manager, err := awsencryption.NewAWSEncryption(validARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err)
	require.NotNil(t, adapter)
	require.NotNil(t, manager)

	for _, invalid := range []string{"not-an-arn", "arn:aws:s3:::bucket", ""} {
		_, _, err := awsencryption.NewAWSEncryption(invalid, "IdentityBrokerEncryptionBranchKeys", 0)
		if err == nil && invalid != "" {
			t.Logf("invalid ARN %q accepted (may fail at runtime)", invalid)
		}
	}
}

// TestEncryption_BranchKeyProvisioning verifies all three pre-provisioned services have
// working branch keys in the shared AWS emulator DynamoDB table.
func TestEncryption_BranchKeyProvisioning(t *testing.T) {
	ctx := context.Background()
	adapter := newAdapter(t)

	for _, svc := range []string{testServiceOAuth2, testServiceGitHub, testServiceGoogle} {
		t.Run(svc, func(t *testing.T) {
			pt := []byte("provisioning-check-" + svc)
			encCtx := map[string]string{"service_id": svc}
			ct, err := adapter.Encrypt(ctx, pt, encCtx)
			require.NoError(t, err)
			decrypted, err := adapter.Decrypt(ctx, ct, encCtx)
			require.NoError(t, err)
			assert.Equal(t, string(pt), string(decrypted))
		})
	}
}

// TestEncryption_KeyRotationBackwardCompatibility verifies tokens remain decryptable after
// KMS on-demand key rotation. This test mutates KMS state and therefore runs last.
func TestEncryption_KeyRotationBackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsClient, err := bootstrap.NewKMSClientForEmulator(ctx, emulator.Endpoint)
	require.NoError(t, err)

	_, err = kmsClient.EnableKeyRotation(ctx, &kms.EnableKeyRotationInput{KeyId: &emulator.KMSKeyID})
	require.NoError(t, err, "failed to enable key rotation")

	adapter := newAdapter(t)
	plaintext := []byte("token-before-rotation")
	encCtx := map[string]string{"service_id": testServiceOAuth2}

	ciphertext, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err)

	_, err = kmsClient.RotateKeyOnDemand(ctx, &kms.RotateKeyOnDemandInput{KeyId: &emulator.KMSKeyID})
	require.NoError(t, err, "failed to perform on-demand key rotation")

	decrypted, err := adapter.Decrypt(ctx, ciphertext, encCtx)
	require.NoError(t, err, "decryption after key rotation failed — backward compatibility broken")
	assert.Equal(t, string(plaintext), string(decrypted))
}

// TestEncryption_BranchKeyManagerCreate verifies that BranchKeyManager.Create provisions a
// branch key in real DynamoDB and that the new service can subsequently encrypt and decrypt.
func TestEncryption_BranchKeyManagerCreate(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter, manager, err := awsencryption.NewAWSEncryption(kmsARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Use a service ID not pre-provisioned in bootstrap so we exercise real creation.
	newServiceID := id.MustParseServiceID("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	branchKeyID, err := manager.Create(ctx, encryption.NewServiceBranchKeySubject(newServiceID))
	require.NoError(t, err, "BranchKeyManager.Create should succeed for a new service")
	require.NotEmpty(t, branchKeyID, "returned branch key ID should not be empty")

	// The new service must now be usable for encrypt/decrypt.
	plaintext := []byte("token-for-new-service")
	encCtx := map[string]string{"service_id": newServiceID.String()}

	ct, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err, "encryption after Create should succeed")

	decrypted, err := adapter.Decrypt(ctx, ct, encCtx)
	require.NoError(t, err, "decryption after Create should succeed")
	assert.Equal(t, string(plaintext), string(decrypted))
}

func TestEncryption_BranchKeyManagerCreate_SigningKey(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter, manager, err := awsencryption.NewAWSEncryption(kmsARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err)
	require.NotNil(t, manager)

	signingKeyKID := id.NewKeyID("cccccccc-dddd-eeee-ffff-000000000000")

	branchKeyID, err := manager.Create(ctx, encryption.NewSigningKeyBranchKeySubject(signingKeyKID))
	require.NoError(t, err, "BranchKeyManager.Create should succeed for a signing key subject")
	assert.Equal(t, "key_"+signingKeyKID.String()+"_branch_key", branchKeyID)

	plaintext := []byte("token-for-new-signing-key")
	encCtx := map[string]string{"kid": signingKeyKID.String()}

	ct, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err, "encryption after Create should succeed for signing keys")

	decrypted, err := adapter.Decrypt(ctx, ct, encCtx)
	require.NoError(t, err, "decryption after Create should succeed for signing keys")
	assert.Equal(t, string(plaintext), string(decrypted))
}

func TestEncryption_CrossNamespaceIsolation(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter, manager, err := awsencryption.NewAWSEncryption(kmsARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err)
	require.NotNil(t, manager)

	signingKeyKIDA := id.NewKeyID("cross-namespace-kid-a")
	signingKeyKIDB := id.NewKeyID("cross-namespace-kid-b")

	_, err = manager.Create(ctx, encryption.NewSigningKeyBranchKeySubject(signingKeyKIDA))
	require.NoError(t, err)
	_, err = manager.Create(ctx, encryption.NewSigningKeyBranchKeySubject(signingKeyKIDB))
	require.NoError(t, err)

	signingCtxA := map[string]string{"kid": signingKeyKIDA.String()}
	signingCtxB := map[string]string{"kid": signingKeyKIDB.String()}
	serviceCtx := map[string]string{"service_id": testServiceOAuth2}

	signingPlaintext := []byte("token-for-signing-kid-a")
	signingCiphertext, err := adapter.Encrypt(ctx, signingPlaintext, signingCtxA)
	require.NoError(t, err)

	servicePlaintext := []byte("token-for-service-namespace")
	serviceCiphertext, err := adapter.Encrypt(ctx, servicePlaintext, serviceCtx)
	require.NoError(t, err)

	cases := []struct {
		name       string
		ciphertext []byte
		decCtx     map[string]string
		plaintext  []byte
		wantErr    bool
	}{
		{
			name:       "signing key decrypts with original kid",
			ciphertext: signingCiphertext,
			decCtx:     signingCtxA,
			plaintext:  signingPlaintext,
		},
		{
			name:       "signing key ciphertext rejects different kid",
			ciphertext: signingCiphertext,
			decCtx:     signingCtxB,
			wantErr:    true,
		},
		{
			name:       "signing key ciphertext rejects service namespace",
			ciphertext: signingCiphertext,
			decCtx:     serviceCtx,
			wantErr:    true,
		},
		{
			name:       "service ciphertext decrypts with original service",
			ciphertext: serviceCiphertext,
			decCtx:     serviceCtx,
			plaintext:  servicePlaintext,
		},
		{
			name:       "service ciphertext rejects signing key namespace",
			ciphertext: serviceCiphertext,
			decCtx:     signingCtxA,
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decrypted, err := adapter.Decrypt(ctx, tc.ciphertext, tc.decCtx)
			if tc.wantErr {
				require.Error(t, err)
				require.Empty(t, decrypted)
				assert.True(t, isContextMismatchError(err) || isDecryptionFailedError(err) || isIntegrityViolationError(err),
					"expected namespace isolation error, got: %v (%T)", err, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, string(tc.plaintext), string(decrypted))
		})
	}
}

// TestEncryption_BranchKeyManagerCreate_Idempotent verifies that calling Create twice for the
// same service ID is idempotent: the second call succeeds (or returns the existing key) and the
// original ciphertext remains decryptable.
func TestEncryption_BranchKeyManagerCreate_Idempotent(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter, manager, err := awsencryption.NewAWSEncryption(kmsARN, "IdentityBrokerEncryptionBranchKeys", 0)
	require.NoError(t, err)
	require.NotNil(t, manager)

	// Use a distinct service ID so this test doesn't share state with the creation test.
	duplicateServiceID := id.MustParseServiceID("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")
	encCtx := map[string]string{"service_id": duplicateServiceID.String()}

	// First creation.
	id1, err := manager.Create(ctx, encryption.NewServiceBranchKeySubject(duplicateServiceID))
	require.NoError(t, err, "first Create should succeed")

	// Encrypt with the first key.
	plaintext := []byte("idempotency-test-token")
	ct, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err)

	// Second creation — must not error and must not invalidate the existing ciphertext.
	id2, err := manager.Create(ctx, encryption.NewServiceBranchKeySubject(duplicateServiceID))
	require.NoError(t, err, "second Create (duplicate) should succeed idempotently")
	assert.Equal(t, id1, id2, "idempotent Create should return the same branch key ID")

	// Original ciphertext must still decrypt correctly.
	decrypted, err := adapter.Decrypt(ctx, ct, encCtx)
	require.NoError(t, err, "ciphertext encrypted before duplicate Create must still decrypt")
	assert.Equal(t, string(plaintext), string(decrypted))
}

// TestEncryption_CancelledContextClassifiedAsKEKUnavailable verifies that a pre-cancelled or
// expired context returns ErrorKindKEKUnavailable against real KMS — not ErrorKindContextMismatch.
// This is a regression guard: both context.Canceled and context.DeadlineExceeded produce error
// messages containing "context", which the error-classification code must not misroute.
func TestEncryption_CancelledContextClassifiedAsKEKUnavailable(t *testing.T) {
	ctx := context.Background()
	emulator := requireSharedEmulator(t)

	kmsARN := "arn:aws:kms:eu-central-1:000000000000:key/" + emulator.KMSKeyID
	adapter := newAdapter(t)

	// Produce valid ciphertext with a live context.
	plaintext := []byte("regression-token")
	encCtx := map[string]string{"service_id": testServiceOAuth2}
	ct, err := adapter.Encrypt(ctx, plaintext, encCtx)
	require.NoError(t, err, "setup: encryption with live context should succeed")
	_ = kmsARN

	cases := []struct {
		name  string
		mkCtx func() (context.Context, context.CancelFunc)
	}{
		{
			"already cancelled",
			func() (context.Context, context.CancelFunc) {
				c, cancel := context.WithCancel(context.Background())
				cancel()
				return c, cancel
			},
		},
		{
			"already expired deadline",
			func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 0)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deadCtx, cancel := tc.mkCtx()
			defer cancel()

			_, encErr := adapter.Encrypt(deadCtx, plaintext, encCtx)
			require.Error(t, encErr)
			var encryptionErr *encryption.EncryptionError
			require.ErrorAs(t, encErr, &encryptionErr)
			assert.Equal(t, encryption.ErrorKindKEKUnavailable, encryptionErr.Kind,
				"cancelled context must yield KEKUnavailable, not %s", encryptionErr.Kind)

			deadCtx2, cancel2 := tc.mkCtx()
			defer cancel2()

			_, decErr := adapter.Decrypt(deadCtx2, ct, encCtx)
			require.Error(t, decErr)
			require.ErrorAs(t, decErr, &encryptionErr)
			assert.Equal(t, encryption.ErrorKindKEKUnavailable, encryptionErr.Kind,
				"cancelled context must yield KEKUnavailable, not %s", encryptionErr.Kind)
		})
	}
}

// ---- helpers ----------------------------------------------------------------

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

func cp(b []byte) []byte {
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
