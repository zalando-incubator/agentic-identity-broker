# Implementation Plan: Persistence Layer

**Branch**: `20251215-120100-persistence-layer` | **Date**: 2025-12-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251215-120100-persistence-layer/spec.md`

## Summary

Add a persistence layer to the application with two storage backend options: in-memory (for development/testing) and PostgreSQL (for production). The implementation follows hexagonal architecture with clear port/adapter separation. Domain logic will depend on storage ports (interfaces), with concrete adapters for each backend. All database specifics (SQL, PostgreSQL libraries) remain isolated in adapters and do not leak into the domain layer. The system uses sqlx for PostgreSQL implementation, providing compile-time query validation and efficient parameter binding.

## Technical Context

**Language/Version**: Go 1.23.0+
**Primary Dependencies**: sqlx v1.3.5+ (PostgreSQL adapter), PostgreSQL Go driver (pgx v5)
**Storage**: In-memory (maps with sync.RWMutex), PostgreSQL 12+
**Testing**: Go standard library testing, testify/assert for assertions, table-driven tests
**Target Platform**: Linux server, containerized (Docker), deployable to Kubernetes/AWS ECS
**Project Type**: Single project (backend service)
**Performance Goals**:
- In-memory startup <1 second (SC-001)
- Support 1,000+ concurrent storage operations (SC-005)
- Read operations <5 seconds, writes <10 seconds (SC-007)

**Constraints**:
- PostgreSQL connection strings must follow postgresql:// URL format (FR-009)
- Exactly one storage backend active at runtime (FR-003)
- In-memory storage has no memory limits (development/testing only)
- PostgreSQL connection pool managed by driver defaults

**Scale/Scope**:
- Single application instance (no distributed storage coordination)
- Generic storage layer (specific entity schemas defined by consumers)
- Foundation for future entity-specific repositories

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

- [x] **Security-First**: PostgreSQL credentials redacted in logs (SR-001), TLS/SSL support (SR-002), fail closed on verification failure (SR-003)
- [x] **Architecture Docs**: ARCHITECTURE.md will be updated with persistence layer subsystem, glossary entries for storage ports/adapters
- [x] **ADRs**: ADR required for storage backend selection and port/adapter design
- [x] **Library-First Security**: Using vetted PostgreSQL drivers (pgx) and sqlx for database access (no custom database protocols)
- [x] **API Documentation**: docs/ will be updated with storage configuration guide and usage examples
- [x] **Domain Model**: New domain concepts (StoragePort, StorageBackend, ConnectionParameters) will be added to ARCHITECTURE.md Glossary
- [x] **Hexagonal Architecture**: Domain logic depends on StoragePort interface; in-memory and PostgreSQL are driven adapters implementing the port
- [x] **Configuration-Driven**: Storage backend selection uses existing ConfigPort; new StorageConfig added to ports.Config struct
- [x] **TDD & Automated Testing**: Unit tests for both adapters, integration tests for PostgreSQL adapter, table-driven tests for validation logic

*All checks pass. No violations requiring justification.*

### Post-Design Re-Check (After Phase 1)

- [ ] Domain model documented in ARCHITECTURE.md Glossary
- [ ] ADR created for storage architecture decisions
- [ ] Ports properly defined with no implementation leakage
- [ ] Configuration integration follows existing patterns
- [ ] Test strategy defined for all components

## Project Structure

### Documentation (this feature)

```text
specs/20251215-120100-persistence-layer/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── storage_port.go  # StoragePort interface definition
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── ports/
│   ├── config.go          # Existing - will add StorageConfig
│   └── storage.go         # NEW - StoragePort interface
│
├── domain/
│   └── storage/           # NEW - storage domain types
│       ├── errors.go      # Storage-specific errors
│       ├── backend.go     # BackendType enum
│       └── connection.go  # Connection parameter types
│
├── adapters/
│   └── storage/           # NEW - storage adapters
│       ├── memory/        # In-memory adapter
│       │   ├── adapter.go # Memory implementation of StoragePort
│       │   └── adapter_test.go
│       │
│       └── postgres/      # PostgreSQL adapter
│           ├── adapter.go # PostgreSQL implementation of StoragePort
│           ├── migrations.go # Schema migration handling
│           ├── adapter_test.go
│           └── integration_test.go
│
└── config/
    ├── schema.go          # Existing - will add storage config
    └── validator.go       # Existing - will add storage validators

test/
└── integration/
    └── storage/           # NEW - integration tests
        └── postgres_test.go

