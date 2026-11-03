package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenSessionStore(t *testing.T) {
	ctx := context.Background()

	t.Run("create and find by signature", func(t *testing.T) {
		store := NewRefreshTokenSessionStore(NewTransactionManager())
		session := &storage.RefreshTokenSession{
			Signature: "sig-1",
			RequestID: "req-1",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-1"),
			Principal: id.NewPrincipal("user@example.com"),
			Scope:     "offline_access read",
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.Create(ctx, session))

		got, err := store.FindBySignature(ctx, "sig-1")
		require.NoError(t, err)
		assert.Equal(t, session.RequestID, got.RequestID)
		assert.Equal(t, session.Scope, got.Scope)
		assert.Nil(t, got.UsedAt)
	})

	t.Run("mark used is single-use", func(t *testing.T) {
		store := NewRefreshTokenSessionStore(NewTransactionManager())
		session := &storage.RefreshTokenSession{
			Signature: "sig-2",
			RequestID: "req-2",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-2"),
			Principal: id.NewPrincipal("user@example.com"),
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}
		require.NoError(t, store.Create(ctx, session))
		require.NoError(t, store.MarkUsed(ctx, "sig-2"))

		err := store.MarkUsed(ctx, "sig-2")
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

		got, err := store.FindBySignature(ctx, "sig-2")
		require.NoError(t, err)
		require.NotNil(t, got.UsedAt)
	})

	t.Run("revoke by request id marks matching sessions used", func(t *testing.T) {
		store := NewRefreshTokenSessionStore(NewTransactionManager())
		require.NoError(t, store.Create(ctx, &storage.RefreshTokenSession{
			Signature: "sig-3",
			RequestID: "req-chain",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-3"),
			Principal: id.NewPrincipal("user@example.com"),
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}))
		require.NoError(t, store.Create(ctx, &storage.RefreshTokenSession{
			Signature: "sig-4",
			RequestID: "req-chain",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-4"),
			Principal: id.NewPrincipal("user@example.com"),
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}))

		require.NoError(t, store.RevokeByRequestID(ctx, "req-chain"))

		for _, signature := range []string{"sig-3", "sig-4"} {
			got, err := store.FindBySignature(ctx, signature)
			require.NoError(t, err)
			require.NotNil(t, got.UsedAt)
		}
	})

	t.Run("delete expired removes expired sessions only", func(t *testing.T) {
		store := NewRefreshTokenSessionStore(NewTransactionManager())
		require.NoError(t, store.Create(ctx, &storage.RefreshTokenSession{
			Signature: "expired",
			RequestID: "req-expired",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-expired"),
			Principal: id.NewPrincipal("user@example.com"),
			ExpiresAt: time.Now().Add(-time.Minute),
			CreatedAt: time.Now().Add(-2 * time.Minute),
		}))
		require.NoError(t, store.Create(ctx, &storage.RefreshTokenSession{
			Signature: "active",
			RequestID: "req-active",
			AgentID:   id.NewAgentID(),
			ClientID:  id.NewClientID("client-active"),
			Principal: id.NewPrincipal("user@example.com"),
			ExpiresAt: time.Now().Add(time.Hour),
			CreatedAt: time.Now(),
		}))

		deleted, err := store.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, deleted)

		_, err = store.FindBySignature(ctx, "expired")
		assert.Error(t, err)
		_, err = store.FindBySignature(ctx, "active")
		require.NoError(t, err)
	})
}

