# Data Model: Dual-Port HTTP Server

**Feature**: 20251215-120000-dual-port-server | **Date**: 2025-12-15

## Overview

This document defines the domain entities, configuration structures, and state models for the dual-port HTTP server feature.

## Domain Entities

### ServerConfig

Configuration structure for both HTTP servers.

**Location**: `internal/ports/config.go` (extends existing `Config` struct)

**Structure**:
```go
type ServerConfig struct {
    EndUser  ServerInstanceConfig `mapstructure:"enduser" validate:"required"`
    Admin    ServerInstanceConfig `mapstructure:"admin" validate:"required"`
    Shutdown ShutdownConfig       `mapstructure:"shutdown" validate:"required"`
}
```

**Attributes**:
| Field    | Type                   | Required | Validation | Description |
|----------|------------------------|----------|------------|-------------|
| EndUser  | ServerInstanceConfig   | Yes      | required   | End-user server configuration |
| Admin    | ServerInstanceConfig   | Yes      | required   | Admin server configuration |
| Shutdown | ShutdownConfig         | Yes      | required   | Graceful shutdown settings |

**Relationships**:
- Embedded in root `Config` struct
- Used by `Manager` to initialize servers
- Validated at application startup

**Invariants**:
- EndUser and Admin ports must be different
- Both ports must be in range 1-65535
- Bind addresses must be valid IPv4, IPv6, or hostnames

---

### ServerInstanceConfig

Configuration for a single HTTP server instance.

**Location**: `internal/ports/config.go`

**Structure**:
```go
type ServerInstanceConfig struct {
    Port int    `mapstructure:"port" validate:"required,min=1,max=65535"`
    Bind string `mapstructure:"bind" validate:"required"`
}
```

**Attributes**:
| Field | Type   | Required | Validation       | Default | Description |
|-------|--------|----------|------------------|---------|-------------|
| Port  | int    | Yes      | min=1, max=65535 | 8000 (enduser), 14000 (admin) | TCP port number |
| Bind  | string | Yes      | valid IP/hostname | "::" (IPv6 dual-stack) | Network bind address |

**Validation Rules**:
1. Port must be between 1 and 65535 (inclusive)
2. Ports <1024 may require elevated privileges (logged as warning)
3. Bind address must be:
   - Valid IPv4 address (e.g., "0.0.0.0", "127.0.0.1", "192.168.1.1")
   - Valid IPv6 address (e.g., "::", "::1", "2001:db8::1")
   - Valid hostname (resolved at startup)
   - Empty string defaults to "::" on IPv6 systems, "0.0.0.0" on IPv4-only

**State Transitions**: None (immutable after loading)

---

### ShutdownConfig

Graceful shutdown timeout configuration.

**Location**: `internal/ports/config.go`

**Structure**:
```go
type ShutdownConfig struct {
    Timeout time.Duration `mapstructure:"timeout" validate:"required,min=1s,max=5m"`
}
```

**Attributes**:
| Field   | Type          | Required | Validation | Default | Description |
|---------|---------------|----------|------------|---------|-------------|
| Timeout | time.Duration | Yes      | min=1s, max=5m | 30s | Maximum wait time for graceful shutdown |

**Validation Rules**:
- Minimum: 1 second
- Maximum: 5 minutes
- Parsed from strings like "30s", "1m", "90s"

**Behavior**:
- After timeout expires, remaining connections are force-closed
- Applies to both servers independently during shutdown

---

### ServerPort (Interface)

Port interface defining HTTP server operations.

**Location**: `internal/ports/server.go` (NEW)

**Interface**:
```go
type ServerPort interface {
    // Start initializes and starts the HTTP server.
    // Blocks until server is shut down or context is cancelled.
    // Returns error if binding fails or server encounters fatal error.
    Start(ctx context.Context) error

    // Shutdown initiates graceful shutdown with timeout.
    // Waits for in-flight requests to complete or timeout to expire.
    // Returns error if shutdown fails or is forced due to timeout.
    Shutdown(timeout time.Duration) error

    // HealthStatus returns current server health state.
    HealthStatus() HealthState
}
```

