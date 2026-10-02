package oauth2server

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/require"
)

func setProviderAbsoluteLifetime(f *providerRefreshFixture, lifetime time.Duration) {
	f.provider.refreshDeps.Policy.AbsoluteLifetime = lifetime
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
}

func advanceRefreshAbsoluteTime(t *testing.T, at time.Time) {
	t.Helper()
	require.True(t, at.After(time.Now()), "decision time must advance with the in-memory store's clock")
	time.Sleep(time.Until(at))
}

func TestRefreshAbsolute_OriginalStartAndEquality(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderRetryFixture(t)
		ctx := context.Background()
		started := f.root.StartedAt
		deadline := started.Add(40 * time.Minute)
		setProviderAbsoluteLifetime(f, 40*time.Minute)

		advanceRefreshAbsoluteTime(t, started.Add(10*time.Minute))
		f.decideAt(started.Add(10 * time.Minute))
		beforeSigning := time.Now().UTC()
		first := rotateProviderRetry(t, f, ctx, "")
		firstRoot := providerRetryRoot(t, f)
		require.Equal(t, started, firstRoot.StartedAt)
		require.Equal(t, deadline, *firstRoot.AbsoluteExpiresAt)
		require.Equal(t, started.Add(70*time.Minute), firstRoot.InactivityExpiresAt)
		firstToken, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, firstRoot.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, firstRoot.InactivityExpiresAt, firstToken.ExpiresAt, "the successor's immutable expiry is not clipped to the absolute deadline")

		keys, err := f.provider.accessStrategy.signingKeyService.BuildJWKS(ctx)
		require.NoError(t, err)
		access, err := jwt.Parse([]byte(first.AccessToken), jwt.WithKeySet(keys))
		require.NoError(t, err)
		expiresAt, ok := access.Expiration()
		require.True(t, ok)
		require.WithinDuration(t, beforeSigning.Add(time.Hour), expiresAt, 2*time.Second, "the signed access JWT keeps its original TTL")
		require.True(t, expiresAt.After(deadline), "the absolute refresh deadline cannot shorten an issued access JWT")

		advanceRefreshAbsoluteTime(t, started.Add(25*time.Minute))
		f.decideAt(started.Add(25 * time.Minute))
		second, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, first.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, second.RefreshToken)
		secondRoot := providerRetryRoot(t, f)
		require.Equal(t, started, secondRoot.StartedAt)
		require.Equal(t, deadline, *secondRoot.AbsoluteExpiresAt)

		advanceRefreshAbsoluteTime(t, deadline.Add(-time.Nanosecond))
		f.decideAt(deadline.Add(-time.Nanosecond))
		third, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, second.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, third.RefreshToken)
		beforeDenial := providerRetryRoot(t, f)
		require.Equal(t, deadline, *beforeDenial.AbsoluteExpiresAt)
		require.NotEmpty(t, beforeDenial.RetryCiphertext)
		require.Zero(t, beforeDenial.RetryCount)
		advanceRefreshAbsoluteTime(t, deadline)
		f.decideAt(deadline)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, third.RefreshToken, "", "invalid_grant")
		f.requireDenial(t, f.root.ClientID.String(), f.secret, second.RefreshToken, "", "invalid_grant")
		advanceRefreshAbsoluteTime(t, deadline.Add(time.Second))
		f.decideAt(deadline.Add(time.Second))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, third.RefreshToken, "", "invalid_grant")
		accessAfter, err := jwt.Parse([]byte(first.AccessToken), jwt.WithKeySet(keys))
		require.NoError(t, err, "refresh denial cannot revoke or rewrite a previously signed access JWT")
		expiryAfter, ok := accessAfter.Expiration()
		require.True(t, ok)
		require.Equal(t, expiresAt, expiryAfter)
	})
}

