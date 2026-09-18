// Package admin provides HTTP handlers for administrative APIs.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

// ProviderScopeValidator resolves provider scope configuration.
type ProviderScopeValidator interface {
	Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

// PermissionSetsHandler handles HTTP requests for permission set CRUD operations.
type PermissionSetsHandler struct {
	svc                    *permissionset.Service
	providerScopeValidator ProviderScopeValidator
	logger                 *slog.Logger
}

// NewPermissionSetsHandler creates a new permission sets handler.
func NewPermissionSetsHandler(svc *permissionset.Service, providerScopeValidator ProviderScopeValidator, logger *slog.Logger) *PermissionSetsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &PermissionSetsHandler{
		svc:                    svc,
		providerScopeValidator: providerScopeValidator,
		logger:                 logger,
	}
}

// ServiceScopeRequest represents a service scope in the request.
type ServiceScopeRequest struct {
	ServiceID       string   `json:"service_id"`
	Scopes          []string `json:"scopes"`
	RequirementType string   `json:"requirement_type,omitempty"`
}

// CreatePermissionSetRequest represents the request body for creating/updating a permission set.
type CreatePermissionSetRequest struct {
	CanonicalID   *string               `json:"canonical_id,omitempty"`
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	ServiceScopes []ServiceScopeRequest `json:"service_scopes"`
}

// ServiceScopeResponse represents a service scope in the response.
type ServiceScopeResponse struct {
	ServiceID       string   `json:"service_id"`
	Scopes          []string `json:"scopes"`
	RequirementType string   `json:"requirement_type"`
}

// PermissionSetResponse represents the response body for permission set operations.
type PermissionSetResponse struct {
	ID            string                 `json:"id"`
	CanonicalID   *string                `json:"canonical_id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	ServiceScopes []ServiceScopeResponse `json:"service_scopes"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
}

// PermissionSetListResponse represents a list of permission sets.
type PermissionSetListResponse struct {
	Items []PermissionSetResponse `json:"items"`
}

