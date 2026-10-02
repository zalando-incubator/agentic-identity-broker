package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

func enableProviderRetry(t *testing.T, f *providerRefreshFixture) {
	t.Helper()
	cipher := testutil.NewTestEncryptionAdapter(t)
	f.provider.refreshDeps.Encryption = cipher
	f.provider.fositeStorage.refresh.Encryption = cipher
	f.provider.refreshDeps.Policy.ReuseInterval = 30 * time.Second
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
	f.decideAt(time.Now().UTC())
}

func newProviderRetryFixture(t *testing.T) *providerRefreshFixture {
	t.Helper()
	f := newProviderRefreshFixture(t)
	enableProviderRetry(t, f)
	return f
}

func providerRetryRoot(t *testing.T, f *providerRefreshFixture) *storage.RefreshSession {
	t.Helper()
	root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
	require.NoError(t, err)
	return root
}

func rotateProviderRetry(t *testing.T, f *providerRefreshFixture, ctx context.Context, scope string) *ports.TokenResponse {
	t.Helper()
	result, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, scope)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
	root := providerRetryRoot(t, f)
	require.Equal(t, f.root.OriginalTokenSignature, *root.PreviousSignature)
	require.Equal(t, f.provider.refreshStrategy.RefreshTokenSignature(ctx, result.RefreshToken), root.CurrentSignature)
	return result
}

func requireOriginalRetryResult(t *testing.T, original, recovered *ports.TokenResponse) {
	t.Helper()
	require.NotNil(t, recovered)
	require.True(t, original.AccessToken == recovered.AccessToken, "retry must return the original access token")
	require.True(t, original.RefreshToken == recovered.RefreshToken, "retry must return the original refresh token")
	require.Equal(t, original.TokenType, recovered.TokenType)
	require.Equal(t, original.Scope, recovered.Scope)
	require.GreaterOrEqual(t, recovered.ExpiresIn, int64(0))
	require.LessOrEqual(t, recovered.ExpiresIn, original.ExpiresIn)
}

func requireRetryOnlyCountChanged(t *testing.T, f *providerRefreshFixture, before *storage.RefreshSession, count int) {
	t.Helper()
	after := providerRetryRoot(t, f)
	expected := *before
	expected.RetryCount = count
	expected.RetryCiphertext = nil
	observed := *after
	observed.RetryCiphertext = nil
	require.Equal(t, &expected, &observed, "accepted retry may change only its counter")
	require.True(t, bytes.Equal(before.RetryCiphertext, after.RetryCiphertext), "accepted retry cannot replace the encrypted result")
	previous, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), *before.PreviousSignature)
	require.NoError(t, err)
	require.NotNil(t, previous.UsedAt)
	require.Equal(t, *before.PreviousConsumedAt, *previous.UsedAt)
	current, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), before.CurrentSignature)
	require.NoError(t, err)
	require.Nil(t, current.UsedAt)
	require.Equal(t, before.LastFreshAt, current.IssuedAt)
	require.Equal(t, before.InactivityExpiresAt, current.ExpiresAt)
}

func requireProhibitedProviderRetry(t *testing.T, f *providerRefreshFixture, ctx context.Context, rawToken, scope string) {
	t.Helper()
	result, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, rawToken, scope)
	require.True(t, result == nil, "prohibited reuse must not return credentials")
	require.ErrorIs(t, err, ErrInvalidGrant)
	var oauthErr *RFC6749Error
	require.ErrorAs(t, err, &oauthErr)
	require.Equal(t, "invalid_grant", oauthErr.Code())
	require.Equal(t, http.StatusBadRequest, oauthErr.HTTPStatus())
	root := providerRetryRoot(t, f)
	require.NotNil(t, root.RevokedAt)
	require.NotNil(t, root.TerminalReason)
	require.Equal(t, storage.RefreshReasonProhibitedReuse, *root.TerminalReason)
	require.True(t, len(root.RetryCiphertext) == 0, "prohibited reuse must erase the encrypted result")
}

