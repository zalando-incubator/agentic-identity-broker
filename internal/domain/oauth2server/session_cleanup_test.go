package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/require"
)

type cleanupControlledClock struct {
	at      *time.Time
	failure error
}

func (c *cleanupControlledClock) Now(context.Context) (time.Time, error) {
	if c.failure != nil {
		return time.Time{}, c.failure
	}
	return *c.at, nil
}

// synctest advances this shared clock with the worker's virtual time.
type cleanupAdvancingClock struct {
	from    time.Time
	started time.Time
}

func (c cleanupAdvancingClock) Now(context.Context) (time.Time, error) {
	return c.from.Add(time.Since(c.started)), nil
}

type cleanupClockCoordinator struct {
	base  ports.AuthorizationSessionCoordinator
	clock ports.AuthorizationClock
}

func (c cleanupClockCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	return c.base.Run(ctx, agentID, func(scoped context.Context, _ time.Time) error {
		at, err := c.clock.Now(scoped)
		if err != nil {
			return err
		}
		return operation(scoped, at)
	})
}

type cleanupFaultMaintenance struct {
	ports.RefreshSessionMaintenanceRepository
	failure          error
	retentionFailure error
}

func (m *cleanupFaultMaintenance) ListAgentIDs(ctx context.Context, after id.AgentID, limit int) ([]id.AgentID, error) {
	if m.failure != nil {
		return nil, m.failure
	}
	return m.RefreshSessionMaintenanceRepository.ListAgentIDs(ctx, after, limit)
}

func (m *cleanupFaultMaintenance) ListActive(ctx context.Context, after id.RefreshSessionID, limit int) ([]*storage.RefreshSession, error) {
	if m.failure != nil {
		return nil, m.failure
	}
	return m.RefreshSessionMaintenanceRepository.ListActive(ctx, after, limit)
}

func (m *cleanupFaultMaintenance) ListDue(ctx context.Context, at time.Time, limit int) ([]*storage.RefreshSession, error) {
	if m.failure != nil {
		return nil, m.failure
	}
	return m.RefreshSessionMaintenanceRepository.ListDue(ctx, at, limit)
}

func (m *cleanupFaultMaintenance) DeleteTerminal(ctx context.Context, before time.Time, limit int) (int, error) {
	if m.retentionFailure != nil {
		return 0, m.retentionFailure
	}
	return m.RefreshSessionMaintenanceRepository.DeleteTerminal(ctx, before, limit)
}

type cleanupFaultSessions struct {
	ports.RefreshSessionRepository
	failure error
}

func (s *cleanupFaultSessions) Save(ctx context.Context, root *storage.RefreshSession) error {
	if s.failure != nil {
		return s.failure
	}
	return s.RefreshSessionRepository.Save(ctx, root)
}

type cleanupBlockingSessions struct {
	ports.RefreshSessionRepository
	started chan struct{}
}

func (s *cleanupBlockingSessions) Save(ctx context.Context, _ *storage.RefreshSession) error {
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

// A native root and its first token come from PKCE and the real repositories.
// Backdating only the shared issuance decision allows a controlled rotation at
// StartedAt + 1s without advancing wall time or bypassing native Save checks.
func newCleanupUnrotatedFixture(t *testing.T) *providerRefreshFixture {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.RedirectURIs = []string{providerRefreshRedirect}
	agent.AllowedScopes = []string{"offline_access", "read", "write"}
	require.NoError(t, agents.Update(context.Background(), agent))
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent, secret: secret}
	f.decideAt(time.Now().UTC().Add(-2 * time.Second))
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	return f
}

