package storage

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdapter_Memory(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	require.NoError(t, err)
	assert.NotNil(t, adapter)
	assert.NotNil(t, adapter.Users())
}

func TestNewAdapter_MemoryClientAssertionReplayRepository(t *testing.T) {
	adapter, err := NewAdapter(&ports.StorageConfig{Backend: "memory"})
	require.NoError(t, err)
	repo := adapter.ClientAssertionReplays()
	require.NotNil(t, repo)
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, repo.Consume(context.Background(), "client-a", "jti-1", expiresAt))
	require.Error(t, repo.Consume(context.Background(), "client-a", "jti-1", expiresAt))
}

func TestNewAdapter_MemoryExposesSigningKeyBootstrapCoordinator(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)
	require.NoError(t, err)

	coordinator := adapter.SigningKeyBootstrapCoordinator()
	require.NotNil(t, coordinator)

	called := false
	err = coordinator.WithBootstrapLock(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestNewAdapter_Postgres(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	// NewAdapter now initializes the adapter at creation time.
	// Without a running database, initialization must fail.
	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "failed to initialize storage adapter")
}

func TestNewAdapter_InvalidBackend(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "invalid",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "unsupported storage backend")
}

func TestNewAdapter_PostgresNoURL(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config)

	assert.Error(t, err)
	assert.Nil(t, adapter)
	assert.Contains(t, err.Error(), "connection URL cannot be empty")
}

// TestBackendSwitching verifies that different adapters can be created independently
func TestBackendSwitching(t *testing.T) {
	// Create memory adapter
	memConfig := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}
	memAdapter, err := NewAdapter(memConfig)
	require.NoError(t, err)
	require.NotNil(t, memAdapter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create user in memory adapter
	userID := id.MustParseUserID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	user := &ports.User{
		ID:        userID,
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	err = memAdapter.Users().CreateUser(ctx, user)
	require.NoError(t, err)

	// Verify user exists in memory adapter
	retrieved, err := memAdapter.Users().GetUser(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, userID, retrieved.ID)

	// Memory adapter implements all repository interfaces
	assert.NotNil(t, memAdapter.Users())
	assert.NotNil(t, memAdapter.Agents())
	assert.NotNil(t, memAdapter.Services())
	assert.NotNil(t, memAdapter.UserGrants())
	assert.NotNil(t, memAdapter.UserSessions())

	// Close memory adapter
	err = memAdapter.Close(context.Background())
	assert.NoError(t, err)
}

// TestBackendSelectionValidation verifies backend selection validation during factory creation
func TestBackendSelectionValidation(t *testing.T) {
	tests := []struct {
		name       string
		backend    string
		shouldFail bool
	}{
		{
			name:       "valid memory backend",
			backend:    "memory",
			shouldFail: false,
		},
		{
			name:       "valid postgres backend",
			backend:    "postgres",
			shouldFail: true, // Init fails without a running database in unit tests
		},
		{
			name:       "invalid backend sqlite",
			backend:    "sqlite",
			shouldFail: true,
		},
		{
			name:       "invalid backend mongodb",
			backend:    "mongodb",
			shouldFail: true,
		},
		{
			name:       "empty backend",
			backend:    "",
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ports.StorageConfig{
				Backend: tt.backend,
				Postgres: ports.PostgresConfig{
					ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
				},
				Timeouts: ports.StorageTimeouts{
					Read:  5 * time.Second,
					Write: 10 * time.Second,
				},
			}

			adapter, err := NewAdapter(config)

			if tt.shouldFail {
				assert.Error(t, err)
				assert.Nil(t, adapter)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, adapter)
			}
		})
	}
}
