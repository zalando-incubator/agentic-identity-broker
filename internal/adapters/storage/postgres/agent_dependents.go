package postgres

import (
	"context"
	"encoding/json"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.AgentDependentRepository = (*AgentRepository)(nil)

func (r *AgentRepository) ListGrantsByAgent(ctx context.Context, agentID id.AgentID) ([]*storage.UserGrant, error) {
	const operation = "ListAgentGrants"
	if r.adapter == nil || r.adapter.db == nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	query := `SELECT id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at
		FROM public.user_grants WHERE agent_id = $1 ORDER BY id`
	if _, joined := storageTransaction(ctx); joined {
		query += " FOR UPDATE"
	}
	rows, err := r.adapter.storageExecutor(queryCtx).QueryContext(queryCtx, query, agentID)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	defer func() { _ = rows.Close() }()
	var result []*storage.UserGrant
	for rows.Next() {
		grant := &storage.UserGrant{}
		var permissionSetsJSON []byte
		if err := rows.Scan(&grant.ID, &grant.Principal, &grant.AgentID, &grant.ValidUntil, &permissionSetsJSON, &grant.CreatedAt, &grant.UpdatedAt); err != nil {
			return nil, businessEventStorageError(operation, err)
		}
		if err := json.Unmarshal(permissionSetsJSON, &grant.GrantedPermissionSets); err != nil {
			return nil, businessEventStorageError(operation, err)
		}
		result = append(result, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	return result, nil
}

func (r *AgentRepository) ListApprovalsByAgent(ctx context.Context, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	const operation = "ListAgentApprovals"
	if r.adapter == nil || r.adapter.db == nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	query := `SELECT id, principal, agent_id, gateway_client_id, tool_name,
		arguments, arguments_hash, description, risk_level, mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		status, persistence, consumed, approval_url, created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM public.tool_approvals WHERE agent_id = $1 ORDER BY id`
	if _, joined := storageTransaction(ctx); joined {
		query += " FOR UPDATE"
	}
	rows, err := r.adapter.storageExecutor(queryCtx).QueryContext(queryCtx, query, agentID)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	defer func() { _ = rows.Close() }()
	return scanApprovalRows(rows, operation)
}