func newCleanupShortLifetimeFixture(t *testing.T, inactivity time.Duration) *providerRefreshFixture {
	t.Helper()
	provider, agents, _ := newTestProvider(t)
	agent, _, secret := setupTestCredentials(t, provider, agents)
	agent.RedirectURIs = []string{providerRefreshRedirect}
	agent.AllowedScopes = []string{"offline_access", "read", "write"}
	require.NoError(t, agents.Update(context.Background(), agent))
	provider.refreshDeps.Policy.ReuseInterval = time.Second
	provider.refreshDeps.Policy.InactivityLifetime = inactivity
	provider.fositeStorage.refresh.Policy = provider.refreshDeps.Policy
	provider.config.RefreshTokenLifespan = inactivity
	f := &providerRefreshFixture{provider: provider, agents: agents, agent: agent, secret: secret}
	f.decideAt(time.Now().UTC().Add(-2 * time.Second))
	issueProviderRefresh(t, f, agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	return f
}

func rotateCleanupSession(t *testing.T, f *providerRefreshFixture, accessTTL time.Duration) (*storage.RefreshSession, *ports.TokenResponse) {
	t.Helper()
	if accessTTL > 0 {
		f.provider.config.AccessTokenLifespan = accessTTL
		f.provider.accessStrategy.tokenTTL = accessTTL
	}
	f.decideAt(f.root.StartedAt.Add(time.Second))
	response, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
	if err != nil {
		t.Fatal("native refresh rotation failed")
	}
	require.NotNil(t, response)
	root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
	require.NoError(t, err)
	require.NotNil(t, root.PreviousSignature)
	require.NotNil(t, root.PreviousConsumedAt)
	require.NotNil(t, root.ReuseUntil)
	require.NotNil(t, root.RetryAccessExpiresAt)
	require.NotNil(t, root.RetryExpiresAt)
	require.NotEmpty(t, root.RetryCiphertext, "fresh rotation must retain an encrypted result")
	f.root = root
	return root, response
}

func newCleanupRefreshFixture(t *testing.T, accessTTL time.Duration) (*providerRefreshFixture, *storage.RefreshSession, *ports.TokenResponse) {
	t.Helper()
	f := newCleanupUnrotatedFixture(t)
	encryption := testutil.NewTestEncryptionAdapter(t)
	f.provider.refreshDeps.Encryption = encryption
	f.provider.fositeStorage.refresh.Encryption = encryption
	f.provider.refreshDeps.Policy.ReuseInterval = 30 * time.Second
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
	root, response := rotateCleanupSession(t, f, accessTTL)
	return f, root, response
}

type cleanupControl struct{}

func (cleanupControl) TryAcquireLease(ctx context.Context, _ time.Duration) (bool, error) {
	return ctx.Err() == nil, ctx.Err()
}

func (cleanupControl) DatabaseID(context.Context) (string, error) {
	return "", errors.New("ephemeral test store")
}

func newCleanupWorker(f *providerRefreshFixture, now *time.Time, logger *slog.Logger) *SessionCleanup {
	refresh := f.provider.refreshDeps
	refresh.Coordinator = &providerRefreshCoordinator{base: refresh.Coordinator, at: now}
	refresh.Clock = &cleanupControlledClock{at: now}
	return NewSessionCleanup(f.provider.fositeStorage.codeRepo, f.provider.fositeStorage.pkceRepo, refresh, f.provider.refreshDeps.Sessions.(ports.RefreshSessionMaintenanceRepository), cleanupControl{}, logger)
}

func cleanupRoot(t *testing.T, f *providerRefreshFixture, sessionID id.RefreshSessionID) *storage.RefreshSession {
	t.Helper()
	root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), sessionID)
	require.NoError(t, err)
	return root
}

func requireCleanupLineage(t *testing.T, f *providerRefreshFixture, before, after *storage.RefreshSession) {
	t.Helper()
	require.Equal(t, before.ID, after.ID)
	require.NotNil(t, after.PreviousSignature)
	require.NotNil(t, after.PreviousConsumedAt)
	require.NotNil(t, after.ReuseUntil)
	require.NotNil(t, after.RetryAccessExpiresAt)
	require.NotNil(t, after.RetryExpiresAt)
	require.Equal(t, before.CurrentSignature, after.CurrentSignature)
	require.Equal(t, *before.PreviousSignature, *after.PreviousSignature)
	require.Equal(t, *before.PreviousConsumedAt, *after.PreviousConsumedAt)
	require.Equal(t, *before.ReuseUntil, *after.ReuseUntil)
	require.Equal(t, *before.RetryAccessExpiresAt, *after.RetryAccessExpiresAt)
	require.Equal(t, before.RetryCount, after.RetryCount)
	require.Nil(t, after.TerminalReason)
	previous, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), *before.PreviousSignature)
	require.NoError(t, err)
	require.NotNil(t, previous.UsedAt)
	require.Equal(t, *before.PreviousConsumedAt, *previous.UsedAt)
	successor, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), before.CurrentSignature)
	require.NoError(t, err)
	require.Nil(t, successor.UsedAt)
}

func TestSessionCleanup_ExpiredCachedAccessDeniesWithoutRequestCleanup(t *testing.T) {
	// The signed access expiry is genuinely earlier than the fixed reuse window.
	// A denied request must not do maintenance's stateful erasure.
	f, before, _ := newCleanupRefreshFixture(t, 5*time.Second)
	require.True(t, before.RetryAccessExpiresAt.Before(*before.ReuseUntil))
	require.Equal(t, *before.RetryAccessExpiresAt, *before.RetryExpiresAt)
	at := *before.RetryAccessExpiresAt
	require.True(t, at.Before(before.InactivityExpiresAt))
	f.decideAt(at)
	response, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
	if response != nil {
		t.Error("expired cached access returned a token pair")
	}
	if !errors.Is(err, ErrInvalidGrant) {
		t.Error("expired cached access did not deny with invalid_grant")
	}
	after := cleanupRoot(t, f, before.ID)
	requireCleanupLineage(t, f, before, after)
	require.Equal(t, *before.RetryExpiresAt, *after.RetryExpiresAt)
	require.True(t, bytes.Equal(before.RetryCiphertext, after.RetryCiphertext), "request denial must not erase the retry result")
}

