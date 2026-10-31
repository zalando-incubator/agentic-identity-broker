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

func (scopePermissionSets) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*dstorage.PermissionSet, error) {
	sets := make([]*dstorage.PermissionSet, len(ids))
	for i, setID := range ids {
		sets[i] = &dstorage.PermissionSet{ID: setID, ServiceScopes: []dstorage.ServiceScope{{ServiceID: scopeGitHubID, Scopes: []string{"repo"}}}}
	}
	return sets, nil
}

type scopeSnapshots struct {
	snapshot servicescope.SessionSnapshot
}

func (s *scopeSnapshots) ServiceSessionSnapshot(context.Context, id.Principal, id.ServiceID) servicescope.SessionSnapshot {
	return s.snapshot
}

type serviceScopeFixture struct {
	provider  *Provider
	agents    ports.AgentRepository
	agent     *dstorage.Agent
	secret    string
	services  *scopeServices
	snapshots *scopeSnapshots
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
	snapshots := &scopeSnapshots{snapshot: servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}}}
	provider.WithServiceScopes(servicescope.NewResolver(services, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, snapshots))
	grant, err := provider.fositeStorage.verifier.(*testConsentVerifier).grants.FindByPrincipalAndAgent(context.Background(), id.Principal("user@example.com"), agent.ID)
	require.NoError(t, err)
	grant.GrantedPermissionSets = []dstorage.GrantedPermissionSetEntry{{PermissionSetID: agent.PermissionSets[0].PermissionSetID, IncludedServiceIDs: []id.ServiceID{scopeGitHubID}}}
	require.NoError(t, provider.fositeStorage.verifier.(*testConsentVerifier).grants.Update(context.Background(), grant))
	return &serviceScopeFixture{provider: provider, agents: agentRepo, agent: agent, secret: secret, services: services, snapshots: snapshots, refreshes: provider.fositeStorage.refreshRepo}
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

func (f *serviceScopeFixture) refresh(token, scope string) (*ports.TokenResponse, error) {
	return f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), ports.ClientCredentials{Secret: f.secret}, token, scope)
}

func requireRFC6749(t *testing.T, err error, code, description string) {
	t.Helper()
	var rfcErr *RFC6749Error
	require.ErrorAs(t, err, &rfcErr)
	assert.Equal(t, code, rfcErr.Code())
	assert.Equal(t, description, rfcErr.Description())
}

// Refresh widening and narrowing from specs/055-incremental-service-authorization (FR-012, US1 S2, API-LOCAL-SCOPES).
func TestProviderRefreshServiceScopeWidening(t *testing.T) {
	for _, allowList := range [][]string{{"read"}, nil} {
		t.Run(strings.Join(allowList, ",")+" allow-list", func(t *testing.T) {
			f := newServiceScopeFixture(t)
			f.agent.AllowedScopes = allowList
			require.NoError(t, f.agents.Update(context.Background(), f.agent))
			code, err := f.authorize(t, "read offline_access", nil)
			require.NoError(t, err)
			issued, err := f.exchange(code)
			require.NoError(t, err)

			_, err = f.refresh(issued.RefreshToken, "read offline_access "+githubScope)
			requireRFC6749(t, err, "invalid_scope", "scope "+githubScope+" requires interactive authorization")

			refreshed, err := f.refresh(issued.RefreshToken, "")
			require.NoError(t, err, "the refused widening must not consume the refresh token")
			assert.Equal(t, "read offline_access", refreshed.Scope)
		})
	}

	t.Run("narrowing persists and a scope-less successor cannot restore removed scopes", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		code, err := f.authorize(t, "read offline_access "+githubScope, map[string]id.ServiceID{githubScope: scopeGitHubID})
		require.NoError(t, err)
		issued, err := f.exchange(code)
		require.NoError(t, err)

		narrowed, err := f.refresh(issued.RefreshToken, "offline_access read")
		require.NoError(t, err)
		assert.Equal(t, "offline_access read", narrowed.Scope)
		assert.Equal(t, "offline_access read", decodeJWTPayload(t, narrowed.AccessToken)["scope"])
		session, err := f.refreshes.FindBySignature(context.Background(), f.provider.refreshStrategy.RefreshTokenSignature(context.Background(), narrowed.RefreshToken))
		require.NoError(t, err)
		assert.Equal(t, "offline_access read", session.Scope)
		assert.Equal(t, dstorage.ServiceScopeBindings{githubScope: scopeGitHubID}, session.ServiceScopeBindings, "narrowing retains assurance bindings")

		_, err = f.refresh(narrowed.RefreshToken, "read offline_access "+githubScope)
		requireRFC6749(t, err, "invalid_scope", "scope "+githubScope+" requires interactive authorization")
		_, err = f.refresh(narrowed.RefreshToken, "READ offline_access")
		requireRFC6749(t, err, "invalid_scope", "scope READ requires interactive authorization")
		_, err = f.refresh(narrowed.RefreshToken, "read write offline_access")
		requireRFC6749(t, err, "invalid_scope", "scope write requires interactive authorization")

		scopeless, err := f.refresh(narrowed.RefreshToken, "")
		require.NoError(t, err)
		assert.Equal(t, "offline_access read", scopeless.Scope)
	})
}

