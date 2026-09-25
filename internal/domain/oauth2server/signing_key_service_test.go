package oauth2server

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// testEncryptor is a minimal encryption implementation for testing.
// It stores ciphertext as plaintext (no actual encryption) to enable testing
// without AWS KMS infrastructure.
type testEncryptor struct{}

func (e *testEncryptor) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	// For testing: return plaintext with a marker prefix to distinguish from raw data
	result := make([]byte, 0, len(plaintext)+4)
	result = append(result, []byte("ENC:")...)
	result = append(result, plaintext...)
	return result, nil
}

func (e *testEncryptor) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	// For testing: strip the marker prefix
	if len(ciphertext) < 4 || string(ciphertext[:4]) != "ENC:" {
		return nil, assert.AnError
	}
	return ciphertext[4:], nil
}

type bootstrapContextKey struct{}

type contextCheckingEncryptor struct {
	marker     string
	sawEncrypt bool
}

func (e *contextCheckingEncryptor) Encrypt(ctx context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	if ctx.Value(bootstrapContextKey{}) == e.marker {
		e.sawEncrypt = true
	}

	result := make([]byte, 0, len(plaintext)+4)
	result = append(result, []byte("ENC:")...)
	result = append(result, plaintext...)
	return result, nil
}

func (e *contextCheckingEncryptor) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	if len(ciphertext) < 4 || string(ciphertext[:4]) != "ENC:" {
		return nil, assert.AnError
	}
	return ciphertext[4:], nil
}

// mockBranchKeyManager is a hand-rolled mock for ports.BranchKeyManager.
type mockBranchKeyManager struct {
	createFn    func(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error)
	createCalls int
	lastSubject domainencryption.BranchKeySubject
}

func (m *mockBranchKeyManager) Create(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	m.createCalls++
	m.lastSubject = subject
	if m.createFn != nil {
		return m.createFn(ctx, subject)
	}
	return "", nil
}

func newNoopBranchKeyManager() *mockBranchKeyManager {
	return &mockBranchKeyManager{}
}

type testSigningKeyStore = strategySigningKeyStore

func newTestSigningKeyStore() *testSigningKeyStore {
	return newStrategySigningKeyStore()
}

type bootstrapContextCheckingRepo struct {
	*testSigningKeyStore
	marker                 string
	sawCountActive         bool
	sawCreateAndSetCurrent bool
}

type bootstrapContextCheckingCoordinator struct {
	marker string
}

func (c *bootstrapContextCheckingCoordinator) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	return fn(context.WithValue(ctx, bootstrapContextKey{}, c.marker))
}

type postCallbackErrorBootstrapCoordinator struct {
	err          error
	afterSuccess func()
}

func (c *postCallbackErrorBootstrapCoordinator) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	if err := fn(ctx); err != nil {
		return err
	}
	if c.afterSuccess != nil {
		c.afterSuccess()
	}
	return c.err
}

type recoveryCountContextRepo struct {
	*testSigningKeyStore
	rejectCanceledCountContext bool
	sawRecoveryCount           bool
}

func (r *recoveryCountContextRepo) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	if r.rejectCanceledCountContext {
		r.sawRecoveryCount = true
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
	return r.testSigningKeyStore.CountActiveInDomain(ctx, domain)
}

type recoveryCountProbeRepo struct {
	*testSigningKeyStore
	countCalls               int
	sawRecoveryCount         bool
	recoveryCountHasDeadline bool
	recoveryCountErr         error
}

func (r *recoveryCountProbeRepo) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	r.countCalls++
	if r.countCalls > 1 {
		r.sawRecoveryCount = true
		_, r.recoveryCountHasDeadline = ctx.Deadline()
		if r.recoveryCountErr != nil {
			return 0, r.recoveryCountErr
		}
	}
	return r.testSigningKeyStore.CountActiveInDomain(ctx, domain)
}

func (r *bootstrapContextCheckingRepo) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	if ctx.Value(bootstrapContextKey{}) == r.marker {
		r.sawCountActive = true
	}
	return r.testSigningKeyStore.CountActiveInDomain(ctx, domain)
}

func (r *bootstrapContextCheckingRepo) CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error {
	if ctx.Value(bootstrapContextKey{}) == r.marker {
		r.sawCreateAndSetCurrent = true
	}
	return r.testSigningKeyStore.CreateAndSetCurrent(ctx, key)
}

// failingDecryptor always errors on Decrypt, simulating KMS unavailability.
type failingDecryptor struct{}

func (e *failingDecryptor) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	result := make([]byte, 0, len(plaintext)+4)
	result = append(result, []byte("ENC:")...)
	result = append(result, plaintext...)
	return result, nil
}

func (e *failingDecryptor) Decrypt(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
	return nil, errors.New("decrypt: KMS unavailable")
}

// failingEncryptor always returns an error on Encrypt, used to verify ordering.
type failingEncryptor struct{}

func (e *failingEncryptor) Encrypt(_ context.Context, _ []byte, _ map[string]string) ([]byte, error) {
	return nil, errors.New("encrypt: simulated failure")
}

func (e *failingEncryptor) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	if len(ciphertext) < 4 || string(ciphertext[:4]) != "ENC:" {
		return nil, assert.AnError
	}
	return ciphertext[4:], nil
}

func newTestSigningKeyService() (*SigningKeyService, *testSigningKeyStore) {
	repo := newTestSigningKeyStore()
	enc := &testEncryptor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewSigningKeyService(repo, repo, enc, newNoopBranchKeyManager(), logger), repo
}

func newTestSigningKeyServiceWithBranchKeyManager(bkm ports.BranchKeyManager) (*SigningKeyService, *testSigningKeyStore) {
	repo := newTestSigningKeyStore()
	enc := &testEncryptor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewSigningKeyService(repo, repo, enc, bkm, logger), repo
}

