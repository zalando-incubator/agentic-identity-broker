package oauth2server

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

// Issue through the native provider after applying the resolved local policy.
// The retry result uses the real authenticated encryption adapter.
func newProviderInactivityFixture(t *testing.T, lifetime time.Duration) *providerRefreshFixture {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.RedirectURIs = []string{providerRefreshRedirect}
	agent.AllowedScopes = []string{"offline_access", "read", "write"}
	require.NoError(t, agents.Update(context.Background(), agent))
	cipher := testutil.NewTestEncryptionAdapter(t)
	provider.refreshDeps.Encryption = cipher
	provider.fositeStorage.refresh.Encryption = cipher
	provider.refreshDeps.Policy = storage.RefreshSessionPolicy{ReuseInterval: 30 * time.Second, InactivityLifetime: lifetime}
	provider.fositeStorage.refresh.Policy = provider.refreshDeps.Policy
	provider.config.RefreshTokenLifespan = lifetime
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent, secret: secret}
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	return f
}

func setProviderInactivityLifetime(f *providerRefreshFixture, lifetime time.Duration) {
	f.provider.refreshDeps.Policy.InactivityLifetime = lifetime
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
	f.provider.config.RefreshTokenLifespan = lifetime
}

func TestRefreshInactivity_InitialIssuanceExpiresAtResolvedLifetime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lifetime time.Duration
	}{
		{name: "default thirty days", lifetime: 720 * time.Hour},
		{name: "configured seventeen minutes", lifetime: 17 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newProviderInactivityFixture(t, tc.lifetime)
				ctx := context.Background()
				initial, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
				require.NoError(t, err)
				deadline := f.root.StartedAt.Add(tc.lifetime)
				require.Equal(t, f.root.StartedAt, f.root.LastFreshAt)
				require.Equal(t, f.root.StartedAt, initial.IssuedAt)
				require.Equal(t, deadline, f.root.InactivityExpiresAt)
				require.Equal(t, deadline, initial.ExpiresAt)
				require.Equal(t, initial.ExpiresAt, f.root.RetainUntil)
				advanceRefreshAbsoluteTime(t, deadline)
				f.decideAt(deadline)
				f.requireDenial(t, f.agent.ID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
			})
		})
	}
}

func TestRefreshInactivity_OnlyFreshRotationsRenewTheInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 12*time.Minute)
		ctx := context.Background()
		start := f.root.StartedAt
		firstAt := start.Add(9 * time.Minute)
		advanceRefreshAbsoluteTime(t, firstAt)
		f.decideAt(firstAt)
		first := rotateProviderRetry(t, f, ctx, "")
		beforeRetry := providerRetryRoot(t, f)
		require.Equal(t, firstAt, beforeRetry.LastFreshAt)
		require.Equal(t, firstAt.Add(12*time.Minute), beforeRetry.InactivityExpiresAt)
		require.Equal(t, beforeRetry.InactivityExpiresAt, beforeRetry.RetainUntil)
		require.Equal(t, firstAt.Add(30*time.Second), *beforeRetry.ReuseUntil)
		require.NotEmpty(t, beforeRetry.RetryCiphertext)
		successor, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, beforeRetry.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, beforeRetry.InactivityExpiresAt, successor.ExpiresAt)

		retryAt := firstAt.Add(5 * time.Second)
		advanceRefreshAbsoluteTime(t, retryAt)
		f.decideAt(retryAt)
		recovered, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		requireOriginalRetryResult(t, first, recovered)
		requireRetryOnlyCountChanged(t, f, beforeRetry, 1)
		require.Equal(t, *beforeRetry.ReuseUntil, *providerRetryRoot(t, f).ReuseUntil)

		// The original initial-issuance deadline no longer applies after the fresh rotation.
		secondAt := start.Add(12*time.Minute + time.Second)
		advanceRefreshAbsoluteTime(t, secondAt)
		f.decideAt(secondAt)
		second, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, first.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, second.RefreshToken)
		updated := providerRetryRoot(t, f)
		require.Equal(t, secondAt, updated.LastFreshAt)
		require.Equal(t, secondAt.Add(12*time.Minute), updated.InactivityExpiresAt)
		require.Equal(t, updated.InactivityExpiresAt, updated.RetainUntil)
	})
}

