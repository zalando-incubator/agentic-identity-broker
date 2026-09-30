package cimdclient

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestKeyService_GenerateKeyUsesCIMDClientAuthenticationDomainAndES256(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	encryption := &cimdKeyServiceEncryptor{}
	branchKeys := &cimdKeyServiceBranchKeyManager{}
	service := newCIMDKeyServiceForTest(repository, encryption, branchKeys)

	key, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, storage.KeyDomainCIMDClientAuthentication, key.KeyDomain)
	assert.Equal(t, "ES256", key.Algorithm)

	require.Len(t, branchKeys.subjects, 1)
	subject := branchKeys.subjects[0]
	assert.Equal(t, domainencryption.BranchKeySubjectKindCIMDClientAuthenticationKey, subject.Kind())
	keyID, ok := subject.KeyID()
	require.True(t, ok)
	assert.Equal(t, key.KID, keyID)

	require.Len(t, encryption.encryptContexts, 1)
	assert.Equal(t, map[string]string{domainencryption.ContextKeyKID: key.KID.String()}, encryption.encryptContexts[0])

	_, err = service.GenerateKey(ctx, "RS256")
	require.ErrorIs(t, err, ErrUnsupportedCIMDAlgorithm)
	assert.Len(t, branchKeys.subjects, 1, "non-ES256 input must not provision a branch key")
	assert.Len(t, encryption.encryptContexts, 1, "non-ES256 input must not encrypt private material")
	assert.Len(t, repository.createdKeys(), 1, "non-ES256 input must not persist a key")
}

func TestKeyService_GeneratedPublicJWKDoesNotDecryptOnAnonymousReads(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	encryption := &cimdKeyServiceEncryptor{}
	service := newCIMDKeyServiceForTest(repository, encryption, &cimdKeyServiceBranchKeyManager{})

	generated, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	require.NotEmpty(t, generated.PublicJWK)

	for range 2 {
		set, err := service.PublicJWKSet(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{generated.KID.String()}, cimdJWKSetKIDs(t, set))
	}
	assert.Empty(t, encryption.decryptContexts, "anonymous public-key retrieval must not decrypt private key material")
}

func TestKeyService_PublicJWKSetUsesOnlyPublicCIMDES256Keys(t *testing.T) {
	ctx := context.Background()
	privatePEM := newCIMDTestES256PEM(t)
	now := time.Now().UTC()

	cimdES256 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("cimd-es256"),
		KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: privatePEM,
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
	cimdRS256 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("cimd-rs256"),
		KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "RS256",
		PrivateKeyEncrypted: privatePEM,
		CreatedAt:           now,
	}
	tokenES256 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("token-es256"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: privatePEM,
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}

	repository := newCIMDKeyServiceRepository()
	repository.activeByDomain[storage.KeyDomainCIMDClientAuthentication] = []*storage.SigningKey{cimdES256, cimdRS256}
	repository.activeByDomain[storage.KeyDomainTokenSigning] = []*storage.SigningKey{tokenES256}
	encryption := &cimdKeyServiceEncryptor{}
	service := newCIMDKeyServiceForTest(repository, encryption, &cimdKeyServiceBranchKeyManager{})

	set, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	require.NotNil(t, set)
	require.Equal(t, 1, set.Len())
	for _, domain := range repository.listedDomains {
		assert.Equal(t, storage.KeyDomainCIMDClientAuthentication, domain)
	}

	key, ok := set.Key(0)
	require.True(t, ok)
	kid, ok := key.KeyID()
	require.True(t, ok)
	assert.Equal(t, cimdES256.KID.String(), kid)
	algorithm, ok := key.Algorithm()
	require.True(t, ok)
	assert.Equal(t, jwa.ES256(), algorithm)
	usage, ok := key.KeyUsage()
	require.True(t, ok)
	assert.Equal(t, "sig", usage)
	_, err = jwk.Get[any](key, "d")
	assert.Error(t, err, "public JWKs must not contain private key material")

	require.Len(t, encryption.decryptContexts, 1)
	assert.Equal(t, map[string]string{domainencryption.ContextKeyKID: cimdES256.KID.String()}, encryption.decryptContexts[0])

	_, err = service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.Len(t, encryption.decryptContexts, 1, "a legacy key must decrypt only until its public JWK is persisted")
}

