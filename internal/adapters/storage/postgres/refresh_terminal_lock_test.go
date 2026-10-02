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

func TestPGRefreshTerminalTransitionDoesNotDeadlockOldConsumption(t *testing.T) {
	for _, operation := range []string{"session revocation", "principal revocation", "agent revocation", "terminal save"} {
		t.Run(operation, func(t *testing.T) {
			adapter, cleanup := setupMigratedAdapter(t)
			defer cleanup()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
			neighbor, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("neighbor@example.test"))
			old, err := adapter.db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = old.Rollback() }()
			_, err = old.ExecContext(ctx, `SET LOCAL deadlock_timeout = '100ms'`)
			require.NoError(t, err)
			_, err = old.ExecContext(ctx, `UPDATE refresh_token_sessions SET used_at = clock_timestamp()
				WHERE signature = $1 AND used_at IS NULL`, first.Signature)
			require.NoError(t, err)

			backend := make(chan int, 1)
			completed := make(chan error, 1)
			go func() {
				completed <- NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(owner context.Context, at time.Time) error {
					var pid int
					if err := adapter.storageExecutor(owner).QueryRowxContext(owner, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
						return err
					}
					backend <- pid
					repo := NewRefreshSessionRepo(adapter)
					switch operation {
					case "session revocation":
						return repo.RevokeByID(owner, root.ID, at, storage.RefreshReasonGrantDeleted)
					case "principal revocation":
						return repo.RevokeByPrincipalAndAgent(owner, root.Principal, root.AgentID, at, storage.RefreshReasonGrantDeleted)
					case "agent revocation":
						return repo.RevokeByAgent(owner, root.AgentID, at, storage.RefreshReasonAgentDeleted)
					default:
						changed, err := repo.FindByID(owner, root.ID)
						if err != nil {
							return err
						}
						reason := storage.RefreshReasonInactivityExpiry
						changed.TerminalReason, changed.ExpiredAt = &reason, &at
						changed.RetryCiphertext = nil
						return repo.Save(owner, changed)
					}
				})
			}()
			var pid int
			select {
			case pid = <-backend:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Eventually(t, func() bool {
				var blocked bool
				err := adapter.db.GetContext(ctx, &blocked, `SELECT EXISTS (
					SELECT 1 FROM pg_stat_activity WHERE pid = $1 AND wait_event_type = 'Lock')`, pid)
				return err == nil && blocked
			}, 2*time.Second, time.Millisecond, "terminal transition must wait on the held legacy row before old commit")
			require.NoError(t, old.Commit(), "old consumption and terminal transition must both acknowledge commit")
			select {
			case err := <-completed:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			ended, err := NewRefreshSessionRepo(adapter).FindByID(ctx, root.ID)
			require.NoError(t, err)
			require.NotNil(t, ended.TerminalReason)
			var fenced, receipt bool
			require.NoError(t, adapter.db.GetContext(ctx, &fenced, `SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`, first.Signature))
			require.True(t, fenced)
			require.NoError(t, adapter.db.GetContext(ctx, &receipt, `SELECT EXISTS (SELECT 1 FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = $2)`, root.ID, *ended.TerminalReason))
			require.True(t, receipt)
			require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, neighbor.AgentID, func(owner context.Context, _ time.Time) error {
				return NewRefreshTokenRepo(adapter).CheckCurrentLineage(owner, neighbor.ID)
			}))
		})
	}
}
