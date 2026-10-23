package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/requestbody"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// SigningKeysHandler handles admin API requests for signing key management.
type SigningKeysHandler struct {
	signingKeyService ports.SigningKeyManager
	logger            *slog.Logger
}

// NewSigningKeysHandler creates a new SigningKeysHandler.
func NewSigningKeysHandler(
	signingKeyService ports.SigningKeyManager,
	logger *slog.Logger,
) *SigningKeysHandler {
	return &SigningKeysHandler{
		signingKeyService: signingKeyService,
		logger:            logger,
	}
}

type signingKeyResponse struct {
	KID         string  `json:"kid"`
	Algorithm   string  `json:"algorithm"`
	IsCurrent   bool    `json:"is_current"`
	ActivatesAt string  `json:"activates_at"`
	CreatedAt   string  `json:"created_at"`
	RemovedAt   *string `json:"removed_at,omitempty"`
}

type signingKeyAddRequest struct {
	Algorithm string `json:"algorithm"`
}

const (
	signingKeyDeleteLastKeyError   = "last_key"
	signingKeyDeleteLastKeyMessage = "cannot delete the last remaining signing key"

	signingKeyDeleteCurrentKeyError            = "current_key"
	signingKeyDeleteCurrentKeyMessage          = "promote another signing key before removing the current key"
	signingKeyDeleteEffectiveCurrentKeyMessage = "wait for the promoted signing key to activate or promote a different key before removing the key still signing tokens"

	signingKeyOperatorPrincipalError   = "server_misconfiguration"
	signingKeyOperatorPrincipalMessage = "operator principal required for mutating signing key operations"
)