func TestProviderRefreshRetry_ReturnsOriginalPairWithoutChangingRotation(t *testing.T) {
	f := newProviderRetryFixture(t)
	ctx := context.Background()
	original := rotateProviderRetry(t, f, ctx, "read write")
	before := providerRetryRoot(t, f)
	require.Zero(t, before.RetryCount)
	require.Equal(t, "read write", *before.OriginalRequestedScope)
	require.Equal(t, f.root.Scope, before.Scope)
	previous, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, *before.PreviousSignature)
	require.NoError(t, err)
	current, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, before.CurrentSignature)
	require.NoError(t, err)

	f.decideAt(before.PreviousConsumedAt.Add(2 * time.Second))
	recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read write")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	requireRetryOnlyCountChanged(t, f, before, 1)
	previousAfter, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, *before.PreviousSignature)
	require.NoError(t, err)
	currentAfter, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, before.CurrentSignature)
	require.NoError(t, err)
	require.Equal(t, previous, previousAfter)
	require.Equal(t, current, currentAfter)
	require.NotEmpty(t, providerRetryRoot(t, f).RetryCiphertext, "the original pair must remain encrypted while retryable")
}

func TestProviderRefreshRetry_CanonicalScopeAndChangedScope(t *testing.T) {
	t.Run("duplicate and reordered requested scopes recover the same result", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		ctx := context.Background()
		original := rotateProviderRetry(t, f, ctx, "write read")
		before := providerRetryRoot(t, f)
		require.Equal(t, "read write", *before.OriginalRequestedScope)
		f.decideAt(before.LastFreshAt.Add(time.Second))
		recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read  write read")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
		requireRetryOnlyCountChanged(t, f, before, 1)
	})
	t.Run("different scope is prohibited reuse, not a new result", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		ctx := context.Background()
		rotated := rotateProviderRetry(t, f, ctx, "write read")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		requireProhibitedProviderRetry(t, f, ctx, f.issued.RefreshToken, "read")
		f.requireDenial(t, f.root.ClientID.String(), f.secret, rotated.RefreshToken, "", "invalid_grant")
	})
	t.Run("omitted scope differs from an explicit narrowing request", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		rotateProviderRetry(t, f, context.Background(), "read")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		requireProhibitedProviderRetry(t, f, context.Background(), f.issued.RefreshToken, "")
	})
}

func TestProviderRefreshRetry_ThreeReturnsThenProhibitedReuse(t *testing.T) {
	f := newProviderRetryFixture(t)
	ctx := context.Background()
	original := rotateProviderRetry(t, f, ctx, "")
	before := providerRetryRoot(t, f)
	for count := 1; count <= 3; count++ {
		f.decideAt(before.LastFreshAt.Add(time.Duration(count) * time.Second))
		recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
		requireRetryOnlyCountChanged(t, f, before, count)
	}
	f.decideAt(before.LastFreshAt.Add(4 * time.Second))
	requireProhibitedProviderRetry(t, f, ctx, f.issued.RefreshToken, "")
	f.requireDenial(t, f.root.ClientID.String(), f.secret, original.RefreshToken, "", "invalid_grant")
}

func TestProviderRefreshRetry_FixedReuseDeadlineEquality(t *testing.T) {
	f := newProviderRetryFixture(t)
	ctx := context.Background()
	original := rotateProviderRetry(t, f, ctx, "")
	before := providerRetryRoot(t, f)
	require.Equal(t, before.PreviousConsumedAt.Add(30*time.Second), *before.ReuseUntil)
	f.decideAt(before.ReuseUntil.Add(-time.Nanosecond))
	recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	requireRetryOnlyCountChanged(t, f, before, 1)
	f.decideAt(*before.ReuseUntil)
	requireProhibitedProviderRetry(t, f, ctx, f.issued.RefreshToken, "")
}