func TestSessionCleanup_SweepErasesOverdueCiphertextAndKeepsReplayEvidence(t *testing.T) {
	f, before, _ := newCleanupRefreshFixture(t, 0)
	now := before.RetryExpiresAt.Add(time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	require.Error(t, cleanup.HealthCheck(context.Background()), "startup cannot be ready before reconciliation")
	cleanup.sweep(context.Background())
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	after := cleanupRoot(t, f, before.ID)
	require.Empty(t, after.RetryCiphertext)
	requireCleanupLineage(t, f, before, after)
	require.Equal(t, *before.RetryExpiresAt, *after.RetryExpiresAt)
}

func TestSessionCleanup_RunErasesWithinOneSecondOfDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, before, _ := newCleanupRefreshFixture(t, 0)
		now := *before.PreviousConsumedAt
		cleanup := newCleanupWorker(f, &now, slog.Default())
		clock := cleanupAdvancingClock{from: now, started: time.Now()}
		cleanup.refresh.Clock = clock
		cleanup.refresh.Coordinator = cleanupClockCoordinator{base: f.provider.refreshDeps.Coordinator, clock: clock}
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			cleanup.Run(ctx)
		}()
		defer func() {
			cancel()
			<-finished
		}()
		synctest.Wait()
		require.NoError(t, cleanup.HealthCheck(ctx))
		time.Sleep(before.RetryExpiresAt.Sub(now) + time.Second)
		synctest.Wait()
		after := cleanupRoot(t, f, before.ID)
		require.Empty(t, after.RetryCiphertext, "worker must erase by the one-second deadline")
		requireCleanupLineage(t, f, before, after)
		require.NoError(t, cleanup.HealthCheck(ctx))
	})
}

func TestSessionCleanup_StorageFailureLosesReadinessUntilActualRecovery(t *testing.T) {
	for _, faulty := range []string{"clock", "maintenance", "sessions"} {
		t.Run(faulty, func(t *testing.T) {
			f, before, original := newCleanupRefreshFixture(t, 0)
			now := before.PreviousConsumedAt.Add(time.Second)
			var logs bytes.Buffer
			cleanup := newCleanupWorker(f, &now, slog.New(slog.NewJSONHandler(&logs, nil)))
			clock := cleanup.refresh.Clock.(*cleanupControlledClock)
			maintenance := &cleanupFaultMaintenance{RefreshSessionMaintenanceRepository: cleanup.maintenance}
			cleanup.maintenance = maintenance
			sessions := &cleanupFaultSessions{RefreshSessionRepository: cleanup.refresh.Sessions}
			cleanup.refresh.Sessions = sessions
			cleanup.sweep(context.Background())
			require.NoError(t, cleanup.HealthCheck(context.Background()))
			stillLive := cleanupRoot(t, f, before.ID)
			require.True(t, bytes.Equal(before.RetryCiphertext, stillLive.RetryCiphertext))

			now = before.RetryExpiresAt.Add(time.Second)
			fault := errors.New("storage failure with sensitive-token-value")
			switch faulty {
			case "clock":
				clock.failure = fault
			case "maintenance":
				maintenance.failure = fault
			case "sessions":
				sessions.failure = fault
			}
			cleanup.sweep(context.Background())
			require.Error(t, cleanup.HealthCheck(context.Background()), "an unresolved cleanup fault cannot report readiness")
			stillLive = cleanupRoot(t, f, before.ID)
			require.True(t, bytes.Equal(before.RetryCiphertext, stillLive.RetryCiphertext), "failed sweep must not claim erasure")
			if logs.Len() == 0 {
				t.Error("cleanup storage failure produced no operator warning")
			}
			warning := logs.String()
			if strings.Contains(warning, "sensitive-token-value") || strings.Contains(warning, f.issued.RefreshToken) || strings.Contains(warning, original.AccessToken) || strings.Contains(warning, original.RefreshToken) {
				t.Error("cleanup warning exposed a credential")
			}

			clock.failure, maintenance.failure, sessions.failure = nil, nil, nil
			cleanup.sweep(context.Background())
			require.NoError(t, cleanup.HealthCheck(context.Background()))
			after := cleanupRoot(t, f, before.ID)
			require.Empty(t, after.RetryCiphertext)
			requireCleanupLineage(t, f, before, after)
		})
	}
}

