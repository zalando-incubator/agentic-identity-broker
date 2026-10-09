// Package postgres implements PostgreSQL storage adapters.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
)

// emptyIfNil converts a nil string slice to an empty slice.
// pq.Array returns SQL NULL for a nil slice, which violates NOT NULL constraints.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// AgentRepository implements ports.AgentRepository using PostgreSQL.
type AgentRepository struct {
	adapter *Adapter
}

// NewAgentRepository creates a new PostgreSQL agent repository.
// The adapter must be initialized before use.
func NewAgentRepository(adapter *Adapter) *AgentRepository {
	return &AgentRepository{
		adapter: adapter,
	}
}

// fetchClientURIs retrieves all client URIs for an agent from the agent_client_uris table.
func (r *AgentRepository) fetchClientURIs(ctx context.Context, agentID id.AgentID) ([]string, error) {
	rows, err := r.adapter.db.QueryContext(
		ctx,
		`SELECT client_uri FROM agent_client_uris WHERE agent_id = $1 ORDER BY client_uri`,
		agentID,
	)
	if err != nil {
		return nil, storage.NewStorageError("fetchClientURIs", storage.ErrorKindConnection, err, "failed to query client URIs")
	}
	defer func() { _ = rows.Close() }()

	var uris []string
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return nil, storage.NewStorageError("fetchClientURIs", storage.ErrorKindConnection, err, "failed to scan client URI")
		}
		uris = append(uris, uri)
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("fetchClientURIs", storage.ErrorKindConnection, err, "error iterating client URI rows")
	}
	if uris == nil {
		uris = []string{}
	}
	return uris, nil
}

// insertClientURIs inserts client URIs into agent_client_uris within an existing transaction.
// Returns StorageError with Kind=Conflict if a client_uri uniqueness violation occurs.
func insertClientURIs(ctx context.Context, tx *sql.Tx, agentID id.AgentID, uris []string) error {
	for _, uri := range uris {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO agent_client_uris (agent_id, client_uri) VALUES ($1, $2)`,
			agentID,
			uri,
		)
		if err != nil {
			if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
				if strings.Contains(pgErr.ConstraintName, "client_uri") {
					return storage.NewStorageError(
						"insertClientURIs",
						storage.ErrorKindConflict,
						err,
						"client URI already registered to another agent",
					)
				}
			}
			return storage.NewStorageError("insertClientURIs", storage.ErrorKindConnection, err, "failed to insert client URI")
		}
	}
	return nil
}

// Create creates a new agent entity in PostgreSQL.
// Generates a UUID for the agent if ID is empty.
// Returns StorageError with Kind=Conflict if agent ID, client_id, or a client_uri already exists.
func (r *AgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"CreateAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if agent == nil {
		return storage.NewStorageError(
			"CreateAgent",
			storage.ErrorKindValidation,
			nil,
			"agent cannot be nil",
		)
	}

	if err := agent.ValidateForCreate(); err != nil {
		return storage.NewStorageError(
			"CreateAgent",
			storage.ErrorKindValidation,
			err,
			"agent validation failed",
		)
	}

	if agent.ID.IsZero() {
		agent.ID = id.NewAgentID()
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var serviceReqsJSON []byte
	var err error
	if agent.ServiceRequirements != nil {
		serviceReqsJSON, err = json.Marshal(agent.ServiceRequirements)
		if err != nil {
			return storage.NewStorageError(
				"CreateAgent",
				storage.ErrorKindValidation,
				err,
				"failed to marshal service_requirements to JSON",
			)
		}
	}

	var permissionSetsJSON []byte
	if len(agent.PermissionSets) > 0 {
		permissionSetsJSON, err = json.Marshal(agent.PermissionSets)
		if err != nil {
			return storage.NewStorageError(
				"CreateAgent",
				storage.ErrorKindValidation,
				err,
				"failed to marshal permission_sets to JSON",
			)
		}
	}

	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return storage.NewStorageError("CreateAgent", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if len(agent.PermissionSets) > 0 {
		psIDs := make([]id.PermissionSetID, len(agent.PermissionSets))
		for i, aps := range agent.PermissionSets {
			psIDs[i] = aps.PermissionSetID
		}
		if err = verifyPermissionSetExistenceInTx(execCtx, tx, psIDs); err != nil {
			return err
		}
	}

	if len(agent.ServiceRequirements) > 0 {
		serviceIDs := make([]id.ServiceID, len(agent.ServiceRequirements))
		for i, requirement := range agent.ServiceRequirements {
			serviceIDs[i] = requirement.ServiceID
		}
		if err = verifyServiceExistenceInTx(execCtx, tx, serviceIDs); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(
		execCtx,
		`INSERT INTO agents (
			id, canonical_id, client_id, external_id, display_name, description,
			governance_url, user_documentation_url, agent_interface_url,
			service_requirements, permission_sets, redirect_uris, allowed_scopes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		agent.ID, agent.CanonicalID, agent.ClientID, agent.ExternalID, agent.DisplayName, agent.Description,
		agent.GovernanceURL, agent.UserDocumentationURL, agent.AgentInterfaceURL,
		serviceReqsJSON, permissionSetsJSON, pq.Array(emptyIfNil(agent.RedirectURIs)),
		pq.Array(emptyIfNil(agent.AllowedScopes)), agent.CreatedAt, agent.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return storage.NewStorageError("CreateAgent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "pkey") {
				return storage.NewStorageError("CreateAgent", storage.ErrorKindConflict, err, "agent with this ID already exists")
			}
			if strings.Contains(pgErr.ConstraintName, "client_id") {
				return storage.NewStorageError("CreateAgent", storage.ErrorKindConflict, err, "agent with this client_id already exists")
			}
			if pgErr.ConstraintName == "uq_agents_canonical_id" {
				return storage.NewStorageError("CreateAgent", storage.ErrorKindConflict, err, "agent canonical_id already exists")
			}
		}
		return storage.NewStorageError("CreateAgent", storage.ErrorKindConnection, err, "failed to create agent")
	}

	if err := insertClientURIs(execCtx, tx, agent.ID, agent.ClientURIs); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return storage.NewStorageError("CreateAgent", storage.ErrorKindConnection, err, "failed to commit transaction")
	}
	return nil
}

