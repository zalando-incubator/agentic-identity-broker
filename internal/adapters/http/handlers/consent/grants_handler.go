// Package consent provides HTTP handlers for consent management APIs.
package consent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
)

// GrantsHandler handles HTTP requests for user grants management.
// Implements FR-011 through FR-014 (grant CRUD operations).
type GrantsHandler struct {
	consentService        ConsentService
	sessionTokenValidator ports.SessionTokenValidator
	logger                *slog.Logger
}

// NewGrantsHandler creates a new grants handler.
func NewGrantsHandler(consentService ConsentService, logger *slog.Logger, sessionTokenValidator ports.SessionTokenValidator) *GrantsHandler {
	if sessionTokenValidator == nil {
		panic("GrantsHandler requires a non-nil SessionTokenValidator")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &GrantsHandler{
		consentService:        consentService,
		sessionTokenValidator: sessionTokenValidator,
		logger:                logger,
	}
}

// DelegatedTokenRequest represents a delegated token in the request.
type DelegatedTokenRequest struct {
	ThirdpartyOAuth2ServiceID string   `json:"thirdparty_oauth2_service_id"`
	Scopes                    []string `json:"scopes"`
}

// GrantRequest represents the request body for creating/updating a grant.
type GrantRequest struct {
	ValidUntil            *time.Time          `json:"valid_until,omitempty"`
	GrantedPermissionSets map[string][]string `json:"granted_permission_sets"`
}

// GrantResponse represents a user grant in the response.
type GrantResponse struct {
	ID                    string              `json:"id"`
	Principal             string              `json:"principal"`
	AgentID               string              `json:"agent_id"`
	ValidUntil            *time.Time          `json:"valid_until,omitempty"`
	GrantedPermissionSets map[string][]string `json:"granted_permission_sets"`
	CreatedAt             string              `json:"created_at"`
	UpdatedAt             string              `json:"updated_at"`
}

// GetGrant handles GET /api/consent/agent/:agent-id/grants.
func (h *GrantsHandler) GetGrant(w http.ResponseWriter, r *http.Request) {
	principalValue, agentID, ok := h.extractPrincipalAndAgent(w, r)
	if !ok {
		return
	}

	grants, err := h.consentService.GetUserGrants(r.Context(), principalValue, agentID)
	if err != nil {
		if errors.Is(err, consent.ErrAgentNotFound) {
			h.logger.Warn("agent not found", "agent_id", agentID, "principal", principalValue)
			h.writeError(w, http.StatusNotFound, "not found", "agent not found")
			return
		}

		h.logger.Error("failed to get user grants", "agent_id", agentID, "principal", principalValue, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	var response *GrantResponse
	if len(grants) > 0 {
		grantResponse := h.toGrantResponse(grants[0])
		response = &grantResponse
	}

	h.logger.Info("user grant retrieved", "agent_id", agentID, "principal", principalValue, "has_grant", response != nil)
	h.writeJSON(w, http.StatusOK, map[string]*GrantResponse{"data": response})
}

// CreateGrant handles POST /api/consent/agent/:agent-id/grants
// Creates or updates a grant (upsert semantics). For agents that declare
// permission sets, at least one PS must be included (FR-014).
//
// Response codes:
// - 201 Created: Grant created/updated
// - 400 Bad Request: Invalid request body or validation error
// - 401 Unauthorized: No principal in context
// - 404 Not Found: Agent doesn't exist
// - 500 Internal Server Error: Service error
func (h *GrantsHandler) CreateGrant(w http.ResponseWriter, r *http.Request) {
	principalValue, parsedAgentID, ok := h.extractPrincipalAndAgent(w, r)
	if !ok {
		return
	}
	agentID := parsedAgentID.String()

	// Parse request
	var req GrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid request body",
			"error", err,
			"principal", principalValue,
			"agent_id", agentID)
		h.writeError(w, http.StatusBadRequest, "invalid request", "request body must be valid JSON")
		return
	}

	// FR-029: Validate JWE session_token when present.
	sessionToken := r.URL.Query().Get("session_token")
	var sessionRedirectURI string
	if sessionToken != "" {
		claims, err := h.sessionTokenValidator.ValidateAuthorizationSessionToken(sessionToken, parsedAgentID, id.Principal(principalValue))
		if err != nil {
			switch {
			case errors.Is(err, ports.ErrSessionExpired):
				h.logger.Warn("authorization session token expired", "agent_id", agentID, "principal", principalValue)
				h.writeError(w, http.StatusBadRequest, "session_expired", "authorization session has expired, please restart the authorization flow")
			case errors.Is(err, ports.ErrSessionAgentMismatch):
				h.logger.Warn("authorization session agent mismatch",
					"expected_agent", parsedAgentID,
					"principal", principalValue)
				h.writeError(w, http.StatusBadRequest, "bad request", "authorization session does not match requested agent")
			case errors.Is(err, ports.ErrSessionPrincipalMismatch):
				h.logger.Warn("authorization session principal mismatch", "principal", principalValue)
				h.writeError(w, http.StatusForbidden, "forbidden", "authorization session does not belong to this user")
			case errors.Is(err, ports.ErrSessionInvalidToken):
				h.logger.Warn("authorization session token invalid", "agent_id", agentID, "principal", principalValue)
				h.writeError(w, http.StatusBadRequest, "invalid_token", "authorization session token is invalid")
			default:
				h.logger.Error("unexpected error validating authorization session token", "agent_id", agentID, "principal", principalValue, "error", err)
				h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			}
			return
		}
		sessionRedirectURI = claims.OriginalURL
		if sessionRedirectURI == "" {
			h.logger.Error("authorization session token contains empty original_url",
				"agent_id", agentID, "principal", principalValue)
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			return
		}
	}

	// Parse permission set entries from map — sort for deterministic order
	entries := make([]storage.GrantedPermissionSetEntry, 0, len(req.GrantedPermissionSets))
	for psStr, svcStrs := range req.GrantedPermissionSets {
		psID, err := id.ParsePermissionSetID(psStr)
		if err != nil {
			h.logger.Warn("invalid permission set ID format",
				"permission_set_id", psStr,
				"principal", principalValue,
				"agent_id", agentID)
			h.writeError(w, http.StatusBadRequest, "invalid request", fmt.Sprintf("permission set ID %q must be a valid UUID", psStr))
			return
		}
		svcIDs := make([]id.ServiceID, len(svcStrs))
		for i, svcStr := range svcStrs {
			svcID, err := id.ParseServiceID(svcStr)
			if err != nil {
				h.logger.Warn("invalid service ID format",
					"service_id", svcStr,
					"principal", principalValue,
					"agent_id", agentID)
				h.writeError(w, http.StatusBadRequest, "invalid request", fmt.Sprintf("service ID %q must be a valid UUID", svcStr))
				return
			}
			svcIDs[i] = svcID
		}
		entries = append(entries, storage.GrantedPermissionSetEntry{
			PermissionSetID:    psID,
			IncludedServiceIDs: svcIDs,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].PermissionSetID.String() < entries[j].PermissionSetID.String()
	})

	// Create grant request
	grantReq := &consent.GrantRequest{
		Principal:             principalValue,
		AgentID:               parsedAgentID,
		ValidUntil:            req.ValidUntil,
		GrantedPermissionSets: entries,
	}

	handleGrantError := func(err error) {
		// Check for specific error types
		if errors.Is(err, consent.ErrAgentNotFound) {
			h.logger.Warn("agent not found",
				"agent_id", agentID,
				"principal", principalValue)
			h.writeError(w, http.StatusNotFound, "agent not found", "")
			return
		}

		if errors.Is(err, consent.ErrInvalidScopes) {
			h.logger.Warn("invalid scopes requested",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "invalid scopes", err.Error())
			return
		}

		if errors.Is(err, consent.ErrServiceNotFound) {
			h.logger.Warn("service not found",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "service not found", err.Error())
			return
		}

		if errors.Is(err, consent.ErrMissingMandatoryPS) {
			h.logger.Warn("missing mandatory permission set",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "missing mandatory permission set", err.Error())
			return
		}

		if errors.Is(err, consent.ErrInvalidServiceInclusion) {
			h.logger.Warn("invalid service inclusion in grant",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "invalid service inclusion", err.Error())
			return
		}

		if errors.Is(err, consent.ErrUnconnectedServices) {
			h.logger.WarnContext(r.Context(), "grant submission includes services without active sessions (FR-020)",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "unconnected services", err.Error())
			return
		}

		if errors.Is(err, consent.ErrGrantValidation) {
			h.logger.WarnContext(r.Context(), "grant validation failed",
				"agent_id", agentID,
				"principal", principalValue,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "invalid request", err.Error())
			return
		}

		h.logger.ErrorContext(r.Context(), "failed to grant consent",
			"agent_id", agentID,
			"principal", principalValue,
			"error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
	}

	grant, err := h.consentService.GrantConsent(r.Context(), grantReq)
	if err != nil {
		handleGrantError(err)
		return
	}

	if grant == nil {
		h.logger.ErrorContext(r.Context(), "grant consent returned nil grant without error", "agent_id", agentID)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	// Audit logging
	h.logger.InfoContext(r.Context(), "grant created",
		"principal", principalValue,
		"agent_id", agentID,
		"grant_id", grant.ID)

	resp := map[string]interface{}{"data": h.toGrantResponse(grant)}
	if sessionRedirectURI != "" {
		resp["redirect_url"] = sessionRedirectURI
	}
	h.writeJSON(w, http.StatusCreated, resp)
}

// RevokeGrant handles DELETE /api/consent/agent/{agent-id}/grants.
func (h *GrantsHandler) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	principalValue, agentID, ok := h.extractPrincipalAndAgent(w, r)
	if !ok {
		return
	}

	if err := h.consentService.RevokeConsentForPrincipal(r.Context(), principalValue, agentID); err != nil {
		if errors.Is(err, consent.ErrGrantNotFound) {
			h.writeError(w, http.StatusNotFound, "not found", "no active grant exists for this agent")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "internal server error", "an unexpected error occurred")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *GrantsHandler) extractPrincipalAndAgent(w http.ResponseWriter, r *http.Request) (id.Principal, id.AgentID, bool) {
	principalValue, ok := principal.FromContext(r.Context())
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return "", id.AgentID{}, false
	}

	rawAgentID := chi.URLParam(r, "agent-id")
	agentID, err := id.ParseAgentID(rawAgentID)
	if err != nil {
		h.logger.Warn("invalid agent ID format", "agent_id", rawAgentID)
		h.writeError(w, http.StatusBadRequest, "bad request", "agent ID must be a valid UUID")
		return "", id.AgentID{}, false
	}

	return id.Principal(principalValue), agentID, true
}

// toGrantResponse converts a UserGrant to GrantResponse.
func (h *GrantsHandler) toGrantResponse(grant *storage.UserGrant) GrantResponse {
	psMap := make(map[string][]string, len(grant.GrantedPermissionSets))
	for _, entry := range grant.GrantedPermissionSets {
		svcIDs := make([]string, len(entry.IncludedServiceIDs))
		for i, svcID := range entry.IncludedServiceIDs {
			svcIDs[i] = svcID.String()
		}
		psMap[entry.PermissionSetID.String()] = svcIDs
	}

	return GrantResponse{
		ID:                    grant.ID.String(),
		Principal:             grant.Principal.String(),
		AgentID:               grant.AgentID.String(),
		ValidUntil:            grant.ValidUntil,
		GrantedPermissionSets: psMap,
		CreatedAt:             grant.CreatedAt.Format(time.RFC3339),
		UpdatedAt:             grant.UpdatedAt.Format(time.RFC3339),
	}
}

// writeJSON writes a JSON response.
func (h *GrantsHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	if err := writeBufferedJSON(w, statusCode, data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *GrantsHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
