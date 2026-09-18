package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ToolApprovalRepository implements the three tool approval repository interfaces using PostgreSQL.
type ToolApprovalRepository struct {
	adapter *Adapter
}

// NewToolApprovalRepository creates a new PostgreSQL tool approval repository.
func NewToolApprovalRepository(adapter *Adapter) *ToolApprovalRepository {
	return &ToolApprovalRepository{adapter: adapter}
}

var (
	_ ports.ToolApprovalRepository        = (*ToolApprovalRepository)(nil)
	_ ports.ToolApprovalQueryRepository   = (*ToolApprovalRepository)(nil)
	_ ports.ToolApprovalMetricsRepository = (*ToolApprovalRepository)(nil)
)

var _ ports.ApprovalMutationSyncRepository = (*ToolApprovalRepository)(nil)

func (*ToolApprovalRepository) ApprovalMutationsSyncAtomically() bool {
	return true
}

// Create inserts a new tool approval or returns existing pending duplicate (idempotent).
// Uses ON CONFLICT against the partial unique index on (principal, agent_id, tool_name, arguments_hash)
// WHERE status='pending' AND consumed=FALSE.
func (r *ToolApprovalRepository) Create(ctx context.Context, approval *storage.ToolApproval) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.create.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "CreateToolApproval"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if approval == nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindValidation,
			nil,
			"approval cannot be nil",
		)
	}
	if approval.ToolPattern == "" {
		return nil, storage.NewStorageError("CreateToolApproval", storage.ErrorKindValidation, storage.ErrApprovalPatternMissing, "approval tool pattern is required")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	// Marshal arguments to JSON
	argsJSON, err := json.Marshal(approval.Arguments)
	if err != nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindValidation,
			err,
			"failed to marshal arguments to JSON",
		)
	}
	paramsPatternJSON, err := marshalParamsPattern(approval.ParamsPattern)
	if err != nil {
		return nil, storage.NewStorageError("CreateToolApproval", storage.ErrorKindValidation, err, "failed to marshal params pattern to JSON")
	}

	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to begin transaction",
		)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(execCtx, `
		UPDATE tool_approvals
		SET consumed = TRUE
		WHERE principal = $1 AND agent_id = $2 AND tool_name = $3 AND arguments_hash = $4
		  AND status = 'pending' AND consumed = FALSE AND expires_at <= NOW()
	`, approval.Principal, approval.AgentID, approval.ToolName, approval.ArgumentsHash)
	if err != nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to retire expired duplicate approval",
		)
	}

	// Use INSERT ... ON CONFLICT DO NOTHING + a subsequent SELECT to handle idempotency
	// The partial unique index idx_tool_approvals_pending_dedup enforces uniqueness for pending records.
	insertQuery := `
		INSERT INTO tool_approvals (
			id, principal, agent_id, gateway_client_id, tool_name,
			arguments, arguments_hash, description, risk_level,
			mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
			status, approval_url, created_at, expires_at, tool_pattern, params_pattern
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (principal, agent_id, tool_name, arguments_hash) WHERE status = 'pending' AND consumed = FALSE
		DO NOTHING
	`

	result, err := tx.ExecContext(
		execCtx,
		insertQuery,
		approval.ID,
		approval.Principal,
		approval.AgentID,
		approval.GatewayClientID,
		approval.ToolName,
		argsJSON,
		approval.ArgumentsHash,
		approval.Description,
		approval.RiskLevel,
		approval.MCPSessionID,
		approval.AgentSessionID,
		approval.ToolInvocationID,
		approval.OpenTelemetryTraceparent,
		approval.Status,
		approval.ApprovalURL,
		approval.CreatedAt,
		approval.ExpiresAt,
		approval.ToolPattern,
		paramsPatternJSON,
	)

	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"CreateToolApproval",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to create tool approval",
		)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 1 {
		if err := tx.Commit(); err != nil {
			return nil, storage.NewStorageError(
				"CreateToolApproval",
				storage.ErrorKindConnection,
				err,
				"failed to commit tool approval",
			)
		}
		return approval, nil
	}

	// No rows inserted — existing pending record with same dedup key exists.
	// Fetch and return the existing record.
	selectQuery := `
		SELECT id, principal, agent_id, gateway_client_id, tool_name,
		       arguments, arguments_hash, description, risk_level,
		       mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		       status, persistence, consumed, approval_url,
		       created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM tool_approvals
		WHERE principal = $1 AND agent_id = $2 AND tool_name = $3 AND arguments_hash = $4
		  AND status = 'pending' AND consumed = FALSE
		LIMIT 1
	`

	var argumentsJSON []byte
	var existingParamsPatternJSON []byte
	existing := &storage.ToolApproval{}
	err = tx.QueryRowContext(execCtx, selectQuery,
		approval.Principal, approval.AgentID, approval.ToolName, approval.ArgumentsHash,
	).Scan(
		&existing.ID,
		&existing.Principal,
		&existing.AgentID,
		&existing.GatewayClientID,
		&existing.ToolName,
		&argumentsJSON,
		&existing.ArgumentsHash,
		&existing.Description,
		&existing.RiskLevel,
		&existing.MCPSessionID,
		&existing.AgentSessionID,
		&existing.ToolInvocationID,
		&existing.OpenTelemetryTraceparent,
		&existing.Status,
		&existing.Persistence,
		&existing.Consumed,
		&existing.ApprovalURL,
		&existing.CreatedAt,
		&existing.ApprovedAt,
		&existing.DeniedAt,
		&existing.ConsumedAt,
		&existing.ExpiresAt,
		&existing.ToolPattern,
		&existingParamsPatternJSON,
	)

	if err != nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to fetch existing duplicate approval",
		)
	}

	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &existing.Arguments); err != nil {
			return nil, storage.NewStorageError(
				"CreateToolApproval",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal arguments from JSON",
			)
		}
	}
	if err := unmarshalParamsPattern(existingParamsPatternJSON, &existing.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("CreateToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}

	if err := tx.Commit(); err != nil {
		return nil, storage.NewStorageError(
			"CreateToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to commit duplicate approval lookup",
		)
	}

	return existing, nil
}

// Get retrieves a tool approval by ID from PostgreSQL.
// Returns StorageError with Kind=NotFound if not found.
func (r *ToolApprovalRepository) Get(ctx context.Context, approvalID id.ApprovalID) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.get.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "GetToolApproval"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"GetToolApproval",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if approvalID.IsZero() {
		return nil, storage.NewStorageError(
			"GetToolApproval",
			storage.ErrorKindValidation,
			nil,
			"approval ID cannot be empty",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var argumentsJSON []byte
	var paramsPatternJSON []byte

	query := `
		SELECT id, principal, agent_id, gateway_client_id, tool_name,
		       arguments, arguments_hash, description, risk_level,
		       mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		       status, persistence, consumed, approval_url,
		       created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM tool_approvals
		WHERE id = $1
	`

	approval := &storage.ToolApproval{}
	err := r.adapter.db.QueryRowContext(queryCtx, query, approvalID).Scan(
		&approval.ID,
		&approval.Principal,
		&approval.AgentID,
		&approval.GatewayClientID,
		&approval.ToolName,
		&argumentsJSON,
		&approval.ArgumentsHash,
		&approval.Description,
		&approval.RiskLevel,
		&approval.MCPSessionID,
		&approval.AgentSessionID,
		&approval.ToolInvocationID,
		&approval.OpenTelemetryTraceparent,
		&approval.Status,
		&approval.Persistence,
		&approval.Consumed,
		&approval.ApprovalURL,
		&approval.CreatedAt,
		&approval.ApprovedAt,
		&approval.DeniedAt,
		&approval.ConsumedAt,
		&approval.ExpiresAt,
		&approval.ToolPattern,
		&paramsPatternJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"GetToolApproval",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"tool approval not found",
			)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"GetToolApproval",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"GetToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to get tool approval",
		)
	}

	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
			return nil, storage.NewStorageError(
				"GetToolApproval",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal arguments from JSON",
			)
		}
	}
	if err := unmarshalParamsPattern(paramsPatternJSON, &approval.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("GetToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}

	return approval, nil
}

// Approve transitions a pending tool approval to approved status in PostgreSQL.
// Returns StorageError with Kind=NotFound if not found or not in pending state.
func (r *ToolApprovalRepository) Approve(ctx context.Context, approvalID id.ApprovalID, decision storage.ApprovalDecision, approvedAt time.Time) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.approve.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ApproveToolApproval"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ApproveToolApproval",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if approvalID.IsZero() {
		return nil, storage.NewStorageError(
			"ApproveToolApproval",
			storage.ErrorKindValidation,
			nil,
			"approval ID cannot be empty",
		)
	}
	if decision.ToolPattern == "" {
		return nil, storage.NewStorageError("ApproveToolApproval", storage.ErrorKindValidation, storage.ErrApprovalPatternMissing, "approval tool pattern is required")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var argumentsJSON []byte
	paramsPatternJSON, err := marshalParamsPattern(decision.ParamsPattern)
	if err != nil {
		return nil, storage.NewStorageError("ApproveToolApproval", storage.ErrorKindValidation, err, "failed to marshal params pattern to JSON")
	}

	query := `
		UPDATE tool_approvals
		SET status = 'approved', persistence = $2, tool_pattern = $3, params_pattern = $4, approved_at = $5
		WHERE id = $1 AND status = 'pending' AND expires_at > NOW()
		RETURNING id, principal, agent_id, gateway_client_id, tool_name,
		          arguments, arguments_hash, description, risk_level,
		          mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		          status, persistence, consumed, approval_url,
		          created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
	`

	approval := &storage.ToolApproval{}
	var storedParamsPatternJSON []byte
	err = r.adapter.db.QueryRowContext(execCtx, query, approvalID, decision.Persistence, decision.ToolPattern, paramsPatternJSON, approvedAt).Scan(
		&approval.ID,
		&approval.Principal,
		&approval.AgentID,
		&approval.GatewayClientID,
		&approval.ToolName,
		&argumentsJSON,
		&approval.ArgumentsHash,
		&approval.Description,
		&approval.RiskLevel,
		&approval.MCPSessionID,
		&approval.AgentSessionID,
		&approval.ToolInvocationID,
		&approval.OpenTelemetryTraceparent,
		&approval.Status,
		&approval.Persistence,
		&approval.Consumed,
		&approval.ApprovalURL,
		&approval.CreatedAt,
		&approval.ApprovedAt,
		&approval.DeniedAt,
		&approval.ConsumedAt,
		&approval.ExpiresAt,
		&approval.ToolPattern,
		&storedParamsPatternJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"ApproveToolApproval",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"tool approval not found or not in pending state",
			)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"ApproveToolApproval",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ApproveToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to approve tool approval",
		)
	}

	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
			return nil, storage.NewStorageError(
				"ApproveToolApproval",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal arguments from JSON",
			)
		}
	}
	if err := unmarshalParamsPattern(storedParamsPatternJSON, &approval.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("ApproveToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}

	return approval, nil
}

// Deny transitions a pending tool approval to denied status in PostgreSQL.
// Returns StorageError with Kind=NotFound if not found or not in pending state.
func (r *ToolApprovalRepository) Deny(ctx context.Context, approvalID id.ApprovalID, persistence *storage.ApprovalPersistence, deniedAt time.Time) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.deny.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "DenyToolApproval"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"DenyToolApproval",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if approvalID.IsZero() {
		return nil, storage.NewStorageError(
			"DenyToolApproval",
			storage.ErrorKindValidation,
			nil,
			"approval ID cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var argumentsJSON []byte
	var paramsPatternJSON []byte

	query := `
		UPDATE tool_approvals
		SET status = 'denied', persistence = $2, denied_at = $3
		WHERE id = $1 AND status = 'pending' AND expires_at > NOW()
		RETURNING id, principal, agent_id, gateway_client_id, tool_name,
		          arguments, arguments_hash, description, risk_level,
		          mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		          status, persistence, consumed, approval_url,
		          created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
	`

	approval := &storage.ToolApproval{}
	err := r.adapter.db.QueryRowContext(execCtx, query, approvalID, persistence, deniedAt).Scan(
		&approval.ID,
		&approval.Principal,
		&approval.AgentID,
		&approval.GatewayClientID,
		&approval.ToolName,
		&argumentsJSON,
		&approval.ArgumentsHash,
		&approval.Description,
		&approval.RiskLevel,
		&approval.MCPSessionID,
		&approval.AgentSessionID,
		&approval.ToolInvocationID,
		&approval.OpenTelemetryTraceparent,
		&approval.Status,
		&approval.Persistence,
		&approval.Consumed,
		&approval.ApprovalURL,
		&approval.CreatedAt,
		&approval.ApprovedAt,
		&approval.DeniedAt,
		&approval.ConsumedAt,
		&approval.ExpiresAt,
		&approval.ToolPattern,
		&paramsPatternJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"DenyToolApproval",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"tool approval not found or not in pending state",
			)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"DenyToolApproval",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"DenyToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to deny tool approval",
		)
	}

	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
			return nil, storage.NewStorageError(
				"DenyToolApproval",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal arguments from JSON",
			)
		}
	}
	if err := unmarshalParamsPattern(paramsPatternJSON, &approval.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("DenyToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}

	return approval, nil
}

// Consume marks a once-persistence approved approval as consumed in PostgreSQL.
// Returns StorageError with Kind=NotFound if not found or not in approved+once state.
func (r *ToolApprovalRepository) Consume(ctx context.Context, approvalID id.ApprovalID, consumedAt time.Time) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.consume.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ConsumeToolApproval"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ConsumeToolApproval",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	if approvalID.IsZero() {
		return nil, storage.NewStorageError(
			"ConsumeToolApproval",
			storage.ErrorKindValidation,
			nil,
			"approval ID cannot be empty",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var argumentsJSON []byte
	var paramsPatternJSON []byte

	query := `
		UPDATE tool_approvals
		SET consumed = TRUE, consumed_at = $2
		WHERE id = $1 AND status = 'approved' AND persistence = 'once' AND consumed = FALSE
		RETURNING id, principal, agent_id, gateway_client_id, tool_name,
		          arguments, arguments_hash, description, risk_level,
		          mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		          status, persistence, consumed, approval_url,
		          created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
	`

	approval := &storage.ToolApproval{}
	err := r.adapter.db.QueryRowContext(execCtx, query, approvalID, consumedAt).Scan(
		&approval.ID,
		&approval.Principal,
		&approval.AgentID,
		&approval.GatewayClientID,
		&approval.ToolName,
		&argumentsJSON,
		&approval.ArgumentsHash,
		&approval.Description,
		&approval.RiskLevel,
		&approval.MCPSessionID,
		&approval.AgentSessionID,
		&approval.ToolInvocationID,
		&approval.OpenTelemetryTraceparent,
		&approval.Status,
		&approval.Persistence,
		&approval.Consumed,
		&approval.ApprovalURL,
		&approval.CreatedAt,
		&approval.ApprovedAt,
		&approval.DeniedAt,
		&approval.ConsumedAt,
		&approval.ExpiresAt,
		&approval.ToolPattern,
		&paramsPatternJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError(
				"ConsumeToolApproval",
				storage.ErrorKindNotFound,
				ports.ErrNotFound,
				"tool approval not found or not in approved+once+unconsumed state",
			)
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"ConsumeToolApproval",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ConsumeToolApproval",
			storage.ErrorKindConnection,
			err,
			"failed to consume tool approval",
		)
	}

	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
			return nil, storage.NewStorageError(
				"ConsumeToolApproval",
				storage.ErrorKindValidation,
				err,
				"failed to unmarshal arguments from JSON",
			)
		}
	}
	if err := unmarshalParamsPattern(paramsPatternJSON, &approval.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("ConsumeToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}

	return approval, nil
}

// RevokePermanent atomically clears a permanent approval or denial.
func (r *ToolApprovalRepository) RevokePermanent(ctx context.Context, approvalID id.ApprovalID, revokedAt time.Time) (*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.revoke_permanent.tool_approval")
	defer span.End()

	if r.adapter.db == nil {
		return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if approvalID.IsZero() {
		return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindValidation, nil, "approval ID cannot be empty")
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	var argumentsJSON []byte
	var paramsPatternJSON []byte
	query := `
		UPDATE tool_approvals
		SET status = 'denied', persistence = NULL, denied_at = $2
		WHERE id = $1 AND status IN ('approved', 'denied') AND persistence = 'permanent'
		RETURNING id, principal, agent_id, gateway_client_id, tool_name,
		          arguments, arguments_hash, description, risk_level,
		          mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		          status, persistence, consumed, approval_url,
		          created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
	`

	approval := &storage.ToolApproval{}
	err := r.adapter.db.QueryRowContext(execCtx, query, approvalID, revokedAt).Scan(
		&approval.ID,
		&approval.Principal,
		&approval.AgentID,
		&approval.GatewayClientID,
		&approval.ToolName,
		&argumentsJSON,
		&approval.ArgumentsHash,
		&approval.Description,
		&approval.RiskLevel,
		&approval.MCPSessionID,
		&approval.AgentSessionID,
		&approval.ToolInvocationID,
		&approval.OpenTelemetryTraceparent,
		&approval.Status,
		&approval.Persistence,
		&approval.Consumed,
		&approval.ApprovalURL,
		&approval.CreatedAt,
		&approval.ApprovedAt,
		&approval.DeniedAt,
		&approval.ConsumedAt,
		&approval.ExpiresAt,
		&approval.ToolPattern,
		&paramsPatternJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindNotFound, ports.ErrNotFound, "tool approval not found or not permanently approved or denied")
		}
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindConnection, err, "failed to revoke permanent tool approval")
	}
	if len(argumentsJSON) > 0 {
		if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
			return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal arguments from JSON")
		}
	}
	if err := unmarshalParamsPattern(paramsPatternJSON, &approval.ParamsPattern); err != nil {
		return nil, storage.NewStorageError("RevokePermanentToolApproval", storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
	}
	return approval, nil
}

