// Package contracts defines the storage port interfaces for the persistence layer.
// These are hexagonal architecture ports - domain logic depends on these interfaces,
// NOT on concrete implementations.
//
// This file serves as a contract specification and should NOT be compiled.
// Actual implementation will be in internal/ports/storage.go
//
// Interface Design Philosophy:
// Following Go best practices and Rob Pike's proverb "the bigger the interface,
// the weaker the abstraction", we use small, focused interfaces instead of a
// monolithic StoragePort. This follows patterns from the Go standard library:
//   - io.Reader, io.Writer, io.Closer (not monolithic IO interface)
//   - database/sql.DB, sql.Tx, sql.Stmt, sql.Rows (not monolithic Database)
//   - net/http.Handler, http.RoundTripper (not monolithic HTTP)
//
// Benefits:
//   - Interface Segregation Principle: consumers depend only on what they use
//   - Better testing: mock UserRepository (5 methods), not entire storage (20+)
//   - Clear boundaries: each repository represents a domain aggregate
//   - Extensibility: add ProductRepository without touching UserRepository
package contracts

import (
	"context"
	"time"
)

// ============================================================================
// Lifecycle Management
// ============================================================================

// StorageLifecycle manages storage backend lifecycle operations.
//
// This interface handles initialization, health checking, and cleanup.
// Separated from data operations following Interface Segregation Principle.
//
// Implementations:
//   - internal/adapters/storage/memory: In-memory adapter
//   - internal/adapters/storage/postgres: PostgreSQL adapter
//
// Lifecycle:
//  1. Create adapter via factory: storage.NewAdapter(config)
//  2. Call Initialize(ctx) - fails startup if error (FR-007)
//  3. Perform operations via repository interfaces
//  4. Call HealthCheck(ctx) periodically to verify backend health
//  5. Call Close(ctx) during graceful shutdown
//
// Example Usage:
//
//	adapter, err := storage.NewAdapter(config)
//	if err != nil {
//	    log.Fatal("Failed to create storage adapter", err)
//	}
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	if err := adapter.Lifecycle().Initialize(ctx); err != nil {
//	    log.Fatal("Storage initialization failed", err) // Abort startup
//	}
//	defer adapter.Lifecycle().Close(context.Background())
type StorageLifecycle interface {
	// Initialize performs storage backend initialization and verification.
	//
	// For in-memory backend:
	//   - Initializes empty data structures (maps)
	//   - Completes in <10ms (SC-001: <1s startup)
	//   - Never fails (unless context cancelled)
	//
	// For PostgreSQL backend:
	//   - Establishes connection to database using configured connection URL
	//   - Verifies connection with ping/health check
	//   - Checks schema version matches expected version (FR-013)
	//   - Returns error with migration instructions if schema outdated
	//   - Typically completes in 100-500ms
	//
	// Context:
	//   - Respects context timeout (recommended: 30s for startup)
	//   - Respects context cancellation (returns context.Canceled)
	//
	// Returns:
	//   - nil on success (backend ready for operations)
	//   - StorageError on failure:
	//     - ErrorKindConnection: Cannot connect to PostgreSQL
	//     - ErrorKindValidation: Schema version mismatch
	//     - ErrorKindTimeout: Initialization exceeded timeout
	//     - ErrorKindUnknown: Other failures
	//
	// Error Messages:
	//   - MUST be actionable (include remediation steps)
	//   - MUST NOT expose credentials or sensitive connection details
	//   - Example: "PostgreSQL schema mismatch (expected: v3, found: v2). Run: ./bin/agentic-identity-broker migrate up"
	//
	// Specification:
	//   - FR-006: System MUST initialize storage backend during startup
	//   - FR-007: System MUST fail startup if initialization fails
	//   - FR-013: PostgreSQL MUST verify schema version
	//   - SC-001: In-memory initialization <1 second
	//   - SC-006: PostgreSQL failures prevent inconsistent startup state
	Initialize(ctx context.Context) error

	// Close gracefully closes storage backend connections and releases resources.
	//
	// For in-memory backend:
	//   - Clears data structures (optional, helps GC)
	//   - Marks adapter as closed
	//   - Completes immediately (<1ms)
	//
	// For PostgreSQL backend:
	//   - Closes connection pool
	//   - Waits for in-flight queries (respects context timeout)
	//   - Releases database handles
	//   - Typically completes in 10-100ms
	//
	// Context:
	//   - Respects context timeout (recommended: 10s for shutdown)
	//   - Best-effort cleanup even if context cancelled
	//
	// Returns:
	//   - nil on successful cleanup
	//   - StorageError if cleanup encounters errors
	//   - Even on error, adapter should be considered closed
	//
	// Idempotency:
	//   - Calling Close() multiple times is safe
	//   - Second call returns nil or previous error
	//
	// Usage:
	//   - MUST be called during application shutdown
	//   - Typically deferred after successful Initialize()
	//   - Should use a separate context (not request contexts)
	//
	// Example:
	//
	//	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	//	defer cancel()
	//	if err := adapter.Lifecycle().Close(shutdownCtx); err != nil {
	//	    log.Warn("Storage cleanup failed", err) // Log but continue shutdown
	//	}
	Close(ctx context.Context) error

	// HealthCheck verifies the storage backend is operational.
	//
	// For in-memory backend:
	//   - Always returns nil (no external dependencies)
	//   - Completes in <1ms
	//
	// For PostgreSQL backend:
	//   - Executes ping/health query (SELECT 1)
	//   - Verifies connection pool has available connections
	//   - Checks for connection pool exhaustion
	//   - Typically completes in 5-20ms
	//
	// Context:
	//   - Respects context timeout (recommended: 5s for health checks)
	//   - Respects context cancellation
	//
	// Returns:
	//   - nil if backend is healthy and operational
	//   - StorageError if backend is unhealthy:
	//     - ErrorKindConnection: Cannot reach PostgreSQL
	//     - ErrorKindTimeout: Health check exceeded timeout
	//     - ErrorKindUnknown: Other health issues
	//
	// Usage:
	//   - Called by HTTP health check endpoints
	//   - May be called frequently (every 10-30s)
	//   - MUST be lightweight (no heavy operations)
	//
	// Idempotency:
	//   - Safe to call repeatedly
	//   - Does not modify state
	//
	// Specification:
	//   - Used to verify SC-006 (startup health)
	//   - Enables operational monitoring
	HealthCheck(ctx context.Context) error
}

// ============================================================================
// Repository Interfaces (one per entity)
// ============================================================================

// UserRepository defines storage operations for user entities.
//
// This interface follows the Repository pattern with focused CRUD operations.
// Separated from other entities following Interface Segregation Principle.
//
// Design Philosophy:
//   - Small interface (5 methods) easier to mock and test
//   - Services that only need user operations don't depend on products
//   - Follows Go proverb: "Accept interfaces, return structs"
//   - Pattern used by domain services: depend on UserRepository, not concrete adapter
//
// All methods:
//   - Accept context.Context for timeout and cancellation
//   - Return domain.StorageError (never adapter-specific errors)
//   - Are thread-safe (concurrent calls supported)
//   - Respect storage timeouts from configuration (5s read, 10s write)
//
// Example Usage:
//
//	type UserService struct {
//	    users ports.UserRepository  // Depends on interface, not concrete adapter
//	}
//
//	func (s *UserService) GetUser(ctx context.Context, id string) (*User, error) {
//	    return s.users.GetUser(ctx, id)  // Uses interface method
//	}
//
//	// Testing: mock only UserRepository (5 methods), not entire storage
//	func TestUserService(t *testing.T) {
//	    mockUsers := &MockUserRepository{}
//	    service := &UserService{users: mockUsers}
//	    // ... test with focused mock
//	}
type UserRepository interface {
	// CreateUser inserts a new user into storage.
	//
	// Behavior:
	//   - Validates user data via domain rules
	//   - Generates timestamps if not provided
	//   - Assigns unique ID if not provided
	//   - Stores user in backend
	//
	// Timeout:
	//   - Write timeout applies (default: 10s from config)
	//   - Context deadline enforced
	//
	// Returns:
	//   - nil on success
	//   - StorageError on failure:
	//     - ErrorKindConflict: User with same ID/email already exists
	//     - ErrorKindValidation: User data invalid (missing required fields)
	//     - ErrorKindTimeout: Operation exceeded timeout
	//     - ErrorKindConnection: Backend unreachable
	//
	// Idempotency:
	//   - Not idempotent: calling twice with same data returns Conflict
	//
	// Example:
	//
	//	user := &storage.User{
	//	    Email:     "user@example.com",
	//	    CreatedAt: time.Now(),
	//	}
	//	if err := repo.CreateUser(ctx, user); err != nil {
	//	    if errors.Is(err, storage.ErrorKindConflict) {
	//	        return fmt.Errorf("user already exists")
	//	    }
	//	    return err
	//	}
	CreateUser(ctx context.Context, user *User) error

	// GetUser retrieves a user by unique identifier.
	//
	// Behavior:
	//   - Looks up user by ID
	//   - Returns copy of user data (prevents external mutation)
	//
	// Timeout:
	//   - Read timeout applies (default: 5s from config)
	//   - Context deadline enforced
	//
	// Returns:
	//   - User and nil on success
	//   - nil and StorageError on failure:
	//     - ErrorKindNotFound: User with ID does not exist
	//     - ErrorKindTimeout: Operation exceeded timeout
	//     - ErrorKindConnection: Backend unreachable
	//
	// Idempotency:
	//   - Idempotent: same result for repeated calls
	//
	// Example:
	//
	//	user, err := repo.GetUser(ctx, "user-123")
	//	if err != nil {
	//	    if errors.Is(err, storage.ErrorKindNotFound) {
	//	        return nil, fmt.Errorf("user not found")
	//	    }
	//	    return nil, err
	//	}
	GetUser(ctx context.Context, id string) (*User, error)

	// UpdateUser modifies an existing user.
	//
	// Behavior:
	//   - Validates updated user data via domain rules
	//   - Updates all user fields (full replacement)
	//   - Updates UpdatedAt timestamp automatically
	//
	// Timeout:
	//   - Write timeout applies (default: 10s from config)
	//   - Context deadline enforced
	//
	// Returns:
	//   - nil on success
	//   - StorageError on failure:
	//     - ErrorKindNotFound: User with ID does not exist
	//     - ErrorKindValidation: Updated data invalid
	//     - ErrorKindTimeout: Operation exceeded timeout
	//     - ErrorKindConnection: Backend unreachable
	//
	// Idempotency:
	//   - Idempotent: repeated calls with same data have same result
	//
	// Example:
	//
	//	user.Email = "newemail@example.com"
	//	if err := repo.UpdateUser(ctx, user); err != nil {
	//	    if errors.Is(err, storage.ErrorKindNotFound) {
	//	        return fmt.Errorf("cannot update non-existent user")
	//	    }
	//	    return err
	//	}
	UpdateUser(ctx context.Context, user *User) error

	// DeleteUser removes a user by unique identifier.
	//
	// Behavior:
	//   - Removes user from storage permanently
	//   - Cascading deletes handled by backend (if configured)
	//
	// Timeout:
	//   - Write timeout applies (default: 10s from config)
	//   - Context deadline enforced
	//
	// Returns:
	//   - nil on success (including if user already deleted)
	//   - StorageError on failure:
	//     - ErrorKindTimeout: Operation exceeded timeout
	//     - ErrorKindConnection: Backend unreachable
	//
	// Idempotency:
	//   - Idempotent: deleting already-deleted user returns nil
	//
	// Example:
	//
	//	if err := repo.DeleteUser(ctx, "user-123"); err != nil {
	//	    return fmt.Errorf("failed to delete user: %w", err)
	//	}
	DeleteUser(ctx context.Context, id string) error

	// ListUsers returns users matching filter criteria.
	//
	// Behavior:
	//   - Queries users matching filter
	//   - Applies pagination (Limit/Offset)
	//   - Returns empty slice if no matches
	//   - Returns copies of user data
	//
	// Timeout:
	//   - Read timeout applies (default: 5s from config)
	//   - Context deadline enforced
	//
	// Filter:
	//   - EmailContains: Substring match (case-insensitive)
	//   - CreatedAfter: Users created after timestamp
	//   - CreatedBefore: Users created before timestamp
	//   - Limit: Max results (default: 100, max: 1000)
	//   - Offset: Skip N results (for pagination)
	//
	// Returns:
	//   - Slice of users and nil on success
	//   - nil and StorageError on failure:
	//     - ErrorKindTimeout: Operation exceeded timeout
	//     - ErrorKindConnection: Backend unreachable
	//     - ErrorKindValidation: Invalid filter parameters
	//
	// Idempotency:
	//   - Idempotent: same filter returns same results
	//
	// Example:
	//
	//	filter := &UserFilter{
	//	    EmailContains: "example.com",
	//	    Limit:         10,
	//	    Offset:        0,
	//	}
	//	users, err := repo.ListUsers(ctx, filter)
	//	if err != nil {
	//	    return nil, err
	//	}
	ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}

// ProductRepository defines storage operations for product entities.
//
// Separate from UserRepository following Interface Segregation Principle.
// Services that only need products don't depend on user operations.
//
// Future extension - not part of 20251215-120100-persistence-layer initial scope.
// Included here to show extensibility pattern.
type ProductRepository interface {
	CreateProduct(ctx context.Context, product *Product) error
	GetProduct(ctx context.Context, id string) (*Product, error)
	UpdateProduct(ctx context.Context, product *Product) error
	DeleteProduct(ctx context.Context, id string) error
	ListProducts(ctx context.Context, filter *ProductFilter) ([]*Product, error)
}

// ============================================================================
// Factory and Adapter (returns struct implementing multiple interfaces)
// ============================================================================

// Adapter composes all storage port interfaces.
//
// This struct is returned by the factory and implements multiple interfaces.
// Consumers access specific interfaces via accessor methods.
//
// Design Philosophy:
//   - Factory pattern: "Accept interfaces, return structs"
//   - Composition over inheritance
//   - Single adapter implements all interfaces
//   - Consumers depend on specific interfaces, not entire adapter
//
// Implementation location: internal/adapters/storage/factory.go
type Adapter struct {
	// Internal fields (not exposed)
	// Actual adapters (memory or postgres) stored here
}

// NewAdapter creates a storage adapter based on configuration.
//
// Parameters:
//   - config: Storage configuration (backend type, connection params, timeouts)
//
// Returns:
//   - *Adapter implementing all storage interfaces
//   - Error if configuration is invalid or adapter cannot be created
//
// Backend Selection:
//   - config.Backend == "memory": Returns in-memory adapter
//   - config.Backend == "postgres": Returns PostgreSQL adapter
//   - Other values: Returns error
//
// Example:
//
//	adapter, err := storage.NewAdapter(config.Storage)
//	if err != nil {
//	    return fmt.Errorf("failed to create storage adapter: %w", err)
//	}
//
//	// Initialize lifecycle
//	if err := adapter.Lifecycle().Initialize(ctx); err != nil {
//	    return fmt.Errorf("storage initialization failed: %w", err)
//	}
//
//	// Use specific repositories
//	user, err := adapter.Users().GetUser(ctx, id)
//
// Implementation location: internal/adapters/storage/factory.go
//
// func NewAdapter(config *StorageConfig) (*Adapter, error)

// Lifecycle returns the storage lifecycle interface.
// Use this for initialization, health checks, and cleanup.
//
// func (a *Adapter) Lifecycle() StorageLifecycle

// Users returns the user repository interface.
// Use this for all user-related storage operations.
//
// func (a *Adapter) Users() UserRepository

// Products returns the product repository interface.
// Use this for all product-related storage operations.
// Future extension - not part of 20251215-120100-persistence-layer initial scope.
//
// func (a *Adapter) Products() ProductRepository

// ============================================================================
// Supporting Types (defined in domain and ports packages)
// ============================================================================

// User represents a user entity (domain type).
// Implementation location: internal/domain/storage/user.go
type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserFilter defines query filters for listing users.
// Implementation location: internal/ports/storage.go
type UserFilter struct {
	EmailContains string
	CreatedAfter  time.Time
	CreatedBefore time.Time
	Limit         int // Default: 100, Max: 1000
	Offset        int // For pagination
}

// Product represents a product entity (domain type).
// Implementation location: internal/domain/storage/product.go
type Product struct {
	ID          string
	Name        string
	Description string
	Price       float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProductFilter defines query filters for listing products.
// Implementation location: internal/ports/storage.go
type ProductFilter struct {
	NameContains string
	MinPrice     float64
	MaxPrice     float64
	Limit        int
	Offset       int
}

// StorageError wraps storage operation errors in domain-friendly form.
// All repository methods return *StorageError (or nil on success).
//
// Fields:
//   - Operation: Human-readable operation name (e.g., "CreateUser", "GetProduct")
//   - Cause: Wrapped underlying error (adapter-specific, but wrapped)
//   - Message: User-friendly error description (actionable, no impl details)
//   - Kind: Error classification (Connection, Timeout, Validation, etc.)
//
// Methods:
//   - Error() string: Implements error interface
//   - Unwrap() error: Returns wrapped cause (for errors.Is, errors.As)
//   - Is(target error) bool: Supports errors.Is comparisons
//
// Implementation location: internal/domain/storage/errors.go
//
// type StorageError struct {
//     Operation string
//     Kind      ErrorKind
//     Cause     error
//     Message   string
// }
//
// func NewStorageError(operation string, kind ErrorKind, cause error, message string) *StorageError

// StorageConfig defines storage layer configuration.
// Implementation location: internal/ports/config.go
//
// type StorageConfig struct {
//     Backend  string              `mapstructure:"backend" validate:"required,oneof=memory postgres"`
//     Postgres PostgresConfig      `mapstructure:"postgres"`
//     Timeouts StorageTimeouts     `mapstructure:"timeouts" validate:"required"`
// }
