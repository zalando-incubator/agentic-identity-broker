package oauth2server

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const providerRefreshRedirect = "http://localhost:8080/callback"

type providerRefreshFixture struct {
	provider *Provider
	agents   *memory.AgentRepository
	agent    *storage.Agent
	secret   string
	issued   *ports.TokenResponse
	root     *storage.RefreshSession
}

// Issue through PKCE and the real in-memory repositories: the resulting root,
// original grant and token must be the same ones HandleRefreshToken later reads.
func issueProviderRefresh(t *testing.T, f *providerRefreshFixture, clientID, redirectURI, scope string) {
	t.Helper()
	ctx := context.Background()
	verifier := "provider-refresh-pkce-verifier-01234567890123456789"
	code, err := f.provider.HandleAuthorize(ctx, clientID, redirectURI, "code", scope, "state", generateS256Challenge(verifier), "S256", id.NewPrincipal("user@example.com"))
	require.NoError(t, err)
	f.issued, err = f.provider.HandleAuthorizationCodeExchange(ctx, clientID, f.secret, code, redirectURI, verifier)
	require.NoError(t, err)
	require.NotEmpty(t, f.issued.RefreshToken)
	record, err := f.provider.fositeStorage.codeRepo.FindByCodeHash(ctx, sha256Hex(code))
	require.NoError(t, err)
	rootID, err := id.ParseRefreshSessionID(record.ID.String())
	require.NoError(t, err)
	f.root, err = f.provider.refreshDeps.Sessions.FindByID(ctx, rootID)
	require.NoError(t, err)
	require.Equal(t, f.provider.refreshStrategy.RefreshTokenSignature(ctx, f.issued.RefreshToken), f.root.OriginalTokenSignature)
	initial, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.OriginalTokenSignature)
	require.NoError(t, err)
	require.Equal(t, f.root.ID, initial.SessionID)
	require.Nil(t, initial.UsedAt)
}

func newProviderRefreshFixture(t *testing.T) *providerRefreshFixture {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.RedirectURIs = []string{providerRefreshRedirect}
	agent.AllowedScopes = []string{"offline_access", "read", "write"}
	require.NoError(t, agents.Update(context.Background(), agent))
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent, secret: secret}
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	return f
}

// The same registered CIMD client can withdraw its refresh grant between requests.
func newCIMDProviderRefreshFixture(t *testing.T) (*providerRefreshFixture, *bool) {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	const clientID = "https://client.example.com/refresh-client.json"
	const redirect = "https://client.example.com/callback"
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "CIMD refresh agent", Description: "Client metadata refresh capability test",
		PermissionSets: testAgentPermissionSets(), ClientURIs: []string{clientID}, AllowedScopes: []string{"offline_access", "read"},
	}
	require.NoError(t, agents.Create(context.Background(), agent))
	seedProviderTestGrant(t, provider, agent.ID)
	allowsRefresh := true
	resolver := &mockClientResolver{resolveFunc: func(ctx context.Context, requested id.ClientID) (*ports.ClientResolution, error) {
		if requested.String() != clientID {
			return nil, &ports.ClientIDError{Code: "invalid_client", Desc: "unknown metadata document"}
		}
		current, err := agents.Get(ctx, agent.ID)
		if err != nil {
			return nil, err
		}
		grantTypes := []string{"authorization_code"}
		if allowsRefresh {
			grantTypes = append(grantTypes, "refresh_token")
		}
		return &ports.ClientResolution{Agent: current, CIMDMetadata: &ports.CIMDMetadataDTO{
			ClientID: clientID, RedirectURIs: []string{redirect}, GrantTypes: grantTypes,
		}}, nil
	}}
	provider.fositeStorage.clientResolver = resolver
	provider.clientAuth.clientResolver = resolver
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent}
	issueProviderRefresh(t, f, clientID, redirect, "offline_access read")
	return f, &allowsRefresh
}

