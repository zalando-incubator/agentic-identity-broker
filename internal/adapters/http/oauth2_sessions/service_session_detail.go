package oauth2_sessions

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

type serviceDetailService struct {
	ID          string  `json:"id"`
	CanonicalID *string `json:"canonical_id"`
	DisplayName string  `json:"display_name"`
	ScopeURL    string  `json:"scope_url"`
}

type serviceDetailSession struct {
	SessionID             string     `json:"session_id"`
	InitiatedAt           time.Time  `json:"initiated_at"`
	AccessTokenExpiresAt  *time.Time `json:"access_token_expires_at,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refresh_token_expires_at,omitempty"`
	IsExpired             bool       `json:"is_expired"`
	HasRefreshToken       bool       `json:"has_refresh_token"`
	Scope                 []string   `json:"scope"`
	DependentAgentCount   int        `json:"dependent_agent_count"`
	TokensEncrypted       bool       `json:"tokens_encrypted"`
}

type serviceDetailAgent struct {
	AgentID     string `json:"agent_id"`
	DisplayName string `json:"display_name"`
}

type serviceDetailData struct {
	Service         serviceDetailService  `json:"service"`
	Session         *serviceDetailSession `json:"session"`
	DependentAgents []serviceDetailAgent  `json:"dependent_agents"`
}

func toServiceDetailData(detail *oauth2session.ServiceSessionDetail) serviceDetailData {
	data := serviceDetailData{
		Service: serviceDetailService{
			ID:          detail.Service.ID.String(),
			CanonicalID: detail.Service.CanonicalID,
			DisplayName: detail.Service.DisplayName,
			ScopeURL:    detail.ScopeURL,
		},
		DependentAgents: make([]serviceDetailAgent, len(detail.DependentAgents)),
	}
	for i, agent := range detail.DependentAgents {
		data.DependentAgents[i] = serviceDetailAgent{AgentID: agent.ID.String(), DisplayName: agent.DisplayName}
	}
	if session := detail.Session; session != nil {
		scope := session.Scope
		if scope == nil {
			scope = []string{}
		}
		data.Session = &serviceDetailSession{
			SessionID:             session.ID.String(),
			InitiatedAt:           session.InitiatedAt,
			AccessTokenExpiresAt:  session.AccessTokenExpiresAt,
			RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
			IsExpired:             session.IsExpired(),
			HasRefreshToken:       len(session.EncryptedRefreshToken) > 0,
			Scope:                 scope,
			DependentAgentCount:   len(detail.DependentAgents),
			TokensEncrypted:       true,
		}
	}
	return data
}

// GetServiceSessionDetail handles GET /api/third-party/sessions/{service}, where {service} is
// a current canonical ID or UUID. A known service without a session returns session: null.
func (h *Handler) GetServiceSessionDetail(w http.ResponseWriter, r *http.Request) {
	principalValue, ok := principal.FromContext(r.Context())
	if !ok || principalValue == "" {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "principal not found in context")
		return
	}
	detail, err := h.service.GetServiceSessionDetail(r.Context(), id.Principal(principalValue), chi.URLParam(r, "service"))
	if err != nil {
		if errors.Is(err, oauth2session.ErrServiceNotFound) {
			writeJSONError(w, http.StatusNotFound, "not_found", "service not found")
			return
		}
		h.logger.ErrorContext(r.Context(), "failed to get service session detail", "principal", principalValue, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve service session detail")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": toServiceDetailData(detail)})
}
