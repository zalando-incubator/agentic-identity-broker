package approval

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

var errNullParamsPattern = errors.New("params_pattern must be an object")

type approveRequest struct {
	Persistence           string          `json:"persistence"`
	DisallowedToolPattern json.RawMessage `json:"tool_pattern"`
	ParamsPattern         json.RawMessage `json:"params_pattern"`
}

type approveResponse struct {
	Data approveResponseData `json:"data"`
}

type approveResponseData struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Persistence string `json:"persistence"`
	ApprovedAt  string `json:"approved_at"`
}

// ApproveHandler handles POST /api/approvals/{id}/approve.
type ApproveHandler struct {
	service *domainapproval.Service
}

// NewApproveHandler creates a new ApproveHandler.
func NewApproveHandler(service *domainapproval.Service) *ApproveHandler {
	return &ApproveHandler{service: service}
}

func (h *ApproveHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req approveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "request body must be valid JSON")
		return
	}

	persistence := storage.ApprovalPersistence(req.Persistence)
	if !persistence.IsValid() {
		writeError(w, http.StatusBadRequest, "bad_request", "persistence must be one of: once, session, permanent")
		return
	}

	if len(req.DisallowedToolPattern) > 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "tool_pattern is not allowed")
		return
	}
	paramsPattern, err := decodeParamsPattern(req.ParamsPattern)
	if err != nil {
		if errors.Is(err, errNullParamsPattern) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_pattern", err.Error())
		} else {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}

	result, err := h.service.ApproveApproval(r.Context(), approvalID, id.Principal(principalValue), domainapproval.ApproveRequest{Persistence: persistence, ParamsPattern: paramsPattern})
	if err != nil {
		if errors.Is(err, domainapproval.ErrApprovalInvalidPattern) {
			writeError(w, http.StatusUnprocessableEntity, "invalid_pattern", err.Error())
			return
		}
		mapDomainError(w, err)
		return
	}

	if result.Persistence == nil || result.ApprovedAt == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "approved approval is missing required fields")
		return
	}

	resp := approveResponse{
		Data: approveResponseData{
			ID:          result.ID.String(),
			Status:      string(result.Status),
			Persistence: string(*result.Persistence),
			ApprovedAt:  result.ApprovedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func decodeParamsPattern(rawParamsPattern json.RawMessage) (map[string]string, error) {
	if len(rawParamsPattern) == 0 {
		return nil, nil
	}
	rawParamsPattern = bytes.TrimSpace(rawParamsPattern)
	if bytes.Equal(rawParamsPattern, []byte("null")) {
		return nil, errNullParamsPattern
	}
	paramsPattern := make(map[string]string)
	if err := json.Unmarshal(rawParamsPattern, &paramsPattern); err != nil {
		return nil, errors.New("params_pattern must be an object")
	}
	return paramsPattern, nil
}
