package oauth2_sessions

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionReadContracts_PrincipalIsolation(t *testing.T) {
	ctx := context.Background()
	owner := id.Principal("owner@example.com")
	other := id.Principal("other@example.com")
	serviceID := id.NewServiceID()
	emptyServiceID := id.NewServiceID()
	ownerPermissionSetID := id.NewPermissionSetID()
	otherPermissionSetID := id.NewPermissionSetID()
	sessions := memory.NewInMemoryUserSessionRepository()
	grants := memory.NewUserGrantRepository()
	agents := memory.NewAgentRepository()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := oauth2session.NewOAuth2SessionService(nil, sessions, sessions, grants, agents, nil, nil, nil, oauth2session.DefaultConfig(), logger)
	handler := NewHandler(service)
	handler.logger = logger
	router := chi.NewRouter()
	router.Route("/api", handler.RegisterRoutes)

	for _, session := range []*storage.UserSession{
		{ID: id.NewSessionID(), Principal: owner, ServiceID: serviceID, EncryptedAccessToken: []byte("owner-token"), TokenType: "Bearer", InitiatedAt: time.Now(), CreatedAt: time.Now()},
		{ID: id.NewSessionID(), Principal: other, ServiceID: serviceID, EncryptedAccessToken: []byte("other-token"), TokenType: "Bearer", InitiatedAt: time.Now(), CreatedAt: time.Now()},
		{ID: id.NewSessionID(), Principal: owner, ServiceID: emptyServiceID, EncryptedAccessToken: []byte("empty-token"), TokenType: "Bearer", InitiatedAt: time.Now(), CreatedAt: time.Now()},
	} {
		require.NoError(t, sessions.Create(ctx, session))
	}
	ownerAgent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Owner agent", Description: "Owner delegation",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: ownerPermissionSetID, RequirementType: storage.RequirementTypeMandatory}},
	}
	otherAgent := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "Other agent", Description: "Other delegation",
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: otherPermissionSetID, RequirementType: storage.RequirementTypeMandatory}},
	}
	for _, agent := range []*storage.Agent{ownerAgent, otherAgent} {
		require.NoError(t, agents.Create(ctx, agent))
	}
	for _, grant := range []*storage.UserGrant{
		{Principal: owner, AgentID: ownerAgent.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: ownerPermissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}}},
		{Principal: other, AgentID: otherAgent.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: otherPermissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}}},
	} {
		require.NoError(t, grants.Create(ctx, grant))
	}

	for _, suffix := range []string{"", "/affected-agents"} {
		t.Run("session"+suffix, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/third-party/"+serviceID.String()+"/session"+suffix, nil)
			request = request.WithContext(principal.WithPrincipal(ctx, owner.String()))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var body struct {
				Data struct {
					Session         *storage.UserSession      `json:"session"`
					DependentAgents []oauth2session.AgentInfo `json:"dependent_agents"`
					AffectedAgents  []struct {
						AgentID     string `json:"agent_id"`
						DisplayName string `json:"display_name"`
					} `json:"affected_agents"`
				} `json:"data"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			if suffix == "" {
				require.NotNil(t, body.Data.Session)
				assert.Equal(t, owner, body.Data.Session.Principal)
				require.Len(t, body.Data.DependentAgents, 1)
				assert.Equal(t, ownerAgent.ID, body.Data.DependentAgents[0].ID)
				assert.Equal(t, ownerAgent.DisplayName, body.Data.DependentAgents[0].DisplayName)
			} else {
				assert.Nil(t, body.Data.Session)
				require.Len(t, body.Data.AffectedAgents, 1)
				assert.Equal(t, ownerAgent.ID.String(), body.Data.AffectedAgents[0].AgentID)
				assert.Equal(t, ownerAgent.DisplayName, body.Data.AffectedAgents[0].DisplayName)
			}
			assert.NotContains(t, response.Body.String(), otherAgent.DisplayName)
			assert.NotContains(t, response.Body.String(), "owner-token")
		})
	}

	t.Run("empty affected agents stays an array", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/third-party/"+emptyServiceID.String()+"/session/affected-agents", nil)
		request = request.WithContext(principal.WithPrincipal(ctx, owner.String()))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, `{"data":{"affected_agents":[]}}`, response.Body.String())
	})

	for _, test := range []struct {
		name, serviceID, actingPrincipal, errorCode string
		status                                      int
	}{
		{"authentication required", serviceID.String(), "", "unauthorized", http.StatusUnauthorized},
		{"invalid service ID", "not-a-uuid", owner.String(), "invalid_request", http.StatusBadRequest},
		{"session absent", id.NewServiceID().String(), owner.String(), "not_found", http.StatusNotFound},
		{"other user's session is not visible", emptyServiceID.String(), other.String(), "not_found", http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/third-party/"+test.serviceID+"/session/affected-agents", nil)
			if test.actingPrincipal != "" {
				request = request.WithContext(principal.WithPrincipal(ctx, test.actingPrincipal))
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, test.status, response.Code, response.Body.String())
			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			assert.Equal(t, test.errorCode, body.Error)
		})
	}
}
