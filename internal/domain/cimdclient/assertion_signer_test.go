package cimdclient

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestAssertionSigner_SignsInteroperableCIMDClientAssertions(t *testing.T) {
	const (
		clientID      = "https://broker.example.com/.well-known/oauth-client/service-123"
		tokenEndpoint = "https://issuer.example.com/oauth/token"
		keyID         = "cimd-assertion-key"
	)

	var logOutput bytes.Buffer
	key := newAssertionSignerKey(keyID, newCIMDTestES256PEM(t), time.Now().UTC().Add(-time.Minute))
	keyService := newAssertionSignerKeyService([]*storage.SigningKey{key}, &assertionSignerEncryptor{}, slog.New(slog.NewJSONHandler(&logOutput, nil)))
	signer := NewAssertionSigner(keyService)

	first, err := signer.SignClientAssertion(context.Background(), id.ClientID(clientID), tokenEndpoint)
	require.NoError(t, err)
	second, err := signer.SignClientAssertion(context.Background(), id.ClientID(clientID), tokenEndpoint)
	require.NoError(t, err)

	publicJWKSet, err := keyService.PublicJWKSet(context.Background())
	require.NoError(t, err)

	firstClaims := assertValidCIMDClientAssertion(t, first, publicJWKSet, keyID, clientID, tokenEndpoint)
	secondClaims := assertValidCIMDClientAssertion(t, second, publicJWKSet, keyID, clientID, tokenEndpoint)
	firstJTI, ok := firstClaims.JwtID()
	require.True(t, ok, "client assertion must carry a jti")
	secondJTI, ok := secondClaims.JwtID()
	require.True(t, ok, "client assertion must carry a jti")
	assert.NotEmpty(t, firstJTI)
	assert.NotEmpty(t, secondJTI)
	assert.NotEqual(t, firstJTI, secondJTI, "each outbound client assertion must have a fresh jti")

	records := assertionSignerAuditRecords(t, logOutput.String())
	require.Len(t, records, 2)
	assertAssertionSigningAuditRecord(t, records, keyID, "success")
	assertCredentialFreeAssertionSigningAudit(t, records, logOutput.String())
}

func TestAssertionSigner_FailsClosedForUnusableKeysAndSigningFailures(t *testing.T) {
	const (
		clientID      = "https://broker.example.com/.well-known/oauth-client/service-123"
		tokenEndpoint = "https://issuer.example.com/oauth/token"
	)

	t.Run("does not use a published key before it becomes usable", func(t *testing.T) {
		var logOutput bytes.Buffer
		key := newAssertionSignerKey("not-yet-active-cimd-key", newCIMDTestES256PEM(t), time.Now().UTC().Add(time.Hour))
		keyService := newAssertionSignerKeyService([]*storage.SigningKey{key}, &assertionSignerEncryptor{}, slog.New(slog.NewJSONHandler(&logOutput, nil)))

		publishedSet, err := keyService.PublicJWKSet(context.Background())
		require.NoError(t, err)
		_, found := publishedSet.LookupKeyID(key.KID.String())
		require.True(t, found, "the future key is deliberately advertised before it may sign")

		assertAssertionSigningFailsClosed(t, NewAssertionSigner(keyService), clientID, tokenEndpoint, "", logOutput.String)
	})

	t.Run("does not leak encrypted material when decryption fails", func(t *testing.T) {
		const encryptedMaterial = "encrypted-cimd-private-key-material"

		var logOutput bytes.Buffer
		key := newAssertionSignerKey("undecryptable-cimd-key", []byte(encryptedMaterial), time.Now().UTC().Add(-time.Minute))
		keyService := newAssertionSignerKeyService(
			[]*storage.SigningKey{key},
			&assertionSignerEncryptor{decryptErr: errors.New("decrypt failed for " + encryptedMaterial)},
			slog.New(slog.NewJSONHandler(&logOutput, nil)),
		)

		assertAssertionSigningFailsClosed(t, NewAssertionSigner(keyService), clientID, tokenEndpoint, key.KID.String(), logOutput.String)
	})

	t.Run("does not emit an assertion when JWX signing cannot obtain entropy", func(t *testing.T) {
		const entropyFailure = "signing-entropy-secret"

		var logOutput bytes.Buffer
		key := newAssertionSignerKey("signing-failure-cimd-key", newCIMDTestES256PEM(t), time.Now().UTC().Add(-time.Minute))
		keyService := newAssertionSignerKeyService([]*storage.SigningKey{key}, &assertionSignerEncryptor{}, slog.New(slog.NewJSONHandler(&logOutput, nil)))

		originalReader := crand.Reader
		crand.Reader = assertionSignerErrorReader{err: errors.New(entropyFailure)}
		t.Cleanup(func() {
			crand.Reader = originalReader
		})

		assertAssertionSigningFailsClosed(t, NewAssertionSigner(keyService), clientID, tokenEndpoint, key.KID.String(), logOutput.String)
	})
}

