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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
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
	ResourceURL     *string `json:"resource_url,omitempty"`
	ClientMethod    *string `json:"client_method,omitempty"` // Read-only; reject when supplied for resource discovery.
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
	EnableDiscovery                    bool    `json:"enable_discovery"`
	MetadataURL                        *string `json:"metadata_url,omitempty"`
	ResourceURL                        *string `json:"resource_url,omitempty"`
	ClientMethod                       *string `json:"client_method"`
	AuthorizationParamResourceStrategy *string `json:"authorization_param_resource_strategy"`
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
	if req.Discovery.ResourceURL != nil {
		if req.TokenEndpointAuthMethod != nil {
			return "", model.Secret{}, errors.New("token_endpoint_auth_method must be absent for protected-resource discovery")
		}
		if req.Endpoints != nil {
			return "", model.Secret{}, errors.New("endpoints must be absent for protected-resource discovery")
		}
		if req.Discovery.ClientMethod != nil {
			return "", model.Secret{}, errors.New("discovery.client_method must be selected by the broker")
		}
		if req.ClientSecret != "" {
			return "", model.NewPlaintextSecret(req.ClientSecret), nil
		}
		return "", model.NewAbsentSecret(), nil
	}
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
		Discovery:               model.DiscoveryConfig{EnableDiscovery: req.Discovery.EnableDiscovery, MetadataURL: req.Discovery.MetadataURL, ResourceURL: req.Discovery.ResourceURL},
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

	if req.Discovery.ResourceURL != nil {
		if err := entity.ValidateDiscoveryRequest(); err != nil {
			h.writeDiscoveryRequestError(w, err)
			return
		}
	} else if err := entity.ValidateForCreate(skipHTTPSValidation); err != nil {
		h.logger.Warn("service validation failed",
			"client_id", entity.ClientID,
			"error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if flavor != model.OAuth2FlavorGoogle && req.Discovery.EnableDiscovery && req.Discovery.ResourceURL == nil {
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

	entity.NormalizeProtectedResources()
	if req.Discovery.ResourceURL == nil {
		if err := entity.ValidateProtectedResources(); err != nil {
			h.logger.Warn("protected_resources validation failed",
				"service_id", entity.ID,
				"error", err)
			h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
			return
		}
	}

	// Create service (branch key provisioning and encryption handled by domain service)
	if err := h.providerService.Create(ctx, entity); err != nil {
		h.handleServiceMutationError(w, r, "CreateService", err, req.Discovery.ResourceURL != nil)
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

// GetDiscoveryStatus reads committed state without contacting the provider.
func (h *ServicesHandler) GetDiscoveryStatus(w http.ResponseWriter, r *http.Request) {
	if subject, ok := principal.FromContext(r.Context()); !ok || subject == "" {
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	serviceID := chi.URLParam(r, "service-id")
	if serviceID == "" {
		h.writeError(w, http.StatusBadRequest, "invalid service ID", "service ID is required")
		return
	}
	if _, err := id.ParseServiceID(serviceID); err != nil {
		if canonical.Validate(&serviceID) != nil {
			h.writeError(w, http.StatusBadRequest, "invalid service ID", "service ID is invalid")
			return
		}
	}
	parsedID, err := h.providerService.ResolveID(r.Context(), serviceID)
	if err != nil {
		h.handleStorageError(w, r, "GetDiscoveryStatus", err)
		return
	}
	status, err := h.providerService.GetDiscoveryStatus(r.Context(), parsedID)
	if err != nil {
		h.handleStorageError(w, r, "GetDiscoveryStatus", err)
		return
	}
	h.writeJSON(w, http.StatusOK, struct {
		Status        string     `json:"status"`
		ResourceURL   *string    `json:"resource_url"`
		IssuerURI     *string    `json:"issuer_uri"`
		ClientMethod  *string    `json:"client_method"`
		LastAttemptAt *time.Time `json:"last_attempt_at"`
		LastSuccessAt *time.Time `json:"last_success_at"`
		FailureReason *string    `json:"failure_reason"`
	}{status.Status, status.ResourceURL, status.IssuerURI, status.ClientMethod, status.LastAttemptAt, status.LastSuccessAt, status.FailureReason})
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
		Discovery:               model.DiscoveryConfig{EnableDiscovery: req.Discovery.EnableDiscovery, MetadataURL: req.Discovery.MetadataURL, ResourceURL: req.Discovery.ResourceURL},
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

	if req.Discovery.ResourceURL != nil {
		if err := entity.ValidateDiscoveryRequest(); err != nil {
			h.writeDiscoveryRequestError(w, err)
			return
		}
	} else if err := entity.ValidateForUpdate(skipHTTPSValidation); err != nil {
		h.logger.Warn("service validation failed",
			"client_id", entity.ClientID,
			"error", err)
		h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
		return
	}

	if flavor != model.OAuth2FlavorGoogle && req.Discovery.EnableDiscovery && req.Discovery.ResourceURL == nil {
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
		if req.Discovery.ResourceURL == nil {
			if err := entity.ValidateProtectedResources(); err != nil {
				h.logger.Warn("protected_resources validation failed", "service_id", entity.ID, "error", err)
				h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
				return
			}
		}
	}

	if err := h.providerService.Update(ctx, entity, expectedVersion); err != nil {
		var storageErr *storage.StorageError
		if expectedVersion != nil && errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict && strings.Contains(storageErr.Message, "version") {
			h.writeError(w, http.StatusPreconditionFailed, "precondition failed", storageErr.Message)
			return
		}
		h.handleServiceMutationError(w, r, "UpdateService", err, req.Discovery.ResourceURL != nil)
		return
	}

	if req.Discovery.ResourceURL == nil {
		h.logger.Info("OAuth2 service updated",
			"service_id", entity.ID,
			"client_id", entity.ClientID)
	}

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
			ResourceURL:     entity.Discovery.ResourceURL,
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
	if entity.Discovery.ClientMethod != "" {
		clientMethod := string(entity.Discovery.ClientMethod)
		response.Discovery.ClientMethod = &clientMethod
	}
	if entity.Discovery.ResourceURL != nil {
		strategy := "derived"
		if entity.ResourceExplicit {
			strategy = "pinned"
		}
		response.Discovery.AuthorizationParamResourceStrategy = &strategy
	}

	if !entity.TokenEndpointAuthMethod.IsAbsent() {
		tokenEndpointAuthMethod := string(entity.TokenEndpointAuthMethod)
		response.TokenEndpointAuthMethod = &tokenEndpointAuthMethod
	}
	if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() && entity.Discovery.ClientMethod != model.ClientBootstrapDCR {
		clientSecret := entity.Secret.Redacted()
		response.ClientSecret = &clientSecret
	}

	return response
}

func (h *ServicesHandler) writeDiscoveryRequestError(w http.ResponseWriter, err error) {
	if errors.Is(err, model.ErrUnsafeDiscoveryResourceURL) {
		h.writeDiscoveryError(w, &thirdparty.DiscoveryError{Code: "unsafe_destination"})
		return
	}
	h.writeError(w, http.StatusBadRequest, "validation failed", err.Error())
}

// handleServiceMutationError keeps remote discovery and registration errors out of handler logs.
func (h *ServicesHandler) handleServiceMutationError(w http.ResponseWriter, r *http.Request, operation string, err error, resourceDiscovery bool) {
	if !resourceDiscovery {
		h.handleStorageError(w, r, operation, err)
		return
	}
	if errors.Is(r.Context().Err(), context.Canceled) {
		return
	}
	var discoveryErr *thirdparty.DiscoveryError
	if errors.As(err, &discoveryErr) {
		h.writeDiscoveryError(w, discoveryErr)
		return
	}
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict {
		if errors.Is(storageErr, storage.ErrDuplicateDCRClientIdentity) {
			h.writeError(w, http.StatusConflict, "conflict", "duplicate_client_identity")
			return
		}
		if errors.Is(storageErr, storage.ErrIssuerChangeHasSessions) {
			h.writeError(w, http.StatusConflict, "conflict", "issuer_change_requires_no_sessions")
			return
		}
		if errors.Is(storageErr, storage.ErrResourceChangeHasSessions) {
			h.writeError(w, http.StatusConflict, "conflict", "resource_change_requires_no_sessions")
			return
		}
		if errors.Is(storageErr, storage.ErrProtectedResourceOwned) {
			h.writeError(w, http.StatusConflict, "conflict", "protected resource URI already configured for another service")
			return
		}
		h.writeError(w, http.StatusConflict, "conflict", "service conflict")
		return
	}
	failureClass := "internal_failure"
	if storageErr != nil {
		failureClass = "storage_" + string(storageErr.Kind)
	}
	h.logger.Error("discovery service mutation failed", "operation", operation, "failure_class", failureClass)
	h.writeError(w, http.StatusInternalServerError, "internal server error", "")
}

// handleStorageError converts storage errors to HTTP responses.
func (h *ServicesHandler) handleStorageError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var discoveryErr *thirdparty.DiscoveryError
	if errors.As(err, &discoveryErr) {
		h.writeDiscoveryError(w, discoveryErr)
		return
	}
	var storageErr *storage.StorageError
	if !errors.As(err, &storageErr) {
		h.logger.Error("unexpected error type", "operation", operation, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	if errors.Is(storageErr, storage.ErrResourceChangeHasSessions) {
		h.writeError(w, http.StatusConflict, "conflict", "resource_change_requires_no_sessions")
		return
	}
	if errors.Is(storageErr, storage.ErrProtectedResourceOwned) {
		h.writeError(w, http.StatusConflict, "conflict", "protected resource URI already configured for another service")
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

// writeDiscoveryError returns only approved failure codes, never remote response text.
func (h *ServicesHandler) writeDiscoveryError(w http.ResponseWriter, discoveryErr *thirdparty.DiscoveryError) {
	status, category := http.StatusBadRequest, "discovery failed"
	switch discoveryErr.Code {
	case "resource_metadata_not_found", "resource_metadata_unavailable", "resource_metadata_invalid",
		"resource_mismatch", "authorization_server_missing", "issuer_selection_required",
		"issuer_not_advertised", "authorization_server_metadata_not_found",
		"authorization_server_metadata_unavailable", "authorization_server_metadata_invalid",
		"issuer_mismatch", "unsafe_destination", "response_too_large":
	case "no_compatible_client_method", "cimd_unavailable", "client_registration_rejected",
		"client_name_unconfigured", "client_registration_invalid", "client_method_changed":
		category = "client registration failed"
	case "timeout":
		status = http.StatusGatewayTimeout
	case "duplicate_client_identity", "issuer_change_requires_no_sessions", "resource_change_requires_no_sessions":
		status, category = http.StatusConflict, "conflict"
	default:
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	if discoveryErr.Code == "issuer_selection_required" {
		if len(discoveryErr.AuthorizationServers) < 2 {
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			return
		}
		seen := make(map[string]struct{}, len(discoveryErr.AuthorizationServers))
		for _, issuer := range discoveryErr.AuthorizationServers {
			parsed, err := url.Parse(issuer)
			if err != nil || model.ValidatePublicHTTPSURL(issuer) != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(issuer, "#") {
				h.writeError(w, http.StatusInternalServerError, "internal server error", "")
				return
			}
			if _, exists := seen[issuer]; exists {
				h.writeError(w, http.StatusInternalServerError, "internal server error", "")
				return
			}
			seen[issuer] = struct{}{}
		}
		h.writeJSON(w, status, struct {
			Error                string   `json:"error"`
			Message              string   `json:"message"`
			AuthorizationServers []string `json:"authorization_servers"`
		}{category, discoveryErr.Code, discoveryErr.AuthorizationServers})
		return
	}

	h.writeError(w, status, category, discoveryErr.Code)
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
