// Package postgres implements PostgreSQL storage adapters.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
)

// UserGrantRepository implements ports.UserGrantRepository using PostgreSQL.
type UserGrantRepository struct {
	adapter *Adapter
}

// NewUserGrantRepository creates a new PostgreSQL user grant repository.
// The adapter must be initialized before use.
func NewUserGrantRepository(adapter *Adapter) *UserGrantRepository {
	return &UserGrantRepository{
		adapter: adapter,
	}
}

// Create creates a new user grant or updates existing grant for same principal+agent (upsert).
// The grant ID should be generated before calling this method.
// Uses ON CONFLICT to implement upsert semantics (one grant per principal-agent pair).
func (r *UserGrantRepository) Create(ctx context.Context, grant *storage.UserGrant) error {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.create.user_grant")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "CreateUserGrant"),
	)

	if r.adapter.db == nil {
		return storage.NewStorageError(
			"CreateUserGrant",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if grant == nil {
		return storage.NewStorageError(
			"CreateUserGrant",
			storage.ErrorKindValidation,
			nil,
			"grant cannot be nil",
		)
	}

	// Generate ID if not provided
	if grant.ID.IsZero() {
		grant.ID = id.NewGrantID()
	}

	// Validate before storing
	if err := grant.ValidateForCreate(); err != nil {
		return storage.NewStorageError(
			"CreateUserGrant",
			storage.ErrorKindValidation,
			err,
			"grant validation failed",
		)
	}

	// Serialize granted permission sets to JSON for PostgreSQL JSONB
	permissionSetsJSON, err := json.Marshal(grant.GrantedPermissionSets)
	if err != nil {
		return storage.NewStorageError(
			"CreateUserGrant",
			storage.ErrorKindUnknown,
			err,
			"failed to serialize granted permission sets",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTx(ctxTimeout, nil)
	if err != nil {
		return r.handlePostgresError("CreateUserGrant", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Lock referenced permission set rows within the transaction to prevent
	// concurrent deletes from committing before this write (TOCTOU guard).
	if len(grant.GrantedPermissionSets) > 0 {
		psIDs := make([]id.PermissionSetID, len(grant.GrantedPermissionSets))
		for i, entry := range grant.GrantedPermissionSets {
			psIDs[i] = entry.PermissionSetID
		}
		if err = verifyPermissionSetExistenceInTx(ctxTimeout, tx, psIDs); err != nil {
			return err
		}
	}

	// Use ON CONFLICT to implement upsert semantics
	query := `
		INSERT INTO user_grants (
			id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (principal, agent_id)
		DO UPDATE SET
			valid_until = EXCLUDED.valid_until,
			granted_permission_sets = EXCLUDED.granted_permission_sets,
			updated_at = EXCLUDED.updated_at
		RETURNING id
	`

	var returnedID id.GrantID
	err = tx.QueryRowContext(
		ctxTimeout,
		query,
		grant.ID,
		grant.Principal,
		grant.AgentID,
		grant.ValidUntil,
		permissionSetsJSON,
		grant.CreatedAt,
		grant.UpdatedAt,
	).Scan(&returnedID)

	if err != nil {
		return r.handlePostgresError("CreateUserGrant", err)
	}

	if err = tx.Commit(); err != nil {
		return r.handlePostgresError("CreateUserGrant", err)
	}

	// Update grant ID if it was changed by upsert
	grant.ID = returnedID

	return nil
}

// Get retrieves a user grant by ID.
// Returns StorageError with Kind=NotFound if grant not found.
func (r *UserGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"GetUserGrant",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		FROM user_grants
		WHERE id = $1
	`

	var grant storage.UserGrant
	var permissionSetsJSON []byte

	err := r.adapter.db.QueryRowContext(ctxTimeout, query, grantID).Scan(
		&grant.ID,
		&grant.Principal,
		&grant.AgentID,
		&grant.ValidUntil,
		&permissionSetsJSON,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"GetUserGrant",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"user grant not found",
			)
		}
		return nil, r.handlePostgresError("GetUserGrant", err)
	}

	// Deserialize JSONB to GrantedPermissionSetEntry slice
	if err := json.Unmarshal(permissionSetsJSON, &grant.GrantedPermissionSets); err != nil {
		return nil, storage.NewStorageError(
			"GetUserGrant",
			storage.ErrorKindUnknown,
			err,
			"failed to parse granted permission sets",
		)
	}

	return grant.Copy(), nil
}

// Update updates an existing user grant.
// Returns StorageError with Kind=NotFound if grant not found.
func (r *UserGrantRepository) Update(ctx context.Context, grant *storage.UserGrant) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if grant == nil {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindValidation,
			nil,
			"grant cannot be nil",
		)
	}

	// Serialize granted permission sets to JSON for PostgreSQL JSONB
	permissionSetsJSON, err := json.Marshal(grant.GrantedPermissionSets)
	if err != nil {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindUnknown,
			err,
			"failed to serialize granted permission sets",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.db.BeginTx(ctxTimeout, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return storage.NewStorageError("UpdateUserGrant", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	// Run the UPDATE first so a stale grant ID returns NotFound before any PS check,
	// preserving the documented error contract. The UPDATE result is not yet committed,
	// so we can still lock PS rows to close the concurrent-delete window.
	query := `
		UPDATE user_grants
		SET valid_until = $2,
		    granted_permission_sets = $3,
		    updated_at = $4
		WHERE id = $1
	`

	result, err := tx.ExecContext(
		ctxTimeout,
		query,
		grant.ID,
		grant.ValidUntil,
		permissionSetsJSON,
		grant.UpdatedAt,
	)

	if err != nil {
		return r.handlePostgresError("UpdateUserGrant", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindUnknown,
			err,
			"failed to get rows affected",
		)
	}

	if rowsAffected == 0 {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"user grant not found",
		)
	}

	// After confirming the grant exists, lock PS rows to prevent concurrent deletes
	// from creating dangling references before this transaction commits.
	psIDs := make([]id.PermissionSetID, 0, len(grant.GrantedPermissionSets))
	for _, entry := range grant.GrantedPermissionSets {
		psIDs = append(psIDs, entry.PermissionSetID)
	}
	if err := verifyPermissionSetExistenceInTx(ctxTimeout, tx, psIDs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return storage.NewStorageError("UpdateUserGrant", storage.ErrorKindUnknown, err, "failed to commit transaction")
	}

	return nil
}

// Delete deletes a user grant by ID.
// Idempotent: returns nil if grant doesn't exist.
func (r *UserGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"DeleteUserGrant",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	query := `DELETE FROM user_grants WHERE id = $1`

	_, err := r.adapter.db.ExecContext(ctxTimeout, query, grantID)
	if err != nil {
		return r.handlePostgresError("DeleteUserGrant", err)
	}

	// Idempotent: success even if no rows deleted
	return nil
}

// ListByPrincipalAndAgent retrieves all grants for a principal and specific agent.
// Includes expired grants (filtering happens in service layer).
// Returns empty slice if no grants exist (not an error).
func (r *UserGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListUserGrants",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		FROM user_grants
		WHERE principal = $1 AND agent_id = $2
		ORDER BY created_at DESC
	`

	rows, err := r.adapter.db.QueryContext(ctxTimeout, query, principal, agentID)
	if err != nil {
		return nil, r.handlePostgresError("ListUserGrants", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []*storage.UserGrant

	for rows.Next() {
		var grant storage.UserGrant
		var permissionSetsJSON []byte

		err := rows.Scan(
			&grant.ID,
			&grant.Principal,
			&grant.AgentID,
			&grant.ValidUntil,
			&permissionSetsJSON,
			&grant.CreatedAt,
			&grant.UpdatedAt,
		)
		if err != nil {
			return nil, storage.NewStorageError(
				"ListUserGrants",
				storage.ErrorKindUnknown,
				err,
				"failed to scan grant row",
			)
		}

		if err := json.Unmarshal(permissionSetsJSON, &grant.GrantedPermissionSets); err != nil {
			return nil, storage.NewStorageError(
				"ListUserGrants",
				storage.ErrorKindUnknown,
				err,
				"failed to parse granted permission sets",
			)
		}

		grants = append(grants, grant.Copy())
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError(
			"ListUserGrants",
			storage.ErrorKindUnknown,
			err,
			"error iterating grant rows",
		)
	}

	return grants, nil
}

// FindByPrincipalAndAgent retrieves the grant for a principal and agent pair.
// Returns StorageError with Kind=NotFound if grant not found.
func (r *UserGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"FindUserGrant",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		FROM user_grants
		WHERE principal = $1 AND agent_id = $2
		LIMIT 1
	`

	var grant storage.UserGrant
	var permissionSetsJSON []byte

	err := r.adapter.db.QueryRowContext(ctxTimeout, query, principal, agentID).Scan(
		&grant.ID,
		&grant.Principal,
		&grant.AgentID,
		&grant.ValidUntil,
		&permissionSetsJSON,
		&grant.CreatedAt,
		&grant.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"FindUserGrant",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"user grant not found",
			)
		}
		return nil, r.handlePostgresError("FindUserGrant", err)
	}

	if err := json.Unmarshal(permissionSetsJSON, &grant.GrantedPermissionSets); err != nil {
		return nil, storage.NewStorageError(
			"FindUserGrant",
			storage.ErrorKindUnknown,
			err,
			"failed to parse granted permission sets",
		)
	}

	return grant.Copy(), nil
}

// DeleteByAgent deletes all grants associated with an agent.
// Used during cascade deletion when agent is deleted (FR-021).
// Idempotent: returns nil if agent has no grants.
func (r *UserGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"DeleteGrantsByAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	query := `DELETE FROM user_grants WHERE agent_id = $1`

	_, err := r.adapter.db.ExecContext(ctxTimeout, query, agentID)
	if err != nil {
		return r.handlePostgresError("DeleteGrantsByAgent", err)
	}

	// Idempotent: success even if no rows deleted
	return nil
}

// ListByPrincipal retrieves all active grants for a principal across all agents.
// Filters expired grants (valid_until < NOW()).
// Returns empty slice if no active grants exist (not an error).
func (r *UserGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.list.user_grants_by_principal")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ListByPrincipal"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListByPrincipal",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		FROM user_grants
		WHERE principal = $1
		  AND (valid_until IS NULL OR valid_until > NOW())
		ORDER BY updated_at DESC
	`

	rows, err := r.adapter.db.QueryContext(ctxTimeout, query, principal)
	if err != nil {
		return nil, r.handlePostgresError("ListByPrincipal", err)
	}
	defer func() { _ = rows.Close() }()

	var grants []storage.UserGrant

	for rows.Next() {
		var grant storage.UserGrant
		var permissionSetsJSON []byte

		err := rows.Scan(
			&grant.ID,
			&grant.Principal,
			&grant.AgentID,
			&grant.ValidUntil,
			&permissionSetsJSON,
			&grant.CreatedAt,
			&grant.UpdatedAt,
		)
		if err != nil {
			return nil, storage.NewStorageError(
				"ListByPrincipal",
				storage.ErrorKindUnknown,
				err,
				"failed to scan grant row",
			)
		}

		if err := json.Unmarshal(permissionSetsJSON, &grant.GrantedPermissionSets); err != nil {
			return nil, storage.NewStorageError(
				"ListByPrincipal",
				storage.ErrorKindUnknown,
				err,
				"failed to parse granted permission sets",
			)
		}

		grants = append(grants, *grant.Copy())
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError(
			"ListByPrincipal",
			storage.ErrorKindUnknown,
			err,
			"error iterating grant rows",
		)
	}

	return grants, nil
}

// CountAgentsByPrincipalAndServiceID counts distinct agents for the exact principal
// whose GrantedPermissionSets include the given service. Expired grants are included for session dependency warnings.
func (r *UserGrantRepository) CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError(
			"CountAgentsByPrincipalAndServiceID",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	query := `
		SELECT COUNT(DISTINCT ug.agent_id)
		FROM user_grants ug,
		     jsonb_array_elements(ug.granted_permission_sets) AS entry
		WHERE ug.principal = $1
		  AND entry->'included_service_ids' @> to_jsonb($2::text)
	`

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var count int
	err := r.adapter.db.QueryRowContext(ctxTimeout, query, principal, serviceID.String()).Scan(&count)
	if err != nil {
		return 0, r.handlePostgresError("CountAgentsByPrincipalAndServiceID", err)
	}

	return count, nil
}

// ListByPrincipalAndServiceID returns distinct agent IDs for the exact principal
// whose GrantedPermissionSets include the given service. Expired grants are included for session dependency warnings.
func (r *UserGrantRepository) ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListByPrincipalAndServiceID",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	query := `
		SELECT DISTINCT ug.agent_id
		FROM user_grants ug,
		     jsonb_array_elements(ug.granted_permission_sets) AS entry
		WHERE ug.principal = $1
		  AND entry->'included_service_ids' @> to_jsonb($2::text)
		ORDER BY ug.agent_id
	`

	// Create context with timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	rows, err := r.adapter.db.QueryContext(ctxTimeout, query, principal, serviceID.String())
	if err != nil {
		return nil, r.handlePostgresError("ListByPrincipalAndServiceID", err)
	}
	defer func() { _ = rows.Close() }()

	var agentIDs []id.AgentID

	for rows.Next() {
		var agentID id.AgentID
		err := rows.Scan(&agentID)
		if err != nil {
			return nil, storage.NewStorageError(
				"ListByPrincipalAndServiceID",
				storage.ErrorKindUnknown,
				err,
				"failed to scan agent ID row",
			)
		}
		agentIDs = append(agentIDs, agentID)
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError(
			"ListByPrincipalAndServiceID",
			storage.ErrorKindUnknown,
			err,
			"error iterating agent ID rows",
		)
	}

	return agentIDs, nil
}

// CountGrantsReferencingPermissionSet counts user grants whose granted_permission_sets JSONB
// array contains an entry with the given permission set ID.
func (r *UserGrantRepository) CountGrantsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("CountGrantsReferencingPermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	jsonFilter, err := json.Marshal([]map[string]string{{"permission_set_id": psID.String()}})
	if err != nil {
		return 0, storage.NewStorageError("CountGrantsReferencingPermissionSet", storage.ErrorKindUnknown, err, "failed to build JSONB filter")
	}

	var count int
	err = r.adapter.db.QueryRowContext(ctxTimeout,
		`SELECT COUNT(*) FROM user_grants WHERE granted_permission_sets @> $1::jsonb AND (valid_until IS NULL OR valid_until > NOW())`,
		jsonFilter,
	).Scan(&count)
	if err != nil {
		return 0, r.handlePostgresError("CountGrantsReferencingPermissionSet", err)
	}

	return count, nil
}

// DeleteByPrincipalAndAgentID deletes the grant owned by principal for the given agent.
// Returns StorageError wrapping ports.ErrNotFound when no grant exists for the pair.
// This is NOT idempotent: absence of a grant is an error (revocation semantics FR-014).
func (r *UserGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"DeleteByPrincipalAndAgentID",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	query := `DELETE FROM user_grants WHERE principal = $1 AND agent_id = $2`

	result, err := r.adapter.db.ExecContext(ctxTimeout, query, principal, agentID)
	if err != nil {
		return r.handlePostgresError("DeleteByPrincipalAndAgentID", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError(
			"DeleteByPrincipalAndAgentID",
			storage.ErrorKindUnknown,
			err,
			"failed to get rows affected",
		)
	}

	if rowsAffected == 0 {
		return storage.NewStorageError(
			"DeleteByPrincipalAndAgentID",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"no active grant exists for this principal and agent",
		)
	}

	return nil
}

// handlePostgresError converts PostgreSQL errors to StorageError.
func (r *UserGrantRepository) handlePostgresError(operation string, err error) error {
	if pgErr, ok := err.(*pgconn.PgError); ok {
		switch pgErr.Code {
		case "23505": // unique_violation
			return storage.NewStorageError(
				operation,
				storage.ErrorKindConflict,
				err,
				fmt.Sprintf("conflict: %s", pgErr.Detail),
			)
		case "23503": // foreign_key_violation
			return storage.NewStorageError(
				operation,
				storage.ErrorKindNotFound,
				err,
				fmt.Sprintf("foreign key violation: %s", pgErr.Detail),
			)
		}
	}

	// Check for context errors in the wrapped error
	if err == context.DeadlineExceeded {
		return storage.NewStorageError(
			operation,
			storage.ErrorKindTimeout,
			err,
			"operation timed out",
		)
	}

	// Default to connection error
	return storage.NewStorageError(
		operation,
		storage.ErrorKindConnection,
		err,
		"PostgreSQL operation failed",
	)
}