func TestRefreshInactivity_ResourceUseAndTransientFailuresDoNotCountAsActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 16*time.Minute)
		ctx := context.Background()
		start := f.root.StartedAt
		keys, err := f.provider.accessStrategy.signingKeyService.BuildJWKS(ctx)
		require.NoError(t, err)
		accessAt := start.Add(5 * time.Minute)
		advanceRefreshAbsoluteTime(t, accessAt)
		access, err := jwt.Parse([]byte(f.issued.AccessToken), jwt.WithKeySet(keys))
		require.NoError(t, err, "resource-side signature validation must not be a refresh event")
		accessExpiry, ok := access.Expiration()
		require.True(t, ok)
		require.True(t, accessExpiry.After(f.root.InactivityExpiresAt))

		f.decideAt(accessAt)
		verifier := f.observeConsent()
		verifier.fault = errors.New("transient grant lookup failure")
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.Equal(t, start, providerRetryRoot(t, f).LastFreshAt)
		verifier.fault = nil
		deadline := f.root.InactivityExpiresAt
		advanceRefreshAbsoluteTime(t, deadline)
		f.decideAt(deadline)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})
}

func TestRefreshInactivity_ShorterCurrentPolicyDeniesAtEquality(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 40*time.Minute)
		ctx := context.Background()
		original, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
		require.NoError(t, err)
		setProviderInactivityLifetime(f, 10*time.Minute)
		deadline := f.root.LastFreshAt.Add(10 * time.Minute)
		require.True(t, deadline.Before(f.root.InactivityExpiresAt))
		require.True(t, deadline.Before(original.ExpiresAt))
		advanceRefreshAbsoluteTime(t, deadline)
		f.decideAt(deadline)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
		unchanged, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, original.Signature)
		require.NoError(t, err)
		require.Equal(t, original, unchanged)
	})
}

func TestRefreshInactivity_IncreaseAffectsOnlyFutureFreshRotations(t *testing.T) {
	for _, rotateBeforeOldDeadline := range []bool{false, true} {
		name := "unrotated token expires at its original deadline"
		if rotateBeforeOldDeadline {
			name = "valid rotation applies the longer lifetime to its successor"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newProviderInactivityFixture(t, 11*time.Minute)
				ctx := context.Background()
				start := f.root.StartedAt
				oldDeadline := start.Add(11 * time.Minute)
				original, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
				require.NoError(t, err)
				setProviderInactivityLifetime(f, 35*time.Minute)
				require.Equal(t, oldDeadline, providerRetryRoot(t, f).InactivityExpiresAt)
				require.Equal(t, oldDeadline, original.ExpiresAt)
				if !rotateBeforeOldDeadline {
					advanceRefreshAbsoluteTime(t, oldDeadline)
					f.decideAt(oldDeadline)
					f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
					return
				}

				at := start.Add(10 * time.Minute)
				advanceRefreshAbsoluteTime(t, at)
				f.decideAt(at)
				rotated := rotateProviderRetry(t, f, ctx, "")
				require.NotEmpty(t, rotated.RefreshToken)
				updated := providerRetryRoot(t, f)
				require.Equal(t, at, updated.LastFreshAt)
				require.Equal(t, at.Add(35*time.Minute), updated.InactivityExpiresAt)
				require.Equal(t, updated.InactivityExpiresAt, updated.RetainUntil)
				successor, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, updated.CurrentSignature)
				require.NoError(t, err)
				require.Equal(t, updated.InactivityExpiresAt, successor.ExpiresAt)
				old, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, original.Signature)
				require.NoError(t, err)
				require.Equal(t, original.ExpiresAt, old.ExpiresAt)
			})
		})
	}
}

