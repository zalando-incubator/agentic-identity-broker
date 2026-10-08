# In-Memory Storage Adapter

## Overview

The in-memory adapter provides ephemeral storage with shared transaction coordination and store-local `sync.RWMutex` locks.

**Use Cases:**
- Development and testing environments
- Rapid prototyping without database setup
- CI/CD test suites with isolated data
- Educational demonstrations

**Characteristics:**
- **Instant Initialization**: Zero I/O, returns in nanoseconds
- **No Persistence**: All data lost on application restart
- **Thread-Safe**: Shared visibility gates and store-local locks
- **No Infrastructure**: No database or external service required
- **Transactions**: Owner/join scopes, rollback journals, and commit-only effects

## Performance

Historical measurements before shared transaction coordination (M3 Max):
- **Create**: ~172 ns/op
- **Get**: ~42.5 ns/op
- **List** (1000 items): ~29 µs/op

These measurements do not describe the current transaction implementation. Feature 048 records current performance separately.

## Limitations

1. **No Persistence**: Data is lost when the application terminates
2. **No Data Durability Guarantees**: No write-ahead logging or crash recovery
3. **In-Process Only**: Cannot be shared across multiple processes or instances
4. **Memory Growth**: No built-in eviction or memory limits

## Configuration

```yaml
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
```

No additional parameters required for the memory backend.

## Concurrency Model

The factory injects one `TransactionManager` into every repository.
Manually composed repositories must receive the same coordinator to participate in one transaction.

Standalone operations acquire the lifecycle gate, then the visibility gate, then store-local locks.
Readers share visibility access. Writers acquire exclusive visibility access.
Composite reads propagate an operation context to prevent gate reentry.

`BeginTX` holds exclusive visibility until the owning scope commits or rolls back.
Joined commits do not publish state. Joined rollback makes the owner rollback-only.
Rollback restores touched records and indexes in reverse order without a whole-store snapshot.
Returned mutable values are independent copies.
Commit-only effects run after the owner releases the gates.

## Thread Safety

All operations are thread-safe:

```go
// Safe concurrent reads
var wg sync.WaitGroup
for i := 0; i < 1000; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        adapter.GetUser(ctx, "user123")
    }()
}
wg.Wait() // All reads complete safely
```

## Data Isolation

Each adapter instance maintains separate data:

```go
adapter1 := memory.NewAdapter(memory.NewTransactionManager())
adapter2 := memory.NewAdapter(memory.NewTransactionManager())

// Data created in adapter1 is not visible to adapter2
adapter1.CreateUser(ctx, user)
_, err := adapter2.GetUser(ctx, user.ID) // Returns NotFound
```

## Error Handling

All operations return domain-specific `StorageError`:

- **ValidationError**: Invalid input (nil user, empty ID)
- **ConflictError**: Duplicate user ID during create
- **NotFoundError**: User doesn't exist during get/update
- **TimeoutError**: Context cancelled or deadline exceeded

## Testing

The adapter includes comprehensive tests:

- **Unit Tests**: Table-driven tests for all operations
- **Concurrent Tests**: 100+ simultaneous operations
- **Isolation Tests**: Separate adapter instances
- **Context Tests**: Cancellation and timeout handling
- **Benchmarks**: Performance profiling

Run tests with race detection:
```bash
go test -race ./internal/adapters/storage/memory/...
```

## Example Usage

```go
package main

import (
    "context"
    "time"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func main() {
    // Create adapter
    adapter := memory.NewAdapter(memory.NewTransactionManager())
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    // Initialize (no-op for in-memory)
    if err := adapter.Initialize(ctx); err != nil {
        panic(err)
    }
    defer adapter.Close(ctx)

    // Create user
    user := &ports.User{
        ID:        "user123",
        Email:     "test@example.com",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }
    if err := adapter.CreateUser(ctx, user); err != nil {
        panic(err)
    }

    // Get user
    retrieved, err := adapter.GetUser(ctx, "user123")
    if err != nil {
        panic(err)
    }
    println("Retrieved:", retrieved.Email)

    // List all users
    all, _ := adapter.ListUsers(ctx, nil)
    println("Total users:", len(all))

    // Delete user
    adapter.DeleteUser(ctx, "user123")
}
```

## Migration Path

When moving from in-memory to PostgreSQL:

1. No code changes needed (same `UserRepository` interface)
2. Update configuration: change `backend: memory` to `backend: postgres`
3. Provide PostgreSQL connection URL in config
4. Run database migrations
5. Restart application

The hexagonal architecture ensures storage backend switching requires only configuration changes.

## References

- [Storage Layer Documentation](../../docs/storage.md)
- [Hexagonal Architecture Pattern](../../../../ARCHITECTURE.md)
- [Implementation Tests](./adapter_test.go)