func assertValidCIMDClientAssertion(
	t *testing.T,
	assertion string,
	publicJWKSet jwk.Set,
	wantKeyID, wantClientID, wantTokenEndpoint string,
) jwt.Token {
	t.Helper()

	message, err := jws.Parse([]byte(assertion))
	require.NoError(t, err)
	require.Len(t, message.Signatures(), 1)
	protectedHeaders := message.Signatures()[0].ProtectedHeaders()
	algorithm, ok := protectedHeaders.Algorithm()
	require.True(t, ok, "client assertion must carry protected alg")
	assert.Equal(t, jwa.ES256(), algorithm)
	keyID, ok := protectedHeaders.KeyID()
	require.True(t, ok, "client assertion must carry protected kid")
	assert.Equal(t, wantKeyID, keyID)
	_, ok = publicJWKSet.LookupKeyID(keyID)
	require.True(t, ok, "client assertion kid must belong to the advertised CIMD public JWK Set")

	claims, err := jwt.Parse(
		[]byte(assertion),
		jwt.WithKeySet(publicJWKSet),
		jwt.WithValidate(false),
	)
	require.NoError(t, err, "client assertion signature must verify through its advertised CIMD public JWK Set")

	issuer, ok := claims.Issuer()
	require.True(t, ok, "client assertion must carry iss")
	assert.Equal(t, wantClientID, issuer)
	subject, ok := claims.Subject()
	require.True(t, ok, "client assertion must carry sub")
	assert.Equal(t, wantClientID, subject)
	audience, ok := claims.Audience()
	require.True(t, ok, "client assertion must carry aud")
	assert.Equal(t, []string{wantTokenEndpoint}, audience, "client assertion must have one exact token-endpoint audience")

	issuedAt, ok := claims.IssuedAt()
	require.True(t, ok, "client assertion must carry iat")
	expiresAt, ok := claims.Expiration()
	require.True(t, ok, "client assertion must carry exp")
	lifetime := expiresAt.Sub(issuedAt)
	assert.Greater(t, lifetime, time.Duration(0))
	assert.LessOrEqual(t, lifetime, 5*time.Minute, "client assertion expiry must be no more than five minutes after iat")

	return claims
}

func assertAssertionSigningFailsClosed(
	t *testing.T,
	signer *AssertionSigner,
	clientID, tokenEndpoint, wantKeyID string,
	logOutput func() string,
) {
	t.Helper()

	assertion, err := signer.SignClientAssertion(context.Background(), id.ClientID(clientID), tokenEndpoint)
	require.ErrorIs(t, err, ports.ErrCIMDKeyUnavailable)
	assert.Empty(t, assertion, "an assertion signing failure must stop before a token request can be authenticated")
	assert.NotContains(t, err.Error(), "PRIVATE KEY")
	assert.NotContains(t, err.Error(), "encrypted-cimd-private-key-material")
	assert.NotContains(t, err.Error(), "signing-entropy-secret")

	logs := logOutput()
	records := assertionSignerAuditRecords(t, logs)
	require.Len(t, records, 1)
	assertAssertionSigningAuditRecord(t, records, wantKeyID, "rejected")
	assertCredentialFreeAssertionSigningAudit(t, records, logs)
}

func newAssertionSignerKey(keyID string, privateMaterial []byte, activatesAt time.Time) *storage.SigningKey {
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID(keyID),
		KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: append([]byte(nil), privateMaterial...),
		IsCurrent:           true,
		ActivatesAt:         activatesAt,
		CreatedAt:           time.Now().UTC(),
	}
}

func newAssertionSignerKeyService(
	keys []*storage.SigningKey,
	encryption ports.EncryptionPort,
	logger *slog.Logger,
) *KeyService {
	repository := &assertionSignerRepository{keys: keys}
	return NewKeyService(repository, repository, encryption, &cimdKeyServiceBranchKeyManager{}, logger)
}