func TestProviderRefreshRetry_AccessExpiryDoesNotRevokeCurrent(t *testing.T) {
	f := newProviderRetryFixture(t)
	f.provider.accessStrategy.tokenTTL = 10 * time.Second
	f.provider.config.AccessTokenLifespan = 10 * time.Second
	rotateProviderRetry(t, f, context.Background(), "")
	root := providerRetryRoot(t, f)
	require.True(t, root.RetryAccessExpiresAt.Before(*root.ReuseUntil))
	f.decideAt(*root.RetryAccessExpiresAt)
	f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	require.True(t, root.InactivityExpiresAt.After(*root.RetryAccessExpiresAt))
	current, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), root.CurrentSignature)
	require.NoError(t, err)
	require.Nil(t, current.UsedAt)
	require.True(t, current.ExpiresAt.After(*root.RetryAccessExpiresAt))
	require.Nil(t, providerRetryRoot(t, f).TerminalReason)
}

func TestProviderRefreshRetry_ExpiredConsumedAncestorStillRevokes(t *testing.T) {
	f := newProviderRetryFixture(t)
	ctx := context.Background()
	f.decideAt(time.Now().UTC())
	first := rotateProviderRetry(t, f, ctx, "")
	f.decideAt(time.Now().UTC())
	second, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, first.RefreshToken, "")
	require.NoError(t, err)
	require.NotEmpty(t, second.RefreshToken)
	root := providerRetryRoot(t, f)
	require.True(t, f.root.InactivityExpiresAt.Before(root.InactivityExpiresAt), "fresh rotations keep the current successor alive past the oldest token's expiry")
	f.decideAt(f.root.InactivityExpiresAt)
	requireProhibitedProviderRetry(t, f, ctx, f.issued.RefreshToken, "")
	f.requireDenial(t, f.root.ClientID.String(), f.secret, second.RefreshToken, "", "invalid_grant")
}

func TestProviderRefreshRetry_NarrowResultKeepsOriginalCeiling(t *testing.T) {
	f := newProviderRetryFixture(t)
	ctx := context.Background()
	narrow := rotateProviderRetry(t, f, ctx, "read")
	require.Equal(t, "read", narrow.Scope)
	root := providerRetryRoot(t, f)
	require.Equal(t, f.root.Scope, root.Scope)
	f.decideAt(time.Now().UTC())
	recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
	require.NoError(t, err)
	requireOriginalRetryResult(t, narrow, recovered)
	requireRetryOnlyCountChanged(t, f, root, 1)
	f.decideAt(time.Now().UTC())
	full, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, narrow.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "offline_access read write", full.Scope)
	require.True(t, full.RefreshToken != narrow.RefreshToken)
	require.Equal(t, f.root.Scope, providerRetryRoot(t, f).Scope)
}

