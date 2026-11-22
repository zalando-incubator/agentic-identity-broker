package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestConsentRejectsInvalidSessionToken(t *testing.T) {
	agentID := id.NewAgentID()
	user := "user@example.com"
	token := url.QueryEscape(newTestSessionToken(newTestJWETokenService(), agentID, user, "/oauth2/authorize"))
	for _, tc := range []struct{ name, query string }{
		{"invalid", "session_token=synthetic-invalid-JWE"},
		{"empty", "session_token="}, {"bare", "session_token"},
		{"repeated", "session_token=" + token + "&session_token=" + token},
		{"trailing_empty", "session_token=" + token + "&session_token="},
		{"leading_empty", "session_token=&session_token=" + token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				t.Run(method, func(t *testing.T) {
					var logs bytes.Buffer
					logger := slog.New(slog.NewJSONHandler(&logs, nil))
					detail := NewAgentDetailHandler(&mockAgentDetailService{
						getAgentConsentDetailFunc: func(context.Context, id.AgentID, id.Principal) (*consent.AgentConsentDetail, error) {
							return &consent.AgentConsentDetail{Agent: &storage.Agent{ID: agentID}}, nil
						},
					}, logger, newTestSessionTokenValidator())
					grant := NewGrantsHandler(&mockConsentService{
						grantConsentFunc: func(context.Context, *consent.GrantRequest) (*storage.UserGrant, error) {
							t.Fatal("invalid continuation must not mutate a grant")
							return nil, nil
						},
					}, logger, newTestSessionTokenValidator())
					req := newRequestWithPrincipal(method, "/api/consent/agents/"+agentID.String()+"?"+tc.query, user, GrantRequest{GrantedPermissionSets: map[string][]string{}})
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("agent-id", agentID.String())
					req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
					req = req.WithContext(principal.WithPrincipal(req.Context(), id.Principal(user)))
					rr := httptest.NewRecorder()
					if method == http.MethodGet {
						detail.GetAgentDetail(rr, req)
					} else {
						grant.CreateGrant(rr, req)
					}
					require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
					var body ErrorResponse
					require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
					assert.Equal(t, "invalid_token", body.Error)
					logOutput := logs.String()
					var record map[string]any
					require.NoError(t, json.NewDecoder(&logs).Decode(&record))
					assert.Equal(t, "WARN", record["level"])
					assert.Equal(t, "consent.authorization_session.rejected", record["event"])
					assert.Equal(t, user, record["principal"])
					assert.Equal(t, agentID.String(), record["agent_id"])
					assert.Equal(t, "invalid_authorization_session", record["reason"])
					assert.NotContains(t, logOutput, token)
					assert.NotContains(t, logOutput, "synthetic-invalid-JWE")
				})
			}
		})
	}
}

func TestConsentAuditsAuthorizationSessionRejections(t *testing.T) {
	agentID := id.NewAgentID()
	user := "user@example.com"
	tokens := newTestJWETokenService()
	for _, tc := range []struct {
		name, token, reason string
		detailStatus        int
		detailError         string
		grantStatus         int
		grantError          string
	}{
		{"expired", newExpiredTestSessionToken(tokens, agentID, user), "expired_authorization_session",
			http.StatusBadRequest, "session_expired", http.StatusBadRequest, "session_expired"},
		{"agent_mismatch", newTestSessionToken(tokens, id.NewAgentID(), user, "/oauth2/authorize"), "agent_mismatch",
			http.StatusBadRequest, "bad request", http.StatusBadRequest, "bad request"},
		{"principal_mismatch", newTestSessionToken(tokens, agentID, "other@example.com", "/oauth2/authorize"), "principal_mismatch",
			http.StatusBadRequest, "bad request", http.StatusForbidden, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				t.Run(method, func(t *testing.T) {
					var logs bytes.Buffer
					logger := slog.New(slog.NewJSONHandler(&logs, nil))
					detail := NewAgentDetailHandler(&mockAgentDetailService{
						getAgentConsentDetailFunc: func(context.Context, id.AgentID, id.Principal) (*consent.AgentConsentDetail, error) {
							return &consent.AgentConsentDetail{Agent: &storage.Agent{ID: agentID}}, nil
						},
					}, logger, newTestSessionTokenValidator())
					grant := NewGrantsHandler(&mockConsentService{
						grantConsentFunc: func(context.Context, *consent.GrantRequest) (*storage.UserGrant, error) {
							t.Fatal("rejected authorization session must not mutate a grant")
							return nil, nil
						},
					}, logger, newTestSessionTokenValidator())
					req := newRequestWithPrincipal(method, "/api/consent/agents/"+agentID.String()+"?session_token="+url.QueryEscape(tc.token), user, GrantRequest{GrantedPermissionSets: map[string][]string{}})
					rctx := chi.NewRouteContext()
					rctx.URLParams.Add("agent-id", agentID.String())
					req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
					rr := httptest.NewRecorder()
					wantStatus, wantError := tc.detailStatus, tc.detailError
					if method == http.MethodGet {
						detail.GetAgentDetail(rr, req)
					} else {
						grant.CreateGrant(rr, req)
						wantStatus, wantError = tc.grantStatus, tc.grantError
					}
					require.Equal(t, wantStatus, rr.Code, rr.Body.String())
					var body ErrorResponse
					require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
					assert.Equal(t, wantError, body.Error)
					logOutput := logs.String()
					var record map[string]any
					require.NoError(t, json.NewDecoder(&logs).Decode(&record))
					assert.Equal(t, "WARN", record["level"])
					assert.Equal(t, "consent.authorization_session.rejected", record["event"])
					assert.Equal(t, user, record["principal"])
					assert.Equal(t, agentID.String(), record["agent_id"])
					assert.Equal(t, tc.reason, record["reason"])
					assert.NotContains(t, logOutput, tc.token)
				})
			}
		})
	}
}