// A denied exchange returns no usable result and changes neither the root nor
// any already-issued native token, including a consumed predecessor.
func (f *providerRefreshFixture) requireDenial(t *testing.T, clientID, secret, rawToken, scope, code string) {
	t.Helper()
	ctx := context.Background()
	beforeRoot, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	signatures := []string{beforeRoot.OriginalTokenSignature, beforeRoot.CurrentSignature}
	if beforeRoot.PreviousSignature != nil {
		signatures = append(signatures, *beforeRoot.PreviousSignature)
	}
	beforeTokens := make(map[string]*storage.RefreshToken, len(signatures))
	for _, signature := range signatures {
		token, lookupErr := f.provider.refreshDeps.Tokens.FindBySignature(ctx, signature)
		require.NoError(t, lookupErr)
		beforeTokens[signature] = token
	}

	response, err := f.provider.HandleRefreshToken(ctx, clientID, secret, rawToken, scope)
	require.True(t, response == nil, "denied refresh must not expose minted or cached tokens")
	var oauthErr *RFC6749Error
	require.ErrorAs(t, err, &oauthErr)
	require.Equal(t, code, oauthErr.Code())
	status := map[string]int{
		"invalid_client": http.StatusUnauthorized, "invalid_grant": http.StatusBadRequest,
		"server_error": http.StatusInternalServerError, "unauthorized_client": http.StatusBadRequest,
		"invalid_scope": http.StatusBadRequest,
	}
	require.Equal(t, status[code], oauthErr.HTTPStatus())
	sentinels := map[string]error{
		"invalid_client": ErrInvalidClient, "invalid_grant": ErrInvalidGrant,
		"server_error": ErrServerError, "invalid_scope": ErrInvalidScope,
	}
	if sentinel := sentinels[code]; sentinel != nil {
		require.ErrorIs(t, err, sentinel)
	}

	afterRoot, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(beforeRoot, afterRoot), "denial must preserve origin, clocks, scope ceiling, retry result and terminal state")
	for signature, before := range beforeTokens {
		after, lookupErr := f.provider.refreshDeps.Tokens.FindBySignature(ctx, signature)
		require.NoError(t, lookupErr)
		require.Equal(t, before, after, "denial must preserve token %s", signature)
	}
}

type providerRefreshVerifier struct {
	base  ports.UserDelegationVerifier
	fault error
	times []time.Time
}

func (v *providerRefreshVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time) (ports.UserDelegationDecision, error) {
	v.times = append(v.times, at)
	if v.fault != nil {
		return ports.UserDelegationDecision{}, v.fault
	}
	return v.base.VerifyUserDelegation(ctx, principal, agentID, at)
}

func (f *providerRefreshFixture) observeConsent() *providerRefreshVerifier {
	verifier := &providerRefreshVerifier{base: f.provider.refreshDeps.Verifier}
	f.provider.refreshDeps.Verifier = verifier
	f.provider.fositeStorage.refresh.Verifier = verifier
	return verifier
}

type providerRefreshCoordinator struct {
	base   ports.AuthorizationSessionCoordinator
	before func() error
	at     *time.Time
}

func (c *providerRefreshCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	if c.before != nil {
		if err := c.before(); err != nil {
			return err
		}
	}
	return c.base.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
		if c.at != nil {
			at = *c.at
		}
		return operation(owner, at)
	})
}

type providerRefreshClock struct{ at time.Time }

func (c providerRefreshClock) Now(context.Context) (time.Time, error) { return c.at, nil }

func (f *providerRefreshFixture) decideAt(at time.Time) {
	coordinator := &providerRefreshCoordinator{base: f.provider.refreshDeps.Coordinator, at: &at}
	f.provider.refreshDeps.Coordinator = coordinator
	f.provider.fositeStorage.refresh.Coordinator = coordinator
	clock := providerRefreshClock{at: at}
	f.provider.refreshDeps.Clock = clock
	f.provider.fositeStorage.refresh.Clock = clock
}

func TestProviderRefresh_OriginalGrantIdentityAndUnknownConsent(t *testing.T) {
	t.Run("deleted grant denies its existing refresh token without consuming it", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
		grant, err := grants.FindByPrincipalAndAgent(ctx, f.root.Principal, f.agent.ID)
		require.NoError(t, err)
		require.NoError(t, grants.Delete(ctx, grant.ID))
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})

	t.Run("re-created grant with a new ID cannot adopt an old session", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
		original, err := grants.FindByPrincipalAndAgent(ctx, f.root.Principal, f.agent.ID)
		require.NoError(t, err)
		require.Equal(t, original.ID, f.root.OriginalGrantID)
		require.NoError(t, grants.Delete(ctx, original.ID))
		replacement := &storage.UserGrant{ID: id.NewGrantID(), Principal: original.Principal, AgentID: original.AgentID}
		require.NoError(t, grants.Create(ctx, replacement))
		require.NotEqual(t, f.root.OriginalGrantID, replacement.ID)
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})

	t.Run("unknown consent returns server error and never consumes a token", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		verifier := f.observeConsent()
		verifier.fault = errors.New("delegation storage unavailable")
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.Len(t, verifier.times, 1)
		verifier.fault = nil
		result, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, result.RefreshToken, "the previously denied token remains usable")
	})

	t.Run("active grant changes preserve origin and original session start", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
		grant, err := grants.FindByPrincipalAndAgent(ctx, f.root.Principal, f.agent.ID)
		require.NoError(t, err)
		until := f.root.StartedAt.Add(2 * time.Hour)
		grant.ValidUntil = &until
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}}
		require.NoError(t, grants.Update(ctx, grant))
		result, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "read")
		require.NoError(t, err)
		require.Equal(t, "read", result.Scope)
		root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
		require.NoError(t, err)
		require.Equal(t, grant.ID, root.OriginalGrantID)
		require.Equal(t, f.root.StartedAt, root.StartedAt)
		require.Equal(t, f.root.Scope, root.Scope)
	})
}

