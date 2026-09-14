// Package admin provides HTTP handlers for administrative APIs.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/go-chi/chi/v5"
)

// PermissionSetValidator is an interface for validating permission set IDs
// and resolving permission sets for coverage invariant checks.
// Implemented by *permissionset.Service.
type PermissionSetValidator interface {
	ValidateIDs(ctx context.Context, ids []id.PermissionSetID) error
	GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)
	ResolveID(ctx context.Context, value string) (id.PermissionSetID, error)
	CanonicalIDs(ctx context.Context, ids []id.PermissionSetID) (map[id.PermissionSetID]string, error)
}

// AgentsHandler handles HTTP requests for agent CRUD operations.
type AgentsHandler struct {
	agentService    *agents.Service
	providerService *thirdparty.ThirdpartyOAuth2ProviderService
	psService       PermissionSetValidator
	logger          *slog.Logger
}

// NewAgentsHandler creates a new agents handler.
func NewAgentsHandler(agentService *agents.Service, providerService *thirdparty.ThirdpartyOAuth2ProviderService, psService PermissionSetValidator, logger *slog.Logger) *AgentsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentsHandler{
		agentService:    agentService,
		providerService: providerService,
		psService:       psService,
		logger:          logger,
	}
}

// ServiceRequirementRequest represents a service requirement in the request.
type ServiceRequirementRequest struct {
	ServiceID        string   `json:"service_id"`
	RequirementType  string   `json:"requirement_type"`
	RequiredScopes   []string `json:"required_scopes"`
	RequireAllScopes bool     `json:"require_all_scopes,omitempty"`
}

// PermissionSetRequest represents a permission set declaration in the request.
type PermissionSetRequest struct {
	PermissionSetID string `json:"permission_set_id"`
	RequirementType string `json:"requirement_type"`
}

