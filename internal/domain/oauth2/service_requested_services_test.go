package oauth2

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

const (
	requestedPublicURL = "https://broker.example.com"
	requestedGitHub    = requestedPublicURL + "/sessions/github"
	requestedCallback  = "https://client.example.com/cb"
	notPermitted       = "requested service scope is not permitted"
)

var (
	requestedGitHubID  = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	requestedReadSet   = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440001")
	requestedTriageSet = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440003")
	requestedPrincipal = id.Principal("alice@example.com")
)

type requestedServices struct {
	services map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity
	calls    int
}

func (r *requestedServices) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.calls++
	if service, ok := r.services[serviceID]; ok {
		return service, nil
	}
	return nil, ports.ErrNotFound
}

func (r *requestedServices) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.calls++
	for _, service := range r.services {
		if service.CanonicalID != nil && *service.CanonicalID == canonicalID {
			return service, nil
		}
	}
	return nil, ports.ErrNotFound
}

type requestedPermissionSets struct{}

func (requestedPermissionSets) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	sets := map[id.PermissionSetID]*storage.PermissionSet{
		requestedReadSet:   {ID: requestedReadSet, ServiceScopes: []storage.ServiceScope{{ServiceID: requestedGitHubID, Scopes: []string{"repo", "user:email"}}}},
		requestedTriageSet: {ID: requestedTriageSet, ServiceScopes: []storage.ServiceScope{{ServiceID: requestedGitHubID, Scopes: []string{"repo"}}}},
	}
	var result []*storage.PermissionSet
	for _, setID := range ids {
		if set, ok := sets[setID]; ok {
			result = append(result, set)
		}
	}
	return result, nil
}

type requestedSnapshots struct {
	snapshot servicescope.SessionSnapshot
}

func (r *requestedSnapshots) ServiceSessionSnapshot(context.Context, id.Principal, id.ServiceID) servicescope.SessionSnapshot {
	return r.snapshot
}

type requestedFixture struct {
	svc       *AuthorizationService
	agent     *storage.Agent
	grants    *MockGrantRepository
	services  *requestedServices
	snapshots *requestedSnapshots
	tokens    ports.SessionTokenValidator
}

func newRequestedFixture(t *testing.T, cfg *OAuth2Config, covered bool) *requestedFixture {
	t.Helper()
	agents := NewMockAgentRepository()
	agent := &storage.Agent{
		ID:           id.NewAgentID(),
		DisplayName:  "Data Analysis Assistant",
		RedirectURIs: []string{requestedCallback},
		PermissionSets: []storage.AgentPermissionSetEntry{
			{PermissionSetID: requestedReadSet, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: requestedTriageSet, RequirementType: storage.RequirementTypeOptional},
		},
		ServiceRequirements: []storage.ServiceRequirement{{ServiceID: requestedGitHubID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"repo", "user:email"}}},
	}
	require.NoError(t, agents.Create(context.Background(), agent))
	grants := NewMockGrantRepository()
	grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: requestedPrincipal, AgentID: agent.ID}
	if covered {
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: requestedReadSet, IncludedServiceIDs: []id.ServiceID{requestedGitHubID}}}
	} else {
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: requestedTriageSet}}
	}
	require.NoError(t, grants.Create(context.Background(), grant))
	if cfg == nil {
		cfg = &OAuth2Config{PublicURL: requestedPublicURL, ModeStrategy: NewLocalModeStrategy()}
	}
	cfg.PublicURL = requestedPublicURL
	tokens := newTestSessionTokenService()
	services := &requestedServices{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{
		requestedGitHubID: {ID: requestedGitHubID, CanonicalID: ptr.To("github")},
	}}
	snapshots := &requestedSnapshots{snapshot: servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo", "user:email"}}}
	svc := NewAuthorizationService(grants, newConnectedSessionRepository(), NewAgentClientResolver(agents, nil), cfg, nil, tokens).
		WithRequestedServices(servicescope.NewResolver(services, requestedPermissionSets{}, requestedPublicURL), servicescope.NewEvaluator(requestedPermissionSets{}, snapshots))
	return &requestedFixture{svc: svc, agent: agent, grants: grants, services: services, snapshots: snapshots, tokens: tokens}
}

func (f *requestedFixture) request(scope string) *ports.AuthorizationRequest {
	query := url.Values{"client_id": {f.agent.ID.String()}, "redirect_uri": {requestedCallback}, "response_type": {"code"}, "state": {"client-state"}, "scope": {scope}}
	return &ports.AuthorizationRequest{
		ClientID: id.ClientID(f.agent.ID.String()), RedirectURI: requestedCallback, ResponseType: "code",
		Scope: scope, State: "client-state", OriginalURL: "/oauth2/authorize?" + query.Encode(),
	}
}

