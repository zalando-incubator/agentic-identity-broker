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

func setupAuthCodeTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func TestAuthorizationCodeRepo(t *testing.T) {
	adapter, cleanup := setupAuthCodeTestDB(t)
	defer cleanup()

	repo := NewAuthorizationCodeRepo(adapter)
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		code := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      "sha256hashvalue" + id.NewAuthorizationCodeID().String()[:12],
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			Scope:         "read write",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			CreatedAt:     time.Now().UTC(),
		}

		err := repo.Create(ctx, code)
		require.NoError(t, err)
	})

	t.Run("FindByCodeHash", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		codeHash := "findbyhash" + id.NewAuthorizationCodeID().String()[:10]
		code := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      codeHash,
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			Scope:         "read",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			Email:         ptr.To("u@example.com"),
			DisplayName:   "Jane Doe",
			CreatedAt:     time.Now().UTC(),
		}
		err := repo.Create(ctx, code)
		require.NoError(t, err)

		got, err := repo.FindByCodeHash(ctx, codeHash)
		require.NoError(t, err)
		assert.Equal(t, codeHash, got.CodeHash)
		assert.Equal(t, agent.ID, got.AgentID)
		assert.Equal(t, "user@example.com", got.Principal.String())
		assert.Equal(t, "u@example.com", *got.Email)
		assert.Equal(t, "Jane Doe", got.DisplayName)
	})

	t.Run("FindByCodeHash preserves nil email", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		codeHash := "nil-email-" + id.NewAuthorizationCodeID().String()[:10]
		code := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      codeHash,
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			CreatedAt:     time.Now().UTC(),
		}
		require.NoError(t, repo.Create(ctx, code))

		got, err := repo.FindByCodeHash(ctx, codeHash)
		require.NoError(t, err)
		assert.Nil(t, got.Email)
	})

	t.Run("MarkUsed", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		code := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      "markused" + id.NewAuthorizationCodeID().String()[:10],
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			Scope:         "read",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			CreatedAt:     time.Now().UTC(),
		}
		err := repo.Create(ctx, code)
		require.NoError(t, err)

		err = repo.MarkUsed(ctx, code.ID)
		require.NoError(t, err)

		got, err := repo.FindByCodeHash(ctx, code.CodeHash)
		require.NoError(t, err)
		assert.NotNil(t, got.UsedAt)
	})

	t.Run("UniqueCodeHash", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		codeHash := "uniquehash" + id.NewAuthorizationCodeID().String()[:10]
		code1 := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      codeHash,
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			Scope:         "read",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			CreatedAt:     time.Now().UTC(),
		}
		err := repo.Create(ctx, code1)
		require.NoError(t, err)

		code2 := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      codeHash,
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user2@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge2",
			Scope:         "write",
			ExpiresAt:     time.Now().UTC().Add(60 * time.Second),
			CreatedAt:     time.Now().UTC(),
		}
		err = repo.Create(ctx, code2)
		assert.Error(t, err, "duplicate code_hash should fail unique constraint")
	})

	t.Run("FindByCodeHash excludes expired codes", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		var now time.Time
		require.NoError(t, adapter.db.GetContext(ctx, &now, "SELECT NOW()"))
		tests := []struct {
			name      string
			expiresAt time.Time
			used      bool
		}{
			{name: "expired unused", expiresAt: now.Add(-time.Minute)},
			{name: "expired used", expiresAt: now.Add(-time.Minute), used: true},
			{name: "expires now", expiresAt: now},
			{name: "missing expiry"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				code := &storage.AuthorizationCode{
					ID:            id.NewAuthorizationCodeID(),
					CodeHash:      "expired-" + id.NewAuthorizationCodeID().String(),
					AgentID:       agent.ID,
					Principal:     id.NewPrincipal("user@example.com"),
					RedirectURI:   "http://localhost/callback",
					CodeChallenge: "S256challenge",
					Scope:         "read",
					ExpiresAt:     tt.expiresAt,
					CreatedAt:     now.Add(-time.Hour),
				}
				require.NoError(t, repo.Create(ctx, code))
				if tt.used {
					require.NoError(t, repo.MarkUsed(ctx, code.ID))
				}

				got, err := repo.FindByCodeHash(ctx, code.CodeHash)
				require.Error(t, err)
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
				assert.Nil(t, got)
			})
		}
	})

	t.Run("FindByCodeHash not found", func(t *testing.T) {
		_, err := repo.FindByCodeHash(ctx, "nonexistentcodehash")
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindNotFound, se.Kind)
	})

	t.Run("FindByCodeHash timeout", func(t *testing.T) {
		timeoutCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
		defer cancel()

		_, err := repo.FindByCodeHash(timeoutCtx, "somehash")
		require.Error(t, err)

		var se *storage.StorageError
		require.True(t, errors.As(err, &se))
		assert.Equal(t, storage.ErrorKindTimeout, se.Kind)
	})

	t.Run("DeleteExpired", func(t *testing.T) {
		agent := createTestAgent(t, adapter)
		code := &storage.AuthorizationCode{
			ID:            id.NewAuthorizationCodeID(),
			CodeHash:      "expired" + id.NewAuthorizationCodeID().String()[:10],
			AgentID:       agent.ID,
			Principal:     id.NewPrincipal("user@example.com"),
			RedirectURI:   "http://localhost:9999/callback",
			CodeChallenge: "S256challenge",
			Scope:         "read",
			ExpiresAt:     time.Now().UTC().Add(-10 * time.Second),
			CreatedAt:     time.Now().UTC().Add(-70 * time.Second),
		}
		err := repo.Create(ctx, code)
		require.NoError(t, err)

		count, err := repo.DeleteExpired(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 1)
	})
}
