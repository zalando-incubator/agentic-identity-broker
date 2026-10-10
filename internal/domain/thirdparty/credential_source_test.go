package thirdparty_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the existing registration path and real storage mutations.
// File bindings belong to the session domain; registration has no file-reader port.
type credentialSourceRepository struct {
	*memory.InMemoryThirdpartyOAuth2ProviderRepository
	creates  int
	updates  int
	afterGet func()
}

func (r *credentialSourceRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	r.creates++
	return r.InMemoryThirdpartyOAuth2ProviderRepository.Create(ctx, entity)
}

func (r *credentialSourceRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	entity, err := r.InMemoryThirdpartyOAuth2ProviderRepository.Get(ctx, serviceID)
	if err == nil && r.afterGet != nil {
		hook := r.afterGet
		r.afterGet = nil
		hook()
	}
	return entity, err
}

func (r *credentialSourceRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	r.updates++
	return r.InMemoryThirdpartyOAuth2ProviderRepository.Update(ctx, entity, expectedVersion)
}

type credentialSourceEncryption struct {
	ports.EncryptionPort
	encryptCalls int
	decryptCalls int
	encryptErr   error
}

func (e *credentialSourceEncryption) Encrypt(ctx context.Context, plaintext []byte, aad map[string]string) ([]byte, error) {
	e.encryptCalls++
	if e.encryptErr != nil {
		return nil, e.encryptErr
	}
	return e.EncryptionPort.Encrypt(ctx, plaintext, aad)
}

func (e *credentialSourceEncryption) Decrypt(ctx context.Context, ciphertext []byte, aad map[string]string) ([]byte, error) {
	e.decryptCalls++
	return e.EncryptionPort.Decrypt(ctx, ciphertext, aad)
}

type credentialSourceBranchKeys struct {
	calls int
}

func (b *credentialSourceBranchKeys) Create(context.Context, domainencryption.BranchKeySubject) (string, error) {
	b.calls++
	return "", nil
}

type credentialSourceFixture struct {
	repo    *credentialSourceRepository
	crypto  *credentialSourceEncryption
	keys    *credentialSourceBranchKeys
	service *thirdparty.ThirdpartyOAuth2ProviderService
}

func newCredentialSourceFixture(t *testing.T) *credentialSourceFixture {
	t.Helper()
	repo := &credentialSourceRepository{InMemoryThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository()}
	crypto := &credentialSourceEncryption{EncryptionPort: testutil.NewTestEncryptionAdapter(t)}
	keys := &credentialSourceBranchKeys{}
	service := thirdparty.NewThirdpartyOAuth2ProviderService(repo, crypto, keys, nil, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return &credentialSourceFixture{repo: repo, crypto: crypto, keys: keys, service: service}
}

func credentialSourceEntity(serviceID id.ServiceID, source model.CredentialSource) *model.ThirdpartyOAuth2ProviderEntity {
	canonicalID := "credential-service"
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                       serviceID,
		CanonicalID:              &canonicalID,
		DisplayName:              "Credential service",
		CredentialSource:         source,
		CredentialSourceProvided: source != "",
		IssuerURI:                "https://provider.example",
		Discovery:                model.DiscoveryConfig{EnableDiscovery: true},
		Scopes:                   []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}},
		ProtectedResources:       []string{"https://api.example/original"},
		Secret:                   model.NewAbsentSecret(),
	}
	if source != model.CredentialSourceFilesystem {
		entity.ClientID = id.ClientID("inline-client")
		entity.Secret = model.NewPlaintextSecret("inline-secret")
		entity.ClientIDProvided = true
		entity.ClientSecretProvided = true
	}
	return entity
}

func persistedCredentialSource(t *testing.T, fixture *credentialSourceFixture, serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	t.Helper()
	entity, err := fixture.repo.InMemoryThirdpartyOAuth2ProviderRepository.Get(context.Background(), serviceID)
	require.NoError(t, err)
	return entity
}

func assertCredentialSourceUnchanged(t *testing.T, fixture *credentialSourceFixture, before *model.ThirdpartyOAuth2ProviderEntity) {
	t.Helper()
	after := persistedCredentialSource(t, fixture, before.ID)
	assert.Equal(t, before, after, "a rejected update must preserve the entire persisted record, including ciphertext and version")
	if before.CanonicalID != nil {
		owner, err := fixture.repo.GetByCanonicalID(context.Background(), *before.CanonicalID)
		require.NoError(t, err)
		assert.Equal(t, before.ID, owner.ID)
	}
	for _, resource := range before.ProtectedResources {
		owner, err := fixture.repo.FindByProtectedResource(context.Background(), resource)
		require.NoError(t, err)
		assert.Equal(t, before.ID, owner.ID)
	}
}

