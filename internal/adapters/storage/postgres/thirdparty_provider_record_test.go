package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCiphertext = []byte("encrypted-secret-bytes")

func newTestEntity() *model.ThirdpartyOAuth2ProviderEntity {
	metaURL := "https://example.com/.well-known/openid-configuration"
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
		DisplayName: "Test Provider",
		ClientID:    id.ClientID("client-123"),
		Secret:      model.NewEncryptedSecret(testCiphertext),
		IssuerURI:   "https://example.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
			MetadataURL:     &metaURL,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://example.com/token",
			AuthorizeEndpoint: "https://example.com/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
			{ScopeValue: "write", Description: "Write access"},
		},
		ProtectedResources: []string{"https://api.example.com"},
		CreatedAt:          time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	}
}

func TestProviderRecord_AuthorizationParamsRoundTrip(t *testing.T) {
	entity := newTestEntity()
	entity.AuthorizationParams = map[string]string{"business_partner_id": "12345"}

	record, err := entityToRecord(entity)
	require.NoError(t, err)
	entity.AuthorizationParams["business_partner_id"] = "changed"
	assert.Equal(t, "12345", record.AuthorizationParams["business_partner_id"])

	roundTripped, err := recordToEntity(record)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"business_partner_id": "12345"}, roundTripped.AuthorizationParams)
}

func TestEntityToRecord_Success(t *testing.T) {
	entity := newTestEntity()
	record, err := entityToRecord(entity)
	require.NoError(t, err)
	require.NotNil(t, record)

	assert.Equal(t, entity.ID.String(), record.ID)
	assert.Equal(t, entity.DisplayName, record.DisplayName)
	require.NotNil(t, record.ClientID)
	assert.Equal(t, string(entity.ClientID), *record.ClientID)
	assert.Equal(t, string(model.CredentialSourceStored), record.CredentialSource)
	assert.Equal(t, testCiphertext, record.SecretCiphertext)
	assert.Equal(t, entity.IssuerURI, record.IssuerURI)
	assert.Equal(t, entity.Discovery.EnableDiscovery, record.EnableDiscovery)
	assert.Equal(t, entity.Discovery.MetadataURL, record.MetadataURL)
	assert.Equal(t, entity.Endpoints.TokenEndpoint, record.TokenEndpoint)
	assert.Equal(t, entity.Endpoints.AuthorizeEndpoint, record.AuthorizeEndpoint)
	assert.Equal(t, entity.ProtectedResources, record.ProtectedResources)
	assert.Equal(t, entity.CreatedAt, record.CreatedAt)
	assert.Equal(t, entity.UpdatedAt, record.UpdatedAt)

	require.Len(t, record.Scopes, 2)
	assert.Equal(t, "read", record.Scopes[0].ScopeValue)
	assert.Equal(t, "write", record.Scopes[1].ScopeValue)
}