// ListAllActive returns all non-expired, non-consumed active approvals, optionally filtered by principal and active agent sessions.
func (r *ToolApprovalRepository) ListAllActive(ctx context.Context, principalFilter *id.Principal, activeAgentSessionIDs []string) ([]*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.list_active.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ListAllActiveToolApprovals"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListAllActiveToolApprovals",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, gateway_client_id, tool_name,
		       arguments, arguments_hash, description, risk_level,
		       mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		       status, persistence, consumed, approval_url,
		       created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM tool_approvals
		WHERE consumed = FALSE
		  AND (status != 'pending' OR expires_at > NOW())
		  AND (persistence IS DISTINCT FROM 'session' OR agent_session_id = ANY($1::text[]))
		  AND ($2::text IS NULL OR principal = $2)
		ORDER BY created_at DESC
	`

	var principal any
	if principalFilter != nil {
		principal = string(*principalFilter)
	}
	rows, err := r.adapter.db.QueryContext(queryCtx, query, pq.Array(activeAgentSessionIDs), principal)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"ListAllActiveToolApprovals",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ListAllActiveToolApprovals",
			storage.ErrorKindConnection,
			err,
			"failed to list active approvals",
		)
	}
	defer func() { _ = rows.Close() }()

	return scanApprovalRows(rows, "ListAllActiveToolApprovals")
}

// ListActiveByPrincipalAndAgent returns active approvals for a specific (principal, agent) pair.
func (r *ToolApprovalRepository) ListActiveByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.list_active_by_pair.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ListActiveByPairToolApprovals"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListActiveByPairToolApprovals",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, gateway_client_id, tool_name,
		       arguments, arguments_hash, description, risk_level,
		       mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		       status, persistence, consumed, approval_url,
		       created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM tool_approvals
		WHERE principal = $1 AND agent_id = $2 AND consumed = FALSE
		  AND (status != 'pending' OR expires_at > NOW())
		ORDER BY created_at DESC
	`

	rows, err := r.adapter.db.QueryContext(queryCtx, query, principal, agentID)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"ListActiveByPairToolApprovals",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ListActiveByPairToolApprovals",
			storage.ErrorKindConnection,
			err,
			"failed to list active approvals by pair",
		)
	}
	defer func() { _ = rows.Close() }()

	return scanApprovalRows(rows, "ListActiveByPairToolApprovals")
}

