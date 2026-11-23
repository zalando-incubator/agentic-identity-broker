// Package bootstrap provides test infrastructure for E2E testing.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// StorageFactory creates fresh test storage instances for E2E tests.
// This factory uses PRODUCTION storage.Adapter with PRODUCTION memory backend.
//
// Key design:
// - Each call to NewTestStorage() creates a FRESH storage instance
// - This ensures complete test isolation
// - Uses PRODUCTION memory storage from internal/adapters/storage/memory
// - All data is ephemeral (lost when storage is garbage collected)
//
// Architecture:
// - THIN WRAPPER around production storage.NewAdapter()
// - NO test-specific storage logic
// - NO mock repositories
// - Identical to production memory storage setup
type StorageFactory struct {
	logger *slog.Logger
}

// NewStorageFactory creates a new StorageFactory.
//
// Parameters:
//   - logger: Structured logger for storage diagnostics
//
// Returns: Factory ready to create test storage instances
//
// Postconditions:
//   - Factory is ready to call NewTestStorage() multiple times
func NewStorageFactory(logger *slog.Logger) *StorageFactory {
	return &StorageFactory{
		logger: logger,
	}
}

// NewTestStorage creates a fresh in-memory storage adapter for a single test.
// This method wraps production storageadapter.NewAdapter() configured for memory backend.
//
// Returns:
//   - *storageadapter.Adapter: Fresh in-memory storage (all repositories initialized)
//   - error: If storage initialization fails
//
// Behavior:
// - Each call creates a FRESH storage instance (test isolation)
// - Storage is initialized and ready for use immediately
// - All data is ephemeral (not persisted)
// - Thread-safe due to sync.RWMutex in memory adapters
//
// Storage includes (via production adapters):
// - Users repository (ports.UserRepository)
// - Agents repository (ports.AgentRepository)
// - Services repository (ports.ThirdpartyOAuth2ServiceRepository)
// - User grants repository (ports.UserGrantRepository)
// - User sessions repository (ports.UserSessionRepository)
//
// Example usage in tests:
//
//	storage, err := storageFactory.NewTestStorage()
//	require.NoError(t, err)
//	defer storage.Close(context.Background())
//	// storage is ready to pass to app.Builder
func (f *StorageFactory) NewTestStorage() (*storageadapter.Adapter, error) {
	// Configuration for in-memory backend
	// This matches production memory storage setup exactly
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			// These timeouts are not used for in-memory storage
			// but required by the config schema
			Read:  0, // N/A for memory
			Write: 0, // N/A for memory
		},
	}

	// Use production storage factory
	// This creates fresh memory adapters for all repositories
	adapter, err := storageadapter.NewAdapter(config, slog.New(slog.DiscardHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to create test storage: %w", err)
	}

	return adapter, nil
}

// HealthCheck verifies storage is operational.
// This is useful for test setup verification.
//
// Parameters:
//   - storage: Storage adapter to check
//
// Returns: error if storage is not healthy
//
// In-memory storage is always healthy, but this provides consistency
// with production code where storage health checks matter.
func (f *StorageFactory) HealthCheck(storage *storageadapter.Adapter) error {
	if storage == nil {
		return fmt.Errorf("storage is nil")
	}

	ctx := context.Background()
	if err := storage.HealthCheck(ctx); err != nil {
		return fmt.Errorf("storage health check failed: %w", err)
	}

	return nil
}

// CloseStorage gracefully closes a storage adapter.
// This is called in test cleanup (defer).
//
// Parameters:
//   - storage: Storage adapter to close
//
// Returns: error if close fails
//
// Safe to call multiple times (in-memory is idempotent).
func (f *StorageFactory) CloseStorage(storage *storageadapter.Adapter) error {
	if storage == nil {
		return nil // Idempotent
	}

	ctx := context.Background()
	if err := storage.Close(ctx); err != nil {
		return fmt.Errorf("failed to close storage: %w", err)
	}

	return nil
}