func TestRefreshAbsolute_ZeroRemainsUnlimitedBeyondThirtyDays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderRefreshFixture(t)
		ctx := context.Background()
		started := f.root.StartedAt
		require.Zero(t, f.provider.refreshDeps.Policy.AbsoluteLifetime)
		require.Nil(t, f.root.AbsoluteExpiresAt)
		f.provider.refreshDeps.Policy.InactivityLifetime = 720 * time.Hour
		f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy

		advanceRefreshAbsoluteTime(t, started.Add(30*time.Minute))
		f.decideAt(started.Add(30 * time.Minute))
		first, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, first.RefreshToken)
		time.Sleep(29 * 24 * time.Hour)
		f.decideAt(time.Now().UTC())
		second, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, first.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, second.RefreshToken)
		time.Sleep(6 * 24 * time.Hour)
		f.decideAt(time.Now().UTC())
		third, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, second.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, third.RefreshToken)
		root := providerRetryRoot(t, f)
		require.Equal(t, started, root.StartedAt)
		require.Nil(t, root.AbsoluteExpiresAt)
		require.True(t, root.LastFreshAt.After(started.Add(30*24*time.Hour)))
		current, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, root.CurrentSignature)
		require.NoError(t, err)
		require.Equal(t, root.InactivityExpiresAt, current.ExpiresAt)
	})
}

func TestRefreshAbsolute_ShorterDeadlineSurvivesLongerAndZeroPolicies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderRetryFixture(t)
		ctx := context.Background()
		started := f.root.StartedAt
		setProviderAbsoluteLifetime(f, 50*time.Minute)
		advanceRefreshAbsoluteTime(t, started.Add(5*time.Minute))
		f.decideAt(started.Add(5 * time.Minute))
		first := rotateProviderRetry(t, f, ctx, "")
		require.Equal(t, started.Add(50*time.Minute), *providerRetryRoot(t, f).AbsoluteExpiresAt)

		setProviderAbsoluteLifetime(f, 30*time.Minute)
		advanceRefreshAbsoluteTime(t, started.Add(15*time.Minute))
		f.decideAt(started.Add(15 * time.Minute))
		second, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, first.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, second.RefreshToken)
		require.Equal(t, started.Add(30*time.Minute), *providerRetryRoot(t, f).AbsoluteExpiresAt)

		setProviderAbsoluteLifetime(f, 2*time.Hour)
		advanceRefreshAbsoluteTime(t, started.Add(20*time.Minute))
		f.decideAt(started.Add(20 * time.Minute))
		third, err := f.provider.HandleRefreshToken(ctx, f.root.ClientID.String(), f.secret, second.RefreshToken, "")
		require.NoError(t, err)
		require.NotEmpty(t, third.RefreshToken)
		before := providerRetryRoot(t, f)
		require.Equal(t, started, before.StartedAt)
		require.Equal(t, started.Add(30*time.Minute), *before.AbsoluteExpiresAt)
		require.NotEmpty(t, before.RetryCiphertext)

		advanceRefreshAbsoluteTime(t, *before.AbsoluteExpiresAt)
		f.decideAt(*before.AbsoluteExpiresAt)
		f.requireDenial(t, f.root.ClientID.String(), f.secret, third.RefreshToken, "", "invalid_grant")
		setProviderAbsoluteLifetime(f, 0)
		advanceRefreshAbsoluteTime(t, started.Add(31*time.Minute))
		f.decideAt(started.Add(31 * time.Minute))
		f.requireDenial(t, f.root.ClientID.String(), f.secret, third.RefreshToken, "", "invalid_grant")
	})
}

