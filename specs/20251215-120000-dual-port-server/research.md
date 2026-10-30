# Research: Dual-Port HTTP Server

**Feature**: 20251215-120000-dual-port-server | **Date**: 2025-12-15

## Overview

This document captures research decisions for implementing dual independent HTTP servers with IPv4/IPv6 dual-stack support, atomic startup, and graceful shutdown.

## Technology Decisions

### Decision 1: HTTP Framework Selection - chi v5

**Decision**: Use chi v5 as the HTTP routing framework for both servers.

**Rationale**:
- **Stdlib Compatible**: chi is built on `net/http` standard library, no custom server implementation needed
- **Lightweight**: Minimal abstraction, ~1000 LOC, no hidden magic or performance overhead
- **Idiomatic Go**: Follows Go idioms and patterns, aligns with project's Go-first approach
- **Context-Based**: Native support for `context.Context` for request scoping, timeouts, cancellation
- **Middleware Support**: Clean middleware chain composition for logging, recovery, metrics
- **Production Proven**: Used by many Go projects, stable API, good maintenance
- **Zero External Dependencies**: chi itself has no dependencies beyond Go stdlib

**Alternatives Considered**:
1. **gorilla/mux**: Heavier, more features than needed, larger API surface
2. **gin**: Fast but opinionated, custom context type breaks stdlib compatibility
3. **echo**: Similar to gin, custom context, more framework-like
4. **stdlib only (net/http)**: Would require manual routing logic, reinventing middleware patterns
5. **fiber**: Built on fasthttp (not net/http), incompatible with stdlib patterns

**Reference**: https://github.com/go-chi/chi

---

### Decision 2: IPv6 Dual-Stack Strategy

**Decision**: Use `::` as default bind address on IPv6-capable systems, with automatic fallback to `0.0.0.0` on IPv4-only systems.

**Rationale**:
- **RFC 4038 Compliance**: Binding to `::` with `IPV6_V6ONLY=0` (Go default) accepts both IPv4 and IPv6
- **IPv4-Mapped IPv6 Addresses**: IPv4 clients connecting to `::` are mapped to `::ffff:192.0.2.1` format
- **Automatic Detection**: Go's `net.Listen("tcp", "[::]:port")` automatically detects IPv6 capability
- **Graceful Fallback**: If IPv6 fails (not supported or disabled), fall back to `0.0.0.0` binding
- **Single Socket**: One listener handles both protocols, simpler than dual-stack with separate sockets

**Implementation Approach**:
```go
// Try IPv6 dual-stack first
listener, err := net.Listen("tcp", "[::]:"+port)
if err != nil {
    // Fallback to IPv4 only
    listener, err = net.Listen("tcp", "0.0.0.0:"+port)
    if err != nil {
        return err
    }
}
```

**Alternatives Considered**:
1. **Separate IPv4 and IPv6 listeners**: More complexity, two sockets per server (4 total)
2. **IPv4-only**: Violates requirement for IPv6 support
3. **IPv6-only**: Breaks IPv4-only environments

**Reference**:
- Go net package documentation: https://pkg.go.dev/net
- RFC 4038: Application Aspects of IPv6 Transition

---

### Decision 3: Atomic Startup Implementation

**Decision**: Use Go's `errgroup` pattern for concurrent server startup with atomic failure.

**Rationale**:
- **Concurrent Binding**: Both servers attempt to bind simultaneously (faster startup)
- **Atomic Failure**: If either server fails during startup, both are torn down
- **Context Cancellation**: Failed binding cancels the other server's startup context
- **Standard Pattern**: `golang.org/x/sync/errgroup` is idiomatic for this use case
- **Error Aggregation**: Captures errors from both goroutines for detailed failure reporting

**Implementation Pattern**:
```go
import "golang.org/x/sync/errgroup"

func (m *Manager) Start(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    g.Go(func() error {
        return m.enduserServer.Start(ctx)
    })

    g.Go(func() error {
        return m.adminServer.Start(ctx)
    })

    // Wait returns first non-nil error, cancels context for other goroutine
    return g.Wait()
}
```

**Alternatives Considered**:
1. **Sequential Startup**: Slower, one server must fully start before the other begins
2. **WaitGroup + Manual Error Handling**: More boilerplate, error-prone
3. **Channels for Coordination**: Custom synchronization, more complexity

**Reference**: https://pkg.go.dev/golang.org/x/sync/errgroup

---

### Decision 4: Graceful Shutdown Strategy

**Decision**: Use `http.Server.Shutdown(ctx)` with configurable timeout context.

**Rationale**:
- **Built-in Support**: Go 1.8+ `http.Server` has native graceful shutdown
- **Context-Based Timeout**: Clean timeout handling via `context.WithTimeout`
- **In-Flight Request Handling**: `Shutdown()` waits for active connections to complete
- **New Connection Rejection**: Immediately stops accepting new connections
- **Force Close on Timeout**: After timeout, remaining connections are force-closed

