package oauth2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateMetadata_ProxyMode(t *testing.T) {
	svc := NewAuthorizationService(nil, NewMockSessionRepository(), nil, &OAuth2Config{
		ModeStrategy:           NewProxyModeStrategy(),
		PublicURL:              "https://broker.example.com",
		SupportedResponseTypes: []string{"code"},
		SupportedGrantTypes:    []string{"authorization_code", "refresh_token"},
		SupportedScopes:        nil,
	}, nil, newTestSessionTokenService(), testAuthorizationClock{now: time.Now()})

	metadata, err := svc.GenerateMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "https://broker.example.com", metadata.Issuer)
	assert.Equal(t, "https://broker.example.com/oauth2/authorize", metadata.AuthorizationEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/token", metadata.TokenEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/jwks.json", metadata.JWKSURI, "proxy mode must advertise JWKS URI (aggregated key surface)")
	assert.Empty(t, metadata.CodeChallengeMethodsSupported, "code challenge methods should be empty in proxy mode")
	assert.ElementsMatch(t, []string{"client_secret_post", "client_secret_basic"}, metadata.TokenEndpointAuthMethodsSupported,
		"proxy mode must advertise both client_secret_post and client_secret_basic")
	assert.Empty(t, metadata.ScopesSupported)
}

func TestGenerateMetadata_LocalMode(t *testing.T) {
	svc := NewAuthorizationService(nil, NewMockSessionRepository(), nil, &OAuth2Config{
		ModeStrategy:           NewLocalModeStrategy(),
		PublicURL:              "https://broker.example.com",
		SupportedResponseTypes: []string{"code"},
		SupportedGrantTypes:    []string{"authorization_code", "client_credentials", "refresh_token"},
		SupportedScopes:        []string{"offline_access"},
	}, nil, newTestSessionTokenService(), testAuthorizationClock{now: time.Now()})

	metadata, err := svc.GenerateMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "https://broker.example.com", metadata.Issuer)
	assert.Equal(t, "https://broker.example.com/oauth2/authorize", metadata.AuthorizationEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/token", metadata.TokenEndpoint)
	assert.Equal(t, "https://broker.example.com/oauth2/jwks.json", metadata.JWKSURI)
	assert.Equal(t, []string{"S256"}, metadata.CodeChallengeMethodsSupported)
	assert.Equal(t, []string{"client_secret_post"}, metadata.TokenEndpointAuthMethodsSupported)
	assert.Equal(t, []string{"offline_access"}, metadata.ScopesSupported)
}

// T045b: hybrid mode metadata reflects union of proxy + local capabilities.
func TestGenerateMetadata_HybridMode(t *testing.T) {
	svc := NewAuthorizationService(nil, NewMockSessionRepository(), nil, &OAuth2Config{
		ModeStrategy:           NewHybridModeStrategy(),
		PublicURL:              "https://broker.example.com",
		SupportedResponseTypes: []string{"code"},
		SupportedGrantTypes:    []string{"authorization_code", "client_credentials", "refresh_token"},
		SupportedScopes:        []string{"offline_access"},
	}, nil, newTestSessionTokenService(), testAuthorizationClock{now: time.Now()})

	metadata, err := svc.GenerateMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "https://broker.example.com", metadata.Issuer)
	assert.Equal(t, "https://broker.example.com/oauth2/jwks.json", metadata.JWKSURI, "hybrid mode must include JWKS URI")
	assert.Equal(t, []string{"S256"}, metadata.CodeChallengeMethodsSupported, "hybrid mode must include PKCE methods")
	assert.Contains(t, metadata.TokenEndpointAuthMethodsSupported, "client_secret_post")
	assert.Equal(t, []string{"offline_access"}, metadata.ScopesSupported)
}

func TestGenerateMetadata_LocalModeWithCIMD(t *testing.T) {
	svc := NewAuthorizationService(nil, NewMockSessionRepository(), nil, &OAuth2Config{
		ModeStrategy:           NewLocalModeStrategy(),
		PublicURL:              "https://broker.example.com",
		SupportedResponseTypes: []string{"code"},
		SupportedGrantTypes:    []string{"authorization_code", "client_credentials", "refresh_token"},
		SupportedScopes:        []string{"offline_access"},
		CIMDEnabled:            true,
	}, nil, newTestSessionTokenService(), testAuthorizationClock{now: time.Now()})

	metadata, err := svc.GenerateMetadata(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"none", "client_secret_post"}, metadata.TokenEndpointAuthMethodsSupported)
	cimdEnabled := metadata.ClientIDMetadataDocumentSupported
	require.NotNil(t, cimdEnabled)
	assert.True(t, *cimdEnabled)
}