func TestRefreshInactivity_RetentionCoversOriginalAfterPolicyShortening(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 2*time.Hour)
		ctx := context.Background()
		original, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
		require.NoError(t, err)
		setProviderInactivityLifetime(f, 8*time.Minute)
		at := f.root.StartedAt.Add(5 * time.Minute)
		advanceRefreshAbsoluteTime(t, at)
		f.decideAt(at)
		rotated := rotateProviderRetry(t, f, ctx, "")
		updated := providerRetryRoot(t, f)
		require.Equal(t, at.Add(8*time.Minute), updated.InactivityExpiresAt)
		require.Equal(t, original.ExpiresAt, updated.RetainUntil, "original token remains replay evidence through its issued expiry")
		successor, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, updated.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, updated.InactivityExpiresAt, successor.ExpiresAt)
		advanceRefreshAbsoluteTime(t, successor.ExpiresAt)
		f.decideAt(successor.ExpiresAt)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, rotated.RefreshToken, "", "invalid_grant")
	})
}

func TestRefreshInactivity_MaintainedExpiryCannotBeRevived(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 20*time.Minute)
		now := f.root.StartedAt.Add(time.Minute)
		advanceRefreshAbsoluteTime(t, now)
		cleanup := newCleanupWorker(f, &now, slog.Default())
		cleanup.refresh.Policy.InactivityLifetime = 9 * time.Minute
		require.NoError(t, cleanup.Reconcile(context.Background()))
		deadline := f.root.StartedAt.Add(9 * time.Minute)
		require.Equal(t, deadline, providerRetryRoot(t, f).InactivityExpiresAt)
		advanceRefreshAbsoluteTime(t, deadline)
		now = deadline
		cleanup.refresh.Policy.InactivityLifetime = time.Hour
		require.NoError(t, cleanup.Reconcile(context.Background()))
		expired := providerRetryRoot(t, f)
		require.NotNil(t, expired.TerminalReason)
		require.Equal(t, storage.RefreshReasonInactivityExpiry, *expired.TerminalReason)
		require.Equal(t, f.root.RetainUntil, expired.RetainUntil)
		setProviderInactivityLifetime(f, time.Hour)
		f.decideAt(deadline)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
	})
}

type inactivityRecheckFaultClock struct {
	at    time.Time
	calls int
}

func (c *inactivityRecheckFaultClock) Now(context.Context) (time.Time, error) {
	c.calls++
	if c.calls > 1 {
		return time.Time{}, errors.New("shared clock temporarily unavailable")
	}
	return c.at, nil
}

func TestRefreshInactivity_ClockFailureAfterStagingCannotAdvanceActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderInactivityFixture(t, 20*time.Minute)
		ctx := context.Background()
		at := f.root.StartedAt.Add(5 * time.Minute)
		advanceRefreshAbsoluteTime(t, at)
		f.decideAt(at)
		clock := &inactivityRecheckFaultClock{at: at}
		f.provider.refreshDeps.Clock = clock
		f.provider.fositeStorage.refresh.Clock = clock
		f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "server_error")
		require.Equal(t, 2, clock.calls, "the error must occur after a candidate rotation was staged")
		recoveredAt := at.Add(time.Minute)
		advanceRefreshAbsoluteTime(t, recoveredAt)
		f.decideAt(recoveredAt)
		recovered := rotateProviderRetry(t, f, ctx, "")
		require.NotEmpty(t, recovered.RefreshToken)
		updated := providerRetryRoot(t, f)
		require.Equal(t, recoveredAt, updated.LastFreshAt)
		require.Equal(t, recoveredAt.Add(20*time.Minute), updated.InactivityExpiresAt)
	})
}
