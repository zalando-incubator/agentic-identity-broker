package storage

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runRefreshTokenSessionConsentContract(t *testing.T, newRepository func(*testing.T) (ports.RefreshTokenSessionRepository, id.AgentID, id.AgentID)) {
	t.Helper()
	ctx := context.Background()
	principal := id.NewPrincipal("refresh-user@example.com")
	otherPrincipal := id.NewPrincipal("other@example.com")
	oldUsedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		count int
	}{
		{"principal and agent", 2},
		{"agent", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, agentID, otherAgentID := newRepository(t)
			revoke := func(ctx context.Context) (int, error) {
				if tc.name == "agent" {
					return repo.RevokeByAgent(ctx, agentID)
				}
				return repo.RevokeByPrincipalAndAgent(ctx, principal, agentID)
			}
			fixtures := []struct {
				name      string
				agent     id.AgentID
				principal id.Principal
				usedAt    *time.Time
				inactive  bool
			}{
				{"unused", agentID, principal, nil, true},
				{"expired unused", agentID, principal, nil, true},
				{"already used", agentID, principal, &oldUsedAt, true},
				{"other principal", agentID, otherPrincipal, nil, tc.count == 3},
				{"other agent", otherAgentID, principal, nil, false},
			}
			for _, f := range fixtures {
				expiresAt := time.Now().Add(time.Hour)
				if f.name == "expired unused" {
					expiresAt = oldUsedAt
				}
				require.NoError(t, repo.Create(ctx, &storage.RefreshTokenSession{
					Signature: tc.name + f.name, RequestID: tc.name + f.name,
					AgentID: f.agent, ClientID: id.NewClientID("refresh-client"), Principal: f.principal,
					ExpiresAt: expiresAt, CreatedAt: oldUsedAt, UsedAt: f.usedAt,
				}))
			}
			count, err := revoke(ctx)
			require.NoError(t, err)
			assert.Equal(t, tc.count, count)
			count, err = revoke(ctx)
			require.NoError(t, err)
			assert.Zero(t, count)
			for _, f := range fixtures {
				got, err := repo.FindBySignature(ctx, tc.name+f.name)
				require.NoError(t, err)
				assert.Equal(t, f.inactive, got.UsedAt != nil, f.name)
				if f.usedAt != nil {
					require.NotNil(t, got.UsedAt)
					assert.True(t, oldUsedAt.Equal(*got.UsedAt), "old use timestamp must survive revocation")
				}
			}
		})
	}
	t.Run("nullable grant identity is isolated from caller mutation", func(t *testing.T) {
		grantID := id.NewGrantID()
		original := grantID
		repo, agentID, _ := newRepository(t)
		session := &storage.RefreshTokenSession{
			Signature: "grant-identity", RequestID: "grant-identity", AgentID: agentID,
			ClientID: id.NewClientID("refresh-client"), Principal: principal,
			ExpiresAt: time.Now().Add(time.Hour), CreatedAt: oldUsedAt, GrantID: &grantID,
		}
		require.NoError(t, repo.Create(ctx, session))
		grantID = id.NewGrantID()
		got, err := repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		require.NotNil(t, got.GrantID)
		assert.Equal(t, original, *got.GrantID)
		*got.GrantID = id.NewGrantID()
		got, err = repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		require.NotNil(t, got.GrantID)
		assert.Equal(t, original, *got.GrantID)
		session.Signature = "legacy-grant"
		session.GrantID = nil
		require.NoError(t, repo.Create(ctx, session))
		got, err = repo.FindBySignature(ctx, session.Signature)
		require.NoError(t, err)
		assert.Nil(t, got.GrantID)
	})
}

func TestRefreshTokenSessionStoreConsent(t *testing.T) {
	runRefreshTokenSessionConsentContract(t, func(t *testing.T) (ports.RefreshTokenSessionRepository, id.AgentID, id.AgentID) {
		return memory.NewRefreshTokenSessionStore(), id.NewAgentID(), id.NewAgentID()
	})
}