func TestKeyService_RequireUsablePublishedKeyAcceptsEffectiveCIMDPublicKey(t *testing.T) {
	privatePEM := newCIMDTestES256PEM(t)
	now := time.Now().UTC()
	repository := newCIMDKeyServiceRepository()
	repository.activeByDomain[storage.KeyDomainCIMDClientAuthentication] = []*storage.SigningKey{
		{
			ID:                  id.NewSigningKeyID(),
			KID:                 id.NewKeyID("ready-cimd-es256"),
			KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: privatePEM,
			IsCurrent:           true,
			ActivatesAt:         now,
			CreatedAt:           now,
		},
	}
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

	require.NoError(t, service.RequireUsablePublishedKey(context.Background()))
	assert.NotEmpty(t, repository.listedDomains)
	for _, domain := range repository.listedDomains {
		assert.Equal(t, storage.KeyDomainCIMDClientAuthentication, domain)
	}
}

func TestKeyService_ReadinessRejectsPendingOnlyButPreservesOverlap(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

	pending, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	_, err = repository.SetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, pending.KID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	set, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{pending.KID.String()}, cimdJWKSetKIDs(t, set))
	require.ErrorIs(t, service.RequireUsablePublishedKey(ctx), ports.ErrCIMDPublicKeyUnavailable)

	previous, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	_, err = repository.SetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, previous.KID, time.Now().UTC())
	require.NoError(t, err)
	require.NoError(t, service.RequireUsablePublishedKey(ctx))
	set, err = service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{previous.KID.String(), pending.KID.String()}, cimdJWKSetKIDs(t, set))
}

func TestKeyService_UnavailabilityUsesTypedSentinels(t *testing.T) {
	service := newCIMDKeyServiceForTest(
		newCIMDKeyServiceRepository(),
		&cimdKeyServiceEncryptor{},
		&cimdKeyServiceBranchKeyManager{},
	)

	t.Run("public key readiness", func(t *testing.T) {
		err := service.RequireUsablePublishedKey(context.Background())
		require.ErrorIs(t, err, ports.ErrCIMDPublicKeyUnavailable)
	})

	t.Run("assertion signing without a usable key", func(t *testing.T) {
		signer := NewAssertionSigner(service)
		_, err := signer.SignClientAssertion(context.Background(), id.ClientID("https://broker.example/client"), "https://issuer.example/token")
		require.ErrorIs(t, err, ports.ErrCIMDKeyUnavailable)
	})
}

func TestKeyService_InitialCIMDKeysActivateImmediately(t *testing.T) {
	ctx := context.Background()

	t.Run("bootstrap key", func(t *testing.T) {
		repository := newCIMDKeyServiceRepository()
		service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

		key, created, err := service.EnsureInitialKey(ctx)
		require.NoError(t, err)
		require.True(t, created)
		require.NotNil(t, key)
		assert.Equal(t, "ES256", key.Algorithm)
		assert.True(t, key.IsCurrent)

		current, err := repository.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err, "the bootstrap key must be usable without waiting for publication grace")
		assert.Equal(t, key.KID, current.KID)
	})

	t.Run("first operator-generated key", func(t *testing.T) {
		repository := newCIMDKeyServiceRepository()
		service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

		key, err := service.GenerateKey(ctx, "")
		require.NoError(t, err)
		require.NotNil(t, key)
		assert.Equal(t, "ES256", key.Algorithm, "an omitted algorithm must default to the only permitted CIMD algorithm")
		assert.True(t, key.IsCurrent)

		current, err := repository.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err, "the first operator-generated key must be usable without waiting for publication grace")
		assert.Equal(t, key.KID, current.KID)
	})
}

