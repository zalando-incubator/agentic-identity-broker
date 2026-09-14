//go:build integration
// +build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCredentialTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func createTestAgent(t *testing.T, adapter *Adapter) *storage.Agent {
	t.Helper()
	repo := NewAgentRepository(adapter)
	now := time.Now().UTC()
	agent := &storage.Agent{
		ClientID:    ptr.To(id.ClientID("cred-test-" + id.NewAgentID().String()[:8])),
		DisplayName: "Credential Test Agent",
		Description: "Agent for credential testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	attachTestPermissionSet(t, adapter, agent)
	err := repo.Create(context.Background(), agent)
	require.NoError(t, err)
	return agent
}

func TestClientCredentialRepo(t *testing.T) {
	adapter, cleanup := setupCredentialTestDB(t)
	defer cleanup()

	repo := NewClientCredentialRepo(adapter)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
			CreatedAt:  time.Now().UTC(),
		}

		err := repo.Create(ctx, cred)
		require.NoError(t, err)
	})

	t.Run("GetByAgentID", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
			CreatedAt:  time.Now().UTC(),
		}
		err := repo.Create(ctx, cred)
		require.NoError(t, err)

		got, err := repo.GetByAgentID(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, cred.AgentID, got.AgentID)
		assert.Equal(t, cred.SecretHash, got.SecretHash)
	})

	t.Run("GetByClientID", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		clientID := id.NewClientID(agent.ID.String())
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
			CreatedAt:  time.Now().UTC(),
		}
		err := repo.Create(ctx, cred)
		require.NoError(t, err)

		got, err := repo.GetByClientID(ctx, clientID)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, got.AgentID)
		assert.Equal(t, cred.SecretHash, got.SecretHash)
	})

	t.Run("GetByAgentID timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()

		_, err := repo.GetByAgentID(timeoutCtx, id.NewAgentID())
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindTimeout, se.Kind)
	})

	t.Run("GetByClientID timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()

		_, err := repo.GetByClientID(timeoutCtx, id.NewClientID(id.NewAgentID().String()))
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindTimeout, se.Kind)
	})

	t.Run("GetByAgentID not found", func(t *testing.T) {
		_, err := repo.GetByAgentID(ctx, id.NewAgentID())
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindNotFound, se.Kind)
	})

	t.Run("GetByClientID not found", func(t *testing.T) {
		_, err := repo.GetByClientID(ctx, id.NewClientID(id.NewAgentID().String()))
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindNotFound, se.Kind)
	})

	t.Run("Delete", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		cred := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
			CreatedAt:  time.Now().UTC(),
		}
		err := repo.Create(ctx, cred)
		require.NoError(t, err)

		err = repo.Delete(ctx, agent.ID)
		require.NoError(t, err)

		_, err = repo.GetByAgentID(ctx, agent.ID)
		require.Error(t, err)
		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindNotFound, se.Kind)
	})

	t.Run("UniquePerAgent", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		cred1 := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash1",
			CreatedAt:  time.Now().UTC(),
		}
		err := repo.Create(ctx, cred1)
		require.NoError(t, err)

		cred2 := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash2",
			CreatedAt:  time.Now().UTC(),
		}
		err = repo.Create(ctx, cred2)
		assert.Error(t, err, "second credential for same agent should fail unique constraint")
	})

	t.Run("UniqueClientID", func(t *testing.T) {
		agent1 := createTestAgent(t, adapter)
		_ = createTestAgent(t, adapter)

		cred1 := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent1.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash1",
			CreatedAt:  time.Now().UTC(),
		}
		err := repo.Create(ctx, cred1)
		require.NoError(t, err)

		cred2 := &storage.ClientCredential{
			ID:         id.NewCredentialID(),
			AgentID:    agent1.ID,
			SecretHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash2",
			CreatedAt:  time.Now().UTC(),
		}
		err = repo.Create(ctx, cred2)
		assert.Error(t, err, "duplicate agent_id should fail unique constraint")
	})
}
