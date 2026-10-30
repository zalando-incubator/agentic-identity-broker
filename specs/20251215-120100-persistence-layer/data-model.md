# Data Model: Persistence Layer

**Feature**: 20251215-120100-persistence-layer
**Date**: 2025-12-15
**Status**: Design

## Overview

This document defines the domain entities and types for the persistence layer. All types are technology-agnostic and represent pure domain concepts. Implementation-specific details (SQL schemas, database drivers) are isolated in adapters.

## Domain Entities

### 1. StorageBackend (Enumeration)

**Purpose**: Identifies the active storage mechanism at runtime.

**Values**:
- `memory`: In-memory storage using Go maps (development/testing only)
- `postgres`: PostgreSQL database (production)

**Validation Rules**:
- MUST be one of the defined values
- Case-insensitive matching (normalized to lowercase)
- Empty string or unknown values are invalid

**Usage**: Selected via configuration (`storage.backend` key)

**Type Definition**:
```go
type StorageBackend string

const (
    StorageBackendMemory   StorageBackend = "memory"
    StorageBackendPostgres StorageBackend = "postgres"
)

// Valid returns true if the backend type is recognized
func (b StorageBackend) Valid() bool

// String returns the string representation
func (b StorageBackend) String() string
```

---

### 2. ConnectionParameters

**Purpose**: Encapsulates PostgreSQL-specific connection details without exposing implementation.

**Attributes**:
- **ConnectionURL** (string, required when backend=postgres):
  - PostgreSQL connection string in URL format
  - Format: `postgresql://[user[:password]@][host][:port][/dbname][?param=value&...]`
  - Contains sensitive data (credentials) - must be redacted in logs

**Validation Rules**:
- Required when `StorageBackend` is `postgres`
- Must start with `postgresql://` or `postgres://`
- Must contain host component
- Must contain database name
- URL query parameters validated for known keys
- Common parameters: `sslmode`, `connect_timeout`, `application_name`

**Relationships**:
- Used by `StorageConfig` when `Backend == StorageBackendPostgres`
- Parsed by PostgreSQL adapter, never exposed to domain logic

**Type Definition**:
```go
type ConnectionParameters struct {
    ConnectionURL string `mapstructure:"connection_url" validate:"required_if=Backend postgres,postgresql_url"`
}

// Redacted returns a safe version for logging (passwords masked)
func (cp ConnectionParameters) Redacted() string

// Validate performs connection string validation
func (cp ConnectionParameters) Validate() error
```

---

### 3. StorageTimeouts

**Purpose**: Defines timeout durations for storage operations to prevent indefinite hangs.

**Attributes**:
- **Read** (time.Duration, required):
  - Timeout for read operations (queries, fetches)
  - Default: 5 seconds (from clarifications)
  - Minimum: 1 second
  - Maximum: 60 seconds

- **Write** (time.Duration, required):
  - Timeout for write operations (inserts, updates, deletes)
  - Default: 10 seconds (from clarifications)
  - Minimum: 1 second
  - Maximum: 120 seconds

**Validation Rules**:
- Both timeouts MUST be positive durations
- Write timeout SHOULD be >= Read timeout (warning if violated)
- Values parsed from configuration (supports `5s`, `10s`, `1m` formats)

**Usage**: Applied to context.WithTimeout in adapter methods

**Type Definition**:
```go
type StorageTimeouts struct {
    Read  time.Duration `mapstructure:"read" validate:"required,min=1s,max=60s"`
    Write time.Duration `mapstructure:"write" validate:"required,min=1s,max=120s"`
}

// Validate checks timeout constraints
func (st StorageTimeouts) Validate() error
```

---

### 4. StorageConfig

**Purpose**: Complete storage layer configuration combining backend selection and parameters.

**Attributes**:
- **Backend** (StorageBackend, required):
  - Selected storage backend
  - Determines which adapter to instantiate

- **Postgres** (ConnectionParameters, conditional):
  - PostgreSQL connection details
  - Required when `Backend == StorageBackendPostgres`
  - Ignored when `Backend == StorageBackendMemory`

- **Timeouts** (StorageTimeouts, required):
  - Operation timeout settings
  - Applied to all backends