func TestKeyService_LaterGeneratedKeyOverlapsDuringPublicationGrace(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

	previous, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)

	generatedAt := time.Now().UTC()
	pending, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	require.NotNil(t, pending)
	assert.True(t, pending.IsCurrent)
	assert.WithinDuration(t, generatedAt.Add(cimdKeyGracePeriod), pending.ActivatesAt, time.Second)

	storedPrevious, err := repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, previous.KID)
	require.NoError(t, err)
	assert.False(t, storedPrevious.IsCurrent, "the previous key must be demoted when the pending key becomes current")

	effectiveCurrent, err := repository.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	require.NoError(t, err)
	assert.Equal(t, previous.KID, effectiveCurrent.KID, "the previous usable key must continue signing during grace")

	published, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{previous.KID.String(), pending.KID.String()}, cimdJWKSetKIDs(t, published),
		"both the current pending key and the effective fallback must be published during grace")
}

func TestKeyService_PromoteKeyActivatesImmediately(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

	previous, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	pending, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	require.True(t, pending.ActivatesAt.After(time.Now().UTC()), "the generated replacement must start in grace before promotion")

	promotedAt := time.Now().UTC()
	promoted, err := service.PromoteKey(ctx, pending.KID)
	require.NoError(t, err)
	require.NotNil(t, promoted)
	assert.Equal(t, pending.KID, promoted.KID)
	assert.True(t, promoted.IsCurrent)
	assert.WithinDuration(t, promotedAt, promoted.ActivatesAt, time.Second,
		"operator promotion must bypass the activation grace period")

	effectiveCurrent, err := repository.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	require.NoError(t, err)
	assert.Equal(t, pending.KID, effectiveCurrent.KID)

	storedPrevious, err := repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, previous.KID)
	require.NoError(t, err)
	assert.False(t, storedPrevious.IsCurrent)

	published, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{previous.KID.String(), pending.KID.String()}, cimdJWKSetKIDs(t, published),
		"promotion retains the prior key for verification until explicit retirement")
}

func TestKeyService_DeleteKeyPreservesCIMDLifecycleGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("last active key", func(t *testing.T) {
		repository := newCIMDKeyServiceRepository()
		service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})
		key, err := service.GenerateKey(ctx, "ES256")
		require.NoError(t, err)

		err = service.DeleteKey(ctx, key.KID)
		require.ErrorIs(t, err, ports.ErrLastActiveKey)

		_, err = repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, key.KID)
		require.NoError(t, err, "a rejected last-key retirement must leave the key active")
	})

	t.Run("current key", func(t *testing.T) {
		repository := newCIMDKeyServiceRepository()
		service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})
		_, err := service.GenerateKey(ctx, "ES256")
		require.NoError(t, err)
		current, err := service.GenerateKey(ctx, "ES256")
		require.NoError(t, err)

		err = service.DeleteKey(ctx, current.KID)
		require.ErrorIs(t, err, ports.ErrCurrentKey)

		stored, err := repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, current.KID)
		require.NoError(t, err)
		assert.Nil(t, stored.RemovedAt)
	})

	t.Run("effective current fallback during grace", func(t *testing.T) {
		repository := newCIMDKeyServiceRepository()
		service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})
		fallback, err := service.GenerateKey(ctx, "ES256")
		require.NoError(t, err)
		_, err = service.GenerateKey(ctx, "ES256")
		require.NoError(t, err)

		effectiveCurrent, err := repository.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, fallback.KID, effectiveCurrent.KID)

		err = service.DeleteKey(ctx, fallback.KID)
		require.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)

		stored, err := repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, fallback.KID)
		require.NoError(t, err)
		assert.Nil(t, stored.RemovedAt)
	})
}

