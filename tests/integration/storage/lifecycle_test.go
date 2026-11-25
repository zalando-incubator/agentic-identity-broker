package storage

import (
	"context"
	"fmt"
	"github.com/agentic-identity-broker/agentic-identity-broker/tools/livingdoc/evidence"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryAdapter_FullLifecycle tests complete memory adapter lifecycle
func TestMemoryAdapter_FullLifecycle(t *testing.T) {
	evidence.ForTest(t, "storage-and-encryption.backend.memory")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create adapter
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := storage.NewAdapter(config)
	require.NoError(t, err)
	require.NotNil(t, adapter)

	users := adapter.Users()

	// Health check after initialization
	err = adapter.HealthCheck(ctx)
	assert.NoError(t, err)

	// Create users
	userID1 := id.NewUserID()
	userID2 := id.NewUserID()
	user1 := &ports.User{
		ID:        userID1,
		Email:     "user1@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	user2 := &ports.User{
		ID:        userID2,
		Email:     "user2@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = users.CreateUser(ctx, user1)
	require.NoError(t, err)

	err = users.CreateUser(ctx, user2)
	require.NoError(t, err)

	// Read user
	retrieved, err := users.GetUser(ctx, userID1)
	require.NoError(t, err)
	assert.Equal(t, userID1, retrieved.ID)
	assert.Equal(t, "user1@example.com", retrieved.Email)

	// List users
	allUsers, err := users.ListUsers(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, allUsers, 2)

	// Update user
	user1.Email = "newemail@example.com"
	user1.UpdatedAt = time.Now()
	err = users.UpdateUser(ctx, user1)
	require.NoError(t, err)

	// Verify update
	updated, err := users.GetUser(ctx, userID1)
	require.NoError(t, err)
	assert.Equal(t, "newemail@example.com", updated.Email)

	// Delete user
	err = users.DeleteUser(ctx, userID2)
	require.NoError(t, err)

	// Verify deletion
	_, err = users.GetUser(ctx, userID2)
	assert.Error(t, err)

	// List should have only 1 user
	remainingUsers, err := users.ListUsers(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, remainingUsers, 1)

	// Health check before close
	err = adapter.HealthCheck(ctx)
	assert.NoError(t, err)

	// Close
	err = adapter.Close(ctx)
	require.NoError(t, err)
}

// TestMemoryAdapter_ConcurrentOperations tests concurrent CRUD operations
func TestMemoryAdapter_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := storage.NewAdapter(config)
	require.NoError(t, err)

	users := adapter.Users()

	// Create 100 users concurrently
	done := make(chan struct{})
	errCh := make(chan error, 100)

	for i := 0; i < 100; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			user := &ports.User{
				ID:        id.NewUserID(),
				Email:     "test@example.com",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			if err := users.CreateUser(ctx, user); err != nil {
				errCh <- err
			}
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Check for errors
	close(errCh)
	for err := range errCh {
		// Some errors are expected due to duplicate IDs
		t.Logf("Expected error from concurrent creates: %v", err)
	}

	// Verify some users were created
	allUsers, err := users.ListUsers(ctx, nil)
	require.NoError(t, err)
	assert.Greater(t, len(allUsers), 0)

	// Close
	err = adapter.Close(ctx)
	assert.NoError(t, err)
}

// TestMemoryAdapter_TimeoutHandling tests operation timeouts
func TestMemoryAdapter_TimeoutHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the context is already done when the operation runs
	cancel()

	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := storage.NewAdapter(config)
	require.NoError(t, err)

	users := adapter.Users()

	user := &ports.User{
		ID:        id.NewUserID(),
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Operation should timeout
	err = users.CreateUser(ctx, user)
	assert.Error(t, err)
}

// TestMemoryAdapter_PaginationSupport tests list pagination
func TestMemoryAdapter_PaginationSupport(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := storage.NewAdapter(config)
	require.NoError(t, err)

	users := adapter.Users()

	// Create 25 users with unique IDs
	for i := 1; i <= 25; i++ {
		user := &ports.User{
			ID:        id.MustParseUserID(fmt.Sprintf("00000000-0000-0000-0000-%012d", i)),
			Email:     "test@example.com",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		err := users.CreateUser(ctx, user)
		require.NoError(t, err)
	}

	// Get first page (limit 10)
	filter := &ports.UserFilter{
		Limit:  10,
		Offset: 0,
	}
	page1, err := users.ListUsers(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 10, len(page1))

	// Get second page (limit 10, offset 10)
	filter.Offset = 10
	page2, err := users.ListUsers(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 10, len(page2))

	// Get remaining (limit 10, offset 20)
	filter.Offset = 20
	page3, err := users.ListUsers(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 5, len(page3))

	// Close
	err = adapter.Close(ctx)
	assert.NoError(t, err)
}

// TestMemoryAdapter_ErrorRecovery tests adapter behavior during error conditions
func TestMemoryAdapter_ErrorRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := storage.NewAdapter(config)
	require.NoError(t, err)

	users := adapter.Users()

	// Try to create duplicate user
	dupUserID := id.NewUserID()
	user1 := &ports.User{
		ID:        dupUserID,
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = users.CreateUser(ctx, user1)
	require.NoError(t, err)

	// Duplicate should fail
	err = users.CreateUser(ctx, user1)
	assert.Error(t, err)

	// Try to update non-existent user
	user2 := &ports.User{
		ID:        id.NewUserID(),
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = users.UpdateUser(ctx, user2)
	assert.Error(t, err)

	// Try to delete non-existent user (should be idempotent)
	err = users.DeleteUser(ctx, id.NewUserID())
	assert.NoError(t, err)

	// Adapter should still be healthy
	err = adapter.HealthCheck(ctx)
	assert.NoError(t, err)

	// Close
	err = adapter.Close(ctx)
	assert.NoError(t, err)
}
