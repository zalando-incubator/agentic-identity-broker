package oauth2server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

const (
	scopePublicURL = "https://broker.example.com"
	githubScope    = scopePublicURL + "/sessions/github"
	scopeCallback  = "http://localhost:8080/callback"
	scopeVerifier  = "service-scope-verifier-1234567890123456789012"
)

var scopeGitHubID = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")

type scopeServices struct {
	services map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity
}

func (s *scopeServices) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if service, ok := s.services[serviceID]; ok {
		return service, nil
	}
	return nil, ports.ErrNotFound
}

func (s *scopeServices) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	for _, service := range s.services {
		if service.CanonicalID != nil && *service.CanonicalID == canonicalID {
			return service, nil
		}
	}
	return nil, ports.ErrNotFound
}

type scopePermissionSets struct{}

func (scopePermissionSets) GetByIDs(context.Context, []id.PermissionSetID) ([]*dstorage.PermissionSet, error) {
	return nil, nil
}

type serviceScopeFixture struct {
	provider  *Provider
	agent     *dstorage.Agent
	secret    string
	services  *scopeServices
	refreshes ports.RefreshTokenSessionRepository
}

func newServiceScopeFixture(t *testing.T) *serviceScopeFixture {
	t.Helper()
	provider, agentRepo, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agentRepo)
	agent.RedirectURIs = []string{scopeCallback}
	agent.AllowedScopes = []string{"read"}
	agent.ServiceRequirements = []dstorage.ServiceRequirement{{ServiceID: scopeGitHubID, RequirementType: dstorage.RequirementTypeOptional, RequiredScopes: []string{"repo"}}}
	require.NoError(t, agentRepo.Update(context.Background(), agent))
	services := &scopeServices{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{
		scopeGitHubID: {ID: scopeGitHubID, CanonicalID: ptr.To("github")},
	}}
	provider.WithServiceScopes(servicescope.NewResolver(services, scopePermissionSets{}, scopePublicURL))
	return &serviceScopeFixture{provider: provider, agent: agent, secret: secret, services: services, refreshes: provider.fositeStorage.refreshRepo}
}

func (f *serviceScopeFixture) authorize(t *testing.T, scope string, bindings map[string]id.ServiceID) (string, error) {
	t.Helper()
	return f.provider.HandleBoundAuthorize(context.Background(), f.agent.ID.String(), scopeCallback, "code", scope, "state",
		generateS256Challenge(scopeVerifier), "S256", id.NewPrincipal("user@example.com"), bindings)
}

func (f *serviceScopeFixture) exchange(code string) (*ports.TokenResponse, error) {
	return f.provider.HandleAuthorizationCodeExchange(context.Background(), f.agent.ID.String(), ports.ClientCredentials{Secret: f.secret}, code, scopeCallback, scopeVerifier)
}

// Local issuance from specs/055-incremental-service-authorization (FR-002, FR-019–FR-020, API-LOCAL-SCOPES).
func TestProviderVirtualServiceScopeIssuance(t *testing.T) {
	bindings := map[string]id.ServiceID{githubScope: scopeGitHubID}

	t.Run("virtual scopes bypass the allow-list and keep their exact form through code exchange", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		code, err := f.authorize(t, "read offline_access "+githubScope, bindings)
		require.NoError(t, err)
		resp, err := f.exchange(code)
		require.NoError(t, err)
		assert.Equal(t, "read offline_access "+githubScope, resp.Scope)
		assert.Contains(t, strings.Fields(decodeJWTPayload(t, resp.AccessToken)["scope"].(string)), githubScope)

		session, err := f.refreshes.FindBySignature(context.Background(), f.provider.refreshStrategy.RefreshTokenSignature(context.Background(), resp.RefreshToken))
		require.NoError(t, err)
		assert.Equal(t, dstorage.ServiceScopeBindings(bindings), session.ServiceScopeBindings)
	})

	t.Run("ordinary scopes keep allow-list enforcement", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		_, err := f.authorize(t, "write "+githubScope, bindings)
		assert.ErrorIs(t, err, ErrInvalidScope)
	})

	t.Run("an unbound virtual scope fails closed", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		_, err := f.authorize(t, "read "+githubScope, nil)
		assert.ErrorIs(t, err, ErrInvalidScope)
		_, err = f.authorize(t, "read "+githubScope+"/", nil)
		assert.ErrorIs(t, err, ErrInvalidScope)
	})

	t.Run("code exchange rejects a canonical ID renamed or reused after authorization", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		code, err := f.authorize(t, "read offline_access "+githubScope, bindings)
		require.NoError(t, err)
		reused := id.NewServiceID()
		f.services.services[scopeGitHubID].CanonicalID = ptr.To("github-production")
		f.services.services[reused] = &model.ThirdpartyOAuth2ProviderEntity{ID: reused, CanonicalID: ptr.To("github")}
		_, err = f.exchange(code)
		assert.ErrorIs(t, err, ErrInvalidScope)
	})

	t.Run("PKCE and code replay protections still apply", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		code, err := f.authorize(t, "read "+githubScope, bindings)
		require.NoError(t, err)
		_, err = f.provider.HandleAuthorizationCodeExchange(context.Background(), f.agent.ID.String(), ports.ClientCredentials{Secret: f.secret}, code, scopeCallback, "wrong-"+scopeVerifier)
		assert.Error(t, err)
		code, err = f.authorize(t, "read "+githubScope, bindings)
		require.NoError(t, err)
		_, err = f.exchange(code)
		require.NoError(t, err)
		_, err = f.exchange(code)
		assert.ErrorIs(t, err, ErrInvalidGrant)
	})
}

func decodeJWTPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims
}