func newTestSigningKeyServiceWithBranchKeyManagerAndEncryptor(bkm ports.BranchKeyManager, enc ports.EncryptionPort, logBuf *bytes.Buffer) (*SigningKeyService, *testSigningKeyStore) {
	repo := newTestSigningKeyStore()
	var logger *slog.Logger
	if logBuf != nil {
		logger = slog.New(slog.NewJSONHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return NewSigningKeyService(repo, repo, enc, bkm, logger), repo
}

func TestNewSigningKeySubject(t *testing.T) {
	t.Run("valid kid", func(t *testing.T) {
		subject, err := newSigningKeySubject(id.NewKeyID("kid-123"))
		require.NoError(t, err)
		assert.Equal(t, domainencryption.BranchKeySubjectKindSigningKey, subject.Kind())
		assert.Equal(t, "kid-123", subject.Identifier())
	})

	t.Run("empty kid", func(t *testing.T) {
		_, err := newSigningKeySubject(id.KeyID(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signing key subject")
		assert.Contains(t, err.Error(), "kid is required")
	})
}

func TestEncodePEMBlock(t *testing.T) {
	t.Run("nil block returns error", func(t *testing.T) {
		encoded, err := encodePEMBlock(nil)
		require.Error(t, err)
		assert.Nil(t, encoded)
	})

	t.Run("valid block encodes to pem", func(t *testing.T) {
		encoded, err := encodePEMBlock(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("test-key")})
		require.NoError(t, err)
		assert.NotNil(t, encoded)
		assert.Contains(t, string(encoded), "BEGIN PRIVATE KEY")
	})
}

func TestSigningKeyService_GenerateAndStoreKey(t *testing.T) {
	t.Run("generates ES256 key pair", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)
		require.NotNil(t, key)

		assert.Equal(t, "ES256", key.Algorithm)
		assert.False(t, key.KID.IsZero())
		assert.True(t, len(key.PrivateKeyEncrypted) > 0)
	})

	t.Run("default algorithm is ES256", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		key, err := svc.GenerateAndStoreKey(context.Background(), "", true)
		require.NoError(t, err)
		assert.Equal(t, "ES256", key.Algorithm)
	})

	t.Run("unsupported algorithm rejected", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		_, err := svc.GenerateAndStoreKey(context.Background(), "RS384", true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported algorithm")
	})

	t.Run("makeCurrent marks key as is_current", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)

		// Key is stored as is_current=true even though it is in the grace period.
		stored, err := repo.GetByKIDInDomain(context.Background(), storage.KeyDomainTokenSigning, key.KID)
		require.NoError(t, err)
		assert.True(t, stored.IsCurrent)
	})

	t.Run("new current key has future activates_at (grace period)", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		before := time.Now()
		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)

		stored, err := repo.GetByKIDInDomain(context.Background(), storage.KeyDomainTokenSigning, key.KID)
		require.NoError(t, err)
		assert.True(t, stored.ActivatesAt.After(before.Add(jwksGracePeriod-time.Second)),
			"activates_at should be approximately now+jwksGracePeriod")

		// GetCurrent must not return this key while it is in its grace period.
		_, err = repo.GetCurrentInDomain(context.Background(), storage.KeyDomainTokenSigning)
		assert.Error(t, err, "key should not be available for signing during grace period")
	})

	t.Run("private key is encrypted (has ENC: prefix)", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)

		// Verify the stored key has the test encryption prefix
		assert.True(t, len(key.PrivateKeyEncrypted) > 4)
		assert.Equal(t, "ENC:", string(key.PrivateKeyEncrypted[:4]))
	})
}

func TestSigningKeyService_BuildJWKS(t *testing.T) {
	t.Run("JWKS contains active public keys", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()

		// Generate two keys
		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)
		key2, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)

		jwks, err := svc.BuildJWKS(context.Background())
		require.NoError(t, err)
		require.NotNil(t, jwks)

		// Should have 2 keys in the set
		assert.Equal(t, 2, jwks.Len())

		// Verify the second key is in the set
		found := false
		for i := 0; i < jwks.Len(); i++ {
			k, ok := jwks.Key(i)
			if !ok {
				continue
			}
			kid, ok := k.KeyID()
			if !ok {
				continue
			}
			if kid == key2.KID.String() {
				found = true
			}
		}
		assert.True(t, found, "JWKS should contain the second key")
	})

	t.Run("empty JWKS when no keys exist", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()

		jwks, err := svc.BuildJWKS(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, jwks.Len())
	})

	t.Run("returns error when all active keys fail processing", func(t *testing.T) {
		_, repo := newTestSigningKeyService()
		ctx := context.Background()

		// Build a key record with a bogus algorithm directly in the store,
		// bypassing GenerateAndStoreKey which rejects unknown algorithms.
		privPEM, err := generateES256KeyPEM()
		require.NoError(t, err)
		encrypted := append([]byte("ENC:"), privPEM...)

		kid := id.NewKeyID(uuid.New().String())
		err = repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 kid,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "BOGUS",
			PrivateKeyEncrypted: encrypted,
			IsCurrent:           true,
		})
		require.NoError(t, err)

		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())
		_, err = svc.BuildJWKS(ctx)
		require.Error(t, err, "all active keys failed processing — should return error")
		assert.Contains(t, err.Error(), "failed to build JWKS")
	})

	t.Run("returns error when KMS is down and all decrypts fail", func(t *testing.T) {
		ctx := context.Background()
		repo := newTestSigningKeyStore()

		// Store a key that cannot be decrypted (KMS-down scenario).
		kid := id.NewKeyID(uuid.New().String())
		err := repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 kid,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: []byte("ciphertext-that-will-fail-decrypt"),
			IsCurrent:           true,
		})
		require.NoError(t, err)

		// failingDecryptor simulates KMS unavailability.
		svc := NewSigningKeyService(repo, repo, &failingDecryptor{}, newNoopBranchKeyManager(), testSlogger())
		_, buildErr := svc.BuildJWKS(ctx)
		require.Error(t, buildErr, "KMS down — all decrypts fail — should return error")
		assert.Contains(t, buildErr.Error(), "failed to build JWKS")
	})

	t.Run("returns error when the current signer fails processing during grace-period fallback", func(t *testing.T) {
		ctx := context.Background()
		repo := newTestSigningKeyStore()

		privPEM, err := generateES256KeyPEM()
		require.NoError(t, err)

		fallbackKID := id.NewKeyID(uuid.New().String())
		err = repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 fallbackKID,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "BOGUS",
			PrivateKeyEncrypted: append([]byte("ENC:"), privPEM...),
			IsCurrent:           false,
			ActivatesAt:         time.Now().UTC().Add(-time.Minute),
		})
		require.NoError(t, err)

		futureCurrentKID := id.NewKeyID(uuid.New().String())
		err = repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 futureCurrentKID,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: append([]byte("ENC:"), privPEM...),
			IsCurrent:           true,
			ActivatesAt:         time.Now().UTC().Add(time.Hour),
		})
		require.NoError(t, err)

		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())
		_, buildErr := svc.BuildJWKS(ctx)
		require.Error(t, buildErr)
		assert.ErrorContains(t, buildErr, "current signing key")
	})

	t.Run("partial failure: future key skipped, set returned with current signer", func(t *testing.T) {
		ctx := context.Background()
		repo := newTestSigningKeyStore()

		// Key 1: valid, decryptable, and currently used for signing while the replacement key waits for activation.
		privPEM, err := generateES256KeyPEM()
		require.NoError(t, err)
		goodKID := id.NewKeyID(uuid.New().String())
		err = repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 goodKID,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: append([]byte("ENC:"), privPEM...),
			IsCurrent:           false,
			ActivatesAt:         time.Now().UTC().Add(-time.Minute),
		})
		require.NoError(t, err)

		// Key 2: bogus algorithm — will fail processing, but it is not yet used for signing.
		badKID := id.NewKeyID(uuid.New().String())
		err = repo.Create(ctx, &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 badKID,
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "BOGUS",
			PrivateKeyEncrypted: append([]byte("ENC:"), privPEM...),
			IsCurrent:           true,
			ActivatesAt:         time.Now().UTC().Add(time.Hour),
		})
		require.NoError(t, err)

		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())
		jwks, buildErr := svc.BuildJWKS(ctx)
		require.NoError(t, buildErr, "the currently used signing key still succeeded — should return partial JWKS")
		assert.Equal(t, 1, jwks.Len(), "only the good current signer should be in the set")

		k, ok := jwks.Key(0)
		require.True(t, ok)
		kid, _ := k.KeyID()
		assert.Equal(t, goodKID.String(), kid)
	})
}