func TestProviderRefresh_ErrorOrderAndSharedDeadline(t *testing.T) {
	t.Run("authentication and client binding precede faulty consent and elapsed lifetime", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		other, _, otherSecret := setupTestCredentials(t, f.provider, f.agents)
		f.decideAt(f.root.InactivityExpiresAt)
		verifier := f.observeConsent()
		verifier.fault = errors.New("consent lookup failed")
		f.requireDenial(t, f.agent.ID.String(), "incorrect-secret", f.issued.RefreshToken, "", "invalid_client")
		f.requireDenial(t, other.ID.String(), otherSecret, f.issued.RefreshToken, "", "invalid_grant")
		require.Empty(t, verifier.times, "neither unauthenticated nor unbound clients may reach consent")
	})

	t.Run("consent fault precedes expired lifetime", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		f.decideAt(f.root.InactivityExpiresAt)
		verifier := f.observeConsent()
		verifier.fault = errors.New("consent lookup failed")
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.Equal(t, []time.Time{f.root.InactivityExpiresAt}, verifier.times)
	})

	t.Run("grant validity equal to decision time is expired before session lifetime", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		at := f.root.StartedAt.Add(10 * time.Minute)
		grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
		grant, err := grants.FindByPrincipalAndAgent(ctx, f.root.Principal, f.agent.ID)
		require.NoError(t, err)
		grant.ValidUntil = &at
		require.NoError(t, grants.Update(ctx, grant))
		f.decideAt(at)
		verifier := f.observeConsent()
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
		require.Equal(t, []time.Time{at}, verifier.times)
		require.True(t, at.Before(f.root.InactivityExpiresAt))
	})

	t.Run("stored inactivity equality rejects without advancing the clock", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		f.decideAt(f.root.InactivityExpiresAt)
		verifier := f.observeConsent()
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
		require.Equal(t, []time.Time{f.root.InactivityExpiresAt}, verifier.times)
	})

	t.Run("shortened effective inactivity equality rejects before stored expiry", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		f.provider.refreshDeps.Policy.InactivityLifetime = 10 * time.Minute
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
		at := f.root.LastFreshAt.Add(10 * time.Minute)
		require.True(t, at.Before(f.root.InactivityExpiresAt))
		f.decideAt(at)
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})

	t.Run("shortened effective absolute equality rejects before stored expiry", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		f.provider.refreshDeps.Policy.AbsoluteLifetime = 10 * time.Minute
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
		at := f.root.StartedAt.Add(10 * time.Minute)
		require.True(t, at.Before(f.root.InactivityExpiresAt))
		f.decideAt(at)
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})

	t.Run("consent, rotation and recheck share supplied decision time", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		at := time.Now().UTC()
		f.decideAt(at)
		verifier := f.observeConsent()
		result, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, result.RefreshToken)
		require.NotEmpty(t, verifier.times)
		for _, observed := range verifier.times {
			require.Equal(t, at, observed)
		}
		root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
		require.NoError(t, err)
		require.Equal(t, at, root.LastFreshAt)
		require.Equal(t, at.Add(time.Hour), root.InactivityExpiresAt)
		previous, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), f.root.OriginalTokenSignature)
		require.NoError(t, err)
		require.NotNil(t, previous.UsedAt)
		require.Equal(t, at, *previous.UsedAt)
		next, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), root.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, at, next.IssuedAt)
		require.Equal(t, root.InactivityExpiresAt, next.ExpiresAt)
	})

	t.Run("deadline reached during commit recheck rolls back the staged successor", func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		firstAt := time.Now().UTC()
		f.decideAt(firstAt)
		secondAt := firstAt.Add(f.provider.refreshDeps.Policy.InactivityLifetime)
		clock := &absoluteRecheckClock{times: [2]time.Time{firstAt, secondAt}}
		f.provider.refreshDeps.Clock = clock
		f.provider.fositeStorage.refresh.Clock = clock
		verifier := f.observeConsent()
		f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
		require.Contains(t, verifier.times, firstAt)
		require.Contains(t, verifier.times, secondAt)
	})
}

