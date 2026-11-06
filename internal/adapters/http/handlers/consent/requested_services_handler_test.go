package consent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const requestedScope = "https://broker.example.com/sessions/github"

func requestedSessionToken(t *testing.T, agentID id.AgentID, principalValue, originalURL string, serviceID id.ServiceID) string {
	t.Helper()
	claims, err := sessiontoken.NewAuthorizationSessionClaims(agentID, id.Principal(principalValue), originalURL, nil)
	require.NoError(t, err)
	claims.RequestedServiceIDs = []id.ServiceID{serviceID}
	claims.ServiceScopeBindings = map[string]id.ServiceID{requestedScope: serviceID}
	token, err := newTestJWETokenService().Encrypt(claims)
	require.NoError(t, err)
	return token
}

func routedRequest(method, path, agentID, principalValue string, body any) *http.Request {
	req := newRequestWithPrincipal(method, path, principalValue, body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// Consent API contract from specs/055-incremental-service-authorization (API-CONSENT; FR-009–FR-010).
func TestCreateGrantRequestedServices(t *testing.T) {
	agentID, serviceID := id.NewAgentID(), id.NewServiceID()
	originalURL := "/oauth2/authorize?client_id=" + agentID.String() + "&scope=" + url.QueryEscape("openid "+requestedScope)
	token := requestedSessionToken(t, agentID, "alice@example.com", originalURL, serviceID)
	path := "/api/consent/agents/" + agentID.String() + "/grants?session_token=" + url.QueryEscape(token)

	t.Run("successful Save returns the sealed request with the same session token", func(t *testing.T) {
		var received *consent.GrantRequest
		handler := NewGrantsHandler(&mockConsentService{grantConsentFunc: func(_ context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
			received = req
			return &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: "alice@example.com", CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		}}, nil, newTestSessionTokenValidator())
		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, routedRequest(http.MethodPost, path, agentID.String(), "alice@example.com", GrantRequest{GrantedPermissionSets: map[string][]string{}}))
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, originalURL+"&session_token="+url.QueryEscape(token), body["redirect_url"])
		require.NotNil(t, received)
		assert.Equal(t, []id.ServiceID{serviceID}, received.RequestedServiceIDs)
		assert.Equal(t, map[string]id.ServiceID{requestedScope: serviceID}, received.ServiceScopeBindings)
	})

	t.Run("an unsatisfied requested service returns the approved error without redirect", func(t *testing.T) {
		handler := NewGrantsHandler(&mockConsentService{grantConsentFunc: func(context.Context, *consent.GrantRequest) (*storage.UserGrant, error) {
			return nil, fmt.Errorf("%w: requested services must be selected, cover required scopes and have a usable session: %s", consent.ErrRequestedServiceNotSatisfied, serviceID)
		}}, nil, newTestSessionTokenValidator())
		rr := httptest.NewRecorder()
		handler.CreateGrant(rr, routedRequest(http.MethodPost, path, agentID.String(), "alice@example.com", GrantRequest{GrantedPermissionSets: map[string][]string{}}))
		require.Equal(t, http.StatusBadRequest, rr.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		assert.Equal(t, map[string]any{
			"error":   "requested service not satisfied",
			"message": "requested services must be selected, cover required scopes and have a usable session: " + serviceID.String(),
		}, body)
	})
}

func TestAgentDetailRequestedServices(t *testing.T) {
	agentID, serviceID := id.NewAgentID(), id.NewServiceID()
	readSet, triageSet := id.NewPermissionSetID(), id.NewPermissionSetID()
	mockService := &mockConsentService{
		getAgentConsentDetailFunc: func(context.Context, id.AgentID, id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{Agent: &storage.Agent{ID: agentID, DisplayName: "Data Analysis Assistant"}}, nil
		},
		requestedServiceCoverageFunc: func(_ context.Context, _ id.Principal, _ id.AgentID, ids []id.ServiceID) ([]servicescope.Outcome, error) {
			require.Equal(t, []id.ServiceID{serviceID}, ids)
			return []servicescope.Outcome{{
				ServiceID: serviceID, RequiredScopes: []string{"repo", "user:email"}, Baseline: servicescope.UsabilityUnusableDefinitive,
				Candidates: []servicescope.Candidate{
					{PermissionSetID: readSet, SuppliedRequiredScopes: []string{"repo", "user:email"}, Usability: servicescope.UsabilityUnverifiableTransient},
					{PermissionSetID: triageSet, SuppliedRequiredScopes: []string{"repo"}, Usability: servicescope.UsabilityMissing},
				},
			}}, nil
		},
	}
	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
	detail := func(path string) map[string]any {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", agentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		req = req.WithContext(principal.WithPrincipal(req.Context(), "alice@example.com"))
		rr := httptest.NewRecorder()
		handler.GetAgentDetail(rr, req)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var body struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
		return body.Data
	}

	data := detail("/api/consent/agents/" + agentID.String())
	assert.Equal(t, []any{}, data["requested_service_ids"], "arrays are always present")
	assert.Equal(t, []any{}, data["requested_service_coverage"])

	token := requestedSessionToken(t, agentID, "alice@example.com", "/oauth2/authorize?client_id=x", serviceID)
	data = detail("/api/consent/agents/" + agentID.String() + "?session_token=" + url.QueryEscape(token))
	assert.Equal(t, []any{serviceID.String()}, data["requested_service_ids"])
	assert.Equal(t, []any{map[string]any{
		"service_id": serviceID.String(), "required_scopes": []any{"repo", "user:email"}, "usability": "unusable",
		"covering_permission_sets": []any{
			map[string]any{"permission_set_id": readSet.String(), "supplied_required_scopes": []any{"repo", "user:email"}, "usability": "unavailable"},
			map[string]any{"permission_set_id": triageSet.String(), "supplied_required_scopes": []any{"repo"}, "usability": "missing"},
		},
	}}, data["requested_service_coverage"])
}