func (f *requestedFixture) continuation(req *ports.AuthorizationRequest, sessionToken string) *ports.AuthorizationRequest {
	resumed := *req
	resumed.OriginalURL = req.OriginalURL + "&session_token=" + url.QueryEscape(sessionToken)
	resumed.SessionToken = sessionToken
	return &resumed
}

func (f *requestedFixture) authorize(t *testing.T, req *ports.AuthorizationRequest) *ports.AuthorizationDecision {
	t.Helper()
	decision, err := f.svc.HandleAuthorization(context.Background(), req, requestedPrincipal)
	require.NoError(t, err)
	return decision
}

func requireAuthorizationError(t *testing.T, decision *ports.AuthorizationDecision, code, description string) {
	t.Helper()
	require.Equal(t, "error", decision.Action)
	assert.Equal(t, code, decision.ErrorCode)
	redirect, err := url.Parse(decision.RedirectURL)
	require.NoError(t, err)
	assert.Equal(t, code, redirect.Query().Get("error"))
	assert.Equal(t, description, redirect.Query().Get("error_description"))
	assert.Equal(t, "client-state", redirect.Query().Get("state"))
	assert.Empty(t, redirect.Query().Get("code"))
}

func consentToken(t *testing.T, decision *ports.AuthorizationDecision) string {
	t.Helper()
	require.Equal(t, "redirect_to_consent", decision.Action)
	consent, err := url.Parse(decision.RedirectURL)
	require.NoError(t, err)
	return consent.Query().Get("session_token")
}