func TestEntityToRecord_NilEntity(t *testing.T) {
	_, err := entityToRecord(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestEntityToRecord_PlaintextSecretFails(t *testing.T) {
	entity := newTestEntity()
	entity.Secret = model.NewPlaintextSecret("plaintext-secret")

	_, err := entityToRecord(entity)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "encrypted")
}

func TestEntityToRecord_MetadataURLNilWhenNotSet(t *testing.T) {
	entity := newTestEntity()
	entity.Discovery.MetadataURL = nil

	record, err := entityToRecord(entity)
	require.NoError(t, err)
	assert.Nil(t, record.MetadataURL)
}

func TestEntityToRecord_EmptySlicesHandled(t *testing.T) {
	entity := newTestEntity()
	entity.Scopes = nil
	entity.ProtectedResources = nil

	record, err := entityToRecord(entity)
	require.NoError(t, err)
	assert.Nil(t, record.Scopes)
	assert.Nil(t, record.ProtectedResources)
}

func TestEntityToRecord_DeepCopiesSlices(t *testing.T) {
	entity := newTestEntity()
	record, err := entityToRecord(entity)
	require.NoError(t, err)

	// Mutate original entity's protected resources
	entity.ProtectedResources[0] = "https://mutated.example.com"
	assert.Equal(t, "https://api.example.com", record.ProtectedResources[0],
		"record should not be affected by entity mutation")
}

func TestRecordToEntity_Success(t *testing.T) {
	entity := newTestEntity()
	record, err := entityToRecord(entity)
	require.NoError(t, err)

	roundTripped, err := recordToEntity(record)
	require.NoError(t, err)
	require.NotNil(t, roundTripped)

	assert.Equal(t, entity.ID, roundTripped.ID)
	assert.Equal(t, entity.DisplayName, roundTripped.DisplayName)
	assert.Equal(t, entity.ClientID, roundTripped.ClientID)
	assert.True(t, roundTripped.Secret.IsEncrypted())

	ciphertext, err := roundTripped.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, testCiphertext, ciphertext)

	assert.Equal(t, entity.IssuerURI, roundTripped.IssuerURI)
	assert.Equal(t, entity.Discovery.EnableDiscovery, roundTripped.Discovery.EnableDiscovery)
	assert.Equal(t, entity.Endpoints.TokenEndpoint, roundTripped.Endpoints.TokenEndpoint)
	assert.Equal(t, entity.Endpoints.AuthorizeEndpoint, roundTripped.Endpoints.AuthorizeEndpoint)
	assert.Equal(t, entity.ProtectedResources, roundTripped.ProtectedResources)
	assert.Equal(t, entity.CreatedAt, roundTripped.CreatedAt)
	assert.Equal(t, entity.UpdatedAt, roundTripped.UpdatedAt)

	require.Len(t, roundTripped.Scopes, 2)
	assert.Equal(t, "read", roundTripped.Scopes[0].ScopeValue)
	assert.Equal(t, "Read access", roundTripped.Scopes[0].Description)
}

