package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var credentialSourceValidators = []struct {
	name     string
	validate func(*ThirdpartyOAuth2ProviderEntity) error
}{
	{
		name: "create",
		validate: func(entity *ThirdpartyOAuth2ProviderEntity) error {
			return entity.ValidateForCreate(false)
		},
	},
	{
		name: "update",
		validate: func(entity *ThirdpartyOAuth2ProviderEntity) error {
			return entity.ValidateForUpdate(false)
		},
	},
	{
		name:     "outbound",
		validate: (*ThirdpartyOAuth2ProviderEntity).ValidateOutboundCredentials,
	},
}

func filesystemCredentialSourceEntity() *ThirdpartyOAuth2ProviderEntity {
	entity := validEndpointSchemeEntity()
	canonicalID := "Platform.Employee_1"
	entity.CanonicalID = &canonicalID
	entity.CredentialSource = CredentialSourceFilesystem
	entity.CredentialSourceProvided = true
	entity.ClientID = ""
	entity.Secret = NewAbsentSecret()
	return entity
}

func TestThirdpartyOAuth2ProviderEntity_CredentialSourceSelector(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   CredentialSource
		provided bool
		wantErr  bool
	}{
		{"explicit stored", CredentialSourceStored, true, false},
		{"resolved stored with omitted selector", CredentialSourceStored, false, false},
		{"unknown source", "remote", true, true},
		{"unknown source without request presence", "remote", false, true},
		{"stored case variant", "STORED", true, true},
		{"filesystem case variant", "Filesystem", true, true},
		{"leading whitespace", " stored", true, true},
		{"trailing whitespace", "filesystem ", true, true},
		{"whitespace only", " ", true, true},
		// Both JSON null and an empty selector reach the model as a supplied zero value.
		{"supplied zero source is not omission", "", true, true},
	}

	for _, validation := range credentialSourceValidators {
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					entity := validEndpointSchemeEntity()
					entity.CredentialSource = tt.source
					entity.CredentialSourceProvided = tt.provided
					err := validation.validate(entity)
					if tt.wantErr {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_StoredCredentialSourceRequiresInlineCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*ThirdpartyOAuth2ProviderEntity)
		wantErr   bool
	}{
		{name: "valid inline pair"},
		{
			name: "missing client ID",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientID = ""
			},
			wantErr: true,
		},
		{
			name: "absent secret",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = NewAbsentSecret()
			},
			wantErr: true,
		},
		{
			name: "empty secret",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = NewPlaintextSecret("")
			},
			wantErr: true,
		},
		{
			name: "uninitialized secret",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = Secret{}
			},
			wantErr: true,
		},
		{
			name: "encrypted secret is not registration input",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = NewEncryptedSecret([]byte("stored-ciphertext"))
			},
			wantErr: true,
		},
	}

	for _, validation := range credentialSourceValidators[:2] {
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()
			for _, selector := range []struct {
				name     string
				provided bool
			}{
				{"omitted", false},
				{"explicit stored", true},
			} {
				t.Run(selector.name, func(t *testing.T) {
					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							entity := validEndpointSchemeEntity()
							entity.CredentialSource = CredentialSourceStored
							entity.CredentialSourceProvided = selector.provided
							entity.ClientIDProvided = true
							entity.ClientSecretProvided = true
							if validation.name == "create" && !selector.provided {
								entity.CredentialSource = ""
							}
							if tt.configure != nil {
								tt.configure(entity)
							}
							err := validation.validate(entity)
							if tt.wantErr {
								require.Error(t, err)
								return
							}
							require.NoError(t, err)
							// The validated entity is what registration persists, not a request-field copy.
							assert.Equal(t, CredentialSourceStored, entity.CredentialSource)
							if validation.name == "create" {
								assert.False(t, entity.CredentialSourceTransitioned)
							}
						})
					}
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_FilesystemCredentialSourceInvariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*ThirdpartyOAuth2ProviderEntity)
		wantErr   bool
	}{
		{name: "standard with zero client ID and explicitly absent secret"},
		{
			name: "omitted flavor remains eligible standard",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Flavor = ""
			},
		},
		{
			name: "GitHub remains eligible confidential",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Flavor = OAuth2FlavorGitHub
			},
		},
		{
			name: "omitted selector on resolved filesystem entity",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.CredentialSourceProvided = false
			},
		},
		{
			name: "supplied null or empty client ID",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientIDProvided = true
			},
			wantErr: true,
		},
		{
			name: "supplied null secret",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientSecretProvided = true
			},
			wantErr: true,
		},
		{
			name: "supplied empty secret",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientSecretProvided = true
				entity.Secret = NewPlaintextSecret("")
			},
			wantErr: true,
		},
		{
			name: "both inline fields supplied as null",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientIDProvided = true
				entity.ClientSecretProvided = true
			},
			wantErr: true,
		},
		{
			name: "inline client ID without request presence flag",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientID = "inline-client"
			},
			wantErr: true,
		},
		{
			name: "inline plaintext secret without request presence flag",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = NewPlaintextSecret("inline-secret")
			},
			wantErr: true,
		},
		{
			name: "encrypted secret is not an absent credential",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = NewEncryptedSecret([]byte("stored-ciphertext"))
			},
			wantErr: true,
		},
		{
			name: "uninitialized secret is not explicit absence",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Secret = Secret{}
			},
			wantErr: true,
		},
		{
			name: "inline placeholders are not filesystem credentials",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.ClientID = "placeholder-client"
				entity.Secret = NewPlaintextSecret("REDACTED")
				entity.ClientIDProvided = true
				entity.ClientSecretProvided = true
			},
			wantErr: true,
		},
	}

	for _, validation := range credentialSourceValidators {
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					entity := filesystemCredentialSourceEntity()
					if tt.configure != nil {
						tt.configure(entity)
					}
					err := validation.validate(entity)
					if tt.wantErr {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					assert.Equal(t, CredentialSourceFilesystem, entity.CredentialSource)
					assert.True(t, entity.ClientID.IsZero())
					assert.True(t, entity.Secret.IsAbsent())
					assert.False(t, entity.IsPublicClient(), "filesystem must not masquerade as a public client")
					assert.False(t, entity.IsCIMDConfidentialClient(), "filesystem must retain shared-secret authentication")
					if validation.name == "create" {
						assert.False(t, entity.CredentialSourceTransitioned, "initial filesystem selection is not a transition")
					}
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_FilesystemCredentialSourceRequiresCanonicalID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		canonicalID *string
		wantErr     bool
	}{
		{name: "missing", wantErr: true},
		{name: "empty", canonicalID: new(""), wantErr: true},
		{name: "whitespace", canonicalID: new(" "), wantErr: true},
		{name: "invalid character", canonicalID: new("platform/employee"), wantErr: true},
		{name: "UUID-shaped", canonicalID: new("650e8400-e29b-41d4-a716-446655440001"), wantErr: true},
		{name: "compact UUID-shaped", canonicalID: new("650e8400e29b41d4a716446655440001"), wantErr: true},
		{name: "too long", canonicalID: new(strings.Repeat("a", 129)), wantErr: true},
		{name: "case dots underscores and hyphens", canonicalID: new("Platform.Employee_1-test")},
		{name: "maximum length", canonicalID: new(strings.Repeat("a", 128))},
	}

	for _, validation := range credentialSourceValidators {
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					entity := filesystemCredentialSourceEntity()
					entity.CanonicalID = nil
					if tt.canonicalID != nil {
						canonicalID := *tt.canonicalID
						entity.CanonicalID = &canonicalID
					}
					err := validation.validate(entity)
					if tt.wantErr {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, entity.CanonicalID)
					assert.Equal(t, *tt.canonicalID, *entity.CanonicalID, "validation must preserve exact selector spelling")
				})
			}
		})
	}

	t.Run("clearing canonical ID on update", func(t *testing.T) {
		entity := filesystemCredentialSourceEntity()
		entity.CanonicalID = nil
		entity.ClearCanonicalID = true
		require.Error(t, entity.ValidateForUpdate(false))
	})
}