func TestSessionCleanup_StartupShortensReuseWithoutRewritingSealedDeadline(t *testing.T) {
	f, before, original := newCleanupRefreshFixture(t, 0)
	now := before.PreviousConsumedAt.Add(2 * time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	cleanup.refresh.Policy.ReuseInterval = 8 * time.Second
	cleanup.sweep(context.Background())
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	shortened := cleanupRoot(t, f, before.ID)
	require.NotNil(t, shortened.RetryExpiresAt)
	bound := before.PreviousConsumedAt.Add(8 * time.Second)
	require.Equal(t, bound, *shortened.RetryExpiresAt)
	require.Equal(t, *before.ReuseUntil, *shortened.ReuseUntil)
	require.True(t, bytes.Equal(before.RetryCiphertext, shortened.RetryCiphertext), "policy reconciliation rewrote the sealed result")
	plaintext, err := cleanup.refresh.Encryption.Decrypt(context.Background(), shortened.RetryCiphertext, domainencryption.NewRefreshSessionBranchKeySubject(shortened.ID).EncryptionContext())
	require.NoError(t, err)
	var sealed struct {
		RetryExpiresAt time.Time `json:"retry_expires_at"`
	}
	require.NoError(t, json.Unmarshal(plaintext, &sealed))
	require.Equal(t, earliestRefreshDeadline(*before.ReuseUntil, *before.RetryAccessExpiresAt), sealed.RetryExpiresAt)

	// This is the same stored result, not another fresh rotation. The original
	// sealed expiry is later than the shorter effective bound.
	f.provider.refreshDeps.Policy.ReuseInterval = 8 * time.Second
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
	now = before.PreviousConsumedAt.Add(7 * time.Second)
	f.decideAt(now)
	retry, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, f.issued.RefreshToken, "")
	if err != nil || retry == nil {
		t.Fatal("eligible retry failed before the shortened effective deadline")
	}
	require.True(t, retry.AccessToken == original.AccessToken, "eligible retry returned a different access token")
	require.True(t, retry.RefreshToken == original.RefreshToken, "eligible retry returned a different refresh token")
	require.Equal(t, 1, cleanupRoot(t, f, before.ID).RetryCount)

	cleanup.refresh.Policy.ReuseInterval = time.Minute
	cleanup.sweep(context.Background())
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	afterIncrease := cleanupRoot(t, f, before.ID)
	require.Equal(t, bound, *afterIncrease.RetryExpiresAt, "later policy increase must not extend a predecessor")
	require.True(t, bytes.Equal(before.RetryCiphertext, afterIncrease.RetryCiphertext), "later policy increase must not rewrite the sealed result")
}