// AgentRequest represents the request body for creating/updating an agent.
type AgentRequest struct {
	CanonicalID          *string                     `json:"canonical_id,omitempty"`
	ClientID             *string                     `json:"client_id,omitempty"`
	ExternalID           *string                     `json:"external_id,omitempty"`
	DisplayName          string                      `json:"display_name"`
	Description          string                      `json:"description"`
	GovernanceURL        *string                     `json:"governance_url,omitempty"`
	UserDocumentationURL *string                     `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string                     `json:"agent_interface_url,omitempty"`
	ServiceRequirements  []ServiceRequirementRequest `json:"service_requirements,omitempty"`
	PermissionSets       []PermissionSetRequest      `json:"permission_sets,omitempty"`
	RedirectURIs         []string                    `json:"redirect_uris,omitempty"`
	AllowedScopes        []string                    `json:"allowed_scopes,omitempty"`
	ClientURIs           []string                    `json:"client_uris,omitempty"`
}

// ServiceRequirementResponse represents a service requirement in the response.
type ServiceRequirementResponse struct {
	ServiceID        string   `json:"service_id"`
	ServiceName      string   `json:"service_name,omitempty"`
	RequirementType  string   `json:"requirement_type"`
	RequiredScopes   []string `json:"required_scopes"`
	RequireAllScopes bool     `json:"require_all_scopes,omitempty"`
}

// PermissionSetDeclarationResponse represents a permission set declaration in the response.
type PermissionSetDeclarationResponse struct {
	PermissionSetID string `json:"permission_set_id"`
	RequirementType string `json:"requirement_type"`
}

// AgentResponse represents the response body for agent operations.
type AgentResponse struct {
	ID                   string                             `json:"id"`
	CanonicalID          *string                            `json:"canonical_id"`
	ClientID             *string                            `json:"client_id,omitempty"`
	ExternalID           *string                            `json:"external_id,omitempty"`
	DisplayName          string                             `json:"display_name"`
	Description          string                             `json:"description"`
	GovernanceURL        *string                            `json:"governance_url,omitempty"`
	UserDocumentationURL *string                            `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string                            `json:"agent_interface_url,omitempty"`
	ServiceRequirements  []ServiceRequirementResponse       `json:"service_requirements,omitempty"`
	PermissionSets       []PermissionSetDeclarationResponse `json:"permission_sets,omitempty"`
	RedirectURIs         []string                           `json:"redirect_uris,omitempty"`
	AllowedScopes        []string                           `json:"allowed_scopes,omitempty"`
	ClientURIs           []string                           `json:"client_uris,omitempty"`
	CreatedAt            string                             `json:"created_at"`
	UpdatedAt            string                             `json:"updated_at"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// CreateAgent handles POST /api/agents
func (h *AgentsHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req AgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Convert and validate permission sets (FR-006: fail fast before any repository calls)
	permissionSets, err := h.convertPermissionSetRequests(ctx, req.PermissionSets)
	if err != nil {
		h.logger.Warn("permission set validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	serviceReqs, err := h.convertServiceRequirements(ctx, req.ServiceRequirements)
	if err != nil {
		h.logger.Warn("invalid service requirements", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid service requirements", err.Error())
		return
	}

	// FR-019: Validate that every SR service_id is covered by at least one PS ServiceScope
	if err := h.validateServiceRequirementsCoverage(ctx, serviceReqs, permissionSets); err != nil {
		h.logger.Warn("FR-019 coverage invariant violated", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if err := storage.ValidateClientURIsForWrite(req.ClientURIs); err != nil {
		h.logger.Warn("client_uris validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	now := time.Now().UTC()
	agent := &storage.Agent{
		CanonicalID:          req.CanonicalID,
		ClientID:             clientIDFromRequest(req.ClientID),
		ExternalID:           convertExternalID(req.ExternalID),
		DisplayName:          req.DisplayName,
		Description:          req.Description,
		GovernanceURL:        req.GovernanceURL,
		UserDocumentationURL: req.UserDocumentationURL,
		AgentInterfaceURL:    req.AgentInterfaceURL,
		ServiceRequirements:  serviceReqs,
		PermissionSets:       permissionSets,
		RedirectURIs:         req.RedirectURIs,
		AllowedScopes:        req.AllowedScopes,
		ClientURIs:           req.ClientURIs,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if err := h.agentService.Create(ctx, agent); err != nil {
		h.handleDomainError(w, r, "CreateAgent", err)
		return
	}

	serviceMap := h.batchLoadServices(ctx, []*storage.Agent{agent})
	resp, err := h.toResponseWithServiceMap(agent, serviceMap)
	if err != nil {
		h.logger.Error("failed to convert agent to response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	h.writeJSON(w, http.StatusCreated, resp)
}

// GetAgent handles GET /api/agents/:agent-id
func (h *AgentsHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()
	agentID := chi.URLParam(r, "agent-id")

	if agentID == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	parsedAgentID, err := h.agentService.ResolveID(ctx, agentID)
	if err != nil {
		h.handleDomainError(w, r, "GetAgent", err)
		return
	}

	agent, err := h.agentService.Get(ctx, parsedAgentID)
	if err != nil {
		h.handleDomainError(w, r, "GetAgent", err)
		return
	}

	serviceMap := h.batchLoadServices(ctx, []*storage.Agent{agent})
	resp, err := h.toResponseWithServiceMap(agent, serviceMap)
	if err != nil {
		h.logger.Error("failed to convert agent to response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	if requestsCanonicalReferences(r.Header.Get("Prefer")) {
		if err := h.applyCanonicalReferences(ctx, []*storage.Agent{agent}, []*AgentResponse{&resp}); err != nil {
			h.handleDomainError(w, r, "GetAgent", err)
			return
		}
		w.Header().Set("Preference-Applied", "reference-id=canonical")
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// UpdateAgent handles PUT /api/agents/:agent-id
func (h *AgentsHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID := chi.URLParam(r, "agent-id")

	if agentID == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	parsedAgentID, err := h.agentService.ResolveID(ctx, agentID)
	if err != nil {
		h.handleDomainError(w, r, "UpdateAgent", err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	var req AgentRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Detect three-state client_id semantics: absent=preserve, null=clear, string=update.
	var rawFields map[string]json.RawMessage
	_ = json.Unmarshal(body, &rawFields) // cannot fail: same bytes already parsed above
	rawClientID, clientIDPresent := rawFields["client_id"]
	clearClientID := clientIDPresent && bytes.Equal(rawClientID, []byte("null"))
	_, canonicalIDPresent := rawFields["canonical_id"]

	serviceReqs, err := h.convertServiceRequirements(ctx, req.ServiceRequirements)
	if err != nil {
		h.logger.Warn("invalid service requirements", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid service requirements", err.Error())
		return
	}

	// Convert and validate permission sets
	permissionSets, err := h.convertPermissionSetRequests(ctx, req.PermissionSets)
	if err != nil {
		h.logger.Warn("permission set validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// FR-019: Validate that every SR service_id is covered by at least one PS ServiceScope
	if err := h.validateServiceRequirementsCoverage(ctx, serviceReqs, permissionSets); err != nil {
		h.logger.Warn("FR-019 coverage invariant violated", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// Enforce client URI cardinality on admin writes. Validate() (called by the storage
	// adapter on Update) skips cardinality to allow CIMD snapshot refreshes — so this
	// explicit check is required for admin mutations.
	if err := storage.ValidateClientURIsForWrite(req.ClientURIs); err != nil {
		h.logger.Warn("client_uris validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	agent := &storage.Agent{
		CanonicalID:          req.CanonicalID,
		ClearCanonicalID:     canonicalIDPresent && bytes.Equal(rawFields["canonical_id"], []byte("null")),
		ClientID:             clientIDFromRequest(req.ClientID),
		ExternalID:           convertExternalID(req.ExternalID),
		DisplayName:          req.DisplayName,
		Description:          req.Description,
		GovernanceURL:        req.GovernanceURL,
		UserDocumentationURL: req.UserDocumentationURL,
		AgentInterfaceURL:    req.AgentInterfaceURL,
		ServiceRequirements:  serviceReqs,
		PermissionSets:       permissionSets,
		RedirectURIs:         req.RedirectURIs,
		AllowedScopes:        req.AllowedScopes,
		ClientURIs:           req.ClientURIs,
		UpdatedAt:            time.Now().UTC(),
	}

	if !canonicalIDPresent {
		existing, err := h.agentService.Get(ctx, parsedAgentID)
		if err != nil {
			h.handleDomainError(w, r, "GetAgent", err)
			return
		}
		agent.CanonicalID = existing.CanonicalID
	}

	if err := h.agentService.Update(ctx, parsedAgentID, agent, clearClientID); err != nil {
		h.handleDomainError(w, r, "UpdateAgent", err)
		return
	}

	serviceMap := h.batchLoadServices(ctx, []*storage.Agent{agent})
	resp, err := h.toResponseWithServiceMap(agent, serviceMap)
	if err != nil {
		h.logger.Error("failed to convert agent to response", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// DeleteAgent handles DELETE /api/agents/:agent-id
func (h *AgentsHandler) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agentID := chi.URLParam(r, "agent-id")

	if agentID == "" {
		h.writeError(w, http.StatusBadRequest, "agent ID is required", "")
		return
	}

	parsedID, err := h.agentService.ResolveID(ctx, agentID)
	if err != nil {
		h.handleDomainError(w, r, "DeleteAgent", err)
		return
	}

	if err := h.agentService.Delete(ctx, parsedID); err != nil {
		h.handleDomainError(w, r, "DeleteAgent", err)
		return
	}

	h.logger.Info("agent deleted", "agent_id", agentID)
	w.WriteHeader(http.StatusNoContent)
}

// ListAgents handles GET /api/agents
func (h *AgentsHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()

	agents, err := h.agentService.List(ctx)
	if err != nil {
		h.handleDomainError(w, r, "ListAgents", err)
		return
	}

	serviceMap := h.batchLoadServices(ctx, agents)

	responses := make([]AgentResponse, len(agents))
	for i, agent := range agents {
		resp, err := h.toResponseWithServiceMap(agent, serviceMap)
		if err != nil {
			h.logger.Error("failed to convert agent to response", "agent_id", agent.ID, "error", err)
			resp = AgentResponse{
				ID:          agent.ID.String(),
				CanonicalID: agent.CanonicalID,
				ClientID:    clientIDToString(agent.ClientID),
				DisplayName: agent.DisplayName,
				Description: agent.Description,
				CreatedAt:   agent.CreatedAt.Format(time.RFC3339),
				UpdatedAt:   agent.UpdatedAt.Format(time.RFC3339),
			}
		}
		responses[i] = resp
	}
	if requestsCanonicalReferences(r.Header.Get("Prefer")) {
		responsePointers := make([]*AgentResponse, len(responses))
		for i := range responses {
			responsePointers[i] = &responses[i]
		}
		if err := h.applyCanonicalReferences(ctx, agents, responsePointers); err != nil {
			h.handleDomainError(w, r, "ListAgents", err)
			return
		}
		w.Header().Set("Preference-Applied", "reference-id=canonical")
	}
	h.writeJSON(w, http.StatusOK, responses)
}

// batchLoadServices loads all unique services referenced by agents in a single batch.
func (h *AgentsHandler) batchLoadServices(ctx context.Context, agents []*storage.Agent) map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity {
	serviceIDs := make(map[id.ServiceID]bool)
	for _, agent := range agents {
		for _, sr := range agent.ServiceRequirements {
			serviceIDs[sr.ServiceID] = true
		}
	}

	serviceMap := make(map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity)
	for serviceID := range serviceIDs {
		service, err := h.providerService.Get(ctx, serviceID)
		if err != nil {
			h.logger.Warn("failed to load service for batch", "service_id", serviceID, "error", err)
			continue
		}
		serviceMap[serviceID] = service
	}

	return serviceMap
}

// toResponseWithServiceMap converts an Agent entity to AgentResponse using a pre-loaded service map.
func (h *AgentsHandler) toResponseWithServiceMap(agent *storage.Agent, serviceMap map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity) (AgentResponse, error) {
	resp := AgentResponse{
		ID:                   agent.ID.String(),
		CanonicalID:          agent.CanonicalID,
		ClientID:             clientIDToString(agent.ClientID),
		ExternalID:           convertExternalIDToString(agent.ExternalID),
		DisplayName:          agent.DisplayName,
		Description:          agent.Description,
		GovernanceURL:        agent.GovernanceURL,
		UserDocumentationURL: agent.UserDocumentationURL,
		AgentInterfaceURL:    agent.AgentInterfaceURL,
		RedirectURIs:         agent.RedirectURIs,
		AllowedScopes:        agent.AllowedScopes,
		ClientURIs:           agent.ClientURIs,
		CreatedAt:            agent.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            agent.UpdatedAt.Format(time.RFC3339),
	}

	if len(agent.ServiceRequirements) > 0 {
		resp.ServiceRequirements = make([]ServiceRequirementResponse, len(agent.ServiceRequirements))
		for i, sr := range agent.ServiceRequirements {
			respSR := ServiceRequirementResponse{
				ServiceID:        sr.ServiceID.String(),
				RequirementType:  sr.RequirementType.String(),
				RequiredScopes:   sr.RequiredScopes,
				RequireAllScopes: sr.RequireAllScopes,
			}
			if service, ok := serviceMap[sr.ServiceID]; ok {
				respSR.ServiceName = service.DisplayName
			}
			resp.ServiceRequirements[i] = respSR
		}
	}

	// Convert permission sets
	if len(agent.PermissionSets) > 0 {
		resp.PermissionSets = make([]PermissionSetDeclarationResponse, len(agent.PermissionSets))
		for i, ps := range agent.PermissionSets {
			resp.PermissionSets[i] = PermissionSetDeclarationResponse{
				PermissionSetID: ps.PermissionSetID.String(),
				RequirementType: ps.RequirementType.String(),
			}
		}
	}

	return resp, nil
}

func (h *AgentsHandler) applyCanonicalReferences(ctx context.Context, agents []*storage.Agent, responses []*AgentResponse) error {
	serviceIDs := make([]id.ServiceID, 0)
	permissionSetIDs := make([]id.PermissionSetID, 0)
	seenServices := make(map[id.ServiceID]struct{})
	seenPermissionSets := make(map[id.PermissionSetID]struct{})
	for _, agent := range agents {
		for _, requirement := range agent.ServiceRequirements {
			if _, seen := seenServices[requirement.ServiceID]; !seen {
				seenServices[requirement.ServiceID] = struct{}{}
				serviceIDs = append(serviceIDs, requirement.ServiceID)
			}
		}
		for _, permissionSet := range agent.PermissionSets {
			if _, seen := seenPermissionSets[permissionSet.PermissionSetID]; !seen {
				seenPermissionSets[permissionSet.PermissionSetID] = struct{}{}
				permissionSetIDs = append(permissionSetIDs, permissionSet.PermissionSetID)
			}
		}
	}
	serviceCanonicalIDs, err := h.providerService.CanonicalIDs(ctx, serviceIDs)
	if err != nil {
		return err
	}
	permissionSetCanonicalIDs := map[id.PermissionSetID]string{}
	if h.psService != nil {
		permissionSetCanonicalIDs, err = h.psService.CanonicalIDs(ctx, permissionSetIDs)
		if err != nil {
			return err
		}
	}
	for i, agent := range agents {
		for j, requirement := range agent.ServiceRequirements {
			if canonicalID, ok := serviceCanonicalIDs[requirement.ServiceID]; ok {
				responses[i].ServiceRequirements[j].ServiceID = canonicalID
			}
		}
		for j, permissionSet := range agent.PermissionSets {
			if canonicalID, ok := permissionSetCanonicalIDs[permissionSet.PermissionSetID]; ok {
				responses[i].PermissionSets[j].PermissionSetID = canonicalID
			}
		}
	}
	return nil
}

// convertServiceRequirements converts request DTOs to domain models.
func (h *AgentsHandler) convertServiceRequirements(ctx context.Context, reqSRs []ServiceRequirementRequest) ([]storage.ServiceRequirement, error) {
	if len(reqSRs) == 0 {
		return nil, nil
	}

	result := make([]storage.ServiceRequirement, len(reqSRs))
	for i, req := range reqSRs {
		parsedSvcID, err := h.providerService.ResolveID(ctx, req.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("service_requirements[%d]: invalid service_id: %w", i, err)
		}

		reqType := storage.RequirementType(req.RequirementType)
		if !reqType.Valid() {
			return nil, fmt.Errorf("service_requirements[%d]: invalid requirement_type %q (must be 'mandatory' or 'optional')", i, req.RequirementType)
		}

		result[i] = storage.ServiceRequirement{
			ServiceID:        parsedSvcID,
			RequirementType:  reqType,
			RequiredScopes:   req.RequiredScopes,
			RequireAllScopes: req.RequireAllScopes,
		}
	}

	return result, nil
}

// convertPermissionSetRequests converts request DTOs to domain entries and validates all IDs exist.
// Both an omitted field (nil) and an explicit empty array are rejected per FR-006.
// Returns an error if any ID is invalid or not found.
func (h *AgentsHandler) convertPermissionSetRequests(ctx context.Context, reqs []PermissionSetRequest) ([]storage.AgentPermissionSetEntry, error) {
	if len(reqs) == 0 {
		return nil, fmt.Errorf("at least one permission set entry is required")
	}
	entries := make([]storage.AgentPermissionSetEntry, len(reqs))
	for i, ps := range reqs {
		var (
			psID id.PermissionSetID
			err  error
		)
		if h.psService != nil {
			psID, err = h.psService.ResolveID(ctx, ps.PermissionSetID)
		} else {
			psID, err = id.ParsePermissionSetID(ps.PermissionSetID)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid permission_set_id at index %d: %s", i, err.Error())
		}
		rt := storage.RequirementType(ps.RequirementType)
		if !rt.Valid() {
			return nil, fmt.Errorf("invalid requirement_type at index %d: %s", i, ps.RequirementType)
		}
		entries[i] = storage.AgentPermissionSetEntry{
			PermissionSetID: psID,
			RequirementType: rt,
		}
	}
	if h.psService != nil && len(entries) > 0 {
		ids := make([]id.PermissionSetID, len(entries))
		for i, e := range entries {
			ids[i] = e.PermissionSetID
		}
		if err := h.psService.ValidateIDs(ctx, ids); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// handleDomainError converts domain/storage errors to HTTP responses.
func (h *AgentsHandler) handleDomainError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		switch storageErr.Kind {
		case storage.ErrorKindNotFound:
			h.writeError(w, http.StatusNotFound, "agent not found", storageErr.Message)
		case storage.ErrorKindConflict:
			h.writeError(w, http.StatusConflict, "conflict", storageErr.Message)
		case storage.ErrorKindValidation:
			h.writeError(w, http.StatusBadRequest, "validation failed", storageErr.Message)
		case storage.ErrorKindTimeout:
			h.writeError(w, http.StatusGatewayTimeout, "operation timed out", storageErr.Message)
		default:
			h.logger.Error("storage operation failed", "operation", operation, "kind", storageErr.Kind, "error", storageErr)
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		}
		return
	}

	// Domain validation errors (fmt.Errorf wrapping validation)
	h.logger.Error("operation failed", "operation", operation, "error", err)
	h.writeError(w, http.StatusBadRequest, "invalid request", err.Error())
}

// writeJSON writes a JSON response.
func (h *AgentsHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
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

// convertExternalID converts *string to *id.ExternalID.
func convertExternalID(s *string) *id.ExternalID {
	if s == nil {
		return nil
	}
	eid := id.ExternalID(*s)
	return &eid
}

// convertExternalIDToString converts *id.ExternalID to *string.
func convertExternalIDToString(eid *id.ExternalID) *string {
	if eid == nil {
		return nil
	}
	s := string(*eid)
	return &s
}

func clientIDFromRequest(s *string) *id.ClientID {
	if s == nil {
		return nil
	}
	c := id.ClientID(*s)
	return &c
}

func clientIDToString(c *id.ClientID) *string {
	if c == nil {
		return nil
	}
	s := c.String()
	return &s
}

// validateServiceRequirementsCoverage enforces FR-019: every service_requirements[].service_id
// must be covered by at least one permission set's ServiceScope.
// Returns a descriptive error listing uncovered service IDs if the invariant is violated.
func (h *AgentsHandler) validateServiceRequirementsCoverage(ctx context.Context, serviceReqs []storage.ServiceRequirement, psEntries []storage.AgentPermissionSetEntry) error {
	if len(serviceReqs) == 0 || h.psService == nil {
		return nil
	}

	// Resolve permission sets to get their ServiceScopes
	psIDs := make([]id.PermissionSetID, len(psEntries))
	for i, e := range psEntries {
		psIDs[i] = e.PermissionSetID
	}

	resolvedSets, err := h.psService.GetByIDs(ctx, psIDs)
	if err != nil {
		return fmt.Errorf("failed to resolve permission sets for coverage check: %w", err)
	}

	// Collect all service IDs covered by any PS ServiceScope
	coveredServiceIDs := make(map[id.ServiceID]bool)
	for _, ps := range resolvedSets {
		for _, ss := range ps.ServiceScopes {
			coveredServiceIDs[ss.ServiceID] = true
		}
	}

	// Check every SR service_id is covered
	var uncoveredIDs []string
	for _, sr := range serviceReqs {
		if !coveredServiceIDs[sr.ServiceID] {
			uncoveredIDs = append(uncoveredIDs, sr.ServiceID.String())
		}
	}

	if len(uncoveredIDs) > 0 {
		return fmt.Errorf("service_requirements reference services not covered by any permission set: %v", uncoveredIDs)
	}

	return nil
}