**Methods**:

**Start(ctx context.Context) error**:
- Binds to configured address and port
- Starts accepting HTTP connections
- Blocks until shutdown or fatal error
- Cancellable via context

**Shutdown(timeout time.Duration) error**:
- Stops accepting new connections immediately
- Waits for active requests to complete
- Force-closes connections after timeout
- Returns nil on clean shutdown, error on timeout or failure

**HealthStatus() HealthState**:
- Returns current server health state
- Called by health check endpoint handler
- Non-blocking, fast (<1ms)

---

### HealthState (Enum)

Server health status enumeration.

**Location**: `internal/ports/server.go`

**Definition**:
```go
type HealthState int

const (
    HealthStateStarting HealthState = iota  // Server initializing
    HealthStateHealthy                       // Server accepting requests
    HealthStateShuttingDown                  // Graceful shutdown in progress
    HealthStateUnhealthy                     // Server error state
)

func (h HealthState) String() string {
    return [...]string{"starting", "healthy", "shutting_down", "unhealthy"}[h]
}

func (h HealthState) HTTPStatus() int {
    switch h {
    case HealthStateHealthy:
        return http.StatusOK  // 200
    case HealthStateStarting, HealthStateShuttingDown, HealthStateUnhealthy:
        return http.StatusServiceUnavailable  // 503
    default:
        return http.StatusServiceUnavailable
    }
}
```

**States**:
| State          | HTTP Status | Meaning |
|----------------|-------------|---------|
| Starting       | 503         | Server is initializing, not yet ready |
| Healthy        | 200         | Server is accepting and processing requests normally |
| ShuttingDown   | 503         | Graceful shutdown initiated, in-flight requests completing |
| Unhealthy      | 503         | Server encountered error, not accepting requests |

**State Transitions**:
```
Starting → Healthy → ShuttingDown → (stopped)
Starting → Unhealthy → (stopped)
Healthy → Unhealthy → (stopped)
```

**Invariants**:
- State changes are one-way (no back-transitions except restart)
- Once ShuttingDown, server never returns to Healthy
- Unhealthy is terminal (requires restart)

---

### Manager

Domain entity coordinating dual-server lifecycle.

**Location**: `internal/domain/server/manager.go`

**Structure**:
```go
type Manager struct {
    enduserServer ServerPort
    adminServer   ServerPort
    shutdownTimeout time.Duration
    logger        *slog.Logger
}
```

**Attributes**:
| Field           | Type          | Description |
|-----------------|---------------|-------------|
| enduserServer   | ServerPort    | End-user server instance |
| adminServer     | ServerPort    | Admin server instance |
| shutdownTimeout | time.Duration | Graceful shutdown timeout |
| logger          | *slog.Logger  | Structured logger |

**Methods**:

**Start(ctx context.Context) error**:
- Starts both servers concurrently using errgroup
- Atomic: if either server fails, both are stopped
- Blocks until both servers shut down or context cancelled
- Returns first error encountered during startup

**Shutdown(ctx context.Context) error**:
- Initiates graceful shutdown on both servers
- Runs shutdowns concurrently (parallel)
- Waits for both to complete or timeout
- Aggregates errors from both shutdowns

**State Management**:
- No internal state (delegates to ServerPort instances)
- Coordination only (startup, shutdown)

**Invariants**:
- Both servers must start successfully (atomic startup)
- Both servers shut down independently (parallel)
- Manager does not hold references after shutdown completes

---

### HealthResponse

HTTP response structure for health check endpoints.

**Location**: `internal/adapters/http/health.go`

**Structure**:
```go
type HealthResponse struct {
    Status         string    `json:"status"`
    Server         string    `json:"server"`
    Timestamp      time.Time `json:"timestamp"`
    UptimeSeconds  int64     `json:"uptime_seconds"`
}
```

