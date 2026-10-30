# Storage Layer Extension Guide

This guide explains how to add a storage backend to the Agentic Identity Broker.

## Overview

The storage layer uses hexagonal architecture. Ports define the contracts. Adapters implement
the contracts without changing domain logic. The project supports these backends:

- **Memory** for development and testing.
- **PostgreSQL** for production.

This guide uses MongoDB, Redis, and DynamoDB as example backends.

## Architecture Overview

```
┌─────────────────────────────────────────────────────┐
│            Domain Layer                             │
│     (User entities, business logic)                 │
└──────────────┬──────────────────────┬───────────────┘
               │ implements            │ implements
      ┌────────▼────────┐    ┌────────▼──────────┐
      │   Ports         │    │ StorageLifecycle  │
      │ (interfaces)    │    │ UserRepository    │
      └────────┬────────┘    └───────────────────┘
               │ adapts to
      ┌────────▼──────────────────┐
      │   Adapter                 │
      │ (Your new backend)        │
      └───────┬────────────────────┘
              │
      ┌───────▼────────────┐
      │ Actual Database    │
      │ (Driver/SDK)       │
      └────────────────────┘
```

## Step 1: Understand the Port Interfaces

Location: [internal/ports/storage.go](../internal/ports/storage.go)

### StorageLifecycle Interface
```go
type StorageLifecycle interface {
    Initialize(ctx context.Context) error
    HealthCheck(ctx context.Context) error
    Close(ctx context.Context) error
}
```

**Responsibilities:**

- `Initialize()` connects to the backend, makes sure that the schema exists, and creates
  connection pools.
- `HealthCheck()` makes sure that the backend responds.
- `Close()` closes connections and releases resources.

### UserRepository Interface
```go
type UserRepository interface {
    CreateUser(ctx context.Context, user *User) error
    GetUser(ctx context.Context, id string) (*User, error)
    UpdateUser(ctx context.Context, user *User) error
    DeleteUser(ctx context.Context, id string) error
    ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}
```

**Responsibilities:**

- Implement CRUD operations for user entities.
- Filter and paginate with `UserFilter`.
- Return domain errors that use `storage.ErrorKind`.

## Step 2: Create Your Adapter Package

Create a new directory for your adapter:

```bash
mkdir -p internal/adapters/storage/mybackend
touch internal/adapters/storage/mybackend/adapter.go
touch internal/adapters/storage/mybackend/adapter_test.go
```

## Step 3: Implement the Adapter

### Basic Structure

```go
package mybackend

import (
    "context"
    "fmt"
    "time"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Adapter implements ports.StorageLifecycle and ports.UserRepository
type Adapter struct {
    config   *ports.StorageConfig
    timeouts ports.StorageTimeouts
    // Add your backend-specific fields (client, connection pool, etc.)
}

// NewAdapter creates and validates a new adapter
func NewAdapter(config *ports.StorageConfig) (*Adapter, error) {
    if config == nil {
        return nil, storage.NewStorageError(
            "NewAdapter",
            storage.ErrorKindValidation,
            nil,
            "configuration cannot be nil",
        )
    }

    if config.Backend != "mybackend" {
        return nil, storage.NewStorageError(
            "NewAdapter",
            storage.ErrorKindValidation,
            nil,
            fmt.Sprintf("invalid backend: %s", config.Backend),
        )
    }

    // Validate backend-specific config
    // ...

    timeouts := config.Timeouts
    if timeouts.Read == 0 {
        timeouts.Read = 5 * time.Second
    }
    if timeouts.Write == 0 {
        timeouts.Write = 10 * time.Second
    }

    return &Adapter{
        config:   config,
        timeouts: timeouts,
    }, nil
}
```

### Implementing StorageLifecycle

```go
// Initialize connects to your backend
func (a *Adapter) Initialize(ctx context.Context) error {
    // Check context
    select {
    case <-ctx.Done():
        return storage.NewStorageError(
            "Initialize",
            storage.ErrorKindTimeout,
            ctx.Err(),
            "initialization cancelled or timed out",
        )
    default:
    }

    // Connect to your backend
    // Verify schema/collections/tables exist
    // Set up connection pools
    // Test connection with health check

    return nil
}

func (a *Adapter) HealthCheck(ctx context.Context) error {
    // Verify backend is responsive
    // Check connection pool health
    return nil
}

func (a *Adapter) Close(ctx context.Context) error {
    // Gracefully close connections
    // Cleanup resources
    return nil
}
```

### Implementing UserRepository

```go
func (a *Adapter) CreateUser(ctx context.Context, user *ports.User) error {
    // Validate input
    if user == nil {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindValidation,
            nil,
            "user cannot be nil",
        )
    }

    if user.ID == "" {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindValidation,
            nil,
            "user ID cannot be empty",
        )
    }

    // Create context with timeout
    execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
    defer cancel()

    // Execute create operation
    // Map backend-specific errors to storage.ErrorKind

    return nil
}

// Similar implementations for GetUser, UpdateUser, DeleteUser, ListUsers
```

## Step 4: Handle Errors Properly

Always wrap backend-specific errors in `storage.StorageError`:

```go
// Map backend errors to error kinds
if err != nil {
    if isConnectionError(err) {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindConnection,
            err,
            "failed to create user",
        )
    } else if isDuplicateKeyError(err) {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindConflict,
            err,
            fmt.Sprintf("user with ID %q already exists", user.ID),
        )
    } else if isTimeoutError(err) {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindTimeout,
            err,
            "operation exceeded timeout",
        )
    } else if isNotFoundError(err) {
        return storage.NewStorageError(
            "CreateUser",
            storage.ErrorKindNotFound,
            err,
            fmt.Sprintf("user with ID %q not found", user.ID),
        )
    }
}
```

