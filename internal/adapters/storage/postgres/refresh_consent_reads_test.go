//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

func TestPGRefreshConsentReadsUseOwnedConnection(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	agent := createTestAgent(t, adapter)
	root, first := createPGRefreshRoot(t, adapter, agent.ID, id.Principal("owned-consent@example.test"))
	require.NotEmpty(t, agent.PermissionSets)
	adapter.db.SetMaxOpenConns(1)
	adapter.db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := NewAuthorizationSessionCoordinator(adapter).Run(ctx, agent.ID, func(owner context.Context, at time.Time) error {
		sets, err := NewPermissionSetRepository(adapter).GetByIDs(owner, []id.PermissionSetID{agent.PermissionSets[0].PermissionSetID})
		if err != nil {
			return err
		}
		require.Len(t, sets, 1)
		require.Equal(t, agent.PermissionSets[0].PermissionSetID, sets[0].ID)
		sessions, err := NewUserSessionRepository(adapter).ListActiveByPrincipal(owner, root.Principal)
		if err != nil {
			return err
		}
		require.Empty(t, sessions, "no connected upstream session was seeded")
		stored, err := NewRefreshSessionRepo(adapter).FindByID(owner, root.ID)
		if err != nil {
			return err
		}
		require.Equal(t, root.OriginalGrantID, stored.OriginalGrantID)
		return nil
	})
	require.NoError(t, err, "consent validation must borrow the owner connection, not wait on its own transaction")
	stored, err := NewRefreshTokenRepo(adapter).FindBySignature(context.Background(), first.Signature)
	require.NoError(t, err)
	require.Nil(t, stored.UsedAt)
}