func TestRefreshTokenSessionStoreSuccessorRevocation(t *testing.T) {
	for _, action := range []string{"rotation first", "request revocation", "principal revocation", "agent revocation", "other principal", "expired predecessor", "unconsumed predecessor", "different chain"} {
		t.Run(action, func(t *testing.T) {
			ctx := t.Context()
			store := NewRefreshTokenSessionStore(NewTransactionManager())
			root := &storage.RefreshTokenSession{
				Signature: "root", RequestID: "chain", AgentID: id.NewAgentID(),
				ClientID: id.NewClientID("client"), Principal: id.Principal("alice"),
				ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
			}
			if action == "expired predecessor" {
				root.ExpiresAt = time.Now().Add(-time.Hour)
			}
			require.NoError(t, store.Create(ctx, root))
			if action != "unconsumed predecessor" {
				require.NoError(t, store.MarkUsed(ctx, root.Signature))
			}
			consumed, err := store.FindBySignature(ctx, root.Signature)
			require.NoError(t, err)
			successor := *root
			successor.Signature = "successor"
			successor.ExpiresAt = time.Now().Add(time.Hour)
			switch action {
			case "rotation first":
				require.NoError(t, store.CreateSuccessor(ctx, &successor, root.Signature))
				count, err := store.RevokeByPrincipalAndAgent(ctx, root.Principal, root.AgentID)
				require.NoError(t, err)
				assert.Equal(t, 1, count)
				stored, err := store.FindBySignature(ctx, successor.Signature)
				require.NoError(t, err)
				assert.NotNil(t, stored.UsedAt)
				return
			case "request revocation":
				require.NoError(t, store.RevokeByRequestID(ctx, root.RequestID))
			case "principal revocation":
				count, err := store.RevokeByPrincipalAndAgent(ctx, root.Principal, root.AgentID)
				require.NoError(t, err)
				assert.Zero(t, count, "a consumed ancestor is fenced without counting it again")
			case "agent revocation":
				count, err := store.RevokeByAgent(ctx, root.AgentID)
				require.NoError(t, err)
				assert.Zero(t, count)
			case "other principal":
				_, err := store.RevokeByPrincipalAndAgent(ctx, id.Principal("bob"), root.AgentID)
				require.NoError(t, err)
				require.NoError(t, store.CreateSuccessor(ctx, &successor, root.Signature))
				stored, err := store.FindBySignature(ctx, successor.Signature)
				require.NoError(t, err)
				assert.Nil(t, stored.UsedAt)
				return
			case "expired predecessor":
				_, err := store.DeleteExpired(ctx)
				require.NoError(t, err)
			case "different chain":
				successor.RequestID = "unrelated"
			}
			err = store.CreateSuccessor(ctx, &successor, root.Signature)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
			_, err = store.FindBySignature(ctx, successor.Signature)
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
			if action != "expired predecessor" {
				stored, err := store.FindBySignature(ctx, root.Signature)
				require.NoError(t, err)
				assert.Equal(t, consumed.UsedAt, stored.UsedAt)
			}
		})
	}
}

func TestRefreshTokenSessionStoreRevocationRollback(t *testing.T) {
	for _, action := range []string{"request", "principal", "agent"} {
		t.Run(action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			manager := NewTransactionManager()
			store := NewRefreshTokenSessionStore(manager)
			grantID := id.NewGrantID()
			root := &storage.RefreshTokenSession{
				Signature: "root", RequestID: "chain", AgentID: id.NewAgentID(),
				Principal: id.Principal("alice"), GrantID: &grantID,
				ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
			}
			require.NoError(t, store.Create(ctx, root))
			require.NoError(t, store.MarkUsed(ctx, root.Signature))
			consumed, err := store.FindBySignature(ctx, root.Signature)
			require.NoError(t, err)
			successor := *root
			successor.Signature = "successor"
			require.NoError(t, store.CreateSuccessor(ctx, &successor, root.Signature))
			txCtx, err := manager.BeginTX(ctx)
			require.NoError(t, err)
			defer func() { _ = manager.Rollback(txCtx) }()
			switch action {
			case "request":
				require.NoError(t, store.RevokeByRequestID(txCtx, root.RequestID))
			case "principal":
				count, err := store.RevokeByPrincipalAndAgent(txCtx, root.Principal, root.AgentID)
				require.NoError(t, err)
				assert.Equal(t, 1, count)
			case "agent":
				count, err := store.RevokeByAgent(txCtx, root.AgentID)
				require.NoError(t, err)
				assert.Equal(t, 1, count)
			}
			late := successor
			late.Signature = "late"
			err = store.CreateSuccessor(txCtx, &late, root.Signature)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
			require.NoError(t, manager.Rollback(txCtx))

			stored, err := store.FindBySignature(ctx, root.Signature)
			require.NoError(t, err)
			assert.Equal(t, consumed.UsedAt, stored.UsedAt)
			assert.Equal(t, &grantID, stored.GrantID)
			stored, err = store.FindBySignature(ctx, successor.Signature)
			require.NoError(t, err)
			assert.Nil(t, stored.UsedAt)
			require.NoError(t, store.CreateSuccessor(ctx, &late, root.Signature), "rollback must restore the consumed ancestor's authority")
		})
	}
}

func TestRefreshTokenSessionStoreRotationRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	manager := NewTransactionManager()
	store := NewRefreshTokenSessionStore(manager)
	root := &storage.RefreshTokenSession{
		Signature: "root", RequestID: "chain", AgentID: id.NewAgentID(),
		ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
	}
	require.NoError(t, store.Create(ctx, root))
	txCtx, err := manager.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = manager.Rollback(txCtx) }()
	require.NoError(t, store.MarkUsed(txCtx, root.Signature))
	successor := *root
	successor.Signature = "successor"
	require.NoError(t, store.CreateSuccessor(txCtx, &successor, root.Signature))
	require.NoError(t, manager.Rollback(txCtx))
	stored, err := store.FindBySignature(ctx, root.Signature)
	require.NoError(t, err)
	assert.Nil(t, stored.UsedAt)
	_, err = store.FindBySignature(ctx, successor.Signature)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
}

func TestRefreshTokenSessionStoreExpiryRollbackRestoresRevocation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	manager := NewTransactionManager()
	store := NewRefreshTokenSessionStore(manager)
	root := &storage.RefreshTokenSession{
		Signature: "root", RequestID: "chain", AgentID: id.NewAgentID(),
		ExpiresAt: time.Now().Add(-time.Hour), CreatedAt: time.Now().Add(-2 * time.Hour),
	}
	require.NoError(t, store.Create(ctx, root))
	require.NoError(t, store.MarkUsed(ctx, root.Signature))
	require.NoError(t, store.RevokeByRequestID(ctx, root.RequestID))
	txCtx, err := manager.BeginTX(ctx)
	require.NoError(t, err)
	defer func() { _ = manager.Rollback(txCtx) }()
	count, err := store.DeleteExpired(txCtx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	require.NoError(t, manager.Rollback(txCtx))
	stored, err := store.FindBySignature(ctx, root.Signature)
	require.NoError(t, err)
	require.NotNil(t, stored.UsedAt)
	successor := *root
	successor.Signature = "successor"
	successor.ExpiresAt = time.Now().Add(time.Hour)
	err = store.CreateSuccessor(ctx, &successor, root.Signature)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind, "restoring the expired row must also restore its revocation fence")
	count, err = store.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	root.ExpiresAt = successor.ExpiresAt
	require.NoError(t, store.Create(ctx, root))
	require.NoError(t, store.MarkUsed(ctx, root.Signature))
	require.NoError(t, store.CreateSuccessor(ctx, &successor, root.Signature), "committed cleanup must remove the old revocation fence with its row")
}

func TestRefreshTokenSessionStoreLifecycleOperationsHonorTransactionGate(t *testing.T) {
	for _, action := range []string{"successor", "principal", "agent"} {
		t.Run(action, func(t *testing.T) {
			manager := NewTransactionManager()
			store := NewRefreshTokenSessionStore(manager)
			ctx := t.Context()
			root := &storage.RefreshTokenSession{
				Signature: "root", RequestID: "chain", AgentID: id.NewAgentID(),
				Principal: id.Principal("alice"), ExpiresAt: time.Now().Add(time.Hour),
			}
			require.NoError(t, store.Create(ctx, root))
			require.NoError(t, store.MarkUsed(ctx, root.Signature))
			txCtx, err := manager.BeginTX(ctx)
			require.NoError(t, err)
			defer func() { _ = manager.Rollback(txCtx) }()
			blocked, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
			defer cancel()
			switch action {
			case "successor":
				successor := *root
				successor.Signature = "successor"
				err = store.CreateSuccessor(blocked, &successor, root.Signature)
			case "principal":
				_, err = store.RevokeByPrincipalAndAgent(blocked, root.Principal, root.AgentID)
			case "agent":
				_, err = store.RevokeByAgent(blocked, root.AgentID)
			}
			require.ErrorIs(t, err, context.DeadlineExceeded, "independent lifecycle operations must wait for transaction visibility")
			require.NoError(t, manager.Rollback(txCtx))
		})
	}
}