func TestRefreshAbsolute_StoredShorterInactivitySurvivesPolicyIncrease(t *testing.T) {
	for _, tc := range []struct {
		name          string
		rotatesInTime bool
	}{
		{name: "elapsed stored deadline denies without extending the current token"},
		{name: "fresh rotation before stored deadline applies the longer lifetime", rotatesInTime: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newProviderRetryFixture(t)
				ctx := context.Background()
				started := f.root.StartedAt
				shorter := started.Add(25 * time.Minute)
				root := providerRetryRoot(t, f)
				root.InactivityExpiresAt = shorter
				require.NoError(t, f.provider.refreshDeps.Sessions.Save(ctx, root), "persist the effective shorter activity deadline before policy increases")
				original, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, root.CurrentSignature)
				require.NoError(t, err)
				require.Equal(t, started.Add(time.Hour), original.ExpiresAt, "a policy change cannot rewrite an issued refresh token")
				f.provider.refreshDeps.Policy.InactivityLifetime = 2 * time.Hour
				f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
				require.Equal(t, shorter, providerRetryRoot(t, f).InactivityExpiresAt)
				if !tc.rotatesInTime {
					advanceRefreshAbsoluteTime(t, shorter)
					f.decideAt(shorter)
					f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
					unchanged, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, original.Signature)
					require.NoError(t, err)
					require.Equal(t, original, unchanged)
					return
				}
				at := started.Add(20 * time.Minute)
				advanceRefreshAbsoluteTime(t, at)
				f.decideAt(at)
				next := rotateProviderRetry(t, f, ctx, "")
				require.NotEmpty(t, next.RefreshToken)
				updated := providerRetryRoot(t, f)
				require.Equal(t, started, updated.StartedAt)
				require.Equal(t, at.Add(2*time.Hour), updated.InactivityExpiresAt)
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

type absoluteRecheckClock struct {
	times [2]time.Time
	calls int
}

func (c *absoluteRecheckClock) Now(context.Context) (time.Time, error) {
	if c.calls >= len(c.times) {
		return time.Time{}, errors.New("unexpected shared-clock read")
	}
	at := c.times[c.calls]
	c.calls++
	return at, nil
}

func TestRefreshAbsolute_CommitRechecksOriginalActivityAndCurrentToken(t *testing.T) {
	for _, tc := range []struct {
		name     string
		firstAt  time.Duration
		deadline time.Duration
		shorten  bool
	}{
		{name: "stored shorter inactivity elapses during a staged rotation", firstAt: 20 * time.Minute, deadline: 25 * time.Minute, shorten: true},
		{name: "the originally issued current token expires during a staged rotation", firstAt: 59 * time.Minute, deadline: time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newProviderRetryFixture(t)
				ctx := context.Background()
				started := f.root.StartedAt
				if tc.shorten {
					root := providerRetryRoot(t, f)
					root.InactivityExpiresAt = started.Add(tc.deadline)
					require.NoError(t, f.provider.refreshDeps.Sessions.Save(ctx, root))
				}
				f.provider.refreshDeps.Policy.InactivityLifetime = 2 * time.Hour
				f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
				advanceRefreshAbsoluteTime(t, started.Add(tc.firstAt))
				f.decideAt(started.Add(tc.firstAt))
				clock := &absoluteRecheckClock{times: [2]time.Time{started.Add(tc.firstAt), started.Add(tc.deadline)}}
				f.provider.refreshDeps.Clock = clock
				f.provider.fositeStorage.refresh.Clock = clock
				f.requireDenial(t, f.root.ClientID.String(), f.secret, f.issued.RefreshToken, "", "invalid_grant")
				require.Equal(t, 2, clock.calls, "the first time must permit staging; the second must deny before commit")
			})
		})
	}
}

func TestRefreshAbsolute_CommitRechecksFiniteOriginalStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProviderRetryFixture(t)
		started := f.root.StartedAt
		setProviderAbsoluteLifetime(f, 35*time.Minute)
		advanceRefreshAbsoluteTime(t, started.Add(5*time.Minute))
		f.decideAt(started.Add(5 * time.Minute))
		first := rotateProviderRetry(t, f, context.Background(), "")
		before := providerRetryRoot(t, f)
		require.Equal(t, started.Add(35*time.Minute), *before.AbsoluteExpiresAt)
		require.NotEmpty(t, before.RetryCiphertext)
		advanceRefreshAbsoluteTime(t, started.Add(30*time.Minute))
		f.decideAt(started.Add(30 * time.Minute))
		clock := &absoluteRecheckClock{times: [2]time.Time{started.Add(30 * time.Minute), *before.AbsoluteExpiresAt}}
		f.provider.refreshDeps.Clock = clock
		f.provider.fositeStorage.refresh.Clock = clock
		f.requireDenial(t, f.root.ClientID.String(), f.secret, first.RefreshToken, "", "invalid_grant")
		require.Equal(t, 2, clock.calls, "a staged successor must be rejected when the absolute deadline arrives before commit")
	})
}
