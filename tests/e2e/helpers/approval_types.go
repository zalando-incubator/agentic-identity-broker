package helpers

import "time"

// ApprovalRecord represents a ToolApprovalDetail in E2E test responses.
type ApprovalRecord struct {
	ID               string            `json:"id"`
	Principal        string            `json:"principal"`
	AgentID          string            `json:"agent_id"`
	AgentDisplayName string            `json:"agent_display_name,omitempty"`
	MCPSessionID     *string           `json:"mcp_session_id,omitempty"`
	AgentSessionID   *string           `json:"agent_session_id,omitempty"`
	ToolInvocationID *string           `json:"tool_invocation_id,omitempty"`
	ToolName         string            `json:"tool_name"`
	Arguments        map[string]any    `json:"arguments"`
	ToolPattern      string            `json:"tool_pattern"`
	ParamsPattern    map[string]string `json:"params_pattern"`
	Description      string            `json:"description"`
	RiskLevel        string            `json:"risk_level"`
	Status           string            `json:"status"`
	Persistence      *string           `json:"persistence,omitempty"`
	Consumed         bool              `json:"consumed"`
	ApprovalURL      string            `json:"approval_url"`
	CreatedAt        time.Time         `json:"created_at"`
	ApprovedAt       *time.Time        `json:"approved_at,omitempty"`
	DeniedAt         *time.Time        `json:"denied_at,omitempty"`
	ConsumedAt       *time.Time        `json:"consumed_at,omitempty"`
	ExpiresAt        time.Time         `json:"expires_at"`
}

// CreateApprovalRequest is the request body for POST /api/approvals.
type CreateApprovalRequest struct {
	Metadata  CreateApprovalMetadata `json:"metadata"`
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]any         `json:"arguments"`
	RiskLevel string                 `json:"risk_level,omitempty"`
}

// CreateApprovalMetadata contains optional metadata for approval creation.
type CreateApprovalMetadata struct {
	MCPSessionID     string `json:"mcp_session_id,omitempty"`
	AgentSessionID   string `json:"agent_session_id,omitempty"`
	ToolInvocationID string `json:"tool_invocation_id,omitempty"`
	Description      string `json:"description"`
}

// CreateApprovalResponse is the response body for POST /api/approvals.
type CreateApprovalResponse struct {
	Data struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ApprovalURL string `json:"approval_url"`
		CreatedAt   string `json:"created_at"`
	} `json:"data"`
}

// ApprovalDetailResponse is the response body for GET /api/approvals/{id}.
type ApprovalDetailResponse struct {
	Data ApprovalRecord `json:"data"`
}

// ApprovalListResponse is the response body for list approval endpoints.
type ApprovalListResponse struct {
	Data []ApprovalRecord `json:"data"`
}

// ApproveRequest is the request body for POST /api/approvals/{id}/approve.
type ApproveRequest struct {
	Persistence   string            `json:"persistence"`
	ParamsPattern map[string]string `json:"params_pattern,omitempty"`
}

// ApproveResponse is the response body for POST /api/approvals/{id}/approve.
type ApproveResponse struct {
	Data struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		Persistence string `json:"persistence"`
		ApprovedAt  string `json:"approved_at"`
	} `json:"data"`
}

// DenyRequest is the request body for POST /api/approvals/{id}/deny.
type DenyRequest struct {
	Persistence string `json:"persistence,omitempty"`
}

// DenyResponse is the response body for POST /api/approvals/{id}/deny.
type DenyResponse struct {
	Data struct {
		ID          string  `json:"id"`
		Status      string  `json:"status"`
		Persistence *string `json:"persistence,omitempty"`
		DeniedAt    string  `json:"denied_at"`
	} `json:"data"`
}

// ConsumeResponse is the response body for POST /api/approvals/{id}/consume.
type ConsumeResponse struct {
	Data struct {
		ID         string `json:"id"`
		Consumed   bool   `json:"consumed"`
		ConsumedAt string `json:"consumed_at"`
	} `json:"data"`
}

// ApprovalSyncResponse is the response body for GET /api/approvals.
type ApprovalSyncResponse struct {
	Data struct {
		Pairs []ApprovalPair `json:"pairs"`
	} `json:"data"`
}

// ApprovalPair represents a (principal, agent) pair in the sync response.
type ApprovalPair struct {
	Principal             string            `json:"principal"`
	AgentID               string            `json:"agent_id"`
	Approvals             []ApprovalSummary `json:"approvals"`
	GrantedPermissionSets map[string]any    `json:"granted_permission_sets"`
}

// ApprovalSummary represents a trimmed approval in the sync response.
type ApprovalSummary struct {
	ID             string            `json:"id"`
	ToolName       string            `json:"tool_name"`
	ArgumentsHash  string            `json:"arguments_hash"`
	ToolPattern    string            `json:"tool_pattern"`
	ParamsPattern  map[string]string `json:"params_pattern"`
	Status         string            `json:"status"`
	Persistence    *string           `json:"persistence,omitempty"`
	Consumed       bool              `json:"consumed"`
	AgentSessionID *string           `json:"agent_session_id,omitempty"`
	ApprovedAt     *time.Time        `json:"approved_at,omitempty"`
}

// ApprovalErrorResponse is the error response for approval endpoints.
type ApprovalErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
