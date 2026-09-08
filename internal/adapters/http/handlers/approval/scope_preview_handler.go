package approval

import (
	"encoding/json"
	"errors"
	"net/http"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/go-chi/chi/v5"
)

type scopePreviewRequest struct {
	ToolPattern   *string         `json:"tool_pattern"`
	ParamsPattern json.RawMessage `json:"params_pattern"`
}

type scopePreviewResponse struct {
	Data *domainapproval.ScopePreview `json:"data"`
}

type ScopePreviewHandler struct {
	service *domainapproval.Service
}

func NewScopePreviewHandler(service *domainapproval.Service) *ScopePreviewHandler {
	return &ScopePreviewHandler{service: service}
}

func (h *ScopePreviewHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principalValue, ok := principal.FromContext(r.Context())
	if !ok || principalValue == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	approvalID, err := id.ParseApprovalID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "approval ID must be a valid UUID")
		return
	}
	var req scopePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid JSON")
		return
	}
	toolPattern, paramsPattern, err := decodePatternFields(req.ToolPattern, req.ParamsPattern)
	if err != nil {
		if errors.Is(err, errNullParamsPattern) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_pattern", err.Error())
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	preview, err := h.service.PreviewApprovalScope(r.Context(), approvalID, id.Principal(principalValue), domainapproval.ApproveRequest{ToolPattern: toolPattern, ParamsPattern: paramsPattern})
	if err != nil {
		if errors.Is(err, domainapproval.ErrApprovalInvalidPattern) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_pattern", err.Error())
			return
		}
		mapDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scopePreviewResponse{Data: preview})
}
