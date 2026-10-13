package oauth2server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingCredentialRefreshRepo struct {
	ports.RefreshTokenSessionRepository
	err error
}

func (r failingCredentialRefreshRepo) RevokeByAgent(context.Context, id.AgentID) (int, error) {
	return 0, r.err
}

type failingCredentialDeleteRepo struct {
	ports.ClientCredentialRepository
	err error
}

func (r failingCredentialDeleteRepo) Delete(context.Context, id.AgentID) error {
	return r.err
}

func TestCredentialService_RevokeInvalidatesRefreshSessions(t *testing.T) {
	for _, failure := range []string{"none", "delete", "refresh"} {
		t.Run(failure, func(t *testing.T) {
			ctx := context.Background()
			agentID, otherAgent := id.NewAgentID(), id.NewAgentID()
			credentials, refresh := memory.NewClientCredentialStore(), memory.NewRefreshTokenSessionStore()
			for _, agent := range []id.AgentID{agentID, otherAgent} {
				require.NoError(t, credentials.Create(ctx, &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent}))
			}
			usedAt := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
			for _, row := range []storage.RefreshTokenSession{
				{Signature: "alice", AgentID: agentID, Principal: id.Principal("alice")},
				{Signature: "bob", AgentID: agentID, Principal: id.Principal("bob")},
				{Signature: "used", AgentID: agentID, UsedAt: &usedAt},
				{Signature: "other-agent", AgentID: otherAgent},
			} {
				require.NoError(t, refresh.Create(ctx, &row))
			}
			var logs bytes.Buffer
			svc := NewCredentialService(nil, credentials, refresh, nil, slog.New(slog.NewJSONHandler(&logs, nil)))
			failureErr := errors.New("storage unavailable")
			switch failure {
			case "delete":
				svc.credentials = failingCredentialDeleteRepo{credentials, failureErr}
			case "refresh":
				svc.refreshRepo = failingCredentialRefreshRepo{refresh, failureErr}
			}
			err := svc.Revoke(ctx, agentID)
			if failure == "none" {
				require.NoError(t, err)
				var audit map[string]any
				require.NoError(t, json.Unmarshal(logs.Bytes(), &audit))
				assert.Equal(t, "credential_revoked", audit["trigger"])
				assert.Equal(t, float64(2), audit["row_count"])
			} else {
				require.ErrorIs(t, err, failureErr)
			}
			_, err = credentials.GetByAgentID(ctx, agentID)
			assert.Equal(t, failure == "delete", err == nil)
			_, err = credentials.GetByAgentID(ctx, otherAgent)
			require.NoError(t, err)
			for _, signature := range []string{"alice", "bob", "used", "other-agent"} {
				row, err := refresh.FindBySignature(ctx, signature)
				require.NoError(t, err)
				switch {
				case signature == "used":
					assert.Equal(t, usedAt, *row.UsedAt)
				case failure == "none" && signature != "other-agent":
					assert.NotNil(t, row.UsedAt)
					require.Error(t, refresh.MarkUsed(ctx, signature))
				default:
					assert.Nil(t, row.UsedAt)
				}
			}
		})
	}
}