type failingSetJWK struct {
	jwk.Key
	failOn string
	err    error
}

func (k *failingSetJWK) Set(name string, value interface{}) error {
	if name == k.failOn {
		return k.err
	}
	return k.Key.Set(name, value)
}

func TestSetJWKMetadata(t *testing.T) {
	newJWK := func(t *testing.T) jwk.Key {
		t.Helper()
		privPEM, err := generateES256KeyPEM()
		require.NoError(t, err)
		pubKey, err := publicKeyFromPEM(privPEM, "ES256")
		require.NoError(t, err)
		jwkKey, err := jwk.Import[jwk.Key](pubKey)
		require.NoError(t, err)
		return jwkKey
	}

	t.Run("sets kid alg and use", func(t *testing.T) {
		jwkKey := newJWK(t)

		err := setJWKMetadata(jwkKey, id.NewKeyID("test-kid"), jwa.ES256())
		require.NoError(t, err)

		kid, ok := jwkKey.KeyID()
		require.True(t, ok)
		assert.Equal(t, "test-kid", kid)

		alg, ok := jwkKey.Algorithm()
		require.True(t, ok)
		assert.Equal(t, jwa.ES256(), alg)

		use, ok := jwkKey.KeyUsage()
		require.True(t, ok)
		assert.Equal(t, "sig", use)
	})

	t.Run("returns error when Set fails", func(t *testing.T) {
		tests := []struct {
			name       string
			failOn     string
			wantErrMsg string
		}{
			{name: "kid", failOn: jwk.KeyIDKey, wantErrMsg: "failed to set kid"},
			{name: "alg", failOn: jwk.AlgorithmKey, wantErrMsg: "failed to set alg"},
			{name: "use", failOn: jwk.KeyUsageKey, wantErrMsg: "failed to set use"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				jwkKey := &failingSetJWK{
					Key:    newJWK(t),
					failOn: tt.failOn,
					err:    errors.New("set failed"),
				}

				err := setJWKMetadata(jwkKey, id.NewKeyID("test-kid"), jwa.ES256())
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErrMsg)
				assert.ErrorContains(t, err, "set failed")
			})
		}
	})
}

func TestAlgorithmToJWA(t *testing.T) {
	t.Run("ES256", func(t *testing.T) {
		alg, err := algorithmToJWA("ES256")
		require.NoError(t, err)
		assert.Equal(t, jwa.ES256(), alg)
	})

	t.Run("RS256", func(t *testing.T) {
		alg, err := algorithmToJWA("RS256")
		require.NoError(t, err)
		assert.Equal(t, jwa.RS256(), alg)
	})

	t.Run("unrecognized algorithm returns error and zero value", func(t *testing.T) {
		alg, err := algorithmToJWA("BOGUS")
		assert.ErrorContains(t, err, "unrecognized algorithm")
		assert.Equal(t, jwa.SignatureAlgorithm{}, alg)
	})
}