examples/
└── config/
    ├── storage-memory.yaml  # NEW - in-memory example
    └── storage-postgres.yaml # NEW - PostgreSQL example

docs/
└── storage.md            # NEW - storage layer documentation

adrs/
└── 20251215-120300-storage-layer-architecture.md  # NEW - ADR for storage decisions
```

**Structure Decision**: Single project structure maintained. New storage components follow existing hexagonal architecture pattern with clear port (interface) in `internal/ports/` and concrete adapters in `internal/adapters/storage/`. Domain types isolated in `internal/domain/storage/` to prevent leakage of adapter implementation details.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations. All constitution requirements are met.*

## Phase 0: Research & Technology Decisions

**Goal**: Resolve all technical unknowns and establish implementation patterns.

### Research Tasks

1. **sqlx Integration Pattern**
   - How to integrate sqlx with existing hexagonal architecture
   - Connection pool management best practices
   - Query timeout implementation (5s reads, 10s writes)
   - Error handling patterns for database operations

2. **PostgreSQL Schema Migration Strategy**
   - Approach for verifying schema version on startup (FR-013)
   - Migration instruction format for error messages
   - Integration with startup lifecycle

3. **In-Memory Concurrency Patterns**
   - Thread-safe map patterns using sync.RWMutex
   - Lock granularity for read vs write operations
   - Test isolation strategies for concurrent tests

4. **Configuration Integration**
   - Extending existing ConfigPort with storage configuration
   - Connection string parsing and validation
   - SSL/TLS parameter handling in connection URLs

5. **Testing Strategy**
   - Unit test patterns for both adapters
   - Integration test setup for PostgreSQL (testcontainers?)
   - Mock/stub patterns for testing domain logic

**Output**: `research.md` documenting decisions for all areas above

## Phase 1: Design & Contracts

### 1.1 Data Model (`data-model.md`)

**Storage Domain Entities**:

1. **StorageBackend** (enum)
   - Values: `memory`, `postgres`
   - Validation: Must be one of allowed values
   - Used in configuration to select active backend

2. **ConnectionParameters**
   - ConnectionURL (string): PostgreSQL connection string
   - SSLMode (string, optional): SSL/TLS configuration
   - Validation rules: URL format, required components

3. **StorageConfig**
   - Backend (StorageBackend): Selected backend type
   - Postgres (ConnectionParameters): PostgreSQL settings (when backend=postgres)
   - Validation: Backend-specific parameter validation

4. **StorageError** (domain error type)
   - Fields: Operation, Cause, Message
   - Wraps underlying errors without exposing adapter details
   - Error classification: ConnectionError, TimeoutError, ValidationError

**Relationships**:
- StorageConfig → StorageBackend (composition)
- StorageConfig → ConnectionParameters (optional, when backend=postgres)
- StoragePort returns StorageError (not adapter-specific errors)

### 1.2 API Contracts (`contracts/storage_port.go`)

**Storage Port Interfaces** (following Interface Segregation Principle):

Go best practices recommend small, focused interfaces over monolithic ones ("the bigger the interface, the weaker the abstraction"). Following patterns from the standard library (io.Reader/Writer, database/sql.DB/Tx/Stmt), we use separate interfaces for different concerns:

**1. StorageLifecycle Interface**:
```go
// StorageLifecycle manages storage backend lifecycle operations.
// Handles initialization, health checking, and cleanup.
type StorageLifecycle interface {
	// Initialize performs storage backend initialization and verification.
	// For PostgreSQL: connects to database, verifies schema
	// For in-memory: initializes empty storage structures
	// Returns error if initialization fails (FR-007)
	Initialize(ctx context.Context) error

	// Close gracefully closes storage connections and releases resources.
	// Should be called during application shutdown
	// Idempotent: safe to call multiple times
	Close(ctx context.Context) error

	// HealthCheck verifies storage backend is operational.
	// Lightweight operation for monitoring (5-20ms)
	// Returns nil if healthy, error with details if unhealthy
	HealthCheck(ctx context.Context) error
}
```

**2. UserRepository Interface**:
```go
// UserRepository defines storage operations for user entities.
// Follows Repository pattern with focused CRUD operations.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}
```

**3. ProductRepository Interface** (future):
```go
// ProductRepository defines storage operations for product entities.
// Separate from UserRepository following Interface Segregation Principle.
type ProductRepository interface {
	CreateProduct(ctx context.Context, product *Product) error
	GetProduct(ctx context.Context, id string) (*Product, error)
	UpdateProduct(ctx context.Context, product *Product) error
	DeleteProduct(ctx context.Context, id string) error
	ListProducts(ctx context.Context, filter *ProductFilter) ([]*Product, error)
}
```

**Factory Pattern**:

```go
// Adapter composes all storage port interfaces.
// Returned by factory, implements multiple interfaces.
type Adapter struct {
	lifecycle StorageLifecycle
	users     UserRepository
	products  ProductRepository
}