func TestSessionCleanup_ZeroReuseErasesEveryLiveResultBeforeReadiness(t *testing.T) {
	f := newCleanupUnrotatedFixture(t)
	other := &providerRefreshFixture{provider: f.provider, agents: f.agents, agent: f.agent, secret: f.secret}
	issueProviderRefresh(t, other, f.agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	encryption := testutil.NewTestEncryptionAdapter(t)
	f.provider.refreshDeps.Encryption = encryption
	f.provider.fositeStorage.refresh.Encryption = encryption
	f.provider.refreshDeps.Policy.ReuseInterval = 30 * time.Second
	f.provider.fositeStorage.refresh.Policy = f.provider.refreshDeps.Policy
	first, _ := rotateCleanupSession(t, f, 0)
	second, _ := rotateCleanupSession(t, other, 0)
	now := second.PreviousConsumedAt.Add(time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	cleanup.refresh.Policy.ReuseInterval = 0
	require.Error(t, cleanup.HealthCheck(context.Background()))
	cleanup.sweep(context.Background())
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	for _, before := range []*storage.RefreshSession{first, second} {
		after := cleanupRoot(t, f, before.ID)
		require.Empty(t, after.RetryCiphertext)
		require.Equal(t, *before.PreviousConsumedAt, *after.RetryExpiresAt)
		requireCleanupLineage(t, f, before, after)
	}
	cleanup.refresh.Policy.ReuseInterval = time.Minute
	cleanup.sweep(context.Background())
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	for _, before := range []*storage.RefreshSession{first, second} {
		require.Equal(t, *before.PreviousConsumedAt, *cleanupRoot(t, f, before.ID).RetryExpiresAt)
	}
}

func TestSessionCleanup_CancellationLeavesUnreconciledCiphertextUnchanged(t *testing.T) {
	f, before, _ := newCleanupRefreshFixture(t, 0)
	now := before.RetryExpiresAt.Add(time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	started := make(chan struct{})
	cleanup.refresh.Sessions = &cleanupBlockingSessions{RefreshSessionRepository: cleanup.refresh.Sessions, started: started}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		cleanup.Run(ctx)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		cancel()
		<-finished
		t.Fatal("native ciphertext cleanup never started")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("native cleanup did not stop after cancellation")
	}
	after := cleanupRoot(t, f, before.ID)
	require.True(t, bytes.Equal(before.RetryCiphertext, after.RetryCiphertext), "cancellation must not run subsequent cleanup")
	require.Error(t, cleanup.HealthCheck(context.Background()), "an unreconciled startup must not report readiness")
}

func TestSessionCleanup_StartupReconcilesEveryActiveRoot(t *testing.T) {
	f, rotated, _ := newCleanupRefreshFixture(t, 0)
	unrotated := &providerRefreshFixture{provider: f.provider, agents: f.agents, agent: f.agent, secret: f.secret}
	f.decideAt(rotated.LastFreshAt.Add(time.Second))
	issueProviderRefresh(t, unrotated, f.agent.ID.String(), providerRefreshRedirect, "offline_access read write")
	require.Nil(t, unrotated.root.PreviousSignature)
	require.Empty(t, unrotated.root.RetryCiphertext)

	now := unrotated.root.StartedAt.Add(time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	cleanup.refresh.Policy.ReuseInterval = 5 * time.Second
	cleanup.refresh.Policy.InactivityLifetime = 10 * time.Second
	cleanup.refresh.Policy.AbsoluteLifetime = 20 * time.Second
	require.NoError(t, cleanup.Reconcile(context.Background()))
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	for _, before := range []*storage.RefreshSession{rotated, unrotated.root} {
		after := cleanupRoot(t, f, before.ID)
		current, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), before.CurrentSignature)
		require.NoError(t, err)
		require.NotNil(t, after.AbsoluteExpiresAt, "startup must persist the current absolute deadline")
		require.Equal(t, before.StartedAt.Add(20*time.Second), *after.AbsoluteExpiresAt)
		require.Equal(t, earliestRefreshDeadline(before.InactivityExpiresAt, before.LastFreshAt.Add(10*time.Second), current.ExpiresAt), after.InactivityExpiresAt)
		require.Equal(t, before.StartedAt, after.StartedAt)
		require.Equal(t, before.LastFreshAt, after.LastFreshAt)
		require.Equal(t, before.RetainUntil, after.RetainUntil)
		require.Equal(t, current.ExpiresAt, before.InactivityExpiresAt, "issuance must retain the original token expiry")
		require.Nil(t, after.TerminalReason)
		if before.PreviousSignature == nil {
			require.Nil(t, after.PreviousSignature)
			require.Nil(t, after.RetryExpiresAt)
		} else {
			require.Equal(t, before.RetryCiphertext, after.RetryCiphertext)
			require.NotNil(t, after.RetryExpiresAt)
			require.Equal(t, earliestRefreshDeadline(*before.RetryExpiresAt, before.PreviousConsumedAt.Add(5*time.Second), after.InactivityExpiresAt, *after.AbsoluteExpiresAt), *after.RetryExpiresAt)
		}
	}
}

func TestSessionCleanup_StartupRejectsElapsedStoredDeadlineAfterPolicyIncrease(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reason    storage.RefreshRevocationReason
		unlimited bool
	}{
		{"absolute", storage.RefreshReasonAbsoluteExpiry, false},
		{"absolute zero", storage.RefreshReasonAbsoluteExpiry, true},
		{"inactivity", storage.RefreshReasonInactivityExpiry, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCleanupUnrotatedFixture(t)
			now := f.root.StartedAt.Add(2 * time.Second)
			cleanup := newCleanupWorker(f, &now, slog.Default())
			if tc.reason == storage.RefreshReasonAbsoluteExpiry {
				cleanup.refresh.Policy.AbsoluteLifetime = 5 * time.Second
			} else {
				cleanup.refresh.Policy.InactivityLifetime = 5 * time.Second
			}
			require.NoError(t, cleanup.Reconcile(context.Background()))
			shortened := cleanupRoot(t, f, f.root.ID)
			if tc.reason == storage.RefreshReasonAbsoluteExpiry {
				require.NotNil(t, shortened.AbsoluteExpiresAt)
				require.Equal(t, f.root.StartedAt.Add(5*time.Second), *shortened.AbsoluteExpiresAt)
				if tc.unlimited {
					cleanup.refresh.Policy.AbsoluteLifetime = 0
				} else {
					cleanup.refresh.Policy.AbsoluteLifetime = 60 * time.Second
				}
			} else {
				require.Equal(t, f.root.StartedAt.Add(5*time.Second), shortened.InactivityExpiresAt)
				cleanup.refresh.Policy.InactivityLifetime = time.Hour
			}
			now = f.root.StartedAt.Add(5 * time.Second)
			require.NoError(t, cleanup.Reconcile(context.Background()))
			require.NoError(t, cleanup.HealthCheck(context.Background()))
			expired := cleanupRoot(t, f, f.root.ID)
			require.NotNil(t, expired.TerminalReason, "stored elapsed deadline must terminalize before readiness")
			require.NotNil(t, expired.ExpiredAt)
			require.Equal(t, tc.reason, *expired.TerminalReason)
			require.Equal(t, now, *expired.ExpiredAt)
			require.Equal(t, shortened.InactivityExpiresAt, expired.InactivityExpiresAt)
			if shortened.AbsoluteExpiresAt != nil {
				require.NotNil(t, expired.AbsoluteExpiresAt)
				require.Equal(t, *shortened.AbsoluteExpiresAt, *expired.AbsoluteExpiresAt)
			}
		})
	}
}