**Validation Rules**:
- `Backend` must be valid (memory or postgres)
- If `Backend == postgres`, `Postgres.ConnectionURL` is required
- If `Backend == memory`, `Postgres` is optional/ignored
- `Timeouts` validated per StorageTimeouts rules

**Relationships**:
- Part of application `Config` struct (in `internal/ports/config.go`)
- Used by storage factory to create appropriate adapter
- Loaded via existing `ConfigPort` interface

**Type Definition**:
```go
type StorageConfig struct {
    Backend  string              `mapstructure:"backend" validate:"required,oneof=memory postgres"`
    Postgres PostgresConfig      `mapstructure:"postgres"`
    Timeouts StorageTimeouts     `mapstructure:"timeouts" validate:"required"`
}

// Validate performs comprehensive validation
func (sc StorageConfig) Validate() error

// GetBackend returns the StorageBackend enum
func (sc StorageConfig) GetBackend() (StorageBackend, error)
```

---

### 5. StorageError (Domain Error Type)

**Purpose**: Wraps storage operation errors in domain-friendly form without exposing adapter internals.

**Attributes**:
- **Operation** (string):
  - Operation that failed (e.g., "Initialize", "CreateEntity", "UpdateEntity")
  - Human-readable operation name

- **Cause** (error):
  - Underlying error (wrapped)
  - May be adapter-specific (PostgreSQL, memory) but wrapped

- **Message** (string):
  - User-friendly error description
  - Does NOT expose SQL, connection strings, or implementation details

- **Kind** (ErrorKind):
  - Classification of error type
  - Values: `ConnectionError`, `TimeoutError`, `ValidationError`, `NotFoundError`, `ConflictError`

**Validation Rules**:
- N/A (error type, not validated)

**Relationships**:
- Returned by all `StoragePort` methods
- Adapters wrap implementation-specific errors into `StorageError`
- Domain logic catches `StorageError` without knowing adapter details

**Type Definition**:
```go
type ErrorKind string

const (
    ErrorKindConnection  ErrorKind = "connection"
    ErrorKindTimeout     ErrorKind = "timeout"
    ErrorKindValidation  ErrorKind = "validation"
    ErrorKindNotFound    ErrorKind = "not_found"
    ErrorKindConflict    ErrorKind = "conflict"
    ErrorKindUnknown     ErrorKind = "unknown"
)

type StorageError struct {
    Operation string
    Cause     error
    Message   string
    Kind      ErrorKind
}

// Error implements error interface
func (se *StorageError) Error() string

// Unwrap returns the wrapped error
func (se *StorageError) Unwrap() error

// Is supports errors.Is comparisons
func (se *StorageError) Is(target error) bool

// NewStorageError creates a new storage error
func NewStorageError(operation string, kind ErrorKind, cause error, message string) *StorageError
```

---

## Entity Relationships

```
Config (ports.Config)
  └── StorageConfig
        ├── Backend (enum: memory | postgres)
        ├── Postgres (ConnectionParameters)
        │     └── ConnectionURL (string)
        └── Timeouts (StorageTimeouts)
              ├── Read (duration)
              └── Write (duration)

StoragePort (interface)
  └── returns → StorageError
                  ├── Operation (string)
                  ├── Cause (error)
                  ├── Message (string)
                  └── Kind (ErrorKind enum)
```

## State Transitions

### Storage Backend Lifecycle

```
[Configured] → Initialize() → [Initialized] → HealthCheck() → [Healthy]
                     ↓                              ↓
                  [Failed]                      [Unhealthy]
                     ↓                              ↓
              (Startup Aborted)            (Retry/Recover)

[Initialized/Healthy] → Close() → [Closed]
```

**States**:
- **Configured**: Config loaded, adapter created, not yet initialized
- **Initialized**: `Initialize()` succeeded, ready for operations
- **Healthy**: `HealthCheck()` confirms operational state
- **Unhealthy**: `HealthCheck()` detects issues (connection lost, resource exhaustion)
- **Failed**: `Initialize()` failed, cannot proceed (startup aborted per FR-007)
- **Closed**: `Close()` called, connections released, no further operations

