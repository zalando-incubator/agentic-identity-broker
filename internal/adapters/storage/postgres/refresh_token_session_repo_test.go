//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRefreshTokenSessionTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func TestRefreshTokenSessionRepo(t *testing.T) {
	adapter, cleanup := setupRefreshTokenSessionTestDB(t)
	defer cleanup()

	repo := NewRefreshTokenSessionRepo(adapter)
	ctx := context.Background()

	newSession := func(t *testing.T, signature, requestID string, expiresAt time.Time) *storage.RefreshTokenSession {
		t.Helper()
		agent := createTestAgent(t, adapter)
		return &storage.RefreshTokenSession{
			Signature:   signature,
			RequestID:   requestID,
			AgentID:     agent.ID,
			ClientID:    id.NewClientID("client-" + signature),
			Principal:   id.NewPrincipal("user@example.com"),
			Email:       ptr.To("u@example.com"),
			DisplayName: "Jane Doe",
			Scope:       "offline_access read",
			ExpiresAt:   expiresAt,
			CreatedAt:   time.Now().UTC(),
		}
	}

	t.Run("Create and FindBySignature", func(t *testing.T) {
		session := newSession(t, "sig-1", "req-1", time.Now().Add(time.Hour).UTC())
		require.NoError(t, repo.Create(ctx, session))

		got, err := repo.FindBySignature(ctx, "sig-1")
		require.NoError(t, err)
		assert.Equal(t, session.RequestID, got.RequestID)
		assert.Equal(t, session.Scope, got.Scope)
		assert.Nil(t, got.UsedAt)
		assert.Equal(t, "u@example.com", *got.Email)
		assert.Equal(t, "Jane Doe", got.DisplayName)
	})

	t.Run("Create and FindBySignature preserves nil email", func(t *testing.T) {
		session := newSession(t, "sig-nil-email", "req-nil-email", time.Now().Add(time.Hour).UTC())
		session.Email = nil
		require.NoError(t, repo.Create(ctx, session))

		got, err := repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		assert.Nil(t, got.Email)
	})

	t.Run("MarkUsed makes token inactive", func(t *testing.T) {
		session := newSession(t, "sig-2", "req-2", time.Now().Add(time.Hour).UTC())
		require.NoError(t, repo.Create(ctx, session))
		require.NoError(t, repo.MarkUsed(ctx, "sig-2"))

		got, err := repo.FindBySignature(ctx, "sig-2")
		require.NoError(t, err)
		require.NotNil(t, got.UsedAt)
	})

	t.Run("Rollback restores a rotated token when replacement creation fails", func(t *testing.T) {
		session := newSession(t, "sig-rollback", "req-rollback", time.Now().Add(time.Hour).UTC())
		require.NoError(t, repo.Create(ctx, session))

		txCtx, err := adapter.BeginTX(ctx)
		require.NoError(t, err)
		require.NoError(t, repo.MarkUsed(txCtx, session.Signature))
		require.Error(t, repo.Create(txCtx, session))
		require.NoError(t, adapter.Rollback(txCtx))

		got, err := repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		assert.Nil(t, got.UsedAt)
	})

	t.Run("Successor creation requires the rotation transaction", func(t *testing.T) {
		root := newSession(t, "sig-no-transaction", "req-no-transaction", time.Now().Add(time.Hour).UTC())
		require.NoError(t, repo.Create(ctx, root))
		require.NoError(t, repo.MarkUsed(ctx, root.Signature))
		require.NoError(t, repo.RevokeByRequestID(ctx, root.RequestID))
		successor := *root
		successor.Signature += "-successor"
		err := repo.CreateSuccessor(ctx, &successor, root.Signature)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		_, err = repo.FindBySignature(ctx, successor.Signature)
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	})

	t.Run("RevokeByRequestID marks all matching sessions used", func(t *testing.T) {
		require.NoError(t, repo.Create(ctx, newSession(t, "sig-3", "req-chain", time.Now().Add(time.Hour).UTC())))
		require.NoError(t, repo.Create(ctx, newSession(t, "sig-4", "req-chain", time.Now().Add(time.Hour).UTC())))

		require.NoError(t, repo.RevokeByRequestID(ctx, "req-chain"))

		for _, signature := range []string{"sig-3", "sig-4"} {
			got, err := repo.FindBySignature(ctx, signature)
			require.NoError(t, err)
			require.NotNil(t, got.UsedAt)
		}
	})

	t.Run("DeleteExpired removes expired sessions only", func(t *testing.T) {
		require.NoError(t, repo.Create(ctx, newSession(t, "sig-expired", "req-expired", time.Now().Add(-time.Minute).UTC())))
		require.NoError(t, repo.Create(ctx, newSession(t, "sig-active", "req-active", time.Now().Add(time.Hour).UTC())))

		deleted, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, deleted)

		_, err = repo.FindBySignature(ctx, "sig-expired")
		assert.Error(t, err)
		_, err = repo.FindBySignature(ctx, "sig-active")
		require.NoError(t, err)
	})
}
