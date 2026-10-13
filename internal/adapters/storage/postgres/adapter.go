// Package postgres implements a PostgreSQL storage adapter.
// Uses sqlx for query building and pgx as the PostgreSQL driver.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// Adapter implements storage repository and lifecycle helpers
// using PostgreSQL as the persistence backend.
type Adapter struct {
	db       *sqlx.DB
	config   *ports.StorageConfig
	timeouts ports.StorageTimeouts
}

// NewAdapter creates a new PostgreSQL storage adapter.
// Does not connect to database - connection happens during Initialize().
// Returns error if configuration is invalid.
func NewAdapter(config *ports.StorageConfig) (*Adapter, error) {
	if config == nil {
		return nil, storage.NewStorageError(
			"NewAdapter",
			storage.ErrorKindValidation,
			nil,
			"configuration cannot be nil",
		)
	}

	if config.Backend != "postgres" {
		return nil, storage.NewStorageError(
			"NewAdapter",
			storage.ErrorKindValidation,
			nil,
			fmt.Sprintf("invalid backend for PostgreSQL adapter: %s", config.Backend),
		)
	}

	if config.Postgres.ConnectionURL == "" {
		return nil, storage.NewStorageError(
			"NewAdapter",
			storage.ErrorKindValidation,
			nil,
			"PostgreSQL connection URL cannot be empty",
		)
	}

	// Set default timeouts if not provided
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

// Initialize connects to PostgreSQL and verifies schema.
// Satisfies storage lifecycle expectations.
// Returns error if connection fails or schema is invalid.
func (a *Adapter) Initialize(ctx context.Context) error {
	// Check context for cancellation
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

	// Connect to database
	db, err := sqlx.ConnectContext(ctx, "pgx", a.config.Postgres.ConnectionURL)
	if err != nil {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindConnection,
			err,
			"failed to connect to PostgreSQL database",
		)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)
	db.SetConnMaxIdleTime(15 * time.Minute)

	// Verify connection with ping
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindConnection,
			err,
			"failed to ping PostgreSQL database",
		)
	}

	// Verify schema exists
	if err := a.verifySchema(ctx, db); err != nil {
		_ = db.Close()
		return err
	}

	a.db = db
	return nil
}

// verifySchema checks that the required schema exists.
// Returns error if schema is missing or version is outdated.
func (a *Adapter) verifySchema(ctx context.Context, db *sqlx.DB) error {
	schemaCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	// Check if schema_migrations table exists
	query := `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'schema_migrations'
		)
	`

	var exists bool
	if err := db.GetContext(schemaCtx, &exists, query); err != nil {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindConnection,
			err,
			"failed to query schema information",
		)
	}

	if !exists {
		return storage.NewStorageError(
			"Initialize",
			storage.ErrorKindValidation,
			nil,
			"schema_migrations table not found; run migrations: identity-broker migrate up",
		)
	}

	// Check if users table exists (optional - from previous feature)
	query = `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = 'users'
		)
	`

	if err := db.GetContext(schemaCtx, &exists, query); err != nil {
		// Log warning but don't fail - users table may not be created yet
		slog.Warn("failed to query users table existence",
			"error", err,
			"note", "users table may not exist yet, continuing initialization")
	} else if !exists {
		// Users table doesn't exist, log warning but continue
		slog.Warn("users table not found",
			"note", "users table from previous feature may not be migrated yet")
	}

	return nil
}

// Close gracefully closes the PostgreSQL connection.
// Satisfies storage lifecycle expectations.
func (a *Adapter) Close(ctx context.Context) error {
	if a.db == nil {
		return nil // Already closed
	}

	// Check context for cancellation
	select {
	case <-ctx.Done():
		// Still close even if context is cancelled
	default:
	}

	if err := a.db.Close(); err != nil {
		return storage.NewStorageError(
			"Close",
			storage.ErrorKindConnection,
			err,
			"failed to close PostgreSQL connection",
		)
	}

	a.db = nil
	return nil
}