func assertCredentialSourceConflict(t *testing.T, err error) {
	t.Helper()
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

func TestCredentialSourceCreateFilesystemStoresAbsentCredentials(t *testing.T) {
	for _, flavor := range []model.OAuth2Flavor{model.OAuth2FlavorStandard, model.OAuth2FlavorGitHub} {
		t.Run(string(flavor), func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			entity.Flavor = flavor

			require.NoError(t, fixture.service.Create(context.Background(), entity))

			stored := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, model.CredentialSourceFilesystem, stored.CredentialSource)
			assert.True(t, stored.ClientID.IsZero(), "filesystem registration must not synthesize a client ID")
			assert.True(t, stored.Secret.IsAbsent(), "filesystem registration must not encrypt a placeholder")
			_, err := stored.Secret.GetCiphertext()
			assert.Error(t, err)
			assert.False(t, stored.CredentialSourceTransitioned)
			assert.Zero(t, fixture.crypto.encryptCalls)

			read, err := fixture.service.Get(context.Background(), entity.ID)
			require.NoError(t, err)
			assert.True(t, read.Secret.IsAbsent())
			assert.True(t, read.ClientID.IsZero())
			listed, err := fixture.service.List(context.Background())
			require.NoError(t, err)
			require.Len(t, listed, 1)
			assert.True(t, listed[0].Secret.IsAbsent())
			assert.Equal(t, model.CredentialSourceFilesystem, listed[0].CredentialSource)
			assert.Zero(t, fixture.crypto.decryptCalls)
		})
	}
}

func TestCredentialSourceCreateStoredDefaultsAndEncryptsInlineSecret(t *testing.T) {
	for _, source := range []model.CredentialSource{"", model.CredentialSourceStored} {
		t.Run("source="+string(source), func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), source)
			require.NoError(t, fixture.service.Create(context.Background(), entity))

			stored := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, model.CredentialSourceStored, stored.CredentialSource)
			assert.Equal(t, id.ClientID("inline-client"), stored.ClientID)
			assert.True(t, stored.Secret.IsEncrypted())
			assert.False(t, stored.CredentialSourceTransitioned)
			ciphertext, err := stored.Secret.GetCiphertext()
			require.NoError(t, err)
			assert.NotEqual(t, []byte("inline-secret"), ciphertext)
			plaintext, err := fixture.crypto.EncryptionPort.Decrypt(context.Background(), ciphertext, map[string]string{"service_id": entity.ID.String()})
			require.NoError(t, err)
			assert.Equal(t, "inline-secret", string(plaintext))
		})
	}
}

func TestCredentialSourceStoredRequiresInlineCredentialsBeforeMutation(t *testing.T) {
	cases := []struct {
		name       string
		invalidate func(*model.ThirdpartyOAuth2ProviderEntity)
	}{
		{"missing client ID", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientID = ""; e.ClientIDProvided = false }},
		{"absent secret", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.Secret = model.NewAbsentSecret()
			e.ClientSecretProvided = false
		}},
		{"empty secret", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.Secret = model.NewPlaintextSecret("") }},
		{"encrypted request secret", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.Secret = model.NewEncryptedSecret([]byte("not-an-inline-secret"))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceStored)
			test.invalidate(entity)
			require.Error(t, fixture.service.Create(context.Background(), entity))
			assert.Zero(t, fixture.repo.creates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.keys.calls)

			existing := credentialSourceEntity(entity.ID, model.CredentialSourceStored)
			require.NoError(t, fixture.service.Create(context.Background(), existing))
			before := persistedCredentialSource(t, fixture, existing.ID)
			update := credentialSourceEntity(existing.ID, model.CredentialSourceStored)
			update.DisplayName = "Rejected replacement"
			update.ProtectedResources = []string{"https://api.example/rejected"}
			test.invalidate(update)
			encryptCalls, keyCalls := fixture.crypto.encryptCalls, fixture.keys.calls
			require.Error(t, fixture.service.Update(context.Background(), update, &before.Version))
			assert.Zero(t, fixture.repo.updates)
			assert.Equal(t, encryptCalls, fixture.crypto.encryptCalls)
			assert.Equal(t, keyCalls, fixture.keys.calls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceInvalidSelectorsAndMixedFilesystemInputDoNotMutate(t *testing.T) {
	cases := []struct {
		name       string
		invalidate func(*model.ThirdpartyOAuth2ProviderEntity)
	}{
		{"unknown selector", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.CredentialSource = "automatic"
			e.ClientID = "valid-stored-client"
			e.Secret = model.NewPlaintextSecret("valid-stored-secret")
			e.ClientIDProvided = true
			e.ClientSecretProvided = true
		}},
		{"null selector", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.CredentialSource = ""
			e.CredentialSourceProvided = true
			e.ClientID = "valid-stored-client"
			e.Secret = model.NewPlaintextSecret("valid-stored-secret")
			e.ClientIDProvided = true
			e.ClientSecretProvided = true
		}},
		{"missing canonical ID", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.CanonicalID = nil; e.ClearCanonicalID = true }},
		{"inline client ID", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientID = "mixed-client"; e.ClientIDProvided = true }},
		{"inline secret", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.Secret = model.NewPlaintextSecret("mixed-secret")
			e.ClientSecretProvided = true
		}},
		{"both inline credentials", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.ClientID = "mixed-client"
			e.Secret = model.NewPlaintextSecret("mixed-secret")
			e.ClientIDProvided = true
			e.ClientSecretProvided = true
		}},
		{"empty client ID", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientIDProvided = true }},
		{"empty secret", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.Secret = model.NewPlaintextSecret("")
			e.ClientSecretProvided = true
		}},
		{"null secret", func(e *model.ThirdpartyOAuth2ProviderEntity) { e.ClientSecretProvided = true }},
		{"both null credentials", func(e *model.ThirdpartyOAuth2ProviderEntity) {
			e.ClientIDProvided = true
			e.ClientSecretProvided = true
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			invalid := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
			test.invalidate(invalid)
			require.Error(t, fixture.service.Create(context.Background(), invalid))
			assert.Zero(t, fixture.repo.creates)
			assert.Zero(t, fixture.crypto.encryptCalls)
			assert.Zero(t, fixture.keys.calls)

			existing := credentialSourceEntity(invalid.ID, model.CredentialSourceStored)
			require.NoError(t, fixture.service.Create(context.Background(), existing))
			before := persistedCredentialSource(t, fixture, existing.ID)
			invalid = credentialSourceEntity(existing.ID, model.CredentialSourceFilesystem)
			invalid.DisplayName = "Rejected replacement"
			invalid.ProtectedResources = []string{"https://api.example/rejected"}
			test.invalidate(invalid)
			encryptCalls, keyCalls := fixture.crypto.encryptCalls, fixture.keys.calls
			require.Error(t, fixture.service.Update(context.Background(), invalid, &before.Version))
			assert.Zero(t, fixture.repo.updates)
			assert.Equal(t, encryptCalls, fixture.crypto.encryptCalls)
			assert.Equal(t, keyCalls, fixture.keys.calls)
			assertCredentialSourceUnchanged(t, fixture, before)
		})
	}
}