func TestKeyService_RetiredCIMDKeyIsNoLongerPublished(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})

	current, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	retiring, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	_, err = service.PromoteKey(ctx, current.KID)
	require.NoError(t, err)

	publishedBeforeRetirement, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{current.KID.String(), retiring.KID.String()}, cimdJWKSetKIDs(t, publishedBeforeRetirement))

	require.NoError(t, service.DeleteKey(ctx, retiring.KID))

	publishedAfterRetirement, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{current.KID.String()}, cimdJWKSetKIDs(t, publishedAfterRetirement),
		"retired keys must be removed from the public verification set")

	_, err = repository.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, retiring.KID)
	require.ErrorIs(t, err, ports.ErrNotFound)
}

func TestKeyService_LifecycleAuditRecordsAreCredentialFree(t *testing.T) {
	ctx := context.Background()
	var logOutput bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logOutput, nil))
	repository := newCIMDKeyServiceRepository()
	service := NewKeyService(repository, repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, logger)

	first, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	second, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	_, err = service.PromoteKey(ctx, first.KID)
	require.NoError(t, err)
	require.NoError(t, service.DeleteKey(ctx, second.KID))

	_, err = service.GenerateKey(ctx, "RS256")
	require.ErrorIs(t, err, ErrUnsupportedCIMDAlgorithm)
	_, err = service.PromoteKey(ctx, id.NewKeyID("missing-cimd-key"))
	require.ErrorIs(t, err, ports.ErrNotFound)
	err = service.DeleteKey(ctx, id.NewKeyID("missing-cimd-key"))
	require.ErrorIs(t, err, ports.ErrNotFound)

	logs := logOutput.String()
	assert.NotContains(t, logs, string(first.PrivateKeyEncrypted))
	for _, credential := range []string{"cimd-test-branch-key", "sentinel-client-assertion", "sentinel-access-token", "sentinel-refresh-token"} {
		assert.NotContains(t, logs, credential)
	}

	records := cimdLifecycleAuditRecords(t, logs)
	require.Len(t, records, 7)
	assertCIMDKeyLifecycleAuditRecord(t, records, first.KID.String(), "generate", "success")
	assertCIMDKeyLifecycleAuditRecord(t, records, second.KID.String(), "generate", "success")
	assertCIMDKeyLifecycleAuditRecord(t, records, first.KID.String(), "promote", "success")
	assertCIMDKeyLifecycleAuditRecord(t, records, second.KID.String(), "remove", "success")
	assertCIMDKeyLifecycleAuditRecord(t, records, "", "generate", "rejected")
	assertCIMDKeyLifecycleAuditRecord(t, records, "missing-cimd-key", "promote", "rejected")
	assertCIMDKeyLifecycleAuditRecord(t, records, "missing-cimd-key", "remove", "rejected")

	allowedFields := map[string]struct{}{
		"time":      {},
		"level":     {},
		"msg":       {},
		"key_id":    {},
		"operation": {},
		"outcome":   {},
	}
	for _, record := range records {
		assert.Equal(t, "CIMD client-authentication key lifecycle", record["msg"])
		for field := range record {
			_, allowed := allowedFields[field]
			assert.Truef(t, allowed, "audit field %q must not be recorded", field)
		}
		for _, credentialField := range []string{
			"private_key", "private_key_encrypted", "ciphertext", "branch_key", "client_secret",
			"client_assertion", "authorization_code", "access_token", "refresh_token",
		} {
			assert.NotContains(t, record, credentialField)
		}
	}
}

