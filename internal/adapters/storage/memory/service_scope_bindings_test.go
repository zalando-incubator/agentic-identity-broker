package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const bindingGitHubScope = "https://broker.example.com/sessions/github"

var bindingGitHubID = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")

func TestAuthorizationCodeStoreServiceScopeBindings(t *testing.T) {
	ctx := context.Background()
	store := NewAuthorizationCodeStore()
	bindings := storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID}
	code := &storage.AuthorizationCode{
		ID: id.NewAuthorizationCodeID(), CodeHash: "hash", AgentID: id.NewAgentID(), Principal: "alice@example.com",
		RedirectURI: "https://client.example.com/cb", CodeChallenge: "challenge",
		Scope: "openid offline_access " + bindingGitHubScope, ServiceScopeBindings: bindings,
		ExpiresAt: time.Now().Add(time.Minute),
	}
	require.NoError(t, store.Create(ctx, code))
	bindings[bindingGitHubScope] = id.NewServiceID()

	found, err := store.FindByCodeHash(ctx, "hash")
	require.NoError(t, err)
	assert.Equal(t, storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID}, found.ServiceScopeBindings, "the store copies bindings on write")
	found.ServiceScopeBindings[bindingGitHubScope] = id.NewServiceID()
	again, err := store.FindByCodeHash(ctx, "hash")
	require.NoError(t, err)
	assert.Equal(t, bindingGitHubID, again.ServiceScopeBindings[bindingGitHubScope], "the store copies bindings on read")

	legacy := *code
	legacy.ID, legacy.CodeHash, legacy.Scope, legacy.ServiceScopeBindings = id.NewAuthorizationCodeID(), "legacy", "openid", nil
	require.NoError(t, store.Create(ctx, &legacy))
	found, err = store.FindByCodeHash(ctx, "legacy")
	require.NoError(t, err)
	assert.Nil(t, found.ServiceScopeBindings)

	invalid := *code
	invalid.ID, invalid.CodeHash, invalid.Scope = id.NewAuthorizationCodeID(), "invalid", "openid"
	invalid.ServiceScopeBindings = storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID}
	var storageErr *storage.StorageError
	require.ErrorAs(t, store.Create(ctx, &invalid), &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}

func TestRefreshTokenSessionStoreServiceScopeBindingSuccession(t *testing.T) {
	ctx := context.Background()
	store := NewRefreshTokenSessionStore()
	full := "openid offline_access " + bindingGitHubScope
	bindings := storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID}
	session := func(signature, scope string, bindings storage.ServiceScopeBindings) *storage.RefreshTokenSession {
		return &storage.RefreshTokenSession{
			Signature: signature, RequestID: "request", AgentID: id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
			ClientID: "client", Principal: "alice@example.com", Scope: scope, ServiceScopeBindings: bindings,
			ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
		}
	}
	rotate := func(previous string, successor *storage.RefreshTokenSession) error {
		require.NoError(t, store.MarkUsed(ctx, previous))
		return store.CreateSuccessor(ctx, successor, previous)
	}

	require.NoError(t, store.Create(ctx, session("code", full, bindings)))
	bindings[bindingGitHubScope] = id.NewServiceID()
	found, err := store.FindBySignature(ctx, "code")
	require.NoError(t, err)
	assert.Equal(t, bindingGitHubID, found.ServiceScopeBindings[bindingGitHubScope])

	pinned := storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID}
	require.NoError(t, rotate("code", session("narrowed", "openid offline_access", pinned.Clone())))
	found, err = store.FindBySignature(ctx, "narrowed")
	require.NoError(t, err)
	assert.Equal(t, "openid offline_access", found.Scope)
	assert.Equal(t, pinned, found.ServiceScopeBindings, "narrowing retains every assurance binding")

	require.NoError(t, rotate("narrowed", session("scopeless", "openid offline_access", pinned.Clone())))
	found, err = store.FindBySignature(ctx, "scopeless")
	require.NoError(t, err)
	assert.Equal(t, "openid offline_access", found.Scope)
	assert.Equal(t, pinned, found.ServiceScopeBindings)

	require.NoError(t, store.MarkUsed(ctx, "scopeless"))
	for name, successor := range map[string]*storage.RefreshTokenSession{
		"restored scope":  session("widened", full, pinned.Clone()),
		"dropped binding": session("dropped", "openid offline_access", nil),
		"added binding":   session("added", "openid offline_access", storage.ServiceScopeBindings{bindingGitHubScope: bindingGitHubID, "https://broker.example.com/sessions/jira": id.NewServiceID()}),
		"rebound binding": session("rebound", "openid offline_access", storage.ServiceScopeBindings{bindingGitHubScope: id.NewServiceID()}),
	} {
		var storageErr *storage.StorageError
		require.ErrorAs(t, store.CreateSuccessor(ctx, successor, "scopeless"), &storageErr, name)
		assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind, name)
		_, err := store.FindBySignature(ctx, successor.Signature)
		assert.Error(t, err, "%s must not be stored", name)
	}
	require.NoError(t, store.CreateSuccessor(ctx, session("valid", "openid", pinned.Clone()), "scopeless"))
}