## Step 5: Update the Factory

Location: [internal/adapters/storage/factory.go](../internal/adapters/storage/factory.go)

Add your backend to the factory:

```go
func NewAdapter(config *ports.StorageConfig) (*Adapter, error) {
    switch config.Backend {
    case "memory":
        return newMemoryAdapter(config)
    case "postgres":
        return newPostgresAdapter(config)
    case "mybackend":  // Add this
        return newMyBackendAdapter(config)
    default:
        return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
    }
}
```

## Step 6: Write tests

Write tests that cover at least these behaviors:

### Unit tests

- Configuration validation
- Initialization success and error paths
- Connection errors
- CRUD operation success and error paths
- Context timeouts
- Concurrent operations
- Input validation

### Integration tests

- Backend connection lifecycle
- Data persistence after restart
- Transaction behavior
- Connection-pool behavior

Example test structure:

```go
func TestMyBackendAdapter_Initialize_Success(t *testing.T) {
    config := &ports.StorageConfig{
        Backend: "mybackend",
        // ... backend-specific config
        Timeouts: ports.StorageTimeouts{
            Read:  5 * time.Second,
            Write: 10 * time.Second,
        },
    }

    adapter, err := mybackend.NewAdapter(config)
    require.NoError(t, err)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    err = adapter.Initialize(ctx)
    if err != nil {
        t.Fatalf("Initialize() = %v, want nil", err)
    }

    defer adapter.Close(ctx)
}

func TestMyBackendAdapter_CreateUser_Duplicate(t *testing.T) {
    // Setup adapter and initialize
    // ...

    user := &ports.User{
        ID:        "user123",
        Email:     "test@example.com",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    err := adapter.CreateUser(ctx, user)
    require.NoError(t, err)

    // Try to create duplicate
    err = adapter.CreateUser(ctx, user)
    if storErr, ok := err.(*storage.StorageError); ok {
        assert.Equal(t, storage.ErrorKindConflict, storErr.Kind)
    }
}
```

## Step 7: Security considerations

- Do not record connection strings in plaintext.
- Redact credentials when the backend uses credentials.
- Use environment variables for sensitive configuration.
- Use TLS for production connections.
- Validate input before sending it to the backend.
- Wrap backend errors without exposing internal data.

Example credential handling:

```go
// internal/domain/storage/connection.go (if needed for your backend)
type MyBackendConnectionParams struct {
    ConnectionURL string
}

func (cp *MyBackendConnectionParams) Redacted() string {
    // Mask sensitive parts of connection URL
    return "mybackend://user@host:port/database"
}
```

## Step 8: Configuration Support

Add configuration validation to [internal/config/validator.go](../internal/config/validator.go):

```go
// Add backend-specific validation
case "mybackend":
    if config.MyBackend == nil {
        return NewValidationError("storage.mybackend configuration is required for mybackend backend")
    }
    // Validate backend-specific fields
```

Add example configuration file:

```yaml
# config.mybackend.yaml
storage:
  backend: mybackend
  mybackend:
    connection_url: ${IDENTITY_BROKER_STORAGE_MYBACKEND_URL:mybackend://localhost:port/database}
    # Add backend-specific options
  timeouts:
    read: 5s
    write: 10s
```

## Step 9: Documentation

- Add the backend to the README supported-backend list.
- Document configuration options and examples.
- Document environment variables.
- Add troubleshooting information.
- Document performance characteristics.
- List dependency versions.

## Step 10: Code quality checklist

Before you submit a PR, run:

```bash
# Format code
gofmt -s -w internal/adapters/storage/mybackend/

# Run linter
golangci-lint run internal/adapters/storage/mybackend/

# Run tests with race detection
go test -race ./internal/adapters/storage/mybackend/...

# Generate coverage report
go test -cover ./internal/adapters/storage/mybackend/...

# Verify integration
go test ./...
```

All checks must pass before merge.

## Example: Add a Redis adapter

Add a Redis backend with these steps:

1. Create `internal/adapters/storage/redis/adapter.go`.
2. Implement `StorageLifecycle` for connection, health, and close operations.
3. Implement `UserRepository` CRUD operations.
4. Create `adapter_test.go` with behavior tests.
5. Add the Redis backend to `factory.go`.
6. Add configuration validation.
7. Add `config.redis.yaml` as an example.
8. Run `go test -race ./...`.
9. Document the backend in `STORAGE_EXTENSION_GUIDE.md`.

## Support

For a question or problem:

1. Examine the memory and PostgreSQL adapters for patterns.
2. Read `internal/domain/storage/error.go` for error handling.
3. Read `internal/ports/storage.go` for interface definitions.
4. Read `SECURITY.md` for the security checklist.

## Performance benchmarks

A production-ready adapter includes benchmarks for:

- Create operation latency
- Get operation latency
- List operation latency with 1000 items
- Concurrent operation throughput

Run benchmarks:

```bash
go test -bench=. -benchmem ./internal/adapters/storage/mybackend/
```

## References

- Current Memory Adapter: `internal/adapters/storage/memory/`
- PostgreSQL Adapter: `internal/adapters/storage/postgres/`
- Storage Architecture ADR: `adrs/20251215-120300-storage-layer-architecture.md`
- Security Checklist: `SECURITY.md`