func TestKeyService_InternalLifecycleFailuresAreAudited(t *testing.T) {
	t.Run("generation count failure", func(t *testing.T) {
		var logs bytes.Buffer
		repository := &failingCIMDKeyServiceRepository{
			cimdKeyServiceRepository: newCIMDKeyServiceRepository(),
			countErr:                 errors.New("count failure"),
		}
		service := NewKeyService(repository, repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, slog.New(slog.NewJSONHandler(&logs, nil)))

		_, err := service.GenerateKey(context.Background(), "ES256")

		require.Error(t, err)
		assertCIMDKeyLifecycleAuditRecord(t, cimdLifecycleAuditRecords(t, logs.String()), "", "generate", "rejected")
	})

	t.Run("bootstrap lock failure", func(t *testing.T) {
		var logs bytes.Buffer
		repository := &failingCIMDKeyServiceRepository{
			cimdKeyServiceRepository: newCIMDKeyServiceRepository(),
			bootstrapErr:             errors.New("bootstrap lock failure"),
		}
		service := NewKeyService(repository, repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, slog.New(slog.NewJSONHandler(&logs, nil)))

		_, _, err := service.EnsureInitialKey(context.Background())

		require.Error(t, err)
		assertCIMDKeyLifecycleAuditRecord(t, cimdLifecycleAuditRecords(t, logs.String()), "", "bootstrap", "rejected")
	})

	t.Run("default logger", func(t *testing.T) {
		service := NewKeyService(newCIMDKeyServiceRepository(), newCIMDKeyServiceRepository(), &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, nil)
		assert.NotNil(t, service.logger)
	})
}

func cimdJWKSetKIDs(t *testing.T, set jwk.Set) []string {
	t.Helper()

	kids := make([]string, 0, set.Len())
	for index := range set.Len() {
		key, ok := set.Key(index)
		require.True(t, ok)
		kid, ok := key.KeyID()
		require.True(t, ok)
		kids = append(kids, kid)
	}
	return kids
}

func cimdLifecycleAuditRecords(t *testing.T, logs string) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == "CIMD client-authentication key lifecycle" {
			records = append(records, record)
		}
	}
	return records
}

func assertCIMDKeyLifecycleAuditRecord(t *testing.T, records []map[string]any, keyID, operation, outcome string) {
	t.Helper()

	for _, record := range records {
		if record["key_id"] == keyID && record["operation"] == operation && record["outcome"] == outcome {
			return
		}
	}
	assert.Failf(t, "missing CIMD key lifecycle audit record", "key_id=%q operation=%q outcome=%q", keyID, operation, outcome)
}

func TestKeyService_PublicJWKSetRejectsNilRowAlongsideValidKey(t *testing.T) {
	base := newCIMDKeyServiceRepository()
	repository := &nilRowCIMDKeyRepository{
		SigningKeyRepository: base,
		keys:                 []*storage.SigningKey{newCachedCIMDKey(t, "valid"), nil},
	}
	service := NewKeyService(repository, base, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, slog.Default())
	var set jwk.Set
	var err error
	require.NotPanics(t, func() { set, err = service.PublicJWKSet(context.Background()) })
	require.ErrorIs(t, err, ports.ErrCIMDPublicKeyUnavailable)
	assert.Nil(t, set, "a malformed row must not publish a partial set")
}

type nilRowCIMDKeyRepository struct {
	ports.SigningKeyRepository
	keys []*storage.SigningKey
}

func (r *nilRowCIMDKeyRepository) ListActiveInDomain(_ context.Context, _ storage.KeyDomain) ([]*storage.SigningKey, error) {
	return r.keys, nil
}

func TestKeyService_PublicJWKSetCachesByCommittedRevision(t *testing.T) {
	ctx := context.Background()
	repository := newCIMDKeyServiceRepository()
	service := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})
	first, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	for range 3 {
		set, err := service.PublicJWKSet(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{first.KID.String()}, cimdJWKSetKIDs(t, set))
	}
	assert.Len(t, repository.listedDomains, 1, "unchanged revision must not reparse the public key")

	remote := newCIMDKeyServiceForTest(repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{})
	second, err := remote.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	set, err := service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{first.KID.String(), second.KID.String()}, cimdJWKSetKIDs(t, set))
	_, err = remote.PromoteKey(ctx, second.KID)
	require.NoError(t, err)
	require.NoError(t, remote.DeleteKey(ctx, first.KID))
	set, err = service.PublicJWKSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{second.KID.String()}, cimdJWKSetKIDs(t, set), "a remote removal must be visible on the next read")
}

