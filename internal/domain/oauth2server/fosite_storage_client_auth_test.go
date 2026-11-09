package oauth2server

import (
	"context"
	"testing"
	"time"

	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestFositeStorageRefreshClientAuthMethodRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name   string
		method string
	}{
		{name: "none"},
		{name: "private_key_jwt", method: "private_key_jwt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			refreshRepo := memory.NewRefreshTokenSessionStore()
			agentRepo := memory.NewAgentRepository()
			credRepo := memory.NewClientCredentialStore()
			store := NewFositeStorage(memory.NewAuthorizationCodeStore(), refreshRepo, memory.NewPKCESessionStore(), credRepo, &testClientResolver{agentRepo: agentRepo}, newTestGrantVerifier(agentRepo), testSlogger())
			agent := testAgent()
			require.NoError(t, agentRepo.Create(context.Background(), agent))
			credential := &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash"}
			require.NoError(t, credRepo.Create(context.Background(), credential))
			seedTestGrant(t, store.verifier, agent.ID, id.Principal("user@example.com"))
			session := &fosite.DefaultSession{Subject: "user@example.com", ExpiresAt: map[fosite.TokenType]time.Time{fosite.RefreshToken: time.Now().Add(time.Hour)}}
			request := &fosite.Request{
				ID:           "request-" + test.name,
				Client:       &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: credential},
				Session:      session,
				GrantedScope: fosite.Arguments{"offline_access"},
			}
			signature := "signature-" + test.name
			if test.method != "" {
				ctx := context.WithValue(context.Background(), privateKeyJWTAuthenticatedContextKey{}, true)
				require.NoError(t, store.CreateRefreshTokenSession(ctx, signature, "", request))
			} else {
				require.NoError(t, store.CreateRefreshTokenSession(context.Background(), signature, "", request))
			}
			stored, err := refreshRepo.FindBySignature(context.Background(), signature)
			require.NoError(t, err)
			if test.method == "" {
				assert.Nil(t, stored.ClientAuthMethod)
			} else {
				require.NotNil(t, stored.ClientAuthMethod)
				assert.Equal(t, test.method, *stored.ClientAuthMethod)
			}
		})
	}
}

func TestFositeStorageRefreshClientAuthMethodSurvivesRotation(t *testing.T) {
	refreshRepo := memory.NewRefreshTokenSessionStore()
	agentRepo := memory.NewAgentRepository()
	credRepo := memory.NewClientCredentialStore()
	store := NewFositeStorage(memory.NewAuthorizationCodeStore(), refreshRepo, memory.NewPKCESessionStore(), credRepo, &testClientResolver{agentRepo: agentRepo}, newTestGrantVerifier(agentRepo), testSlogger())
	agent := testAgent()
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	credential := &dstorage.ClientCredential{ID: id.NewCredentialID(), AgentID: agent.ID, SecretHash: "hash"}
	require.NoError(t, credRepo.Create(context.Background(), credential))
	seedTestGrant(t, store.verifier, agent.ID, id.Principal("user@example.com"))

	session := &fosite.DefaultSession{Subject: "user@example.com", ExpiresAt: map[fosite.TokenType]time.Time{fosite.RefreshToken: time.Now().Add(time.Hour)}}
	session.GetExtraClaims()["client_auth_method"] = "private_key_jwt"
	root := &fosite.Request{ID: "rotation-request", Client: &confidentialClient{clientID: agent.ID.String(), agent: agent, credential: credential}, Session: session, GrantedScope: fosite.Arguments{"offline_access"}}
	require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "root-signature", "", root))

	rotatedSession, err := store.GetRefreshTokenSession(context.Background(), "root-signature", nil)
	require.NoError(t, err)
	extra := rotatedSession.GetSession().(fosite.ExtraClaimsSession).GetExtraClaims()
	assert.Equal(t, "private_key_jwt", extra["client_auth_method"])

	require.NoError(t, store.DeleteRefreshTokenSession(context.Background(), "root-signature"))
	rotatedSession.GetSession().(fosite.ExtraClaimsSession).GetExtraClaims()["refresh_parent_signature"] = "root-signature"
	require.NoError(t, store.CreateRefreshTokenSession(context.Background(), "successor-signature", "", rotatedSession))
	storedSuccessor, err := refreshRepo.FindBySignature(context.Background(), "successor-signature")
	require.NoError(t, err)
	require.NotNil(t, storedSuccessor.ClientAuthMethod)
	assert.Equal(t, "private_key_jwt", *storedSuccessor.ClientAuthMethod)
}
