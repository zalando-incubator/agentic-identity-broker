package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ClientCredentialsHandler handles admin API requests for broker client credentials.
type ClientCredentialsHandler struct {
	credentials  ports.ClientCredentialManager
	agentService *agents.Service
	logger       *slog.Logger
}

// NewClientCredentialsHandler creates a new ClientCredentialsHandler.
func NewClientCredentialsHandler(
	credentials ports.ClientCredentialManager,
	agentService *agents.Service,
	logger *slog.Logger,
) *ClientCredentialsHandler {
	return &ClientCredentialsHandler{
		credentials:  credentials,
		agentService: agentService,
		logger:       logger,
	}
}

func (h *ClientCredentialsHandler) resolveAgentID(ctx context.Context, value string) (id.AgentID, error) {
	return h.agentService.ResolveID(ctx, value)
}
func (h *ClientCredentialsHandler) handleAgentResolutionError(w http.ResponseWriter, err error) {
	if ports.IsNotFoundErr(err) {
		h.writeError(w, http.StatusNotFound, "agent not found", "")
		return
	}
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindTimeout {
		h.writeError(w, http.StatusGatewayTimeout, "operation timed out", "")
		return
	}
	h.logger.Error("failed to resolve agent", "error", err)
	h.writeError(w, http.StatusInternalServerError, "internal server error", "")
}

// credentialGenerateResponse is the JSON response for POST (generate/rotate).
// Matches ClientCredentialResponse schema in OpenAPI.
type credentialGenerateResponse struct {
	ClientID              string  `json:"client_id"`
	ClientSecret          string  `json:"client_secret"`
	CreatedAt             string  `json:"created_at"`
	PreviousInvalidatedAt *string `json:"previous_invalidated_at,omitempty"`
}

// credentialMetadataResponse is the JSON response for GET (read-only metadata).
// Matches ClientCredentialMetadata schema in OpenAPI.
type credentialMetadataResponse struct {
	ClientID  string  `json:"client_id"`
	CreatedAt string  `json:"created_at"`
	RotatedAt *string `json:"rotated_at,omitempty"`
}

// Generate creates or rotates broker client credentials for an agent.
// POST /api/agents/{agent-id}/client-credentials
func (h *ClientCredentialsHandler) Generate(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agent-id")
	if agentIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	agentID, err := h.resolveAgentID(r.Context(), agentIDStr)
	if err != nil {
		h.handleAgentResolutionError(w, err)
		return
	}

	result, err := h.credentials.Generate(r.Context(), agentID)
	if errors.Is(err, ports.ErrCredentialAgentNotFound) {
		h.writeError(w, http.StatusNotFound, "agent not found", "")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	credential := result.Credential
	plaintextSecret := result.PlaintextSecret
	isRotation := result.Rotated

	resp := credentialGenerateResponse{
		ClientID:     credential.AgentID.String(),
		ClientSecret: plaintextSecret,
		CreatedAt:    credential.CreatedAt.Format(time.RFC3339),
	}
	if credential.RotatedAt != nil {
		rotatedStr := credential.RotatedAt.Format(time.RFC3339)
		resp.PreviousInvalidatedAt = &rotatedStr
	}

	if isRotation {
		h.logger.Info("CredentialRotated",
			"event", "CredentialRotated",
			"agent_id", agentID,
			"client_id", credential.AgentID,
		)
	} else {
		h.logger.Info("CredentialGenerated",
			"event", "CredentialGenerated",
			"agent_id", agentID,
			"client_id", credential.AgentID,
		)
	}

	statusCode := http.StatusCreated
	if isRotation {
		statusCode = http.StatusOK
	}
	h.writeJSON(w, statusCode, resp)
}

// Get retrieves credential metadata (without secret) for an agent.
// GET /api/agents/{agent-id}/client-credentials
func (h *ClientCredentialsHandler) Get(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agent-id")
	if agentIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	agentID, err := h.resolveAgentID(r.Context(), agentIDStr)
	if err != nil {
		h.handleAgentResolutionError(w, err)
		return
	}

	cred, err := h.credentials.Get(r.Context(), agentID)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			h.writeError(w, http.StatusNotFound, "credentials not found", "")
			return
		}
		h.logger.Error("failed to get credentials", "agent_id", agentID, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	h.logger.Debug("CredentialRetrieved",
		"event", "CredentialRetrieved",
		"agent_id", agentID,
	)

	resp := credentialMetadataResponse{
		ClientID:  cred.AgentID.String(),
		CreatedAt: cred.CreatedAt.Format(time.RFC3339),
	}
	if cred.RotatedAt != nil {
		rotatedStr := cred.RotatedAt.Format(time.RFC3339)
		resp.RotatedAt = &rotatedStr
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// Revoke deletes broker client credentials for an agent.
// DELETE /api/agents/{agent-id}/client-credentials
func (h *ClientCredentialsHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agent-id")
	if agentIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	agentID, err := h.resolveAgentID(r.Context(), agentIDStr)
	if err != nil {
		h.handleAgentResolutionError(w, err)
		return
	}

	err = h.credentials.Revoke(r.Context(), agentID)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			h.writeError(w, http.StatusNotFound, "credentials not found", "")
			return
		}
		h.logger.Error("failed to revoke credentials", "agent_id", agentID, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	h.logger.Info("CredentialRevoked",
		"event", "CredentialRevoked",
		"agent_id", agentID,
	)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ClientCredentialsHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *ClientCredentialsHandler) writeError(w http.ResponseWriter, statusCode int, errMsg string, message string) {
	resp := ErrorResponse{
		Error:   errMsg,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
