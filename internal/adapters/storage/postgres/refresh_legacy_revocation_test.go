//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestPGRefreshLifecycleInvalidatesRootlessLegacyAuthority(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	for _, tc := range []struct {
		name      string
		agentWide bool
		reason    storage.RefreshRevocationReason
	}{
		{"grant deletion", false, storage.RefreshReasonGrantDeleted},
		{"expired grant renewal", false, storage.RefreshReasonExpiredGrantRenewal},
		{"credential revocation", true, storage.RefreshReasonCredentialRevoked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			owner, other := createTestAgent(t, adapter), createTestAgent(t, adapter)
			principal, unrelated := id.Principal("legacy-owner@example.test"), id.Principal("legacy-other@example.test")
			signatures := []string{pgRefreshSignature(owner.ID.String() + "owner"), pgRefreshSignature(owner.ID.String() + "other-user"), pgRefreshSignature(other.ID.String())}
			for i, entry := range []struct {
				agent     id.AgentID
				principal id.Principal
			}{{owner.ID, principal}, {owner.ID, unrelated}, {other.ID, principal}} {
				_, err := adapter.db.ExecContext(ctx, `INSERT INTO refresh_token_sessions(signature, request_id, agent_id, client_id, principal, expires_at) VALUES($1,$2,$3,$4,$5,$6)`, signatures[i], id.NewAuthorizationCodeID().String(), entry.agent, entry.agent.String(), entry.principal, time.Now().UTC().Add(time.Hour))
				require.NoError(t, err)
			}
			repo := NewRefreshSessionRepo(adapter)
			var decision time.Time
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.ID, func(scope context.Context, at time.Time) error {
				decision = at
				if tc.agentWide {
					return repo.RevokeByAgent(scope, owner.ID, at, tc.reason)
				}
				return repo.RevokeByPrincipalAndAgent(scope, principal, owner.ID, at, tc.reason)
			}))
			for i, signature := range signatures {
				var row struct {
					UsedAt    *time.Time           `db:"used_at"`
					SessionID *id.RefreshSessionID `db:"session_id"`
				}
				require.NoError(t, adapter.db.GetContext(ctx, &row, `SELECT used_at, session_id FROM refresh_token_sessions WHERE signature=$1`, signature))
				require.Nil(t, row.SessionID, "lifecycle must not fabricate native origin evidence")
				if i == 0 || (i == 1 && tc.agentWide) {
					require.NotNil(t, row.UsedAt, "old-only local authority must not survive lifecycle success")
					require.True(t, row.UsedAt.Equal(decision))
					result, err := adapter.db.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = clock_timestamp() WHERE signature = $1 AND used_at IS NULL`, signature)
					require.NoError(t, err)
					rows, err := result.RowsAffected()
					require.NoError(t, err)
					require.Zero(t, rows, "old binary must not consume an already revoked token")
				} else {
					require.Nil(t, row.UsedAt, "unrelated authority must survive")
				}
			}
		})
	}
}

func TestPGRefreshRootlessLegacyFailureRollsBackRevocation(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	oldOnly := pgRefreshSignature("rootless-rollback-" + root.ID.String())
	_, err := adapter.db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, oldOnly, id.NewAuthorizationCodeID().String(),
		root.AgentID, root.ClientID, root.Principal, first.ExpiresAt)
	require.NoError(t, err)
	_, err = adapter.db.ExecContext(ctx, `CREATE FUNCTION reject_rootless_refresh_transition() RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'simulated old-only bridge failure'; END; $$;
		CREATE TRIGGER reject_rootless_refresh_transition BEFORE UPDATE OF used_at ON refresh_token_sessions
		FOR EACH ROW WHEN (OLD.session_id IS NULL AND NEW.used_at IS NOT NULL)
		EXECUTE FUNCTION reject_rootless_refresh_transition();`)
	require.NoError(t, err)

	err = NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return NewRefreshSessionRepo(adapter).RevokeByPrincipalAndAgent(scope, root.Principal, root.AgentID, at, storage.RefreshReasonGrantDeleted)
	})
	require.Error(t, err, "failed old-only invalidation must abort the complete revocation")
	stored, err := NewRefreshSessionRepo(adapter).FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, stored.TerminalReason)
	token, err := NewRefreshTokenRepo(adapter).FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, token.UsedAt)
	for _, signature := range []string{first.Signature, oldOnly} {
		var usedAt *time.Time
		require.NoError(t, adapter.db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.Nil(t, usedAt, "failed transaction cannot publish a partial legacy invalidation")
	}
	var receipts int
	require.NoError(t, adapter.db.GetContext(ctx, &receipts, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1`, root.ID))
	require.Zero(t, receipts, "rolled-back revocation cannot publish a receipt")
}
