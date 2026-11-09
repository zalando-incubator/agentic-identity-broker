package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenSessionStoreClientAuthMethodRoundTrip(t *testing.T) {
	store := NewRefreshTokenSessionStore()
	method := "private_key_jwt"
	for _, test := range []struct {
		name   string
		method *string
	}{
		{name: "nil"},
		{name: "private_key_jwt", method: &method},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := &storage.RefreshTokenSession{
				Signature:        "signature-" + test.name,
				RequestID:        "request-" + test.name,
				AgentID:          id.NewAgentID(),
				ClientID:         id.NewClientID("client-" + test.name),
				Principal:        id.NewPrincipal("user@example.com"),
				ExpiresAt:        time.Now().Add(time.Hour),
				CreatedAt:        time.Now(),
				ClientAuthMethod: test.method,
			}
			require.NoError(t, store.Create(context.Background(), session))
			if test.method != nil {
				*test.method = "none"
			}
			stored, err := store.FindBySignature(context.Background(), session.Signature)
			require.NoError(t, err)
			if test.name == "nil" {
				assert.Nil(t, stored.ClientAuthMethod)
				return
			}
			require.NotNil(t, stored.ClientAuthMethod)
			assert.Equal(t, "private_key_jwt", *stored.ClientAuthMethod)
			*stored.ClientAuthMethod = "none"
			storedAgain, err := store.FindBySignature(context.Background(), session.Signature)
			require.NoError(t, err)
			require.NotNil(t, storedAgain.ClientAuthMethod)
			assert.Equal(t, "private_key_jwt", *storedAgain.ClientAuthMethod)
		})
	}
}

func TestRefreshTokenSessionStoreSuccessorCarriesClientAuthMethod(t *testing.T) {
	store := NewRefreshTokenSessionStore()
	method := "private_key_jwt"
	root := &storage.RefreshTokenSession{
		Signature:        "root-signature",
		RequestID:        "request-chain",
		AgentID:          id.NewAgentID(),
		ClientID:         id.NewClientID("client"),
		Principal:        id.NewPrincipal("user@example.com"),
		ExpiresAt:        time.Now().Add(time.Hour),
		CreatedAt:        time.Now(),
		ClientAuthMethod: &method,
	}
	require.NoError(t, store.Create(context.Background(), root))
	require.NoError(t, store.MarkUsed(context.Background(), root.Signature))
	successor := *root
	successor.Signature = "successor-signature"
	successor.UsedAt = nil
	require.NoError(t, store.CreateSuccessor(context.Background(), &successor, root.Signature))
	method = "none"
	stored, err := store.FindBySignature(context.Background(), successor.Signature)
	require.NoError(t, err)
	require.NotNil(t, stored.ClientAuthMethod)
	assert.Equal(t, "private_key_jwt", *stored.ClientAuthMethod)
}