func TestKeyService_PublicJWKSetFailsClosedAfterWarmCache(t *testing.T) {
	ctx := context.Background()
	repository := &failingCIMDKeyServiceRepository{cimdKeyServiceRepository: newCIMDKeyServiceRepository()}
	service := NewKeyService(repository, repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, slog.Default())
	_, err := service.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	_, err = service.PublicJWKSet(ctx)
	require.NoError(t, err)

	repository.versionErr = errors.New("revision unavailable")
	set, err := service.PublicJWKSet(ctx)
	require.Error(t, err)
	assert.Nil(t, set)
	repository.versionErr = nil
	repository.version++
	repository.listErr = errors.New("rebuild unavailable")
	set, err = service.PublicJWKSet(ctx)
	require.Error(t, err)
	assert.Nil(t, set)
}

func TestKeyService_PublicJWKSetFencesConcurrentRebuild(t *testing.T) {
	ctx := context.Background()
	base := newCIMDKeyServiceRepository()
	repository := &concurrentCIMDKeyRepository{cimdKeyServiceRepository: base, entered: make(chan struct{}), release: make(chan struct{})}
	service := NewKeyService(repository, repository, &cimdKeyServiceEncryptor{}, &cimdKeyServiceBranchKeyManager{}, slog.Default())
	first := newCachedCIMDKey(t, "first")
	require.NoError(t, repository.Create(ctx, first))

	type readResult struct {
		set jwk.Set
		err error
	}
	results := make(chan readResult, 12)
	for range 12 {
		go func() {
			set, err := service.PublicJWKSet(ctx)
			results <- readResult{set, err}
		}()
	}
	select {
	case <-repository.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("cold build never reached repository")
	}
	second := newCachedCIMDKey(t, "second")
	require.NoError(t, repository.Create(ctx, second))
	close(repository.release)
	for range 12 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			assert.ElementsMatch(t, []string{"first", "second"}, cimdJWKSetKIDs(t, result.set))
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent reader did not finish")
		}
	}
}

func newCachedCIMDKey(t *testing.T, kid string) *storage.SigningKey {
	t.Helper()
	privatePEM := newCIMDTestES256PEM(t)
	public, err := cimdPublicJWKFromPEM(privatePEM, id.NewKeyID(kid))
	require.NoError(t, err)
	publicJSON, err := json.Marshal(public)
	require.NoError(t, err)
	now := time.Now().UTC()
	return &storage.SigningKey{ID: id.NewSigningKeyID(), KID: id.NewKeyID(kid), KeyDomain: storage.KeyDomainCIMDClientAuthentication, Algorithm: "ES256", PrivateKeyEncrypted: privatePEM, PublicJWK: publicJSON, ActivatesAt: now, CreatedAt: now}
}

type concurrentCIMDKeyRepository struct {
	*cimdKeyServiceRepository
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
	lists   int
}

func (r *concurrentCIMDKeyRepository) KeySetVersion(ctx context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cimdKeyServiceRepository.KeySetVersion(ctx)
}

func (r *concurrentCIMDKeyRepository) Create(ctx context.Context, key *storage.SigningKey) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cimdKeyServiceRepository.Create(ctx, key)
}

func (r *concurrentCIMDKeyRepository) ListActiveInDomain(ctx context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	r.mu.Lock()
	keys, err := r.cimdKeyServiceRepository.ListActiveInDomain(ctx, domain)
	r.lists++
	first := r.lists == 1
	r.mu.Unlock()
	if first {
		close(r.entered)
		<-r.release
	}
	return keys, err
}

