# In-Memory Storage Adapter

## Overview

The in-memory storage adapter provides ephemeral, development-focused storage using thread-safe Go maps with `sync.RWMutex` synchronization.

**Use Cases:**
- Development and testing environments
- Rapid prototyping without database setup
- CI/CD test suites with isolated data
- Educational demonstrations

**Characteristics:**
- **Instant Initialization**: Zero I/O, returns in nanoseconds
- **No Persistence**: All data lost on application restart
- **Thread-Safe**: Concurrent read/write operations with RWMutex
- **No External Dependencies**: Pure Go implementation
- **Low Latency**: Sub-microsecond operations

## Performance

Typical operation latencies (measured on M3 Max):
- **Create**: ~172 ns/op
- **Get**: ~42.5 ns/op
- **List** (1000 items): ~29 µs/op

All operations complete well under configured timeouts (5s read, 10s write).

## Limitations

1. **No Persistence**: Data is lost when the application terminates
2. **No Data Durability Guarantees**: No write-ahead logging or crash recovery
3. **In-Process Only**: Cannot be shared across multiple processes or instances
4. **Memory Growth**: No built-in eviction or memory limits
5. **No Transaction Support**: Operations are atomic but not transactional

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

The adapter uses `sync.RWMutex` for synchronization:

- **Write Operations** (Create, Update, Delete): Exclusive lock
  - Only one writer at a time
  - Blocks all readers during modification

- **Read Operations** (Get, List): Shared lock
  - Multiple concurrent readers allowed
  - Blocked during write operations

This model provides excellent performance for read-heavy workloads typical in development/testing.

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
adapter1 := memory.NewAdapter()
adapter2 := memory.NewAdapter()

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
    adapter := memory.NewAdapter()
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
- [Hexagonal Architecture Pattern](../../../../docs/ARCHITECTURE.md)
- [Implementation Tests](./adapter_test.go)