**Implementation Pattern**:
```go
func (s *Server) Shutdown(timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()

    return s.httpServer.Shutdown(ctx)
}
```

**Alternatives Considered**:
1. **Manual Connection Tracking**: Complex, error-prone, reinventing stdlib functionality
2. **Immediate Close**: Violates requirement for graceful shutdown
3. **No Timeout**: Risk of hanging indefinitely on slow clients

**Reference**: https://pkg.go.dev/net/http#Server.Shutdown

---

### Decision 5: Health Check Endpoint Design

**Decision**: Implement `/health` endpoint on each server returning JSON with status details.

**Rationale**:
- **Standard Convention**: `/health` is widely recognized health check path
- **JSON Response**: Structured format for machine parsing and human readability
- **HTTP Status Codes**: 200 (healthy), 503 (unhealthy/shutting down)
- **Separate Endpoints**: Each server independently reports its own health
- **Minimal Logic**: Health check should be fast (<10ms), no complex checks in initial implementation

**Response Format**:
```json
{
  "status": "healthy",
  "server": "enduser",
  "timestamp": "2025-12-15T10:30:00Z",
  "uptime_seconds": 3600
}
```

**Status Values**:
- `healthy`: Server is accepting requests normally
- `shutting_down`: Server received shutdown signal, still processing in-flight requests
- `unhealthy`: Server encountered an error (future: dependency health checks)

**Alternatives Considered**:
1. **Combined Health Endpoint**: Violates independence requirement
2. **HEAD Request Only**: Less informative, harder to debug
3. **Complex Health Checks**: Out of scope for initial implementation (no dependencies yet)

**Reference**:
- Google SRE Book: Health Checking
- Kubernetes Liveness/Readiness Probes

---

### Decision 6: Configuration Structure

**Decision**: Extend existing `Config` struct in `internal/ports/config.go` with nested `Server` configuration.

**Rationale**:
- **Constitution Compliance**: Must use unified configuration system (Principle VII)
- **Namespaced**: Server config under `server:` YAML key prevents collisions
- **Consistent Precedence**: Inherits CLI > env > YAML > defaults from existing system
- **Type Safety**: Go struct with validation tags enforces correctness at startup

**Configuration Schema**:
```go
type Config struct {
    Log    LogConfig    `mapstructure:"log" validate:"required"`
    Server ServerConfig `mapstructure:"server" validate:"required"`
}

type ServerConfig struct {
    EndUser  ServerInstanceConfig `mapstructure:"enduser" validate:"required"`
    Admin    ServerInstanceConfig `mapstructure:"admin" validate:"required"`
    Shutdown ShutdownConfig       `mapstructure:"shutdown" validate:"required"`
}

type ServerInstanceConfig struct {
    Port int    `mapstructure:"port" validate:"required,min=1,max=65535"`
    Bind string `mapstructure:"bind" validate:"required,ip|hostname"`
}

type ShutdownConfig struct {
    Timeout time.Duration `mapstructure:"timeout" validate:"required,min=1s,max=5m"`
}
```

**YAML Example**:
```yaml
server:
  enduser:
    port: 8000
    bind: "::"  # Dual-stack, or 0.0.0.0 for IPv4 only
  admin:
    port: 14000
    bind: "::"
  shutdown:
    timeout: 30s
```

**Environment Variables**:
- `IDENTITY_BROKER_SERVER_ENDUSER_PORT=8000`
- `IDENTITY_BROKER_SERVER_ENDUSER_BIND="::"`
- `IDENTITY_BROKER_SERVER_ADMIN_PORT=14000`
- `IDENTITY_BROKER_SERVER_ADMIN_BIND="::"`
- `IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT=30s`

**CLI Flags**:
- `--server.enduser.port 8000`
- `--server.enduser.bind "::"`
- `--server.admin.port 14000`
- `--server.admin.bind "::"`
- `--server.shutdown.timeout 30s`

**Alternatives Considered**:
1. **Flat Configuration**: No nesting, harder to namespace, collision risk
2. **Separate Config Files**: Violates unified configuration principle
3. **Custom Config Loading**: Forbidden by constitution (Principle VII)

---

## Implementation Strategy

### Hexagonal Architecture Mapping

**Ports (Interfaces)**:
- `internal/ports/server.go`: `ServerPort` interface (Start, Shutdown, HealthStatus)
- `internal/ports/config.go`: `ConfigPort` interface (existing, extend with ServerConfig)

**Adapters**:
- `internal/adapters/http/server.go`: chi-based HTTP server implementing `ServerPort`
- `internal/adapters/http/health.go`: Health check HTTP handlers
- `internal/adapters/config/loader.go`: Viper-based config loader (existing, extend)

