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
