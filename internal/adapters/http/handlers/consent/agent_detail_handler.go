// Package consent provides HTTP handlers for consent management APIs.
package consent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
)

// ServiceRequirementForUser represents a service requirement enriched with user session status.
// This is used in the consent screen to display which services the agent requires and user's connection status.
type ServiceRequirementForUser struct {
	ServiceID        string                 `json:"serviceId"`
	ServiceName      string                 `json:"serviceName"`
	RequirementType  string                 `json:"requirementType"` // "mandatory" or "optional"
	RequiredScopes   []ScopeWithDescription `json:"requiredScopes"`
	ConnectionStatus string                 `json:"connectionStatus"` // "connected" or "not_connected"
}

// ScopeWithDescription represents an OAuth2 scope with its description.
type ScopeWithDescription struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// errSessionExpired is returned by resolveSessionContext when the JWE session token
// TTL has elapsed. Callers return "session_expired" so the frontend can restart the
// /oauth2/authorize flow.
var errSessionExpired = errors.New("authorization session expired")

// errInvalidToken is returned by resolveSessionContext when the JWE session token
// cannot be decrypted or unmarshalled (tampered, wrong key, truncated). Distinct from
// errSessionExpired to allow callers to log at appropriate severity.
var errInvalidToken = errors.New("authorization session token invalid")

// errInternalSession is returned by resolveSessionContext when session validation fails
// due to a server-side misconfiguration or invariant violation (not a bad client token).
var errInternalSession = errors.New("internal session validation error")

// AgentDetailHandler handles HTTP requests for retrieving detailed agent information.
// Implements User Story 2: Review Agent-Specific Grants (GET /api/consent/agent/:agentId).
// Phase 6 extension: Includes service requirements with user connection status.
type AgentDetailHandler struct {
	consentService        ConsentService
	sessionTokenValidator ports.SessionTokenValidator
	logger                *slog.Logger
}