type assertionSignerRepository struct {
	keys []*storage.SigningKey
}

func (r *assertionSignerRepository) Create(_ context.Context, key *storage.SigningKey) error {
	r.keys = append(r.keys, cloneAssertionSignerKey(key))
	return nil
}

func (r *assertionSignerRepository) CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error {
	return r.Create(ctx, key)
}

func (r *assertionSignerRepository) GetByKIDInDomain(_ context.Context, domain storage.KeyDomain, keyID id.KeyID) (*storage.SigningKey, error) {
	for _, key := range r.activeKeys(domain) {
		if key.KID == keyID {
			return cloneAssertionSignerKey(key), nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *assertionSignerRepository) GetCurrentInDomain(_ context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	key := currentUsableKey(r.activeKeys(domain), time.Now().UTC())
	if key == nil {
		return nil, ports.ErrNotFound
	}
	return cloneAssertionSignerKey(key), nil
}

func (r *assertionSignerRepository) ListActiveInDomain(_ context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	keys := r.activeKeys(domain)
	clones := make([]*storage.SigningKey, 0, len(keys))
	for _, key := range keys {
		clones = append(clones, cloneAssertionSignerKey(key))
	}
	return clones, nil
}

func (r *assertionSignerRepository) SetCurrentInDomain(_ context.Context, _ storage.KeyDomain, _ id.KeyID, _ time.Time) (*storage.SigningKey, error) {
	return nil, ports.ErrNotFound
}

func (r *assertionSignerRepository) DeleteInDomain(_ context.Context, _ storage.KeyDomain, _ id.KeyID) error {
	return ports.ErrNotFound
}

func (r *assertionSignerRepository) CountActiveInDomain(_ context.Context, domain storage.KeyDomain) (int, error) {
	return len(r.activeKeys(domain)), nil
}

func (r *assertionSignerRepository) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *assertionSignerRepository) activeKeys(domain storage.KeyDomain) []*storage.SigningKey {
	keys := make([]*storage.SigningKey, 0, len(r.keys))
	for _, key := range r.keys {
		if key != nil && key.KeyDomain == domain && key.RemovedAt == nil {
			keys = append(keys, key)
		}
	}
	return keys
}

func cloneAssertionSignerKey(key *storage.SigningKey) *storage.SigningKey {
	clone := *key
	clone.PrivateKeyEncrypted = append([]byte(nil), key.PrivateKeyEncrypted...)
	return &clone
}

type assertionSignerEncryptor struct {
	decryptErr error
}

func (e *assertionSignerEncryptor) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (e *assertionSignerEncryptor) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	if e.decryptErr != nil {
		return nil, e.decryptErr
	}
	return append([]byte(nil), ciphertext...), nil
}

type assertionSignerErrorReader struct {
	err error
}

func (r assertionSignerErrorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func assertionSignerAuditRecords(t *testing.T, logs string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == "CIMD client assertion signing" {
			records = append(records, record)
		}
	}
	return records
}

func assertAssertionSigningAuditRecord(t *testing.T, records []map[string]any, wantKeyID, wantOutcome string) {
	t.Helper()

	for _, record := range records {
		if record["key_id"] == wantKeyID && record["operation"] == "sign" && record["outcome"] == wantOutcome {
			return
		}
	}
	assert.Failf(t, "missing client assertion signing audit record", "key_id=%q operation=%q outcome=%q", wantKeyID, wantOutcome)
}

func assertCredentialFreeAssertionSigningAudit(t *testing.T, records []map[string]any, logs string) {
	t.Helper()

	for _, credential := range []string{
		"PRIVATE KEY",
		"encrypted-cimd-private-key-material",
		"signing-entropy-secret",
		"client_assertion",
		"authorization_code",
		"access_token",
		"refresh_token",
		"client_secret",
	} {
		assert.NotContains(t, logs, credential)
	}

	allowedFields := map[string]struct{}{
		"time":      {},
		"level":     {},
		"msg":       {},
		"key_id":    {},
		"operation": {},
		"outcome":   {},
	}
	for _, record := range records {
		for field := range record {
			_, allowed := allowedFields[field]
			assert.Truef(t, allowed, "audit field %q must not be recorded", field)
		}
	}
}

var _ ports.SigningKeyRepository = (*assertionSignerRepository)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*assertionSignerRepository)(nil)
var _ ports.EncryptionPort = (*assertionSignerEncryptor)(nil)
var _ io.Reader = assertionSignerErrorReader{}
