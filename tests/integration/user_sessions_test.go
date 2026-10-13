package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestUserSessionRepository_ListByPrincipal(t *testing.T) {
	repo := memory.NewInMemoryUserSessionRepository(memory.NewTransactionManager())
	ctx := context.Background()

	principal := id.Principal("user@example.com")

	// Create 3 sessions for same principal, different services
	for i := 1; i <= 3; i++ {
		session := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal,
			ServiceID:            id.NewServiceID(),
			EncryptedAccessToken: []byte("token"),
			TokenType:            "Bearer",
			CreatedAt:            time.Now(),
		}
		err := repo.Create(ctx, session)
		assert.NoError(t, err)
	}

	// List sessions for principal
	sessions, err := repo.ListByPrincipal(ctx, principal)
	assert.NoError(t, err)
	assert.Len(t, sessions, 3)

	// Verify all belong to principal
	for _, s := range sessions {
		assert.Equal(t, principal, s.Principal)
	}
}

func TestUserSessionRepository_ListByPrincipal_Empty(t *testing.T) {
	repo := memory.NewInMemoryUserSessionRepository(memory.NewTransactionManager())
	ctx := context.Background()

	sessions, err := repo.ListByPrincipal(ctx, id.Principal("nonexistent@example.com"))
	assert.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestUserSessionRepository_ListByPrincipal_MultiplePrincipals(t *testing.T) {
	repo := memory.NewInMemoryUserSessionRepository(memory.NewTransactionManager())
	ctx := context.Background()

	principal1 := id.Principal("user1@example.com")
	principal2 := id.Principal("user2@example.com")

	// Create sessions for two different principals
	for i := 1; i <= 2; i++ {
		session1 := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal1,
			ServiceID:            id.NewServiceID(),
			EncryptedAccessToken: []byte("token"),
			TokenType:            "Bearer",
			CreatedAt:            time.Now(),
		}
		err := repo.Create(ctx, session1)
		assert.NoError(t, err)

		session2 := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal2,
			ServiceID:            id.NewServiceID(),
			EncryptedAccessToken: []byte("token"),
			TokenType:            "Bearer",
			CreatedAt:            time.Now(),
		}
		err = repo.Create(ctx, session2)
		assert.NoError(t, err)
	}

	// List sessions for each principal
	sessions1, err := repo.ListByPrincipal(ctx, principal1)
	assert.NoError(t, err)
	assert.Len(t, sessions1, 2)

	sessions2, err := repo.ListByPrincipal(ctx, principal2)
	assert.NoError(t, err)
	assert.Len(t, sessions2, 2)

	// Verify isolation
	for _, s := range sessions1 {
		assert.Equal(t, principal1, s.Principal)
	}
	for _, s := range sessions2 {
		assert.Equal(t, principal2, s.Principal)
	}
}
