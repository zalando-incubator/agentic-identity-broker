package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func credentialSourceProvider(source model.CredentialSource, resources ...string) *model.ThirdpartyOAuth2ProviderEntity {
	provider := testProvider(id.NewServiceID(), resources...)
	canonicalID := "credential-source-" + provider.ID.String()
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = source
	if source == model.CredentialSourceFilesystem {
		provider.ClientID = ""
		provider.Secret = model.NewAbsentSecret()
	}
	return provider
}

func TestCredentialSourceMemoryRoundTrips(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		source model.CredentialSource
		method model.TokenEndpointAuthMethod
		flavor model.OAuth2Flavor
		absent bool
	}{
		{name: "stored shared secret", source: model.CredentialSourceStored},
		{name: "filesystem standard", source: model.CredentialSourceFilesystem, flavor: model.OAuth2FlavorStandard, absent: true},
		{name: "filesystem github", source: model.CredentialSourceFilesystem, flavor: model.OAuth2FlavorGitHub, absent: true},
		{name: "stored public", source: model.CredentialSourceStored, method: model.TokenEndpointAuthMethodNone, absent: true},
		{name: "stored CIMD", source: model.CredentialSourceStored, method: cimdPrivateKeyJWTAuthMethod, absent: true},
		{name: "stored Google", source: model.CredentialSourceStored, flavor: model.OAuth2FlavorGoogle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := credentialSourceProvider(tc.source, "https://api.example.test/credential-source")
			provider.TokenEndpointAuthMethod = tc.method
			provider.Flavor = tc.flavor
			if tc.absent {
				provider.Secret = model.NewAbsentSecret()
			}
			if tc.method == cimdPrivateKeyJWTAuthMethod {
				provider.ClientID = id.ClientID(cimdClientIDPrefix + provider.ID.String())
			}
			require.NoError(t, repo.Create(ctx, provider))
			canonicalID := *provider.CanonicalID
			// Mutating the caller's source must not alter any repository projection.
			provider.CredentialSource = "corrupted-by-caller"
			provider.CredentialSourceTransitioned = true
			*provider.CanonicalID = "changed-by-caller"
			get, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			canonical, err := repo.GetByCanonicalID(ctx, canonicalID)
			require.NoError(t, err)
			resource, err := repo.FindByProtectedResource(ctx, "https://api.example.test/credential-source")
			require.NoError(t, err)
			listed, err := repo.List(ctx)
			require.NoError(t, err)
			require.Len(t, listed, 1)
			for _, stored := range []*model.ThirdpartyOAuth2ProviderEntity{get, canonical, resource, listed[0]} {
				assert.Equal(t, tc.source, stored.CredentialSource)
				assert.False(t, stored.CredentialSourceTransitioned)
				assert.Equal(t, tc.absent, stored.Secret.IsAbsent())
				assert.EqualValues(t, 1, stored.Version)
				assert.Equal(t, tc.method, stored.TokenEndpointAuthMethod)
				assert.Equal(t, canonicalID, *stored.CanonicalID)
				if tc.source == model.CredentialSourceFilesystem {
					assert.Empty(t, stored.ClientID)
					_, err := stored.Secret.GetCiphertext()
					assert.Error(t, err, "filesystem has no ciphertext placeholder")
				} else {
					assert.NotEmpty(t, stored.ClientID)
					if !tc.absent {
						ciphertext, err := stored.Secret.GetCiphertext()
						require.NoError(t, err)
						assert.Equal(t, []byte("ciphertext"), ciphertext)
					}
				}
			}
		})
	}
}

func TestCredentialSourceMemoryRejectsInvalidRecordsWithoutIndexClaims(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*model.ThirdpartyOAuth2ProviderEntity)
	}{
		{"unknown source", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.CredentialSource = "other" }},
		{"stored missing client ID", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.CredentialSource = model.CredentialSourceStored
			p.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
		}},
		{"stored absent secret", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.CredentialSource = model.CredentialSourceStored
			p.ClientID = "stored-client"
		}},
		{"stored plaintext secret", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.CredentialSource = model.CredentialSourceStored
			p.ClientID = "stored-client"
			p.Secret = model.NewPlaintextSecret("must-not-persist")
		}},
		{"stored empty ciphertext", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.CredentialSource = model.CredentialSourceStored
			p.ClientID = "stored-client"
			p.Secret = model.NewEncryptedSecret(nil)
		}},
		{"filesystem inline client ID", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.ClientID = "inline-client" }},
		{"filesystem encrypted secret", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.Secret = model.NewEncryptedSecret([]byte("ciphertext"))
		}},
		{"filesystem plaintext secret", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.Secret = model.NewPlaintextSecret("inline-secret") }},
		{"filesystem no canonical ID", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.CanonicalID = nil }},
		{"filesystem invalid canonical ID", func(p *model.ThirdpartyOAuth2ProviderEntity) { value := "invalid canonical"; p.CanonicalID = &value }},
		{"filesystem empty canonical ID", func(p *model.ThirdpartyOAuth2ProviderEntity) { value := ""; p.CanonicalID = &value }},
		{"filesystem overlong canonical ID", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			value := strings.Repeat("a", 129)
			p.CanonicalID = &value
		}},
		{"filesystem UUID canonical ID", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			value := id.NewServiceID().String()
			p.CanonicalID = &value
		}},
		{"filesystem public", func(p *model.ThirdpartyOAuth2ProviderEntity) {
			p.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
		}},
		{"filesystem CIMD", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod }},
		{"filesystem Google", func(p *model.ThirdpartyOAuth2ProviderEntity) { p.Flavor = model.OAuth2FlavorGoogle }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := credentialSourceProvider(model.CredentialSourceFilesystem, "https://api.example.test/rejected")
			canonicalID := *provider.CanonicalID
			tc.mutate(provider)
			requireStorageKind(t, repo.Create(ctx, provider), storage.ErrorKindValidation)
			_, err := repo.Get(ctx, provider.ID)
			requireStorageKind(t, err, storage.ErrorKindNotFound)
			// Failed validation must not reserve resource or canonical ownership.
			valid := credentialSourceProvider(model.CredentialSourceStored, "https://api.example.test/rejected")
			valid.CanonicalID = &canonicalID
			require.NoError(t, repo.Create(ctx, valid))
			owner, err := repo.FindByProtectedResource(ctx, "https://api.example.test/rejected")
			require.NoError(t, err)
			assert.Equal(t, valid.ID, owner.ID)
		})
	}
}

func TestCredentialSourceMemoryCanonicalOwnershipIsExact(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	upper := credentialSourceProvider(model.CredentialSourceFilesystem)
	lower := credentialSourceProvider(model.CredentialSourceFilesystem)
	upperCanonical, lowerCanonical := "Platform.v1", "platform.v1"
	upper.CanonicalID, lower.CanonicalID = &upperCanonical, &lowerCanonical
	require.NoError(t, repo.Create(ctx, upper))
	require.NoError(t, repo.Create(ctx, lower))
	for _, provider := range []*model.ThirdpartyOAuth2ProviderEntity{upper, lower} {
		found, err := repo.GetByCanonicalID(ctx, *provider.CanonicalID)
		require.NoError(t, err)
		assert.Equal(t, provider.ID, found.ID)
	}
	duplicate := credentialSourceProvider(model.CredentialSourceFilesystem)
	duplicate.CanonicalID = &upperCanonical
	requireStorageKind(t, repo.Create(ctx, duplicate), storage.ErrorKindConflict)
}
