//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenSessionRepoClientAuthMethod(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewRefreshTokenSessionRepo(adapter)
	ctx := context.Background()
	newSession := func(signature string, method *string) *storage.RefreshTokenSession {
		agent := createTestAgent(t, adapter)
		return &storage.RefreshTokenSession{
			Signature:        signature,
			RequestID:        "request-" + signature,
			AgentID:          agent.ID,
			ClientID:         id.NewClientID("client-" + signature),
			Principal:        id.NewPrincipal("user@example.com"),
			Scope:            "offline_access read",
			ExpiresAt:        time.Now().Add(time.Hour).UTC(),
			CreatedAt:        time.Now().UTC(),
			ClientAuthMethod: method,
		}
	}

	for _, test := range []struct {
		name   string
		method *string
	}{
		{name: "nil"},
		{name: "private_key_jwt", method: ptr.To("private_key_jwt")},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := newSession("auth-method-"+test.name, test.method)
			require.NoError(t, repo.Create(ctx, session))
			stored, err := repo.FindBySignature(ctx, session.Signature)
			require.NoError(t, err)
			assert.Equal(t, test.method, stored.ClientAuthMethod)
		})
	}

	root := newSession("auth-method-root", ptr.To("private_key_jwt"))
	require.NoError(t, repo.Create(ctx, root))
	txCtx, err := adapter.BeginTX(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.MarkUsed(txCtx, root.Signature))
	successor := *root
	successor.Signature = "auth-method-successor"
	successor.RequestID = root.RequestID
	successor.UsedAt = nil
	require.NoError(t, repo.CreateSuccessor(txCtx, &successor, root.Signature))
	require.NoError(t, adapter.Commit(txCtx))
	storedSuccessor, err := repo.FindBySignature(ctx, successor.Signature)
	require.NoError(t, err)
	require.NotNil(t, storedSuccessor.ClientAuthMethod)
	assert.Equal(t, "private_key_jwt", *storedSuccessor.ClientAuthMethod)
}