// NewAdapter creates a storage adapter based on configuration.
// Returns struct that implements multiple interfaces.
func NewAdapter(config *StorageConfig) (*Adapter, error)

// Adapter exposes interfaces via accessor methods
func (a *Adapter) Lifecycle() StorageLifecycle
func (a *Adapter) Users() UserRepository
func (a *Adapter) Products() ProductRepository
```

**Design Rationale**:
- **Interface Segregation Principle**: Services depend only on interfaces they use
- **Go Idiomaticity**: Follows standard library patterns (io.Reader/Writer, not monolithic IO)
- **Testing Excellence**: Mock only UserRepository (5 methods), not entire storage (20+ methods)
- **Clear Boundaries**: Each repository represents a domain aggregate
- **Extensibility**: Add new repositories without modifying existing ones
- **Rob Pike's Proverb**: "The bigger the interface, the weaker the abstraction"

### 1.3 Quickstart Guide (`quickstart.md`)

Document the following patterns for future developers:

1. **How to Define a Storage Port Interface**
   - Small, focused interface definition in `internal/ports/`
   - One interface per entity (UserRepository, ProductRepository)
   - Separate lifecycle interface (StorageLifecycle)
   - Method signatures with context and error handling
   - Documentation requirements

2. **How to Implement an Adapter**
   - Memory adapter example with sync.RWMutex
   - PostgreSQL adapter example with sqlx
   - Implementing multiple interfaces in single adapter struct
   - Error wrapping to domain errors

3. **How to Add a New Entity Repository**
   - Define new Repository interface in `internal/ports/storage.go`
   - Add accessor method to Adapter struct (`Products() ProductRepository`)
   - Implement interface methods in both memory and postgres adapters
   - Write tests for both implementations

4. **Configuration Integration**
   - Adding entity-specific config to StorageConfig
   - Validation patterns
   - Configuration examples

5. **Testing Patterns**
   - Mock specific interfaces (UserRepository, not entire Adapter)
   - Unit tests for adapter logic
   - Integration tests for PostgreSQL
   - Table-driven test examples

### 1.4 Configuration Schema Updates

Add to `internal/ports/config.go`:

```go
type Config struct {
	Log     LogConfig     `mapstructure:"log" validate:"required"`
	Server  ServerConfig  `mapstructure:"server" validate:"required"`
	Storage StorageConfig `mapstructure:"storage" validate:"required"` // NEW
}

type StorageConfig struct {
	Backend  string              `mapstructure:"backend" validate:"required,oneof=memory postgres"`
	Postgres PostgresConfig      `mapstructure:"postgres"`
	Timeouts StorageTimeouts     `mapstructure:"timeouts"`
}

type PostgresConfig struct {
	ConnectionURL string `mapstructure:"connection_url" validate:"required_if=Backend postgres"`
}

type StorageTimeouts struct {
	Read  time.Duration `mapstructure:"read" validate:"required"`  // Default: 5s
	Write time.Duration `mapstructure:"write" validate:"required"` // Default: 10s
}
```

Configuration examples:

**storage-memory.yaml**:
```yaml
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
```

**storage-postgres.yaml**:
```yaml
storage:
  backend: postgres
  postgres:
    connection_url: "postgresql://user:password@localhost:5432/agentic_identity_broker?sslmode=require"
  timeouts:
    read: 5s
    write: 10s