func TestProviderRefreshRetry_CurrentAuthorizationAndResponseScopeOrder(t *testing.T) {
	t.Run("consent uncertainty precedes retry classification", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		original := rotateProviderRetry(t, f, context.Background(), "")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		verifier := f.observeConsent()
		verifier.fault = errors.New("grant storage unavailable")
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read", "server_error")
		verifier.fault = nil
		recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
	})
	t.Run("expired consent precedes scope-mismatched reuse", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		rotateProviderRetry(t, f, context.Background(), "")
		root := providerRetryRoot(t, f)
		grants := f.provider.refreshDeps.Verifier.(testGrantVerifier).grants
		grant, err := grants.FindByPrincipalAndAgent(context.Background(), root.Principal, root.AgentID)
		require.NoError(t, err)
		until := root.LastFreshAt.Add(500 * time.Millisecond)
		grant.ValidUntil = &until
		require.NoError(t, grants.Update(context.Background(), grant))
		f.decideAt(root.LastFreshAt.Add(time.Second))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read", "invalid_grant")
	})
	t.Run("session deadline precedes scope-mismatched reuse", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		rotateProviderRetry(t, f, context.Background(), "")
		f.provider.refreshDeps.Policy.AbsoluteLifetime = 30*time.Second + 500*time.Millisecond
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
		f.decideAt(f.root.StartedAt.Add(f.provider.refreshDeps.Policy.AbsoluteLifetime))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read", "invalid_grant")
	})
	t.Run("CIMD refresh capability precedes replay and does not consume retry", func(t *testing.T) {
		f, allowsRefresh := newCIMDProviderRefreshFixture(t)
		enableProviderRetry(t, f)
		original := rotateProviderRetry(t, f, context.Background(), "")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		*allowsRefresh = false
		f.requireDenial(t, f.root.ClientID.String(), "", f.issued.RefreshToken, "read", "unauthorized_client")
		*allowsRefresh = true
		recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), "", f.issued.RefreshToken, "")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
	})
	t.Run("eligible result checks its exact original response scope", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		original := rotateProviderRetry(t, f, context.Background(), "read")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		f.agent.AllowedScopes = []string{"offline_access", "write"}
		require.NoError(t, f.agents.Update(context.Background(), f.agent))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read", "invalid_scope")
		f.agent.AllowedScopes = []string{"offline_access", "read", "write"}
		require.NoError(t, f.agents.Update(context.Background(), f.agent))
		recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "read")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
	})
	t.Run("prohibited reuse revokes before checking withdrawn result scope", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		rotateProviderRetry(t, f, context.Background(), "read write")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		f.agent.AllowedScopes = []string{"offline_access", "read"}
		require.NoError(t, f.agents.Update(context.Background(), f.agent))
		requireProhibitedProviderRetry(t, f, context.Background(), f.issued.RefreshToken, "read")
	})
}

