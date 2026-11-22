package impersonation

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const scopePublicURL = "https://broker.example.com"

var (
	scopeGithubID     = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	scopeJiraID       = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440020")
	scopeSalesforceID = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440030")
	scopeGithubSetID  = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440001")
	scopeJiraSetID    = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440004")
)

type scopeServices struct {
	err   error
	calls int
}

func (s *scopeServices) services() map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity {
	services := map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}
	for serviceID, canonicalID := range map[id.ServiceID]string{scopeGithubID: "github", scopeJiraID: "jira", scopeSalesforceID: "salesforce"} {
		services[serviceID] = &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: &canonicalID, DisplayName: canonicalID}
	}
	return services
}

func (s *scopeServices) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if service, ok := s.services()[serviceID]; ok {
		return service, nil
	}
	return nil, ports.ErrNotFound
}

func (s *scopeServices) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	for _, service := range s.services() {
		if *service.CanonicalID == canonicalID {
			return service, nil
		}
	}
	return nil, ports.ErrNotFound
}

type scopePermissionSets struct{ err error }

func (s scopePermissionSets) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	if s.err != nil {
		return nil, s.err
	}
	sets := map[id.PermissionSetID]*storage.PermissionSet{
		scopeGithubSetID: {ID: scopeGithubSetID, ServiceScopes: []storage.ServiceScope{{ServiceID: scopeGithubID, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional}}},
		scopeJiraSetID:   {ID: scopeJiraSetID, ServiceScopes: []storage.ServiceScope{{ServiceID: scopeJiraID, Scopes: []string{"read:jira-work"}, RequirementType: storage.RequirementTypeOptional}}},
	}
	var result []*storage.PermissionSet
	for _, setID := range ids {
		if set, ok := sets[setID]; ok {
			result = append(result, set)
		}
	}
	return result, nil
}

type scopeSessions map[id.ServiceID]servicescope.SessionSnapshot

func (s scopeSessions) ServiceSessionSnapshot(_ context.Context, _ id.Principal, serviceID id.ServiceID) servicescope.SessionSnapshot {
	if snapshot, ok := s[serviceID]; ok {
		return snapshot
	}
	return servicescope.SessionSnapshot{Usability: servicescope.UsabilityMissing}
}

// scopeTarget declares GitHub and Jira, but not Salesforce, and allow-lists only "read".
func scopeTarget() *Target {
	target := testTarget()
	target.Agent.AllowedScopes = []string{"read"}
	target.Agent.PermissionSets = []storage.AgentPermissionSetEntry{
		{PermissionSetID: scopeGithubSetID, RequirementType: storage.RequirementTypeOptional},
		{PermissionSetID: scopeJiraSetID, RequirementType: storage.RequirementTypeOptional},
	}
	target.Agent.ServiceRequirements = []storage.ServiceRequirement{
		{ServiceID: scopeGithubID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"repo"}},
		{ServiceID: scopeJiraID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"read:jira-work"}},
	}
	return target
}

// githubOnlySelection grants GitHub but not Jira.
func githubOnlySelection() []storage.GrantedPermissionSetEntry {
	return []storage.GrantedPermissionSetEntry{{PermissionSetID: scopeGithubSetID, IncludedServiceIDs: []id.ServiceID{scopeGithubID}}}
}

func usableGithub() scopeSessions {
	return scopeSessions{
		scopeGithubID: {Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}},
		scopeJiraID:   {Usability: servicescope.UsabilityUsable, Scopes: []string{"read:jira-work"}},
	}
}

type serviceScopeFixture struct {
	services       *scopeServices
	permissionSets scopePermissionSets
	sessions       scopeSessions
	categories     []string
	agentSubject   bool
	unconfigured   bool
}

type serviceScopeResult struct {
	outcome  *Outcome
	err      error
	issuer   *stubIssuer
	verifier *recordingDelegationVerifier
	target   *Target
}

