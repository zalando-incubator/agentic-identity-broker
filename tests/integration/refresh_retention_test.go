//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestRefreshRollout_LiveUnrotatedExpiryMirrorFaultAndRecovery(t *testing.T) {
	store, db := newRolloutStorage(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	startedAt := now.UTC().Add(-5 * time.Second)
	root := createRolloutRoot(t, store, db, startedAt)
	oldOnly := refreshContractSignature("unanchored-live-descendant-" + root.ID.String())
	lateExpiry := root.RetainUntil.Add(time.Minute)
	_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, oldOnly, root.ID.String(), root.AgentID, root.ClientID, root.Principal, lateExpiry)
	require.NoError(t, err)
	cleanup := rolloutCleanup(store, storage.RefreshSessionPolicy{ReuseInterval: time.Second, AbsoluteLifetime: 8 * time.Second, InactivityLifetime: time.Hour})
	require.NoError(t, cleanup.Reconcile(ctx))
	require.NoError(t, cleanup.HealthCheck(ctx))
	before, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, before.PreviousSignature)
	require.Empty(t, before.RetryCiphertext)
	require.Nil(t, before.TerminalReason)
	require.NotNil(t, before.AbsoluteExpiresAt)
	require.True(t, time.Now().Before(*before.AbsoluteExpiresAt), "the mirror fault must occur in live maintenance, not startup")
	_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_live_mirror_write() RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'simulated live mirror failure'; END; $$;
		CREATE TRIGGER reject_live_mirror_write BEFORE UPDATE OF used_at ON refresh_token_sessions
		FOR EACH ROW WHEN (OLD.used_at IS NULL) EXECUTE FUNCTION reject_live_mirror_write();`)
	require.NoError(t, err)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		cleanup.Run(ctx)
	}()
	defer func() {
		cancel()
		<-finished
	}()

	deadline := before.AbsoluteExpiresAt.Add(time.Second)
	for cleanup.HealthCheck(ctx) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.Error(t, cleanup.HealthCheck(ctx), "a failed live mirror transition must block admission by the expiry deadline")
	unchanged, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, unchanged.TerminalReason)
	require.Nil(t, unchanged.ExpiredAt)
	require.Equal(t, before.RetainUntil, unchanged.RetainUntil)
	for _, signature := range []string{root.CurrentSignature, oldOnly} {
		var usedAt *time.Time
		require.NoError(t, db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.Nil(t, usedAt, "failed terminal transition cannot consume part of the legacy lineage")
	}
	var receipts int
	require.NoError(t, db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1`, root.ID))
	require.Zero(t, receipts)

	_, err = db.ExecContext(ctx, `DROP TRIGGER reject_live_mirror_write ON refresh_token_sessions; DROP FUNCTION reject_live_mirror_write()`)
	require.NoError(t, err)
	recoveryDeadline := time.Now().Add(2 * time.Second)
	var expired *storage.RefreshSession
	for time.Now().Before(recoveryDeadline) {
		expired, err = store.RefreshSessions().FindByID(ctx, root.ID)
		require.NoError(t, err)
		if expired.TerminalReason != nil && cleanup.HealthCheck(ctx) == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotNil(t, expired.TerminalReason, "maintenance must complete after the mirror recovers")
	require.Equal(t, storage.RefreshReasonAbsoluteExpiry, *expired.TerminalReason)
	require.NotNil(t, expired.ExpiredAt)
	require.Equal(t, lateExpiry, expired.RetainUntil)
	require.NoError(t, cleanup.HealthCheck(ctx))
	for _, signature := range []string{root.CurrentSignature, oldOnly} {
		var usedAt *time.Time
		require.NoError(t, db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.NotNil(t, usedAt)
		require.Equal(t, *expired.ExpiredAt, usedAt.UTC())
	}
	require.NoError(t, db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'absolute_expiry'`, root.ID))
	require.Equal(t, 1, receipts)
}

func TestRefreshRollout_TerminalRetentionPreservesReceiptAndPurgesAllLineage(t *testing.T) {
	store, db := newRolloutStorage(t)
	ctx := context.Background()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	root := createRolloutRoot(t, store, db, now.UTC().Add(-10*time.Second))
	unrelated := createRolloutRoot(t, store, db, now.UTC())
	first, second := root.CurrentSignature, refreshContractSignature("retention-second-"+root.ID.String())
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return rotateContractRoot(scope, store.RefreshSessions(), store.RefreshTokens(), root.ID, first, second, at)
	}))
	third := refreshContractSignature("retention-third-" + root.ID.String())
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return rotateContractRoot(scope, store.RefreshSessions(), store.RefreshTokens(), root.ID, second, third, at)
	}))
	active, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, active.TerminalReason)
	require.Equal(t, second, *active.PreviousSignature)
	require.Equal(t, third, active.CurrentSignature)
	oldOnly := refreshContractSignature("retention-old-only-" + root.ID.String())
	lateExpiry := active.RetainUntil.Add(time.Minute)
	_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, oldOnly, root.ID.String(), root.AgentID, root.ClientID, root.Principal, lateExpiry)
	require.NoError(t, err)
	count, err := store.RefreshMaintenance().DeleteTerminal(ctx, lateExpiry, 1)
	require.NoError(t, err)
	require.Zero(t, count, "active lineage is never purged, even when its issued tokens would all have expired")
	for _, signature := range []string{first, second, third} {
		token, lookupErr := store.RefreshTokens().FindBySignature(ctx, signature)
		require.NoError(t, lookupErr)
		require.Equal(t, root.ID, token.SessionID)
		var requestID string
		require.NoError(t, db.GetContext(ctx, &requestID, `SELECT request_id FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.Equal(t, root.ID.String(), requestID)
	}

	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return store.RefreshRevocations().RevokeByID(scope, root.ID, at, storage.RefreshReasonGrantDeleted)
	}))
	terminal, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Equal(t, lateExpiry, terminal.RetainUntil, "an unanchored descendant extends the whole lineage retention")
	require.Equal(t, storage.RefreshReasonGrantDeleted, *terminal.TerminalReason)
	var oldOnlyUsedAt *time.Time
	require.NoError(t, db.GetContext(ctx, &oldOnlyUsedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1 AND session_id IS NULL`, oldOnly))
	require.NotNil(t, oldOnlyUsedAt)
	var receipts int
	require.NoError(t, db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'grant_deleted'`, root.ID))
	require.Equal(t, 1, receipts)
	count, err = store.RefreshMaintenance().DeleteTerminal(ctx, terminal.RetainUntil.Add(-time.Microsecond), 1)
	require.NoError(t, err)
	require.Zero(t, count, "one microsecond before RetainUntil is still retained")
	for _, signature := range []string{first, second, third, oldOnly} {
		var requestID string
		require.NoError(t, db.GetContext(ctx, &requestID, `SELECT request_id FROM refresh_token_sessions WHERE signature = $1`, signature), "pre-boundary legacy history must remain")
		require.Equal(t, root.ID.String(), requestID)
	}
	_, err = store.RefreshTokens().FindBySignature(ctx, first)
	require.NoError(t, err, "pre-boundary consumed native history must remain")
	count, err = store.RefreshMaintenance().DeleteTerminal(ctx, terminal.RetainUntil, 1)
	require.NoError(t, err)
	require.Equal(t, 1, count, "retention equality must purge one terminal lineage")
	_, err = store.RefreshSessions().FindByID(ctx, root.ID)
	require.True(t, ports.IsNotFoundErr(err))
	for _, signature := range []string{first, second, third} {
		_, err = store.RefreshTokens().FindBySignature(ctx, signature)
		require.True(t, ports.IsNotFoundErr(err), "native token history must follow its purged root")
	}
	for _, signature := range []string{first, second, third, oldOnly} {
		var exists bool
		require.NoError(t, db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM refresh_token_sessions WHERE signature = $1)`, signature))
		require.False(t, exists, "legacy token history must follow its purged root")
	}
	require.NoError(t, db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'grant_deleted'`, root.ID))
	require.Equal(t, 1, receipts, "independent revocation receipt survives root deletion")
	stillActive, err := store.RefreshSessions().FindByID(ctx, unrelated.ID)
	require.NoError(t, err)
	require.Nil(t, stillActive.TerminalReason)
}