**Domain Logic**:
- `internal/domain/server/manager.go`: Coordinates dual-server lifecycle, atomic startup
- `internal/domain/server/lifecycle.go`: Server state management (starting, running, shutting_down, stopped)

**Entry Point**:
- `cmd/agentic-identity-broker/main.go`: Initializes dependencies, starts manager, handles signals

### Testing Strategy

**Unit Tests**:
- Configuration validation (invalid ports, bind addresses, timeouts)
- Lifecycle state transitions (starting → running → shutting_down → stopped)
- Health check status logic (healthy, shutting_down, unhealthy)

**Integration Tests**:
- Server startup with valid configuration
- Atomic failure (one server fails, both stop)
- Health endpoint responses (HTTP 200, HTTP 503)
- Graceful shutdown (in-flight requests complete, new requests rejected)
- IPv4/IPv6 connectivity (bind to ::, verify both protocols work)

**Table-Driven Tests**:
- Port validation (negative, zero, out of range, valid)
- Bind address validation (IPv4, IPv6, hostnames, invalid formats)
- Configuration precedence (CLI > env > YAML > defaults)

---

## Open Questions Resolved

### Q1: Should servers share a chi.Router or have separate routers?
**Answer**: Separate routers. Each server is fully independent with its own middleware chain, routes, and lifecycle. This ensures complete isolation per security requirement SR-001.

### Q2: How to handle OS-level IPv6 disabled?
**Answer**: Attempt `::` binding first, catch error, fall back to `0.0.0.0`. Log the fallback decision for observability. This is transparent to end users.

### Q3: Should health endpoint be middleware or explicit route?
**Answer**: Explicit route. Health checks should have minimal overhead and clear registration. Middleware adds unnecessary layering for a simple status endpoint.

### Q4: How to test atomic startup without race conditions?
**Answer**: Use synchronization in tests: start servers in goroutines with errgroup, inject a "fail after bind" error into one server, verify both are torn down. Use `t.Cleanup()` for test teardown.

### Q5: Should configuration defaults be in code or config files?
**Answer**: Code. Defaults in `internal/ports/config.go` as struct field default values or in a `DefaultConfig()` function. Config files provide overrides. This keeps defaults version-controlled and testable.

---

## Dependencies

### External Packages

**chi v5** (`github.com/go-chi/chi/v5`):
- License: MIT
- Version: v5.1.0 (latest stable)
- Import path: `github.com/go-chi/chi/v5`
- Usage: HTTP routing and middleware

**errgroup** (`golang.org/x/sync/errgroup`):
- License: BSD 3-Clause (Go Authors)
- Part of: Go extended packages (official)
- Import path: `golang.org/x/sync/errgroup`
- Usage: Atomic concurrent server startup

### Stdlib Packages

- `net/http`: HTTP server and client
- `net`: Network I/O (TCP listeners, IPv4/IPv6)
- `context`: Request scoping, cancellation, timeouts
- `time`: Duration handling, timestamps
- `encoding/json`: Health check response serialization
- `log/slog`: Structured logging (existing)
- `os/signal`: Graceful shutdown signal handling

---

## Risk Assessment

### Low Risk
- **chi Framework**: Mature, stable, wide adoption, minimal API surface
- **Stdlib HTTP**: Go's `net/http` is battle-tested, well-documented
- **Configuration Extension**: Low-risk addition to existing system

### Medium Risk
- **IPv6 Dual-Stack Testing**: Requires IPv6-capable test environment (mitigated with fallback)
- **Atomic Startup Timing**: Potential for rare race conditions in startup logic (mitigated with errgroup)

### High Risk
- None identified

### Mitigation Strategies
- **IPv6 Testing**: Document IPv6 test requirements, provide Docker-based test environment with IPv6 enabled
- **Race Condition Detection**: Run tests with `go test -race` in CI pipeline
- **Integration Test Coverage**: Test all startup/shutdown scenarios (normal, error, timeout)

---

## Performance Considerations

**Startup Time**:
- Target: <5 seconds (SC-001)
- Chi router initialization: <1ms per route
- TCP binding: <100ms per socket
- Total estimated: <500ms under normal conditions

**Memory Footprint**:
- chi router: ~10KB per server (minimal state)
- HTTP server goroutines: ~2KB per connection
- Configuration struct: <1KB
- Total baseline: ~50KB for dual servers (before connections)

**Concurrency**:
- Each server handles connections independently
- Go's goroutine-per-connection model scales to 10k+ concurrent connections
- No shared state between servers (no mutex contention)

---

## Next Steps

1. Create ADRs for chi framework and atomic startup strategy
2. Implement Phase 1 artifacts (data-model.md, contracts/, quickstart.md)
3. Request golang-pro agent review of research decisions and architecture
4. Begin implementation with TDD approach (tests first)

