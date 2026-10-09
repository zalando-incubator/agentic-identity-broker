package model

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

const (
	insecureTokenEndpointError     = "token_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)"
	insecureAuthorizeEndpointError = "authorize_endpoint must be a valid HTTPS URL (HTTP allowed only for localhost in dev mode)"
)

func validEndpointSchemeEntity() *ThirdpartyOAuth2ProviderEntity {
	return &ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Provider",
		ClientID:    "client-id",
		Secret:      NewPlaintextSecret("client-secret"),
		Flavor:      OAuth2FlavorStandard,
		IssuerURI:   "https://issuer.example.com",
		Endpoints: OAuth2Endpoints{
			TokenEndpoint:     "https://issuer.example.com/token",
			AuthorizeEndpoint: "https://issuer.example.com/authorize",
		},
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForCreate_EndpointScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		enableDiscovery     bool
		skipHTTPSValidation bool
		endpoints           OAuth2Endpoints
		wantErr             string
	}{
		{
			name:                "rejects HTTP token endpoint when discovery is disabled",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
			wantErr: insecureTokenEndpointError,
		},
		{
			name:                "rejects HTTP authorization endpoint when discovery is disabled",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
			wantErr: insecureAuthorizeEndpointError,
		},
		{
			name:                "rejects HTTP fallback token endpoint when discovery is enabled",
			enableDiscovery:     true,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
			wantErr: insecureTokenEndpointError,
		},
		{
			name:                "rejects HTTP fallback authorization endpoint when discovery is enabled",
			enableDiscovery:     true,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
			wantErr: insecureAuthorizeEndpointError,
		},
		{
			name:                "allows loopback HTTP endpoints without skipping validation",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://127.0.0.1:18080/token",
				AuthorizeEndpoint: "http://127.0.0.1:18080/authorize",
			},
		},
		{
			name:                "allows HTTP endpoints when validation is explicitly skipped",
			enableDiscovery:     false,
			skipHTTPSValidation: true,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := validEndpointSchemeEntity()
			entity.Discovery.EnableDiscovery = tt.enableDiscovery
			entity.Endpoints = tt.endpoints

			err := entity.ValidateForCreate(tt.skipHTTPSValidation)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateForUpdate_EndpointScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		enableDiscovery     bool
		skipHTTPSValidation bool
		endpoints           OAuth2Endpoints
		wantErr             string
	}{
		{
			name:                "rejects HTTP token endpoint when discovery is disabled",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
			wantErr: insecureTokenEndpointError,
		},
		{
			name:                "rejects HTTP authorization endpoint when discovery is disabled",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
			wantErr: insecureAuthorizeEndpointError,
		},
		{
			name:                "rejects HTTP fallback token endpoint when discovery is enabled",
			enableDiscovery:     true,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "https://issuer.example.com/authorize",
			},
			wantErr: insecureTokenEndpointError,
		},
		{
			name:                "rejects HTTP fallback authorization endpoint when discovery is enabled",
			enableDiscovery:     true,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "https://issuer.example.com/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
			wantErr: insecureAuthorizeEndpointError,
		},
		{
			name:                "allows loopback HTTP endpoints without skipping validation",
			enableDiscovery:     false,
			skipHTTPSValidation: false,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://127.0.0.1:18080/token",
				AuthorizeEndpoint: "http://127.0.0.1:18080/authorize",
			},
		},
		{
			name:                "allows HTTP endpoints when validation is explicitly skipped",
			enableDiscovery:     false,
			skipHTTPSValidation: true,
			endpoints: OAuth2Endpoints{
				TokenEndpoint:     "http://attacker.invalid/token",
				AuthorizeEndpoint: "http://attacker.invalid/authorize",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			entity := validEndpointSchemeEntity()
			entity.Discovery.EnableDiscovery = tt.enableDiscovery
			entity.Endpoints = tt.endpoints

			err := entity.ValidateForUpdate(tt.skipHTTPSValidation)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ClientAuthenticationBySourceAndStage(t *testing.T) {
	t.Parallel()

	resourceURL := "https://resource.example.com/mcp"
	plaintext := NewPlaintextSecret("provider-issued-credential")
	encrypted := NewEncryptedSecret([]byte("opaque-ciphertext"))
	absent := NewAbsentSecret()
	dcr := DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: ClientBootstrapDCR}
	cimd := DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: ClientBootstrapCIMD}
	manual := DiscoveryConfig{}
	directMetadata := DiscoveryConfig{EnableDiscovery: true}
	tests := []struct {
		name       string
		discovery  DiscoveryConfig
		method     TokenEndpointAuthMethod
		secret     Secret
		cimd       bool
		wantValid  bool
		wantPublic bool
	}{
		{"DCR Basic with plaintext credential", dcr, TokenEndpointAuthMethodClientSecretBasic, plaintext, false, true, false},
		{"DCR POST with plaintext credential", dcr, TokenEndpointAuthMethodClientSecretPost, plaintext, false, true, false},
		{"DCR Basic without credential", dcr, TokenEndpointAuthMethodClientSecretBasic, absent, false, false, false},
		{"DCR POST without credential", dcr, TokenEndpointAuthMethodClientSecretPost, absent, false, false, false},
		{"DCR public without credential", dcr, TokenEndpointAuthMethodNone, absent, false, true, true},
		{"DCR public with plaintext credential", dcr, TokenEndpointAuthMethodNone, plaintext, false, false, true},
		{"DCR public with encrypted credential", dcr, TokenEndpointAuthMethodNone, encrypted, false, false, true},
		{"DCR cannot omit selected method", dcr, "", plaintext, false, false, false},
		{"resource discovery without DCR cannot use Basic", DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL}, TokenEndpointAuthMethodClientSecretBasic, plaintext, false, false, false},
		{"resource discovery without DCR cannot use POST", DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL}, TokenEndpointAuthMethodClientSecretPost, plaintext, false, false, false},
		{"manual Basic remains unsupported", manual, TokenEndpointAuthMethodClientSecretBasic, plaintext, false, false, false},
		{"manual POST remains unsupported", manual, TokenEndpointAuthMethodClientSecretPost, plaintext, false, false, false},
		{"direct metadata Basic remains unsupported", directMetadata, TokenEndpointAuthMethodClientSecretBasic, plaintext, false, false, false},
		{"direct metadata POST remains unsupported", directMetadata, TokenEndpointAuthMethodClientSecretPost, plaintext, false, false, false},
		{"manual confidential without method retains auto-detection", manual, "", plaintext, false, true, false},
		{"manual confidential without method needs credential", manual, "", absent, false, false, false},
		{"manual public remains secretless", manual, TokenEndpointAuthMethodNone, absent, false, true, true},
		{"hosted CIMD remains secretless", cimd, TokenEndpointAuthMethodPrivateKeyJWT, absent, true, true, false},
		{"hosted CIMD rejects shared secret", cimd, TokenEndpointAuthMethodPrivateKeyJWT, plaintext, true, false, false},
	}
	for _, operation := range []struct {
		name     string
		validate func(*ThirdpartyOAuth2ProviderEntity) error
	}{
		{"create", func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForCreate(false) }},
		{"update", func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForUpdate(false) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					entity := validEndpointSchemeEntity()
					entity.Discovery = tc.discovery
					entity.TokenEndpointAuthMethod = tc.method
					entity.Secret = tc.secret
					if tc.discovery.ResourceURL != nil {
						entity.AuthorizationParams = map[string]string{"resource": resourceURL}
					}
					if tc.cimd {
						entity.ClientID = ""
					}
					err := operation.validate(entity)
					if tc.wantValid {
						require.NoError(t, err)
						require.Equal(t, tc.wantPublic, entity.IsPublicClient())
						if tc.wantPublic || tc.cimd {
							require.True(t, entity.Secret.IsAbsent())
						} else {
							require.True(t, entity.Secret.IsPlaintext())
						}
						return
					}
					require.Error(t, err)
				})
			}
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_DCRRetainsEncryptedCredentialOnUpdate(t *testing.T) {
	t.Parallel()
	for _, method := range []TokenEndpointAuthMethod{
		TokenEndpointAuthMethodClientSecretBasic,
		TokenEndpointAuthMethodClientSecretPost,
	} {
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()
			entity := validEndpointSchemeEntity()
			resourceURL := "https://resource.example.com/mcp"
			entity.Discovery = DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: ClientBootstrapDCR}
			entity.AuthorizationParams = map[string]string{"resource": resourceURL}
			entity.TokenEndpointAuthMethod = method
			entity.Secret = NewEncryptedSecret([]byte("opaque-ciphertext"))
			require.Error(t, entity.ValidateForCreate(false), "new DCR credentials must be plaintext before encryption")
			require.NoError(t, entity.ValidateForUpdate(false), "an unchanged DCR identity retains its encrypted credential")
			require.True(t, entity.Secret.IsEncrypted())
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_DCRSecretCopiesDoNotExposeResponseCredential(t *testing.T) {
	t.Parallel()

	for _, method := range []TokenEndpointAuthMethod{
		TokenEndpointAuthMethodClientSecretBasic,
		TokenEndpointAuthMethodClientSecretPost,
	} {
		t.Run(string(method), func(t *testing.T) {
			t.Parallel()
			resourceURL := "https://resource.example.com/mcp"
			entity := validEndpointSchemeEntity()
			entity.Discovery = DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: ClientBootstrapDCR}
			entity.AuthorizationParams = map[string]string{"resource": resourceURL}
			entity.TokenEndpointAuthMethod = method
			entity.Secret = NewPlaintextSecret("provider-issued-credential")

			response := entity.RedactedCopy()
			require.NotSame(t, entity, response)
			require.Equal(t, method, response.TokenEndpointAuthMethod)
			require.Equal(t, ClientBootstrapDCR, response.Discovery.ClientMethod)
			responseSecret, err := response.Secret.GetPlaintext()
			require.NoError(t, err)
			require.Equal(t, "REDACTED", responseSecret)
			copiedResponseSecret, err := response.Copy().Secret.GetPlaintext()
			require.NoError(t, err)
			require.Equal(t, "REDACTED", copiedResponseSecret)

			// Persistence and repository copies start from ciphertext; Copy must not turn it into plaintext.
			entity.Secret = NewEncryptedSecret([]byte("opaque-ciphertext"))
			require.NoError(t, entity.Validate())
			storedCopy := entity.Copy()
			require.True(t, storedCopy.Secret.IsEncrypted())
			_, err = storedCopy.Secret.GetPlaintext()
			require.Error(t, err)
			storedResponseSecret, err := storedCopy.RedactedCopy().Secret.GetPlaintext()
			require.NoError(t, err)
			require.Equal(t, "REDACTED", storedResponseSecret)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_Validate_RejectsInvalidCIMDDiscoverySource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		change  func(*ThirdpartyOAuth2ProviderEntity)
		wantErr string
	}{
		{"conflicting authorization server metadata", func(e *ThirdpartyOAuth2ProviderEntity) {
			metadataURL := "https://auth.example.test/.well-known/oauth-authorization-server"
			e.Discovery.MetadataURL = &metadataURL
		}, "metadata_url"},
		{"unsafe protected resource URL", func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "https://127.0.0.1/mcp"
		}, "resource_url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := discoveryEntity()
			e.ClientID = "https://broker.example.test/.well-known/oauth-client/registered"
			tc.change(e)
			require.ErrorContains(t, e.Validate(), tc.wantErr)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_TokenEndpointResourceQueryIsDiscoveryOnly(t *testing.T) {
	t.Parallel()
	for _, operation := range []struct {
		name     string
		validate func(*ThirdpartyOAuth2ProviderEntity) error
	}{
		{"create", func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForCreate(false) }},
		{"update", func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForUpdate(false) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			for _, query := range []string{
				"?resource=https%3A%2F%2Fother.example.test&resource=https%3A%2F%2Fmcp.example.test%2Fmcp",
				"?resource=https%3A%2F%2Fother.example.test&res%6Furce=https%3A%2F%2Fmcp.example.test%2Fmcp",
				"?Resource=https%3A%2F%2Fother.example.test%2Faudience",
				"?rEsOuRcE=https%3A%2F%2Fother.example.test%2Faudience",
			} {
				t.Run(query, func(t *testing.T) {
					t.Parallel()
					discovered := discoveryEntity()
					discovered.Endpoints.TokenEndpoint += query
					require.ErrorContains(t, operation.validate(discovered), "token_endpoint must not contain a resource query parameter")

					manual := validEndpointSchemeEntity()
					manual.Endpoints.TokenEndpoint += query
					require.NoError(t, operation.validate(manual), "manual providers retain their existing endpoint query behavior")
				})
			}

			metadataURL := "https://auth.example.test/.well-known/oauth-authorization-server"
			directMetadata := validEndpointSchemeEntity()
			directMetadata.Discovery = DiscoveryConfig{EnableDiscovery: true, MetadataURL: &metadataURL}
			directMetadata.Endpoints.TokenEndpoint += "?Resource=https%3A%2F%2Fother.example.test%2Fdata"
			require.NoError(t, operation.validate(directMetadata), "direct metadata discovery retains its existing endpoint query behavior")
		})
	}
}
