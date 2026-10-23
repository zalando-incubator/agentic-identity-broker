package approval

import (
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/requestbody"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
)

type denyRequest struct {
	Persistence *string `json:"persistence,omitempty"`
}

type denyResponse struct {
	Data denyResponseData `json:"data"`
}

type denyResponseData struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	Persistence *string `json:"persistence,omitempty"`
	DeniedAt    string  `json:"denied_at"`
}

// DenyHandler handles POST /api/approvals/{id}/deny.
type DenyHandler struct {
	service *domainapproval.Service
}

// NewDenyHandler creates a new DenyHandler.
func NewDenyHandler(service *domainapproval.Service) *DenyHandler {
	return &DenyHandler{service: service}
}

func (h *DenyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	var req denyRequest
	body, err := requestbody.Read(w, r, 4<<10)
	if err != nil {
		writeError(w, requestbody.Status(err), "bad_request", "request body must be valid JSON")
		return
	}
	if len(body) > 0 {
		if err := requestbody.Unmarshal(body, &req); err != nil {
			writeError(w, requestbody.Status(err), "bad_request", "request body must be valid JSON")
			return
		}
	}

	var persistence *storage.ApprovalPersistence
	if req.Persistence != nil {
		p := storage.ApprovalPersistence(*req.Persistence)
		if p != storage.ApprovalPersistencePermanent {
			writeError(w, http.StatusBadRequest, "bad_request", "persistence for deny must be \"permanent\" if provided")
			return
		}
		persistence = &p
	}

	result, err := h.service.DenyApproval(r.Context(), approvalID, id.Principal(principalValue), persistence)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	if result.DeniedAt == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "denied approval is missing denied_at")
		return
	}

	respData := denyResponseData{
		ID:       result.ID.String(),
		Status:   string(result.Status),
		DeniedAt: result.DeniedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if result.Persistence != nil {
		s := string(*result.Persistence)
		respData.Persistence = &s
	}
	writeJSON(w, http.StatusOK, denyResponse{Data: respData})
}
