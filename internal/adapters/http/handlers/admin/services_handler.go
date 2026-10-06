// Package admin provides HTTP handlers for administrative APIs.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ServicesHandler handles HTTP requests for third-party OAuth2 service CRUD operations.
type ServicesHandler struct {
	providerService *thirdparty.ThirdpartyOAuth2ProviderService
	config          *ports.Config
	logger          *slog.Logger
}

// NewServicesHandler creates a new services handler.
func NewServicesHandler(providerService *thirdparty.ThirdpartyOAuth2ProviderService, config *ports.Config, logger *slog.Logger) *ServicesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ServicesHandler{
		providerService: providerService,
		config:          config,
		logger:          logger,
	}
}

// ServiceRequest represents the request body for creating/updating a service.
type ServiceRequest struct {
	CanonicalID             *string                 `json:"canonical_id,omitempty"`
	DisplayName             string                  `json:"display_name"`
	ClientID                string                  `json:"client_id"`
	ClientSecret            string                  `json:"client_secret"`
	TokenEndpointAuthMethod *string                 `json:"token_endpoint_auth_method,omitempty"`
	OAuth2Flavor            string                  `json:"oauth2_flavor,omitempty"`
	IssuerURI               string                  `json:"issuer_uri"`
	Discovery               DiscoveryConfigRequest  `json:"discovery"`
	Endpoints               *OAuth2EndpointsRequest `json:"endpoints,omitempty"`
	Scopes                  []OAuthScopeRequest     `json:"scopes"`
	ProtectedResources      []string                `json:"protected_resources,omitempty"` // RFC 8693 resource URIs
	AuthorizationParams     map[string]string       `json:"authorization_params,omitempty"`
}

// DiscoveryConfigRequest represents the discovery configuration in requests.
type DiscoveryConfigRequest struct {
	EnableDiscovery bool    `json:"enable_discovery"`
	MetadataURL     *string `json:"metadata_url,omitempty"`
}

// OAuth2EndpointsRequest represents OAuth2 endpoints in requests.
type OAuth2EndpointsRequest struct {
	TokenEndpoint     string `json:"token_endpoint"`
	AuthorizeEndpoint string `json:"authorize_endpoint"`
}

// OAuthScopeRequest represents a scope in requests.
type OAuthScopeRequest struct {
	ScopeValue  string `json:"scope_value"`
	Description string `json:"description"`
}

// ServiceResponse represents the response body for service operations.
// Client secrets are redacted for confidential services and omitted for public services per SR-003.
type ServiceResponse struct {
	ID                      string                  `json:"id"`
	CanonicalID             *string                 `json:"canonical_id"`
	DisplayName             string                  `json:"display_name"`
	ClientID                string                  `json:"client_id"`
	ClientSecret            *string                 `json:"client_secret,omitempty"`
	TokenEndpointAuthMethod *string                 `json:"token_endpoint_auth_method"`
	OAuth2Flavor            string                  `json:"oauth2_flavor"`
	IssuerURI               string                  `json:"issuer_uri"`
	Discovery               DiscoveryConfigResponse `json:"discovery"`
	Endpoints               OAuth2EndpointsResponse `json:"endpoints"`
	Scopes                  []OAuthScopeResponse    `json:"scopes"`
	ProtectedResources      []string                `json:"protected_resources,omitempty"` // RFC 8693 resource URIs
	AuthorizationParams     map[string]string       `json:"authorization_params,omitempty"`
	CreatedAt               string                  `json:"created_at"`
	UpdatedAt               string                  `json:"updated_at"`
}

// DiscoveryConfigResponse represents the discovery configuration in responses.
type DiscoveryConfigResponse struct {
	EnableDiscovery bool    `json:"enable_discovery"`
	MetadataURL     *string `json:"metadata_url,omitempty"`
}

// OAuth2EndpointsResponse represents OAuth2 endpoints in responses.
type OAuth2EndpointsResponse struct {
	TokenEndpoint     string `json:"token_endpoint"`
	AuthorizeEndpoint string `json:"authorize_endpoint"`
}

// OAuthScopeResponse represents a scope in responses.
type OAuthScopeResponse struct {
	ScopeValue  string `json:"scope_value"`
	Description string `json:"description"`
}