func TestThirdpartyOAuth2ProviderEntity_CredentialSourceExcludedModes(t *testing.T) {
	t.Parallel()

	modes := []struct {
		name      string
		configure func(*ThirdpartyOAuth2ProviderEntity)
		public    bool
		cimd      bool
	}{
		{
			name: "public",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.TokenEndpointAuthMethod = TokenEndpointAuthMethodNone
				entity.Secret = NewAbsentSecret()
			},
			public: true,
		},
		{
			name: "CIMD confidential",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.TokenEndpointAuthMethod = TokenEndpointAuthMethod("private_key_jwt")
				entity.ClientID = ""
				entity.Secret = NewAbsentSecret()
			},
			cimd: true,
		},
		{
			name: "Google",
			configure: func(entity *ThirdpartyOAuth2ProviderEntity) {
				entity.Flavor = OAuth2FlavorGoogle
				entity.ClientID = ""
				entity.Secret = NewPlaintextSecret(validGoogleServiceAccountJSON)
				entity.IssuerURI = ""
				entity.Endpoints = OAuth2Endpoints{}
			},
		},
	}

	for _, validation := range credentialSourceValidators {
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range modes {
				t.Run(mode.name, func(t *testing.T) {
					for _, source := range []CredentialSource{CredentialSourceStored, CredentialSourceFilesystem} {
						t.Run(string(source), func(t *testing.T) {
							entity := validEndpointSchemeEntity()
							canonicalID := "excluded-service"
							entity.CanonicalID = &canonicalID
							entity.CredentialSource = source
							entity.CredentialSourceProvided = true
							mode.configure(entity)
							clientID, endpoints, issuerURI := entity.ClientID, entity.Endpoints, entity.IssuerURI
							err := validation.validate(entity)
							if source == CredentialSourceFilesystem {
								require.Error(t, err)
								assert.Equal(t, clientID, entity.ClientID, "unsupported source must reject before credential enrichment")
								assert.Equal(t, endpoints, entity.Endpoints)
								assert.Equal(t, issuerURI, entity.IssuerURI)
							} else {
								require.NoError(t, err)
							}
							assert.Equal(t, mode.public, entity.IsPublicClient())
							assert.Equal(t, mode.cimd, entity.IsCIMDConfidentialClient())
						})
					}
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_CredentialSourceNoOpValidationPreservesTransitionEvidence(t *testing.T) {
	t.Parallel()

	for _, source := range []CredentialSource{CredentialSourceStored, CredentialSourceFilesystem} {
		t.Run(string(source), func(t *testing.T) {
			t.Parallel()
			for _, history := range []struct {
				name         string
				transitioned bool
			}{
				{"never transitioned", false},
				{"previously transitioned", true},
			} {
				t.Run(history.name, func(t *testing.T) {
					for _, selector := range []struct {
						name     string
						provided bool
					}{
						{"explicit no-op selection", true},
						{"omitted selector after current-source resolution", false},
					} {
						t.Run(selector.name, func(t *testing.T) {
							entity := validEndpointSchemeEntity()
							if source == CredentialSourceFilesystem {
								entity = filesystemCredentialSourceEntity()
							}
							entity.CredentialSource = source
							entity.CredentialSourceProvided = selector.provided
							entity.CredentialSourceTransitioned = history.transitioned
							require.NoError(t, entity.ValidateForUpdate(false))
							assert.Equal(t, source, entity.CredentialSource)
							assert.Equal(t, history.transitioned, entity.CredentialSourceTransitioned)

							entity.DisplayName = "Renamed provider"
							require.NoError(t, entity.ValidateForUpdate(false))
							assert.Equal(t, source, entity.CredentialSource)
							assert.Equal(t, history.transitioned, entity.CredentialSourceTransitioned, "metadata validation cannot reset or invent transition history")
						})
					}
				})
			}
		})
	}
}