func TestSessionCleanup_StartupRecomputesUnexpiredAbsoluteWithoutExtendingRetry(t *testing.T) {
	f, before, _ := newCleanupRefreshFixture(t, 0)
	now := before.StartedAt.Add(3 * time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	cleanup.refresh.Policy.ReuseInterval = 5 * time.Second
	cleanup.refresh.Policy.AbsoluteLifetime = 12 * time.Second
	require.NoError(t, cleanup.Reconcile(context.Background()))
	shortened := cleanupRoot(t, f, before.ID)
	require.NotNil(t, shortened.AbsoluteExpiresAt)
	require.NotNil(t, shortened.RetryExpiresAt)
	require.Equal(t, before.StartedAt.Add(12*time.Second), *shortened.AbsoluteExpiresAt)
	require.Equal(t, before.PreviousConsumedAt.Add(5*time.Second), *shortened.RetryExpiresAt)
	cleanup.refresh.Policy.AbsoluteLifetime = 40 * time.Second
	now = before.StartedAt.Add(4 * time.Second)
	require.NoError(t, cleanup.Reconcile(context.Background()))
	after := cleanupRoot(t, f, before.ID)
	require.NotNil(t, after.AbsoluteExpiresAt)
	require.NotNil(t, after.RetryExpiresAt)
	require.Equal(t, before.StartedAt.Add(40*time.Second), *after.AbsoluteExpiresAt)
	require.Equal(t, *shortened.RetryExpiresAt, *after.RetryExpiresAt, "the consumed predecessor cannot regain retry eligibility")
	require.Equal(t, shortened.RetryCiphertext, after.RetryCiphertext, "sealed original expiry must not be rewritten")
	require.Equal(t, before.RetryAccessExpiresAt, after.RetryAccessExpiresAt)
	require.Equal(t, before.StartedAt, after.StartedAt)
	require.Equal(t, before.LastFreshAt, after.LastFreshAt)
	current, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), after.CurrentSignature)
	require.NoError(t, err)
	require.Equal(t, before.InactivityExpiresAt, current.ExpiresAt, "absolute changes cannot extend the issued token")
	cleanup.refresh.Policy.AbsoluteLifetime = 0
	now = before.StartedAt.Add(5 * time.Second)
	require.NoError(t, cleanup.Reconcile(context.Background()))
	unlimited := cleanupRoot(t, f, before.ID)
	require.Nil(t, unlimited.AbsoluteExpiresAt, "zero removes an unexpired absolute deadline")
	require.NotNil(t, unlimited.RetryExpiresAt)
	require.Equal(t, *shortened.RetryExpiresAt, *unlimited.RetryExpiresAt, "zero cannot restore a consumed predecessor's retry window")
	require.Equal(t, before.InactivityExpiresAt, unlimited.InactivityExpiresAt)
	require.Equal(t, current.ExpiresAt, before.InactivityExpiresAt)
}

func TestSessionCleanup_StartupExpiresShortenedLifetimeAndErasesLiveResult(t *testing.T) {
	f, before, _ := newCleanupRefreshFixture(t, 0)
	now := before.StartedAt.Add(6 * time.Second)
	cleanup := newCleanupWorker(f, &now, slog.Default())
	cleanup.refresh.Policy.ReuseInterval = 2 * time.Second
	cleanup.refresh.Policy.AbsoluteLifetime = 5 * time.Second
	require.NoError(t, cleanup.Reconcile(context.Background()))
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	after := cleanupRoot(t, f, before.ID)
	require.NotNil(t, after.TerminalReason, "effective expiry must terminalize before readiness")
	require.NotNil(t, after.ExpiredAt)
	require.Equal(t, storage.RefreshReasonAbsoluteExpiry, *after.TerminalReason)
	require.Equal(t, now, *after.ExpiredAt)
	require.Empty(t, after.RetryCiphertext)
	require.Equal(t, before.RetainUntil, after.RetainUntil)
	require.Equal(t, before.CurrentSignature, after.CurrentSignature)
}

func TestSessionCleanup_RunExpiresUnrotatedRootWithoutCiphertext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newCleanupShortLifetimeFixture(t, 4*time.Second)
		start := time.Now()
		cleanup := newCleanupWorker(f, &start, slog.Default())
		cleanup.refresh.Policy.InactivityLifetime = 3 * time.Second
		clock := cleanupAdvancingClock{from: start.UTC(), started: start}
		cleanup.refresh.Clock = clock
		cleanup.refresh.Coordinator = cleanupClockCoordinator{base: f.provider.refreshDeps.Coordinator, clock: clock}
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			cleanup.Run(ctx)
		}()
		defer func() {
			cancel()
			<-finished
		}()
		synctest.Wait()
		active := cleanupRoot(t, f, f.root.ID)
		require.NoError(t, cleanup.HealthCheck(ctx))
		require.Nil(t, active.PreviousSignature)
		require.Empty(t, active.RetryCiphertext)
		require.Nil(t, active.TerminalReason)
		require.Equal(t, f.root.StartedAt.Add(3*time.Second), active.InactivityExpiresAt)

		time.Sleep(active.InactivityExpiresAt.Sub(start) + 300*time.Millisecond)
		synctest.Wait()
		expired := cleanupRoot(t, f, f.root.ID)
		require.NotNil(t, expired.TerminalReason, "live maintenance must expire the unrotated root")
		require.Equal(t, storage.RefreshReasonInactivityExpiry, *expired.TerminalReason)
		require.NotNil(t, expired.ExpiredAt)
		require.Empty(t, expired.RetryCiphertext)
		require.Equal(t, f.root.RetainUntil, expired.RetainUntil)
		require.NoError(t, cleanup.HealthCheck(ctx))
		token, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, active.CurrentSignature)
		require.NoError(t, err, "terminal evidence must survive until the issued token expires")
		require.Nil(t, token.UsedAt)
	})
}

