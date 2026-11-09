package memory

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderEntityCopy_Success(t *testing.T) {
	ciphertext := []byte("encrypted-bytes")
	original := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001"),
		DisplayName: "Test Provider",
		ClientID:    id.ClientID("client-abc"),
		Secret:      model.NewEncryptedSecret(ciphertext),
		IssuerURI:   "https://issuer.example.com",
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
		},
		ProtectedResources: []string{"https://api.example.com"},
	}

	copied, err := providerEntityCopy(original)
	require.NoError(t, err)
	require.NotNil(t, copied)

	assert.Equal(t, original.ID, copied.ID)
	assert.Equal(t, original.DisplayName, copied.DisplayName)
	assert.True(t, copied.Secret.IsEncrypted())

	ct, err := copied.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, ciphertext, ct)
}

func TestProviderEntityCopy_ResolvesOmittedClientRegistrationOnce(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider *model.ThirdpartyOAuth2ProviderEntity
		want     model.ClientRegistration
	}{
		{"static confidential", testProvider(id.NewServiceID()), model.ClientRegistrationStatic},
		{"CIMD confidential", testCIMDProvider(id.NewServiceID()), model.ClientRegistrationCIMD},
	} {
		t.Run(tt.name, func(t *testing.T) {
			copied, err := providerEntityCopy(tt.provider)
			require.NoError(t, err)
			assert.Equal(t, tt.want, copied.ClientRegistration)
			assert.False(t, copied.ClientRegistrationProvided)

			again, err := providerEntityCopy(copied)
			require.NoError(t, err)
			assert.Equal(t, tt.want, again.ClientRegistration)
		})
	}
}

func TestProviderEntityCopy_RejectsCorruptClientRegistration(t *testing.T) {
	for _, registration := range []model.ClientRegistration{"", "other"} {
		t.Run(string(registration), func(t *testing.T) {
			provider := testProvider(id.NewServiceID())
			provider.ClientRegistration = registration
			provider.ClientRegistrationProvided = true
			_, err := providerEntityCopy(provider)
			require.Error(t, err)
		})
	}
}

func TestProviderEntityCopy_NilEntityFails(t *testing.T) {
	_, err := providerEntityCopy(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestProviderEntityCopy_PlaintextSecretFails(t *testing.T) {
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001"),
		DisplayName: "Provider",
		ClientID:    id.ClientID("client-1"),
		Secret:      model.NewPlaintextSecret("plain-secret"),
		IssuerURI:   "https://issuer.example.com",
	}

	_, err := providerEntityCopy(entity)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encrypted")
}

func TestProviderEntityCopy_IsolatesFromOriginal(t *testing.T) {
	original := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001"),
		DisplayName:        "Original",
		ClientID:           id.ClientID("client-1"),
		Secret:             model.NewEncryptedSecret([]byte("ct")),
		IssuerURI:          "https://issuer.example.com",
		ProtectedResources: []string{"https://resource.example.com"},
		Scopes:             []model.OAuthScope{{ScopeValue: "read", Description: "Read"}},
	}

	copied, err := providerEntityCopy(original)
	require.NoError(t, err)

	// Mutate original — copy must remain unchanged
	original.DisplayName = "Mutated"
	original.ProtectedResources[0] = "https://mutated.example.com"
	original.Scopes[0].ScopeValue = "mutated"

	assert.Equal(t, "Original", copied.DisplayName)
	assert.Equal(t, "https://resource.example.com", copied.ProtectedResources[0])
	assert.Equal(t, "read", copied.Scopes[0].ScopeValue)
	original.AuthorizationParams = map[string]string{"business_partner_id": "12345"}
	copied, err = providerEntityCopy(original)
	require.NoError(t, err)
	original.AuthorizationParams["business_partner_id"] = "changed"
	assert.Equal(t, "12345", copied.AuthorizationParams["business_partner_id"])
}

func TestProviderRecordConversion_MaterializesVersionedChildResources(t *testing.T) {
	original := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001"),
		DisplayName:        "Provider",
		ClientID:           id.ClientID("client-1"),
		Secret:             model.NewEncryptedSecret([]byte("ct")),
		IssuerURI:          "https://issuer.example.com",
		ProtectedResources: []string{"https://stale.example.com"},
		Version:            7,
	}

	record, err := providerEntityToRecord(original)
	require.NoError(t, err)
	assert.Nil(t, record.entity.ProtectedResources)

	materialized := providerRecordToEntity(record, map[string]struct{}{
		"https://z.example.com": {},
		"https://a.example.com": {},
	})
	require.NotNil(t, materialized)
	assert.EqualValues(t, 7, materialized.Version)
	assert.Equal(t, []string{"https://a.example.com", "https://z.example.com"}, materialized.ProtectedResources)
	assert.True(t, materialized.Secret.IsEncrypted())
}

func TestResourceSetSlice_EmptyReturnsNonNilSlice(t *testing.T) {
	resources := resourceSetSlice(map[string]struct{}{})
	assert.NotNil(t, resources)
	assert.Empty(t, resources)
}