func newCIMDKeyServiceForTest(
	repository *cimdKeyServiceRepository,
	encryption *cimdKeyServiceEncryptor,
	branchKeys *cimdKeyServiceBranchKeyManager,
) *KeyService {
	return NewKeyService(repository, repository, encryption, branchKeys, slog.Default())
}

type cimdKeyServiceRepository struct {
	activeByDomain map[storage.KeyDomain][]*storage.SigningKey
	listedDomains  []storage.KeyDomain
	created        []*storage.SigningKey
	version        int64
}

func newCIMDKeyServiceRepository() *cimdKeyServiceRepository {
	return &cimdKeyServiceRepository{activeByDomain: make(map[storage.KeyDomain][]*storage.SigningKey)}
}

func (r *cimdKeyServiceRepository) Create(_ context.Context, key *storage.SigningKey) error {
	r.created = append(r.created, cloneCIMDSigningKey(key))
	r.activeByDomain[key.KeyDomain] = append(r.activeByDomain[key.KeyDomain], cloneCIMDSigningKey(key))
	r.version++
	return nil
}

func (r *cimdKeyServiceRepository) CreateAndSetCurrent(_ context.Context, key *storage.SigningKey) error {
	for _, existing := range r.activeByDomain[key.KeyDomain] {
		existing.IsCurrent = false
	}
	clone := cloneCIMDSigningKey(key)
	clone.IsCurrent = true
	r.created = append(r.created, cloneCIMDSigningKey(clone))
	r.activeByDomain[key.KeyDomain] = append(r.activeByDomain[key.KeyDomain], clone)
	r.version++
	return nil
}

func (r *cimdKeyServiceRepository) GetByKIDInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	for _, key := range r.activeByDomain[domain] {
		if key.KID == kid && key.RemovedAt == nil {
			return cloneCIMDSigningKey(key), nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *cimdKeyServiceRepository) GetCurrentInDomain(_ context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	now := time.Now().UTC()
	var current *storage.SigningKey
	for _, key := range r.activeByDomain[domain] {
		if key.RemovedAt != nil || key.ActivatesAt.After(now) {
			continue
		}
		if current == nil || (!current.IsCurrent && key.IsCurrent) || (current.IsCurrent == key.IsCurrent && key.ActivatesAt.After(current.ActivatesAt)) {
			current = key
		}
	}
	if current == nil {
		return nil, ports.ErrNotFound
	}
	return cloneCIMDSigningKey(current), nil
}

func (r *cimdKeyServiceRepository) ListActiveInDomain(_ context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	r.listedDomains = append(r.listedDomains, domain)
	keys := make([]*storage.SigningKey, 0, len(r.activeByDomain[domain]))
	for _, key := range r.activeByDomain[domain] {
		if key.RemovedAt == nil {
			keys = append(keys, cloneCIMDSigningKey(key))
		}
	}
	return keys, nil
}

func (r *cimdKeyServiceRepository) KeySetVersion(context.Context) (int64, error) {
	return r.version, nil
}

func (r *cimdKeyServiceRepository) SetPublicJWK(_ context.Context, kid id.KeyID, publicJWK []byte) (bool, error) {
	for _, keys := range r.activeByDomain {
		for _, key := range keys {
			if key.KID == kid && key.RemovedAt == nil {
				if len(key.PublicJWK) != 0 {
					return false, nil
				}
				key.PublicJWK = append([]byte(nil), publicJWK...)
				r.version++
				return true, nil
			}
		}
	}
	return false, ports.ErrNotFound
}

func (r *cimdKeyServiceRepository) SetCurrentInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	for _, key := range r.activeByDomain[domain] {
		if key.KID == kid && key.RemovedAt == nil {
			for _, other := range r.activeByDomain[domain] {
				other.IsCurrent = false
			}
			key.IsCurrent = true
			key.ActivatesAt = activatesAt
			r.version++
			return cloneCIMDSigningKey(key), nil
		}
	}
	return nil, ports.ErrNotFound
}

