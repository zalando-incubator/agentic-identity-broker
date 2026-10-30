# Research: Persistence Layer

**Feature**: 20251215-120100-persistence-layer | **Date**: 2025-12-15

## Overview

This document captures research decisions for implementing a hexagonal architecture persistence layer with in-memory and PostgreSQL backends using Go 1.23.0+. The design prioritizes production readiness, testability, and clear separation between domain logic and infrastructure concerns.

## Technology Decisions

### Decision 1: sqlx for PostgreSQL Integration

**Decision**: Use `github.com/jmoiron/sqlx` with `github.com/jackc/pgx/v5` driver for PostgreSQL interactions.

**Rationale**:
- **Extends database/sql**: sqlx wraps stdlib `database/sql` with ergonomic improvements while remaining fully compatible
- **Struct Scanning**: Direct scanning into structs with `sqlx.Get()` and `sqlx.Select()` reduces boilerplate
- **Named Parameters**: Support for `:name` syntax improves query readability and safety vs positional `$1, $2`
- **Mapper Flexibility**: Configurable field mapping (snake_case DB columns to PascalCase Go fields)
- **pgx v5 Performance**: jackc/pgx is the fastest, most feature-complete PostgreSQL driver for Go
- **Native pgx Features**: Connection pooling (pgxpool), prepared statements, LISTEN/NOTIFY, COPY support
- **Context-First Design**: All operations accept `context.Context` for timeout and cancellation
- **Battle-Tested**: sqlx has 16k+ stars, used in production by thousands of projects
- **Minimal Abstraction**: Stays close to SQL, no ORM magic or hidden queries

**Connection Pool Configuration**:
```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jmoiron/sqlx"
)

// Use pgxpool for connection pooling with pgx v5
poolConfig, err := pgxpool.ParseConfig(connectionString)
if err != nil {
    return fmt.Errorf("parse connection string: %w", err)
}

// Configure pool settings
poolConfig.MaxConns = 25                     // Maximum connections (based on Postgres max_connections)
poolConfig.MinConns = 5                      // Minimum idle connections
poolConfig.MaxConnLifetime = 1 * time.Hour   // Prevent stale connections
poolConfig.MaxConnIdleTime = 15 * time.Minute // Close idle connections
poolConfig.HealthCheckPeriod = 1 * time.Minute // Verify connection health

pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
if err != nil {
    return fmt.Errorf("create connection pool: %w", err)
}

// Wrap with sqlx for ergonomic querying
db := sqlx.NewDb(pool.Config().ConnConfig.Database, "pgx")
```

**Query Timeout Implementation**:
```go
// Read operation with 5 second timeout
func (r *PostgresRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    var entity Entity
    query := `SELECT id, name, created_at FROM entities WHERE id = $1`
    err := r.db.GetContext(ctx, &entity, query, id)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, ErrNotFound
        }
        return nil, fmt.Errorf("query entity: %w", err)
    }
    return &entity, nil
}

// Write operation with 10 second timeout
func (r *PostgresRepository) Create(ctx context.Context, entity *Entity) error {
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

    query := `INSERT INTO entities (id, name, created_at) VALUES ($1, $2, $3)`
    _, err := r.db.ExecContext(ctx, query, entity.ID, entity.Name, entity.CreatedAt)
    if err != nil {
        return fmt.Errorf("insert entity: %w", err)
    }
    return nil
}
```

**Error Handling Pattern**:
```go
// Wrap database errors to domain errors
func mapPostgresError(err error) error {
    if err == nil {
        return nil
    }

    // Check for specific error types
    if errors.Is(err, sql.ErrNoRows) {
        return ErrNotFound // Domain error: entity not found
    }
    if errors.Is(err, context.DeadlineExceeded) {
        return ErrTimeout // Domain error: operation timeout
    }

    // Check for pgx-specific errors
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) {
        switch pgErr.Code {
        case "23505": // unique_violation
            return ErrDuplicateKey
        case "23503": // foreign_key_violation
            return ErrConstraintViolation
        case "53300": // too_many_connections
            return ErrPoolExhausted
        }
    }

    // Wrap unknown errors without exposing implementation
    return fmt.Errorf("storage error: %w", err)
}
```

