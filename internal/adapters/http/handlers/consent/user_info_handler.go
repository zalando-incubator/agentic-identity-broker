// Package consent provides HTTP handlers for consent management APIs.
package consent

import (
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// UserInfoHandler handles HTTP requests for retrieving user information.
// Implements GET /api/me endpoint.
type UserInfoHandler struct {
	logger *slog.Logger
}

// NewUserInfoHandler creates a new user info handler.
func NewUserInfoHandler(logger *slog.Logger) *UserInfoHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserInfoHandler{
		logger: logger,
	}
}

// GetUserInfoResponse represents the response for GET /api/me.
type GetUserInfoResponse struct {
	Data consent.UserInfo `json:"data"`
}

// GetUserInfo handles GET /api/me
// Returns information about the currently authenticated user.
// When a PrincipalProfile is available (from JWT pre-auth), returns enriched user info
// with display name, email, and picture URL. Falls back to principal-only info for
// plain header pre-auth (backward-compatible).
//
// Response codes:
// - 200 OK: Returns user info
// - 401 Unauthorized: No principal in context
func (h *UserInfoHandler) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Try enriched profile first (set by JWT pre-auth middleware)
	if profile, ok := principal.ProfileFromContext(ctx); ok && profile.Principal() != "" {
		userInfo := consent.UserInfo{
			Principal:   profile.Principal(),
			DisplayName: profile.DisplayName(),
			Email:       profile.Email(),
			PictureURL:  profile.PictureURL(),
		}

		h.logger.InfoContext(ctx, "user info retrieved from profile",
			"principal", profile.Principal(),
			"has_email", profile.Email() != nil,
			"has_picture_url", profile.PictureURL() != nil)

		response := GetUserInfoResponse{
			Data: userInfo,
		}

		h.writeJSON(w, http.StatusOK, response)
		return
	}

	// Fall back to plain principal (backward-compatible)
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.WarnContext(ctx, "principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	userInfo := consent.UserInfo{
		Principal:   principalValue,
		DisplayName: principalValue.String(),
		PictureURL:  nil,
	}

	h.logger.InfoContext(ctx, "user info retrieved",
		"principal", principalValue)

	response := GetUserInfoResponse{
		Data: userInfo,
	}

	h.writeJSON(w, http.StatusOK, response)
}

// writeJSON writes a JSON response.
func (h *UserInfoHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	if err := writeBufferedJSON(w, statusCode, data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *UserInfoHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