// NewAgentDetailHandler creates a new agent detail handler.
func NewAgentDetailHandler(consentService ConsentService, logger *slog.Logger, sessionTokenValidator ports.SessionTokenValidator) *AgentDetailHandler {
	if sessionTokenValidator == nil {
		panic("AgentDetailHandler requires a non-nil SessionTokenValidator")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentDetailHandler{
		consentService:        consentService,
		sessionTokenValidator: sessionTokenValidator,
		logger:                logger,
	}
}

// CIMDMetadataResponse is included in the agent detail response when the authorization
// request originated from a CIMD-based client_id.
type CIMDMetadataResponse struct {
	ClientIDURL     string   `json:"client_id_url"`
	RedirectURI     string   `json:"redirect_uri"`
	VerifiedDomain  string   `json:"verified_domain"`
	RequestedScopes []string `json:"requested_scopes"`
	LogoURI         string   `json:"logo_uri,omitempty"`
}

// GetAgentDetailResponse represents the response for GET /api/consent/agents/{id}.
type GetAgentDetailResponse struct {
	Data AgentDetailData `json:"data"`
}

// AgentDetailData contains the unified agent detail response: display fields,
// services with connection status, permission sets, and active session IDs.
type AgentDetailData struct {
	Agent               AgentMetadata                  `json:"agent"`
	Services            []ServiceRequirementForUser    `json:"services"`
	PermissionSets      []PermissionSetWithRequirement `json:"permission_sets"`
	ActiveSessionIDs    []string                       `json:"active_session_service_ids"`
	ServiceRequirements []ServiceRequirementInfo       `json:"service_requirements"`
	CIMDMetadata        *CIMDMetadataResponse          `json:"cimd_metadata,omitempty"`
}

// AgentMetadata is the agent metadata portion of the unified response.
type AgentMetadata struct {
	ID                   string   `json:"agentId"`
	ClientID             string   `json:"client_id,omitempty"`
	ClientURIs           []string `json:"client_uris,omitempty"`
	DisplayName          string   `json:"display_name"`
	Description          string   `json:"description"`
	GovernanceURL        *string  `json:"governance_url,omitempty"`
	UserDocumentationURL *string  `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string  `json:"agent_interface_url,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

// ServiceScopeInfo represents one service within a permission set.
type ServiceScopeInfo struct {
	ServiceID       string `json:"service_id"`
	RequirementType string `json:"requirement_type"`
}

// PermissionSetWithRequirement represents a permission set with its requirement type.
type PermissionSetWithRequirement struct {
	PermissionSet struct {
		ID            string             `json:"id"`
		Name          string             `json:"name"`
		Description   string             `json:"description"`
		ServiceScopes []ServiceScopeInfo `json:"service_scopes"`
	} `json:"permission_set"`
	RequirementType string `json:"requirement_type"`
}

// ServiceRequirementInfo represents a service requirement entry for the consent screen.
type ServiceRequirementInfo struct {
	ServiceID       string `json:"service_id"`
	RequirementType string `json:"requirement_type"`
}

// GetAgentDetail handles GET /api/consent/agents/{id}
// Returns detailed information about an agent and its required services with user session status.
// Only returns services that are configured as requirements for the agent (not all system services).
//
// Response codes:
// - 200 OK: Returns agent details and services with connection status
// - 400 Bad Request: Invalid agent ID format
// - 401 Unauthorized: Principal not found in context
// - 404 Not Found: Agent doesn't exist
// - 500 Internal Server Error: Service error
func (h *AgentDetailHandler) GetAgentDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID := chi.URLParam(r, "agent-id")

	if agentID == "" {
		h.logger.Warn("agent ID is missing in request")
		h.writeError(w, http.StatusBadRequest, "bad request", "agent ID is required")
		return
	}

	userID, ok := getPrincipalFromContext(ctx)
	if !ok {
		h.logger.Warn("principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "principal required")
		return
	}

	parsedAgentID, parseErr := id.ParseAgentID(agentID)
	if parseErr != nil {
		h.logger.Warn("invalid agent ID format", "agent_id", agentID, "error", parseErr)
		h.writeError(w, http.StatusBadRequest, "bad request", "invalid agent ID format")
		return
	}

	userPrincipal := id.Principal(userID)

	detail, err := h.consentService.GetAgentConsentDetail(ctx, parsedAgentID, userPrincipal)
	if err != nil {
		if errors.Is(err, consent.ErrAgentNotFound) {
			h.logger.Warn("agent not found", "agent_id", agentID)
			h.writeError(w, http.StatusNotFound, "not found", "agent not found")
			return
		}
		if errors.Is(err, consent.ErrMissingMandatoryPS) {
			h.logger.Warn("agent permission-set configuration is missing", "agent_id", agentID, "error", err)
			h.writeError(w, http.StatusBadRequest, "missing mandatory permission set", err.Error())
			return
		}
		h.logger.Error("failed to get agent consent detail", "agent_id", agentID, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	services := toServiceRequirementForUser(detail.ServiceRequirements)
	sortServiceRequirements(services)

	cimdMeta, err := h.resolveSessionContext(r, parsedAgentID)
	if err != nil {
		if errors.Is(err, errSessionExpired) {
			h.logger.Warn("authorization session expired", "agent_id", agentID)
			h.writeError(w, http.StatusBadRequest, "session_expired", "authorization session has expired, please restart the authorization flow")
			return
		}
		if errors.Is(err, errInvalidToken) {
			h.logger.Error("authorization session token invalid", "agent_id", agentID)
			h.writeError(w, http.StatusBadRequest, "invalid_token", "authorization session token is invalid")
			return
		}
		if errors.Is(err, errInternalSession) {
			h.logger.Error("internal session validation error", "agent_id", agentID, "error", err)
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			return
		}
		h.logger.Warn("authorization session error", "agent_id", agentID, "error", err)
		h.writeError(w, http.StatusBadRequest, "bad request", "invalid authorization session")
		return
	}

	response := GetAgentDetailResponse{
		Data: h.buildResponse(detail, services, cimdMeta),
	}

	h.logger.Info("agent detail retrieved",
		"agent_id", agentID,
		"user_id", userID,
		"services_count", len(services))

	h.writeJSON(w, http.StatusOK, response)
}

func (h *AgentDetailHandler) buildResponse(detail *consent.AgentConsentDetail, services []ServiceRequirementForUser, cimdMeta *CIMDMetadataResponse) AgentDetailData {
	agent := detail.Agent
	agentResp := AgentMetadata{
		ID:                   agent.ID.String(),
		DisplayName:          agent.DisplayName,
		Description:          agent.Description,
		GovernanceURL:        agent.GovernanceURL,
		UserDocumentationURL: agent.UserDocumentationURL,
		AgentInterfaceURL:    agent.AgentInterfaceURL,
		CreatedAt:            agent.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:            agent.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if agent.ClientID != nil {
		agentResp.ClientID = agent.ClientID.String()
	}
	if len(agent.ClientURIs) > 0 {
		agentResp.ClientURIs = agent.ClientURIs
	}

	permissionSets := make([]PermissionSetWithRequirement, 0, len(detail.ResolvedPermissionSets))
	for _, entry := range detail.ResolvedPermissionSets {
		if entry.PermissionSet == nil {
			h.logger.Warn("resolved permission set entry has nil PermissionSet, omitting")
			continue
		}
		serviceScopes := make([]ServiceScopeInfo, 0, len(entry.PermissionSet.ServiceScopes))
		for _, ss := range entry.PermissionSet.ServiceScopes {
			serviceScopes = append(serviceScopes, ServiceScopeInfo{
				ServiceID:       ss.ServiceID.String(),
				RequirementType: string(ss.RequirementType),
			})
		}
		item := PermissionSetWithRequirement{
			RequirementType: string(entry.RequirementType),
		}
		item.PermissionSet.ID = entry.PermissionSet.ID.String()
		item.PermissionSet.Name = entry.PermissionSet.Name
		item.PermissionSet.Description = entry.PermissionSet.Description
		item.PermissionSet.ServiceScopes = serviceScopes
		permissionSets = append(permissionSets, item)
	}

	activeSessionIDs := make([]string, len(detail.ActiveSessionServiceIDs))
	for i, sid := range detail.ActiveSessionServiceIDs {
		activeSessionIDs[i] = sid.String()
	}

	serviceReqs := make([]ServiceRequirementInfo, len(agent.ServiceRequirements))
	for i, sr := range agent.ServiceRequirements {
		serviceReqs[i] = ServiceRequirementInfo{
			ServiceID:       sr.ServiceID.String(),
			RequirementType: string(sr.RequirementType),
		}
	}
	if len(serviceReqs) == 0 {
		seen := make(map[string]struct{})
		for _, entry := range detail.ResolvedPermissionSets {
			if entry.PermissionSet == nil {
				continue
			}
			for _, ss := range entry.PermissionSet.ServiceScopes {
				key := ss.ServiceID.String()
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					serviceReqs = append(serviceReqs, ServiceRequirementInfo{
						ServiceID:       key,
						RequirementType: string(entry.RequirementType),
					})
				}
			}
		}
	}

	return AgentDetailData{
		Agent:               agentResp,
		Services:            services,
		PermissionSets:      permissionSets,
		ActiveSessionIDs:    activeSessionIDs,
		ServiceRequirements: serviceReqs,
		CIMDMetadata:        cimdMeta,
	}
}

func toServiceRequirementForUser(reqs []consent.ServiceRequirementStatus) []ServiceRequirementForUser {
	result := make([]ServiceRequirementForUser, len(reqs))
	for i, req := range reqs {
		scopes := make([]ScopeWithDescription, len(req.RequiredScopes))
		for j, s := range req.RequiredScopes {
			scopes[j] = ScopeWithDescription{Name: s.Name, Description: s.Description}
		}
		connStatus := "not_connected"
		if req.IsConnected {
			connStatus = "connected"
		}
		result[i] = ServiceRequirementForUser{
			ServiceID:        req.ServiceID.String(),
			ServiceName:      req.DisplayName,
			RequirementType:  string(req.RequirementType),
			RequiredScopes:   scopes,
			ConnectionStatus: connStatus,
		}
	}
	return result
}

// writeJSON writes a JSON response.
func (h *AgentDetailHandler) writeJSON(w http.ResponseWriter, statusCode int, data any) {
	if err := writeBufferedJSON(w, statusCode, data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *AgentDetailHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}

// getPrincipalFromContext extracts the principal from the request context.
func getPrincipalFromContext(ctx context.Context) (string, bool) {
	return principal.FromContext(ctx)
}

// sortServiceRequirements sorts service requirements with mandatory services first, then optional.
func sortServiceRequirements(services []ServiceRequirementForUser) {
	for i := range len(services) {
		for j := i + 1; j < len(services); j++ {
			if services[i].RequirementType == "optional" && services[j].RequirementType == "mandatory" {
				services[i], services[j] = services[j], services[i]
			}
		}
	}
}

// resolveSessionContext decodes the session_token (when present) and returns CIMD display
// metadata extracted from the claims. The session_token itself is passed opaquely to the
// grants endpoint; authorization-resumption state (original_url, PKCE, state) stays server-side.
// CIMD agents require session_token — falling back to query params would reopen the
// metadata-spoofing surface. Non-CIMD/opaque flows do not use session tokens.
func (h *AgentDetailHandler) resolveSessionContext(r *http.Request, agentID id.AgentID) (*CIMDMetadataResponse, error) {
	sessionToken := r.URL.Query().Get("session_token")
	if sessionToken == "" {
		return nil, nil
	}

	userID, ok := getPrincipalFromContext(r.Context())
	if !ok {
		return nil, fmt.Errorf("%w: principal not found in context", errInternalSession)
	}
	claims, err := h.sessionTokenValidator.ValidateAuthorizationSessionToken(sessionToken, agentID, id.Principal(userID))
	if err != nil {
		switch {
		case errors.Is(err, ports.ErrSessionExpired):
			return nil, errSessionExpired
		case errors.Is(err, ports.ErrSessionAgentMismatch):
			return nil, errors.New("authorization session does not match requested agent")
		case errors.Is(err, ports.ErrSessionPrincipalMismatch):
			return nil, errors.New("authorization session does not belong to this user")
		case errors.Is(err, ports.ErrSessionInvalidToken):
			return nil, errInvalidToken
		default:
			return nil, fmt.Errorf("%w: unexpected error: %v", errInternalSession, err)
		}
	}

	if claims.CIMDMetadata == nil {
		return nil, nil
	}

	u, err := url.Parse(claims.CIMDMetadata.ClientID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid client_id in sealed session token", errInternalSession)
	}

	orig, err := url.Parse(claims.OriginalURL)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid original_url in sealed session token", errInternalSession)
	}
	q := orig.Query()

	requestedScopes := strings.Fields(q.Get("scope"))
	if len(requestedScopes) == 0 {
		requestedScopes = []string{}
	}

	return &CIMDMetadataResponse{
		ClientIDURL:     claims.CIMDMetadata.ClientID,
		RedirectURI:     q.Get("redirect_uri"),
		VerifiedDomain:  u.Hostname(),
		RequestedScopes: requestedScopes,
		LogoURI:         claims.CIMDMetadata.LogoURI,
	}, nil
}
