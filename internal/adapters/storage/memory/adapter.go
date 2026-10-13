// Package memory implements an in-memory storage adapter.
// Used for development and testing with zero external dependencies.
// All data is ephemeral - lost on application restart.
package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Adapter implements storage repository and lifecycle helpers
// using in-memory storage with sync.RWMutex for thread-safety.
type Adapter struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	users        map[id.UserID]*ports.User
}

// NewAdapter creates a new in-memory storage adapter.
// Returns immediately (no I/O or initialization required).
func NewAdapter(transactions *TransactionManager) *Adapter {
	return &Adapter{
		transactions: transactions,
		users:        make(map[id.UserID]*ports.User),
	}
}

// Initialize performs in-memory adapter initialization.
// For in-memory storage: returns nil immediately (no I/O needed).
// Satisfies storage lifecycle expectations.
func (a *Adapter) Initialize(ctx context.Context) error {
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// In-memory initialization is instant (no I/O)
	// Just verify internal state is ready
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.users == nil {
		a.users = make(map[id.UserID]*ports.User)
	}

	return nil
}

// Close gracefully closes the in-memory adapter.
// For in-memory storage: clears data and is idempotent.
// Satisfies storage lifecycle expectations.
func (a *Adapter) Close(ctx context.Context) error {
	guard, gateErr := a.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Clear all data
	for userID := range a.users {
		journalEntry(ctx, a.users, userID)
		delete(a.users, userID)
	}

	return nil
}

// HealthCheck verifies in-memory adapter is operational.
// For in-memory storage: always returns nil (no external dependency).
// Satisfies storage lifecycle expectations.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	// In-memory adapter is always healthy if internal map exists
	if a.users == nil {
		return storage.NewStorageError(
			"HealthCheck",
			storage.ErrorKindUnknown,
			nil,
			"in-memory adapter internal state corrupted",
		)
	}

	return nil
}

// CreateUser creates a new user entity in memory.
// Satisfies ports.UserRepository interface.
// Returns error if user ID already exists (conflict).
func (a *Adapter) CreateUser(ctx context.Context, user *ports.User) error {
	guard, gateErr := a.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindTimeout,
			ctx.Err(),
			"operation cancelled or timed out",
		)
	default:
	}

	// Validate input
	if user == nil {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user cannot be nil",
		)
	}

	if user.ID.IsZero() {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	if user.Email == "" {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user email cannot be empty",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check for duplicate
	if _, exists := a.users[user.ID]; exists {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindConflict,
			nil,
			fmt.Sprintf("user with ID %q already exists", user.ID),
		)
	}

	// Copy user to prevent external mutation
	userCopy := *user
	journalEntry(ctx, a.users, user.ID)
	a.users[user.ID] = &userCopy

	return nil
}

// GetUser retrieves a user entity by ID.
// Satisfies ports.UserRepository interface.
// Returns StorageError with Kind=NotFound if user not found.
func (a *Adapter) GetUser(ctx context.Context, userID id.UserID) (*ports.User, error) {
	guard, gateErr := a.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindTimeout,
			ctx.Err(),
			"operation cancelled or timed out",
		)
	default:
	}

	if userID.IsZero() {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	user, exists := a.users[userID]
	if !exists {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindNotFound,
			nil,
			fmt.Sprintf("user with ID %q not found", userID),
		)
	}

	// Copy user to prevent external mutation
	userCopy := *user
	return &userCopy, nil
}

// UpdateUser updates an existing user entity.
// Satisfies ports.UserRepository interface.
// Returns error if user ID not found.
func (a *Adapter) UpdateUser(ctx context.Context, user *ports.User) error {
	guard, gateErr := a.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindTimeout,
			ctx.Err(),
			"operation cancelled or timed out",
		)
	default:
	}

	// Validate input
	if user == nil {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user cannot be nil",
		)
	}

	if user.ID.IsZero() {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	if user.Email == "" {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user email cannot be empty",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check existence
	if _, exists := a.users[user.ID]; !exists {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindNotFound,
			nil,
			fmt.Sprintf("user with ID %q not found", user.ID),
		)
	}

	// Copy user to prevent external mutation
	userCopy := *user
	journalEntry(ctx, a.users, user.ID)
	a.users[user.ID] = &userCopy

	return nil
}

// DeleteUser deletes a user entity by ID.
// Satisfies ports.UserRepository interface.
// Idempotent: safe to delete non-existent users.
func (a *Adapter) DeleteUser(ctx context.Context, userID id.UserID) error {
	guard, gateErr := a.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return storage.NewStorageError(
			"DeleteUser",
			storage.ErrorKindTimeout,
			ctx.Err(),
			"operation cancelled or timed out",
		)
	default:
	}

	if userID.IsZero() {
		return storage.NewStorageError(
			"DeleteUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Remove if exists (idempotent - no error if not found)
	journalEntry(ctx, a.users, userID)
	delete(a.users, userID)

	return nil
}

// ListUsers retrieves multiple user entities matching optional filter.
// Satisfies ports.UserRepository interface.
// Returns empty slice if no users match (not an error).
func (a *Adapter) ListUsers(ctx context.Context, filter *ports.UserFilter) ([]*ports.User, error) {
	guard, gateErr := a.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	// Check context for cancellation
	select {
	case <-ctx.Done():
		return nil, storage.NewStorageError(
			"ListUsers",
			storage.ErrorKindTimeout,
			ctx.Err(),
			"operation cancelled or timed out",
		)
	default:
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var results []*ports.User

	// If no filter, return all users
	if filter == nil {
		for _, user := range a.users {
			userCopy := *user
			results = append(results, &userCopy)
		}
		return results, nil
	}

	// Apply email filter if specified
	for _, user := range a.users {
		// If email pattern specified, apply basic matching
		if filter.EmailPattern != "" {
			// Simple substring matching (not SQL LIKE)
			if user.Email != filter.EmailPattern {
				continue
			}
		}

		userCopy := *user
		results = append(results, &userCopy)
	}

	// Apply offset and limit
	if filter.Offset > 0 && filter.Offset < len(results) {
		results = results[filter.Offset:]
	} else if filter.Offset > 0 {
		return []*ports.User{}, nil
	}

	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}

	return results, nil
}