**Transitions**:
- **Configure → Initialize**: Application startup, 30s timeout
- **Initialize → Failed**: Connection failure, invalid config, schema mismatch
- **Initialize → Initialized**: Successful initialization
- **Initialized → Healthy**: First HealthCheck succeeds
- **Healthy → Unhealthy**: HealthCheck detects failure (transient or permanent)
- **Unhealthy → Healthy**: HealthCheck recovers (transient issue resolved)
- **Any → Closed**: Application shutdown, graceful cleanup

## Validation Summary

| Entity | Validation Rules | Validated By | Failure Action |
|--------|-----------------|--------------|----------------|
| StorageBackend | Must be "memory" or "postgres" | Config validator | Startup fail |
| ConnectionParameters | URL format, required components | Config validator + adapter | Startup fail |
| StorageTimeouts | Positive durations, min/max bounds | Config validator | Startup fail |
| StorageConfig | Backend-specific parameter requirements | Config validator | Startup fail |
| StorageError | N/A (error type) | N/A | Returned to caller |

## Configuration Examples

### In-Memory Configuration

```yaml
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
```

### PostgreSQL Configuration

```yaml
storage:
  backend: postgres
  postgres:
    connection_url: "postgresql://agentic_identity_broker:${DB_PASSWORD}@db.example.com:5432/agentic_identity_broker?sslmode=verify-full&application_name=agentic-identity-broker"
  timeouts:
    read: 5s
    write: 10s
```

**Note**: `${DB_PASSWORD}` is substituted via existing environment variable expansion with security validation.

## Data Volume Assumptions

Per specification assumptions:
- Single application instance (no distributed coordination)
- In-memory storage: Development/testing workloads (< 10,000 entities)
- PostgreSQL storage: Production workloads (millions of entities)
- No explicit row limits enforced by storage layer
- Pagination/limits implemented by consumers of `StoragePort`

## Performance Characteristics

| Backend | Initialize Time | Read Latency | Write Latency | Concurrency | Persistence |
|---------|----------------|--------------|---------------|-------------|-------------|
| Memory | <10ms | <1ms | <1ms | sync.RWMutex | None (ephemeral) |
| PostgreSQL | 100-500ms | 5-50ms | 10-100ms | Connection pool (25 max) | Durable |

**Notes**:
- Initialize time includes connection establishment and schema validation
- Latencies exclude timeout enforcement (5s read, 10s write max)
- Concurrency limits: Memory (Go runtime), PostgreSQL (connection pool)
- Performance targets from success criteria: SC-001 (<1s startup), SC-007 (timeout enforcement)

## Adapter Interface Contract

While adapters implement `StoragePort`, they must adhere to these domain constraints:

1. **Error Wrapping**: All adapter-specific errors MUST be wrapped in `StorageError`
2. **Timeout Enforcement**: All operations MUST respect context deadlines from timeouts
3. **Initialization Failure**: `Initialize()` errors MUST be actionable (clear message, cause)
4. **Schema Verification**: PostgreSQL adapter MUST check schema version in `Initialize()`
5. **Graceful Cleanup**: `Close()` MUST release resources even if errors occur
6. **Health Transparency**: `HealthCheck()` MUST accurately reflect backend state

## Future Extensions

This data model is designed to support future enhancements:

**Planned Extensions** (out of scope for 20251215-120100-persistence-layer):
- Entity-specific repository methods (Create, Read, Update, Delete)
- Transaction support (BeginTx, Commit, Rollback)
- Query builders for complex filters
- Pagination and sorting abstractions
- Caching layer integration
- Read replicas and write-read splitting
- Connection pool tuning parameters
- Observability (metrics, tracing)

**Extension Points**:
- `StoragePort` interface can be extended with new methods
- `StorageConfig` can add new fields for additional backends
- `StorageError.Kind` enum can be extended for new error classifications
- `ConnectionParameters` can add optional fields without breaking changes

## References

- **Specification**: [spec.md](spec.md)
- **Implementation Plan**: [plan.md](plan.md)
- **Research**: [research.md](research.md)
- **Constitution**: [../../.specify/memory/constitution.md](../../.specify/memory/constitution.md) (Principle VI: Hexagonal Architecture)

---

**Document Status**: Complete - Ready for contract definition (contracts/) and implementation (tasks.md)