func TestSessionCleanup_ActiveRootRetainsEveryConsumedToken(t *testing.T) {
	f, first, result := newCleanupRefreshFixture(t, 0)
	f.decideAt(first.LastFreshAt.Add(time.Second))
	second, err := f.provider.HandleRefreshToken(context.Background(), f.agent.ID.String(), f.secret, result.RefreshToken, "")
	require.NoError(t, err)
	require.NotNil(t, second)
	current := cleanupRoot(t, f, first.ID)
	require.Nil(t, current.TerminalReason)
	require.NotEqual(t, current.CurrentSignature, first.CurrentSignature)
	now := time.Now().UTC()
	cleanup := newCleanupWorker(f, &now, slog.Default())
	require.NoError(t, cleanup.Reconcile(context.Background()))
	require.NoError(t, cleanup.HealthCheck(context.Background()))
	require.Equal(t, current.CurrentSignature, cleanupRoot(t, f, first.ID).CurrentSignature)
	for _, signature := range []string{first.OriginalTokenSignature, first.CurrentSignature, current.CurrentSignature} {
		token, err := f.provider.refreshDeps.Tokens.FindBySignature(context.Background(), signature)
		require.NoError(t, err, "active lineage must retain every predecessor")
		require.Equal(t, first.ID, token.SessionID)
	}
}

func TestSessionCleanup_RetentionFailureBlocksReadinessAndRecoversAtEquality(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newCleanupShortLifetimeFixture(t, 4*time.Second)
		start := time.Now()
		cleanup := newCleanupWorker(f, &start, slog.Default())
		cleanup.refresh.Policy.InactivityLifetime = 3 * time.Second
		clock := cleanupAdvancingClock{from: start.UTC(), started: start}
		cleanup.refresh.Clock = clock
		cleanup.refresh.Coordinator = cleanupClockCoordinator{base: f.provider.refreshDeps.Coordinator, clock: clock}
		maintenance := &cleanupFaultMaintenance{RefreshSessionMaintenanceRepository: cleanup.maintenance}
		cleanup.maintenance = maintenance
		ctx := context.Background()
		require.NoError(t, cleanup.Reconcile(ctx))
		shortened := cleanupRoot(t, f, f.root.ID)
		require.NoError(t, cleanup.HealthCheck(ctx))
		time.Sleep(time.Until(shortened.InactivityExpiresAt) + 100*time.Millisecond)
		synctest.Wait()
		maintenance.retentionFailure = errors.New("retention backend unavailable")
		require.Error(t, cleanup.Reconcile(ctx))
		require.Error(t, cleanup.HealthCheck(ctx))
		terminal := cleanupRoot(t, f, f.root.ID)
		require.NotNil(t, terminal.TerminalReason, "expiry commits before retention cleanup is attempted")
		require.Equal(t, storage.RefreshReasonInactivityExpiry, *terminal.TerminalReason)
		_, err := f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
		require.NoError(t, err)
		maintenance.retentionFailure = nil
		require.NoError(t, cleanup.Reconcile(ctx))
		require.NoError(t, cleanup.HealthCheck(ctx))
		_, err = f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
		require.NoError(t, err, "terminal history must remain before RetainUntil")
		time.Sleep(time.Until(terminal.RetainUntil))
		synctest.Wait()
		require.NoError(t, cleanup.Reconcile(ctx))
		require.NoError(t, cleanup.HealthCheck(ctx))
		_, err = f.provider.refreshDeps.Sessions.FindByID(ctx, f.root.ID)
		require.True(t, ports.IsNotFoundErr(err), "retention equality must purge the terminal root")
		_, err = f.provider.refreshDeps.Tokens.FindBySignature(ctx, f.root.CurrentSignature)
		require.True(t, ports.IsNotFoundErr(err), "retention equality must purge the token history")
	})
}

type cleanupReadinessSessions struct {
	ports.RefreshSessionRepository
	cleanup *SessionCleanup
	t       *testing.T
}

func (s cleanupReadinessSessions) Save(ctx context.Context, root *storage.RefreshSession) error {
	require.NoError(s.t, s.cleanup.HealthCheck(ctx), "ordinary erasure must not withdraw readiness before a successful commit")
	return s.RefreshSessionRepository.Save(ctx, root)
}

type cleanupCountingCoordinator struct {
	ports.AuthorizationSessionCoordinator
	calls int
}

func (c *cleanupCountingCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	c.calls++
	return c.AuthorizationSessionCoordinator.Run(ctx, agentID, operation)
}