// HealthCheck verifies PostgreSQL connection is operational.
// Satisfies storage lifecycle expectations.
func (a *Adapter) HealthCheck(ctx context.Context) error {
	if a.db == nil {
		return storage.NewStorageError(
			"HealthCheck",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	healthCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	if err := a.db.PingContext(healthCtx); err != nil {
		return storage.NewStorageError(
			"HealthCheck",
			storage.ErrorKindConnection,
			err,
			"failed to ping PostgreSQL database",
		)
	}

	return a.businessEventPartitionsReady(healthCtx)
}

// CreateUser creates a new user entity in PostgreSQL.
// Satisfies ports.UserRepository interface.
func (a *Adapter) CreateUser(ctx context.Context, user *ports.User) error {
	if a.db == nil {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if user == nil {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user cannot be nil",
		)
	}

	if user.ID.IsZero() {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	if user.Email == "" {
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindValidation,
			nil,
			"user email cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()

	query := `
		INSERT INTO users (id, email, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
	`

	_, err := a.storageExecutor(execCtx).ExecContext(execCtx, query, user.ID, user.Email, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		// Map PostgreSQL errors to domain errors
		if err.Error() == "context deadline exceeded" {
			return storage.NewStorageError(
				"CreateUser",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		// Check for duplicate key error (PostgreSQL error code 23505)
		if err.Error() == "pq: duplicate key value violates unique constraint \"users_pkey\"" {
			return storage.NewStorageError(
				"CreateUser",
				storage.ErrorKindConflict,
				err,
				fmt.Sprintf("user with ID %q already exists", user.ID),
			)
		}
		return storage.NewStorageError(
			"CreateUser",
			storage.ErrorKindConnection,
			err,
			"failed to create user",
		)
	}

	return nil
}

// GetUser retrieves a user entity by ID.
// Satisfies ports.UserRepository interface.
func (a *Adapter) GetUser(ctx context.Context, userID id.UserID) (*ports.User, error) {
	if a.db == nil {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if userID.IsZero() {
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	user := &ports.User{}
	query := `
		SELECT id, email, created_at, updated_at
		FROM users
		WHERE id = $1
	`

	if err := a.storageExecutor(queryCtx).GetContext(queryCtx, user, query, userID); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return nil, storage.NewStorageError(
				"GetUser",
				storage.ErrorKindNotFound,
				err,
				fmt.Sprintf("user with ID %q not found", userID),
			)
		}
		if err.Error() == "context deadline exceeded" {
			return nil, storage.NewStorageError(
				"GetUser",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"GetUser",
			storage.ErrorKindConnection,
			err,
			"failed to get user",
		)
	}

	return user, nil
}

// UpdateUser updates an existing user entity.
// Satisfies ports.UserRepository interface.
func (a *Adapter) UpdateUser(ctx context.Context, user *ports.User) error {
	if a.db == nil {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if user == nil {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user cannot be nil",
		)
	}

	if user.ID.IsZero() {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	if user.Email == "" {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindValidation,
			nil,
			"user email cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()

	query := `
		UPDATE users
		SET email = $1, updated_at = $2
		WHERE id = $3
	`

	result, err := a.storageExecutor(execCtx).ExecContext(execCtx, query, user.Email, user.UpdatedAt, user.ID)
	if err != nil {
		if err.Error() == "context deadline exceeded" {
			return storage.NewStorageError(
				"UpdateUser",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindConnection,
			err,
			"failed to update user",
		)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindConnection,
			err,
			"failed to get affected rows",
		)
	}

	if rows == 0 {
		return storage.NewStorageError(
			"UpdateUser",
			storage.ErrorKindNotFound,
			nil,
			fmt.Sprintf("user with ID %q not found", user.ID),
		)
	}

	return nil
}

// DeleteUser deletes a user entity by ID.
// Satisfies ports.UserRepository interface.
func (a *Adapter) DeleteUser(ctx context.Context, userID id.UserID) error {
	if a.db == nil {
		return storage.NewStorageError(
			"DeleteUser",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if userID.IsZero() {
		return storage.NewStorageError(
			"DeleteUser",
			storage.ErrorKindValidation,
			nil,
			"user ID cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()

	query := `DELETE FROM users WHERE id = $1`

	_, err := a.storageExecutor(execCtx).ExecContext(execCtx, query, userID)
	if err != nil {
		if err.Error() == "context deadline exceeded" {
			return storage.NewStorageError(
				"DeleteUser",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return storage.NewStorageError(
			"DeleteUser",
			storage.ErrorKindConnection,
			err,
			"failed to delete user",
		)
	}

	return nil
}

// ListUsers retrieves multiple user entities matching optional filter.
// Satisfies ports.UserRepository interface.
func (a *Adapter) ListUsers(ctx context.Context, filter *ports.UserFilter) ([]*ports.User, error) {
	if a.db == nil {
		return nil, storage.NewStorageError(
			"ListUsers",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	query := `SELECT id, email, created_at, updated_at FROM users`
	var args []interface{}

	if filter != nil && filter.EmailPattern != "" {
		query += ` WHERE email LIKE $1`
		args = append(args, "%"+filter.EmailPattern+"%")
	}

	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, filter.Limit)
	}

	if filter != nil && filter.Offset > 0 {
		query += fmt.Sprintf(` OFFSET %d`, filter.Offset)
	}

	var users []*ports.User
	if err := a.storageExecutor(queryCtx).SelectContext(queryCtx, &users, query, args...); err != nil {
		if err.Error() == "context deadline exceeded" {
			return nil, storage.NewStorageError(
				"ListUsers",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ListUsers",
			storage.ErrorKindConnection,
			err,
			"failed to list users",
		)
	}

	return users, nil
}
