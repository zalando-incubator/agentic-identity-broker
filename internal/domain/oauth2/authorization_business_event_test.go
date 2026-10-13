package oauth2

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

func TestAuthorizationLedgerAcceptedRequestCommitsBeforeConsentResponse(t *testing.T) {
	agents := NewMockAgentRepository()
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent", RedirectURIs: []string{"https://client.example/callback"}}
	clientID := id.ClientID("upstream-client")
	agent.ClientID = &clientID
	require.NoError(t, agents.Create(context.Background(), agent))
	svc := newTestAuthorizationService(agents, NewMockGrantRepository(), &OAuth2Config{PublicURL: "https://broker.example", ModeStrategy: NewProxyModeStrategy()}).(*AuthorizationService)
	store := &ledgerfixture.Store{}
	svc.ledger = store.Recorder(t)
	request := &ports.AuthorizationRequest{ClientID: id.ClientID(agent.ID.String()), RedirectURI: agent.RedirectURIs[0], ResponseType: "code", OriginalURL: "https://broker.example/oauth2/authorize?response_type=code"}
	decision, err := svc.HandleAuthorization(context.Background(), request, "ledger-user")
	require.NoError(t, err)
	require.Equal(t, "redirect_to_consent", decision.Action)
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"authorization-requested", store.Events[0].Type)
	require.Equal(t, model.BusinessEventPending, store.Events[0].Outcome)
	require.Equal(t, agent.ID, store.Events[0].AgentID)
	require.Equal(t, id.Principal("ledger-user"), *store.Events[0].Subject)
	request.RedirectURI = "https://unregistered.example/callback"
	_, err = svc.HandleAuthorization(context.Background(), request, "ledger-user")
	require.NoError(t, err)
	require.Len(t, store.Events, 1, "an invalid redirect is not an accepted authorization request")
	request.RedirectURI = agent.RedirectURIs[0]
	store.AppendError = errors.New("ledger unavailable")
	decision, err = svc.HandleAuthorization(context.Background(), request, "ledger-user")
	require.NoError(t, err)
	require.Equal(t, "error", decision.Action)
	require.Equal(t, "server_error", decision.ErrorCode)
	require.NotContains(t, decision.RedirectURL, "session_token=")
}