// Add generates and stores a new signing key.
// POST /api/oauth2-server/signing-keys
func (h *SigningKeysHandler) Add(w http.ResponseWriter, r *http.Request) {
	operatorPrincipal, ok := h.requireOperatorPrincipal(w, r.Context())
	if !ok {
		return
	}

	var req signingKeyAddRequest
	if err := requestbody.Decode(w, r, 4<<10, &req); err != nil {
		if errors.Is(err, io.EOF) {
			req.Algorithm = "ES256"
		} else {
			h.writeError(w, requestbody.Status(err), "invalid request body", "")
			return
		}
	}
	if req.Algorithm == "" {
		req.Algorithm = "ES256"
	}
	if req.Algorithm != "ES256" {
		h.writeError(w, http.StatusBadRequest, "unsupported algorithm", "algorithm must be ES256")
		return
	}

	key, err := h.signingKeyService.GenerateAndStoreKey(r.Context(), req.Algorithm, true)
	if err != nil {
		h.logger.Error("failed to add signing key", "operator_principal", operatorPrincipal, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	resp := signingKeyResponse{
		KID:         key.KID.String(),
		Algorithm:   key.Algorithm,
		IsCurrent:   key.IsCurrent,
		ActivatesAt: key.ActivatesAt.Format(time.RFC3339),
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
	}

	h.logger.Info("signing key added", "operator_principal", operatorPrincipal, "kid", key.KID, "algorithm", key.Algorithm, "activates_at", key.ActivatesAt)
	h.writeJSON(w, http.StatusCreated, resp)
}

// List returns all active signing keys (without private material).
// GET /api/oauth2-server/signing-keys
func (h *SigningKeysHandler) List(w http.ResponseWriter, r *http.Request) {
	operatorPrincipal, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		operatorPrincipal = "unknown"
	}

	keys, err := h.signingKeyService.ListKeys(r.Context())
	if err != nil {
		h.logger.Error("failed to list signing keys", "operator_principal", operatorPrincipal, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	items := make([]signingKeyResponse, 0, len(keys))
	for _, key := range keys {
		items = append(items, signingKeyResponse{
			KID:         key.KID.String(),
			Algorithm:   key.Algorithm,
			IsCurrent:   key.IsCurrent,
			ActivatesAt: key.ActivatesAt.Format(time.RFC3339),
			CreatedAt:   key.CreatedAt.Format(time.RFC3339),
		})
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": items,
	})
}

// SetCurrent promotes a signing key to be the current key.
// PUT /api/oauth2-server/signing-keys/{kid}/current
func (h *SigningKeysHandler) SetCurrent(w http.ResponseWriter, r *http.Request) {
	operatorPrincipal, ok := h.requireOperatorPrincipal(w, r.Context())
	if !ok {
		return
	}
	kid := chi.URLParam(r, "kid")
	if kid == "" {
		h.writeError(w, http.StatusBadRequest, "kid is required", "")
		return
	}

	key, err := h.signingKeyService.PromoteKey(r.Context(), id.NewKeyID(kid))
	if err != nil {
		if ports.IsNotFoundErr(err) {
			h.writeError(w, http.StatusNotFound, "signing key not found", "")
			return
		}
		h.logger.Error("failed to set current key", "operator_principal", operatorPrincipal, "kid", kid, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	resp := signingKeyResponse{
		KID:         key.KID.String(),
		Algorithm:   key.Algorithm,
		IsCurrent:   key.IsCurrent,
		ActivatesAt: key.ActivatesAt.Format(time.RFC3339),
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
	}

	h.logger.Info("signing key promoted to current", "operator_principal", operatorPrincipal, "kid", kid)
	h.writeJSON(w, http.StatusOK, resp)
}

// Remove soft-deletes a signing key.
// DELETE /api/oauth2-server/signing-keys/{kid}
func (h *SigningKeysHandler) Remove(w http.ResponseWriter, r *http.Request) {
	operatorPrincipal, ok := h.requireOperatorPrincipal(w, r.Context())
	if !ok {
		return
	}
	kid := chi.URLParam(r, "kid")
	if kid == "" {
		h.writeError(w, http.StatusBadRequest, "kid is required", "")
		return
	}

	if err := h.signingKeyService.DeleteKey(r.Context(), id.NewKeyID(kid)); err != nil {
		switch {
		case ports.IsNotFoundErr(err):
			h.writeError(w, http.StatusNotFound, "signing key not found", "")
		case errors.Is(err, ports.ErrLastActiveKey):
			h.writeError(w, http.StatusConflict, signingKeyDeleteLastKeyError, signingKeyDeleteLastKeyMessage)
		case errors.Is(err, ports.ErrCurrentKey):
			h.writeError(w, http.StatusConflict, signingKeyDeleteCurrentKeyError, signingKeyDeleteCurrentKeyMessage)
		case errors.Is(err, ports.ErrEffectiveCurrentKey):
			h.writeError(w, http.StatusConflict, signingKeyDeleteCurrentKeyError, signingKeyDeleteEffectiveCurrentKeyMessage)
		default:
			h.logger.Error("failed to remove signing key", "operator_principal", operatorPrincipal, "kid", kid, "error", err)
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		}
		return
	}

	h.logger.Info("signing key removed", "operator_principal", operatorPrincipal, "kid", kid)
	w.WriteHeader(http.StatusNoContent)
}

func (h *SigningKeysHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *SigningKeysHandler) writeError(w http.ResponseWriter, statusCode int, errMsg string, message string) {
	resp := ErrorResponse{
		Error:   errMsg,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}

func (h *SigningKeysHandler) requireOperatorPrincipal(w http.ResponseWriter, ctx context.Context) (string, bool) {
	operatorPrincipal, ok := operatorPrincipalFromContext(ctx, h.logger)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, signingKeyOperatorPrincipalError, signingKeyOperatorPrincipalMessage)
		return "", false
	}
	return operatorPrincipal, true
}

func operatorPrincipalFromContext(ctx context.Context, logger *slog.Logger) (string, bool) {
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		if logger != nil {
			logger.Warn("operator principal missing from context; audit trail incomplete")
		}
		return "", false
	}
	return principalValue, true
}
