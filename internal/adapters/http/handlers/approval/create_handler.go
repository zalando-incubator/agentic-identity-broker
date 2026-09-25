package approval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// createRequest maps the CreateApprovalRequest schema from the OpenAPI spec.
type createRequest struct {
	Metadata *struct {
		MCPSessionID     *string `json:"mcp_session_id"`
		AgentSessionID   *string `json:"agent_session_id"`
		ToolInvocationID *string `json:"tool_invocation_id"`
		Description      *string `json:"description"`
	} `json:"metadata"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
	RiskLevel string         `json:"risk_level"`
}

// createResponse maps the CreateApprovalResponse schema.
type createResponse struct {
	Data createResponseData `json:"data"`
}

type createResponseData struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ApprovalURL string `json:"approval_url"`
	CreatedAt   string `json:"created_at"`
}

// CreateHandler handles POST /api/approvals.
type CreateHandler struct {
	service *domainapproval.Service
}

// NewCreateHandler creates a handler for creating pending approvals.
func NewCreateHandler(service *domainapproval.Service) *CreateHandler {
	return &CreateHandler{service: service}
}

func traceparentFromRequest(r *http.Request) *string {
	ctx := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(r.Header))
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return nil
	}
	carrier := propagation.HeaderCarrier{}
	propagation.TraceContext{}.Inject(trace.ContextWithSpanContext(context.Background(), spanContext), carrier)
	value := carrier.Get("Traceparent")
	return &value
}

func (h *CreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body")
		return
	}

	if req.ToolName == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tool_name is required")
		return
	}
	if req.Arguments == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "arguments is required")
		return
	}
	if req.Metadata == nil || req.Metadata.Description == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "metadata.description is required")
		return
	}

	actingPrincipal, ok := principal.FromContext(r.Context())
	if !ok || actingPrincipal == "" {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "approval create authentication is not configured")
		return
	}

	authContext, ok := middleware.ApprovalAuthFromContext(r.Context())
	if !ok || authContext.AgentID.IsZero() || authContext.GatewayClientID == "" {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "approval create authentication is not configured")
		return
	}

	result, err := h.service.CreatePendingApproval(r.Context(), domainapproval.CreateApprovalRequest{
		Principal:                id.Principal(actingPrincipal),
		AgentID:                  authContext.AgentID,
		GatewayClientID:          authContext.GatewayClientID,
		ToolName:                 req.ToolName,
		Arguments:                req.Arguments,
		Description:              *req.Metadata.Description,
		RiskLevel:                req.RiskLevel,
		MCPSessionID:             req.Metadata.MCPSessionID,
		AgentSessionID:           req.Metadata.AgentSessionID,
		ToolInvocationID:         req.Metadata.ToolInvocationID,
		OpenTelemetryTraceparent: traceparentFromRequest(r),
	})
	if err != nil {
		if errors.Is(err, domainapproval.ErrApprovalRateLimit) {
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "rate limit exceeded for this principal/agent pair")
			return
		}
		if errors.Is(err, domainapproval.ErrApprovalInvalidPattern) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_pattern", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to create approval")
		return
	}

	status := http.StatusCreated
	if !result.IsNew {
		status = http.StatusOK
	}

	resp := createResponse{
		Data: createResponseData{
			ID:          result.Approval.ID.String(),
			Status:      string(result.Approval.Status),
			ApprovalURL: result.Approval.ApprovalURL,
			CreatedAt:   result.Approval.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