func TestProviderRefresh_RevocationPrecedesUnavailableConsent(t *testing.T) {
	f := newProviderRefreshFixture(t)
	ctx := context.Background()
	require.NoError(t, f.provider.refreshDeps.Revocations.RevokeByID(ctx, f.root.ID, time.Now().UTC(), storage.RefreshReasonGrantDeleted))
	verifier := f.observeConsent()
	verifier.fault = errors.New("consent unavailable")
	f.decideAt(f.root.InactivityExpiresAt)
	f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	require.Empty(t, verifier.times, "terminal roots must not consult consent")

	// Revoking refresh authority does not retroactively shorten an issued JWT.
	keySet, err := f.provider.accessStrategy.signingKeyService.BuildJWKS(ctx)
	require.NoError(t, err)
	access, err := jwt.Parse([]byte(f.issued.AccessToken), jwt.WithKeySet(keySet))
	require.NoError(t, err)
	issuedAt, ok := access.IssuedAt()
	require.True(t, ok)
	expiresAt, ok := access.Expiration()
	require.True(t, ok)
	require.WithinDuration(t, issuedAt.Add(time.Hour), expiresAt, time.Second)
}

func TestProviderRefresh_RemovedCapabilityAndErrorOrder(t *testing.T) {
	t.Run("fresh request without refresh capability is denied without mutation", func(t *testing.T) {
		f, allowsRefresh := newCIMDProviderRefreshFixture(t)
		*allowsRefresh = false
		f.requireDenial(t, f.root.ClientID.String(), "", f.issued.RefreshToken, "", "unauthorized_client")
		*allowsRefresh = true
		result, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), "", f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, result.RefreshToken)
	})

	t.Run("consent fault outranks both expired lifetime and missing capability", func(t *testing.T) {
		f, allowsRefresh := newCIMDProviderRefreshFixture(t)
		*allowsRefresh = false
		f.decideAt(f.root.InactivityExpiresAt)
		verifier := f.observeConsent()
		verifier.fault = errors.New("grant verification unavailable")
		f.requireDenial(t, f.root.ClientID.String(), "", f.issued.RefreshToken, "", "server_error")
		require.Equal(t, []time.Time{f.root.InactivityExpiresAt}, verifier.times)
	})

	t.Run("elapsed lifetime outranks missing refresh capability", func(t *testing.T) {
		f, allowsRefresh := newCIMDProviderRefreshFixture(t)
		*allowsRefresh = false
		f.decideAt(f.root.InactivityExpiresAt)
		f.requireDenial(t, f.root.ClientID.String(), "", f.issued.RefreshToken, "", "invalid_grant")
	})

	t.Run("capability denial precedes destructive consumed-token replay", func(t *testing.T) {
		f, allowsRefresh := newCIMDProviderRefreshFixture(t)
		rotated, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), "", f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, rotated.RefreshToken)
		*allowsRefresh = false
		f.requireDenial(t, f.root.ClientID.String(), "", f.issued.RefreshToken, "", "unauthorized_client")
		*allowsRefresh = true
		response, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), "", f.issued.RefreshToken, "")
		require.Nil(t, response)
		require.ErrorIs(t, err, ErrInvalidGrant)
		var oauthErr *RFC6749Error
		require.ErrorAs(t, err, &oauthErr)
		require.Equal(t, "invalid_grant", oauthErr.Code())
		require.Equal(t, http.StatusBadRequest, oauthErr.HTTPStatus())
		root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
		require.NoError(t, err)
		require.NotNil(t, root.TerminalReason)
		require.Equal(t, storage.RefreshReasonProhibitedReuse, *root.TerminalReason)
	})
}

func TestProviderRefresh_WithdrawnResponseScope(t *testing.T) {
	f := newProviderRefreshFixture(t)
	f.agent.AllowedScopes = []string{"offline_access", "read"}
	require.NoError(t, f.agents.Update(context.Background(), f.agent))
	f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_scope")
	result, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "read")
	require.NoError(t, err)
	require.Equal(t, "read", result.Scope, "a permitted narrower response remains available")
	require.NotEmpty(t, result.RefreshToken)
}