func TestCredentialSourceOmittedUpdatePreservesSelectedMode(t *testing.T) {
	for _, source := range []model.CredentialSource{model.CredentialSourceStored, model.CredentialSourceFilesystem} {
		t.Run(string(source), func(t *testing.T) {
			fixture := newCredentialSourceFixture(t)
			entity := credentialSourceEntity(id.NewServiceID(), source)
			require.NoError(t, fixture.service.Create(context.Background(), entity))
			before := persistedCredentialSource(t, fixture, entity.ID)
			update := credentialSourceEntity(entity.ID, source)
			update.CredentialSource = ""
			update.CredentialSourceProvided = false
			update.CanonicalID = nil
			update.DisplayName = "Updated metadata"
			update.ProtectedResources = nil
			encryptCalls, decryptCalls := fixture.crypto.encryptCalls, fixture.crypto.decryptCalls

			require.NoError(t, fixture.service.Update(context.Background(), update, nil))

			after := persistedCredentialSource(t, fixture, entity.ID)
			assert.Equal(t, source, after.CredentialSource)
			assert.Equal(t, before.CanonicalID, after.CanonicalID)
			assert.Equal(t, before.ProtectedResources, after.ProtectedResources)
			assert.Equal(t, "Updated metadata", after.DisplayName)
			assert.Equal(t, before.Version+1, after.Version)
			assert.False(t, after.CredentialSourceTransitioned)
			if source == model.CredentialSourceFilesystem {
				assert.True(t, after.ClientID.IsZero())
				assert.True(t, after.Secret.IsAbsent())
				assert.Equal(t, encryptCalls, fixture.crypto.encryptCalls)
				assert.Equal(t, decryptCalls, fixture.crypto.decryptCalls)
			} else {
				assert.Equal(t, before.ClientID, after.ClientID)
				read, err := fixture.service.Get(context.Background(), entity.ID)
				require.NoError(t, err)
				secret, err := read.Secret.GetPlaintext()
				require.NoError(t, err)
				assert.Equal(t, "inline-secret", secret)
			}
		})
	}
}

func TestCredentialSourceFilesystemMetadataDoesNotNeedDecryption(t *testing.T) {
	fixture := newCredentialSourceFixture(t)
	entity := credentialSourceEntity(id.NewServiceID(), model.CredentialSourceFilesystem)
	require.NoError(t, fixture.service.Create(context.Background(), entity))
	fixture.crypto.encryptErr = errors.New("stored encryption backend unavailable")
	update := credentialSourceEntity(entity.ID, model.CredentialSourceFilesystem)
	update.DisplayName = "Still administrable"
	update.ProtectedResources = nil
	require.NoError(t, fixture.service.Update(context.Background(), update, nil))
	read, err := fixture.service.Get(context.Background(), entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "Still administrable", read.DisplayName)
	assert.True(t, read.Secret.IsAbsent())
	assert.Zero(t, fixture.crypto.encryptCalls)
	assert.Zero(t, fixture.crypto.decryptCalls)
}