// Create handles POST /api/permission-sets
func (h *PermissionSetsHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreatePermissionSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	// Validate request
	if err := h.validateCreateRequest(ctx, &req); err != nil {
		h.logger.Warn("validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// Convert request service scopes to domain model
	serviceScopes, err := h.convertServiceScopes(ctx, req.ServiceScopes)
	if err != nil {
		h.logger.Warn("invalid service scopes", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// Create permission set entity
	now := time.Now().UTC()
	ps := &storage.PermissionSet{
		ID:            id.NewPermissionSetID(),
		CanonicalID:   req.CanonicalID,
		Name:          req.Name,
		Description:   req.Description,
		ServiceScopes: serviceScopes,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Create in service
	if err := h.svc.Create(ctx, ps); err != nil {
		h.handleStorageError(w, r, "Create", err)
		return
	}

	h.logger.Info("permission set created", "permission_set_id", ps.ID, "name", ps.Name)

	resp := h.toResponse(ps)
	w.Header().Set("Location", fmt.Sprintf("/api/permission-sets/%s", ps.ID))
	h.writeJSON(w, http.StatusCreated, resp)
}

// Get handles GET /api/permission-sets/:id
func (h *PermissionSetsHandler) Get(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()
	psIDStr := chi.URLParam(r, "id")

	if psIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "permission set ID is required", "")
		return
	}

	psID, err := h.svc.ResolveID(ctx, psIDStr)
	if err != nil {
		h.handleStorageError(w, r, "Get", err)
		return
	}

	ps, err := h.svc.Get(ctx, psID)
	if err != nil {
		h.handleStorageError(w, r, "Get", err)
		return
	}

	resp := h.toResponse(ps)
	if requestsCanonicalReferences(r.Header.Get("Prefer")) {
		honored, err := h.applyCanonicalServiceScopes(ctx, []*storage.PermissionSet{ps}, []*PermissionSetResponse{&resp})
		if err != nil {
			h.handleStorageError(w, r, "Get", err)
			return
		}
		if honored {
			w.Header().Set("Preference-Applied", "reference-id=canonical")
		}
	}
	h.writeJSON(w, http.StatusOK, resp)
}

// List handles GET /api/permission-sets
func (h *PermissionSetsHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()

	// Parse optional service_id filter
	serviceIDStr := r.URL.Query().Get("service_id")
	var serviceID id.ServiceID

	if serviceIDStr != "" {
		var (
			parsedServiceID id.ServiceID
			err             error
		)
		if resolver, ok := h.providerScopeValidator.(interface {
			ResolveID(context.Context, string) (id.ServiceID, error)
		}); ok {
			parsedServiceID, err = resolver.ResolveID(ctx, serviceIDStr)
		} else {
			parsedServiceID, err = id.ParseServiceID(serviceIDStr)
		}
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "invalid service_id", err.Error())
			return
		}
		serviceID = parsedServiceID
	}

	permissionSets, err := h.svc.List(ctx, serviceID)
	if err != nil {
		h.handleStorageError(w, r, "List", err)
		return
	}

	items := make([]PermissionSetResponse, 0)
	if permissionSets != nil {
		items = make([]PermissionSetResponse, len(permissionSets))
		for i, ps := range permissionSets {
			items[i] = h.toResponse(ps)
		}
	}
	if requestsCanonicalReferences(r.Header.Get("Prefer")) {
		responsePointers := make([]*PermissionSetResponse, len(items))
		for i := range items {
			responsePointers[i] = &items[i]
		}
		honored, err := h.applyCanonicalServiceScopes(ctx, permissionSets, responsePointers)
		if err != nil {
			h.handleStorageError(w, r, "List", err)
			return
		}
		if honored {
			w.Header().Set("Preference-Applied", "reference-id=canonical")
		}
	}

	resp := PermissionSetListResponse{Items: items}
	h.writeJSON(w, http.StatusOK, resp)
}

// Update handles PUT /api/permission-sets/:id
func (h *PermissionSetsHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	psIDStr := chi.URLParam(r, "id")

	if psIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "permission set ID is required", "")
		return
	}

	psID, err := h.svc.ResolveID(ctx, psIDStr)
	if err != nil {
		h.handleStorageError(w, r, "Update", err)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	var req CreatePermissionSetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawFields); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	_, canonicalIDPresent := rawFields["canonical_id"]

	// Validate request
	if err := h.validateCreateRequest(ctx, &req); err != nil {
		h.logger.Warn("validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// Convert request service scopes to domain model
	serviceScopes, err := h.convertServiceScopes(ctx, req.ServiceScopes)
	if err != nil {
		h.logger.Warn("invalid service scopes", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	// Get existing permission set to preserve created_at
	existing, err := h.svc.Get(ctx, psID)
	if err != nil {
		h.handleStorageError(w, r, "Get", err)
		return
	}

	// Update permission set entity
	ps := &storage.PermissionSet{
		CanonicalID:      req.CanonicalID,
		ClearCanonicalID: req.CanonicalID == nil && canonicalIDPresent,
		ID:               psID,
		Name:             req.Name,
		Description:      req.Description,
		ServiceScopes:    serviceScopes,
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        time.Now().UTC(),
	}

	if !canonicalIDPresent {
		ps.CanonicalID = existing.CanonicalID
	}

	// Update in service
	if err := h.svc.Update(ctx, ps); err != nil {
		h.handleStorageError(w, r, "Update", err)
		return
	}

	h.logger.Info("permission set updated", "permission_set_id", ps.ID, "name", ps.Name)

	// Return updated permission set
	resp := h.toResponse(ps)
	h.writeJSON(w, http.StatusOK, resp)
}

// Delete handles DELETE /api/permission-sets/:id
func (h *PermissionSetsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	psIDStr := chi.URLParam(r, "id")

	if psIDStr == "" {
		h.writeError(w, http.StatusBadRequest, "permission set ID is required", "")
		return
	}

	psID, err := h.svc.ResolveID(ctx, psIDStr)
	if err != nil {
		h.handleStorageError(w, r, "Delete", err)
		return
	}

	// Delete from service. Both storage implementations are idempotent (missing ID
	// is a no-op), so NotFound never occurs in practice — treat it as success.
	if err := h.svc.Delete(ctx, psID); err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindNotFound {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.handleStorageError(w, r, "Delete", err)
		return
	}

	h.logger.Info("permission set deleted", "permission_set_id", psIDStr)

	// Return 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// validateCreateRequest validates the create permission set request.
func (h *PermissionSetsHandler) validateCreateRequest(ctx context.Context, req *CreatePermissionSetRequest) error {
	if req.Name == "" {
		return errors.New("name is required")
	}
	if len(req.Name) > 255 {
		return errors.New("name must not exceed 255 characters")
	}
	if req.Description == "" {
		return errors.New("description is required")
	}
	if len(req.ServiceScopes) == 0 {
		return errors.New("service_scopes must contain at least one entry")
	}

	for i, ss := range req.ServiceScopes {
		if ss.ServiceID == "" {
			return fmt.Errorf("service_scope[%d]: service_id is required", i)
		}
		for j, scope := range ss.Scopes {
			if scope == "" {
				return fmt.Errorf("service_scope[%d].scopes[%d]: scope cannot be empty", i, j)
			}
		}
		if ss.RequirementType != "" {
			rt := storage.RequirementType(ss.RequirementType)
			if !rt.Valid() {
				return fmt.Errorf("service_scope[%d]: invalid requirement_type %q", i, ss.RequirementType)
			}
		}
		if h.providerScopeValidator != nil {
			serviceID, err := h.resolveServiceID(ctx, ss.ServiceID)
			if err != nil {
				return fmt.Errorf("service_scope[%d]: invalid service_id: %w", i, err)
			}
			provider, err := h.providerScopeValidator.Get(ctx, serviceID)
			if err != nil {
				return fmt.Errorf("service_scope[%d]: failed to load service: %w", i, err)
			}
			if len(ss.Scopes) == 0 && len(provider.Scopes) > 0 {
				return fmt.Errorf("service_scope[%d]: scopes are required for a scoped service", i)
			}
		}
	}

	return nil
}

func (h *PermissionSetsHandler) convertServiceScopes(ctx context.Context, reqScopes []ServiceScopeRequest) ([]storage.ServiceScope, error) {
	if len(reqScopes) == 0 {
		return nil, errors.New("service_scopes must contain at least one entry")
	}

	result := make([]storage.ServiceScope, len(reqScopes))
	for i, req := range reqScopes {
		parsedServiceID, err := h.resolveServiceID(ctx, req.ServiceID)
		if err != nil {
			return nil, fmt.Errorf("service_scope[%d]: invalid service_id", i)
		}

		rt := storage.RequirementTypeOptional
		if req.RequirementType != "" {
			rt = storage.RequirementType(req.RequirementType)
		}

		result[i] = storage.ServiceScope{
			ServiceID:       parsedServiceID,
			Scopes:          req.Scopes,
			RequirementType: rt,
		}
	}

	return result, nil
}

func (h *PermissionSetsHandler) resolveServiceID(ctx context.Context, value string) (id.ServiceID, error) {
	if resolver, ok := h.providerScopeValidator.(interface {
		ResolveID(context.Context, string) (id.ServiceID, error)
	}); ok {
		return resolver.ResolveID(ctx, value)
	}
	return id.ParseServiceID(value)
}

// toResponse converts a PermissionSet domain entity to PermissionSetResponse.
func (h *PermissionSetsHandler) toResponse(ps *storage.PermissionSet) PermissionSetResponse {
	resp := PermissionSetResponse{
		ID:          ps.ID.String(),
		CanonicalID: ps.CanonicalID,
		Name:        ps.Name,
		Description: ps.Description,
		CreatedAt:   ps.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   ps.UpdatedAt.Format(time.RFC3339),
	}

	resp.ServiceScopes = make([]ServiceScopeResponse, len(ps.ServiceScopes))
	for i, ss := range ps.ServiceScopes {
		resp.ServiceScopes[i] = ServiceScopeResponse{
			ServiceID:       ss.ServiceID.String(),
			Scopes:          ss.Scopes,
			RequirementType: string(ss.RequirementType),
		}
	}

	return resp
}

func (h *PermissionSetsHandler) applyCanonicalServiceScopes(ctx context.Context, permissionSets []*storage.PermissionSet, responses []*PermissionSetResponse) (bool, error) {
	canonicalLookup, ok := h.providerScopeValidator.(interface {
		CanonicalIDs(context.Context, []id.ServiceID) (map[id.ServiceID]string, error)
	})
	if !ok {
		return false, nil
	}
	serviceIDs := make([]id.ServiceID, 0)
	seen := make(map[id.ServiceID]struct{})
	for _, permissionSet := range permissionSets {
		for _, scope := range permissionSet.ServiceScopes {
			if _, exists := seen[scope.ServiceID]; !exists {
				seen[scope.ServiceID] = struct{}{}
				serviceIDs = append(serviceIDs, scope.ServiceID)
			}
		}
	}
	canonicalIDs, err := canonicalLookup.CanonicalIDs(ctx, serviceIDs)
	if err != nil {
		return false, err
	}
	for i, permissionSet := range permissionSets {
		for j, scope := range permissionSet.ServiceScopes {
			if canonicalID, exists := canonicalIDs[scope.ServiceID]; exists {
				responses[i].ServiceScopes[j].ServiceID = canonicalID // #nosec G602 -- private helper receives one response per permission set in the same order.
			}
		}
	}
	return true, nil
}

// handleStorageError converts storage errors to HTTP responses.
func (h *PermissionSetsHandler) handleStorageError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var storageErr *storage.StorageError
	if !errors.As(err, &storageErr) {
		h.logger.Error("unexpected error type", "operation", operation, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	switch storageErr.Kind {
	case storage.ErrorKindNotFound:
		h.writeError(w, http.StatusNotFound, "permission set not found", storageErr.Message)
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
}

// writeJSON writes a JSON response.
func (h *PermissionSetsHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *PermissionSetsHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