**Attributes**:
| Field          | Type      | Description |
|----------------|-----------|-------------|
| Status         | string    | Health state: "healthy", "starting", "shutting_down", "unhealthy" |
| Server         | string    | Server identifier: "enduser" or "admin" |
| Timestamp      | time.Time | ISO 8601 timestamp of health check |
| UptimeSeconds  | int64     | Seconds since server started |

**Example**:
```json
{
  "status": "healthy",
  "server": "enduser",
  "timestamp": "2025-12-15T10:30:00Z",
  "uptime_seconds": 3600
}
```

**HTTP Status Codes**:
- 200: Status is "healthy"
- 503: Status is "starting", "shutting_down", or "unhealthy"

---

## Configuration Defaults

**Default Values**:
```go
func DefaultServerConfig() ServerConfig {
    return ServerConfig{
        EndUser: ServerInstanceConfig{
            Port: 8000,
            Bind: "::",  // Dual-stack (IPv6 with IPv4 fallback)
        },
        Admin: ServerInstanceConfig{
            Port: 14000,
            Bind: "::",
        },
        Shutdown: ShutdownConfig{
            Timeout: 30 * time.Second,
        },
    }
}
```

**Override Precedence** (highest to lowest):
1. CLI flags: `--server.enduser.port 8000`
2. Environment variables: `IDENTITY_BROKER_SERVER_ENDUSER_PORT=8000`
3. YAML configuration: `server.enduser.port: 8000`
4. Code defaults: `8000` (enduser), `14000` (admin)

---

## Validation Rules

### Cross-Field Validation

Performed after individual field validation, before server startup:

```go
func (c *ServerConfig) Validate() error {
    // Ports must be different
    if c.EndUser.Port == c.Admin.Port {
        return fmt.Errorf("enduser and admin ports must be different (both are %d)", c.EndUser.Port)
    }

    // Bind addresses must be parseable
    if err := validateBindAddress(c.EndUser.Bind); err != nil {
        return fmt.Errorf("invalid enduser bind address: %w", err)
    }
    if err := validateBindAddress(c.Admin.Bind); err != nil {
        return fmt.Errorf("invalid admin bind address: %w", err)
    }

    return nil
}

func validateBindAddress(addr string) error {
    // Check if valid IPv4
    if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
        return nil
    }

    // Check if valid IPv6
    if ip := net.ParseIP(addr); ip != nil && ip.To16() != nil {
        return nil
    }

    // Check if valid hostname (DNS lookup at startup)
    if _, err := net.LookupHost(addr); err == nil {
        return nil
    }

    return fmt.Errorf("not a valid IPv4, IPv6, or resolvable hostname: %s", addr)
}
```

---

## Domain Glossary

Add to `ARCHITECTURE.md` Glossary section:

**EndUserServer**: HTTP server instance dedicated to serving end-user traffic on configurable port (default 8000), operating independently from AdminServer with separate health status and lifecycle.

**AdminServer**: HTTP server instance dedicated to serving administrative traffic on configurable port (default 14000), operating independently from EndUserServer with separate health status and lifecycle.

**ServerConfig**: Configuration structure containing port numbers, bind addresses, and shutdown timeout settings for both EndUser and Admin servers.

**BindAddress**: Network interface specification (IPv4 address, IPv6 address, or hostname) that determines which network interfaces a server accepts connections from; supports dual-stack operation when bound to :: on IPv6-capable systems.

**Dual-Stack**: Network configuration where a single server socket accepts both IPv4 and IPv6 connections; IPv4 clients are mapped to IPv6 addresses (::ffff:192.0.2.1 format).

**Atomic Startup**: Server initialization strategy where both servers must bind and start successfully, or the entire application fails to start with no partial service state.

**Graceful Shutdown**: Server termination process that stops accepting new connections, waits for active requests to complete (up to timeout), then force-closes remaining connections.