```

### 1.5 Agent Context Update

Run `.specify/scripts/bash/update-agent-context.sh claude` to add:
- sqlx v1.3.5+ (PostgreSQL adapter)
- pgx v5 (PostgreSQL driver)
- Storage layer (20251215-120100-persistence-layer)

## Phase 2: Implementation Planning (Tasks)

*This phase is handled by `/speckit.tasks` command - NOT part of `/speckit.plan` output.*

## Architecture Decision Records

### ADR 20251215-120300: Storage Layer Architecture

**Status**: Proposed

**Context**:
The application requires a persistence layer supporting both in-memory (development/testing) and PostgreSQL (production) backends. The implementation must follow hexagonal architecture principles with clear separation between domain logic and infrastructure.

**Decision**:

1. **Port/Adapter Pattern with Interface Segregation**:
   - Define small, focused interfaces in `internal/ports/storage.go`:
     - `StorageLifecycle`: Initialize, Close, HealthCheck (3 methods)
     - `UserRepository`: User CRUD operations (5 methods)
     - `ProductRepository`: Product CRUD operations (5 methods per entity)
   - Implement memory adapter in `internal/adapters/storage/memory/`
   - Implement PostgreSQL adapter in `internal/adapters/storage/postgres/`
   - Both adapters implement all interfaces
   - Factory returns `*Adapter` struct with accessor methods for each interface
   - Domain logic depends only on specific repository interfaces, never concrete adapters
   - Follows Go proverb: "The bigger the interface, the weaker the abstraction"
   - Aligns with standard library patterns (io.Reader/Writer, database/sql.DB/Tx)

2. **Technology Choices**:
   - **sqlx**: Chosen over standard database/sql for compile-time query validation, named parameter binding, and struct scanning convenience
   - **pgx v5**: Chosen as PostgreSQL driver for performance, comprehensive PostgreSQL feature support, and active maintenance
   - **sync.RWMutex**: Used for in-memory adapter concurrency control (simple, sufficient for dev/test workloads)

3. **Configuration Integration**:
   - Extend existing `ConfigPort` with `StorageConfig` field
   - Reuse configuration precedence system (env vars, YAML, CLI flags)
   - Follow existing validation patterns

4. **Error Handling**:
   - Define domain-specific errors in `internal/domain/storage/errors.go`
   - Adapters wrap implementation errors into domain errors
   - No adapter-specific error types exposed to domain logic

5. **Schema Management**:
   - PostgreSQL adapter verifies schema version on startup (FR-013)
   - Fail startup with clear migration instructions if schema outdated
   - Schema migrations managed separately (out of scope for this feature)

**Consequences**:

**Positive**:
- Clean separation enables easy testing with mock implementations
- Future storage backends (Redis, MongoDB) can be added without domain changes
- sqlx provides type safety and reduces boilerplate
- Configuration consistency with existing system patterns

**Negative**:
- Additional abstraction layer adds slight complexity
- Developers must implement all port methods in both adapters when adding entities
- sqlx learning curve for developers unfamiliar with it

**Alternatives Considered**:

1. **Direct database/sql**: Rejected due to verbose error handling and lack of named parameters
2. **GORM (ORM)**: Rejected to avoid magic abstractions and maintain hexagonal architecture clarity
3. **Single adapter with runtime switching**: Rejected as it would couple domain logic to infrastructure

## Documentation Updates Required

### ARCHITECTURE.md Updates

**Section 3: Core Components** - Add subsection:

#### 3.1.2. Storage Subsystem

**Purpose**: Generic persistence layer supporting multiple storage backends with hexagonal architecture separation.

**Architecture**: Hexagonal (ports and adapters pattern)

**Components**:
- **Port** (internal/ports/storage.go): StoragePort interface defining domain boundary
- **Memory Adapter** (internal/adapters/storage/memory/): In-memory implementation using sync.RWMutex
- **PostgreSQL Adapter** (internal/adapters/storage/postgres/): PostgreSQL implementation using sqlx/pgx
- **Domain Types** (internal/domain/storage/): BackendType enum, connection parameters, storage errors

**Supported Backends**:
- **memory**: In-memory maps, no persistence, zero-configuration (development/testing only)
- **postgres**: PostgreSQL 12+ with connection pooling, TLS support, schema versioning

**Configuration**: Integrated with existing ConfigPort via StorageConfig struct

**Performance**:
- Read timeout: 5 seconds (configurable)
- Write timeout: 10 seconds (configurable)
- Connection pool: Driver defaults (pgx)

**Technologies**:
- sqlx v1.3.5+ (PostgreSQL adapter, query builder)
- pgx v5 (PostgreSQL driver)
- sync.RWMutex (in-memory concurrency control)

**Section 11: Glossary** - Add entries:

**StoragePort**: Hexagonal architecture port (interface) for data persistence. Domain logic depends on this interface, not concrete storage implementations. Located in internal/ports/storage.go.

**Storage Adapter**: Implementation of StoragePort for a specific backend (memory or PostgreSQL). Adapters handle infrastructure concerns (connection pooling, SQL, concurrency) and translate to/from domain types.

**Storage Backend**: The active persistence mechanism selected at runtime (memory or postgres). Specified via configuration backend field.

**Connection Parameters**: PostgreSQL-specific configuration including connection URL, SSL mode, and timeout settings. Encapsulated in domain type to prevent leakage.

**Schema Version**: PostgreSQL schema version number verified on startup. Mismatch triggers startup failure with migration instructions (FR-013).

### docs/storage.md (NEW)

Create comprehensive storage layer documentation:
- Supported backends and use cases
- Configuration guide with examples
- Connection string format and SSL options
- Timeout configuration
- Error handling patterns
- How to add new entity repositories
- Testing strategies

## Testing Strategy

### Unit Tests

**Memory Adapter** (`internal/adapters/storage/memory/adapter_test.go`):
- Concurrent read/write operations
- Data isolation between operations
- Error conditions (invalid parameters)
- Timeout handling

**PostgreSQL Adapter** (`internal/adapters/storage/postgres/adapter_test.go`):
- Connection string parsing
- Configuration validation
- Error wrapping to domain errors
- Mock-based tests (no real database)

### Integration Tests

**PostgreSQL Integration** (`test/integration/storage/postgres_test.go`):
- Real PostgreSQL instance (testcontainers or Docker Compose)
- Full CRUD operations
- Connection failure scenarios
- Timeout verification
- Schema version checking
- TLS/SSL connection

### Table-Driven Tests

All validation logic and error conditions tested with table-driven patterns:
```go
tests := []struct {
    name    string
    input   StorageConfig
    wantErr bool
    errMsg  string
}{
    {"valid memory", memoryConfig, false, ""},
    {"valid postgres", postgresConfig, false, ""},
    {"invalid backend", invalidConfig, true, "unsupported backend"},
    // ...
}
```

## Success Criteria Validation

Map implementation to specification success criteria:

- **SC-001**: In-memory adapter Initialize() completes in <1s ✓ (no I/O, map initialization only)
- **SC-002**: PostgreSQL adapter persists data across restarts ✓ (integration test verifies)
- **SC-003**: Backend switchable via config ✓ (factory pattern with config.Backend selection)
- **SC-004**: All initialization errors return clear messages ✓ (domain errors with context)
- **SC-005**: PostgreSQL startup failure prevents inconsistent state ✓ (Initialize() error aborts startup)
- **SC-006**: Timeout enforcement ✓ (context.WithTimeout in adapter methods)
- **SC-007**: 1,000+ concurrent operations ✓ (RWMutex for memory, connection pool for PostgreSQL)

## Implementation Notes

### Startup Sequence

1. Load configuration (existing ConfigPort)
2. Validate StorageConfig
3. Create storage adapter via factory: `NewStorageAdapter(config.Storage)`
4. Call `adapter.Initialize(ctx)` with 30s timeout
5. On error: Log clear message, exit with non-zero code (FR-007)
6. On success: Register adapter for dependency injection, proceed with server startup

### Shutdown Sequence

1. Receive SIGINT/SIGTERM
2. Stop accepting new requests
3. Wait for in-flight requests (30s timeout)
4. Call `adapter.Close(ctx)` with 10s timeout
5. Exit

### Connection String Format

PostgreSQL connection URLs follow libpq format:
```
postgresql://[user[:password]@][host][:port][/dbname][?param1=value1&...]
```

Common parameters:
- `sslmode`: disable, require, verify-ca, verify-full
- `connect_timeout`: seconds (default: driver default)
- `application_name`: identifies connection source

Example:
```
postgresql://agentic-identity-broker:secret@db.example.com:5432/agentic-identity-broker?sslmode=verify-full&application_name=agentic-identity-broker
```

### Error Message Format

Startup failure example:
```
ERROR: Storage initialization failed
Cause: PostgreSQL schema version mismatch
Expected: v3, Found: v2
Action: Run migrations with: ./bin/agentic-identity-broker migrate up
Documentation: https://docs.example.com/storage#migrations
```

## Next Steps

After this plan is reviewed:

1. Run `/speckit.tasks` to generate implementation task breakdown
2. Implement Phase 0 research (create research.md)
3. Implement Phase 1 design (data-model.md, contracts/, quickstart.md)
4. Update agent context (run update-agent-context.sh)
5. Create ADR 20251215-120300
6. Update ARCHITECTURE.md
7. Update docs/
8. Begin Phase 2 implementation tasks

**Current Status**: Plan complete, awaiting approval to proceed with `/speckit.tasks`