func TestProviderRefreshRetry_PublicPromotionAndCurrentCredentials(t *testing.T) {
	provider, agents, _ := newTestProvider(t)
	agent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Public local agent", Description: "Promoted retry test",
		PermissionSets: testAgentPermissionSets(), RedirectURIs: []string{providerRefreshRedirect},
		AllowedScopes: []string{"offline_access", "read"},
	}
	require.NoError(t, agents.Create(context.Background(), agent))
	seedProviderTestGrant(t, provider, agent.ID)
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent}
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read")
	enableProviderRetry(t, f)
	original := rotateProviderRetry(t, f, context.Background(), "")
	root := providerRetryRoot(t, f)
	credential, secret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Create(context.Background(), credential))
	f.decideAt(root.LastFreshAt.Add(time.Second))
	f.requireDenial(t, agent.ID.String(), "", f.issued.RefreshToken, "", "invalid_client")
	f.requireDenial(t, agent.ID.String(), "incorrect-secret", f.issued.RefreshToken, "", "invalid_client")
	recovered, err := provider.HandleRefreshToken(context.Background(), agent.ID.String(), secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	requireRetryOnlyCountChanged(t, f, root, 1)

	replacement, newSecret, err := provider.clientAuth.GenerateCredentials(agent.ID)
	require.NoError(t, err)
	require.NoError(t, provider.fositeStorage.credRepo.Rotate(context.Background(), agent.ID, replacement))
	f.requireDenial(t, agent.ID.String(), secret, f.issued.RefreshToken, "", "invalid_client")
	recovered, err = provider.HandleRefreshToken(context.Background(), agent.ID.String(), newSecret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	requireRetryOnlyCountChanged(t, f, root, 2)
}

func TestProviderRefreshRetry_RequestContextFingerprintIsAuditOnly(t *testing.T) {
	f := newProviderRetryFixture(t)
	originalContext := security.WithSecurityContext(context.Background(), security.SecurityContext{ClientIP: "203.0.113.20:443", UserAgent: "client/1.0"})
	original := rotateProviderRetry(t, f, originalContext, "")
	before := providerRetryRoot(t, f)
	require.Equal(t, refreshRequestFingerprint(originalContext), *before.OriginalRequestContextFingerprint)
	otherContext := security.WithSecurityContext(context.Background(), security.SecurityContext{ClientIP: "198.51.100.8:8443", UserAgent: "different-client/2.0"})
	require.NotEqual(t, refreshRequestFingerprint(otherContext), *before.OriginalRequestContextFingerprint)
	f.decideAt(before.LastFreshAt.Add(time.Second))
	recovered, err := f.provider.HandleRefreshToken(otherContext, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
	require.NoError(t, err)
	requireOriginalRetryResult(t, original, recovered)
	requireRetryOnlyCountChanged(t, f, before, 1)
}

type providerRetryFaultCoordinator struct {
	base    ports.AuthorizationSessionCoordinator
	after   func(context.Context) error
	lostAck bool
}

func (c *providerRetryFaultCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	err := c.base.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
		if err := operation(owner, at); err != nil {
			return err
		}
		if c.after != nil {
			return c.after(owner)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if c.lostAck {
		return fmt.Errorf("%w: acknowledgement lost", ports.ErrCommitIndeterminate)
	}
	return nil
}

func TestProviderRefreshRetry_ConfirmedRollbackPreservesRotationAndRetry(t *testing.T) {
	t.Run("aborted rotation does not consume the predecessor", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		before := providerRetryRoot(t, f)
		original, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), before.CurrentSignature)
		require.NoError(t, err)
		base := f.provider.refreshDeps.Coordinator
		staged := false
		fault := &providerRetryFaultCoordinator{base: base, after: func(owner context.Context) error {
			root, err := f.provider.refreshDeps.Sessions.FindByID(owner, f.root.ID)
			if err != nil {
				return err
			}
			staged = root.CurrentSignature != before.CurrentSignature
			return errors.New("confirmed rollback after staging rotation")
		}}
		f.provider.refreshDeps.Coordinator = fault
		f.provider.fositeStorage.refresh.Coordinator = fault
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.True(t, staged, "a staged successor must have been rolled back")
		unchanged, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), before.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, original, unchanged)
		f.provider.refreshDeps.Coordinator = base
		f.provider.fositeStorage.refresh.Coordinator = base
		rotateProviderRetry(t, f, context.Background(), "")
	})
	t.Run("aborted retry does not spend a result return", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		original := rotateProviderRetry(t, f, context.Background(), "")
		before := providerRetryRoot(t, f)
		f.decideAt(before.LastFreshAt.Add(time.Second))
		base := f.provider.refreshDeps.Coordinator
		staged := false
		fault := &providerRetryFaultCoordinator{base: base, after: func(owner context.Context) error {
			root, err := f.provider.refreshDeps.Sessions.FindByID(owner, f.root.ID)
			if err != nil {
				return err
			}
			staged = root.RetryCount == 1
			return errors.New("confirmed rollback after staging retry count")
		}}
		f.provider.refreshDeps.Coordinator = fault
		f.provider.fositeStorage.refresh.Coordinator = fault
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.True(t, staged, "a staged retry increment must have been rolled back")
		f.provider.refreshDeps.Coordinator = base
		f.provider.fositeStorage.refresh.Coordinator = base
		recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		requireOriginalRetryResult(t, original, recovered)
		requireRetryOnlyCountChanged(t, f, before, 1)
	})
}

