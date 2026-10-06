package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestMemoryRetentionDoesNotConflictWithLiveSessionForSameAgent(t *testing.T) {
	store := newMemoryRefreshAdapter(t)
	now := time.Now().UTC().Truncate(time.Second)
	terminal, _ := createMemoryRefreshRoot(t, store, id.AgentID{}, id.Principal("retention@example.test"), now)
	live, _ := createMemoryRefreshRoot(t, store, terminal.AgentID, id.Principal("live-retention@example.test"), now)
	require.NoError(t, store.AuthorizationCoordinator().Run(t.Context(), terminal.AgentID, func(ctx context.Context, at time.Time) error {
		return store.RefreshRevocations().RevokeByID(ctx, terminal.ID, at, storage.RefreshReasonGrantDeleted)
	}))
	entered, resume := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- store.AuthorizationCoordinator().Run(t.Context(), live.AgentID, func(ctx context.Context, _ time.Time) error {
			root, err := store.RefreshSessions().FindByID(ctx, live.ID)
			if err != nil {
				return err
			}
			close(entered)
			<-resume
			return store.RefreshSessions().Save(ctx, root)
		})
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatalf("live operation failed before retention: %v", err)
	case <-time.After(time.Second):
		t.Fatal("live operation never entered its owner scope")
	}
	count, err := store.RefreshMaintenance().DeleteTerminal(t.Context(), terminal.RetainUntil, 200)
	close(resume)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NoError(t, <-finished, "purging independent terminal history must not reject a live same-agent operation")
	root, err := store.RefreshSessions().FindByID(t.Context(), live.ID)
	require.NoError(t, err)
	require.Nil(t, root.TerminalReason)
	require.Equal(t, live.CurrentSignature, root.CurrentSignature)
}