func TestSigningKeyService_EnsureInitialKey(t *testing.T) {
	t.Run("creates one immediately-active key when none exist", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)

		count, err := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		current, err := repo.GetCurrentInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, key.KID, current.KID)
		assert.False(t, current.ActivatesAt.After(time.Now().Add(time.Second)))
	})

	t.Run("is a no-op when a key already exists", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		existing, err := svc.generateAndStore(context.Background(), "ES256", true, time.Now().UTC())
		require.NoError(t, err)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		assert.False(t, created)
		assert.Nil(t, key)

		count, err := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		current, err := repo.GetCurrentInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, existing.KID, current.KID)
	})

	t.Run("returns error when generateAndStore fails inside bootstrap lock", func(t *testing.T) {
		svc, repo := newTestSigningKeyServiceWithBranchKeyManagerAndEncryptor(newNoopBranchKeyManager(), &failingEncryptor{}, nil)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to generate initial signing key")
		assert.Nil(t, key)
		assert.False(t, created)

		count, countErr := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, countErr)
		assert.Equal(t, 0, count)
	})

	t.Run("returns error when CountActive fails inside bootstrap lock", func(t *testing.T) {
		repo := &countActiveFailingRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			countErr:            errors.New("count failed"),
		}
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to count active keys")
		assert.ErrorContains(t, err, "count failed")
		assert.Nil(t, key)
		assert.False(t, created)

		keys, listErr := repo.ListActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, listErr)
		assert.Empty(t, keys)
	})

	t.Run("recovers when bootstrap lock times out after the key is persisted", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		var logBuf bytes.Buffer
		repo := &recoveryCountContextRepo{testSigningKeyStore: newTestSigningKeyStore()}
		coordinator := &postCallbackErrorBootstrapCoordinator{
			err: context.DeadlineExceeded,
			afterSuccess: func() {
				repo.rejectCanceledCountContext = true
				cancel()
			},
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		svc := NewSigningKeyService(repo, coordinator, &testEncryptor{}, newNoopBranchKeyManager(), logger)

		key, created, err := svc.EnsureInitialKey(ctx, "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.True(t, repo.sawRecoveryCount)

		count, countErr := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, countErr)
		assert.Equal(t, 1, count)
		assert.Contains(t, logBuf.String(), "bootstrap lock")
	})

	t.Run("uses a bounded timeout for the recovery count probe", func(t *testing.T) {
		repo := &recoveryCountProbeRepo{testSigningKeyStore: newTestSigningKeyStore()}
		svc := NewSigningKeyService(
			repo,
			&postCallbackErrorBootstrapCoordinator{err: context.DeadlineExceeded},
			&testEncryptor{},
			newNoopBranchKeyManager(),
			testSlogger(),
		)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.True(t, repo.sawRecoveryCount)
		assert.True(t, repo.recoveryCountHasDeadline)
	})

	t.Run("recovers when bootstrap lock returns a timeout storage error after the key is persisted", func(t *testing.T) {
		var logBuf bytes.Buffer
		repo := &recoveryCountProbeRepo{testSigningKeyStore: newTestSigningKeyStore()}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		svc := NewSigningKeyService(
			repo,
			&postCallbackErrorBootstrapCoordinator{
				err: storage.NewStorageError(
					"SigningKeyRepo.WithBootstrapLock",
					storage.ErrorKindTimeout,
					errors.New("lock timed out after callback"),
					"operation exceeded timeout",
				),
			},
			&testEncryptor{},
			newNoopBranchKeyManager(),
			logger,
		)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.True(t, repo.sawRecoveryCount)
		assert.Contains(t, logBuf.String(), "bootstrap lock")
	})

	t.Run("logs recovery probe failure before returning the bootstrap error", func(t *testing.T) {
		var logBuf bytes.Buffer
		repo := &recoveryCountProbeRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			recoveryCountErr:    errors.New("count failed"),
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		svc := NewSigningKeyService(
			repo,
			&postCallbackErrorBootstrapCoordinator{err: context.DeadlineExceeded},
			&testEncryptor{},
			newNoopBranchKeyManager(),
			logger,
		)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Nil(t, key)
		assert.False(t, created)
		assert.True(t, repo.sawRecoveryCount)
		assert.Contains(t, logBuf.String(), "recovery count probe failed")
		assert.Contains(t, logBuf.String(), "count failed")
	})

	t.Run("recovers when bootstrap lock commit fails after the key is persisted", func(t *testing.T) {
		var logBuf bytes.Buffer
		repo := &recoveryCountProbeRepo{testSigningKeyStore: newTestSigningKeyStore()}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		svc := NewSigningKeyService(
			repo,
			&postCallbackErrorBootstrapCoordinator{
				err: storage.NewStorageError(
					"SigningKeyRepo.WithBootstrapLock",
					storage.ErrorKindConnection,
					errors.New("commit failed"),
					"failed to commit transaction",
				),
			},
			&testEncryptor{},
			newNoopBranchKeyManager(),
			logger,
		)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.True(t, repo.sawRecoveryCount)
		assert.Contains(t, logBuf.String(), "bootstrap lock")
	})

	t.Run("uses bootstrap context for all initial-key work", func(t *testing.T) {
		const marker = "bootstrap-lock"

		repo := &bootstrapContextCheckingRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			marker:              marker,
		}
		coordinator := &bootstrapContextCheckingCoordinator{marker: marker}
		enc := &contextCheckingEncryptor{marker: marker}
		sawBranchKeyCreate := false
		bkm := &mockBranchKeyManager{
			createFn: func(ctx context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				if ctx.Value(bootstrapContextKey{}) == marker {
					sawBranchKeyCreate = true
				}
				return "branch-key-id", nil
			},
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		svc := NewSigningKeyService(repo, coordinator, enc, bkm, logger)

		key, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.True(t, repo.sawCountActive)
		assert.True(t, repo.sawCreateAndSetCurrent)
		assert.True(t, sawBranchKeyCreate)
		assert.True(t, enc.sawEncrypt)
	})

	t.Run("concurrent callers create only one initial key", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()

		const callers = 8
		var wg sync.WaitGroup
		wg.Add(callers)

		createdCount := 0
		var createdMu sync.Mutex
		errCh := make(chan error, callers)

		for range callers {
			go func() {
				defer wg.Done()
				_, created, err := svc.EnsureInitialKey(context.Background(), "ES256")
				if err != nil {
					errCh <- err
					return
				}
				if created {
					createdMu.Lock()
					createdCount++
					createdMu.Unlock()
				}
			}()
		}

		wg.Wait()
		close(errCh)
		for err := range errCh {
			require.NoError(t, err)
		}

		count, err := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		assert.Equal(t, 1, createdCount)
		current, err := repo.GetCurrentInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.False(t, current.ActivatesAt.After(time.Now().Add(time.Second)))
	})
}

func TestSigningKeyService_GenerateAndStoreKey_Atomic(t *testing.T) {
	t.Run("CreateAndSetCurrent demotes previous current key", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		key1, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		require.True(t, key1.IsCurrent)

		key2, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		require.True(t, key2.IsCurrent)

		// key1 must no longer be current.
		stored1, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key1.KID)
		require.NoError(t, err)
		assert.False(t, stored1.IsCurrent, "previous key must be demoted")

		// key2 is flagged is_current but still in grace period — GetCurrent falls back to key1.
		stored2, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key2.KID)
		require.NoError(t, err)
		assert.True(t, stored2.IsCurrent, "key2 must be flagged is_current")
	})
}

func TestSigningKeyService_DecryptPrivateKey(t *testing.T) {
	t.Run("decrypt round-trip", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.NoError(t, err)

		decrypted, err := svc.DecryptPrivateKey(context.Background(), key)
		require.NoError(t, err)
		assert.True(t, len(decrypted) > 0)
		block, _ := pem.Decode(decrypted)
		require.NotNil(t, block)
		assert.Equal(t, "PRIVATE KEY", block.Type)
	})
}

