//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

func TestPGRefreshReadsOwnConsumedPredecessorBeforeSuccessor(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	root, first := createPGRefreshRoot(t, adapter, id.AgentID{}, id.NewPrincipal("readback@example.test"))
	tokens := NewRefreshTokenRepo(adapter)
	rollback := errors.New("readback only")
	err := NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		if err := tokens.MarkUsed(scope, first.Signature, at); err != nil {
			return err
		}
		consumed, err := tokens.FindBySignature(scope, first.Signature)
		require.NoError(t, err, "fresh rotation must inspect its own consumed predecessor before inserting its successor")
		require.NotNil(t, consumed.UsedAt)
		require.True(t, consumed.UsedAt.Equal(at))
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	stored, err := tokens.FindBySignature(ctx, first.Signature)
	require.NoError(t, err)
	require.Nil(t, stored.UsedAt, "the aborted readback must leave the predecessor usable")
}