// Get retrieves an agent entity by ID from PostgreSQL.
// Returns StorageError with Kind=NotFound if agent not found.
func (r *AgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.get.agent")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "GetAgent"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"GetAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if agentID.IsZero() {
		return nil, storage.NewStorageError(
			"GetAgent",
			storage.ErrorKindValidation,
			nil,
			"agent ID cannot be empty",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var serviceReqsJSON, permissionSetsJSON []byte

	agent := &storage.Agent{}
	err := r.adapter.db.QueryRowContext(
		queryCtx,
		`SELECT id, canonical_id, client_id, external_id, display_name, description,
		        governance_url, user_documentation_url, agent_interface_url,
		        service_requirements, permission_sets, redirect_uris, allowed_scopes, created_at, updated_at
	 FROM agents
	 WHERE id = $1`,
		agentID,
	).Scan(
		&agent.ID, &agent.CanonicalID, &agent.ClientID, &agent.ExternalID,
		&agent.DisplayName, &agent.Description,
		&agent.GovernanceURL, &agent.UserDocumentationURL, &agent.AgentInterfaceURL,
		&serviceReqsJSON, &permissionSetsJSON,
		pq.Array(&agent.RedirectURIs), pq.Array(&agent.AllowedScopes),
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.NewStorageError("GetAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("GetAgent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("GetAgent", storage.ErrorKindConnection, err, "failed to get agent")
	}
	if len(serviceReqsJSON) > 0 {
		if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
			return nil, storage.NewStorageError(
				"GetAgent",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal service_requirements from JSON",
			)
		}
	}

	if len(permissionSetsJSON) > 0 {
		if err := json.Unmarshal(permissionSetsJSON, &agent.PermissionSets); err != nil {
			return nil, storage.NewStorageError(
				"GetAgent",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal permission_sets from JSON",
			)
		}
	}

	uris, err := r.fetchClientURIs(queryCtx, agentID)
	if err != nil {
		return nil, err
	}
	agent.ClientURIs = uris

	return agent.Copy(), nil
}

func (r *AgentRepository) GetByIDs(ctx context.Context, ids []id.AgentID) ([]*storage.Agent, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("GetAgentsByIDs", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if len(ids) == 0 {
		return []*storage.Agent{}, nil
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	rows, err := r.adapter.db.QueryContext(queryCtx, `SELECT id, canonical_id, client_id, external_id, display_name, description,
		governance_url, user_documentation_url, agent_interface_url, service_requirements, permission_sets,
		redirect_uris, allowed_scopes, created_at, updated_at FROM agents WHERE id = ANY($1::uuid[])`, pq.Array(ids))
	if err != nil {
		kind := storage.ErrorKindConnection
		if errors.Is(err, context.DeadlineExceeded) {
			kind = storage.ErrorKindTimeout
		}
		return nil, storage.NewStorageError("GetAgentsByIDs", kind, err, "failed to get agents")
	}
	defer func() { _ = rows.Close() }()
	agents, err := r.scanAgentRows(queryCtx, rows, "GetAgentsByIDs")
	if err != nil {
		return nil, err
	}
	byID := make(map[id.AgentID]*storage.Agent, len(agents))
	for _, agent := range agents {
		byID[agent.ID] = agent
	}
	result := make([]*storage.Agent, 0, len(agents))
	for _, agentID := range ids {
		if agent := byID[agentID]; agent != nil {
			result = append(result, agent)
			delete(byID, agentID)
		}
	}
	return result, nil
}

func (r *AgentRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.Agent, error) {
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var agentID id.AgentID
	if err := r.adapter.db.QueryRowContext(queryCtx, `SELECT id FROM agents WHERE canonical_id = $1`, canonicalID).Scan(&agentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.NewStorageError("GetAgentByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
		}
		return nil, storage.NewStorageError("GetAgentByCanonicalID", storage.ErrorKindConnection, err, "failed to get agent")
	}
	return r.Get(ctx, agentID)
}

// Update updates an existing agent entity in PostgreSQL.
// Returns StorageError with Kind=NotFound if agent ID not found.
// Returns StorageError with Kind=Conflict if a client_uri uniqueness violation occurs.
func (r *AgentRepository) Update(ctx context.Context, agent *storage.Agent) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"UpdateAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if agent == nil {
		return storage.NewStorageError(
			"UpdateAgent",
			storage.ErrorKindValidation,
			nil,
			"agent cannot be nil",
		)
	}

	if err := agent.Validate(); err != nil {
		return storage.NewStorageError(
			"UpdateAgent",
			storage.ErrorKindValidation,
			err,
			"agent validation failed",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var serviceReqsJSON []byte
	var err error
	if agent.ServiceRequirements != nil {
		serviceReqsJSON, err = json.Marshal(agent.ServiceRequirements)
		if err != nil {
			return storage.NewStorageError(
				"UpdateAgent",
				storage.ErrorKindValidation,
				err,
				"failed to marshal service_requirements to JSON",
			)
		}
	}

	var permissionSetsJSON []byte
	if len(agent.PermissionSets) > 0 {
		permissionSetsJSON, err = json.Marshal(agent.PermissionSets)
		if err != nil {
			return storage.NewStorageError(
				"UpdateAgent",
				storage.ErrorKindValidation,
				err,
				"failed to marshal permission_sets to JSON",
			)
		}
	}

	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(
		execCtx,
		`UPDATE agents SET canonical_id=$2, client_id=$3, external_id=$4, display_name=$5,
		 description=$6, governance_url=$7, user_documentation_url=$8, agent_interface_url=$9,
		 service_requirements=$10, permission_sets=$11, redirect_uris=$12, allowed_scopes=$13, updated_at=$14
		 WHERE id=$1`,
		agent.ID, agent.CanonicalID, agent.ClientID, agent.ExternalID, agent.DisplayName, agent.Description,
		agent.GovernanceURL, agent.UserDocumentationURL, agent.AgentInterfaceURL, serviceReqsJSON, permissionSetsJSON,
		pq.Array(emptyIfNil(agent.RedirectURIs)), pq.Array(emptyIfNil(agent.AllowedScopes)), agent.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return storage.NewStorageError("UpdateAgent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
			if strings.Contains(pgErr.ConstraintName, "client_id") {
				return storage.NewStorageError("UpdateAgent", storage.ErrorKindConflict, err, "agent with this client_id already exists")
			}
			if pgErr.ConstraintName == "uq_agents_canonical_id" {
				return storage.NewStorageError("UpdateAgent", storage.ErrorKindConflict, err, "agent canonical_id already exists")
			}
		}
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to update agent")
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to get affected rows")
	}
	if rows == 0 {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}

	_, err = tx.ExecContext(execCtx, `DELETE FROM agent_client_uris WHERE agent_id = $1`, agent.ID)
	if err != nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to delete existing client URIs")
	}

	if err := insertClientURIs(execCtx, tx, agent.ID, agent.ClientURIs); err != nil {
		return err
	}

	if len(agent.PermissionSets) > 0 {
		psIDs := make([]id.PermissionSetID, len(agent.PermissionSets))
		for i, aps := range agent.PermissionSets {
			psIDs[i] = aps.PermissionSetID
		}
		if err = verifyPermissionSetExistenceInTx(execCtx, tx, psIDs); err != nil {
			return err
		}
	}

	if len(agent.ServiceRequirements) > 0 {
		serviceIDs := make([]id.ServiceID, len(agent.ServiceRequirements))
		for i, requirement := range agent.ServiceRequirements {
			serviceIDs[i] = requirement.ServiceID
		}
		if err = verifyServiceExistenceInTx(execCtx, tx, serviceIDs); err != nil {
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to commit transaction")
	}
	return nil
}

// Delete deletes an agent entity by ID from PostgreSQL.
// Idempotent: returns false if agent doesn't exist.
// Associated grants and client URIs are CASCADE deleted per FR-021.
func (r *AgentRepository) Delete(ctx context.Context, agentID id.AgentID) (bool, error) {
	if r.adapter.db == nil {
		return false, storage.NewStorageError(
			"DeleteAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if agentID.IsZero() {
		return false, storage.NewStorageError(
			"DeleteAgent",
			storage.ErrorKindValidation,
			nil,
			"agent ID cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	result, err := r.adapter.db.ExecContext(execCtx, `DELETE FROM agents WHERE id = $1`, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return false, storage.NewStorageError("DeleteAgent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return false, storage.NewStorageError("DeleteAgent", storage.ErrorKindConnection, err, "failed to delete agent")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, storage.NewStorageError("DeleteAgent", storage.ErrorKindUnknown, err, "failed to get rows affected")
	}
	return rowsAffected > 0, nil
}

// List retrieves all agent entities from PostgreSQL.
// Returns empty slice if no agents exist (not an error).
func (r *AgentRepository) List(ctx context.Context) ([]*storage.Agent, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListAgents",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	rows, err := r.adapter.db.QueryContext(
		queryCtx,
		`SELECT id, canonical_id, client_id, external_id, display_name, description,
		        governance_url, user_documentation_url, agent_interface_url,
		        service_requirements, permission_sets, redirect_uris, allowed_scopes, created_at, updated_at
	 FROM agents ORDER BY created_at DESC`,
	)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("ListAgents", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("ListAgents", storage.ErrorKindConnection, err, "failed to list agents")
	}
	defer func() { _ = rows.Close() }()

	return r.scanAgentRows(queryCtx, rows, "ListAgents")
}

func (r *AgentRepository) scanAgentRows(ctx context.Context, rows *sql.Rows, operation string) ([]*storage.Agent, error) {
	agents := make([]*storage.Agent, 0)
	for rows.Next() {
		agent := &storage.Agent{}
		var serviceReqsJSON, permissionSetsJSON []byte
		if err := rows.Scan(
			&agent.ID, &agent.CanonicalID, &agent.ClientID, &agent.ExternalID,
			&agent.DisplayName, &agent.Description,
			&agent.GovernanceURL, &agent.UserDocumentationURL, &agent.AgentInterfaceURL,
			&serviceReqsJSON, &permissionSetsJSON,
			pq.Array(&agent.RedirectURIs), pq.Array(&agent.AllowedScopes),
			&agent.CreatedAt, &agent.UpdatedAt,
		); err != nil {
			return nil, storage.NewStorageError(operation, storage.ErrorKindConnection, err, "failed to scan agent row")
		}
		if len(serviceReqsJSON) > 0 {
			if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
				return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, err, "failed to unmarshal service_requirements from JSON")
			}
		}
		if len(permissionSetsJSON) > 0 {
			if err := json.Unmarshal(permissionSetsJSON, &agent.PermissionSets); err != nil {
				return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, err, "failed to unmarshal permission_sets from JSON")
			}
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindConnection, err, "error iterating agent rows")
	}
	if len(agents) == 0 {
		return agents, nil
	}
	agentIDs := make([]id.AgentID, len(agents))
	for i, agent := range agents {
		agentIDs[i] = agent.ID
	}
	uriMap, err := r.batchFetchClientURIs(ctx, agentIDs)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		agent.ClientURIs = uriMap[agent.ID]
		if agent.ClientURIs == nil {
			agent.ClientURIs = []string{}
		}
	}
	return agents, nil
}

// batchFetchClientURIs retrieves client URIs for all given agent IDs in a single query.
func (r *AgentRepository) batchFetchClientURIs(ctx context.Context, agentIDs []id.AgentID) (map[id.AgentID][]string, error) {
	uuids := make([]string, len(agentIDs))
	for i, agentID := range agentIDs {
		uuids[i] = agentID.String()
	}

	rows, err := r.adapter.db.QueryContext(
		ctx,
		`SELECT agent_id, client_uri FROM agent_client_uris WHERE agent_id = ANY($1::uuid[]) ORDER BY client_uri`,
		pq.Array(uuids),
	)
	if err != nil {
		return nil, storage.NewStorageError("batchFetchClientURIs", storage.ErrorKindConnection, err, "failed to batch query client URIs")
	}
	defer func() { _ = rows.Close() }()

	result := make(map[id.AgentID][]string)
	for rows.Next() {
		var agentID id.AgentID
		var uri string
		if err := rows.Scan(&agentID, &uri); err != nil {
			return nil, storage.NewStorageError("batchFetchClientURIs", storage.ErrorKindConnection, err, "failed to scan client URI row")
		}
		result[agentID] = append(result[agentID], uri)
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("batchFetchClientURIs", storage.ErrorKindConnection, err, "error iterating client URI rows")
	}
	return result, nil
}

// GetByClientID retrieves an agent entity by client_id from PostgreSQL.
// Returns StorageError with Kind=NotFound if agent not found.
func (r *AgentRepository) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"GetAgentByClientID",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if clientID.IsZero() {
		return nil, storage.NewStorageError(
			"GetAgentByClientID",
			storage.ErrorKindValidation,
			nil,
			"client_id cannot be empty",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var serviceReqsJSON, permissionSetsJSON []byte
	agent := &storage.Agent{}
	err := r.adapter.db.QueryRowContext(
		queryCtx,
		`SELECT id, client_id, external_id, display_name, description,
		        governance_url, user_documentation_url, agent_interface_url,
		        service_requirements, permission_sets, redirect_uris, allowed_scopes, created_at, updated_at
		 FROM agents
		 WHERE client_id = $1`,
		clientID,
	).Scan(
		&agent.ID, &agent.ClientID, &agent.ExternalID,
		&agent.DisplayName, &agent.Description,
		&agent.GovernanceURL, &agent.UserDocumentationURL, &agent.AgentInterfaceURL,
		&serviceReqsJSON, &permissionSetsJSON,
		pq.Array(&agent.RedirectURIs), pq.Array(&agent.AllowedScopes),
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError("GetAgentByClientID", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("GetAgentByClientID", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("GetAgentByClientID", storage.ErrorKindConnection, err, "failed to get agent by client_id")
	}

	if len(serviceReqsJSON) > 0 {
		if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
			return nil, storage.NewStorageError(
				"GetAgentByClientID",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal service_requirements from JSON",
			)
		}
	}

	if len(permissionSetsJSON) > 0 {
		if err := json.Unmarshal(permissionSetsJSON, &agent.PermissionSets); err != nil {
			return nil, storage.NewStorageError(
				"GetAgentByClientID",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal permission_sets from JSON",
			)
		}
	}

	uris, err := r.fetchClientURIs(queryCtx, agent.ID)
	if err != nil {
		return nil, err
	}
	agent.ClientURIs = uris

	return agent.Copy(), nil
}

// ExistsOtherWithClientID reports whether any agent other than excludeAgentID shares the given client_id.
// When excludeAgentID is nil, all agents with that client_id are considered (create path).
func (r *AgentRepository) ExistsOtherWithClientID(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) (bool, error) {
	if r.adapter.db == nil {
		return false, storage.NewStorageError("ExistsOtherWithClientID", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var exists bool
	var err error
	if excludeAgentID == nil {
		err = r.adapter.db.QueryRowContext(queryCtx,
			`SELECT EXISTS(SELECT 1 FROM agents WHERE client_id = $1)`,
			clientID,
		).Scan(&exists)
	} else {
		err = r.adapter.db.QueryRowContext(queryCtx,
			`SELECT EXISTS(SELECT 1 FROM agents WHERE client_id = $1 AND id <> $2)`,
			clientID, *excludeAgentID,
		).Scan(&exists)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, storage.NewStorageError("ExistsOtherWithClientID", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return false, storage.NewStorageError("ExistsOtherWithClientID", storage.ErrorKindConnection, err, "database query failed")
	}
	return exists, nil
}

func (r *AgentRepository) findAgentByCIMDClientURIPattern(ctx context.Context, candidate string) (id.AgentID, error) {
	rows, err := r.adapter.db.QueryContext(ctx, `SELECT agent_id, client_uri FROM agent_client_uris WHERE client_uri LIKE '%*%'`)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConnection, err, "failed to find CIMD client URI patterns")
	}
	defer func() { _ = rows.Close() }()

	matchedAgentIDs := make(map[id.AgentID]struct{})
	for rows.Next() {
		var agentID id.AgentID
		var pattern string
		if err := rows.Scan(&agentID, &pattern); err != nil {
			return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConnection, err, "failed to scan CIMD client URI pattern")
		}
		if urivalidation.MatchesCIMDClientURI(pattern, candidate) {
			matchedAgentIDs[agentID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConnection, err, "failed to read CIMD client URI patterns")
	}

	if len(matchedAgentIDs) == 0 {
		return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	if len(matchedAgentIDs) > 1 {
		return id.AgentID{}, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConflict, nil, "CIMD client URI matches multiple agents")
	}
	for agentID := range matchedAgentIDs {
		return agentID, nil
	}

	panic("unreachable: non-empty map has no entries")
}

// GetByClientURI retrieves an agent entity by a pre-registered Client ID Metadata Document URL.
// Returns StorageError with Kind=NotFound if no agent has this URI registered.
func (r *AgentRepository) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"GetAgentByClientURI",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var serviceReqsJSON, permissionSetsJSON []byte
	agent := &storage.Agent{}
	err := r.adapter.db.QueryRowContext(
		queryCtx,
		`SELECT a.id, a.client_id, a.external_id, a.display_name, a.description,
		        a.governance_url, a.user_documentation_url, a.agent_interface_url,
		        a.service_requirements, a.permission_sets, a.redirect_uris, a.allowed_scopes,
		        a.created_at, a.updated_at
		 FROM agents a
		 JOIN agent_client_uris acu ON acu.agent_id = a.id
		 WHERE acu.client_uri = $1`,
		uri,
	).Scan(
		&agent.ID, &agent.ClientID, &agent.ExternalID,
		&agent.DisplayName, &agent.Description,
		&agent.GovernanceURL, &agent.UserDocumentationURL, &agent.AgentInterfaceURL,
		&serviceReqsJSON, &permissionSetsJSON,
		pq.Array(&agent.RedirectURIs), pq.Array(&agent.AllowedScopes),
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			patternAgentID, patternErr := r.findAgentByCIMDClientURIPattern(queryCtx, uri)
			if patternErr != nil {
				return nil, patternErr
			}
			return r.Get(ctx, patternAgentID)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConnection, err, "failed to get agent by client URI")
	}

	if len(serviceReqsJSON) > 0 {
		if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
			return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindValidation, err, "failed to unmarshal service_requirements from JSON")
		}
	}

	if len(permissionSetsJSON) > 0 {
		if err := json.Unmarshal(permissionSetsJSON, &agent.PermissionSets); err != nil {
			return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindValidation, err, "failed to unmarshal permission_sets from JSON")
		}
	}

	uris, err := r.fetchClientURIs(queryCtx, agent.ID)
	if err != nil {
		return nil, err
	}
	agent.ClientURIs = uris

	return agent.Copy(), nil
}