// Authorization with requested services from specs/055-incremental-service-authorization
// (FR-002–FR-003, FR-007–FR-008, FR-019; SR-001–SR-003).
func TestHandleAuthorizationRequestedServices(t *testing.T) {
	t.Run("requests without virtual scopes perform no service lookup", func(t *testing.T) {
		f := newRequestedFixture(t, nil, true)
		decision := f.authorize(t, f.request("openid offline_access https://other.example.com/sessions/github"))
		assert.Equal(t, "proceed", decision.Action)
		assert.Zero(t, f.services.calls)
		assert.Empty(t, decision.ServiceScopeBindings)
	})

	t.Run("malformed, unknown and ineligible scopes are not permitted", func(t *testing.T) {
		f := newRequestedFixture(t, nil, true)
		f.services.services[id.NewServiceID()] = &model.ThirdpartyOAuth2ProviderEntity{CanonicalID: ptr.To("salesforce")}
		for _, scope := range []string{requestedGitHub + "/", requestedPublicURL + "/sessions/unknown", requestedPublicURL + "/sessions/salesforce"} {
			requireAuthorizationError(t, f.authorize(t, f.request("openid "+scope)), "invalid_scope", notPermitted)
		}
	})

	t.Run("virtual scopes are exempt from the ordinary allow-list", func(t *testing.T) {
		f := newRequestedFixture(t, nil, true)
		f.agent.AllowedScopes = []string{"openid"}
		decision := f.authorize(t, f.request("openid "+requestedGitHub))
		assert.Equal(t, "proceed", decision.Action)
		assert.Equal(t, map[string]id.ServiceID{requestedGitHub: requestedGitHubID}, decision.ServiceScopeBindings)
		requireAuthorizationError(t, f.authorize(t, f.request("openid profile "+requestedGitHub)), "invalid_scope", "requested scope is not permitted")
	})

	t.Run("an uncovered requested service redirects to consent with sealed services and bindings", func(t *testing.T) {
		f := newRequestedFixture(t, nil, false)
		token := consentToken(t, f.authorize(t, f.request("openid "+requestedGitHub)))
		session, err := f.tokens.ValidateAuthorizationSessionToken(token, f.agent.ID, requestedPrincipal)
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{requestedGitHubID}, session.RequestedServiceIDs)
		assert.Equal(t, map[string]id.ServiceID{requestedGitHub: requestedGitHubID}, session.ServiceScopeBindings)
	})

	t.Run("an unusable or missing session redirects to consent", func(t *testing.T) {
		for _, state := range []servicescope.Usability{servicescope.UsabilityMissing, servicescope.UsabilityUnusableDefinitive} {
			f := newRequestedFixture(t, nil, true)
			f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: state}
			consentToken(t, f.authorize(t, f.request("openid "+requestedGitHub)))
		}
	})

	t.Run("unverifiable usability is temporarily unavailable", func(t *testing.T) {
		f := newRequestedFixture(t, nil, true)
		f.snapshots.snapshot = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUnverifiableTransient}
		requireAuthorizationError(t, f.authorize(t, f.request("openid "+requestedGitHub)), "temporarily_unavailable", "requested service usability cannot be verified; retry later")
	})

	t.Run("protected re-entry uses sealed bindings and rejects reassignment after Save", func(t *testing.T) {
		f := newRequestedFixture(t, nil, false)
		req := f.request("openid " + requestedGitHub)
		token := consentToken(t, f.authorize(t, req))
		grant, err := f.grants.FindByPrincipalAndAgent(context.Background(), requestedPrincipal, f.agent.ID)
		require.NoError(t, err)
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: requestedReadSet, IncludedServiceIDs: []id.ServiceID{requestedGitHubID}}}

		decision := f.authorize(t, f.continuation(req, token))
		assert.Equal(t, "proceed", decision.Action)
		assert.Equal(t, map[string]id.ServiceID{requestedGitHub: requestedGitHubID}, decision.ServiceScopeBindings)

		reused := id.NewServiceID()
		f.services.services[requestedGitHubID].CanonicalID = ptr.To("github-production")
		f.services.services[reused] = &model.ThirdpartyOAuth2ProviderEntity{ID: reused, CanonicalID: ptr.To("github")}
		f.agent.ServiceRequirements = append(f.agent.ServiceRequirements, storage.ServiceRequirement{ServiceID: reused, RequirementType: storage.RequirementTypeOptional})
		requireAuthorizationError(t, f.authorize(t, f.continuation(req, token)), "invalid_scope", notPermitted)
	})

	t.Run("protected re-entry rejects tampered requests and foreign or invalid tokens", func(t *testing.T) {
		f := newRequestedFixture(t, nil, false)
		req := f.request("openid " + requestedGitHub)
		token := consentToken(t, f.authorize(t, req))
		tampered := f.continuation(f.request("openid profile "+requestedGitHub), token)
		requireAuthorizationError(t, f.authorize(t, tampered), "access_denied", "authorization session is invalid or expired")
		requireAuthorizationError(t, f.authorize(t, f.continuation(req, token[:len(token)-2]+"xx")), "access_denied", "authorization session is invalid or expired")

		decision, err := f.svc.HandleAuthorization(context.Background(), f.continuation(req, token), id.Principal("mallory@example.com"))
		require.NoError(t, err)
		assert.Equal(t, "error", decision.Action)
	})

	t.Run("consent re-entry keeps the same session token and expiry", func(t *testing.T) {
		f := newRequestedFixture(t, nil, false)
		req := f.request("openid " + requestedGitHub)
		token := consentToken(t, f.authorize(t, req))
		assert.Equal(t, token, consentToken(t, f.authorize(t, f.continuation(req, token))))
	})

	t.Run("proxy agents are redirected upstream without virtual scopes", func(t *testing.T) {
		f := newRequestedFixture(t, &OAuth2Config{UpstreamAuthorizeEndpoint: "https://idp.example.com/authorize", ModeStrategy: NewProxyModeStrategy()}, true)
		f.agent.ClientID = ptr.To(id.ClientID("upstream-client"))
		decision := f.authorize(t, f.request("openid "+requestedGitHub+" profile"))
		require.Equal(t, "proceed", decision.Action)
		upstream, err := url.Parse(decision.RedirectURL)
		require.NoError(t, err)
		assert.Equal(t, "openid profile", upstream.Query().Get("scope"))
		assert.NotContains(t, decision.RedirectURL, "session_token")

		decision = f.authorize(t, f.request(requestedGitHub))
		upstream, err = url.Parse(decision.RedirectURL)
		require.NoError(t, err)
		assert.False(t, upstream.Query().Has("scope"), "an empty upstream scope is omitted")
	})

	t.Run("repository failures are server errors", func(t *testing.T) {
		f := newRequestedFixture(t, nil, true)
		f.svc.requestedServices = servicescope.NewResolver(failingServices{}, requestedPermissionSets{}, requestedPublicURL)
		requireAuthorizationError(t, f.authorize(t, f.request("openid "+requestedGitHub)), "server_error", "server error")
	})
}

type failingServices struct{}

func (failingServices) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, errors.New("storage unavailable")
}

func (failingServices) GetByCanonicalID(context.Context, string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, errors.New("storage unavailable")
}

// Discovery non-advertisement from specs/055-incremental-service-authorization (FR-014).
func TestGenerateMetadataNeverAdvertisesServiceScopes(t *testing.T) {
	f := newRequestedFixture(t, &OAuth2Config{
		ModeStrategy:    NewLocalModeStrategy(),
		SupportedScopes: []string{"openid", requestedGitHub, requestedGitHub + "/", "https://other.example.com/sessions/github"},
	}, true)
	metadata, err := f.svc.GenerateMetadata(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"openid", "https://other.example.com/sessions/github"}, metadata.ScopesSupported)
}