// ListPermanentByPrincipal returns all permanent approvals and denials for a principal.
func (r *ToolApprovalRepository) ListPermanentByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.ToolApproval, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.list_permanent.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "ListPermanentToolApprovals"),
	)

	if r.adapter.db == nil {
		return nil, storage.NewStorageError(
			"ListPermanentToolApprovals",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT id, principal, agent_id, gateway_client_id, tool_name,
		       arguments, arguments_hash, description, risk_level,
		       mcp_session_id, agent_session_id, tool_invocation_id, opentelemetry_traceparent,
		       status, persistence, consumed, approval_url,
		       created_at, approved_at, denied_at, consumed_at, expires_at, tool_pattern, params_pattern
		FROM tool_approvals
		WHERE principal = $1 AND persistence = 'permanent'
		ORDER BY created_at DESC
	`

	rows, err := r.adapter.db.QueryContext(queryCtx, query, principal)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return nil, storage.NewStorageError(
				"ListPermanentToolApprovals",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return nil, storage.NewStorageError(
			"ListPermanentToolApprovals",
			storage.ErrorKindConnection,
			err,
			"failed to list permanent approvals",
		)
	}
	defer func() { _ = rows.Close() }()

	return scanApprovalRows(rows, "ListPermanentToolApprovals")
}

// CountPendingByPrincipalAndAgent returns the number of non-expired pending approvals for a (principal, agent) pair.
func (r *ToolApprovalRepository) CountPendingByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.count_pending.tool_approval")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "CountPendingToolApprovals"),
	)

	if r.adapter.db == nil {
		return 0, storage.NewStorageError(
			"CountPendingToolApprovals",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	query := `
		SELECT COUNT(*)
		FROM tool_approvals
		WHERE principal = $1 AND agent_id = $2
		  AND status = 'pending' AND expires_at > NOW()
	`

	var count int
	err := r.adapter.db.QueryRowContext(queryCtx, query, principal, agentID).Scan(&count)
	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return 0, storage.NewStorageError(
				"CountPendingToolApprovals",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return 0, storage.NewStorageError(
			"CountPendingToolApprovals",
			storage.ErrorKindConnection,
			err,
			"failed to count pending approvals",
		)
	}

	return count, nil
}

// scanApprovalRows scans multiple rows into ToolApproval structs, handling JSONB arguments.
func scanApprovalRows(rows *sql.Rows, operation string) ([]*storage.ToolApproval, error) {
	var approvals []*storage.ToolApproval
	for rows.Next() {
		var argumentsJSON []byte
		var paramsPatternJSON []byte
		approval := &storage.ToolApproval{}
		if err := rows.Scan(
			&approval.ID,
			&approval.Principal,
			&approval.AgentID,
			&approval.GatewayClientID,
			&approval.ToolName,
			&argumentsJSON,
			&approval.ArgumentsHash,
			&approval.Description,
			&approval.RiskLevel,
			&approval.MCPSessionID,
			&approval.AgentSessionID,
			&approval.ToolInvocationID,
			&approval.OpenTelemetryTraceparent,
			&approval.Status,
			&approval.Persistence,
			&approval.Consumed,
			&approval.ApprovalURL,
			&approval.CreatedAt,
			&approval.ApprovedAt,
			&approval.DeniedAt,
			&approval.ConsumedAt,
			&approval.ExpiresAt,
			&approval.ToolPattern,
			&paramsPatternJSON,
		); err != nil {
			return nil, storage.NewStorageError(
				operation,
				storage.ErrorKindConnection,
				err,
				"failed to scan approval row",
			)
		}
		if len(argumentsJSON) > 0 {
			if err := json.Unmarshal(argumentsJSON, &approval.Arguments); err != nil {
				return nil, storage.NewStorageError(
					operation,
					storage.ErrorKindValidation,
					err,
					"failed to unmarshal arguments from JSON",
				)
			}
		}
		if err := unmarshalParamsPattern(paramsPatternJSON, &approval.ParamsPattern); err != nil {
			return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, err, "failed to unmarshal params pattern from JSON")
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, storage.NewStorageError(
			operation,
			storage.ErrorKindConnection,
			err,
			"error iterating approval rows",
		)
	}
	return approvals, nil
}

func marshalParamsPattern(params map[string]string) ([]byte, error) {
	if params == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(params)
}

func unmarshalParamsPattern(data []byte, target *map[string]string) error {
	params := make(map[string]string)
	if len(data) == 0 || string(data) == "null" {
		*target = params
		return nil
	}
	if err := json.Unmarshal(data, &params); err != nil {
		return err
	}
	*target = params
	return nil
}