func TestProviderRefresh_NarrowAccessResultDoesNotLowerSessionCeiling(t *testing.T) {
	f := newProviderRefreshFixture(t)
	ctx := context.Background()
	narrow, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "read")
	require.NoError(t, err)
	require.Equal(t, "read", narrow.Scope)
	require.NotEmpty(t, narrow.RefreshToken)
	root, err := f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	require.Equal(t, f.root.Scope, root.Scope)
	require.Equal(t, f.root.OriginalGrantID, root.OriginalGrantID)
	require.Equal(t, f.root.StartedAt, root.StartedAt)
	full, err := f.provider.HandleRefreshToken(ctx, f.agent.ID.String(), f.secret, narrow.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "offline_access read write", full.Scope)
	require.NotEqual(t, narrow.RefreshToken, full.RefreshToken)
	root, err = f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
	require.NoError(t, err)
	require.Equal(t, f.root.Scope, root.Scope)
	require.Equal(t, f.root.OriginalGrantID, root.OriginalGrantID)
	require.Equal(t, f.root.StartedAt, root.StartedAt)
	for _, tc := range []struct{ token, scope string }{{narrow.AccessToken, "read"}, {full.AccessToken, "offline_access read write"}} {
		claims, parseErr := jwt.ParseInsecure([]byte(tc.token))
		require.NoError(t, parseErr)
		actual, claimErr := jwt.Get[string](claims, "scope")
		require.NoError(t, claimErr)
		require.Equal(t, tc.scope, actual)
	}
}

func TestProviderRefresh_PublicSessionUsesCurrentCredentials(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Public local agent", Description: "PKCE agent promoted to credentials", PermissionSets: testAgentPermissionSets(), RedirectURIs: []string{providerRefreshRedirect}, AllowedScopes: []string{"offline_access", "read"}}
	require.NoError(t, agents.Create(context.Background(), agent))
	seedProviderTestGrant(t, provider, agent.ID)
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent}
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read")
	credential, secret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Create(context.Background(), credential))
	f.requireDenial(t, agent.ID.String(), "", f.issued.RefreshToken, "", "invalid_client")
	f.requireDenial(t, agent.ID.String(), "wrong-secret", f.issued.RefreshToken, "", "invalid_client")
	promoted, err := provider.HandleRefreshToken(context.Background(), agent.ID.String(), secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, promoted.RefreshToken)
	root, err := provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
	require.NoError(t, err)
	require.Equal(t, f.root.OriginalGrantID, root.OriginalGrantID)
	require.Equal(t, f.root.StartedAt, root.StartedAt)
	require.Equal(t, f.root.Scope, root.Scope)

	replacement, newSecret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Rotate(context.Background(), agent.ID, replacement))
	f.requireDenial(t, agent.ID.String(), secret, promoted.RefreshToken, "", "invalid_client")
	latest, err := provider.HandleRefreshToken(context.Background(), agent.ID.String(), newSecret, promoted.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, latest.RefreshToken)
}

func TestProviderRefresh_ReauthenticatesInsideOwnerAfterCredentialReplacement(t *testing.T) {
	f := newProviderRefreshFixture(t)
	replacement, newSecret, err := f.provider.clientAuth.GenerateCredentials(f.agent.ID)
	require.NoError(t, err)
	baseCoordinator, baseClock := f.provider.refreshDeps.Coordinator, f.provider.refreshDeps.Clock
	f.decideAt(f.root.InactivityExpiresAt)
	verifier := f.observeConsent()
	verifier.fault = errors.New("consent lookup failed")
	coordinator := &providerRefreshCoordinator{base: f.provider.refreshDeps.Coordinator, before: func() error {
		return f.provider.fositeStorage.credRepo.Rotate(context.Background(), f.agent.ID, replacement)
	}}
	f.provider.refreshDeps.Coordinator = coordinator
	f.provider.fositeStorage.refresh.Coordinator = coordinator
	// The old secret authenticates before the coordinator, then a replacement
	// commits before the owner callback. In-owner reauthentication must reject
	// it BEFORE consulting broken consent or elapsed lifetime.
	f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_client")
	require.Empty(t, verifier.times, "changed credentials must be rejected before consent")

	coordinator.before = nil
	verifier.fault = nil
	f.provider.refreshDeps.Coordinator = baseCoordinator
	f.provider.fositeStorage.refresh.Coordinator = baseCoordinator
	f.provider.refreshDeps.Clock = baseClock
	f.provider.fositeStorage.refresh.Clock = baseClock
	f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_client")
	result, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), newSecret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, result.RefreshToken)
}
