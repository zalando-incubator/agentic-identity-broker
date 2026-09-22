package admin

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// CIMDClientKeysHandler manages the dedicated outbound CIMD key lifecycle.
type CIMDClientKeysHandler struct {
	keys   ports.CIMDClientKeyLifecycle
	logger *slog.Logger
}

// NewCIMDClientKeysHandler constructs the CIMD key handler.
func NewCIMDClientKeysHandler(keys ports.CIMDClientKeyLifecycle, logger *slog.Logger) *CIMDClientKeysHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CIMDClientKeysHandler{keys: keys, logger: logger}
}

type cimdKeyRequest struct {
	Algorithm string `json:"algorithm"`
}

type cimdKeyResponseBody struct {
	KID         string `json:"kid"`
	Algorithm   string `json:"algorithm"`
	IsCurrent   bool   `json:"is_current"`
	ActivatesAt string `json:"activates_at"`
	CreatedAt   string `json:"created_at"`
}

const (
	cimdKeyNotFoundError = "CIMD client key not found"
	cimdKeyLastError     = "last_key"
	cimdKeyCurrentError  = "current_key"
)

// Create generates an ES256 CIMD client-authentication key.
func (h *CIMDClientKeysHandler) Create(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.requireOperator(w, r)
	if !ok {
		return
	}

	var request cimdKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		h.writeError(w, http.StatusBadRequest, "invalid request body", "")
		h.audit(operator, "", "generate", "rejected")
		return
	}
	if request.Algorithm == "" {
		request.Algorithm = "ES256"
	}
	if request.Algorithm != "ES256" {
		h.writeError(w, http.StatusBadRequest, "unsupported algorithm", "algorithm must be ES256")
		h.audit(operator, "", "generate", "rejected")
		return
	}

	key, err := h.keys.GenerateKey(r.Context(), request.Algorithm)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		h.audit(operator, "", "generate", "rejected")
		return
	}
	h.audit(operator, key.KID, "generate", "success")
	h.writeJSON(w, http.StatusCreated, cimdKeyResponse(key))
}

// List returns active CIMD client-authentication key metadata.
func (h *CIMDClientKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	keys, err := h.keys.ListKeys(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	items := make([]cimdKeyResponseBody, 0, len(keys))
	for _, key := range keys {
		items = append(items, cimdKeyResponse(key))
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Promote makes an existing CIMD key immediately current.
func (h *CIMDClientKeysHandler) Promote(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.requireOperator(w, r)
	if !ok {
		return
	}
	kid := id.NewKeyID(chi.URLParam(r, "kid"))
	if kid.IsZero() {
		h.writeError(w, http.StatusBadRequest, "kid is required", "")
		h.audit(operator, kid, "promote", "rejected")
		return
	}
	key, err := h.keys.PromoteKey(r.Context(), kid)
	if err != nil {
		if ports.IsNotFoundErr(err) {
			h.writeError(w, http.StatusNotFound, cimdKeyNotFoundError, "")
		} else {
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		}
		h.audit(operator, kid, "promote", "rejected")
		return
	}
	h.audit(operator, key.KID, "promote", "success")
	h.writeJSON(w, http.StatusOK, cimdKeyResponse(key))
}

// Remove retires a non-signing CIMD client-authentication key.
func (h *CIMDClientKeysHandler) Remove(w http.ResponseWriter, r *http.Request) {
	operator, ok := h.requireOperator(w, r)
	if !ok {
		return
	}
	kid := id.NewKeyID(chi.URLParam(r, "kid"))
	if kid.IsZero() {
		h.writeError(w, http.StatusBadRequest, "kid is required", "")
		h.audit(operator, kid, "remove", "rejected")
		return
	}
	if err := h.keys.DeleteKey(r.Context(), kid); err != nil {
		switch {
		case ports.IsNotFoundErr(err):
			h.writeError(w, http.StatusNotFound, cimdKeyNotFoundError, "")
		case errors.Is(err, ports.ErrLastActiveKey):
			h.writeError(w, http.StatusConflict, cimdKeyLastError, "cannot remove the last CIMD client key")
		case errors.Is(err, ports.ErrCurrentKey), errors.Is(err, ports.ErrEffectiveCurrentKey):
			h.writeError(w, http.StatusConflict, cimdKeyCurrentError, "promote another CIMD client key before removal")
		default:
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		}
		h.audit(operator, kid, "remove", "rejected")
		return
	}
	h.audit(operator, kid, "remove", "success")
	w.WriteHeader(http.StatusNoContent)
}

func (h *CIMDClientKeysHandler) requireOperator(w http.ResponseWriter, r *http.Request) (string, bool) {
	operator, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "server_misconfiguration", "operator principal required for mutating CIMD key operations")
		return "", false
	}
	return operator, true
}

func (h *CIMDClientKeysHandler) audit(operator string, kid id.KeyID, operation, outcome string) {
	h.logger.Info("CIMD client key lifecycle", "operator_principal", operator, "key_id", kid, "operation", operation, "outcome", outcome)
}

func (h *CIMDClientKeysHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Warn("failed to encode CIMD client key response")
	}
}

func (h *CIMDClientKeysHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}

func cimdKeyResponse(key *storage.SigningKey) cimdKeyResponseBody {
	return cimdKeyResponseBody{
		KID:         key.KID.String(),
		Algorithm:   key.Algorithm,
		IsCurrent:   key.IsCurrent,
		ActivatesAt: key.ActivatesAt.Format(time.RFC3339),
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
	}
}