func TestSigningKeyService_AdminOperations(t *testing.T) {
	t.Run("add key becomes current and previous is demoted", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		key1, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)

		key2, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)

		// key1 should no longer be current
		k1, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key1.KID)
		require.NoError(t, err)
		assert.False(t, k1.IsCurrent, "key1 should no longer be current after key2 is added as current")

		// key2 should be current
		k2, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key2.KID)
		require.NoError(t, err)
		assert.True(t, k2.IsCurrent, "key2 should be current")
	})

	t.Run("list returns metadata only", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		ctx := context.Background()

		key1, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		_, err = svc.GenerateAndStoreKey(ctx, "ES256", false)
		require.NoError(t, err)

		keys, err := svc.ListKeys(ctx)
		require.NoError(t, err)
		require.Len(t, keys, 2)

		// Verify metadata fields are present on all keys
		for _, k := range keys {
			assert.False(t, k.KID.IsZero(), "KID should not be zero")
			assert.Equal(t, "ES256", k.Algorithm)
			assert.True(t, len(k.PrivateKeyEncrypted) > 0, "encrypted private key should be stored")
		}

		// Verify is_current status
		kidToCurrent := make(map[string]bool)
		for _, k := range keys {
			kidToCurrent[k.KID.String()] = k.IsCurrent
		}
		assert.True(t, kidToCurrent[key1.KID.String()], "key1 should be current")
	})

	t.Run("promote key changes current", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		key1, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		key2, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)

		// key2 is current; promote key1 back
		promoted, err := svc.PromoteKey(ctx, key1.KID)
		require.NoError(t, err)
		assert.Equal(t, key1.KID, promoted.KID)
		assert.True(t, promoted.IsCurrent)

		k1, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key1.KID)
		require.NoError(t, err)
		assert.True(t, k1.IsCurrent, "key1 should be current after promotion")

		k2, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key2.KID)
		require.NoError(t, err)
		assert.False(t, k2.IsCurrent, "key2 should no longer be current")
	})

	t.Run("remove non-current succeeds", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		_, err := svc.generateAndStore(ctx, "ES256", true, time.Now().UTC()) // key1 is current and already active
		require.NoError(t, err)
		key2, err := svc.GenerateAndStoreKey(ctx, "ES256", false) // key2 is not current
		require.NoError(t, err)

		err = svc.DeleteKey(ctx, key2.KID)
		require.NoError(t, err)

		keys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		require.Len(t, keys, 1, "only one key should remain after deletion")

		// key2 should not be in active list
		for _, k := range keys {
			assert.NotEqual(t, key2.KID, k.KID, "deleted key should not appear in active list")
		}
	})

	t.Run("remove current key returns conflict", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		ctx := context.Background()

		current, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		_, err = svc.GenerateAndStoreKey(ctx, "ES256", false)
		require.NoError(t, err)

		err = svc.DeleteKey(ctx, current.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrCurrentKey)
	})

	t.Run("remove currently usable grace-period fallback returns conflict", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		fallback, err := svc.generateAndStore(ctx, "ES256", true, time.Now().UTC())
		require.NoError(t, err)
		replacement, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)
		assert.NotEqual(t, fallback.KID, replacement.KID)

		current, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, fallback.KID, current.KID)

		err = svc.DeleteKey(ctx, fallback.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)

		stored, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, fallback.KID)
		require.NoError(t, err)
		assert.Nil(t, stored.RemovedAt)
	})

	t.Run("remove last key returns conflict", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		ctx := context.Background()

		key1, err := svc.GenerateAndStoreKey(ctx, "ES256", true)
		require.NoError(t, err)

		// Attempt to remove the only key via service — should fail
		err = svc.DeleteKey(ctx, key1.KID)
		assert.Error(t, err, "should not allow removing the last active signing key")
	})
}

func TestSigningKeyService_DeleteKey_PreflightValidation(t *testing.T) {
	t.Run("returns wrapped not found before last-key validation when the target kid is missing", func(t *testing.T) {
		kid := id.NewKeyID("missing-key")
		repo := &deleteValidationSpyRepo{count: 1}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
		assert.ErrorContains(t, err, "failed to get signing key")
		assert.False(t, repo.deleteCalled)
	})

	t.Run("rejects deleting the last active key before reaching the repository delete path", func(t *testing.T) {
		kid := id.NewKeyID("last-key")
		repo := &deleteValidationSpyRepo{
			count: 1,
			key: &storage.SigningKey{
				KID:       kid,
				IsCurrent: false,
			},
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.ErrorIs(t, err, ports.ErrLastActiveKey)
		assert.False(t, repo.deleteCalled)
	})

	t.Run("rejects deleting the current key before reaching the repository delete path", func(t *testing.T) {
		kid := id.NewKeyID("current-key")
		repo := &deleteValidationSpyRepo{
			count: 2,
			key: &storage.SigningKey{
				KID:       kid,
				IsCurrent: true,
			},
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.ErrorIs(t, err, ports.ErrCurrentKey)
		assert.False(t, repo.deleteCalled)
	})

	t.Run("rejects deleting the currently usable fallback key before reaching the repository delete path", func(t *testing.T) {
		now := time.Now().UTC()
		kid := id.NewKeyID("fallback-key")
		repo := &deleteValidationSpyRepo{
			count: 2,
			key: &storage.SigningKey{
				KID:         kid,
				IsCurrent:   false,
				ActivatesAt: now.Add(-time.Minute),
			},
			activeKeys: []*storage.SigningKey{
				{
					KID:         kid,
					IsCurrent:   false,
					ActivatesAt: now.Add(-time.Minute),
				},
				{
					KID:         id.NewKeyID("future-current"),
					IsCurrent:   true,
					ActivatesAt: now.Add(time.Hour),
				},
			},
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)
		assert.False(t, repo.deleteCalled)
	})

	t.Run("wraps list-active failures before deleting a key", func(t *testing.T) {
		now := time.Now().UTC()
		kid := id.NewKeyID("list-active-error")
		repo := &deleteValidationSpyRepo{
			count: 2,
			key: &storage.SigningKey{
				KID:         kid,
				IsCurrent:   false,
				ActivatesAt: now.Add(-2 * time.Hour),
			},
			listActiveErr: errors.New("list failed"),
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to list active keys")
		assert.ErrorContains(t, err, "list failed")
		assert.False(t, repo.deleteCalled)
	})

	t.Run("propagates repository delete failures after preflight validation passes", func(t *testing.T) {
		now := time.Now().UTC()
		kid := id.NewKeyID("delete-error")
		deleteErr := errors.New("delete failed")
		repo := &deleteValidationSpyRepo{
			count: 2,
			key: &storage.SigningKey{
				KID:         kid,
				IsCurrent:   false,
				ActivatesAt: now.Add(-2 * time.Hour),
			},
			activeKeys: []*storage.SigningKey{
				{
					KID:         kid,
					IsCurrent:   false,
					ActivatesAt: now.Add(-2 * time.Hour),
				},
				{
					KID:         id.NewKeyID("current-key"),
					IsCurrent:   true,
					ActivatesAt: now.Add(-time.Hour),
				},
			},
			deleteErr: deleteErr,
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.ErrorIs(t, err, deleteErr)
		assert.True(t, repo.deleteCalled)
	})

	t.Run("does not perform a redundant GetCurrent read before deleting a non-current key", func(t *testing.T) {
		kid := id.NewKeyID("non-current-key")
		repo := &deleteValidationSpyRepo{
			count:      2,
			currentErr: errors.New("GetCurrent should not be called"),
			key: &storage.SigningKey{
				KID:       kid,
				IsCurrent: false,
			},
		}
		svc := NewSigningKeyService(repo, &bootstrapContextCheckingCoordinator{}, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())

		err := svc.DeleteKey(context.Background(), kid)
		require.NoError(t, err)
		assert.True(t, repo.deleteCalled)
	})
}

func TestSigningKeyService_BranchKeyProvisioning(t *testing.T) {
	t.Run("happy path: branch key created, key stored and decryptable", func(t *testing.T) {
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "branch-key-id", nil
			},
		}
		svc, repo := newTestSigningKeyServiceWithBranchKeyManager(bkm)

		key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)
		require.NotNil(t, key)

		// Branch key was provisioned exactly once for the signing-key subject.
		assert.Equal(t, 1, bkm.createCalls)
		assert.Equal(t, domainencryption.BranchKeySubjectKindSigningKey, bkm.lastSubject.Kind())
		assert.Equal(t, key.KID.String(), bkm.lastSubject.Identifier())

		// Key was stored in the repo.
		stored, err := repo.GetByKIDInDomain(context.Background(), storage.KeyDomainTokenSigning, key.KID)
		require.NoError(t, err)
		assert.Equal(t, key.KID, stored.KID)

		// Private key material is decryptable.
		decrypted, err := svc.DecryptPrivateKey(context.Background(), stored)
		require.NoError(t, err)
		block, _ := pem.Decode(decrypted)
		require.NotNil(t, block)
		assert.Equal(t, "PRIVATE KEY", block.Type)
	})

	t.Run("Create failure: error propagates, nothing stored", func(t *testing.T) {
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "", errors.New("dynamo down")
			},
		}
		svc, repo := newTestSigningKeyServiceWithBranchKeyManager(bkm)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to provision branch key")

		// Nothing should have been stored.
		count, countErr := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, countErr)
		assert.Equal(t, 0, count)
	})

	t.Run("ordering regression: branch key Created before Encrypt is called", func(t *testing.T) {
		// Verifies branchKeyManager.Create is called before Encrypt.
		// Because failingEncryptor always errors on Encrypt, if Create were called after Encrypt
		// (or not at all), createCalls would be 0. createCalls == 1 proves Create ran first.
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "branch-key-id", nil
			},
		}
		repo := newTestSigningKeyStore()
		enc := &failingEncryptor{}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		svc := NewSigningKeyService(repo, repo, enc, bkm, logger)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		// Create must have been called before Encrypt was attempted.
		assert.Equal(t, 1, bkm.createCalls, "branchKeyManager.Create must be called before Encrypt")
	})
}

