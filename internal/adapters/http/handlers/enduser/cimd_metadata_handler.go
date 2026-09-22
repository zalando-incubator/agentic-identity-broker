package enduser

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	cimdMetadataCacheControl = "public, max-age=300"
	cimdMetadataUnavailable  = "CIMD service unavailable"
)

type cimdMetadataErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// CIMDMetadataHandler serves public broker-hosted CIMD metadata and JWK documents.
type CIMDMetadataHandler struct {
	metadata ports.CIMDClientMetadataProvider
	logger   *slog.Logger
}

// NewCIMDMetadataHandler constructs the public CIMD metadata handler.
func NewCIMDMetadataHandler(metadata ports.CIMDClientMetadataProvider, logger *slog.Logger) *CIMDMetadataHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CIMDMetadataHandler{metadata: metadata, logger: logger}
}

// Metadata serves a Client ID Metadata Document.
func (h *CIMDMetadataHandler) Metadata(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := h.serviceID(r)
	if !ok || h.metadata == nil {
		h.writeUnavailable(w)
		return
	}

	metadata, err := h.metadata.Metadata(r.Context(), serviceID)
	if err != nil || metadata == nil {
		h.audit(serviceID, "metadata", "rejected")
		h.writeUnavailable(w)
		return
	}
	h.audit(serviceID, "metadata", "success")
	h.writeJSON(w, http.StatusOK, metadata)
}

// JWKS serves the dedicated public CIMD client-authentication JWK set.
func (h *CIMDMetadataHandler) JWKS(w http.ResponseWriter, r *http.Request) {
	serviceID, ok := h.serviceID(r)
	if !ok || h.metadata == nil {
		h.writeUnavailable(w)
		return
	}

	keys, err := h.metadata.JWKSet(r.Context(), serviceID)
	if err != nil || keys == nil {
		h.audit(serviceID, "jwks", "rejected")
		h.writeUnavailable(w)
		return
	}
	h.audit(serviceID, "jwks", "success")
	h.writeJSON(w, http.StatusOK, keys)
}

func (h *CIMDMetadataHandler) serviceID(r *http.Request) (id.ServiceID, bool) {
	serviceID, err := id.ParseServiceID(chi.URLParam(r, "service-id"))
	if err != nil {
		return id.ServiceID{}, false
	}
	return serviceID, true
}

func (h *CIMDMetadataHandler) audit(serviceID id.ServiceID, operation, outcome string) {
	h.logger.Info("CIMD metadata access", "service_id", serviceID, "operation", operation, "outcome", outcome)
}

func (h *CIMDMetadataHandler) writeUnavailable(w http.ResponseWriter) {
	h.writeJSON(w, http.StatusNotFound, cimdMetadataErrorResponse{Error: "not found", Message: cimdMetadataUnavailable})
}

func (h *CIMDMetadataHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusOK {
		w.Header().Set("Cache-Control", cimdMetadataCacheControl)
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Warn("failed to encode CIMD metadata response")
	}
}