func (f serviceScopeFixture) impersonate(t *testing.T, scope string) serviceScopeResult {
	t.Helper()
	if f.services == nil {
		f.services = &scopeServices{}
	}
	if f.sessions == nil {
		f.sessions = usableGithub()
	}
	signingKey, keySet := signedValidationKey(t, "service-scope")
	cfg := testImpersonationConfig()
	cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
	cfg.Rules[0].AllowedSubjectCategories = f.categories
	issuer := &stubIssuer{}
	verifier := &recordingDelegationVerifier{status: ports.UserDelegationActive, selection: githubOnlySelection()}
	svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return configurableJWKSProvider{set: keySet}, nil
	}, stubAgentRepository{}, issuer, 0, nil, verifier, scopePublicURL+"/")
	require.NoError(t, err)
	if !f.unconfigured {
		svc.WithServiceScopes(
			servicescope.NewResolver(f.services, f.permissionSets, scopePublicURL),
			servicescope.NewEvaluator(f.permissionSets, f.sessions),
		)
	}
	target := scopeTarget()
	subject := "alice@example.com"
	if f.agentSubject {
		subject = target.Agent.ID.String()
	}
	token := signedValidationTokenWithSubject(t, jwa.ES256(), signingKey, "https://idp.example.com", "aud", subject, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
	outcome, err := svc.Impersonate(context.Background(), &Request{
		ClientAssertion: token, ActorToken: token, SubjectToken: token, SubjectTokenType: JWTTokenType,
		Scope: scope, Scopes: oauth2.SplitScope(scope),
	}, target)
	return serviceScopeResult{outcome: outcome, err: err, issuer: issuer, verifier: verifier, target: target}
}

func requireRefused(t *testing.T, result serviceScopeResult, code string) *tokenexchange.TokenExchangeError {
	t.Helper()
	var tokenErr *tokenexchange.TokenExchangeError
	require.ErrorAs(t, result.err, &tokenErr)
	assert.Equal(t, code, tokenErr.Code())
	assert.Empty(t, tokenErr.ErrorURI())
	assert.Equal(t, tokenexchange.ExchangeImpersonation, tokenErr.Diagnostic().ExchangeKind())
	assert.False(t, result.issuer.called, "no token may be minted")
	require.NotNil(t, result.outcome)
	assert.Nil(t, result.outcome.Response)
	assert.Equal(t, code, result.outcome.Audit.OAuthErrorCode)
	assert.Equal(t, "rule-1", result.outcome.Audit.SelectedRule)
	return tokenErr
}