func TestProviderRefreshRetry_LostAcknowledgementsResolveDurableState(t *testing.T) {
	t.Run("committed rotation recovers exactly its original successor", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		base := f.provider.refreshDeps.Coordinator
		fault := &providerRetryFaultCoordinator{base: base, lostAck: true}
		f.provider.refreshDeps.Coordinator = fault
		f.provider.fositeStorage.refresh.Coordinator = fault
		result, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.True(t, result == nil, "lost acknowledgement must not return credentials")
		require.ErrorIs(t, err, ErrServerError)
		committed := providerRetryRoot(t, f)
		require.Equal(t, f.root.OriginalTokenSignature, *committed.PreviousSignature)
		require.Zero(t, committed.RetryCount)
		f.provider.refreshDeps.Coordinator = base
		f.provider.fositeStorage.refresh.Coordinator = base
		f.decideAt(committed.LastFreshAt.Add(time.Second))
		recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotNil(t, recovered)
		require.Equal(t, committed.CurrentSignature, f.provider.refreshStrategy.RefreshTokenSignature(context.Background(), recovered.RefreshToken))
		plaintext, err := f.provider.refreshDeps.Encryption.Decrypt(context.Background(), committed.RetryCiphertext, encryption.NewRefreshSessionBranchKeySubject(committed.ID).EncryptionContext())
		require.NoError(t, err)
		var stored struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		require.NoError(t, json.Unmarshal(plaintext, &stored))
		require.True(t, stored.AccessToken == recovered.AccessToken, "lost rotation must recover its committed access token")
		require.True(t, stored.RefreshToken == recovered.RefreshToken, "lost rotation must recover its committed refresh token")
		requireRetryOnlyCountChanged(t, f, committed, 1)
	})
	t.Run("committed retry count is not refunded after acknowledgement loss", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		original := rotateProviderRetry(t, f, context.Background(), "")
		before := providerRetryRoot(t, f)
		f.decideAt(before.LastFreshAt.Add(time.Second))
		base := f.provider.refreshDeps.Coordinator
		fault := &providerRetryFaultCoordinator{base: base, lostAck: true}
		f.provider.refreshDeps.Coordinator = fault
		f.provider.fositeStorage.refresh.Coordinator = fault
		result, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.True(t, result == nil, "lost acknowledgement must not return credentials")
		require.ErrorIs(t, err, ErrServerError)
		requireRetryOnlyCountChanged(t, f, before, 1)
		f.provider.refreshDeps.Coordinator = base
		f.provider.fositeStorage.refresh.Coordinator = base
		for count := 2; count <= 3; count++ {
			f.decideAt(before.LastFreshAt.Add(time.Duration(count) * time.Second))
			recovered, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
			require.NoError(t, err)
			requireOriginalRetryResult(t, original, recovered)
			requireRetryOnlyCountChanged(t, f, before, count)
		}
		f.decideAt(before.LastFreshAt.Add(4 * time.Second))
		requireProhibitedProviderRetry(t, f, context.Background(), f.issued.RefreshToken, "")
	})
	t.Run("zero reuse cannot recover a committed rotation", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		f.provider.refreshDeps.Policy.ReuseInterval = 0
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
		base := f.provider.refreshDeps.Coordinator
		fault := &providerRetryFaultCoordinator{base: base, lostAck: true}
		f.provider.refreshDeps.Coordinator = fault
		f.provider.fositeStorage.refresh.Coordinator = fault
		result, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.True(t, result == nil, "lost acknowledgement must not return credentials")
		require.ErrorIs(t, err, ErrServerError)
		committed := providerRetryRoot(t, f)
		require.Equal(t, f.root.OriginalTokenSignature, *committed.PreviousSignature)
		require.True(t, len(committed.RetryCiphertext) == 0, "zero reuse must not persist a recovery result")
		f.provider.refreshDeps.Coordinator = base
		f.provider.fositeStorage.refresh.Coordinator = base
		f.decideAt(committed.LastFreshAt.Add(time.Second))
		requireProhibitedProviderRetry(t, f, context.Background(), f.issued.RefreshToken, "")
	})
}

type providerRetryCiphertextInjector struct {
	ports.RefreshSessionRepository
	alter    func(context.Context, *storage.RefreshSession) ([]byte, error)
	injected bool
}

func (r *providerRetryCiphertextInjector) Save(ctx context.Context, root *storage.RefreshSession) error {
	if !r.injected && root.PreviousSignature != nil && root.RetryCount == 0 && len(root.RetryCiphertext) != 0 {
		ciphertext, err := r.alter(ctx, root)
		if err != nil {
			return err
		}
		root.RetryCiphertext = ciphertext
		r.injected = true
	}
	return r.RefreshSessionRepository.Save(ctx, root)
}

