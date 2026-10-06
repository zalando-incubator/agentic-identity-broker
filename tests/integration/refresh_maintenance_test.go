//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestRefreshMaintenanceLeaseSerializesInstancesAndTransfersAfterExpiry(t *testing.T) {
	first, db := newRolloutStorage(t)
	second, err := storageadapter.NewAdapter(&ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: restoreConnectionURL(t, db)}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close(context.Background())) })
	ctx := t.Context()
	acquired, err := first.RefreshMaintenanceControl().TryAcquireLease(ctx, 5*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = second.RefreshMaintenanceControl().TryAcquireLease(ctx, 5*time.Second)
	require.NoError(t, err)
	require.False(t, acquired, "another instance cannot maintain the same database concurrently")
	_, err = db.ExecContext(ctx, "UPDATE refresh_maintenance SET lease_until = clock_timestamp()")
	require.NoError(t, err)
	acquired, err = second.RefreshMaintenanceControl().TryAcquireLease(ctx, 5*time.Second)
	require.NoError(t, err)
	require.True(t, acquired, "a stopped leader's lease must be recoverable")
	acquired, err = first.RefreshMaintenanceControl().TryAcquireLease(ctx, 5*time.Second)
	require.NoError(t, err)
	require.False(t, acquired, "the former leader cannot renew after another owner takes over")
}

func TestRefreshMaintenancePurgesExpiredRootlessLegacyButRetainsNativeHistory(t *testing.T) {
	store, db := newRolloutStorage(t)
	ctx := t.Context()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	root := createRolloutRoot(t, store, db, now.Add(-time.Second))
	expired := refreshContractSignature("expired-rootless-" + root.ID.String())
	live := refreshContractSignature("live-rootless-" + root.ID.String())
	for signature, expiresAt := range map[string]time.Time{expired: now, live: now.Add(time.Hour)} {
		_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, expires_at)
			VALUES ($1, 'pre-feature', $2, $3, $4, $5)`, signature, root.AgentID, root.ClientID, root.Principal, expiresAt)
		require.NoError(t, err)
	}
	count, err := store.RefreshMaintenance().DeleteTerminal(ctx, now, 200)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	var remains bool
	require.NoError(t, db.GetContext(ctx, &remains, "SELECT EXISTS (SELECT 1 FROM refresh_token_sessions WHERE signature = $1)", expired))
	require.False(t, remains, "expired legacy identity data must be purged at equality")
	for _, signature := range []string{live, root.CurrentSignature} {
		require.NoError(t, db.GetContext(ctx, &remains, "SELECT EXISTS (SELECT 1 FROM refresh_token_sessions WHERE signature = $1)", signature))
		require.True(t, remains, "live legacy and native lineage must be retained")
	}
}
