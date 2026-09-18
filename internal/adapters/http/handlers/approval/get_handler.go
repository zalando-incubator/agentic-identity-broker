package approval

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/go-chi/chi/v5"
)

// GetHandler handles GET /api/approvals/{id}.
type GetHandler struct {
	service *approval.Service
}

// NewGetHandler creates a new GetHandler.
func NewGetHandler(service *approval.Service) *GetHandler {
	return &GetHandler{service: service}
}

func (h *GetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principalValue, ok := principal.FromContext(r.Context())
	if !ok || principalValue == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}

	rawID := chi.URLParam(r, "id")
	approvalID, err := id.ParseApprovalID(rawID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "approval ID must be a valid UUID")
		return
	}

	result, err := h.service.GetApproval(r.Context(), approvalID, id.Principal(principalValue))
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		return
	}
}

type errorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, statusCode int, errCode string, message string) {
	writeJSON(w, statusCode, errorResponse{Error: errCode, Message: message})
}

func mapDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, approval.ErrApprovalNotFound):
		writeError(w, http.StatusNotFound, "not_found", "approval not found")
	case errors.Is(err, approval.ErrApprovalForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "you do not have access to this approval")
	case errors.Is(err, approval.ErrApprovalGone):
		writeError(w, http.StatusGone, "gone", "approval has expired")
	case errors.Is(err, approval.ErrApprovalNotPending):
		writeError(w, http.StatusConflict, "conflict", "approval has already been resolved")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}
