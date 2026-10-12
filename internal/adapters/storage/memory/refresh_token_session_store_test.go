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
		store := NewRefreshTokenSessionStore()
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
		store := NewRefreshTokenSessionStore()
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
		store := NewRefreshTokenSessionStore()
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
		store := NewRefreshTokenSessionStore()
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
			store := NewRefreshTokenSessionStore()
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
