package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/requestbody"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
)

type ProtectedResourcesHandler struct {
	providerService *thirdparty.ThirdpartyOAuth2ProviderService
	logger          *slog.Logger
}

func NewProtectedResourcesHandler(providerService *thirdparty.ThirdpartyOAuth2ProviderService, logger *slog.Logger) *ProtectedResourcesHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ProtectedResourcesHandler{providerService: providerService, logger: logger}
}

type protectedResourceCreateRequest struct {
	ResourceURI string `json:"resource_uri"`
}
type protectedResourceRenameRequest struct {
	To string `json:"to"`
}
type protectedResourceMutationResponse struct {
	Resource           string   `json:"resource"`
	ProtectedResources []string `json:"protected_resources"`
}
type protectedResourceSetResponse struct {
	ProtectedResources []string `json:"protected_resources"`
}

func (h *ProtectedResourcesHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req protectedResourceCreateRequest
	if err := requestbody.Decode(w, r, 8<<10, &req); err != nil {
		status := requestbody.Status(err)
		h.error(w, status, "invalid request body", "")
		h.audit(r, "add", chi.URLParam(r, "service-id"), "rejected", status, "", "", "")
		return
	}
	h.add(w, r, req.ResourceURI)
}
func (h *ProtectedResourcesHandler) Add(w http.ResponseWriter, r *http.Request) {
	resource, err := h.memberResource(r)
	if err != nil {
		h.error(w, http.StatusBadRequest, "validation failed", "")
		h.audit(r, "add", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, "", "", "")
		return
	}
	h.add(w, r, resource)
}
func (h *ProtectedResourcesHandler) add(w http.ResponseWriter, r *http.Request, resource string) {
	normalized, _ := h.normalized(resource)
	serviceID, ok := h.serviceID(w, r)
	if !ok {
		h.audit(r, "add", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, normalized, "", "")
		return
	}
	result, err := h.providerService.AddProtectedResource(r.Context(), serviceID, resource)
	if err != nil {
		status := h.storageError(w, err)
		h.audit(r, "add", serviceID.String(), "rejected", status, normalized, "", "")
		return
	}
	status := http.StatusCreated
	if !result.Changed {
		status = http.StatusOK
	}
	h.etag(w, result.Version)
	h.json(w, status, protectedResourceMutationResponse{result.Resource, result.ProtectedResources})
	h.audit(r, "add", serviceID.String(), "success", status, normalized, "", "")
}
func (h *ProtectedResourcesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	resource, err := h.memberResource(r)
	if err != nil {
		h.error(w, http.StatusBadRequest, "validation failed", "")
		h.audit(r, "remove", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, "", "", "")
		return
	}
	normalized, _ := h.normalized(resource)
	serviceID, ok := h.serviceID(w, r)
	if !ok {
		h.audit(r, "remove", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, normalized, "", "")
		return
	}
	result, err := h.providerService.RemoveProtectedResource(r.Context(), serviceID, resource)
	if err != nil {
		status := h.storageError(w, err)
		h.audit(r, "remove", serviceID.String(), "rejected", status, normalized, "", "")
		return
	}
	h.etag(w, result.Version)
	h.json(w, http.StatusOK, protectedResourceMutationResponse{result.Resource, result.ProtectedResources})
	h.audit(r, "remove", serviceID.String(), "success", http.StatusOK, normalized, "", "")
}
func (h *ProtectedResourcesHandler) Rename(w http.ResponseWriter, r *http.Request) {
	from, sourceErr := h.memberResource(r)
	source, sourceValid := h.normalized(from)

	var req protectedResourceRenameRequest
	if err := requestbody.Decode(w, r, 8<<10, &req); err != nil {
		status := requestbody.Status(err)
		h.error(w, status, "invalid request body", "")
		h.audit(r, "rename", chi.URLParam(r, "service-id"), "rejected", status, "", source, "")
		return
	}
	target, targetValid := h.normalized(req.To)
	if sourceErr != nil {
		h.error(w, http.StatusBadRequest, "validation failed", "")
		h.audit(r, "rename", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, "", "", target)
		return
	}
	serviceID, ok := h.serviceID(w, r)
	if !ok {
		h.audit(r, "rename", chi.URLParam(r, "service-id"), "rejected", http.StatusBadRequest, "", source, target)
		return
	}
	if !sourceValid {
		source = ""
	}
	if !targetValid {
		target = ""
	}
	result, err := h.providerService.RenameProtectedResource(r.Context(), serviceID, from, req.To)
	if err != nil {
		status := h.storageError(w, err)
		h.audit(r, "rename", serviceID.String(), "rejected", status, "", source, target)
		return
	}
	h.etag(w, result.Version)
	h.json(w, http.StatusOK, protectedResourceMutationResponse{result.Resource, result.ProtectedResources})
	h.audit(r, "rename", serviceID.String(), "success", http.StatusOK, "", source, target)
}
func (h *ProtectedResourcesHandler) List(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := h.serviceID(w, r)
	if !ok {
		return
	}
	resources, version, err := h.providerService.ListProtectedResources(r.Context(), serviceID)
	if err != nil {
		h.storageError(w, err)
		return
	}
	h.etag(w, version)
	h.json(w, http.StatusOK, protectedResourceSetResponse{resources})
}
func (h *ProtectedResourcesHandler) serviceID(w http.ResponseWriter, r *http.Request) (id.ServiceID, bool) {
	serviceID, err := h.providerService.ResolveID(r.Context(), chi.URLParam(r, "service-id"))
	if err != nil {
		h.storageError(w, err)
		return id.ServiceID{}, false
	}
	return serviceID, true
}
func (h *ProtectedResourcesHandler) memberResource(r *http.Request) (string, error) {
	prefix := "/api/services/" + chi.URLParam(r, "service-id") + "/protected-resources/"
	// r.URL.Path is percent-decoded, so extract EscapedPath to preserve encoded member delimiters for one decode.
	escaped := strings.TrimPrefix(r.URL.EscapedPath(), prefix)
	if escaped == r.URL.EscapedPath() || escaped == "" || strings.Contains(escaped, "/") {
		return "", errors.New("invalid resource path")
	}
	return url.PathUnescape(escaped)
}
func (h *ProtectedResourcesHandler) normalized(value string) (string, bool) {
	normalized, err := model.NormalizeAndValidateProtectedResource(value)
	if err != nil {
		return "", false
	}
	return normalized, true
}
func (h *ProtectedResourcesHandler) storageError(w http.ResponseWriter, err error) int {
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		switch storageErr.Kind {
		case storage.ErrorKindValidation:
			h.error(w, http.StatusBadRequest, "validation failed", storageErr.Message)
			return http.StatusBadRequest
		case storage.ErrorKindNotFound:
			h.error(w, http.StatusNotFound, "service not found", storageErr.Message)
			return http.StatusNotFound
		case storage.ErrorKindConflict:
			h.error(w, http.StatusConflict, "conflict", storageErr.Message)
			return http.StatusConflict
		}
	}
	h.error(w, http.StatusInternalServerError, "internal server error", "")
	return http.StatusInternalServerError
}
func (h *ProtectedResourcesHandler) etag(w http.ResponseWriter, version int64) {
	w.Header().Set("ETag", strongETag(version))
}
func (h *ProtectedResourcesHandler) json(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (h *ProtectedResourcesHandler) error(w http.ResponseWriter, status int, code, message string) {
	h.json(w, status, ErrorResponse{Error: code, Message: message})
}
func (h *ProtectedResourcesHandler) audit(r *http.Request, action, serviceID, outcome string, status int, resource, source, target string) {
	principalValue, _ := principal.FromContext(r.Context())
	h.logger.Info("protected_resource_audit", "principal", principalValue, "action", action, "service_id", serviceID, "outcome", outcome, "status", status, "request_id", r.Header.Get("X-Request-ID"), "resource_uri", nullable(resource), "source_resource_uri", nullable(source), "target_resource_uri", nullable(target))
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