func injectProviderRetryCiphertext(f *providerRefreshFixture, alter func(context.Context, *storage.RefreshSession) ([]byte, error)) *providerRetryCiphertextInjector {
	repo := &providerRetryCiphertextInjector{RefreshSessionRepository: f.provider.refreshDeps.Sessions, alter: alter}
	f.provider.refreshDeps.Sessions = repo
	f.provider.fositeStorage.refresh.Sessions = repo
	return repo
}

func TestProviderRefreshRetry_AuthenticatedCiphertextAndBindings(t *testing.T) {
	t.Run("valid ciphertext holds the original response and authoritative bindings", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		original := rotateProviderRetry(t, f, context.Background(), "read")
		root := providerRetryRoot(t, f)
		require.NotEmpty(t, root.RetryCiphertext)
		payload, err := f.provider.refreshDeps.Encryption.Decrypt(context.Background(), root.RetryCiphertext, encryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))
		for key, expected := range map[string]string{
			"purpose": "refresh_retry", "session_id": root.ID.String(), "original_grant_id": root.OriginalGrantID.String(),
			"original_token_signature": root.OriginalTokenSignature, "principal": root.Principal.String(),
			"agent_id": root.AgentID.String(), "client_id": root.ClientID.String(),
			"predecessor_signature": *root.PreviousSignature, "successor_signature": root.CurrentSignature,
			"original_requested_scope": "read", "scope": original.Scope,
			"original_request_context_fingerprint": *root.OriginalRequestContextFingerprint,
			"token_type":                           original.TokenType,
		} {
			var actual string
			require.NoError(t, json.Unmarshal(fields[key], &actual))
			require.Equal(t, expected, actual)
		}
		require.Equal(t, json.RawMessage("1"), fields["version"])
		for key, expected := range map[string]string{"access_token": original.AccessToken, "refresh_token": original.RefreshToken} {
			var actual string
			require.NoError(t, json.Unmarshal(fields[key], &actual))
			require.True(t, actual == expected, "stored result must match original %s", key)
		}
		for key, expected := range map[string]time.Time{"started_at": root.StartedAt, "access_expires_at": *root.RetryAccessExpiresAt} {
			var actual time.Time
			require.NoError(t, json.Unmarshal(fields[key], &actual))
			require.Equal(t, expected, actual)
		}
		var retryExpiry time.Time
		require.NoError(t, json.Unmarshal(fields["retry_expires_at"], &retryExpiry))
		require.Equal(t, *root.ReuseUntil, retryExpiry)
	})
	t.Run("corrupted encrypted result returns no credentials without consuming successor", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		tamperRejected := false
		repo := injectProviderRetryCiphertext(f, func(ctx context.Context, root *storage.RefreshSession) ([]byte, error) {
			ciphertext := append([]byte(nil), root.RetryCiphertext...)
			ciphertext[len(ciphertext)-1] ^= 1
			_, err := f.provider.refreshDeps.Encryption.Decrypt(ctx, ciphertext, encryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
			tamperRejected = err != nil
			return ciphertext, nil
		})
		rotated := rotateProviderRetry(t, f, context.Background(), "")
		require.True(t, repo.injected, "rotation must store ciphertext through the native repository")
		require.True(t, tamperRejected, "tampering must fail real authenticated decryption")
		f.decideAt(time.Now().UTC())
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		f.decideAt(time.Now().UTC())
		fresh, err := f.provider.HandleRefreshToken(context.Background(), f.root.ClientID.String(), f.secret, rotated.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, fresh.RefreshToken)
	})
	t.Run("ciphertext authenticated for another root cannot be returned", func(t *testing.T) {
		f := newProviderRetryFixture(t)
		proof := false
		repo := injectProviderRetryCiphertext(f, func(ctx context.Context, root *storage.RefreshSession) ([]byte, error) {
			cipher := f.provider.refreshDeps.Encryption
			plain, err := cipher.Decrypt(ctx, root.RetryCiphertext, encryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
			if err != nil {
				return nil, err
			}
			foreignAAD := encryption.NewRefreshSessionBranchKeySubject(id.NewRefreshSessionID()).EncryptionContext()
			altered, err := cipher.Encrypt(ctx, plain, foreignAAD)
			if err != nil {
				return nil, err
			}
			roundTrip, err := cipher.Decrypt(ctx, altered, foreignAAD)
			proof = err == nil && bytes.Equal(plain, roundTrip)
			_, wrongRootErr := cipher.Decrypt(ctx, altered, encryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext())
			proof = proof && wrongRootErr != nil
			return altered, nil
		})
		rotateProviderRetry(t, f, context.Background(), "")
		require.True(t, repo.injected)
		require.True(t, proof, "foreign AAD must authenticate with its own root")
		f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
	})
	for _, tc := range []struct {
		name, field string
		value       func(*storage.RefreshSession) any
	}{
		{"unsupported purpose", "purpose", func(*storage.RefreshSession) any { return "other_purpose" }},
		{"unsupported version", "version", func(*storage.RefreshSession) any { return 2 }},
		{"wrong session", "session_id", func(*storage.RefreshSession) any { return id.NewRefreshSessionID().String() }},
		{"wrong origin time", "started_at", func(root *storage.RefreshSession) any { return root.StartedAt.Add(-time.Second) }},
		{"wrong origin grant", "original_grant_id", func(*storage.RefreshSession) any { return id.NewGrantID().String() }},
		{"wrong original signature", "original_token_signature", func(*storage.RefreshSession) any { return strings.Repeat("a", 64) }},
		{"wrong principal", "principal", func(*storage.RefreshSession) any { return "other@example.com" }},
		{"wrong agent", "agent_id", func(*storage.RefreshSession) any { return id.NewAgentID().String() }},
		{"wrong client", "client_id", func(*storage.RefreshSession) any { return "different-client" }},
		{"wrong predecessor", "predecessor_signature", func(root *storage.RefreshSession) any { return root.CurrentSignature }},
		{"wrong successor", "successor_signature", func(root *storage.RefreshSession) any { return *root.PreviousSignature }},
		{"wrong requested scope", "original_requested_scope", func(*storage.RefreshSession) any { return "write" }},
		{"wrong response scope", "scope", func(*storage.RefreshSession) any { return "read" }},
		{"wrong audit binding", "original_request_context_fingerprint", func(*storage.RefreshSession) any { return strings.Repeat("f", 64) }},
		{"wrong token type", "token_type", func(*storage.RefreshSession) any { return "Other" }},
		{"wrong access token", "access_token", func(*storage.RefreshSession) any { return "different-access-token" }},
		{"wrong response token", "refresh_token", func(*storage.RefreshSession) any { return "different-opaque-token" }},
		{"wrong signed expiry", "access_expires_at", func(root *storage.RefreshSession) any { return root.RetryAccessExpiresAt.Add(time.Second) }},
		{"wrong retry deadline", "retry_expires_at", func(root *storage.RefreshSession) any { return root.RetryExpiresAt.Add(time.Second) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newProviderRetryFixture(t)
			repo := injectProviderRetryCiphertext(f, func(ctx context.Context, root *storage.RefreshSession) ([]byte, error) {
				cipher := f.provider.refreshDeps.Encryption
				aad := encryption.NewRefreshSessionBranchKeySubject(root.ID).EncryptionContext()
				plain, err := cipher.Decrypt(ctx, root.RetryCiphertext, aad)
				if err != nil {
					return nil, err
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(plain, &payload); err != nil {
					return nil, err
				}
				if _, ok := payload[tc.field]; !ok {
					return nil, errors.New("missing required retry payload binding")
				}
				payload[tc.field], err = json.Marshal(tc.value(root))
				if err != nil {
					return nil, err
				}
				changed, err := json.Marshal(payload)
				if err != nil {
					return nil, err
				}
				return cipher.Encrypt(ctx, changed, aad)
			})
			rotateProviderRetry(t, f, context.Background(), "")
			require.True(t, repo.injected, "binding test requires a real encrypted rotation")
			f.decideAt(providerRetryRoot(t, f).LastFreshAt.Add(time.Second))
			f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		})
	}
}
