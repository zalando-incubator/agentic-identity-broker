package model

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThirdpartyOAuth2ProviderEntity_Validate(t *testing.T) {
	t.Parallel()
	metadataURL := "https://github.com/.well-known/oauth-authorization-server"

	tests := []struct {
		name    string
		entity  *ThirdpartyOAuth2ProviderEntity
		wantErr string
	}{
		{
			name: "valid entity with plaintext secret",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewPlaintextSecret("super-secret"),
				IssuerURI:   "https://github.com",
				Discovery: DiscoveryConfig{
					EnableDiscovery: true,
					MetadataURL:     &metadataURL,
				},
				Endpoints: OAuth2Endpoints{
					TokenEndpoint:     "https://github.com/login/oauth/access_token",
					AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
				},
				Scopes: []OAuthScope{
					{ScopeValue: "repo", Description: "Full repository access"},
				},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			wantErr: "",
		},
		{
			name: "valid entity with encrypted secret",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewEncryptedSecret([]byte{1, 2, 3, 4}),
				IssuerURI:   "https://github.com",
				Discovery:   DiscoveryConfig{EnableDiscovery: true},
				Endpoints: OAuth2Endpoints{
					TokenEndpoint:     "https://github.com/login/oauth/access_token",
					AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
				},
				Scopes: []OAuthScope{
					{ScopeValue: "repo", Description: "Full repository access"},
				},
			},
			wantErr: "",
		},
		{
			name: "missing ID",
			entity: &ThirdpartyOAuth2ProviderEntity{
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewPlaintextSecret("secret"),
				IssuerURI:   "https://github.com",
			},
			wantErr: "provider ID cannot be empty",
		},
		{
			name: "missing display_name",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:        id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				ClientID:  id.ClientID("Iv1.abcd1234"),
				Secret:    NewPlaintextSecret("secret"),
				IssuerURI: "https://github.com",
			},
			wantErr: "display_name is required",
		},
		{
			name: "display_name too long",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewPlaintextSecret("secret"),
				IssuerURI:   "https://github.com",
			},
			wantErr: "display_name exceeds 255 characters",
		},
		{
			name: "missing client_id",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				Secret:      NewPlaintextSecret("secret"),
				IssuerURI:   "https://github.com",
			},
			wantErr: "client_id is required",
		},
		{
			name: "secret in plaintext state with empty value",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewPlaintextSecret(""),
				IssuerURI:   "https://github.com",
			},
			wantErr: "secret plaintext is empty",
		},
		{
			// A zero-value Secret (var s Secret / Secret{}) is the "uninitialized" state
			// described in the Secret type comment. IsPlaintext() returns true for it, so
			// callers who write "if s.IsPlaintext() { use(s.GetPlaintext()) }" would reach
			// GetPlaintext() — which then errors. Validate() must catch this so that an
			// entity that was never given a real secret is always rejected.
			name: "zero-value Secret (uninitialized) is rejected by Validate",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      Secret{}, // var s Secret — not constructed via NewPlaintextSecret
				IssuerURI:   "https://github.com",
			},
			wantErr: "secret plaintext is empty",
		},
		{
			name: "secret in encrypted state with empty ciphertext",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewEncryptedSecret([]byte{}),
				IssuerURI:   "https://github.com",
			},
			wantErr: "secret ciphertext is empty",
		},
		{
			name: "missing issuer_uri",
			entity: &ThirdpartyOAuth2ProviderEntity{
				ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
				DisplayName: "GitHub",
				ClientID:    id.ClientID("Iv1.abcd1234"),
				Secret:      NewPlaintextSecret("secret"),
			},
			wantErr: "issuer_uri is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.entity.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreate_FlavorDispatch(t *testing.T) {
	t.Parallel()
	// validGoogleServiceAccountJSON is shared with google_service_account_test.go
	// but we redefine it here for independence of test packages (same package, different file).
	googleJSON := `{
  "type": "service_account",
  "project_id": "my-project-123",
  "private_key_id": "key-id-1234",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA2a2rwplBQLf0kTmkGp5RJFBpJOFBBhfJmLO0YjCGSLCuoP7\noc5RfakePrivateKeyDataForTestingPurposesOnlyNotRealCryptographicKey\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "my-service@my-project-123.iam.gserviceaccount.com",
  "client_id": "123456789012345678901",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/my-service%40my-project-123.iam.gserviceaccount.com"
}`

	tests := []struct {
		name    string
		entity  *ThirdpartyOAuth2ProviderEntity
		wantErr string
	}{
		{
			name: "Flavor google with valid service account JSON credential succeeds",
			entity: &ThirdpartyOAuth2ProviderEntity{
				DisplayName: "Google Service",
				Flavor:      OAuth2FlavorGoogle,
				Secret:      NewPlaintextSecret(googleJSON),
				// client_id and issuer_uri are optional for google flavor
				Discovery: DiscoveryConfig{EnableDiscovery: false},
				Endpoints: OAuth2Endpoints{
					TokenEndpoint:     "https://oauth2.googleapis.com/token",
					AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/auth",
				},
				Scopes: []OAuthScope{{ScopeValue: "https://www.googleapis.com/auth/cloud-platform", Description: "Cloud Platform"}},
			},
			wantErr: "",
		},
		{
			name: "Flavor google with invalid JSON credential returns error",
			entity: &ThirdpartyOAuth2ProviderEntity{
				DisplayName: "Google Service",
				Flavor:      OAuth2FlavorGoogle,
				Secret:      NewPlaintextSecret(`{not valid json}`),
				Scopes:      []OAuthScope{{ScopeValue: "openid", Description: "OpenID"}},
			},
			wantErr: "invalid Google service account JSON",
		},
		{
			name: "Flavor standard with empty credential returns error",
			entity: &ThirdpartyOAuth2ProviderEntity{
				DisplayName: "Standard Service",
				ClientID:    "client-123",
				Flavor:      OAuth2FlavorStandard,
				Secret:      NewPlaintextSecret(""),
				IssuerURI:   "https://issuer.example.com",
				Discovery:   DiscoveryConfig{EnableDiscovery: false},
				Endpoints: OAuth2Endpoints{
					TokenEndpoint:     "https://issuer.example.com/token",
					AuthorizeEndpoint: "https://issuer.example.com/authorize",
				},
				Scopes: []OAuthScope{{ScopeValue: "openid", Description: "OpenID"}},
			},
			wantErr: "client_secret",
		},
		{
			name: "Unknown Flavor returns error",
			entity: &ThirdpartyOAuth2ProviderEntity{
				DisplayName: "Unknown Flavor Service",
				ClientID:    "client-123",
				Flavor:      OAuth2Flavor("azure"),
				Secret:      NewPlaintextSecret("some-secret"),
				IssuerURI:   "https://issuer.example.com",
				Scopes:      []OAuthScope{{ScopeValue: "openid", Description: "OpenID"}},
			},
			wantErr: "oauth2_flavor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.entity.ValidateForCreate(true) // skipHTTPSValidation=true for tests
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreateAndUpdate_TokenEndpointAuthMethod(t *testing.T) {
	t.Parallel()
	privateKeyJWT := TokenEndpointAuthMethod("private_key_jwt")

	newConfidentialEntity := func() *ThirdpartyOAuth2ProviderEntity {
		return &ThirdpartyOAuth2ProviderEntity{
			ID:          id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
			DisplayName: "Provider",
			ClientID:    "client-id",
			Secret:      NewPlaintextSecret("client-secret"),
			IssuerURI:   "https://issuer.example.com",
			Discovery:   DiscoveryConfig{EnableDiscovery: true},
		}
	}
	newPublicEntity := func() *ThirdpartyOAuth2ProviderEntity {
		entity := newConfidentialEntity()
		entity.TokenEndpointAuthMethod = TokenEndpointAuthMethodNone
		entity.Secret = NewAbsentSecret()
		return entity
	}
	newCIMDConfidentialEntity := func() *ThirdpartyOAuth2ProviderEntity {
		entity := newConfidentialEntity()
		entity.ClientID = ""
		entity.Secret = NewAbsentSecret()
		entity.TokenEndpointAuthMethod = privateKeyJWT
		entity.Discovery = DiscoveryConfig{EnableDiscovery: false}
		entity.Endpoints = OAuth2Endpoints{
			TokenEndpoint:     "https://issuer.example.com/token",
			AuthorizeEndpoint: "https://issuer.example.com/authorize",
		}
		return entity
	}

	tests := []struct {
		name            string
		newEntity       func() *ThirdpartyOAuth2ProviderEntity
		wantErr         string
		wantErrContains string
		assertEntity    func(*testing.T, *ThirdpartyOAuth2ProviderEntity)
	}{
		{
			name:      "accepts a public client without a secret",
			newEntity: newPublicEntity,
		},
		{
			name: "rejects an unknown token endpoint authentication method",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newConfidentialEntity()
				entity.TokenEndpointAuthMethod = "client_secret_basic"
				return entity
			},
			wantErrContains: "token_endpoint_auth_method",
		},
		{
			name: "rejects public Google before credential-derived enrichment",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newPublicEntity()
				entity.Flavor = OAuth2FlavorGoogle
				entity.Secret = NewPlaintextSecret(validGoogleServiceAccountJSON)
				entity.ClientID = "operator-supplied-client-id"
				entity.IssuerURI = ""
				entity.Endpoints = OAuth2Endpoints{
					TokenEndpoint:     "https://operator.example.com/token",
					AuthorizeEndpoint: "https://operator.example.com/authorize",
				}
				return entity
			},
			wantErr: `token_endpoint_auth_method "none" is not supported for the google flavor: the client identifier is derived from the credential document`,
			assertEntity: func(t *testing.T, entity *ThirdpartyOAuth2ProviderEntity) {
				assert.Equal(t, id.ClientID("operator-supplied-client-id"), entity.ClientID)
				assert.Empty(t, entity.IssuerURI)
				assert.Equal(t, OAuth2Endpoints{
					TokenEndpoint:     "https://operator.example.com/token",
					AuthorizeEndpoint: "https://operator.example.com/authorize",
				}, entity.Endpoints)
			},
		},
		{
			name: "rejects a public client with a secret",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newPublicEntity()
				entity.Secret = NewPlaintextSecret("client-secret")
				return entity
			},
			wantErr: `client_secret must not have a non-empty value when token_endpoint_auth_method is "none"`,
		},
		{
			name: "rejects a public client without a client ID",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newPublicEntity()
				entity.ClientID = ""
				return entity
			},
			wantErr: "client_id is required",
		},
		{
			name: "rejects a static confidential client without a secret",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newConfidentialEntity()
				entity.Secret = NewAbsentSecret()
				return entity
			},
			wantErr: "client_secret is required",
		},
		{
			name:      "accepts static confidential client when authentication method is omitted or null",
			newEntity: newConfidentialEntity,
		},
		{
			name:      "accepts CIMD confidential client while client ID awaits broker generation",
			newEntity: newCIMDConfidentialEntity,
			assertEntity: func(t *testing.T, entity *ThirdpartyOAuth2ProviderEntity) {
				assert.Empty(t, entity.ClientID)
				assert.True(t, entity.Secret.IsAbsent())
			},
		},
		{
			name: "rejects CIMD confidential client with a caller-supplied client ID",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newCIMDConfidentialEntity()
				entity.ClientID = "caller-supplied-client-id"
				return entity
			},
			wantErrContains: "client_id",
		},
		{
			name: "rejects CIMD confidential client with a non-empty secret",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newCIMDConfidentialEntity()
				entity.Secret = NewPlaintextSecret("client-secret")
				return entity
			},
			wantErrContains: "client_secret",
		},
		{
			name: "rejects CIMD confidential Google client before credential-derived enrichment",
			newEntity: func() *ThirdpartyOAuth2ProviderEntity {
				entity := newCIMDConfidentialEntity()
				entity.Flavor = OAuth2FlavorGoogle
				return entity
			},
			wantErrContains: "google",
		},
	}

	validations := []struct {
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
	}

	for _, validation := range validations {
		validation := validation
		t.Run(validation.name, func(t *testing.T) {
			t.Parallel()

			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					entity := tt.newEntity()
					err := validation.validate(entity)
					if tt.wantErr == "" && tt.wantErrContains == "" {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						if tt.wantErr != "" {
							require.EqualError(t, err, tt.wantErr)
						}
						if tt.wantErrContains != "" {
							require.ErrorContains(t, err, tt.wantErrContains)
						}
					}
					if tt.assertEntity != nil {
						tt.assertEntity(t, entity)
					}
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreateAndUpdate_RejectsPublicOutboundCredentials(t *testing.T) {
	t.Parallel()

	newPublicEntity := func(tokenEndpoint string) *ThirdpartyOAuth2ProviderEntity {
		entity := &ThirdpartyOAuth2ProviderEntity{
			ID:                      id.MustParseServiceID("650e8400-e29b-41d4-a716-446655440001"),
			DisplayName:             "Provider",
			ClientID:                "client-id",
			Secret:                  NewAbsentSecret(),
			TokenEndpointAuthMethod: TokenEndpointAuthMethodNone,
			IssuerURI:               "https://issuer.example.com",
			Discovery:               DiscoveryConfig{EnableDiscovery: true},
		}
		if tokenEndpoint != "" {
			entity.Discovery.EnableDiscovery = false
			entity.Endpoints = OAuth2Endpoints{
				TokenEndpoint:     tokenEndpoint,
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			}
		}
		return entity
	}

	tests := []struct {
		name                string
		authorizationParams map[string]string
		tokenEndpoint       string
		wantErr             string
	}{
		{
			name:                "rejects a client assertion authorization parameter",
			authorizationParams: map[string]string{"Client_Assertion": "assertion"},
			wantErr:             "authorization parameter is not allowed for public clients: Client_Assertion",
		},
		{
			name:                "rejects a client assertion type authorization parameter",
			authorizationParams: map[string]string{"CLIENT_ASSERTION_TYPE": "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			wantErr:             "authorization parameter is not allowed for public clients: CLIENT_ASSERTION_TYPE",
		},
		{
			name:          "rejects token endpoint userinfo",
			tokenEndpoint: "https://username:password@issuer.example.com/token",
			wantErr:       "token_endpoint must not include userinfo for public clients",
		},
		{
			name:          "rejects a token endpoint client secret query parameter",
			tokenEndpoint: "https://issuer.example.com/token?client_secret=secret",
			wantErr:       "token_endpoint must not include client authentication parameter for public clients: client_secret",
		},
		{
			name:          "rejects a token endpoint client assertion query parameter",
			tokenEndpoint: "https://issuer.example.com/token?client_assertion=assertion",
			wantErr:       "token_endpoint must not include client authentication parameter for public clients: client_assertion",
		},
		{
			name:          "rejects a token endpoint malformed query",
			tokenEndpoint: "https://issuer.example.com/token?client_secret=%zz",
			wantErr:       "token_endpoint query parameters are invalid",
		},
	}

	validations := []struct {
		name     string
		validate func(*ThirdpartyOAuth2ProviderEntity) error
	}{
		{name: "create", validate: func(entity *ThirdpartyOAuth2ProviderEntity) error { return entity.ValidateForCreate(false) }},
		{name: "update", validate: func(entity *ThirdpartyOAuth2ProviderEntity) error { return entity.ValidateForUpdate(false) }},
	}
	for _, validation := range validations {
		validation := validation
		t.Run(validation.name, func(t *testing.T) {
			for _, tt := range tests {
				tt := tt
				t.Run(tt.name, func(t *testing.T) {
					entity := newPublicEntity(tt.tokenEndpoint)
					entity.AuthorizationParams = tt.authorizationParams
					require.EqualError(t, validation.validate(entity), tt.wantErr)
				})
			}
		})
	}
}
func TestThirdpartyOAuth2ProviderEntity_ValidateForCreate_AllowsEmptyScopes(t *testing.T) {
	t.Parallel()

	for _, scopes := range [][]OAuthScope{nil, {}} {
		entity := &ThirdpartyOAuth2ProviderEntity{
			DisplayName: "Scope-less provider",
			ClientID:    "scope-less-client",
			Secret:      NewPlaintextSecret("scope-less-secret"),
			IssuerURI:   "https://idp.example.com",
			Discovery:   DiscoveryConfig{EnableDiscovery: false},
			Endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://idp.example.com/token",
				AuthorizeEndpoint: "https://idp.example.com/authorize",
			},
			Scopes: scopes,
		}

		require.NoError(t, entity.ValidateForCreate(false))
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForUpdate_AllowsEmptyScopes(t *testing.T) {
	t.Parallel()

	for _, scopes := range [][]OAuthScope{nil, {}} {
		entity := &ThirdpartyOAuth2ProviderEntity{
			ID:          id.NewServiceID(),
			DisplayName: "Scope-less provider",
			ClientID:    "scope-less-client",
			Secret:      NewPlaintextSecret("scope-less-secret"),
			IssuerURI:   "https://idp.example.com",
			Discovery:   DiscoveryConfig{EnableDiscovery: false},
			Endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://idp.example.com/token",
				AuthorizeEndpoint: "https://idp.example.com/authorize",
			},
			Scopes: scopes,
		}

		require.NoError(t, entity.ValidateForUpdate(false))
	}
}

func TestThirdpartyOAuth2ProviderEntity_NormalizeProtectedResources(t *testing.T) {
	t.Parallel()

	t.Run("normalizes each protected resource in place", func(t *testing.T) {
		t.Parallel()

		entity := &ThirdpartyOAuth2ProviderEntity{
			ProtectedResources: []string{
				"https://api.example.com/",
				"https://api.example.com/v1///",
				"https://api.example.com/v2",
			},
		}

		entity.NormalizeProtectedResources()

		assert.Equal(t, []string{
			"https://api.example.com",
			"https://api.example.com/v1",
			"https://api.example.com/v2",
		}, entity.ProtectedResources)
	})

	t.Run("nil slice remains nil", func(t *testing.T) {
		t.Parallel()

		entity := &ThirdpartyOAuth2ProviderEntity{}

		entity.NormalizeProtectedResources()

		assert.Nil(t, entity.ProtectedResources)
	})

	t.Run("empty slice remains empty", func(t *testing.T) {
		t.Parallel()

		entity := &ThirdpartyOAuth2ProviderEntity{ProtectedResources: []string{}}

		entity.NormalizeProtectedResources()

		assert.Empty(t, entity.ProtectedResources)
	})

	t.Run("normalization is idempotent", func(t *testing.T) {
		t.Parallel()

		entity := &ThirdpartyOAuth2ProviderEntity{ProtectedResources: []string{"https://api.example.com///"}}

		entity.NormalizeProtectedResources()
		firstPass := append([]string(nil), entity.ProtectedResources...)
		entity.NormalizeProtectedResources()

		assert.Equal(t, firstPass, entity.ProtectedResources)
	})
}

func TestThirdpartyOAuth2ProviderEntity_AuthorizationParams(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		params map[string]string
		want   string
	}{
		{name: "valid", params: map[string]string{"business_partner_id": "12345"}},
		{name: "blank key", params: map[string]string{" ": "value"}, want: "authorization parameter name cannot be blank"},
		{name: "blank value", params: map[string]string{"name": "\t"}, want: "authorization parameter value cannot be blank"},
		{name: "reserved key", params: map[string]string{"STATE": "value"}, want: "authorization parameter name is reserved"},
		{name: "reserved authorization code", params: map[string]string{"CoDe": "value"}, want: "authorization parameter name is reserved"},
		{name: "reserved grant type", params: map[string]string{"GrAnT_TyPe": "value"}, want: "authorization parameter name is reserved"},
		{name: "reserved PKCE verifier", params: map[string]string{"CODE_VERIFIER": "value"}, want: "authorization parameter name is reserved"},
		{name: "reserved refresh token", params: map[string]string{"REFRESH_TOKEN": "value"}, want: "authorization parameter name is reserved"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAuthorizationParams(tt.params)
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_CopyAuthorizationParams(t *testing.T) {
	entity := &ThirdpartyOAuth2ProviderEntity{AuthorizationParams: map[string]string{"business_partner_id": "12345"}}
	copy := entity.Copy()

	entity.AuthorizationParams["business_partner_id"] = "changed"
	assert.Equal(t, "12345", copy.AuthorizationParams["business_partner_id"])
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreate_ValidatesProtectedResources(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		resources []string
		wantErr   string
	}{
		{name: "empty URI", resources: []string{""}, wantErr: "protected_resources[0]: cannot be empty"},
		{name: "whitespace URI", resources: []string{" \t"}, wantErr: "protected_resources[0]: invalid URI"},
		{name: "relative URI", resources: []string{"/v1/orders"}, wantErr: "protected_resources[0]: must be an absolute URL with scheme and host"},
		{name: "malformed URI", resources: []string{"://invalid"}, wantErr: "protected_resources[0]: invalid URI"},
		{name: "absolute URI", resources: []string{"https://api.example.com/v1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entity := &ThirdpartyOAuth2ProviderEntity{
				DisplayName:        "Provider",
				ClientID:           "client-id",
				Secret:             NewPlaintextSecret("secret"),
				IssuerURI:          "https://issuer.example.com",
				Discovery:          DiscoveryConfig{EnableDiscovery: true},
				ProtectedResources: tt.resources,
			}

			err := entity.ValidateForCreate(false)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreate_RejectsDuplicateProtectedResources(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		resources []string
		wantErr   string
	}{
		{name: "rejects exact duplicates", resources: []string{"https://api.example.com/v1", "https://api.example.com/v1"}, wantErr: "protected_resources[1]: duplicate resource \"https://api.example.com/v1\""},
		{name: "rejects normalized duplicates", resources: []string{"https://api.example.com/v1", "https://api.example.com/v1/"}, wantErr: "protected_resources[1]: duplicate resource \"https://api.example.com/v1\""},
		{name: "allows distinct resources", resources: []string{"https://api.example.com/v1", "https://api.example.com/v2"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entity := &ThirdpartyOAuth2ProviderEntity{
				DisplayName:        "Provider",
				ClientID:           "client-id",
				Secret:             NewPlaintextSecret("secret"),
				IssuerURI:          "https://issuer.example.com",
				Discovery:          DiscoveryConfig{EnableDiscovery: true},
				ProtectedResources: tt.resources,
			}

			err := entity.ValidateForCreate(false)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForUpdate_ValidatesProtectedResources(t *testing.T) {
	entity := &ThirdpartyOAuth2ProviderEntity{
		ID:                 id.NewServiceID(),
		DisplayName:        "Provider",
		ClientID:           "client-id",
		Secret:             NewPlaintextSecret("secret"),
		IssuerURI:          "https://issuer.example.com",
		Discovery:          DiscoveryConfig{EnableDiscovery: true},
		ProtectedResources: []string{"relative/path"},
	}

	require.ErrorContains(t, entity.ValidateForUpdate(false), "protected_resources[0]: must be an absolute URL with scheme and host")
}

func TestNormalizeAndValidateProtectedResource(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name        string
		resourceURI string
		want        string
		wantErr     string
	}{
		{name: "normalizes trailing slashes", resourceURI: "https://api.example.com/v1///", want: "https://api.example.com/v1"},
		{name: "preserves query and fragment", resourceURI: "https://api.example.com/v1/?page=1#section", want: "https://api.example.com/v1?page=1#section"},
		{name: "rejects empty URI", wantErr: "cannot be empty"},
		{name: "rejects relative URI", resourceURI: "/v1", wantErr: "must be an absolute URL with scheme and host"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := NormalizeAndValidateProtectedResource(tt.resourceURI)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateVersion(t *testing.T) {
	entity := &ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Provider",
		ClientID:    "client-id",
		Secret:      NewPlaintextSecret("secret"),
		IssuerURI:   "https://issuer.example.com",
		Version:     -1,
	}

	require.ErrorContains(t, entity.Validate(), "version cannot be negative")
}

func TestThirdpartyOAuth2ProviderEntity_CopyPreservesVersion(t *testing.T) {
	entity := &ThirdpartyOAuth2ProviderEntity{Version: 1}

	assert.Equal(t, int64(1), entity.Copy().Version)
}

func TestThirdpartyOAuth2ProviderCanonicalIDValidationAndCopy(t *testing.T) {
	canonicalID := "github-service"
	entity := &ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		CanonicalID: &canonicalID,
		DisplayName: "GitHub",
		ClientID:    id.ClientID("github-client"),
		Secret:      NewPlaintextSecret("secret"),
		IssuerURI:   "https://github.com",
	}
	require.NoError(t, entity.Validate())

	copy := entity.Copy()
	require.NotNil(t, copy.CanonicalID)
	*copy.CanonicalID = "other-service"
	assert.Equal(t, canonicalID, *entity.CanonicalID)
}