**Health State**: Server operational status enumeration (Starting, Healthy, ShuttingDown, Unhealthy) exposed via health check endpoints for monitoring and load balancer integration.

---

## Entity Relationship Diagram

```
┌─────────────────┐
│     Config      │
│  (root config)  │
└────────┬────────┘
         │ has-a
         ▼
┌─────────────────┐
│  ServerConfig   │
├─────────────────┤
│ • EndUser       │
│ • Admin         │
│ • Shutdown      │
└────────┬────────┘
         │ has-a (2x)
         ▼
┌──────────────────────┐
│ ServerInstanceConfig │
├──────────────────────┤
│ • Port: int          │
│ • Bind: string       │
└──────────────────────┘

┌─────────────────┐
│    Manager      │
│ (domain logic)  │
├─────────────────┤
│ • Start()       │
│ • Shutdown()    │
└────────┬────────┘
         │ coordinates (2x)
         ▼
┌─────────────────┐
│   ServerPort    │
│  (interface)    │
├─────────────────┤
│ • Start()       │
│ • Shutdown()    │
│ • HealthStatus()│
└────────┬────────┘
         │ implemented-by
         ▼
┌─────────────────┐
│  chi.Server     │
│ (HTTP adapter)  │
├─────────────────┤
│ • router        │
│ • httpServer    │
│ • healthState   │
└─────────────────┘
         │ returns
         ▼
┌─────────────────┐
│  HealthState    │
│    (enum)       │
├─────────────────┤
│ • Starting      │
│ • Healthy       │
│ • ShuttingDown  │
│ • Unhealthy     │
└─────────────────┘
```

---

## State Diagram

**Server Lifecycle States**:

```
[Application Start]
        ↓
   [Starting]  ──(bind fails)──→ [Unhealthy] → [Stopped]
        │
    (bind success)
        ↓
    [Healthy] ──(runtime error)──→ [Unhealthy] → [Stopped]
        │
  (SIGTERM/SIGINT)
        ↓
 [ShuttingDown] ──(timeout or complete)──→ [Stopped]
```

**State Properties**:
- **Starting**: Binding to port, initializing router, not accepting requests yet
- **Healthy**: Accepting connections, processing requests normally
- **ShuttingDown**: No new connections, waiting for in-flight requests to complete
- **Unhealthy**: Fatal error occurred, not accepting requests, restart required
- **Stopped**: Server terminated, resources released (not a HealthState, final state)

---

## Testing Considerations

### Unit Test Scenarios

**Configuration Validation**:
- Valid configurations (default, custom ports, custom bind addresses)
- Invalid configurations (duplicate ports, out-of-range ports, invalid bind addresses)
- Cross-field validation (same port for both servers)

**Health State Transitions**:
- Starting → Healthy
- Healthy → ShuttingDown → Stopped
- Starting → Unhealthy → Stopped

### Integration Test Scenarios

**Atomic Startup**:
- Both servers start successfully
- One server fails to bind (port already in use), both stop
- One server bind address invalid, both fail

**Health Endpoints**:
- GET /health returns 200 when Healthy
- GET /health returns 503 when ShuttingDown
- Response includes correct server identifier ("enduser" vs "admin")

**Graceful Shutdown**:
- In-flight requests complete within timeout
- New requests rejected after shutdown initiated
- Force-close after timeout expires

---

## Performance Characteristics

**Memory**:
- ServerConfig: ~100 bytes per instance
- Manager: ~200 bytes + 2 server references
- HealthState: 4 bytes (int enum)
- Total: <500 bytes for configuration and coordination structures

**CPU**:
- Configuration validation: <1ms at startup
- Health check handler: <100μs per request
- State transitions: <10μs (atomic operations)

**Network**:
- TCP binding: <100ms per server
- Health check response: ~200 bytes JSON
- Concurrent connections: Limited by OS (typically 10k+ per server)

