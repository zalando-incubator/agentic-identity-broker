//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const repoGitHubScope = "https://broker.example.com/sessions/github"

var repoGitHubID = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")

// Bindings from specs/055-incremental-service-authorization/data-model.md (DEC-004, FR-019).
func TestServiceScopeBindingsRoundTrip(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	agent := createTestAgent(t, adapter)
	pinned := storage.ServiceScopeBindings{repoGitHubScope: repoGitHubID}

	t.Run("authorization codes", func(t *testing.T) {
		repo := NewAuthorizationCodeRepo(adapter)
		code := func(hash, scope string, bindings storage.ServiceScopeBindings) *storage.AuthorizationCode {
			return &storage.AuthorizationCode{
				ID: id.NewAuthorizationCodeID(), CodeHash: hash, AgentID: agent.ID, ClientID: "client",
				Principal: "alice@example.com", RedirectURI: "https://client.example.com/cb", CodeChallenge: "challenge",
				Scope: scope, ServiceScopeBindings: bindings, ExpiresAt: time.Now().Add(time.Minute).UTC(), CreatedAt: time.Now().UTC(),
			}
		}
		require.NoError(t, repo.Create(ctx, code("bound", "openid "+repoGitHubScope, pinned.Clone())))
		found, err := repo.FindByCodeHash(ctx, "bound")
		require.NoError(t, err)
		assert.Equal(t, pinned, found.ServiceScopeBindings)

		require.NoError(t, repo.Create(ctx, code("legacy", "openid", nil)))
		found, err = repo.FindByCodeHash(ctx, "legacy")
		require.NoError(t, err)
		assert.Nil(t, found.ServiceScopeBindings)

		var storageErr *storage.StorageError
		require.ErrorAs(t, repo.Create(ctx, code("outside", "openid", pinned.Clone())), &storageErr)
		assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	})

	t.Run("refresh succession narrows scope and inherits every binding", func(t *testing.T) {
		repo := NewRefreshTokenSessionRepo(adapter)
		session := func(signature, scope string, bindings storage.ServiceScopeBindings) *storage.RefreshTokenSession {
			return &storage.RefreshTokenSession{
				Signature: signature, RequestID: "bindings-request", AgentID: agent.ID, ClientID: "client",
				Principal: "alice@example.com", Scope: scope, ServiceScopeBindings: bindings,
				ExpiresAt: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC(),
			}
		}
		rotate := func(previous string, successor *storage.RefreshTokenSession) error {
			rotationCtx, err := adapter.BeginTX(ctx)
			require.NoError(t, err)
			defer func() { _ = adapter.Rollback(rotationCtx) }()
			require.NoError(t, repo.MarkUsed(rotationCtx, previous))
			if err := repo.CreateSuccessor(rotationCtx, successor, previous); err != nil {
				return err
			}
			return adapter.Commit(rotationCtx)
		}

		require.NoError(t, repo.Create(ctx, session("full", "openid offline_access "+repoGitHubScope, pinned.Clone())))
		require.NoError(t, rotate("full", session("narrowed", "openid offline_access", pinned.Clone())))
		require.NoError(t, rotate("narrowed", session("scopeless", "openid offline_access", pinned.Clone())))
		found, err := repo.FindBySignature(ctx, "scopeless")
		require.NoError(t, err)
		assert.Equal(t, "openid offline_access", found.Scope)
		assert.Equal(t, pinned, found.ServiceScopeBindings)

		for name, successor := range map[string]*storage.RefreshTokenSession{
			"restored scope":  session("widened", "openid offline_access "+repoGitHubScope, pinned.Clone()),
			"dropped binding": session("dropped", "openid", nil),
			"added binding":   session("added", "openid", storage.ServiceScopeBindings{repoGitHubScope: repoGitHubID, "https://broker.example.com/sessions/jira": id.NewServiceID()}),
			"rebound binding": session("rebound", "openid", storage.ServiceScopeBindings{repoGitHubScope: id.NewServiceID()}),
		} {
			var storageErr *storage.StorageError
			require.ErrorAs(t, rotate("scopeless", successor), &storageErr, name)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind, name)
			_, err := repo.FindBySignature(ctx, successor.Signature)
			assert.Error(t, err, "%s must roll back", name)
			unused, err := repo.FindBySignature(ctx, "scopeless")
			require.NoError(t, err)
			assert.Nil(t, unused.UsedAt, "%s must not consume the predecessor", name)
		}

		require.NoError(t, repo.Create(ctx, session("legacy-refresh", "openid", nil)))
		found, err = repo.FindBySignature(ctx, "legacy-refresh")
		require.NoError(t, err)
		assert.Nil(t, found.ServiceScopeBindings)
	})
}