// Continuing local refresh assurance from specs/055-incremental-service-authorization
// (FR-008, FR-012, FR-019–FR-020; SR-003; API-LOCAL-SCOPES).
func TestProviderRefreshRechecksBoundServices(t *testing.T) {
	bindings := map[string]id.ServiceID{githubScope: scopeGitHubID}
	unsatisfied := "scope " + githubScope + " is no longer satisfied; interactive authorization required"
	issue := func(t *testing.T, f *serviceScopeFixture) string {
		t.Helper()
		code, err := f.authorize(t, "read offline_access "+githubScope, bindings)
		require.NoError(t, err)
		issued, err := f.exchange(code)
		require.NoError(t, err)
		return issued.RefreshToken
	}

	t.Run("usable services refresh and narrowing keeps rechecking every binding", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		token := issue(t, f)
		narrowed, err := f.refresh(token, "read offline_access")
		require.NoError(t, err)
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUnusableDefinitive}
		_, err = f.refresh(narrowed.RefreshToken, "")
		requireRFC6749(t, err, "invalid_scope", unsatisfied)
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}}
		again, err := f.refresh(narrowed.RefreshToken, "")
		require.NoError(t, err, "a refused refresh does not consume the token")
		assert.Equal(t, "read offline_access", again.Scope)
	})

	t.Run("a valid token missing an effective scope is unusable", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		token := issue(t, f)
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"read:org"}}
		_, err := f.refresh(token, "")
		requireRFC6749(t, err, "invalid_scope", unsatisfied)
	})

	t.Run("transient usability failures are non-consuming server errors", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		token := issue(t, f)
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUnverifiableTransient}
		_, err := f.refresh(token, "")
		requireRFC6749(t, err, "server_error", "requested service usability cannot be verified")
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}}
		_, err = f.refresh(token, "")
		require.NoError(t, err)
	})

	t.Run("grant coverage loss and canonical renames fail closed without consumption", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		token := issue(t, f)
		f.services.services[scopeGitHubID].CanonicalID = ptr.To("github-production")
		_, err := f.refresh(token, "")
		requireRFC6749(t, err, "invalid_scope", unsatisfied)
		f.services.services[scopeGitHubID].CanonicalID = ptr.To("github")

		grants := f.provider.fositeStorage.verifier.(*testConsentVerifier).grants
		grant, err := grants.FindByPrincipalAndAgent(context.Background(), id.Principal("user@example.com"), f.agent.ID)
		require.NoError(t, err)
		grant.GrantedPermissionSets[0].IncludedServiceIDs = nil
		require.NoError(t, grants.Update(context.Background(), grant))
		_, err = f.refresh(token, "")
		requireRFC6749(t, err, "invalid_scope", unsatisfied)
	})

	t.Run("client_credentials cannot request service scopes", func(t *testing.T) {
		f := newServiceScopeFixture(t)
		_, err := f.provider.HandleClientCredentials(context.Background(), f.agent.ID.String(), f.secret, "read "+githubScope)
		requireRFC6749(t, err, "invalid_scope", "service scopes require a user authorization")
	})
}