**Alternatives Considered**:
1. **GORM (ORM)**: Rejected - adds abstraction layer, hides SQL, poor performance vs raw SQL, migrations tied to Go structs
2. **sqlc (code generator)**: Rejected - requires build step, less flexible for dynamic queries, adds toolchain complexity
3. **ent (Facebook's ORM)**: Rejected - heavy abstraction, code generation, opinionated schema migration approach
4. **database/sql only**: Rejected - too much boilerplate for struct scanning, no named parameters
5. **lib/pq driver**: Rejected - slower than pgx, less active maintenance, missing modern Postgres features

**References**:
- sqlx: https://github.com/jmoiron/sqlx
- pgx v5: https://github.com/jackc/pgx
- Go database/sql: https://pkg.go.dev/database/sql
- pgx pool configuration: https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool

---

### Decision 2: PostgreSQL Schema Migration Strategy

**Decision**: Implement lightweight schema version checking on startup with fail-fast approach.

**Rationale**:
- **Fail-Fast Philosophy**: Application refuses to start with wrong schema version (prevents data corruption)
- **Clear Error Messages**: Startup failure provides actionable migration instructions
- **Separation of Concerns**: Migration logic separate from application code (ops responsibility)
- **Version Tracking**: Single source of truth in `schema_migrations` table
- **Idempotent Design**: Safe to run version check multiple times
- **No Auto-Migration**: Application never modifies schema (prevents accidental damage)

**Schema Migrations Table**:
```sql
-- Create migrations tracking table
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL DEFAULT NOW(),
    description TEXT NOT NULL
);

-- Example: Insert migration records
INSERT INTO schema_migrations (version, description) VALUES
    (1, 'Initial schema - create entities table'),
    (2, 'Add indexes on entity columns');
```

**Version Check Implementation**:
```go
// Expected schema version (bumped with each migration)
const expectedSchemaVersion = 2

// VerifySchema checks database schema version on startup
func (r *PostgresRepository) VerifySchema(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    // Query current schema version
    var currentVersion int
    query := `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`
    err := r.db.GetContext(ctx, &currentVersion, query)
    if err != nil {
        // If table doesn't exist, version is 0
        if strings.Contains(err.Error(), "does not exist") {
            currentVersion = 0
        } else {
            return fmt.Errorf("query schema version: %w", err)
        }
    }

    // Compare with expected version
    if currentVersion < expectedSchemaVersion {
        return fmt.Errorf(
            "database schema outdated: current version %d, expected %d\n"+
            "Please run migrations:\n"+
            "  psql -h localhost -U dbuser -d identity_broker -f migrations/%03d_*.sql",
            currentVersion, expectedSchemaVersion, expectedSchemaVersion,
        )
    }

    if currentVersion > expectedSchemaVersion {
        return fmt.Errorf(
            "database schema too new: current version %d, expected %d\n"+
            "Please update application to version compatible with schema %d",
            currentVersion, expectedSchemaVersion, currentVersion,
        )
    }

    r.logger.Info("schema version verified", "version", currentVersion)
    return nil
}
```

**Migration File Structure**:
```
migrations/
├── 001_initial_schema.sql
├── 002_add_indexes.sql
└── README.md  # Instructions for running migrations
```

**Example Migration File** (`001_initial_schema.sql`):
```sql
-- Migration: 001 - Initial Schema
-- Description: Create entities table and schema_migrations tracking

BEGIN;

-- Create entities table
CREATE TABLE entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Record migration
INSERT INTO schema_migrations (version, description)
VALUES (1, 'Initial schema - create entities table');

COMMIT;
```

**Migration Instructions** (in README.md):
```markdown
# Database Migrations

## Running Migrations

1. Connect to PostgreSQL:
   ```bash
   psql -h localhost -U dbuser -d identity_broker
   ```

2. Apply migration:
   ```bash
   \i migrations/001_initial_schema.sql
   ```

3. Verify version:
   ```sql
   SELECT version, description, applied_at FROM schema_migrations ORDER BY version;
   ```

## Creating New Migration

1. Increment version number (e.g., 003)
2. Create file: `003_description.sql`
3. Wrap in transaction (BEGIN/COMMIT)
4. Update `expectedSchemaVersion` in code
```

**Alternatives Considered**:
1. **golang-migrate/migrate**: Rejected - adds external tool dependency, requires separate migration files
2. **goose**: Rejected - similar to golang-migrate, adds toolchain complexity
3. **Auto-migration (GORM-style)**: Rejected - unsafe in production, can cause data loss
4. **Liquibase/Flyway**: Rejected - Java-based tools, heavyweight for simple needs
5. **No version checking**: Rejected - violates fail-fast principle (FR-013)

**References**:
- PostgreSQL schema management: https://www.postgresql.org/docs/current/ddl-schemas.html
- Migration best practices: https://www.postgresql.org/docs/current/ddl-alter.html

---

### Decision 3: In-Memory Concurrency Patterns

**Decision**: Use `sync.RWMutex` with single global lock for in-memory storage.

**Rationale**:
- **Simplicity**: Single lock easier to reason about, less error-prone than complex locking schemes
- **Correctness First**: Prevents all race conditions with straightforward implementation
- **Read Optimization**: `RLock()` allows concurrent reads, only writers block
- **Development Target**: In-memory storage for dev/test only, performance not critical
- **Map Safety**: Go maps are not thread-safe, explicit locking required
- **Testing Focus**: Predictable behavior simplifies unit tests

**Implementation Pattern**:
```go
// InMemoryRepository provides thread-safe in-memory storage
type InMemoryRepository struct {
    mu      sync.RWMutex
    data    map[string]*Entity
    logger  *slog.Logger
}

// NewInMemoryRepository creates a new in-memory repository
func NewInMemoryRepository(logger *slog.Logger) *InMemoryRepository {
    return &InMemoryRepository{
        data:   make(map[string]*Entity),
        logger: logger,
    }
}

// Read operation - allows concurrent reads
func (r *InMemoryRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    entity, exists := r.data[id]
    if !exists {
        return nil, ErrNotFound
    }

    // Return copy to prevent external mutation
    entityCopy := *entity
    return &entityCopy, nil
}

// Write operation - exclusive lock
func (r *InMemoryRepository) Create(ctx context.Context, entity *Entity) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    // Check for duplicate
    if _, exists := r.data[entity.ID]; exists {
        return ErrDuplicateKey
    }

    // Store copy to prevent external mutation
    entityCopy := *entity
    r.data[entity.ID] = &entityCopy

    return nil
}

// List operation - read lock with copy
func (r *InMemoryRepository) List(ctx context.Context) ([]*Entity, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Copy all entities to prevent external mutation
    entities := make([]*Entity, 0, len(r.data))
    for _, entity := range r.data {
        entityCopy := *entity
        entities = append(entities, &entityCopy)
    }

    return entities, nil
}

// Delete operation - exclusive lock
func (r *InMemoryRepository) Delete(ctx context.Context, id string) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    if _, exists := r.data[id]; !exists {
        return ErrNotFound
    }

    delete(r.data, id)
    return nil
}
```

**Lock Granularity Strategy**:
- **Single Lock**: Global `sync.RWMutex` protects entire map
- **Pros**: Simple, correct, no deadlock risk, easy to test
- **Cons**: Writers block all operations (acceptable for dev/test use case)
- **No Per-Entity Locks**: Adds complexity with minimal benefit for in-memory use case

**Read vs Write Lock Usage**:
```go
// Read operations: Use RLock (multiple readers allowed)
// - GetByID, List, Count, Exists
r.mu.RLock()
defer r.mu.RUnlock()

// Write operations: Use Lock (exclusive access)
// - Create, Update, Delete, Clear
r.mu.Lock()
defer r.mu.Unlock()
```

**Data Copying Pattern**:
```go
// Always return copies to prevent external mutation
func (r *InMemoryRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    entity, exists := r.data[id]
    if !exists {
        return nil, ErrNotFound
    }

    // Shallow copy sufficient if Entity has no pointer fields
    entityCopy := *entity
    return &entityCopy, nil
}

// For entities with pointer fields, use deep copy
func deepCopyEntity(e *Entity) *Entity {
    copy := *e
    if e.Metadata != nil {
        metaCopy := make(map[string]string, len(e.Metadata))
        for k, v := range e.Metadata {
            metaCopy[k] = v
        }
        copy.Metadata = metaCopy
    }
    return &copy
}
```

**Alternatives Considered**:
1. **sync.Map**: Rejected - no RLock optimization, less ergonomic API, designed for specific use cases (many writes to disjoint keys)
2. **Per-Entity Locks**: Rejected - adds complexity, risk of deadlocks, unnecessary for in-memory dev storage
3. **Lock-Free Structures**: Rejected - complex to implement correctly, overkill for development use case
4. **Channel-Based Synchronization**: Rejected - more complex than mutex, harder to reason about

**References**:
- Go sync package: https://pkg.go.dev/sync
- sync.RWMutex documentation: https://pkg.go.dev/sync#RWMutex
- Effective Go - Concurrency: https://go.dev/doc/effective_go#concurrency

---

### Decision 4: Configuration Integration

**Decision**: Extend existing Viper-based ConfigPort with `Storage` configuration section.

**Rationale**:
- **Constitution Compliance**: Must use unified configuration system (existing ConfigPort)
- **Consistent Precedence**: Inherits CLI > env > YAML > defaults from existing system
- **Type Safety**: Go struct with validation tags enforces correctness at startup
- **Sensitive Data Protection**: Connection strings automatically redacted by existing system
- **Single Source of Truth**: All configuration in one place

**Configuration Schema**:
```go
// Add to internal/ports/config.go
type Config struct {
    Log     LogConfig     `mapstructure:"log" validate:"required"`
    Server  ServerConfig  `mapstructure:"server" validate:"required"`
    Storage StorageConfig `mapstructure:"storage" validate:"required"`  // NEW
}

type StorageConfig struct {
    Backend  StorageBackend          `mapstructure:"backend" validate:"required,oneof=inmemory postgres"`
    Postgres *PostgresStorageConfig  `mapstructure:"postgres" validate:"required_if=Backend postgres"`
}

// StorageBackend is an enumeration of valid storage backends
type StorageBackend string

const (
    StorageBackendInMemory  StorageBackend = "inmemory"
    StorageBackendPostgres  StorageBackend = "postgres"
)

type PostgresStorageConfig struct {
    // Connection string in PostgreSQL URL format
    // Format: postgresql://user:password@host:port/database?sslmode=verify-full
    ConnectionString string        `mapstructure:"connection_string" validate:"required,postgresql_url"`

    // Connection pool settings
    MaxConnections   int           `mapstructure:"max_connections" validate:"min=1,max=100"`
    MinConnections   int           `mapstructure:"min_connections" validate:"min=1,max=50"`
    MaxConnLifetime  time.Duration `mapstructure:"max_conn_lifetime" validate:"min=1m,max=24h"`
    MaxConnIdleTime  time.Duration `mapstructure:"max_conn_idle_time" validate:"min=1m,max=1h"`

    // Timeouts
    ConnectTimeout   time.Duration `mapstructure:"connect_timeout" validate:"min=1s,max=30s"`

    // SSL/TLS settings
    SSLMode          string        `mapstructure:"ssl_mode" validate:"oneof=disable require verify-ca verify-full"`
}

// DefaultStorageConfig returns default storage configuration values
func DefaultStorageConfig() StorageConfig {
    return StorageConfig{
        Backend: StorageBackendInMemory,
        Postgres: &PostgresStorageConfig{
            MaxConnections:  25,
            MinConnections:  5,
            MaxConnLifetime: 1 * time.Hour,
            MaxConnIdleTime: 15 * time.Minute,
            ConnectTimeout:  5 * time.Second,
            SSLMode:         "verify-full",
        },
    }
}
```

**YAML Configuration Example**:
```yaml
storage:
  backend: postgres
  postgres:
    connection_string: ${IDENTITY_BROKER_DB_URL}
    max_connections: 25
    min_connections: 5
    max_conn_lifetime: 1h
    max_conn_idle_time: 15m
    connect_timeout: 5s
    ssl_mode: verify-full
```

**Environment Variables**:
```bash
IDENTITY_BROKER_STORAGE_BACKEND=postgres
IDENTITY_BROKER_STORAGE_POSTGRES_CONNECTION_STRING=postgresql://user:pass@localhost:5432/identity_broker
IDENTITY_BROKER_STORAGE_POSTGRES_MAX_CONNECTIONS=25
IDENTITY_BROKER_STORAGE_POSTGRES_SSL_MODE=verify-full
```

**CLI Flags**:
```bash
--storage.backend postgres
--storage.postgres.connection-string "postgresql://..."
--storage.postgres.max-connections 25
--storage.postgres.ssl-mode verify-full
```

**Connection String Parsing**:
```go
import "github.com/jackc/pgx/v5/pgxpool"

// ParsePostgresConfig parses and validates PostgreSQL connection string
func ParsePostgresConfig(connStr string) (*pgxpool.Config, error) {
    // pgxpool.ParseConfig handles URL parsing and validation
    config, err := pgxpool.ParseConfig(connStr)
    if err != nil {
        return nil, fmt.Errorf("invalid connection string: %w", err)
    }

    // Validate connection string contains required components
    if config.ConnConfig.Host == "" {
        return nil, fmt.Errorf("connection string missing host")
    }
    if config.ConnConfig.Database == "" {
        return nil, fmt.Errorf("connection string missing database name")
    }
    if config.ConnConfig.User == "" {
        return nil, fmt.Errorf("connection string missing user")
    }

    return config, nil
}
```

**SSL/TLS Parameter Validation**:
```go
// ValidateSSLMode validates SSL mode parameter
func ValidateSSLMode(mode string) error {
    validModes := []string{"disable", "require", "verify-ca", "verify-full"}
    for _, valid := range validModes {
        if mode == valid {
            return nil
        }
    }
    return fmt.Errorf("invalid ssl_mode %q, must be one of: %s",
        mode, strings.Join(validModes, ", "))
}

// Connection string with SSL parameters
// postgresql://user:pass@host:5432/db?sslmode=verify-full&sslrootcert=/path/to/ca.crt
```

**Sensitive Data Redaction**:
```go
// Existing redaction system automatically handles connection strings
// Pattern: IDENTITY_BROKER_* prefix or keywords (password, secret, key)
// Example output in logs:
// storage.postgres.connection_string: ***REDACTED***
```

**Alternatives Considered**:
1. **Separate Config File**: Rejected - violates unified configuration principle
2. **Custom Config Loading**: Rejected - forbidden by architecture (must use ConfigPort)
3. **Flat Configuration**: Rejected - harder to namespace, collision risk
4. **DSN vs URL Format**: Selected URL format (postgresql://) as standard, more explicit

**References**:
- PostgreSQL connection strings: https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING
- pgx connection URL format: https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool#ParseConfig

---

### Decision 5: Testing Strategy

**Decision**: Use table-driven unit tests for adapters, testcontainers for PostgreSQL integration tests, and interface mocks for domain logic tests.

**Rationale**:
- **Unit Tests**: Fast, isolated, cover business logic without external dependencies
- **Integration Tests**: Verify real PostgreSQL behavior with actual database
- **Testcontainers**: Provides real PostgreSQL in Docker, no mock limitations
- **Interface Mocking**: Clean dependency injection for testing domain logic
- **Table-Driven Tests**: Idiomatic Go pattern for testing multiple scenarios

**Unit Test Pattern for In-Memory Repository**:
```go
package storage_test

import (
    "context"
    "log/slog"
    "os"
    "testing"
)

func TestInMemoryRepository(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

    tests := []struct {
        name    string
        setup   func(*InMemoryRepository)
        test    func(*testing.T, *InMemoryRepository)
        wantErr error
    }{
        {
            name: "create and retrieve entity",
            setup: func(repo *InMemoryRepository) {
                // No setup needed
            },
            test: func(t *testing.T, repo *InMemoryRepository) {
                ctx := context.Background()
                entity := &Entity{ID: "test-1", Name: "Test Entity"}

                // Create
                err := repo.Create(ctx, entity)
                if err != nil {
                    t.Fatalf("Create failed: %v", err)
                }

                // Retrieve
                retrieved, err := repo.GetByID(ctx, "test-1")
                if err != nil {
                    t.Fatalf("GetByID failed: %v", err)
                }

                if retrieved.Name != "Test Entity" {
                    t.Errorf("Expected name 'Test Entity', got %q", retrieved.Name)
                }
            },
        },
        {
            name: "create duplicate returns error",
            setup: func(repo *InMemoryRepository) {
                ctx := context.Background()
                entity := &Entity{ID: "test-1", Name: "Existing"}
                _ = repo.Create(ctx, entity)
            },
            test: func(t *testing.T, repo *InMemoryRepository) {
                ctx := context.Background()
                entity := &Entity{ID: "test-1", Name: "Duplicate"}

                err := repo.Create(ctx, entity)
                if !errors.Is(err, ErrDuplicateKey) {
                    t.Errorf("Expected ErrDuplicateKey, got %v", err)
                }
            },
        },
        {
            name: "get nonexistent returns error",
            test: func(t *testing.T, repo *InMemoryRepository) {
                ctx := context.Background()

                _, err := repo.GetByID(ctx, "nonexistent")
                if !errors.Is(err, ErrNotFound) {
                    t.Errorf("Expected ErrNotFound, got %v", err)
                }
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            repo := NewInMemoryRepository(logger)

            if tt.setup != nil {
                tt.setup(repo)
            }

            tt.test(t, repo)
        })
    }
}

// Test concurrent access
func TestInMemoryRepositoryConcurrency(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    repo := NewInMemoryRepository(logger)
    ctx := context.Background()

    // Create initial entity
    entity := &Entity{ID: "concurrent-test", Name: "Initial"}
    if err := repo.Create(ctx, entity); err != nil {
        t.Fatalf("Initial create failed: %v", err)
    }

    // Spawn 100 concurrent readers
    const numReaders = 100
    errChan := make(chan error, numReaders)

    for i := 0; i < numReaders; i++ {
        go func() {
            _, err := repo.GetByID(ctx, "concurrent-test")
            errChan <- err
        }()
    }

    // Check all readers succeeded
    for i := 0; i < numReaders; i++ {
        if err := <-errChan; err != nil {
            t.Errorf("Concurrent read failed: %v", err)
        }
    }
}
```

**Integration Test with Testcontainers**:
```go
package storage_test

import (
    "context"
    "testing"
    "time"

    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/postgres"
)

func TestPostgresRepository(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    ctx := context.Background()

    // Start PostgreSQL container
    postgresContainer, err := postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:16-alpine"),
        postgres.WithDatabase("test_db"),
        postgres.WithUsername("test_user"),
        postgres.WithPassword("test_pass"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections").
                WithOccurrence(2).
                WithStartupTimeout(30*time.Second)),
    )
    if err != nil {
        t.Fatalf("Failed to start PostgreSQL container: %v", err)
    }
    defer func() {
        if err := postgresContainer.Terminate(ctx); err != nil {
            t.Logf("Failed to terminate container: %v", err)
        }
    }()

    // Get connection string
    connStr, err := postgresContainer.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        t.Fatalf("Failed to get connection string: %v", err)
    }

    // Create repository
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    repo, err := NewPostgresRepository(ctx, connStr, logger)
    if err != nil {
        t.Fatalf("Failed to create repository: %v", err)
    }
    defer repo.Close()

    // Run schema migrations
    if err := repo.ApplyMigrations(ctx); err != nil {
        t.Fatalf("Failed to apply migrations: %v", err)
    }

    // Run tests
    t.Run("create and retrieve entity", func(t *testing.T) {
        entity := &Entity{ID: "test-1", Name: "Test Entity"}

        err := repo.Create(ctx, entity)
        if err != nil {
            t.Fatalf("Create failed: %v", err)
        }

        retrieved, err := repo.GetByID(ctx, "test-1")
        if err != nil {
            t.Fatalf("GetByID failed: %v", err)
        }

        if retrieved.Name != "Test Entity" {
            t.Errorf("Expected name 'Test Entity', got %q", retrieved.Name)
        }
    })

    t.Run("timeout on slow query", func(t *testing.T) {
        // Create context with very short timeout
        ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
        defer cancel()

        // Execute slow query
        _, err := repo.SlowQuery(ctx) // Intentionally slow query
        if !errors.Is(err, context.DeadlineExceeded) {
            t.Errorf("Expected DeadlineExceeded, got %v", err)
        }
    })
}
```

**Mock/Interface Pattern for Domain Tests**:
```go
// Define repository interface in domain layer
type EntityRepository interface {
    GetByID(ctx context.Context, id string) (*Entity, error)
    Create(ctx context.Context, entity *Entity) error
    List(ctx context.Context) ([]*Entity, error)
    Delete(ctx context.Context, id string) error
}

// Mock implementation for testing
type MockRepository struct {
    GetByIDFunc func(ctx context.Context, id string) (*Entity, error)
    CreateFunc  func(ctx context.Context, entity *Entity) error
    ListFunc    func(ctx context.Context) ([]*Entity, error)
    DeleteFunc  func(ctx context.Context, id string) error
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
    if m.GetByIDFunc != nil {
        return m.GetByIDFunc(ctx, id)
    }
    return nil, fmt.Errorf("GetByIDFunc not implemented")
}

// Test domain logic with mock
func TestEntityService(t *testing.T) {
    mockRepo := &MockRepository{
        GetByIDFunc: func(ctx context.Context, id string) (*Entity, error) {
            return &Entity{ID: id, Name: "Mocked Entity"}, nil
        },
    }

    service := NewEntityService(mockRepo)

    entity, err := service.ProcessEntity(context.Background(), "test-1")
    if err != nil {
        t.Fatalf("ProcessEntity failed: %v", err)
    }

    if entity.Name != "Mocked Entity" {
        t.Errorf("Expected 'Mocked Entity', got %q", entity.Name)
    }
}
```

**Testing Best Practices**:
1. **Run with Race Detector**: `go test -race ./...` in CI pipeline
2. **Skip Integration Tests in Short Mode**: `if testing.Short() { t.Skip() }`
3. **Use Subtests**: `t.Run("scenario", func(t *testing.T) {...})`
4. **Table-Driven Tests**: Cover multiple scenarios with same test structure
5. **Test Cleanup**: Use `t.Cleanup()` for resource cleanup
6. **Context Timeouts**: Always test timeout behavior explicitly

**Alternatives Considered**:
1. **sqlmock**: Rejected - mocks SQL interactions, doesn't test real database behavior
2. **Manual Docker Setup**: Rejected - testcontainers handles lifecycle automatically
3. **Shared Test Database**: Rejected - test isolation issues, slower cleanup
4. **Mock Generators (gomock/mockery)**: Rejected - simple manual mocks sufficient

**References**:
- Testcontainers Go: https://golang.testcontainers.org/
- Go testing package: https://pkg.go.dev/testing
- Table-driven tests: https://go.dev/wiki/TableDrivenTests

---

## Hexagonal Architecture Mapping

### Ports (Interfaces - Segregated per Interface Segregation Principle)

**Storage Lifecycle Port** (`internal/ports/storage.go`):
```go
package ports

import "context"

// StorageLifecycle manages storage backend lifecycle operations.
// Small, focused interface following Interface Segregation Principle.
type StorageLifecycle interface {
    // Initialize performs storage backend initialization and verification.
    // For PostgreSQL: connects to database, verifies schema
    // For in-memory: initializes empty storage structures
    Initialize(ctx context.Context) error

    // Close gracefully closes storage connections and releases resources.
    Close(ctx context.Context) error

    // HealthCheck verifies storage backend is operational.
    HealthCheck(ctx context.Context) error
}

// UserRepository defines storage operations for user entities.
// Small, focused interface - separate concern from lifecycle.
type UserRepository interface {
    CreateUser(ctx context.Context, user *User) error
    GetUser(ctx context.Context, id string) (*User, error)
    UpdateUser(ctx context.Context, user *User) error
    DeleteUser(ctx context.Context, id string) error
    ListUsers(ctx context.Context, filter *UserFilter) ([]*User, error)
}

// User represents a user domain entity
type User struct {
    ID        string
    Email     string
    CreatedAt time.Time
    UpdatedAt time.Time
}

// UserFilter contains filtering options for list queries
type UserFilter struct {
    Limit  int
    Offset int
}
```

**Note**: ProductRepository is intentionally NOT defined in Phase 1. See quickstart.md for the pattern to follow when adding new entity repositories in future phases.

### Adapters

**In-Memory Adapter** (`internal/adapters/storage/memory/repository.go`):
```go
package memory

import (
    "context"
    "sync"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type Repository struct {
    mu     sync.RWMutex
    data   map[string]*ports.Entity
    logger *slog.Logger
}

func NewRepository(logger *slog.Logger) *Repository {
    return &Repository{
        data:   make(map[string]*ports.Entity),
        logger: logger,
    }
}

// Implements ports.StoragePort interface
func (r *Repository) GetByID(ctx context.Context, id string) (*ports.Entity, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    entity, exists := r.data[id]
    if !exists {
        return nil, ports.ErrNotFound
    }

    entityCopy := *entity
    return &entityCopy, nil
}
```

**PostgreSQL Adapter** (`internal/adapters/storage/postgres/repository.go`):
```go
package postgres

import (
    "context"
    "github.com/jmoiron/sqlx"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type Repository struct {
    db     *sqlx.DB
    logger *slog.Logger
}

func NewRepository(ctx context.Context, connStr string, logger *slog.Logger) (*Repository, error) {
    db, err := sqlx.ConnectContext(ctx, "pgx", connStr)
    if err != nil {
        return nil, fmt.Errorf("connect to database: %w", err)
    }

    repo := &Repository{
        db:     db,
        logger: logger,
    }

    // Verify schema version on startup
    if err := repo.verifySchema(ctx); err != nil {
        db.Close()
        return nil, err
    }

    return repo, nil
}

// Implements ports.StoragePort interface
func (r *Repository) GetByID(ctx context.Context, id string) (*ports.Entity, error) {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()

    var entity ports.Entity
    query := `SELECT id, name, created_at, updated_at FROM entities WHERE id = $1`
    err := r.db.GetContext(ctx, &entity, query, id)
    if err != nil {
        return nil, mapPostgresError(err)
    }

    return &entity, nil
}
```

### Domain Logic

**Storage Factory** (`internal/domain/storage/factory.go`):
```go
package storage

import (
    "context"
    "fmt"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// NewStorage creates a storage adapter based on configuration
func NewStorage(ctx context.Context, config ports.StorageConfig, logger *slog.Logger) (ports.StoragePort, error) {
    switch config.Backend {
    case ports.StorageBackendInMemory:
        logger.Info("initializing in-memory storage")
        return memory.NewRepository(logger), nil

    case ports.StorageBackendPostgres:
        logger.Info("initializing PostgreSQL storage", "host", redactConnString(config.Postgres.ConnectionString))
        return postgres.NewRepository(ctx, config.Postgres.ConnectionString, logger)

    default:
        return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
    }
}

func redactConnString(connStr string) string {
    // Simple redaction for logs
    return "postgresql://***:***@host:port/database"
}
```

### Entry Point Integration

**Main Application** (`cmd/agentic-identity-broker/main.go`):
```go
// Load configuration
cfg, err := configLoader.GetConfig(ctx)
if err != nil {
    log.Fatalf("Failed to load configuration: %v", err)
}

// Initialize storage
storage, err := storage.NewStorage(ctx, cfg.Storage, logger)
if err != nil {
    log.Fatalf("Failed to initialize storage: %v", err)
}
defer storage.Close()

// Verify storage health
if err := storage.Health(ctx); err != nil {
    log.Fatalf("Storage health check failed: %v", err)
}

logger.Info("storage initialized successfully", "backend", cfg.Storage.Backend)
```

---

## Implementation Phases

### Phase 1: Foundation (Days 1-2)
- Define `StoragePort` interface in `internal/ports/storage.go`
- Add `StorageConfig` to configuration schema
- Extend configuration loader with storage settings
- Create domain error types (`ErrNotFound`, `ErrDuplicateKey`, etc.)
- Write unit tests for configuration validation

### Phase 2: In-Memory Implementation (Days 3-4)
- Implement `InMemoryRepository` with `sync.RWMutex`
- Write unit tests for all operations
- Test concurrent access with race detector
- Document in-memory limitations (ephemeral, dev/test only)

### Phase 3: PostgreSQL Implementation (Days 5-7)
- Implement `PostgresRepository` with sqlx/pgx
- Create schema migration structure
- Implement schema version verification
- Write integration tests with testcontainers
- Test timeout behavior for reads/writes
- Test error mapping (duplicate key, not found, etc.)

### Phase 4: Integration & Testing (Days 8-9)
- Implement storage factory
- Integrate with main application
- Write end-to-end tests
- Test configuration switching (in-memory ↔ PostgreSQL)
- Performance testing (connection pool exhaustion)
- Documentation and examples

---

## Performance Considerations

**Startup Time**:
- In-memory: <10ms (instant initialization)
- PostgreSQL: <500ms (connection pool + schema verification)
- Target: <5 seconds total application startup (well within budget)

**Memory Footprint**:
- In-memory: Variable (depends on data volume, no limit)
- PostgreSQL: ~50MB (connection pool + prepared statements)
- Connection pool: 25 connections × ~2MB = ~50MB

**Concurrency**:
- In-memory: RLock allows concurrent reads, writers block
- PostgreSQL: Connection pool supports 25 concurrent queries
- Context timeouts prevent resource exhaustion

**Connection Pool Tuning**:
```go
// Production settings (25 connections)
MaxConnections:  25  // Based on Postgres max_connections=100
MinConnections:  5   // Keep warm connections ready
MaxConnLifetime: 1h  // Rotate connections (prevents stale connections)
MaxConnIdleTime: 15m // Close idle connections (saves resources)

// Development settings (5 connections)
MaxConnections:  5   // Lower overhead
MinConnections:  2
MaxConnLifetime: 30m
MaxConnIdleTime: 5m
```

---

## Security Considerations

**Connection String Protection**:
- Automatically redacted by existing configuration system
- Never logged in plain text
- Environment variable substitution for secrets

**SSL/TLS Requirements**:
- Default: `sslmode=verify-full` (strongest security)
- Validates server certificate against CA
- Prevents MITM attacks

**SQL Injection Prevention**:
- Always use parameterized queries (`$1, $2, $3`)
- Never concatenate user input into SQL strings
- sqlx protects against SQL injection by design

**Error Message Safety**:
- Never expose connection credentials in errors
- Map database errors to domain errors
- Generic "storage error" for unexpected failures

---

## Risk Assessment

### Low Risk
- **In-Memory Implementation**: Simple, well-understood patterns
- **sqlx/pgx**: Mature, widely used libraries
- **Configuration Extension**: Low-risk addition to existing system

### Medium Risk
- **Connection Pool Exhaustion**: Mitigated with queue + timeout (5s reads, 10s writes)
- **Schema Version Mismatch**: Mitigated with fail-fast verification on startup
- **Integration Test Complexity**: Mitigated with testcontainers automation

### High Risk
- None identified

### Mitigation Strategies
- **Connection Pool**: Monitor pool stats, alert on exhaustion, adjust max_connections
- **Schema Migrations**: Clear error messages, documentation, rollback procedures
- **Testing**: Run integration tests in CI with real PostgreSQL container

---

## Dependencies

### External Packages

**sqlx** (`github.com/jmoiron/sqlx`):
- License: MIT
- Version: v1.4.0+ (latest stable)
- Usage: Database querying with struct mapping

**pgx v5** (`github.com/jackc/pgx/v5`):
- License: MIT
- Version: v5.7.0+ (latest stable)
- Usage: PostgreSQL driver and connection pooling

**testcontainers-go** (`github.com/testcontainers/testcontainers-go`):
- License: MIT
- Version: v0.28.0+ (latest stable)
- Usage: Integration testing with real PostgreSQL

### Stdlib Packages
- `database/sql`: SQL database interface
- `context`: Timeout and cancellation
- `sync`: Concurrency primitives (RWMutex)
- `time`: Duration and timestamp handling

---

## Open Questions (Resolved)

### Q1: Should we support multiple storage backends simultaneously?
**Answer**: No. Application supports exactly one backend at runtime (FR-003). Switching requires restart with new configuration.

### Q2: How to handle PostgreSQL connection pool exhaustion?
**Answer**: Queue requests with timeout (5s read, 10s write), then fail with clear error. Log pool stats for monitoring.

### Q3: Should migrations be applied automatically on startup?
**Answer**: No. Application only verifies schema version, never modifies schema. Operations team runs migrations manually.

### Q4: How to test PostgreSQL integration without external dependency?
**Answer**: Use testcontainers to spin up real PostgreSQL in Docker for integration tests. Skip in short mode (`testing.Short()`).

### Q5: Should in-memory storage have memory limits?
**Answer**: No. Intended for dev/test only where memory is adequate. No enforcement needed.

---

## Next Steps

1. Review this research document with team
2. Create ADR for sqlx/pgx selection
3. Create ADR for schema migration strategy
4. Implement Phase 1 (foundation)
5. Request golang-pro agent review after Phase 1 completion

---

## References

- sqlx: https://github.com/jmoiron/sqlx
- pgx: https://github.com/jackc/pgx
- PostgreSQL documentation: https://www.postgresql.org/docs/current/
- Go database/sql: https://pkg.go.dev/database/sql
- Testcontainers: https://golang.testcontainers.org/
- Hexagonal architecture: https://alistair.cockburn.us/hexagonal-architecture/
