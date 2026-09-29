package enduser

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Broker-managed keys are revision-checked on each request before this public
// cache age applies; external verifier caches remain outside broker control.
const jwksCacheMaxAgeSeconds = 300

// JWKSHandler serves the aggregated JWKS endpoint for public key discovery.
type JWKSHandler struct {
	publisher ports.JWKSPublisherPort
	logger    *slog.Logger
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewJWKSHandler creates a new JWKSHandler.
func NewJWKSHandler(
	publisher ports.JWKSPublisherPort,
	logger *slog.Logger,
) *JWKSHandler {
	return &JWKSHandler{
		publisher: publisher,
		logger:    logger,
	}
}

// ServeJWKS returns the aggregated JSON Web Key Set with public keys for token verification.
// GET /oauth2/jwks.json
func (h *JWKSHandler) ServeJWKS(w http.ResponseWriter, r *http.Request) {
	jwks, err := h.publisher.PublishJWKS(r.Context())
	if err != nil {
		if errors.Is(err, ports.ErrUpstreamUnavailable) {
			h.logger.Error("JWKS unavailable", "error", err)
			h.writeErrorJSON(w, http.StatusServiceUnavailable, "upstream key material temporarily unavailable")
			return
		}
		if errors.Is(err, ports.ErrKidConflict) {
			h.logger.Error("JWKS configuration conflict", "error", err)
			h.writeErrorJSON(w, http.StatusServiceUnavailable, "JWKS configuration conflict requires operator action")
			return
		}
		h.logger.Error("failed to build JWKS", "error", err)
		h.writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", jwksCacheMaxAgeSeconds))
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(jwks); err != nil {
		h.logger.Error("failed to encode JWKS response", "error", err)
	}
}

func (h *JWKSHandler) writeErrorJSON(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(errorResponse{Error: message}); err != nil {
		h.logger.Warn("failed to encode error response", "error", err)
	}
}