func serviceClientAuthentication(req ServiceRequest) (model.TokenEndpointAuthMethod, model.Secret, error) {
	if req.TokenEndpointAuthMethod != nil && *req.TokenEndpointAuthMethod == "" {
		return "", model.Secret{}, errors.New(`token_endpoint_auth_method: only "none" and "private_key_jwt" are accepted`)
	}

	method := model.TokenEndpointAuthMethod("")
	if req.TokenEndpointAuthMethod != nil {
		method = model.TokenEndpointAuthMethod(*req.TokenEndpointAuthMethod)
	}
	if err := method.Validate(); err != nil {
		return "", model.Secret{}, err
	}
	if method.IsCIMDConfidential() {
		if req.ClientID != "" {
			return "", model.Secret{}, errors.New(`client_id must be absent when token_endpoint_auth_method is "private_key_jwt"`)
		}
		if req.ClientSecret != "" {
			return "", model.Secret{}, errors.New(`client_secret must be absent when token_endpoint_auth_method is "private_key_jwt"`)
		}
		return method, model.NewAbsentSecret(), nil
	}
	if method == model.TokenEndpointAuthMethodNone && req.ClientSecret == "" {
		return method, model.NewAbsentSecret(), nil
	}
	return method, model.NewPlaintextSecret(req.ClientSecret), nil
}