func TestRecordToEntity_NilRecord(t *testing.T) {
	result, err := recordToEntity(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestRecordToEntity_SecretIsEncryptedState(t *testing.T) {
	clientID := "client-1"
	record := &ThirdpartyOAuth2ProviderRecord{
		ID:               "550e8400-e29b-41d4-a716-446655440099",
		DisplayName:      "Provider",
		ClientID:         &clientID,
		SecretCiphertext: []byte("ciphertext"),
		CredentialSource: string(model.CredentialSourceStored),
		IssuerURI:        "https://issuer.example.com",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	entity, err := recordToEntity(record)
	require.NoError(t, err)
	require.NotNil(t, entity)
	assert.True(t, entity.Secret.IsEncrypted())
	assert.False(t, entity.Secret.IsPlaintext())
}

func TestRecordToEntity_RejectsInvalidClientAuthenticationStates(t *testing.T) {
	none := "none"
	empty := ""
	clientID := "client-1"
	unknown := "client_secret_post"
	tests := []struct {
		name   string
		record *ThirdpartyOAuth2ProviderRecord
	}{
		{
			name: "public service with ciphertext",
			record: &ThirdpartyOAuth2ProviderRecord{
				ID:                      "550e8400-e29b-41d4-a716-446655440099",
				SecretCiphertext:        []byte("ciphertext"),
				TokenEndpointAuthMethod: &none,
			},
		},
		{
			name: "confidential service without ciphertext",
			record: &ThirdpartyOAuth2ProviderRecord{
				ID: "550e8400-e29b-41d4-a716-446655440099",
			},
		},
		{
			name: "empty stored authentication method",
			record: &ThirdpartyOAuth2ProviderRecord{
				ID:                      "550e8400-e29b-41d4-a716-446655440099",
				SecretCiphertext:        []byte("ciphertext"),
				TokenEndpointAuthMethod: &empty,
			},
		},
		{
			name: "unknown stored authentication method",
			record: &ThirdpartyOAuth2ProviderRecord{
				ID:                      "550e8400-e29b-41d4-a716-446655440099",
				TokenEndpointAuthMethod: &unknown,
			},
		},
		{
			name: "empty confidential ciphertext",
			record: &ThirdpartyOAuth2ProviderRecord{
				ID:               "550e8400-e29b-41d4-a716-446655440099",
				SecretCiphertext: []byte{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.record.CredentialSource = string(model.CredentialSourceStored)
			tt.record.ClientID = &clientID
			entity, err := recordToEntity(tt.record)
			require.Error(t, err)
			assert.Nil(t, entity)

			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	}
}

func TestProviderRecord_FilesystemCredentialsRoundTrip(t *testing.T) {
	for _, flavor := range []model.OAuth2Flavor{model.OAuth2FlavorStandard, model.OAuth2FlavorGitHub} {
		t.Run(string(flavor), func(t *testing.T) {
			entity := newTestEntity()
			canonicalID := "filesystem-provider"
			entity.CanonicalID = &canonicalID
			entity.CredentialSource = model.CredentialSourceFilesystem
			entity.CredentialSourceProvided = true
			entity.CredentialSourceTransitioned = true
			entity.Flavor = flavor
			entity.ClientID, entity.Secret = "", model.NewAbsentSecret()

			record, err := entityToRecord(entity)
			require.NoError(t, err)
			assert.Equal(t, string(model.CredentialSourceFilesystem), record.CredentialSource)
			assert.True(t, record.CredentialSourceTransitioned)
			assert.Nil(t, record.ClientID)
			assert.Nil(t, record.SecretCiphertext)
			assert.Nil(t, record.TokenEndpointAuthMethod)

			roundTripped, err := recordToEntity(record)
			require.NoError(t, err)
			assert.Equal(t, model.CredentialSourceFilesystem, roundTripped.CredentialSource)
			assert.True(t, roundTripped.CredentialSourceTransitioned)
			assert.Equal(t, &canonicalID, roundTripped.CanonicalID)
			assert.True(t, roundTripped.ClientID.IsZero())
			assert.True(t, roundTripped.Secret.IsAbsent())
			assert.False(t, roundTripped.CredentialSourceProvided)
			assert.False(t, roundTripped.ClientIDProvided)
			assert.False(t, roundTripped.ClientSecretProvided)
		})
	}
}

func TestRecordToEntity_RejectsCorruptCredentialSources(t *testing.T) {
	empty, inline, unknown, google, none := "", "inline-client", "other", "google", "none"
	invalidCanonical := "not canonical"
	uuidCanonical := "550e8400-e29b-41d4-a716-446655440099"
	tests := []struct {
		name   string
		mutate func(*ThirdpartyOAuth2ProviderRecord)
	}{
		{"unknown source", func(r *ThirdpartyOAuth2ProviderRecord) { r.CredentialSource = unknown }},
		{"empty source", func(r *ThirdpartyOAuth2ProviderRecord) { r.CredentialSource = empty }},
		{"filesystem inline client ID", func(r *ThirdpartyOAuth2ProviderRecord) { r.ClientID = &inline }},
		{"filesystem empty client ID", func(r *ThirdpartyOAuth2ProviderRecord) { r.ClientID = &empty }},
		{"filesystem ciphertext", func(r *ThirdpartyOAuth2ProviderRecord) { r.SecretCiphertext = testCiphertext }},
		{"filesystem empty ciphertext", func(r *ThirdpartyOAuth2ProviderRecord) { r.SecretCiphertext = []byte{} }},
		{"filesystem public authentication", func(r *ThirdpartyOAuth2ProviderRecord) { r.TokenEndpointAuthMethod = &none }},
		{"filesystem empty authentication", func(r *ThirdpartyOAuth2ProviderRecord) { r.TokenEndpointAuthMethod = &empty }},
		{"filesystem Google", func(r *ThirdpartyOAuth2ProviderRecord) { r.Flavor = google }},
		{"filesystem unknown flavor", func(r *ThirdpartyOAuth2ProviderRecord) { r.Flavor = unknown }},
		{"filesystem missing canonical ID", func(r *ThirdpartyOAuth2ProviderRecord) { r.CanonicalID = nil }},
		{"filesystem invalid canonical ID", func(r *ThirdpartyOAuth2ProviderRecord) { r.CanonicalID = &invalidCanonical }},
		{"filesystem UUID canonical ID", func(r *ThirdpartyOAuth2ProviderRecord) { r.CanonicalID = &uuidCanonical }},
		{"stored missing client ID", func(r *ThirdpartyOAuth2ProviderRecord) {
			r.CredentialSource, r.SecretCiphertext = string(model.CredentialSourceStored), testCiphertext
		}},
		{"stored empty client ID", func(r *ThirdpartyOAuth2ProviderRecord) {
			r.CredentialSource, r.ClientID, r.SecretCiphertext = string(model.CredentialSourceStored), &empty, testCiphertext
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canonicalID := "filesystem-provider"
			record := &ThirdpartyOAuth2ProviderRecord{
				ID: "550e8400-e29b-41d4-a716-446655440099", CanonicalID: &canonicalID,
				CredentialSource: string(model.CredentialSourceFilesystem), Flavor: string(model.OAuth2FlavorStandard),
			}
			tt.mutate(record)
			entity, err := recordToEntity(record)
			require.Error(t, err)
			assert.Nil(t, entity)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	}
}

func TestProviderScopeArray_ScanAndValue_RoundTrip(t *testing.T) {
	original := providerScopeArray{
		{ScopeValue: "openid", Description: "OpenID Connect"},
		{ScopeValue: "profile", Description: "User profile"},
	}

	// Serialize via Value
	val, err := original.Value()
	require.NoError(t, err)

	jsonBytes, ok := val.([]byte)
	require.True(t, ok)

	// Deserialize via Scan
	var restored providerScopeArray
	err = restored.Scan(jsonBytes)
	require.NoError(t, err)

	require.Len(t, restored, 2)
	assert.Equal(t, "openid", restored[0].ScopeValue)
	assert.Equal(t, "OpenID Connect", restored[0].Description)
	assert.Equal(t, "profile", restored[1].ScopeValue)
}

func TestProviderScopeArray_Scan_NilValue(t *testing.T) {
	var a providerScopeArray
	err := a.Scan(nil)
	require.NoError(t, err)
	assert.Equal(t, providerScopeArray{}, a)
}

func TestProviderScopeArray_Scan_InvalidType(t *testing.T) {
	var a providerScopeArray
	err := a.Scan("not-bytes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []byte")
}

func TestProviderScopeArray_Value_Nil(t *testing.T) {
	var a providerScopeArray
	val, err := a.Value()
	require.NoError(t, err)
	// Should produce an empty JSON array
	jsonBytes, ok := val.([]byte)
	require.True(t, ok)
	assert.JSONEq(t, "[]", string(jsonBytes))
}

func TestProviderScopeArray_JSONSnakeCaseKeys(t *testing.T) {
	a := providerScopeArray{
		{ScopeValue: "read", Description: "Read access"},
	}
	val, err := a.Value()
	require.NoError(t, err)

	jsonBytes, ok := val.([]byte)
	require.True(t, ok)

	// Verify snake_case JSON keys (not PascalCase)
	var raw []map[string]string
	require.NoError(t, json.Unmarshal(jsonBytes, &raw))
	require.Len(t, raw, 1)
	assert.Equal(t, "read", raw[0]["scope_value"], "key must be snake_case scope_value")
	assert.Equal(t, "Read access", raw[0]["description"])
	assert.Empty(t, raw[0]["ScopeValue"], "PascalCase key must not exist")
}
