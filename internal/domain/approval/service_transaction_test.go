package approval

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestApprovalNotificationsFollowOwningCommit(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		commit bool
	}{{"commit publishes the decision", true}, {"rollback keeps the pending decision", false}} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			transactions := memory.NewTransactionManager()
			repo := memory.NewToolApprovalRepository(transactions)
			syncState := memory.NewApprovalSyncStateRepository(transactions)
			broadcaster := NewApprovalSyncBroadcaster(0)
			registry, err := ledger.NewRegistry(eventschemas.Schemas)
			require.NoError(t, err)
			events := memory.NewBusinessEventRepository(transactions, registry)
			recorder := ledger.NewService(registry, events, nil, transactions, false)
			updates := broadcaster.Subscribe()
			t.Cleanup(func() { broadcaster.Unsubscribe(updates) })
			service := NewService(repo, repo, repo, syncState, nil, nil, broadcaster, time.Minute,
				"https://broker.example", slog.New(slog.NewTextHandler(io.Discard, nil)), recorder)
			principal := id.NewPrincipal("transaction-notification-principal")
			now := time.Now().UTC()
			pending := &storage.ToolApproval{
				ID: id.NewApprovalID(), Principal: principal, AgentID: id.NewAgentID(),
				ToolName: "read", ToolPattern: "read", ArgumentsHash: "notification-arguments",
				Arguments: map[string]any{}, Status: storage.ApprovalStatusPending,
				CreatedAt: now, ExpiresAt: now.Add(time.Hour),
			}
			_, err = repo.Create(ctx, pending)
			require.NoError(t, err)
			owner, err := transactions.BeginTX(ctx)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, transactions.Rollback(owner)) })
			decision, err := service.DenyApproval(owner, pending.ID, principal, nil)
			require.NoError(t, err)
			require.Equal(t, storage.ApprovalStatusDenied, decision.Status)
			version, err := syncState.GetVersion(owner)
			require.NoError(t, err)
			require.Equal(t, int64(1), version)
			select {
			case <-updates:
				t.Fatal("an uncommitted decision woke a long-poll subscriber")
			default:
			}
			if scenario.commit {
				require.NoError(t, transactions.Commit(owner))
				select {
				case <-updates:
				default:
					t.Fatal("committed decision did not wake its subscriber")
				}
			} else {
				require.NoError(t, transactions.Rollback(owner))
				select {
				case <-updates:
					t.Fatal("rolled-back decision woke its subscriber")
				default:
				}
			}
			stored, err := repo.Get(ctx, pending.ID)
			require.NoError(t, err)
			version, err = syncState.GetVersion(ctx)
			require.NoError(t, err)
			if scenario.commit {
				require.Equal(t, storage.ApprovalStatusDenied, stored.Status)
				require.Equal(t, int64(1), version)
			} else {
				require.Equal(t, storage.ApprovalStatusPending, stored.Status)
				require.Zero(t, version)
			}
		})
	}
}