// CreateService handles POST /api/third-party/oauth2/clients
func (h *ServicesHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req ServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	tokenEndpointAuthMethod, secret, err := serviceClientAuthentication(req)
	if err != nil {
		h.logger.Warn("service validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	skipHTTPSValidation := false
	if h.config != nil {
		skipHTTPSValidation = h.config.Security.SkipThirdpartyHTTPSValidation
	}

	// Parse and validate oauth2_flavor; default to "standard" when omitted,
	// or auto-detect "github" from token endpoint URL.
	flavor := model.OAuth2Flavor(req.OAuth2Flavor)
	if flavor == "" {
		var tokenEndpoint string
		if req.Endpoints != nil {
			tokenEndpoint = req.Endpoints.TokenEndpoint
		}
		flavor = model.InferOAuth2Flavor(tokenEndpoint)
	}

	// Build entity from request
	now := time.Now().UTC()
	serviceID := id.NewServiceID()
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      serviceID,
		CanonicalID:             req.CanonicalID,
		DisplayName:             req.DisplayName,
		ClientID:                id.ClientID(req.ClientID),
		Secret:                  secret,
		TokenEndpointAuthMethod: tokenEndpointAuthMethod,
		Flavor:                  flavor,
		IssuerURI:               req.IssuerURI,
		Discovery:               model.DiscoveryConfig{EnableDiscovery: req.Discovery.EnableDiscovery, MetadataURL: req.Discovery.MetadataURL},
		ProtectedResources:      req.ProtectedResources,
		AuthorizationParams:     req.AuthorizationParams,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	// Convert scopes
	entity.Scopes = make([]model.OAuthScope, len(req.Scopes))
	for i, scope := range req.Scopes {
		entity.Scopes[i] = model.OAuthScope{ScopeValue: scope.ScopeValue, Description: scope.Description}
	}

	if flavor != model.OAuth2FlavorGoogle && !req.Discovery.EnableDiscovery && req.Endpoints != nil {
		entity.Endpoints = model.OAuth2Endpoints{
			TokenEndpoint:     req.Endpoints.TokenEndpoint,
			AuthorizeEndpoint: req.Endpoints.AuthorizeEndpoint,
		}
	}

	if err := entity.ValidateForCreate(skipHTTPSValidation); err != nil {
		h.logger.Warn("service validation failed",
			"client_id", entity.ClientID,
			"error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if flavor != model.OAuth2FlavorGoogle && req.Discovery.EnableDiscovery {
		endpoints, err := storage.DiscoverOAuth2Endpoints(ctx, req.IssuerURI, req.Discovery.MetadataURL, skipHTTPSValidation)
		if err != nil {
			h.logger.Warn("OAuth2 endpoint discovery failed, falling back to manual endpoints",
				"issuer_uri", req.IssuerURI,
				"error", err)
			if req.Endpoints != nil {
				entity.Endpoints = model.OAuth2Endpoints{
					TokenEndpoint:     req.Endpoints.TokenEndpoint,
					AuthorizeEndpoint: req.Endpoints.AuthorizeEndpoint,
				}
			}
		} else {
			entity.Endpoints = model.OAuth2Endpoints{
				TokenEndpoint:     endpoints.TokenEndpoint,
				AuthorizeEndpoint: endpoints.AuthorizeEndpoint,
			}
		}

		if err := entity.ValidateForCreate(skipHTTPSValidation); err != nil {
			h.logger.Warn("service validation failed",
				"client_id", entity.ClientID,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
			return
		}
	}

	// Defense-in-depth: normalize and validate protected_resources here so the duplicate
	// check below operates on canonical URIs. The domain service repeats this
	// authoritatively before persistence.
	entity.NormalizeProtectedResources()
	if err := entity.ValidateProtectedResources(); err != nil {
		h.logger.Warn("protected_resources validation failed",
			"service_id", entity.ID,
			"error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if !h.checkProtectedResourceConflicts(ctx, w, r, entity, id.ServiceID{}, "CreateService") {
		return
	}

	// Create service (branch key provisioning and encryption handled by domain service)
	if err := h.providerService.Create(ctx, entity); err != nil {
		h.handleStorageError(w, r, "CreateService", err)
		return
	}

	// Return the created service without exposing its secret.
	resp := h.toResponse(entity.RedactedCopy())
	h.writeJSON(w, http.StatusCreated, resp)
}

// GetService handles GET /api/services/{service-id}
func (h *ServicesHandler) GetService(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()
	serviceID := chi.URLParam(r, "service-id")

	if serviceID == "" {
		h.writeError(w, http.StatusBadRequest, "service ID is required", "")
		return
	}

	parsedSvcID, err := h.providerService.ResolveID(ctx, serviceID)
	if err != nil {
		h.handleStorageError(w, r, "GetService", err)
		return
	}

	entity, err := h.providerService.Get(ctx, parsedSvcID)
	if err != nil {
		h.handleStorageError(w, r, "GetService", err)
		return
	}

	// Return the service without exposing its secret.
	w.Header().Set("ETag", strongETag(entity.Version))
	resp := h.toResponse(entity.RedactedCopy())
	h.writeJSON(w, http.StatusOK, resp)
}

// UpdateService handles PUT /api/services/{service-id}
func (h *ServicesHandler) UpdateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceID := chi.URLParam(r, "service-id")

	if serviceID == "" {
		h.writeError(w, http.StatusBadRequest, "service ID is required", "")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	var req ServiceRequest
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&req); err != nil {
		h.logger.Warn("failed to decode request body", "error", err)
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	var rawRequest map[string]json.RawMessage
	if err := json.Unmarshal(body, &rawRequest); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	protectedResources, protectedResourcesPresent := rawRequest["protected_resources"]
	canonicalID, canonicalIDPresent := rawRequest["canonical_id"]
	protectedResourcesProvided := protectedResourcesPresent && !isJSONNull(protectedResources)

	tokenEndpointAuthMethod, secret, err := serviceClientAuthentication(req)
	if err != nil {
		h.logger.Warn("service validation failed", "error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	skipHTTPSValidation := false
	if h.config != nil {
		skipHTTPSValidation = h.config.Security.SkipThirdpartyHTTPSValidation
	}

	parsedSvcID, err := h.providerService.ResolveID(ctx, serviceID)
	if err != nil {
		h.handleStorageError(w, r, "UpdateService", err)
		return
	}

	// Parse and validate oauth2_flavor; default to "standard" when omitted,
	// or auto-detect "github" from token endpoint URL.
	flavor := model.OAuth2Flavor(req.OAuth2Flavor)
	if flavor == "" {
		var tokenEndpoint string
		if req.Endpoints != nil {
			tokenEndpoint = req.Endpoints.TokenEndpoint
		}
		flavor = model.InferOAuth2Flavor(tokenEndpoint)
	}

	// created_at is not set here; repo.Update() populates it from storage (no KMS decrypt needed).
	entity := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      parsedSvcID,
		CanonicalID:             req.CanonicalID,
		DisplayName:             req.DisplayName,
		ClearCanonicalID:        canonicalIDPresent && isJSONNull(canonicalID),
		ClientID:                id.ClientID(req.ClientID),
		Secret:                  secret,
		TokenEndpointAuthMethod: tokenEndpointAuthMethod,
		Flavor:                  flavor,
		IssuerURI:               req.IssuerURI,
		Discovery:               model.DiscoveryConfig{EnableDiscovery: req.Discovery.EnableDiscovery, MetadataURL: req.Discovery.MetadataURL},
		ProtectedResources:      req.ProtectedResources,
		AuthorizationParams:     req.AuthorizationParams,
		UpdatedAt:               time.Now().UTC(),
	}

	// Convert scopes
	entity.Scopes = make([]model.OAuthScope, len(req.Scopes))
	for i, scope := range req.Scopes {
		entity.Scopes[i] = model.OAuthScope{ScopeValue: scope.ScopeValue, Description: scope.Description}
	}

	if flavor != model.OAuth2FlavorGoogle && !req.Discovery.EnableDiscovery && req.Endpoints != nil {
		entity.Endpoints = model.OAuth2Endpoints{
			TokenEndpoint:     req.Endpoints.TokenEndpoint,
			AuthorizeEndpoint: req.Endpoints.AuthorizeEndpoint,
		}
	}

	if err := entity.ValidateForUpdate(skipHTTPSValidation); err != nil {
		h.logger.Warn("service validation failed",
			"client_id", entity.ClientID,
			"error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if flavor != model.OAuth2FlavorGoogle && req.Discovery.EnableDiscovery {
		endpoints, err := storage.DiscoverOAuth2Endpoints(ctx, req.IssuerURI, req.Discovery.MetadataURL, skipHTTPSValidation)
		if err != nil {
			h.logger.Warn("OAuth2 endpoint discovery failed, falling back to manual endpoints",
				"issuer_uri", req.IssuerURI,
				"error", err)
			if req.Endpoints != nil {
				entity.Endpoints = model.OAuth2Endpoints{
					TokenEndpoint:     req.Endpoints.TokenEndpoint,
					AuthorizeEndpoint: req.Endpoints.AuthorizeEndpoint,
				}
			}
		} else {
			entity.Endpoints = model.OAuth2Endpoints{
				TokenEndpoint:     endpoints.TokenEndpoint,
				AuthorizeEndpoint: endpoints.AuthorizeEndpoint,
			}
		}

		if err := entity.ValidateForUpdate(skipHTTPSValidation); err != nil {
			h.logger.Warn("service validation failed",
				"client_id", entity.ClientID,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
			return
		}
	}

	var expectedVersion *int64
	if protectedResourcesProvided {
		parsedVersion, err := parseStrongETag(r.Header.Get("If-Match"))
		if err != nil {
			h.writeError(w, http.StatusPreconditionRequired, "precondition required", "If-Match with a strong ETag is required when replacing protected_resources")
			return
		}
		expectedVersion = &parsedVersion
		entity.NormalizeProtectedResources()
		if err := entity.ValidateProtectedResources(); err != nil {
			h.logger.Warn("protected_resources validation failed", "service_id", entity.ID, "error", err)
			h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
			return
		}
		if !h.checkProtectedResourceConflicts(ctx, w, r, entity, entity.ID, "UpdateService") {
			return
		}
	}

	if err := h.providerService.Update(ctx, entity, expectedVersion); err != nil {
		var storageErr *storage.StorageError
		if expectedVersion != nil && errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict && strings.Contains(storageErr.Message, "version") {
			h.writeError(w, http.StatusPreconditionFailed, "precondition failed", storageErr.Message)
			return
		}
		h.handleStorageError(w, r, "UpdateService", err)
		return
	}

	h.logger.Info("OAuth2 service updated",
		"service_id", entity.ID,
		"client_id", entity.ClientID)

	resp := h.toResponse(entity.RedactedCopy())
	w.Header().Set("ETag", strongETag(entity.Version))
	h.writeJSON(w, http.StatusOK, resp)
}

func parseStrongETag(value string) (int64, error) {
	if len(value) < 3 || strings.HasPrefix(value, "W/") || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, errors.New("strong ETag required")
	}
	version, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || version < 1 {
		return 0, errors.New("invalid ETag")
	}
	return version, nil
}

func strongETag(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func (h *ServicesHandler) checkProtectedResourceConflicts(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	entity *model.ThirdpartyOAuth2ProviderEntity,
	excludeServiceID id.ServiceID,
	operation string,
) bool {
	for _, resource := range entity.ProtectedResources {
		existing, err := h.providerService.FindByProtectedResource(ctx, resource)
		switch {
		case err == nil && existing == nil:
			continue
		case err == nil && existing.ID == excludeServiceID:
			continue
		case err == nil:
			h.logger.Warn("duplicate protected resource",
				"resource", resource,
				"service_id", existing.ID)
			h.writeError(w, http.StatusConflict, "conflict", "protected resource URI already configured for another service")
			return false
		case tokenexchange.IsResourceNotConfigured(err):
			continue
		case tokenexchange.IsResourceAmbiguous(err):
			h.logger.Warn("protected resource lookup is ambiguous",
				"resource", resource,
				"error", err)
			h.writeError(w, http.StatusConflict, "conflict", "protected resource URI already configured for another service")
			return false
		default:
			var storageErr *storage.StorageError
			if errors.As(err, &storageErr) {
				h.handleStorageError(w, r, operation+"DuplicateCheck", err)
				return false
			}
			h.logger.Error("protected resource duplicate check failed",
				"resource", resource,
				"error", err)
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			return false
		}
	}

	return true
}

// DeleteService handles DELETE /api/services/:service-id
func (h *ServicesHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	serviceID := chi.URLParam(r, "service-id")

	if serviceID == "" {
		h.writeError(w, http.StatusBadRequest, "service ID is required", "")
		return
	}

	// Delete via domain service
	parsedSvcID, err := h.providerService.ResolveID(ctx, serviceID)
	if err != nil {
		h.handleStorageError(w, r, "DeleteService", err)
		return
	}

	if err := h.providerService.Delete(ctx, parsedSvcID); err != nil {
		// Special handling for conflict errors (grants exist); use errors.As for wrapped errors
		var conflictErr *storage.StorageError
		if errors.As(err, &conflictErr) && conflictErr.Kind == storage.ErrorKindConflict {
			message := conflictErr.Message
			h.logger.Warn("service deletion blocked due to existing grants",
				"service_id", serviceID,
				"error", message)
			h.writeError(w, http.StatusConflict, "conflict", message)
			return
		}

		h.handleStorageError(w, r, "DeleteService", err)
		return
	}

	h.logger.Info("OAuth2 service deleted", "service_id", serviceID)

	// Return 204 No Content
	w.WriteHeader(http.StatusNoContent)
}

// ListServices handles GET /api/third-party/oauth2/clients
func (h *ServicesHandler) ListServices(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Vary", "Prefer")
	ctx := r.Context()

	entities, err := h.providerService.List(ctx)
	if err != nil {
		h.handleStorageError(w, r, "ListServices", err)
		return
	}

	// Convert to response format without exposing secrets.
	responses := make([]ServiceResponse, len(entities))
	for i, entity := range entities {
		responses[i] = h.toResponse(entity.RedactedCopy())
	}

	h.writeJSON(w, http.StatusOK, responses)
}

// toResponse converts a ThirdpartyOAuth2ProviderEntity to ServiceResponse.
func (h *ServicesHandler) toResponse(entity *model.ThirdpartyOAuth2ProviderEntity) ServiceResponse {
	scopes := make([]OAuthScopeResponse, len(entity.Scopes))
	for i, scope := range entity.Scopes {
		scopes[i] = OAuthScopeResponse{
			ScopeValue:  scope.ScopeValue,
			Description: scope.Description,
		}
	}

	// Normalize flavor: if empty, use default.
	flavor := entity.Flavor
	if flavor == "" {
		flavor = model.DefaultOAuth2Flavor
	}

	response := ServiceResponse{
		ID:           entity.ID.String(),
		CanonicalID:  entity.CanonicalID,
		DisplayName:  entity.DisplayName,
		ClientID:     entity.ClientID.String(),
		OAuth2Flavor: flavor.String(),
		IssuerURI:    entity.IssuerURI,
		Discovery: DiscoveryConfigResponse{
			EnableDiscovery: entity.Discovery.EnableDiscovery,
			MetadataURL:     entity.Discovery.MetadataURL,
		},
		Endpoints: OAuth2EndpointsResponse{
			TokenEndpoint:     entity.Endpoints.TokenEndpoint,
			AuthorizeEndpoint: entity.Endpoints.AuthorizeEndpoint,
		},
		Scopes:              scopes,
		ProtectedResources:  entity.ProtectedResources,
		AuthorizationParams: entity.AuthorizationParams,
		CreatedAt:           entity.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           entity.UpdatedAt.Format(time.RFC3339),
	}

	if !entity.TokenEndpointAuthMethod.IsAbsent() {
		tokenEndpointAuthMethod := string(entity.TokenEndpointAuthMethod)
		response.TokenEndpointAuthMethod = &tokenEndpointAuthMethod
	}
	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() {
		clientSecret := entity.Secret.Redacted()
		response.ClientSecret = &clientSecret
	}

	return response
}

// handleStorageError converts storage errors to HTTP responses.
func (h *ServicesHandler) handleStorageError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var storageErr *storage.StorageError
	if !errors.As(err, &storageErr) {
		h.logger.Error("unexpected error type", "operation", operation, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	switch storageErr.Kind {
	case storage.ErrorKindNotFound:
		h.writeError(w, http.StatusNotFound, "service not found", storageErr.Message)
	case storage.ErrorKindConflict:
		message := storageErr.Message
		if strings.Contains(message, "grants reference it") {
			h.writeError(w, http.StatusConflict, "conflict", message)
		} else {
			h.writeError(w, http.StatusConflict, "conflict", storageErr.Message)
		}
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
func (h *ServicesHandler) writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *ServicesHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
