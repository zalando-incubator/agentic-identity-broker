// Package consent provides HTTP handlers for consent management APIs.
package consent

import (
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// AgentsHandler handles HTTP requests for retrieving agent delegations.
// Implements User Story 1: View Active Delegations (GET /api/consent/agents).
type AgentsHandler struct {
	consentService ConsentService
	logger         *slog.Logger
}

// NewAgentsHandler creates a new agents handler.
func NewAgentsHandler(consentService ConsentService, logger *slog.Logger) *AgentsHandler {
	if logger == nil {
		panic("consent.NewAgentsHandler: logger must not be nil")
	}
	return &AgentsHandler{
		consentService: consentService,
		logger:         logger,
	}
}

// GetAgentDelegationsResponse represents the response for GET /api/consent/agents.
type GetAgentDelegationsResponse struct {
	Data []consent.AgentDelegation `json:"data"`
}

// GetAgentDelegations handles GET /api/consent/agents
// Returns all agent delegations for the authenticated principal.
//
// Response codes:
// - 200 OK: Returns agent delegations (may be empty array)
// - 401 Unauthorized: No principal in context
// - 500 Internal Server Error: Service error
func (h *AgentsHandler) GetAgentDelegations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from request context
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.WarnContext(ctx, "principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	// Call consent service to get agent delegations
	delegations, err := h.consentService.GetAgentDelegations(ctx, id.Principal(principalValue))
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get agent delegations",
			"principal", principalValue,
			"error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	// Return response (empty array if no delegations)
	h.logger.InfoContext(ctx, "agent delegations retrieved",
		"principal", principalValue,
		"count", len(delegations))

	response := GetAgentDelegationsResponse{
		Data: delegations,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response.
func (h *AgentsHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	if err := writeBufferedJSON(w, statusCode, data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *AgentsHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
