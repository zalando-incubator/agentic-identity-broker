package agents

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

func agentRevocationEvents(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var events []map[string]any
	decoder := json.NewDecoder(logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["event"] == "RefreshSessionRevoked" {
			events = append(events, record)
		}
	}
	return events
}

func TestAgentDeletionAudit_ReceiptsSurviveCascadeAndLogsFollowCommit(t *testing.T) {
	f := newDeleteFixture()
	first := f.addRoot(t, f.agentID, id.NewPrincipal("alice@example.test"), true)
	second := f.addRoot(t, f.agentID, id.NewPrincipal("bob@example.test"), false)
	unrelated := f.addRoot(t, f.otherID, id.NewPrincipal("alice@example.test"), false)
	f.service.logger = slog.New(slog.NewJSONHandler(f.logs, nil))
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		ClientIP: "192.0.2.35", UserAgent: "trusted-admin/1.0",
		RequestTarget: "/api/agents?client_secret=do-not-log",
	})
	require.NoError(t, f.service.Delete(ctx, f.agentID))
	events := agentRevocationEvents(t, f.logs)
	require.Len(t, events, 2, "one committed transition for each root removed by cascade")
	want := map[string]id.Principal{first.ID.String(): first.Principal, second.ID.String(): second.Principal}
	for _, event := range events {
		sessionID, ok := event["session_id"].(string)
		require.True(t, ok)
		principal, present := want[sessionID]
		require.True(t, present, "unrelated or duplicate session %s", sessionID)
		delete(want, sessionID)
		require.Equal(t, principal.String(), event["principal"])
		require.Equal(t, f.agentID.String(), event["agent_id"])
		require.Equal(t, first.ClientID.String(), event["client_id"])
		require.Equal(t, string(storage.RefreshReasonAgentDeleted), event["reason"])
		require.Equal(t, "request", event["origin"])
		require.Equal(t, "192.0.2.35", event["client_ip"])
		require.Equal(t, "trusted-admin/1.0", event["user_agent"])
	}
	require.Empty(t, want)
	require.NotContains(t, f.logs.String(), unrelated.ID.String())
	require.NotContains(t, f.logs.String(), "do-not-log")
	for _, root := range []*storage.RefreshSession{first, second} {
		_, exists := f.state.roots[root.ID]
		require.False(t, exists)
		_, recorded := f.state.receipts[root.ID]
		require.True(t, recorded, "receipt must outlive cascaded root")
	}
}

func TestAgentDeletionAudit_ConfirmedRollbackNeverLogsSuccess(t *testing.T) {
	f := newDeleteFixture()
	root := f.addRoot(t, f.agentID, id.NewPrincipal("owner@example.test"), false)
	f.service.logger = slog.New(slog.NewJSONHandler(f.logs, nil))
	f.coordinator.commitErr = errors.New("confirmed rollback")
	require.ErrorIs(t, f.service.Delete(context.Background(), f.agentID), f.coordinator.commitErr)
	require.Empty(t, agentRevocationEvents(t, f.logs))
	require.NotNil(t, f.state.roots[root.ID])
	require.Empty(t, f.state.receipts)
}
