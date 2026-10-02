// Package postgres implements PostgreSQL storage adapters.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
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
	rows, err := r.adapter.storageExecutor(ctx).QueryxContext(
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
func insertClientURIs(ctx context.Context, tx *sqlx.Tx, agentID id.AgentID, uris []string) error {
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
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agent.ID {
		return refreshValidation("CreateAgent", errors.New("agent belongs to another authorization scope"))
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

	tx, owned, err := r.adapter.participantTx(execCtx)
	if err != nil {
		return storage.NewStorageError("CreateAgent", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() {
		if owned {
			_ = tx.Rollback()
		}
	}()

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

	if owned {
		owned = false
		if err := commitAuthorizationTx(tx); err != nil {
			return err
		}
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
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agentID {
		return nil, refreshValidation("GetAgent", errors.New("agent belongs to another authorization scope"))
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var serviceReqsJSON, permissionSetsJSON []byte

	agent := &storage.Agent{}
	err := r.adapter.storageExecutor(queryCtx).QueryRowxContext(
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

func (r *AgentRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.Agent, error) {
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var agentID id.AgentID
	if err := r.adapter.storageExecutor(queryCtx).QueryRowxContext(queryCtx, `SELECT id FROM agents WHERE canonical_id = $1`, canonicalID).Scan(&agentID); err != nil {
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
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agent.ID {
		return refreshValidation("UpdateAgent", errors.New("agent belongs to another authorization scope"))
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

	tx, owned, err := r.adapter.participantTx(execCtx)
	if err != nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindConnection, err, "failed to begin transaction")
	}
	defer func() {
		if owned {
			_ = tx.Rollback()
		}
	}()

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

	if owned {
		owned = false
		if err := commitAuthorizationTx(tx); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes an agent and its local authorization state in one agent scope.
// Direct repository callers enter the same coordinated revocation as the domain service.
func (r *AgentRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError(
			"DeleteAgent",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if agentID.IsZero() {
		return storage.NewStorageError(
			"DeleteAgent",
			storage.ErrorKindValidation,
			nil,
			"agent ID cannot be empty",
		)
	}
	if _, scoped := scopedAuthorization(ctx); !scoped {
		var entered bool
		err := NewAuthorizationSessionCoordinator(r.adapter).Run(ctx, agentID, func(owner context.Context, at time.Time) error {
			entered = true
			if err := NewRefreshSessionRepo(r.adapter).RevokeByAgent(owner, agentID, at, storage.RefreshReasonAgentDeleted); err != nil {
				return err
			}
			return r.Delete(owner, agentID)
		})
		if !entered && ports.IsNotFoundErr(err) {
			return nil
		}
		return err
	}
	if err := requireAuthorizationAgent(ctx, agentID); err != nil {
		return err
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	exec := r.adapter.storageExecutor(execCtx)
	var missingReceipt bool
	if err := exec.QueryRowxContext(execCtx, `SELECT EXISTS (
		SELECT 1 FROM refresh_sessions s WHERE s.agent_id = $1
		AND (s.terminal_reason IS NULL OR NOT EXISTS (
			SELECT 1 FROM refresh_revocation_receipts receipt
			WHERE receipt.session_id = s.id AND receipt.reason = s.terminal_reason
				AND receipt.agent_id = s.agent_id AND receipt.principal = s.principal
				AND receipt.client_id = s.client_id)))`, agentID).Scan(&missingReceipt); err != nil {
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindConnection, err, "failed to verify refresh revocation receipts")
	}
	if missingReceipt {
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindValidation, nil, "refresh sessions need durable revocation receipts before agent deletion")
	}
	// PKCE rows have no FK to authorization codes, so remove their agent-owned hashes before the code cascade.
	if _, err := exec.ExecContext(execCtx, `DELETE FROM pkce_sessions WHERE signature IN (
		SELECT code_hash FROM authorization_codes WHERE agent_id = $1)`, agentID); err != nil {
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindConnection, err, "failed to delete agent PKCE sessions")
	}
	if _, err := exec.ExecContext(execCtx, `DELETE FROM agents WHERE id = $1`, agentID); err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return storage.NewStorageError("DeleteAgent", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindConnection, err, "failed to delete agent")
	}
	return nil
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

	where, args := "", []any(nil)
	if scope, ok := scopedAuthorization(ctx); ok {
		where, args = " WHERE id = $1", []any{scope.agentID}
	}
	rows, err := r.adapter.storageExecutor(queryCtx).QueryxContext(
		queryCtx,
		`SELECT id, canonical_id, client_id, external_id, display_name, description,
		        governance_url, user_documentation_url, agent_interface_url,
		        service_requirements, permission_sets, redirect_uris, allowed_scopes, created_at, updated_at
	 FROM agents`+where+` ORDER BY created_at DESC`, args...)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("ListAgents", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("ListAgents", storage.ErrorKindConnection, err, "failed to list agents")
	}
	defer func() { _ = rows.Close() }()

	var agents []*storage.Agent
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
			return nil, storage.NewStorageError("ListAgents", storage.ErrorKindConnection, err, "failed to scan agent row")
		}
		if len(serviceReqsJSON) > 0 {
			if err := json.Unmarshal(serviceReqsJSON, &agent.ServiceRequirements); err != nil {
				return nil, storage.NewStorageError("ListAgents", storage.ErrorKindValidation, err, "failed to unmarshal service_requirements from JSON")
			}
		}
		if len(permissionSetsJSON) > 0 {
			if err := json.Unmarshal(permissionSetsJSON, &agent.PermissionSets); err != nil {
				return nil, storage.NewStorageError("ListAgents", storage.ErrorKindValidation, err, "failed to unmarshal permission_sets from JSON")
			}
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError("ListAgents", storage.ErrorKindConnection, err, "error iterating agent rows")
	}
	if err := rows.Close(); err != nil {
		return nil, storage.NewStorageError("ListAgents", storage.ErrorKindConnection, err, "failed to close agent rows")
	}

	if len(agents) == 0 {
		return []*storage.Agent{}, nil
	}

	agentIDs := make([]id.AgentID, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID
	}
	uriMap, err := r.batchFetchClientURIs(queryCtx, agentIDs)
	if err != nil {
		return nil, err
	}

	result := make([]*storage.Agent, len(agents))
	for i, agent := range agents {
		agent.ClientURIs = uriMap[agent.ID]
		if agent.ClientURIs == nil {
			agent.ClientURIs = []string{}
		}
		result[i] = agent.Copy()
	}
	return result, nil
}

// batchFetchClientURIs retrieves client URIs for all given agent IDs in a single query.
func (r *AgentRepository) batchFetchClientURIs(ctx context.Context, agentIDs []id.AgentID) (map[id.AgentID][]string, error) {
	uuids := make([]string, len(agentIDs))
	for i, agentID := range agentIDs {
		uuids[i] = agentID.String()
	}

	rows, err := r.adapter.storageExecutor(ctx).QueryxContext(
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
	err := r.adapter.storageExecutor(queryCtx).QueryRowxContext(
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
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agent.ID {
		return nil, refreshValidation("GetAgentByClientID", errors.New("agent belongs to another authorization scope"))
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
		err = r.adapter.storageExecutor(queryCtx).QueryRowxContext(queryCtx,
			`SELECT EXISTS(SELECT 1 FROM agents WHERE client_id = $1)`,
			clientID,
		).Scan(&exists)
	} else {
		err = r.adapter.storageExecutor(queryCtx).QueryRowxContext(queryCtx,
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
	rows, err := r.adapter.storageExecutor(ctx).QueryxContext(ctx, `SELECT agent_id, client_uri FROM agent_client_uris WHERE client_uri LIKE '%*%'`)
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
	err := r.adapter.storageExecutor(queryCtx).QueryRowxContext(
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
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agent.ID {
		return nil, refreshValidation("GetAgentByClientURI", errors.New("agent belongs to another authorization scope"))
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
