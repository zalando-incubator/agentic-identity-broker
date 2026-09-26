# Quickstart Guide: Implementing Storage Adapters and Ports

**Feature**: 004-persistence-layer
**Audience**: Future developers adding new entities or storage backends
**Purpose**: Step-by-step guide for extending the persistence layer

## Table of Contents

1. [Overview](#overview)
2. [How to Define a Storage Port](#how-to-define-a-storage-port)
3. [How to Implement an Adapter](#how-to-implement-an-adapter)
4. [How to Add a New Entity Repository](#how-to-add-a-new-entity-repository)
5. [Configuration Integration](#configuration-integration)
6. [Testing Patterns](#testing-patterns)
7. [Common Patterns and Best Practices](#common-patterns-and-best-practices)

---

## Overview

The persistence layer follows hexagonal architecture (ports and adapters):

- **Port** (Interface): Defines what the storage layer can do (`internal/ports/storage.go`)
- **Adapter** (Implementation): Implements how it does it (`internal/adapters/storage/{memory,postgres}/`)
- **Domain** (Types): Pure business logic types (`internal/domain/storage/`)

**Key Principle**: Domain logic depends ONLY on ports (interfaces), NEVER on adapters (concrete implementations).

```
Domain Logic (Business Rules)
       ↓  depends on
  Port (Interface - StoragePort)
       ↑  implements
  Adapters (Memory, PostgreSQL, Future...)
```

---

## How to Define a Storage Port

### Step 1: Create the Port Interface

Location: `internal/ports/storage.go`

```go
package ports

import (
	"context"
)

// StoragePort defines storage operations for persistence.
// Domain logic depends on this interface, not concrete implementations.
type StoragePort interface {
	// Initialize sets up the storage backend
	Initialize(ctx context.Context) error

	// Close releases storage resources
	Close(ctx context.Context) error

	// HealthCheck verifies backend is operational
	HealthCheck(ctx context.Context) error

	// Add entity-specific methods here...
	// CreateUser(ctx context.Context, user *User) error
	// GetUser(ctx context.Context, id string) (*User, error)
}
```

### Step 2: Define Domain Types

Location: `internal/domain/storage/types.go`

```go
package storage

// User represents a user entity (domain type, not database type)
type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

// Validate performs domain-level validation
func (u *User) Validate() error {
	if u.Email == "" {
		return errors.New("email is required")
	}
	return nil
}
```

### Step 3: Define Domain Errors

Location: `internal/domain/storage/errors.go`

```go
package storage

// StorageError wraps storage operation errors
type StorageError struct {
	Operation string
	Kind      ErrorKind
	Cause     error
	Message   string
}

func (e *StorageError) Error() string {
	return fmt.Sprintf("%s failed: %s", e.Operation, e.Message)
}

func (e *StorageError) Unwrap() error {
	return e.Cause
}

// NewStorageError creates a storage error
func NewStorageError(op string, kind ErrorKind, cause error, msg string) *StorageError {
	return &StorageError{
		Operation: op,
		Kind:      kind,
		Cause:     cause,
		Message:   msg,
	}
}
```

**Key Rules**:
- Ports are in `internal/ports/` (interfaces only)
- Domain types are in `internal/domain/storage/` (pure Go structs, no SQL tags)
- All port methods accept `context.Context` for timeout/cancellation
- All port methods return `error` (wrapped in `StorageError`)
- Methods must be backend-agnostic (no SQL, no PostgreSQL-specific types)

---

## How to Implement an Adapter

### Example 1: In-Memory Adapter

Location: `internal/adapters/storage/memory/adapter.go`

```go
package memory

import (
	"context"
	"sync"

	"agentic-identity-broker/internal/ports"
	"agentic-identity-broker/internal/domain/storage"
)

// Adapter implements StoragePort for in-memory storage
type Adapter struct {
	mu    sync.RWMutex
	users map[string]*storage.User
}

// NewAdapter creates an in-memory storage adapter
func NewAdapter() *Adapter {
	return &Adapter{
		users: make(map[string]*storage.User),
	}
}

// Initialize sets up in-memory storage (always succeeds)
func (a *Adapter) Initialize(ctx context.Context) error {
	// No external dependencies, instant initialization
	return nil
}

// Close releases resources (no-op for memory)
func (a *Adapter) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Clear data to help GC
	a.users = nil
	return nil
}

// HealthCheck always succeeds for in-memory
func (a *Adapter) HealthCheck(ctx context.Context) error {
	return nil
}

// CreateUser adds a user to in-memory storage
func (a *Adapter) CreateUser(ctx context.Context, user *storage.User) error {
	// Validate domain rules
	if err := user.Validate(); err != nil {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			err,
			"invalid user data",
		)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	// Check for duplicate
	if _, exists := a.users[user.ID]; exists {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindConflict,
			nil,
			fmt.Sprintf("user %s already exists", user.ID),
		)
	}

	// Store copy to prevent external mutation
	userCopy := *user
	a.users[user.ID] = &userCopy

	return nil
}

// GetUser retrieves a user from in-memory storage
func (a *Adapter) GetUser(ctx context.Context, id string) (*storage.User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	user, exists := a.users[id]
	if !exists {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindNotFound,
			nil,
			fmt.Sprintf("user %s not found", id),
		)
	}

	// Return copy to prevent external mutation
	userCopy := *user
	return &userCopy, nil
}
```

**In-Memory Adapter Patterns**:
- Use `sync.RWMutex` for thread safety
- `RLock` for reads (concurrent), `Lock` for writes (exclusive)
- Always copy data in/out to prevent external mutation
- No external dependencies = fast initialization, always healthy

### Example 2: PostgreSQL Adapter

Location: `internal/adapters/storage/postgres/adapter.go`

```go
package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/jackc/pgx/v5/stdlib"

	"agentic-identity-broker/internal/ports"
	"agentic-identity-broker/internal/domain/storage"
)

// Adapter implements StoragePort for PostgreSQL
type Adapter struct {
	db       *sqlx.DB
	config   *ports.PostgresConfig
	timeouts *ports.StorageTimeouts
}

// NewAdapter creates a PostgreSQL storage adapter
func NewAdapter(cfg *ports.PostgresConfig, timeouts *ports.StorageTimeouts) *Adapter {
	return &Adapter{
		config:   cfg,
		timeouts: timeouts,
	}
}

// Initialize connects to PostgreSQL and verifies schema
func (a *Adapter) Initialize(ctx context.Context) error {
	// Connect to database
	db, err := sqlx.ConnectContext(ctx, "pgx", a.config.ConnectionURL)
	if err != nil {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindConnection,
			err,
			fmt.Sprintf("failed to connect to PostgreSQL: %v", err),
		)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	a.db = db

	// Verify schema version
	if err := a.verifySchema(ctx); err != nil {
		a.db.Close() // Clean up on failure
		return err
	}

	return nil
}

// verifySchema checks that database schema is current
func (a *Adapter) verifySchema(ctx context.Context) error {
	const expectedVersion = 1 // Update as schema evolves

	var version int
	query := "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1"

	err := a.db.GetContext(ctx, &version, query)
	if err == sql.ErrNoRows {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindValidation,
			err,
			"No schema migrations found. Run: ./bin/agentic-identity-broker migrate up",
		)
	}
	if err != nil {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindConnection,
			err,
			"Failed to check schema version",
		)
	}

	if version != expectedVersion {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindValidation,
			nil,
			fmt.Sprintf(
				"Schema version mismatch (expected: v%d, found: v%d). Run: ./bin/agentic-identity-broker migrate up",
				expectedVersion, version,
			),
		)
	}

	return nil
}

// Close releases PostgreSQL connections
func (a *Adapter) Close(ctx context.Context) error {
	if a.db == nil {
		return nil // Already closed
	}

	if err := a.db.Close(); err != nil {
		return storage.NewStorageError(
			"Close",
			storage.ErrorKindUnknown,
			err,
			"Failed to close database connections",
		)
	}

	a.db = nil
	return nil
}

// HealthCheck verifies PostgreSQL is reachable
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.db == nil {
		return storage.NewStorageError(
			"HealthCheck",
			storage.ErrorKindConnection,
			nil,
			"Database not initialized",
		)
	}

	if err := a.db.PingContext(ctx); err != nil {
		return storage.NewStorageError(
			"HealthCheck",
			storage.ErrorKindConnection,
			err,
			"Database ping failed",
		)
	}

	return nil
}

// CreateUser inserts a user into PostgreSQL
func (a *Adapter) CreateUser(ctx context.Context, user *storage.User) error {
	// Validate domain rules
	if err := user.Validate(); err != nil {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			err,
			"invalid user data",
		)
	}

	// Apply write timeout
	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()

	query := `
		INSERT INTO users (id, email, created_at)
		VALUES ($1, $2, $3)
	`

	_, err := a.db.ExecContext(ctx, query, user.ID, user.Email, user.CreatedAt)
	if err != nil {
		// Check for duplicate key error
		if isDuplicateKeyError(err) {
			return storage.NewStorageError(
				"CreateUser",
				storage.ErrorKindConflict,
				err,
				fmt.Sprintf("user %s already exists", user.ID),
			)
		}

		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindUnknown,
			err,
			"failed to insert user",
		)
	}

	return nil
}

// GetUser retrieves a user from PostgreSQL
func (a *Adapter) GetUser(ctx context.Context, id string) (*storage.User, error) {
	// Apply read timeout
	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, email, created_at
		FROM users
		WHERE id = $1
	`

	var user storage.User
	err := a.db.GetContext(ctx, &user, query, id)
	if err == sql.ErrNoRows {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindNotFound,
			err,
			fmt.Sprintf("user %s not found", id),
		)
	}
	if err != nil {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindUnknown,
			err,
			"failed to query user",
		)
	}

	return &user, nil
}

// isDuplicateKeyError checks if error is a PostgreSQL duplicate key violation
func isDuplicateKeyError(err error) bool {
	// pgx v5 specific error checking
	// Typically check for error code 23505 (unique_violation)
	return strings.Contains(err.Error(), "duplicate key") ||
		strings.Contains(err.Error(), "unique constraint")
}
```

**PostgreSQL Adapter Patterns**:
- Initialize establishes connection and verifies schema
- Always apply timeouts via `context.WithTimeout`
- Wrap all database errors in `StorageError`
- Use `sqlx` for query convenience (struct scanning)
- Check schema version on startup (fail fast if mismatch)

---

## How to Add a New Entity Repository

### Step 1: Define Domain Entity

Location: `internal/domain/storage/product.go`

```go
package storage

import (
	"errors"
	"time"
)

type Product struct {
	ID          string
	Name        string
	Description string
	Price       float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (p *Product) Validate() error {
	if p.Name == "" {
		return errors.New("product name is required")
	}
	if p.Price < 0 {
		return errors.New("product price cannot be negative")
	}
	return nil
}
```

### Step 2: Extend StoragePort Interface

Location: `internal/ports/storage.go`

```go
type StoragePort interface {
	// Existing methods...
	Initialize(ctx context.Context) error
	Close(ctx context.Context) error
	HealthCheck(ctx context.Context) error

	// NEW: Product methods
	CreateProduct(ctx context.Context, product *storage.Product) error
	GetProduct(ctx context.Context, id string) (*storage.Product, error)
	UpdateProduct(ctx context.Context, product *storage.Product) error
	DeleteProduct(ctx context.Context, id string) error
	ListProducts(ctx context.Context, filter *ProductFilter) ([]*storage.Product, error)
}

// ProductFilter defines query filters for products
type ProductFilter struct {
	NameContains string
	MinPrice     float64
	MaxPrice     float64
	Limit        int
	Offset       int
}
```

### Step 3: Implement in Memory Adapter

Location: `internal/adapters/storage/memory/products.go`

```go
package memory

import (
	"context"
	"strings"

	"agentic-identity-broker/internal/domain/storage"
)

func (a *Adapter) CreateProduct(ctx context.Context, product *storage.Product) error {
	if err := product.Validate(); err != nil {
		return storage.NewStorageError("CreateProduct", storage.ErrorKindValidation, err, "invalid product")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.products[product.ID]; exists {
		return storage.NewStorageError("CreateProduct", storage.ErrorKindConflict, nil, "product already exists")
	}

	productCopy := *product
	a.products[product.ID] = &productCopy
	return nil
}

func (a *Adapter) GetProduct(ctx context.Context, id string) (*storage.Product, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	product, exists := a.products[id]
	if !exists {
		return nil, storage.NewStorageError("GetProduct", storage.ErrorKindNotFound, nil, "product not found")
	}

	productCopy := *product
	return &productCopy, nil
}

func (a *Adapter) ListProducts(ctx context.Context, filter *ports.ProductFilter) ([]*storage.Product, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make([]*storage.Product, 0)

	for _, product := range a.products {
		// Apply filters
		if filter.NameContains != "" && !strings.Contains(product.Name, filter.NameContains) {
			continue
		}
		if product.Price < filter.MinPrice || product.Price > filter.MaxPrice {
			continue
		}

		productCopy := *product
		results = append(results, &productCopy)
	}

	// Apply pagination
	start := filter.Offset
	end := start + filter.Limit
	if start >= len(results) {
		return []*storage.Product{}, nil
	}
	if end > len(results) {
		end = len(results)
	}

	return results[start:end], nil
}
```

### Step 4: Implement PostgreSQL Adapter

Location: `internal/adapters/storage/postgres/products.go`

```go
package postgres

import (
	"context"
	"database/sql"

	"agentic-identity-broker/internal/domain/storage"
	"agentic-identity-broker/internal/ports"
)

func (a *Adapter) CreateProduct(ctx context.Context, product *storage.Product) error {
	if err := product.Validate(); err != nil {
		return storage.NewStorageError("CreateProduct", storage.ErrorKindValidation, err, "invalid product")
	}

	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()

	query := `
		INSERT INTO products (id, name, description, price, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := a.db.ExecContext(ctx, query,
		product.ID,
		product.Name,
		product.Description,
		product.Price,
		product.CreatedAt,
		product.UpdatedAt,
	)

	if err != nil {
		if isDuplicateKeyError(err) {
			return storage.NewStorageError("CreateProduct", storage.ErrorKindConflict, err, "product already exists")
		}
		return storage.NewStorageError("CreateProduct", storage.ErrorKindUnknown, err, "failed to insert product")
	}

	return nil
}

func (a *Adapter) GetProduct(ctx context.Context, id string) (*storage.Product, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, name, description, price, created_at, updated_at
		FROM products
		WHERE id = $1
	`

	var product storage.Product
	err := a.db.GetContext(ctx, &product, query, id)
	if err == sql.ErrNoRows {
		return nil, storage.NewStorageError("GetProduct", storage.ErrorKindNotFound, err, "product not found")
	}
	if err != nil {
		return nil, storage.NewStorageError("GetProduct", storage.ErrorKindUnknown, err, "failed to query product")
	}

	return &product, nil
}

func (a *Adapter) ListProducts(ctx context.Context, filter *ports.ProductFilter) ([]*storage.Product, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, name, description, price, created_at, updated_at
		FROM products
		WHERE 1=1
			AND ($1 = '' OR name ILIKE '%' || $1 || '%')
			AND price >= $2
			AND price <= $3
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`

	var products []*storage.Product
	err := a.db.SelectContext(ctx, &products, query,
		filter.NameContains,
		filter.MinPrice,
		filter.MaxPrice,
		filter.Limit,
		filter.Offset,
	)

	if err != nil {
		return nil, storage.NewStorageError("ListProducts", storage.ErrorKindUnknown, err, "failed to list products")
	}

	return products, nil
}
```

### Step 5: Write Tests

Location: `internal/adapters/storage/memory/products_test.go`

```go
package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"agentic-identity-broker/internal/adapters/storage/memory"
	"agentic-identity-broker/internal/domain/storage"
	"agentic-identity-broker/internal/ports"
)

func TestProductCRUD(t *testing.T) {
	adapter := memory.NewAdapter()
	ctx := context.Background()

	// Initialize
	err := adapter.Initialize(ctx)
	require.NoError(t, err)
	defer adapter.Close(ctx)

	// Create product
	product := &storage.Product{
		ID:        "prod-1",
		Name:      "Test Product",
		Price:     99.99,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = adapter.CreateProduct(ctx, product)
	require.NoError(t, err)

	// Get product
	retrieved, err := adapter.GetProduct(ctx, "prod-1")
	require.NoError(t, err)
	assert.Equal(t, product.Name, retrieved.Name)
	assert.Equal(t, product.Price, retrieved.Price)

	// List products
	filter := &ports.ProductFilter{Limit: 10}
	products, err := adapter.ListProducts(ctx, filter)
	require.NoError(t, err)
	assert.Len(t, products, 1)

	// Update product (if implemented)
	// Delete product (if implemented)
}

func TestProductValidation(t *testing.T) {
	tests := []struct {
		name    string
		product *storage.Product
		wantErr bool
	}{
		{
			name: "valid product",
			product: &storage.Product{
				ID:    "prod-1",
				Name:  "Valid Product",
				Price: 50.00,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			product: &storage.Product{
				ID:    "prod-2",
				Price: 50.00,
			},
			wantErr: true,
		},
		{
			name: "negative price",
			product: &storage.Product{
				ID:    "prod-3",
				Name:  "Invalid Product",
				Price: -10.00,
			},
			wantErr: true,
		},
	}

	adapter := memory.NewAdapter()
	ctx := context.Background()
	adapter.Initialize(ctx)
	defer adapter.Close(ctx)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.CreateProduct(ctx, tt.product)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

---

## Configuration Integration

### Adding Entity-Specific Configuration

Sometimes entities need configuration (e.g., cache TTL, batch size).

Location: `internal/ports/config.go`

```go
type StorageConfig struct {
	Backend  string              `mapstructure:"backend" validate:"required,oneof=memory postgres"`
	Postgres PostgresConfig      `mapstructure:"postgres"`
	Timeouts StorageTimeouts     `mapstructure:"timeouts" validate:"required"`
	Products ProductStorageConfig `mapstructure:"products"` // NEW
}

type ProductStorageConfig struct {
	CacheTTL  time.Duration `mapstructure:"cache_ttl"`
	BatchSize int           `mapstructure:"batch_size" validate:"min=1,max=1000"`
}
```

Configuration file example:

```yaml
storage:
  backend: postgres
  postgres:
    connection_url: "postgresql://..."
  timeouts:
    read: 5s
    write: 10s
  products:
    cache_ttl: 5m
    batch_size: 100
```

---

## Testing Patterns

### Table-Driven Unit Tests

```go
func TestCreateUser_Validation(t *testing.T) {
	tests := []struct {
		name    string
		user    *storage.User
		wantErr bool
		errKind storage.ErrorKind
	}{
		{
			name:    "valid user",
			user:    &storage.User{ID: "1", Email: "test@example.com"},
			wantErr: false,
		},
		{
			name:    "missing email",
			user:    &storage.User{ID: "2"},
			wantErr: true,
			errKind: storage.ErrorKindValidation,
		},
		{
			name:    "duplicate ID",
			user:    &storage.User{ID: "1", Email: "duplicate@example.com"},
			wantErr: true,
			errKind: storage.ErrorKindConflict,
		},
	}

	adapter := memory.NewAdapter()
	ctx := context.Background()
	adapter.Initialize(ctx)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.CreateUser(ctx, tt.user)
			if tt.wantErr {
				require.Error(t, err)
				var storageErr *storage.StorageError
				require.ErrorAs(t, err, &storageErr)
				assert.Equal(t, tt.errKind, storageErr.Kind)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

### Integration Tests with Testcontainers

```go
func TestPostgresIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Start PostgreSQL container
	ctx := context.Background()
	postgres, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:15-alpine",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_PASSWORD": "test",
				"POSTGRES_DB":       "testdb",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections"),
		},
		Started: true,
	})
	require.NoError(t, err)
	defer postgres.Terminate(ctx)

	// Get connection details
	host, err := postgres.Host(ctx)
	require.NoError(t, err)
	port, err := postgres.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Build connection URL
	connectionURL := fmt.Sprintf(
		"postgresql://postgres:test@%s:%s/testdb?sslmode=disable",
		host, port.Port(),
	)

	// Create adapter
	config := &ports.PostgresConfig{ConnectionURL: connectionURL}
	timeouts := &ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}
	adapter := postgres.NewAdapter(config, timeouts)

	// Test full lifecycle
	err = adapter.Initialize(ctx)
	require.NoError(t, err)
	defer adapter.Close(ctx)

	// Test operations...
	user := &storage.User{ID: "1", Email: "test@example.com", CreatedAt: time.Now()}
	err = adapter.CreateUser(ctx, user)
	require.NoError(t, err)

	retrieved, err := adapter.GetUser(ctx, "1")
	require.NoError(t, err)
	assert.Equal(t, user.Email, retrieved.Email)
}
```

---

## Common Patterns and Best Practices

### 1. Error Handling

**Always wrap adapter errors in domain errors:**

```go
// BAD: Exposes PostgreSQL error
return nil, err

// GOOD: Wraps in StorageError
return nil, storage.NewStorageError("GetUser", storage.ErrorKindUnknown, err, "failed to query user")
```

### 2. Timeout Enforcement

**Always apply timeouts from configuration:**

```go
ctx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
defer cancel()

result, err := a.db.GetContext(ctx, &entity, query, id)
```

### 3. Data Copying (In-Memory)

**Always copy data to prevent external mutation:**

```go
// BAD: Stores direct reference
a.users[user.ID] = user

// GOOD: Stores copy
userCopy := *user
a.users[user.ID] = &userCopy
```

### 4. Concurrent Safety (In-Memory)

**Use appropriate lock types:**

```go
// Reads: Use RLock (concurrent)
func (a *Adapter) GetUser(id string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	// Read operations...
}

// Writes: Use Lock (exclusive)
func (a *Adapter) CreateUser(user *User) {
	a.mu.Lock()
	defer a.mu.Unlock()
	// Write operations...
}
```

### 5. Schema Versioning

**Check schema version on Initialize:**

```go
func (a *Adapter) verifySchema(ctx context.Context) error {
	const expectedVersion = 3 // Update as schema evolves

	var version int
	err := a.db.GetContext(ctx, &version, "SELECT MAX(version) FROM schema_migrations")

	if version != expectedVersion {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindValidation,
			nil,
			fmt.Sprintf("Schema mismatch (expected: v%d, found: v%d). Run migrations.", expectedVersion, version),
		)
	}

	return nil
}
```

### 6. Connection Pool Configuration

**Configure pools for production:**

```go
db.SetMaxOpenConns(25)  // Max concurrent connections
db.SetMaxIdleConns(5)   // Min idle connections
db.SetConnMaxLifetime(5 * time.Minute) // Connection recycling
db.SetConnMaxIdleTime(1 * time.Minute) // Idle connection timeout
```

### 7. Factory Pattern

**Create adapters via factory:**

```go
// Factory in internal/adapters/storage/factory.go
func NewAdapter(config *ports.StorageConfig) (ports.StoragePort, error) {
	switch config.Backend {
	case "memory":
		return memory.NewAdapter(), nil
	case "postgres":
		return postgres.NewAdapter(&config.Postgres, &config.Timeouts), nil
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", config.Backend)
	}
}

// Usage in main.go
adapter, err := storage.NewAdapter(appConfig.Storage)
if err != nil {
	log.Fatal("Failed to create storage adapter", err)
}
```

---

## Summary

**Key Takeaways**:
1. Ports define interfaces in `internal/ports/`
2. Adapters implement interfaces in `internal/adapters/storage/{memory,postgres}/`
3. Domain types live in `internal/domain/storage/`
4. Always wrap errors in `StorageError`
5. Always apply timeouts from configuration
6. Use table-driven tests for validation logic
7. Use testcontainers for integration tests
8. Follow hexagonal architecture principles

**Next Steps**:
- See [data-model.md](data-model.md) for entity definitions
- See [contracts/storage_port.go](contracts/storage_port.go) for interface details
- See [research.md](research.md) for technology decisions
- See [plan.md](plan.md) for implementation roadmap
- Run `/speckit.tasks` to generate implementation tasks

---

**Questions?** Refer to:
- [Constitution](../../.specify/memory/constitution.md) - Principle VI (Hexagonal Architecture)
- [ARCHITECTURE.md](../../docs/ARCHITECTURE.md) - Section 3.1.2 (Storage Subsystem)
- [Research Document](research.md) - Technology decisions and patterns