// mockFailingSigningKeyRepo wraps testSigningKeyStore and fails Create/CreateAndSetCurrent.
type mockFailingSigningKeyRepo struct {
	*testSigningKeyStore
	createErr error
}

func (r *mockFailingSigningKeyRepo) Create(_ context.Context, _ *storage.SigningKey) error {
	return r.createErr
}

func (r *mockFailingSigningKeyRepo) CreateAndSetCurrent(_ context.Context, _ *storage.SigningKey) error {
	return r.createErr
}

type promoteKeyReadbackFailingRepo struct {
	*testSigningKeyStore
}

func (r *promoteKeyReadbackFailingRepo) GetByKIDInDomain(_ context.Context, _ storage.KeyDomain, _ id.KeyID) (*storage.SigningKey, error) {
	return nil, errors.New("readback failed")
}

func (r *promoteKeyReadbackFailingRepo) SetCurrentInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	return r.testSigningKeyStore.SetCurrentInDomain(ctx, domain, kid, activatesAt)
}

type promoteKeyActivationSpyRepo struct {
	*testSigningKeyStore
	lastActivatesAt time.Time
}

func (r *promoteKeyActivationSpyRepo) SetCurrentInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	r.lastActivatesAt = activatesAt
	return r.testSigningKeyStore.SetCurrentInDomain(ctx, domain, kid, activatesAt)
}

type countActiveFailingRepo struct {
	*testSigningKeyStore
	countErr error
}

func (r *countActiveFailingRepo) CountActiveInDomain(_ context.Context, _ storage.KeyDomain) (int, error) {
	return 0, r.countErr
}

type deleteValidationSpyRepo struct {
	count         int
	key           *storage.SigningKey
	current       *storage.SigningKey
	currentErr    error
	activeKeys    []*storage.SigningKey
	listActiveErr error
	deleteErr     error
	deleteCalled  bool
}

func (r *deleteValidationSpyRepo) Create(context.Context, *storage.SigningKey) error {
	return nil
}

func (r *deleteValidationSpyRepo) CreateAndSetCurrent(context.Context, *storage.SigningKey) error {
	return nil
}

