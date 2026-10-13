# ADR 004: Dual-Server Isolation and Atomic Startup

**Status**: Accepted
**Date**: 2025-12-15
**Feature**: 003-dual-port-server

## Context

The identity broker needs to run two independent HTTP servers (end-user on port 8000, admin on port 14000) that operate completely independently yet start atomically. If either server fails to start, the application should not proceed with partial functionality.

## Decision

We will implement **atomic startup** using Go's `errgroup` pattern, where both servers start concurrently but the application only proceeds if both servers successfully bind and initialize.

## Architecture

### Server Independence

1. **Separate Router Instances**: Each server has its own `chi.Router` with independent routes and middleware
2. **Independent Health State**: Each server maintains its own health state using atomic operations
3. **Independent Lifecycle**: Each server can transition through health states independently during runtime
4. **Separate Listeners**: Each server binds to its own port and network interface

### Atomic Startup

1. **Concurrent Binding**: Both servers attempt to bind to their ports simultaneously using `errgroup.WithContext`
2. **All-or-Nothing**: If either server fails during startup, the errgroup context is cancelled and both servers are torn down
3. **Context Cancellation**: Failed binding automatically cancels the other server's startup context
4. **Error Aggregation**: First error encountered during startup is returned to the caller

### Implementation Pattern

```go
func (m *Manager) Start(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    // Start end-user server
    g.Go(func() error {
        return m.enduserServer.Start(ctx)
    })

    // Start admin server
    g.Go(func() error {
        return m.adminServer.Start(ctx)
    })

    // Wait for both to complete or first error
    return g.Wait()
}
```

## Rationale

### Why Atomic Startup?

1. **Security Requirement (SR-001)**: Complete isolation between servers requires both to be operational
2. **Operational Clarity**: Avoids partial service state that's difficult to debug
3. **Fail-Fast**: Better to fail immediately than run with degraded functionality
4. **Predictable Behavior**: Application is either fully running or not running

### Why errgroup?

1. **Standard Pattern**: `golang.org/x/sync/errgroup` is the idiomatic way to handle this in Go
2. **Context Propagation**: Automatic context cancellation on first error
3. **Error Handling**: Clean error aggregation and propagation
4. **Goroutine Safety**: Proper synchronization without manual wait groups

### Alternatives Considered

1. **Sequential Startup**: Slower, one server must fully start before the other begins
2. **WaitGroup + Manual Error Handling**: More boilerplate, error-prone
3. **Channels for Coordination**: Custom synchronization, more complexity
4. **Independent Startup (No Atomicity)**: Violates security requirements for isolation

## Consequences

### Positive

- Clear failure mode: both servers up or application exits
- Fast startup: concurrent binding reduces startup time
- Idiomatic Go: uses standard library patterns
- Easy to test: can inject failures to verify atomic behavior

### Negative

- If one server has a transient binding issue, both servers must restart
- Cannot run in "degraded" mode (but this is by design for security)

## Implementation Notes

### Startup Sequence

1. Manager creates both server instances with configuration
2. Manager calls `Start(ctx)` which begins concurrent startup
3. Both servers attempt to bind (`Listen()`) to their ports
4. If both succeed, servers begin serving (`Serve()`)
5. If either bind fails, close the successfully bound listener before returning the error.

### Shutdown Sequence

1. Manager receives shutdown signal (SIGTERM/SIGINT)
2. Manager calls `Shutdown()` on both servers (parallel)
3. Each server independently completes in-flight requests
4. Manager waits for both shutdowns to complete
5. Application exits

### Health State Independence

While startup is atomic, health states are independent:
- End-user server can be Healthy while admin is ShuttingDown
- Servers do not coordinate health states after startup
- Each server reports its own status via `/health` endpoint

## References

- errgroup documentation: https://pkg.go.dev/golang.org/x/sync/errgroup
- Research document: specs/003-dual-port-server/research.md
- Security requirements: SR-001 (Server Isolation)