func TestSessionCleanup_RoutineErasurePreservesReadinessDuringCommit(t *testing.T) {
	f, root, _ := newCleanupRefreshFixture(t, 0)
	now := *root.PreviousConsumedAt
	cleanup := newCleanupWorker(f, &now, slog.Default())
	require.NoError(t, cleanup.Reconcile(t.Context()))
	cleanup.refresh.Sessions = cleanupReadinessSessions{RefreshSessionRepository: cleanup.refresh.Sessions, cleanup: cleanup, t: t}
	now = *root.RetryExpiresAt
	cleanup.dueDelay(t.Context())
	require.Empty(t, cleanupRoot(t, f, root.ID).RetryCiphertext)
	require.NoError(t, cleanup.HealthCheck(t.Context()))
}

func TestSessionCleanup_UnchangedActiveRootsDoNotAcquireAgentGates(t *testing.T) {
	f, root, _ := newCleanupRefreshFixture(t, 0)
	now := *root.PreviousConsumedAt
	cleanup := newCleanupWorker(f, &now, slog.Default())
	coordinator := &cleanupCountingCoordinator{AuthorizationSessionCoordinator: cleanup.refresh.Coordinator}
	cleanup.refresh.Coordinator = coordinator
	require.NoError(t, cleanup.Reconcile(t.Context()))
	require.Equal(t, 0, coordinator.calls, "an unchanged active root requires no owner transaction")
}

func TestSessionCleanup_DueRecoveryDoesNotRequireFullActiveScan(t *testing.T) {
	f, root, _ := newCleanupRefreshFixture(t, 0)
	now := *root.PreviousConsumedAt
	cleanup := newCleanupWorker(f, &now, slog.Default())
	require.NoError(t, cleanup.Reconcile(t.Context()))
	clock := cleanup.refresh.Clock.(*cleanupControlledClock)
	clock.failure = errors.New("transient clock outage")
	cleanup.dueDelay(t.Context())
	require.Error(t, cleanup.HealthCheck(t.Context()))
	clock.failure = nil
	now = *root.RetryExpiresAt
	cleanup.dueDelay(t.Context())
	require.Empty(t, cleanupRoot(t, f, root.ID).RetryCiphertext)
	require.NoError(t, cleanup.HealthCheck(t.Context()), "due work alone must recover readiness")
}

type cleanupHorizonMaintenance struct {
	ports.RefreshSessionMaintenanceRepository
	now  *time.Time
	root *storage.RefreshSession
}

func (m cleanupHorizonMaintenance) ListDue(ctx context.Context, at time.Time, limit int) ([]*storage.RefreshSession, error) {
	if at.After(*m.now) {
		deadline := m.now.Add(200 * time.Millisecond)
		consumed := deadline.Add(-30 * time.Second)
		page := make([]*storage.RefreshSession, limit)
		for i := range page {
			future := *m.root
			future.ID = id.RefreshSessionID{15: byte(i + 1)}
			future.BranchKeyID = "refresh_" + future.ID.String() + "_branch_key"
			future.LastFreshAt = consumed
			future.PreviousConsumedAt = &consumed
			future.ReuseUntil, future.RetryExpiresAt = &deadline, &deadline
			page[i] = &future
		}
		return page, nil
	}
	return m.RefreshSessionMaintenanceRepository.ListDue(ctx, at, limit)
}

func TestSessionCleanup_FutureHorizonCannotHideAnOverdueResult(t *testing.T) {
	f, root, _ := newCleanupRefreshFixture(t, 0)
	now := *root.PreviousConsumedAt
	cleanup := newCleanupWorker(f, &now, slog.Default())
	require.NoError(t, cleanup.Reconcile(t.Context()))
	cleanup.maintenance = cleanupHorizonMaintenance{RefreshSessionMaintenanceRepository: cleanup.maintenance, now: &now, root: root}
	now = *root.RetryExpiresAt
	cleanup.dueDelay(t.Context())
	require.Empty(t, cleanupRoot(t, f, root.ID).RetryCiphertext, "future candidates must not starve already-due erasure")
	require.NoError(t, cleanup.HealthCheck(t.Context()))
}

type cleanupSlowRetention struct {
	ports.RefreshSessionMaintenanceRepository
}

func (cleanupSlowRetention) DeleteTerminal(ctx context.Context, _ time.Time, _ int) (int, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestSessionCleanup_OptionalRetentionTimeoutDoesNotWithdrawReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f, root, _ := newCleanupRefreshFixture(t, 0)
		now := *root.PreviousConsumedAt
		cleanup := newCleanupWorker(f, &now, slog.Default())
		require.NoError(t, cleanup.Reconcile(t.Context()))
		cleanup.maintenance = cleanupSlowRetention{RefreshSessionMaintenanceRepository: cleanup.maintenance}
		cleanup.nextPrune = time.Time{}
		cleanup.dueDelay(t.Context())
		require.NoError(t, cleanup.HealthCheck(t.Context()), "retention backlog is not overdue live retry authority")
		require.True(t, bytes.Equal(root.RetryCiphertext, cleanupRoot(t, f, root.ID).RetryCiphertext))
	})
}