func (r *deleteValidationSpyRepo) GetByKIDInDomain(_ context.Context, domain storage.KeyDomain, _ id.KeyID) (*storage.SigningKey, error) {
	if domain != storage.KeyDomainTokenSigning || r.key == nil {
		return nil, storage.NewStorageError("deleteValidationSpyRepo.GetByKIDInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	return r.key, nil
}

func (r *deleteValidationSpyRepo) GetCurrentInDomain(_ context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	if r.currentErr != nil {
		return nil, r.currentErr
	}
	if domain != storage.KeyDomainTokenSigning || r.current == nil {
		return nil, storage.NewStorageError("deleteValidationSpyRepo.GetCurrentInDomain", storage.ErrorKindNotFound, nil, "no current signing key")
	}
	return r.current, nil
}

func (r *deleteValidationSpyRepo) ListActiveInDomain(_ context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	if domain != storage.KeyDomainTokenSigning {
		return nil, nil
	}
	if r.listActiveErr != nil {
		return nil, r.listActiveErr
	}
	return r.activeKeys, nil
}

func (r *deleteValidationSpyRepo) SetCurrentInDomain(_ context.Context, _ storage.KeyDomain, _ id.KeyID, _ time.Time) (*storage.SigningKey, error) {
	return nil, nil
}

func (r *deleteValidationSpyRepo) DeleteInDomain(_ context.Context, domain storage.KeyDomain, _ id.KeyID) error {
	if domain != storage.KeyDomainTokenSigning {
		return storage.NewStorageError("deleteValidationSpyRepo.DeleteInDomain", storage.ErrorKindNotFound, nil, "signing key not found")
	}
	r.deleteCalled = true
	return r.deleteErr
}

func (r *deleteValidationSpyRepo) CountActiveInDomain(_ context.Context, domain storage.KeyDomain) (int, error) {
	if domain != storage.KeyDomainTokenSigning {
		return 0, nil
	}
	return r.count, nil
}

func TestSigningKeyService_OrphanedBranchKeyWarning(t *testing.T) {
	t.Run("warns with kid when Encrypt fails after branch key created", func(t *testing.T) {
		var logBuf bytes.Buffer
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "branch-key-id", nil
			},
		}
		svc, _ := newTestSigningKeyServiceWithBranchKeyManagerAndEncryptor(bkm, &failingEncryptor{}, &logBuf)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "orphaned branch key", "warn log must identify the orphaned entry")
		assert.Contains(t, logOutput, "kid", "warn log must include the kid field")
		assert.Contains(t, logOutput, "branch-key-id", "warn log must include the branch key ID for operator cleanup")
	})

	t.Run("warns with kid when repo.Create fails after branch key created", func(t *testing.T) {
		var logBuf bytes.Buffer
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "branch-key-id", nil
			},
		}
		repo := &mockFailingSigningKeyRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			createErr:           errors.New("storage unavailable"),
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, bkm, logger)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "orphaned branch key", "warn log must identify the orphaned entry")
		assert.Contains(t, logOutput, "kid", "warn log must include the kid field")
		assert.Contains(t, logOutput, "branch-key-id", "warn log must include the branch key ID for operator cleanup")
	})

	t.Run("warns with kid when repo.CreateAndSetCurrent fails after branch key created", func(t *testing.T) {
		var logBuf bytes.Buffer
		bkm := &mockBranchKeyManager{
			createFn: func(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
				return "branch-key-id", nil
			},
		}
		repo := &mockFailingSigningKeyRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			createErr:           errors.New("storage unavailable"),
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, bkm, logger)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "orphaned branch key", "warn log must identify the orphaned entry")
		assert.Contains(t, logOutput, "kid", "warn log must include the kid field")
		assert.Contains(t, logOutput, "branch-key-id", "warn log must include the branch key ID for operator cleanup")
	})

	t.Run("logs debug breadcrumb when branch key id is empty", func(t *testing.T) {
		var logBuf bytes.Buffer
		svc, _ := newTestSigningKeyServiceWithBranchKeyManagerAndEncryptor(newNoopBranchKeyManager(), &failingEncryptor{}, &logBuf)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.Contains(t, logOutput, "branch key ID is empty", "empty branch key IDs must leave a diagnostic breadcrumb")
		assert.Contains(t, logOutput, "kid", "debug breadcrumb must retain the signing key kid")
		assert.NotContains(t, logOutput, "orphaned branch key", "empty branch key IDs must not emit manual-cleanup warnings")
		assert.NotContains(t, logOutput, "branch_key_id", "empty branch key IDs must not log a branch_key_id field")
	})

	t.Run("noop branch key manager does not warn about cleanup when Encrypt fails", func(t *testing.T) {
		var logBuf bytes.Buffer
		svc, _ := newTestSigningKeyServiceWithBranchKeyManagerAndEncryptor(newNoopBranchKeyManager(), &failingEncryptor{}, &logBuf)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.NotContains(t, logOutput, "orphaned branch key", "noop branch key managers must not emit manual-cleanup warnings")
		assert.NotContains(t, logOutput, "branch_key_id", "noop branch key managers must not log an empty branch_key_id")
	})

	t.Run("noop branch key manager does not warn about cleanup when repo.Create fails", func(t *testing.T) {
		var logBuf bytes.Buffer
		repo := &mockFailingSigningKeyRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			createErr:           errors.New("storage unavailable"),
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), logger)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.NotContains(t, logOutput, "orphaned branch key", "noop branch key managers must not emit manual-cleanup warnings")
		assert.NotContains(t, logOutput, "branch_key_id", "noop branch key managers must not log an empty branch_key_id")
	})

	t.Run("noop branch key manager does not warn about cleanup when repo.CreateAndSetCurrent fails", func(t *testing.T) {
		var logBuf bytes.Buffer
		repo := &mockFailingSigningKeyRepo{
			testSigningKeyStore: newTestSigningKeyStore(),
			createErr:           errors.New("storage unavailable"),
		}
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), logger)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", true)
		require.Error(t, err)

		logOutput := logBuf.String()
		assert.NotContains(t, logOutput, "orphaned branch key", "noop branch key managers must not emit manual-cleanup warnings")
		assert.NotContains(t, logOutput, "branch_key_id", "noop branch key managers must not log an empty branch_key_id")
	})

	t.Run("default mock branch key manager never causes provisioning failure", func(t *testing.T) {
		// The default mock returns success and must never block key generation.
		bkm := newNoopBranchKeyManager()
		svc, repo := newTestSigningKeyServiceWithBranchKeyManager(bkm)

		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)

		count, countErr := repo.CountActiveInDomain(context.Background(), storage.KeyDomainTokenSigning)
		require.NoError(t, countErr)
		assert.Equal(t, 1, count)
	})
}

func TestSigningKeyEncCtx(t *testing.T) {
	t.Run("value is bare kid UUID with no prefix", func(t *testing.T) {
		kid := id.NewKeyID("550e8400-e29b-41d4-a716-446655440000")
		ctx := signingKeyEncCtx(kid)
		require.Len(t, ctx, 1, "encryption context must contain exactly one key")
		assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", ctx["kid"],
			"kid must be the signing key identifier used in AAD")
	})

	t.Run("round trips through BranchKeySubjectFromEncryptionContext", func(t *testing.T) {
		kid := id.NewKeyID("550e8400-e29b-41d4-a716-446655440000")
		ctx := signingKeyEncCtx(kid)

		subject, err := domainencryption.BranchKeySubjectFromEncryptionContext(ctx)
		require.NoError(t, err)
		assert.Equal(t, domainencryption.BranchKeySubjectKindSigningKey, subject.Kind())

		extractedKID, ok := subject.KeyID()
		require.True(t, ok)
		assert.Equal(t, kid, extractedKID)
	})
}