func TestImpersonate_VirtualServiceScopes(t *testing.T) {
	github := scopePublicURL + "/sessions/github"

	t.Run("grants a covered, usable service in its requested form without allow-listing it", func(t *testing.T) {
		for _, scope := range []string{github, scopePublicURL + "/sessions/" + scopeGithubID.String()} {
			result := serviceScopeFixture{}.impersonate(t, "read "+scope)
			require.NoError(t, result.err)
			assert.Equal(t, "read "+scope, result.outcome.Response.Scope)
			assert.Equal(t, []string{"read", scope}, result.issuer.input.Scopes)
			assert.Equal(t, id.Principal("alice@example.com"), result.verifier.principal)
		}
	})

	t.Run("ordinary scopes keep the allow-list and need no service lookup", func(t *testing.T) {
		services := &scopeServices{}
		result := serviceScopeFixture{services: services}.impersonate(t, "read")
		require.NoError(t, result.err)
		assert.Equal(t, "read", result.outcome.Response.Scope)
		assert.Zero(t, services.calls)
	})

	for _, tc := range []struct {
		name  string
		scope string
	}{
		{name: "malformed trailing slash", scope: github + "/"},
		{name: "malformed uppercase UUID", scope: scopePublicURL + "/sessions/880E8400-E29B-41D4-A716-446655440010"},
		{name: "malformed empty identifier", scope: scopePublicURL + "/sessions/"},
		{name: "unknown service", scope: scopePublicURL + "/sessions/unknown"},
		{name: "unknown service UUID", scope: scopePublicURL + "/sessions/880e8400-e29b-41d4-a716-446655449999"},
		{name: "service not eligible for the agent", scope: scopePublicURL + "/sessions/salesforce"},
		{name: "service not covered by the user's grant", scope: scopePublicURL + "/sessions/jira"},
		{name: "one invalid scope among valid ones", scope: "read " + github + " " + scopePublicURL + "/sessions/jira"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			result := serviceScopeFixture{}.impersonate(t, tc.scope)
			requireRefused(t, result, tokenexchange.InvalidScopeError)
			assert.Equal(t, 1, result.verifier.calls)
		})
	}

	t.Run("rejects a covered service whose session is not usable", func(t *testing.T) {
		for _, usability := range []servicescope.Usability{servicescope.UsabilityMissing, servicescope.UsabilityUnusableDefinitive} {
			result := serviceScopeFixture{sessions: scopeSessions{scopeGithubID: {Usability: usability}}}.impersonate(t, github)
			requireRefused(t, result, tokenexchange.InvalidScopeError)
		}
	})

	t.Run("rejects a usable session lacking the effective provider scopes", func(t *testing.T) {
		result := serviceScopeFixture{sessions: scopeSessions{scopeGithubID: {Usability: servicescope.UsabilityUsable, Scopes: []string{"read:user"}}}}.impersonate(t, github)
		requireRefused(t, result, tokenexchange.InvalidScopeError)
	})

	t.Run("transient usability fails closed with server_error", func(t *testing.T) {
		result := serviceScopeFixture{sessions: scopeSessions{scopeGithubID: {Usability: servicescope.UsabilityUnverifiableTransient}}}.impersonate(t, github)
		requireRefused(t, result, tokenexchange.ServerErrorCode)
	})

	t.Run("service lookup failure fails closed with server_error", func(t *testing.T) {
		result := serviceScopeFixture{services: &scopeServices{err: assert.AnError}}.impersonate(t, github)
		tokenErr := requireRefused(t, result, tokenexchange.ServerErrorCode)
		assert.ErrorIs(t, tokenErr, assert.AnError)
	})

	t.Run("permission set lookup failure fails closed with server_error", func(t *testing.T) {
		result := serviceScopeFixture{permissionSets: scopePermissionSets{err: assert.AnError}}.impersonate(t, github)
		tokenErr := requireRefused(t, result, tokenexchange.ServerErrorCode)
		assert.ErrorIs(t, tokenErr, assert.AnError)
	})

	t.Run("agent subject never receives a service scope", func(t *testing.T) {
		services := &scopeServices{}
		result := serviceScopeFixture{services: services, categories: []string{"agent"}, agentSubject: true}.impersonate(t, github)
		requireRefused(t, result, tokenexchange.InvalidScopeError)
		assert.Equal(t, SubjectCategoryAgent, result.outcome.Audit.SubjectCategory)
		assert.Zero(t, result.verifier.calls)
		assert.Zero(t, services.calls)
	})

	t.Run("agent subject keeps ordinary scopes", func(t *testing.T) {
		result := serviceScopeFixture{categories: []string{"agent"}, agentSubject: true}.impersonate(t, "read")
		require.NoError(t, result.err)
		assert.Equal(t, "read", result.outcome.Response.Scope)
	})

	t.Run("broker scopes fail closed when service scopes are not configured", func(t *testing.T) {
		result := serviceScopeFixture{unconfigured: true}.impersonate(t, github)
		requireRefused(t, result, tokenexchange.InvalidScopeError)
	})

	t.Run("scopes of another origin stay ordinary and follow the allow-list", func(t *testing.T) {
		result := serviceScopeFixture{}.impersonate(t, "https://other.example.com/sessions/github")
		var tokenErr *tokenexchange.TokenExchangeError
		require.ErrorAs(t, result.err, &tokenErr)
		assert.Equal(t, tokenexchange.InvalidScopeError, tokenErr.Code())
		assert.Empty(t, result.outcome.Audit.SelectedRule)
		assert.Zero(t, result.verifier.calls)
	})
}