func (r *cimdKeyServiceRepository) DeleteInDomain(_ context.Context, domain storage.KeyDomain, kid id.KeyID) error {
	for _, key := range r.activeByDomain[domain] {
		if key.KID == kid && key.RemovedAt == nil {
			now := time.Now().UTC()
			key.RemovedAt = &now
			r.version++
			return nil
		}
	}
	return ports.ErrNotFound
}

func (r *cimdKeyServiceRepository) CountActiveInDomain(_ context.Context, domain storage.KeyDomain) (int, error) {
	count := 0
	for _, key := range r.activeByDomain[domain] {
		if key.RemovedAt == nil {
			count++
		}
	}
	return count, nil
}

func (r *cimdKeyServiceRepository) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *cimdKeyServiceRepository) createdKeys() []*storage.SigningKey {
	return r.created
}

func cloneCIMDSigningKey(key *storage.SigningKey) *storage.SigningKey {
	clone := *key
	clone.PrivateKeyEncrypted = append([]byte(nil), key.PrivateKeyEncrypted...)
	clone.PublicJWK = append([]byte(nil), key.PublicJWK...)
	return &clone
}

type cimdKeyServiceEncryptor struct {
	encryptContexts []map[string]string
	decryptContexts []map[string]string
}

func (e *cimdKeyServiceEncryptor) Encrypt(_ context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	e.encryptContexts = append(e.encryptContexts, cloneCIMDEncryptionContext(encryptionContext))
	return append([]byte(nil), plaintext...), nil
}

func (e *cimdKeyServiceEncryptor) Decrypt(_ context.Context, ciphertext []byte, encryptionContext map[string]string) ([]byte, error) {
	e.decryptContexts = append(e.decryptContexts, cloneCIMDEncryptionContext(encryptionContext))
	return append([]byte(nil), ciphertext...), nil
}

type cimdKeyServiceBranchKeyManager struct {
	subjects []domainencryption.BranchKeySubject
}

func (m *cimdKeyServiceBranchKeyManager) Create(_ context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	m.subjects = append(m.subjects, subject)
	return "cimd-test-branch-key", nil
}

func cloneCIMDEncryptionContext(encryptionContext map[string]string) map[string]string {
	clone := make(map[string]string, len(encryptionContext))
	for key, value := range encryptionContext {
		clone[key] = value
	}
	return clone
}

func newCIMDTestES256PEM(t *testing.T) []byte {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

type failingCIMDKeyServiceRepository struct {
	*cimdKeyServiceRepository
	countErr     error
	listErr      error
	versionErr   error
	bootstrapErr error
}

func (r *failingCIMDKeyServiceRepository) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.cimdKeyServiceRepository.CountActiveInDomain(ctx, domain)
}

func (r *failingCIMDKeyServiceRepository) KeySetVersion(ctx context.Context) (int64, error) {
	if r.versionErr != nil {
		return 0, r.versionErr
	}
	return r.cimdKeyServiceRepository.KeySetVersion(ctx)
}

func (r *failingCIMDKeyServiceRepository) ListActiveInDomain(ctx context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.cimdKeyServiceRepository.ListActiveInDomain(ctx, domain)
}

func (r *failingCIMDKeyServiceRepository) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	if r.bootstrapErr != nil {
		return r.bootstrapErr
	}
	return r.cimdKeyServiceRepository.WithBootstrapLock(ctx, fn)
}

var _ ports.SigningKeyRepository = (*failingCIMDKeyServiceRepository)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*failingCIMDKeyServiceRepository)(nil)

var _ ports.SigningKeyRepository = (*cimdKeyServiceRepository)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*cimdKeyServiceRepository)(nil)
var _ ports.EncryptionPort = (*cimdKeyServiceEncryptor)(nil)
var _ ports.BranchKeyManager = (*cimdKeyServiceBranchKeyManager)(nil)
