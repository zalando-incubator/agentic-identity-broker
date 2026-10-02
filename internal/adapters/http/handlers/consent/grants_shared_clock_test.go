package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainconsent "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

type grantSharedTimeCoordinator struct {
	base ports.AuthorizationSessionCoordinator
	at   time.Time
}

func (c grantSharedTimeCoordinator) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	return c.base.Run(ctx, agentID, func(owner context.Context, _ time.Time) error { return operation(owner, c.at) })
}

func TestGrantHTTPUsesSharedDeadlineInsteadOfNodeClock(t *testing.T) {
	at := time.Date(2020, 6, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		deadline time.Time
		status   int
	}{
		{"future at shared time but past node time", at.Add(time.Hour), http.StatusCreated},
		{"expired at equality", at, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			agents, grants := memory.NewAgentRepository(), memory.NewUserGrantRepository()
			native := newGrantsIntegrationRefreshStore(agents, grants)
			sessions := memory.NewInMemoryUserSessionRepository()
			agentID, psID, serviceID := id.NewAgentID(), id.NewPermissionSetID(), id.NewServiceID()
			user := id.Principal("shared-clock@example.test")
			require.NoError(t, agents.Create(ctx, &storage.Agent{ID: agentID, DisplayName: "Shared clock", Description: "HTTP decision boundary", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeOptional}}}))
			seedActiveSession(t, sessions, user, serviceID)
			coordinator := grantSharedTimeCoordinator{base: native, at: at}
			service := domainconsent.NewService(agents, nil, grants, sessions, newPermissivePermissionSetQuerier(serviceID), slog.Default(), testAuthorizationClock{now: at}, coordinator, native)
			body, err := json.Marshal(GrantRequest{ValidUntil: &tc.deadline, GrantedPermissionSets: map[string][]string{psID.String(): {serviceID.String()}}})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/consent/agents/"+agentID.String()+"/grants", bytes.NewReader(body))
			route := chi.NewRouteContext()
			route.URLParams.Add("agent-id", agentID.String())
			req = req.WithContext(context.WithValue(principal.WithPrincipal(req.Context(), user.String()), chi.RouteCtxKey, route))
			response := httptest.NewRecorder()
			NewGrantsHandler(service, nil, newTestSessionTokenValidator()).CreateGrant(response, req)
			require.Equal(t, tc.status, response.Code)
			grant, err := grants.FindByPrincipalAndAgent(ctx, user, agentID)
			if tc.status == http.StatusCreated {
				require.NoError(t, err)
				require.Equal(t, tc.deadline, *grant.ValidUntil)
				require.Equal(t, at, grant.CreatedAt)
			} else {
				require.True(t, ports.IsNotFoundErr(err))
				require.Nil(t, grant)
			}
		})
	}
}
