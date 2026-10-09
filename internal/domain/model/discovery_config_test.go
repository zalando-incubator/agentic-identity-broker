package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestThirdpartyOAuth2ProviderEntity_ProtectedResourceSourceValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		change  func(*ThirdpartyOAuth2ProviderEntity)
		wantErr string
	}{
		{name: "selected resource discovery", change: func(*ThirdpartyOAuth2ProviderEntity) {}},
		{name: "resource requires discovery enabled", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			e.Discovery.EnableDiscovery = false
		}, wantErr: "resource_url"},
		{name: "resource and direct authorization server metadata are exclusive", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			metadataURL := "https://auth.example.test/.well-known/oauth-authorization-server/tenant"
			e.Discovery.MetadataURL = &metadataURL
		}, wantErr: "metadata_url"},
		{name: "resource URL must be absolute", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "/mcp"
		}, wantErr: "resource_url"},
		{name: "resource URL must use HTTPS even with legacy test-mode bypass", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "http://mcp.example.test/mcp"
		}, wantErr: "resource_url"},
		{name: "resource URL must have a host", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "https:///mcp"
		}, wantErr: "resource_url"},
		{name: "resource URL cannot point at loopback", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "https://127.0.0.1/mcp"
		}, wantErr: "resource_url"},
		{name: "resource URL cannot contain userinfo", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "https://user:secret@mcp.example.test/mcp"
		}, wantErr: "resource_url"},
		{name: "resource URL cannot contain a fragment", change: func(e *ThirdpartyOAuth2ProviderEntity) {
			*e.Discovery.ResourceURL = "https://mcp.example.test/mcp#section"
		}, wantErr: "resource_url"},
	}
	for _, operation := range []struct {
		name     string
		validate func(*ThirdpartyOAuth2ProviderEntity) error
	}{
		{name: "create", validate: func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForCreate(true) }},
		{name: "update", validate: func(e *ThirdpartyOAuth2ProviderEntity) error { return e.ValidateForUpdate(true) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			t.Parallel()
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					e := discoveryEntity()
					tc.change(e)
					err := operation.validate(e)
					if tc.wantErr != "" {
						require.ErrorContains(t, err, tc.wantErr)
						return
					}
					require.NoError(t, err)
				})
			}
		})
	}
}

func discoveryRequestEntity() *ThirdpartyOAuth2ProviderEntity {
	resourceURL := "https://mcp.example.test/mcp"
	return &ThirdpartyOAuth2ProviderEntity{
		DisplayName: "Example MCP",
		Secret:      NewAbsentSecret(),
		Discovery:   DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL},
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateDiscoveryRequest_RejectsConflictingInputs(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		change  func(*ThirdpartyOAuth2ProviderEntity)
		wantErr string
	}{
		{"direct metadata source", func(e *ThirdpartyOAuth2ProviderEntity) {
			metadataURL := "https://auth.example.test/.well-known/oauth-authorization-server"
			e.Discovery.MetadataURL = &metadataURL
		}, "metadata_url"},
		{"manual authorization endpoint", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.Endpoints.AuthorizeEndpoint = "https://other.example.test/authorize"
		}, "endpoints"},
		{"manual token endpoint", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.Endpoints.TokenEndpoint = "https://other.example.test/token"
		}, "endpoints"},
		{"manual client identifier", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.ClientID = "operator-client"
		}, "client_id"},
		{"manual client credential", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.Secret = NewPlaintextSecret("operator-credential")
		}, "client_secret"},
		{"operator-selected DCR authentication", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.TokenEndpointAuthMethod = TokenEndpointAuthMethodClientSecretBasic
		}, "token_endpoint_auth_method"},
		{"operator-selected client bootstrap", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.Discovery.ClientMethod = ClientBootstrapDCR
		}, "client_method"},
		{"relative explicit audience", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.AuthorizationParams = map[string]string{"resource": "/other"}
		}, "authorization_params.resource"},
		{"fragment in explicit audience", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.AuthorizationParams = map[string]string{"resource": "https://api.example.test/data#section"}
		}, "authorization_params.resource"},
		{"malformed explicit audience", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.AuthorizationParams = map[string]string{"resource": "https://api.example.test/%zz"}
		}, "authorization_params.resource"},
		{"case-variant resource parameter", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.AuthorizationParams = map[string]string{"Resource": "https://api.example.test/data"}
		}, "authorization_params"},
		{"duplicate case-variant resource parameter", func(e *ThirdpartyOAuth2ProviderEntity) {
			e.AuthorizationParams = map[string]string{"resource": "https://api.example.test/data", "rEsOuRcE": "https://api.example.test/data"}
		}, "authorization_params"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := discoveryRequestEntity()
			tc.change(e)
			require.ErrorContains(t, e.ValidateDiscoveryRequest(), tc.wantErr)
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_ValidateDiscoveryRequest_ExplicitAudience(t *testing.T) {
	t.Parallel()
	for _, audience := range []string{"https://api.example.test/data", "urn:example:audience:42"} {
		t.Run(audience, func(t *testing.T) {
			t.Parallel()
			e := discoveryRequestEntity()
			e.AuthorizationParams = map[string]string{"resource": audience}
			require.NoError(t, e.ValidateDiscoveryRequest())
		})
	}
}

func TestThirdpartyOAuth2ProviderEntity_StoredDiscoveryRejectsCaseVariantResourceParameter(t *testing.T) {
	t.Parallel()
	for _, resourceKey := range []string{"Resource", "rEsOuRcE"} {
		t.Run(resourceKey, func(t *testing.T) {
			t.Parallel()
			entity := discoveryEntity()
			clientID, err := CIMDClientID("https://broker.example.test", entity.ID)
			require.NoError(t, err)
			entity.ClientID = clientID
			committedAt := time.Now().UTC()
			entity.DiscoveryStatus = DiscoveryStatus{LastAttemptAt: &committedAt, LastSuccessAt: &committedAt}
			entity.AuthorizationParams[resourceKey] = "https://other.example.test/audience"
			require.ErrorContains(t, entity.Validate(), "authorization_params")
			require.ErrorContains(t, entity.validateStoredDiscoveryState(), "authorization_params")
		})
	}
}
