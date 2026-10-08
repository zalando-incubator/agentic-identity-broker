package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
)

// PermissionSetRepository provides PostgreSQL-backed storage for PermissionSet entities.
type PermissionSetRepository struct {
	adapter *Adapter
}

// NewPermissionSetRepository creates a new PostgreSQL permission set repository.
func NewPermissionSetRepository(adapter *Adapter) *PermissionSetRepository {
	return &PermissionSetRepository{
		adapter: adapter,
	}
}

// Create stores a new permission set with its service scopes.
// Uses a transaction to insert into both permission_sets and permission_set_service_scopes.
func (r *PermissionSetRepository) Create(ctx context.Context, ps *storage.PermissionSet) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("CreatePermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.beginSQLTransaction(ctxTimeout, nil)
	if err != nil {
		return r.handlePostgresError("CreatePermissionSet", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert into permission_sets
	query := `INSERT INTO permission_sets (id, canonical_id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err = tx.ExecContext(ctxTimeout, query, ps.ID, ps.CanonicalID, ps.Name, ps.Description, ps.CreatedAt, ps.UpdatedAt)
	if err != nil {
		return r.handlePostgresError("CreatePermissionSet", err)
	}

	// Insert service scopes
	if err := r.insertServiceScopes(ctxTimeout, tx, ps.ID, ps.ServiceScopes); err != nil {
		return err
	}

	return tx.Commit()
}

// Get retrieves a permission set by ID, including its service scopes.
func (r *PermissionSetRepository) Get(ctx context.Context, psID id.PermissionSetID) (*storage.PermissionSet, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("GetPermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `SELECT id, canonical_id, name, description, created_at, updated_at FROM permission_sets WHERE id = $1`
	var ps storage.PermissionSet
	err := r.adapter.storageExecutor(ctxTimeout).QueryRowContext(ctxTimeout, query, psID).Scan(
		&ps.ID, &ps.CanonicalID, &ps.Name, &ps.Description, &ps.CreatedAt, &ps.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError("GetPermissionSet", storage.ErrorKindNotFound, ports.ErrNotFound, "permission set not found")
		}
		return nil, r.handlePostgresError("GetPermissionSet", err)
	}

	scopes, err := r.loadServiceScopes(ctxTimeout, ps.ID)
	if err != nil {
		return nil, err
	}
	ps.ServiceScopes = scopes

	return ps.Copy(), nil
}

func (r *PermissionSetRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.PermissionSet, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var permissionSetID id.PermissionSetID
	if err := r.adapter.storageExecutor(ctxTimeout).QueryRowContext(ctxTimeout, `SELECT id FROM permission_sets WHERE canonical_id = $1`, canonicalID).Scan(&permissionSetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.NewStorageError("GetPermissionSetByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "permission set not found")
		}
		return nil, r.handlePostgresError("GetPermissionSetByCanonicalID", err)
	}
	return r.Get(ctx, permissionSetID)
}

func (r *PermissionSetRepository) GetCanonicalIDs(ctx context.Context, ids []id.PermissionSetID) (map[id.PermissionSetID]string, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("GetCanonicalIDsPermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if len(ids) == 0 {
		return map[id.PermissionSetID]string{}, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	rows, err := r.adapter.storageExecutor(ctxTimeout).QueryContext(ctxTimeout, `SELECT id, canonical_id FROM permission_sets WHERE id = ANY($1::uuid[]) AND canonical_id IS NOT NULL`, pq.Array(ids))
	if err != nil {
		return nil, r.handlePostgresError("GetCanonicalIDsPermissionSet", err)
	}
	defer func() { _ = rows.Close() }()

	canonicalIDs := make(map[id.PermissionSetID]string, len(ids))
	for rows.Next() {
		var permissionSetID id.PermissionSetID
		var canonicalID string
		if err := rows.Scan(&permissionSetID, &canonicalID); err != nil {
			return nil, storage.NewStorageError("GetCanonicalIDsPermissionSet", storage.ErrorKindUnknown, err, "failed to scan canonical ID")
		}
		canonicalIDs[permissionSetID] = canonicalID
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("GetCanonicalIDsPermissionSet", storage.ErrorKindUnknown, err, "failed to read canonical IDs")
	}
	return canonicalIDs, nil
}

// GetByIDs retrieves multiple permission sets by IDs.
// Returns only found permission sets (missing IDs are silently absent).
func (r *PermissionSetRepository) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("GetByIDsPermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if len(ids) == 0 {
		return []*storage.PermissionSet{}, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	// Convert IDs to string array for ANY()
	idStrings := make([]string, len(ids))
	for i, psID := range ids {
		idStrings[i] = psID.String()
	}

	query := `SELECT id, canonical_id, name, description, created_at, updated_at FROM permission_sets WHERE id = ANY($1::uuid[])`
	rows, err := r.adapter.storageExecutor(ctxTimeout).QueryContext(ctxTimeout, query, pq.Array(idStrings))
	if err != nil {
		return nil, r.handlePostgresError("GetByIDsPermissionSet", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*storage.PermissionSet
	for rows.Next() {
		var ps storage.PermissionSet
		if err := rows.Scan(&ps.ID, &ps.CanonicalID, &ps.Name, &ps.Description, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, storage.NewStorageError("GetByIDsPermissionSet", storage.ErrorKindUnknown, err, "failed to scan row")
		}
		results = append(results, &ps)
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("GetByIDsPermissionSet", storage.ErrorKindUnknown, err, "error iterating rows")
	}

	if len(results) == 0 {
		return results, nil
	}

	// Batch-load all service scopes in a single query and group by permission set ID.
	collectedIDs := make([]id.PermissionSetID, len(results))
	for i, ps := range results {
		collectedIDs[i] = ps.ID
	}
	scopesByPS, err := r.loadServiceScopesBatch(ctxTimeout, collectedIDs)
	if err != nil {
		return nil, err
	}
	for _, ps := range results {
		ps.ServiceScopes = scopesByPS[ps.ID]
	}

	copied := make([]*storage.PermissionSet, len(results))
	for i, ps := range results {
		copied[i] = ps.Copy()
	}
	return copied, nil
}

// Update replaces a permission set's fields and service scopes.
// Uses a transaction: updates the parent row and replaces all service scope rows.
func (r *PermissionSetRepository) Update(ctx context.Context, ps *storage.PermissionSet) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("UpdatePermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.beginSQLTransaction(ctxTimeout, nil)
	if err != nil {
		return r.handlePostgresError("UpdatePermissionSet", err)
	}
	defer func() { _ = tx.Rollback() }()

	canonicalIDOmitted := !ps.ClearCanonicalID && ps.CanonicalID == nil
	query := `
		UPDATE permission_sets
		SET canonical_id = CASE WHEN $2 THEN NULL WHEN $3 THEN canonical_id ELSE $4 END,
		    name = $5, description = $6, updated_at = $7
		WHERE id = $1
	`
	result, err := tx.ExecContext(ctxTimeout, query, ps.ID, ps.ClearCanonicalID, canonicalIDOmitted, ps.CanonicalID, ps.Name, ps.Description, ps.UpdatedAt)
	if err != nil {
		return r.handlePostgresError("UpdatePermissionSet", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("UpdatePermissionSet", storage.ErrorKindUnknown, err, "failed to get rows affected")
	}
	if rowsAffected == 0 {
		return storage.NewStorageError("UpdatePermissionSet", storage.ErrorKindNotFound, ports.ErrNotFound, "permission set not found")
	}

	// Delete existing scopes and re-insert
	_, err = tx.ExecContext(ctxTimeout, `DELETE FROM permission_set_service_scopes WHERE permission_set_id = $1`, ps.ID)
	if err != nil {
		return r.handlePostgresError("UpdatePermissionSet", err)
	}

	if err := r.insertServiceScopes(ctxTimeout, tx, ps.ID, ps.ServiceScopes); err != nil {
		return err
	}

	return tx.Commit()
}

// Delete removes a permission set by ID. Idempotent: returns nil if the permission
// set does not exist. Uses a serializable transaction to atomically verify no agents
// reference the permission set before deleting, preventing TOCTOU races with concurrent
// agent creates/updates. Retries up to 3 times on serialization failure (PG code 40001).
func (r *PermissionSetRepository) Delete(ctx context.Context, psID id.PermissionSetID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("DeletePermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	jsonFilter, err := json.Marshal([]map[string]string{{"permission_set_id": psID.String()}})
	if err != nil {
		return storage.NewStorageError("DeletePermissionSet", storage.ErrorKindUnknown, err, "failed to build JSONB filter")
	}

	if _, joined := storageTransaction(ctx); joined {
		return r.deleteInSerializableTx(ctxTimeout, psID, jsonFilter)
	}

	const maxRetries = 3
	var lastErr error
	for range maxRetries {
		lastErr = r.deleteInSerializableTx(ctxTimeout, psID, jsonFilter)
		if lastErr == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if !errors.As(lastErr, &pgErr) || pgErr.Code != "40001" {
			return lastErr
		}
	}
	return lastErr
}

func (r *PermissionSetRepository) deleteInSerializableTx(ctx context.Context, psID id.PermissionSetID, jsonFilter []byte) error {
	tx, err := r.adapter.beginSQLTransaction(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return storage.NewStorageError("DeletePermissionSet", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	var agentCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agents WHERE permission_sets @> $1::jsonb`,
		jsonFilter,
	).Scan(&agentCount); err != nil {
		return r.handlePostgresError("DeletePermissionSet", err)
	}
	if agentCount > 0 {
		return storage.NewStorageError("DeletePermissionSet", storage.ErrorKindConflict, nil,
			fmt.Sprintf("cannot delete permission set: %d agent(s) reference it", agentCount))
	}

	var grantCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_grants WHERE granted_permission_sets @> $1::jsonb AND (valid_until IS NULL OR valid_until > NOW())`,
		jsonFilter,
	).Scan(&grantCount); err != nil {
		return r.handlePostgresError("DeletePermissionSet", err)
	}
	if grantCount > 0 {
		return storage.NewStorageError("DeletePermissionSet", storage.ErrorKindConflict, nil,
			fmt.Sprintf("cannot delete permission set: %d user grant(s) reference it", grantCount))
	}

	// CASCADE on FK will remove permission_set_service_scopes rows
	if _, err = tx.ExecContext(ctx, `DELETE FROM permission_sets WHERE id = $1`, psID); err != nil {
		return r.handlePostgresError("DeletePermissionSet", err)
	}

	if err := tx.Commit(); err != nil {
		return r.handlePostgresError("DeletePermissionSet", err)
	}
	return nil
}

// List returns all permission sets, optionally filtered by service ID.
// When serviceID is zero, returns all permission sets.
func (r *PermissionSetRepository) List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("ListPermissionSets", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var rows *sql.Rows
	var err error

	if serviceID.IsZero() {
		query := `SELECT id, canonical_id, name, description, created_at, updated_at FROM permission_sets ORDER BY name`
		rows, err = r.adapter.storageExecutor(ctxTimeout).QueryContext(ctxTimeout, query)
	} else {
		query := `
			SELECT DISTINCT ps.id, ps.canonical_id, ps.name, ps.description, ps.created_at, ps.updated_at
			FROM permission_sets ps
			JOIN permission_set_service_scopes psss ON psss.permission_set_id = ps.id
			WHERE psss.service_id = $1
			ORDER BY ps.name
		`
		rows, err = r.adapter.storageExecutor(ctxTimeout).QueryContext(ctxTimeout, query, serviceID)
	}
	if err != nil {
		return nil, r.handlePostgresError("ListPermissionSets", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*storage.PermissionSet
	for rows.Next() {
		var ps storage.PermissionSet
		if err := rows.Scan(&ps.ID, &ps.CanonicalID, &ps.Name, &ps.Description, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, storage.NewStorageError("ListPermissionSets", storage.ErrorKindUnknown, err, "failed to scan row")
		}
		results = append(results, &ps)
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("ListPermissionSets", storage.ErrorKindUnknown, err, "error iterating rows")
	}

	if len(results) == 0 {
		return results, nil
	}

	collectedIDs := make([]id.PermissionSetID, len(results))
	for i, ps := range results {
		collectedIDs[i] = ps.ID
	}
	scopesByPS, err := r.loadServiceScopesBatch(ctxTimeout, collectedIDs)
	if err != nil {
		return nil, err
	}
	for _, ps := range results {
		ps.ServiceScopes = scopesByPS[ps.ID]
	}

	copied := make([]*storage.PermissionSet, len(results))
	for i, ps := range results {
		copied[i] = ps.Copy()
	}
	return copied, nil
}

// CountAgentsReferencingPermissionSet counts agents that reference this permission set
// via the agents.permission_sets JSONB column.
func (r *PermissionSetRepository) CountAgentsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("CountAgentsReferencingPermissionSet", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	// agents.permission_sets is JSONB array of objects with "permission_set_id" key
	jsonFilter, err := json.Marshal([]map[string]string{{"permission_set_id": psID.String()}})
	if err != nil {
		return 0, storage.NewStorageError("CountAgentsReferencingPermissionSet", storage.ErrorKindUnknown, err, "failed to build JSONB filter")
	}

	var count int
	err = r.adapter.storageExecutor(ctxTimeout).QueryRowContext(ctxTimeout, `SELECT COUNT(*) FROM agents WHERE permission_sets @> $1::jsonb`,
		jsonFilter).Scan(&count)
	if err != nil {
		return 0, r.handlePostgresError("CountAgentsReferencingPermissionSet", err)
	}

	return count, nil
}

// CountPermissionSetsForService counts permission sets containing a scope for the given service.
func (r *PermissionSetRepository) CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if r.adapter.db == nil {
		return 0, storage.NewStorageError("CountPermissionSetsForService", storage.ErrorKindConnection, nil, "database not initialized")
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT COUNT(DISTINCT permission_set_id)
		FROM permission_set_service_scopes
		WHERE service_id = $1
	`

	var count int
	err := r.adapter.storageExecutor(ctxTimeout).QueryRowContext(ctxTimeout, query, serviceID).Scan(&count)
	if err != nil {
		return 0, r.handlePostgresError("CountPermissionSetsForService", err)
	}

	return count, nil
}

// insertServiceScopes inserts service scope rows within a transaction.
func (r *PermissionSetRepository) insertServiceScopes(ctx context.Context, tx sqlTransactionExecutor, psID id.PermissionSetID, scopes []storage.ServiceScope) error {
	if len(scopes) == 0 {
		return nil
	}

	query := `
		INSERT INTO permission_set_service_scopes (permission_set_id, service_id, scopes, requirement_type)
		VALUES ($1, $2, $3, $4)
	`

	for _, ss := range scopes {
		scopesJSON, err := json.Marshal(ss.Scopes)
		if err != nil {
			return storage.NewStorageError("InsertServiceScopes", storage.ErrorKindUnknown, err, "failed to marshal scopes")
		}
		_, err = tx.ExecContext(ctx, query, psID, ss.ServiceID, scopesJSON, ss.RequirementType)
		if err != nil {
			return r.handlePostgresError("InsertServiceScopes", err)
		}
	}

	return nil
}

// loadServiceScopesBatch loads service scopes for multiple permission sets in a single query.
// Returns a map from permission set ID to its service scopes.
func (r *PermissionSetRepository) loadServiceScopesBatch(ctx context.Context, psIDs []id.PermissionSetID) (map[id.PermissionSetID][]storage.ServiceScope, error) {
	idStrings := make([]string, len(psIDs))
	for i, psID := range psIDs {
		idStrings[i] = psID.String()
	}

	query := `
		SELECT permission_set_id, service_id, scopes, requirement_type
		FROM permission_set_service_scopes
		WHERE permission_set_id = ANY($1::uuid[])
	`

	rows, err := r.adapter.storageExecutor(ctx).QueryContext(ctx, query, pq.Array(idStrings))
	if err != nil {
		return nil, r.handlePostgresError("LoadServiceScopesBatch", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[id.PermissionSetID][]storage.ServiceScope)
	for rows.Next() {
		var psID id.PermissionSetID
		var ss storage.ServiceScope
		var scopesJSON []byte
		if err := rows.Scan(&psID, &ss.ServiceID, &scopesJSON, &ss.RequirementType); err != nil {
			return nil, storage.NewStorageError("LoadServiceScopesBatch", storage.ErrorKindUnknown, err, "failed to scan service scope row")
		}
		if err := json.Unmarshal(scopesJSON, &ss.Scopes); err != nil {
			return nil, storage.NewStorageError("LoadServiceScopesBatch", storage.ErrorKindUnknown, err, "failed to unmarshal scopes")
		}
		result[psID] = append(result[psID], ss)
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("LoadServiceScopesBatch", storage.ErrorKindUnknown, err, "error iterating service scope rows")
	}

	return result, nil
}

// loadServiceScopes loads all service scopes for a permission set.
func (r *PermissionSetRepository) loadServiceScopes(ctx context.Context, psID id.PermissionSetID) ([]storage.ServiceScope, error) {
	query := `
		SELECT service_id, scopes, requirement_type
		FROM permission_set_service_scopes
		WHERE permission_set_id = $1
	`

	rows, err := r.adapter.storageExecutor(ctx).QueryContext(ctx, query, psID)
	if err != nil {
		return nil, r.handlePostgresError("LoadServiceScopes", err)
	}
	defer func() { _ = rows.Close() }()

	var scopes []storage.ServiceScope
	for rows.Next() {
		var ss storage.ServiceScope
		var scopesJSON []byte
		if err := rows.Scan(&ss.ServiceID, &scopesJSON, &ss.RequirementType); err != nil {
			return nil, storage.NewStorageError("LoadServiceScopes", storage.ErrorKindUnknown, err, "failed to scan service scope row")
		}
		if err := json.Unmarshal(scopesJSON, &ss.Scopes); err != nil {
			return nil, storage.NewStorageError("LoadServiceScopes", storage.ErrorKindUnknown, err, "failed to unmarshal scopes")
		}
		scopes = append(scopes, ss)
	}

	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("LoadServiceScopes", storage.ErrorKindUnknown, err, "error iterating service scope rows")
	}

	return scopes, nil
}

// handlePostgresError converts PostgreSQL errors to StorageError.
func (r *PermissionSetRepository) handlePostgresError(operation string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, fmt.Sprintf("conflict: %s", pgErr.Detail))
		case "23503": // foreign_key_violation — referenced service_id does not exist
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, fmt.Sprintf("referenced service_id does not exist: %s", pgErr.Detail))
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation timed out")
	}

	return storage.NewStorageError(operation, storage.ErrorKindConnection, err, "PostgreSQL operation failed")
}
