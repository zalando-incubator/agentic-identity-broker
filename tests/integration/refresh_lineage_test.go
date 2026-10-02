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

func TestRefreshRollout_RejectsIncompleteIntermediateLineage(t *testing.T) {
	for _, scenario := range []struct{ name, mutation string }{
		{"missing native ancestor", `DELETE FROM refresh_tokens WHERE signature = $1`},
		{"different intermediate owner", `UPDATE refresh_token_sessions SET principal = 'other@example.test' WHERE signature = $1`},
		{"broken intermediate ancestry", `UPDATE refresh_token_sessions SET predecessor_signature = repeat('a', 64) WHERE signature = $1`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, db := newRolloutStorage(t)
			ctx := context.Background()
			now, err := store.AuthorizationClock().Now(ctx)
			require.NoError(t, err)
			root := createRolloutRoot(t, store, db, now)
			middle := refreshContractSignature("middle-" + root.ID.String())
			current := refreshContractSignature("current-" + root.ID.String())
			for _, rotation := range [][2]string{{root.CurrentSignature, middle}, {middle, current}} {
				require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(owner context.Context, at time.Time) error {
					return rotateContractRoot(owner, store.RefreshSessions(), store.RefreshTokens(), root.ID, rotation[0], rotation[1], at)
				}))
			}
			require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(owner context.Context, _ time.Time) error {
				return store.RefreshTokens().CheckCurrentLineage(owner, root.ID)
			}), "complete new-issuer lineage must be usable before mutation")
			before, err := store.RefreshSessions().FindByID(ctx, root.ID)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, scenario.mutation, middle)
			require.NoError(t, err)
			err = store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(owner context.Context, _ time.Time) error {
				return store.RefreshTokens().CheckCurrentLineage(owner, root.ID)
			})
			require.True(t, ports.IsNotFoundErr(err), "unsupported intermediate ancestry cannot grant authority")
			after, err := store.RefreshSessions().FindByID(ctx, root.ID)
			require.NoError(t, err)
			require.Equal(t, before, after, "unsupported lineage rejection must not consume or revoke the family")
			var count int
			require.NoError(t, db.GetContext(ctx, &count, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1 AND used_at IS NULL`, root.ID))
			require.Equal(t, 1, count, "no independent successor may be created")
		})
	}
}

func TestRefreshRollout_OldWriterConsumptionDoesNotBlockExpiry(t *testing.T) {
	store, db := newRolloutStorage(t)
	ctx := context.Background()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	root := createRolloutRoot(t, store, db, now.Add(-2*time.Minute))
	current := refreshContractSignature("maintenance-current-" + root.ID.String())
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(owner context.Context, at time.Time) error {
		return rotateContractRoot(owner, store.RefreshSessions(), store.RefreshTokens(), root.ID, root.CurrentSignature, current, at)
	}))
	_, err = db.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = clock_timestamp() WHERE signature = $1`, current)
	require.NoError(t, err)
	oldOnly := refreshContractSignature("maintenance-old-only-" + root.ID.String())
	_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, oldOnly, root.ID.String(), root.AgentID, root.ClientID, root.Principal, now.Add(time.Hour))
	require.NoError(t, err)
	cleanup := rolloutCleanup(store, storage.RefreshSessionPolicy{ReuseInterval: 30 * time.Second, AbsoluteLifetime: time.Minute, InactivityLifetime: time.Hour})
	require.NoError(t, cleanup.Reconcile(ctx), "unsupported issuance history cannot block terminal maintenance")
	require.NoError(t, cleanup.HealthCheck(ctx))
	expired, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.NotNil(t, expired.ExpiredAt)
	require.Equal(t, storage.RefreshReasonAbsoluteExpiry, *expired.TerminalReason)
	require.Empty(t, expired.RetryCiphertext)
	var usable bool
	require.NoError(t, db.GetContext(ctx, &usable, `SELECT EXISTS (
		SELECT 1 FROM refresh_token_sessions WHERE request_id = $1 AND used_at IS NULL
	)`, root.ID.String()))
	require.False(t, usable, "the old-only descendant must end with its original root")
}
