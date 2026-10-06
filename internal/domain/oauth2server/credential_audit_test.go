package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func refreshTransitionEvents(t *testing.T, logs *bytes.Buffer, event string) []map[string]any {
	t.Helper()
	var found []map[string]any
	decoder := json.NewDecoder(logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["event"] == event {
			found = append(found, record)
		}
	}
	return found
}

func TestCredentialRevocationAudit_EveryCommittedRootHasIdentityAndContext(t *testing.T) {
	f := newProviderRefreshFixture(t)
	_, secondRoot := issueCredentialLifecycleForPrincipal(t, f, id.NewPrincipal("second@example.test"))
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	service := NewCredentialService(f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth, logger,
		f.provider.refreshDeps.Coordinator, f.provider.refreshDeps.Revocations)
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		ClientIP: "198.51.100.8", UserAgent: "admin-console/1.0", RequestTarget: "/credentials?client_secret=do-not-log",
	})
	require.NoError(t, service.Revoke(ctx, f.agent.ID))
	events := refreshTransitionEvents(t, &logs, "RefreshSessionRevoked")
	require.Len(t, events, 2)
	want := map[string]id.Principal{f.root.ID.String(): f.root.Principal, secondRoot.ID.String(): secondRoot.Principal}
	for _, event := range events {
		sessionID, ok := event["session_id"].(string)
		require.True(t, ok)
		principal, found := want[sessionID]
		require.True(t, found, "unrelated or duplicate session %s", sessionID)
		delete(want, sessionID)
		require.Equal(t, principal.String(), event["principal"])
		require.Equal(t, f.agent.ID.String(), event["agent_id"])
		require.Equal(t, f.root.ClientID.String(), event["client_id"])
		require.Equal(t, string(storage.RefreshReasonCredentialRevoked), event["reason"])
		require.Equal(t, "request", event["origin"])
		require.Equal(t, "198.51.100.8", event["client_ip"])
		require.Equal(t, "admin-console/1.0", event["user_agent"])
	}
	require.Empty(t, want)
	require.NotContains(t, logs.String(), "do-not-log")
	require.NotContains(t, logs.String(), f.secret)
}

func TestCredentialRevocationAudit_ConfirmedOwnerRollbackHasNoSuccessEvent(t *testing.T) {
	f := newProviderRefreshFixture(t)
	var logs bytes.Buffer
	coordinator := &credentialLifecycleFaultCoordinator{AuthorizationSessionCoordinator: f.provider.refreshDeps.Coordinator,
		afterErr: errors.New("confirmed owner rollback")}
	service := NewCredentialService(f.agents, f.provider.fositeStorage.credRepo, f.provider.clientAuth,
		slog.New(slog.NewJSONHandler(&logs, nil)), coordinator, f.provider.refreshDeps.Revocations)
	require.ErrorIs(t, service.Revoke(context.Background(), f.agent.ID), coordinator.afterErr)
	require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionRevoked"))
	root, err := f.provider.refreshDeps.Sessions.FindByID(context.Background(), f.root.ID)
	require.NoError(t, err)
	require.Nil(t, root.TerminalReason)
}