func TestSigningKeyService_KeyDomainIsolation(t *testing.T) {
	t.Run("selects and lists token signing keys independently", func(t *testing.T) {
		ctx := context.Background()
		repo := newTestSigningKeyStore()
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), testSlogger())
		now := time.Now().UTC()

		newKey := func(kidValue string, domain storage.KeyDomain, isCurrent bool, activatesAt time.Time) *storage.SigningKey {
			return &storage.SigningKey{
				ID:                  id.NewSigningKeyID(),
				KID:                 id.NewKeyID(kidValue),
				KeyDomain:           domain,
				Algorithm:           "ES256",
				PrivateKeyEncrypted: []byte("ciphertext"),
				IsCurrent:           isCurrent,
				ActivatesAt:         activatesAt,
				CreatedAt:           now,
			}
		}

		fallback := newKey("token-fallback", storage.KeyDomainTokenSigning, false, now.Add(-time.Minute))
		pending := newKey("token-pending", storage.KeyDomainTokenSigning, true, now.Add(time.Hour))
		cimdCurrent := newKey("cimd-current", storage.KeyDomainCIMDClientAuthentication, true, now)
		require.NoError(t, repo.Create(ctx, fallback))
		require.NoError(t, repo.Create(ctx, pending))
		require.NoError(t, repo.Create(ctx, cimdCurrent))

		current, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, fallback.KID, current.KID)

		serviceCurrent, err := svc.GetCurrent(ctx)
		require.NoError(t, err)
		assert.Equal(t, fallback.KID, serviceCurrent.KID)

		keys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Len(t, keys, 2)
	})

	t.Run("keeps key identifiers globally unique", func(t *testing.T) {
		ctx := context.Background()
		repo := newTestSigningKeyStore()
		now := time.Now().UTC()
		tokenKey := &storage.SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 id.NewKeyID("shared-kid"),
			KeyDomain:           storage.KeyDomainTokenSigning,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: []byte("ciphertext"),
			IsCurrent:           true,
			ActivatesAt:         now,
			CreatedAt:           now,
		}
		cimdKey := *tokenKey
		cimdKey.ID = id.NewSigningKeyID()
		cimdKey.KeyDomain = storage.KeyDomainCIMDClientAuthentication

		require.NoError(t, repo.Create(ctx, tokenKey))
		assert.Error(t, repo.Create(ctx, &cimdKey))
	})
}

func TestSigningKeyService_GeneratedTokenKeysUseTokenDomain(t *testing.T) {
	svc, _ := newTestSigningKeyService()

	key, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
	require.NoError(t, err)
	assert.Equal(t, storage.KeyDomainTokenSigning, key.KeyDomain)
}

func TestSigningKeyService_ListKeys(t *testing.T) {
	t.Run("returns active keys from repository", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		first, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)
		second, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)

		keys, err := svc.ListKeys(context.Background())
		require.NoError(t, err)
		require.Len(t, keys, 2)

		kids := []id.KeyID{keys[0].KID, keys[1].KID}
		assert.Contains(t, kids, first.KID)
		assert.Contains(t, kids, second.KID)
	})
}

func TestSigningKeyService_PromoteKey(t *testing.T) {
	t.Run("promotes key and returns updated metadata", func(t *testing.T) {
		svc, repo := newTestSigningKeyService()
		ctx := context.Background()

		current, err := svc.generateAndStore(ctx, "ES256", true, time.Now().UTC())
		require.NoError(t, err)
		candidate, err := svc.GenerateAndStoreKey(ctx, "ES256", false)
		require.NoError(t, err)

		promoted, err := svc.PromoteKey(ctx, candidate.KID)
		require.NoError(t, err)
		assert.Equal(t, candidate.KID, promoted.KID)
		assert.True(t, promoted.IsCurrent)

		storedCurrent, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, current.KID)
		require.NoError(t, err)
		assert.False(t, storedCurrent.IsCurrent)
	})

	t.Run("returns not found when promoted key does not exist", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()

		_, err := svc.PromoteKey(context.Background(), id.NewKeyID("missing-kid"))
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
		assert.ErrorContains(t, err, "signing key not found")
	})

	t.Run("passes immediate activation time from domain to repository", func(t *testing.T) {
		repo := &promoteKeyActivationSpyRepo{testSigningKeyStore: newTestSigningKeyStore()}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), logger)
		ctx := context.Background()

		_, err := svc.generateAndStore(ctx, "ES256", true, time.Now().UTC())
		require.NoError(t, err)
		candidate, err := svc.GenerateAndStoreKey(ctx, "ES256", false)
		require.NoError(t, err)

		before := time.Now().UTC()
		promoted, err := svc.PromoteKey(ctx, candidate.KID)
		after := time.Now().UTC()
		require.NoError(t, err)
		assert.Equal(t, candidate.KID, promoted.KID)
		assert.True(t, promoted.IsCurrent)
		assert.False(t, repo.lastActivatesAt.IsZero())
		assert.False(t, repo.lastActivatesAt.Before(before))
		assert.False(t, repo.lastActivatesAt.After(after))
		assert.WithinDuration(t, repo.lastActivatesAt, promoted.ActivatesAt, time.Millisecond)
	})

	t.Run("returns promoted key without a separate read-back call", func(t *testing.T) {
		repo := &promoteKeyReadbackFailingRepo{testSigningKeyStore: newTestSigningKeyStore()}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		svc := NewSigningKeyService(repo, repo, &testEncryptor{}, newNoopBranchKeyManager(), logger)
		ctx := context.Background()

		_, err := svc.generateAndStore(ctx, "ES256", true, time.Now().UTC())
		require.NoError(t, err)
		candidate, err := svc.GenerateAndStoreKey(ctx, "ES256", false)
		require.NoError(t, err)

		promoted, err := svc.PromoteKey(ctx, candidate.KID)
		require.NoError(t, err)
		assert.Equal(t, candidate.KID, promoted.KID)
		assert.True(t, promoted.IsCurrent)
	})
}

func TestSigningKeyService_CountActive(t *testing.T) {
	t.Run("returns zero when no keys exist", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		count, err := svc.CountActive(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("reflects number of active keys", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		_, err := svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)
		_, err = svc.GenerateAndStoreKey(context.Background(), "ES256", false)
		require.NoError(t, err)

		count, err := svc.CountActive(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func TestSigningKeyService_GetCurrent(t *testing.T) {
	t.Run("returns error when no keys exist", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		_, err := svc.GetCurrent(context.Background())
		assert.Error(t, err)
	})

	t.Run("returns the current active key", func(t *testing.T) {
		svc, _ := newTestSigningKeyService()
		// Use generateAndStore with time.Now() so activates_at is in the past.
		key, err := svc.generateAndStore(context.Background(), "ES256", true, time.Now())
		require.NoError(t, err)

		got, err := svc.GetCurrent(context.Background())
		require.NoError(t, err)
		assert.Equal(t, key.KID, got.KID)
	})
}
