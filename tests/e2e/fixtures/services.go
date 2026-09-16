package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// GitHubService returns a fixture for GitHub OAuth2 service.
// This is used for token exchange testing - represents GitHub as a third-party service.
// Includes protected_resources for resource-based service lookup (US2).
func GitHubService() *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	metadataURL := "https://github.com/.well-known/oauth-authorization-server"

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("a0000000-0000-0000-0000-000000000001"),
		DisplayName: "GitHub",
		ClientID:    id.ClientID("github-client-id"),
		Secret:      EncryptedSecret("a0000000-0000-0000-0000-000000000001", "github-client-secret"),
		Flavor:      model.OAuth2FlavorGitHub,
		IssuerURI:   "https://github.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
			MetadataURL:     &metadataURL,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://github.com/login/oauth/access_token",
			AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Access repository"},
			{ScopeValue: "user", Description: "Access user information"},
			{ScopeValue: "read:org", Description: "Read organization information"},
		},
		ProtectedResources: []string{"https://api.github.com"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// GoogleService returns a fixture for Google OAuth2 service.
// This is used for token exchange testing - represents Google as a third-party service.
func GoogleService() *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	metadataURL := "https://accounts.google.com/.well-known/openid-configuration"

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("a0000000-0000-0000-0000-000000000002"),
		DisplayName: "Google",
		ClientID:    id.ClientID("google-client-id"),
		Secret:      EncryptedSecret("a0000000-0000-0000-0000-000000000002", "google-client-secret"),
		IssuerURI:   "https://accounts.google.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
			MetadataURL:     &metadataURL,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://oauth2.googleapis.com/token",
			AuthorizeEndpoint: "https://accounts.google.com/o/oauth2/v2/auth",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "calendar", Description: "Access calendar"},
			{ScopeValue: "drive", Description: "Access Google Drive"},
			{ScopeValue: "userinfo.email", Description: "Access email"},
		},
		ProtectedResources: []string{"https://www.googleapis.com"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// MicrosoftService returns a fixture for Microsoft Azure OAuth2 service.
// This is used for token exchange testing - represents Microsoft as a third-party service.
func MicrosoftService() *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	metadataURL := "https://login.microsoftonline.com/common/v2.0/.well-known/openid-configuration"

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID("a0000000-0000-0000-0000-000000000003"),
		DisplayName: "Microsoft Azure",
		ClientID:    id.ClientID("microsoft-client-id"),
		Secret:      EncryptedSecret("a0000000-0000-0000-0000-000000000003", "microsoft-client-secret"),
		IssuerURI:   "https://login.microsoftonline.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
			MetadataURL:     &metadataURL,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			AuthorizeEndpoint: "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "mail.read", Description: "Read mail"},
			{ScopeValue: "calendar.read", Description: "Read calendar"},
			{ScopeValue: "user.read", Description: "Read user profile"},
		},
		ProtectedResources: []string{"https://graph.microsoft.com"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// ServiceWithID returns a service with the specified ID.
// Base service can be customized for testing specific scenarios.
func ServiceWithID(svcID string) *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.MustParseServiceID(svcID),
		DisplayName: "Test Service " + svcID,
		ClientID:    id.ClientID("test-client-" + svcID),
		Secret:      EncryptedSecret(svcID, "test-secret-"+svcID),
		IssuerURI:   "https://test-issuer.example.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://test-issuer.example.com/token",
			AuthorizeEndpoint: "https://test-issuer.example.com/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
			{ScopeValue: "write", Description: "Write access"},
		},
		ProtectedResources: []string{"https://test-issuer.example.com/api"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// PublicClientService returns a deterministic public-client OAuth2 service fixture.
// Each call creates a fresh service ID for test isolation.
func PublicClientService() *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      id.NewServiceID(),
		DisplayName:             "Public Test Service",
		ClientID:                id.ClientID("public-client-id"),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		Flavor:                  model.OAuth2FlavorStandard,
		IssuerURI:               "https://public-issuer.example.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://public-issuer.example.com/token",
			AuthorizeEndpoint: "https://public-issuer.example.com/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
		},
		ProtectedResources: []string{"https://public-issuer.example.com/api"},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// GoogleServiceAccountFixtureJSON returns a realistic but non-functional Google service
// account JSON document as a string. Used for E2E testing of the google OAuth2 flavor.
// The private_key value is fake and will not pass cryptographic validation,
// but it passes structural validation since we do not verify crypto format (FR-014).
func GoogleServiceAccountFixtureJSON() string {
	return `{
  "type": "service_account",
  "project_id": "test-project-123",
  "private_key_id": "test-key-id-abcdef",
  "private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA2a2rwplBQLf0kTmkGp5RJFBpJOFBBhfJmLO0YjCGSLCuoP7\noc5RfakePrivateKeyDataForTestingPurposesOnlyNotRealCryptographicKey\n-----END RSA PRIVATE KEY-----\n",
  "client_email": "test-service@test-project-123.iam.gserviceaccount.com",
  "client_id": "112233445566778899001",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test-service%40test-project-123.iam.gserviceaccount.com"
}`
}

// ValidGoogleServiceRequest returns a complete ServiceRequest body for creating a
// google-flavor service. Used in E2E tests for google flavor scenarios.
func ValidGoogleServiceRequest() map[string]interface{} {
	return map[string]interface{}{
		"display_name":        "Google Test Service",
		"oauth2_flavor":       "google",
		"client_secret":       GoogleServiceAccountFixtureJSON(),
		"discovery":           map[string]interface{}{"enable_discovery": false},
		"scopes":              []map[string]interface{}{{"scope_value": "https://www.googleapis.com/auth/cloud-platform", "description": "Cloud Platform"}},
		"protected_resources": []string{"https://test.googleapis.com"},
	}
}

// ValidGitHubServiceRequest returns a complete ServiceRequest body for creating a
// github-flavor service. Used in E2E tests for github flavor scenarios.
func ValidGitHubServiceRequest() map[string]interface{} {
	return map[string]interface{}{
		"display_name":  "GitHub Test Service",
		"oauth2_flavor": "github",
		"client_id":     "Iv1.1234567890abcdef",
		"client_secret": "ghp_secretkey1234567890abcdef",
		"issuer_uri":    "https://github.com",
		"discovery":     map[string]interface{}{"enable_discovery": false},
		"endpoints": map[string]interface{}{
			"token_endpoint":     "https://github.com/login/oauth/access_token",
			"authorize_endpoint": "https://github.com/login/oauth/authorize",
		},
		"scopes":              []map[string]interface{}{{"scope_value": "repo", "description": "Repository access"}},
		"protected_resources": []string{"https://api.github.com"},
	}
}
