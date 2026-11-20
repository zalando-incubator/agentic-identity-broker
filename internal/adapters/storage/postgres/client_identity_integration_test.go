//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/storagecontracts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresClientIdentityRepository(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	storagecontracts.ClientIdentityRepository(t, repo, repo)
}

func TestPostgresSessionClientRevisionEstablishment(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	provider := newTestEntity()
	require.NoError(t, NewPostgresThirdpartyOAuth2ProviderRepository(adapter).Create(ctx, provider))
	_, err := adapter.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET client_identity_revision = 7 WHERE id = $1`, provider.ID)
	require.NoError(t, err)
	repo := NewUserSessionRepository(adapter)
	for _, reconnect := range []bool{false, true} {
		session := &storage.UserSession{ID: id.NewSessionID(), Principal: id.Principal(id.NewSessionID().String()), ServiceID: provider.ID, CallbackID: "first", UpstreamClientID: new(id.ClientID("reports")), UpstreamClientRevision: 7, EncryptedAccessToken: []byte("ciphertext"), TokenType: "Bearer", Scope: []string{"read"}, InitiatedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now()}
		_, err := repo.CreateIfAbsent(ctx, session)
		require.NoError(t, err)
		if reconnect {
			_, err := adapter.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET client_identity_revision = 8 WHERE id = $1`, provider.ID)
			require.NoError(t, err)
		}
		updated, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			assert.Equal(t, int64(7), current.UpstreamClientRevision)
			current.UpstreamClientRevision = 8
			if reconnect {
				current.CallbackID = "second"
			}
			return true, nil
		})
		require.NoError(t, err)
		expected := int64(7)
		if reconnect {
			expected = 8
		}
		assert.Equal(t, expected, updated.UpstreamClientRevision)
		found, err := repo.Get(ctx, session.ID)
		require.NoError(t, err)
		assert.Equal(t, expected, found.UpstreamClientRevision)
		summaries, err := repo.ListSummariesByPrincipal(ctx, session.Principal)
		require.NoError(t, err)
		require.Len(t, summaries, 1)
		assert.Equal(t, expected, summaries[0].UpstreamClientRevision)
		assert.Equal(t, session.UpstreamClientID, summaries[0].UpstreamClientID)
	}
}

func createClientIdentityProvider(t *testing.T, repo *PostgresThirdpartyOAuth2ProviderRepository, filesystem bool) *model.ThirdpartyOAuth2ProviderEntity {
	t.Helper()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.CanonicalID = new("reports-" + provider.ID.String())
	provider.ProtectedResources = []string{"https://reports.example.com/" + provider.ID.String()}
	if filesystem {
		provider.CredentialSource = model.CredentialSourceFilesystem
		provider.ClientID = ""
		provider.Secret = model.NewAbsentSecret()
	}
	require.NoError(t, repo.Create(context.Background(), provider))
	return provider
}

func clientIdentityObservation(provider *model.ThirdpartyOAuth2ProviderEntity) ports.ClientIdentityObservation {
	return ports.ClientIdentityObservation{
		ServiceVersion: provider.Version,
		CanonicalID:    *provider.CanonicalID,
		Revision:       provider.ClientIdentityRevision,
	}
}

func createClientIdentitySession(t *testing.T, repo *PostgresUserSessionRepository, provider *model.ThirdpartyOAuth2ProviderEntity) *storage.UserSession {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal(id.NewSessionID().String()), ServiceID: provider.ID,
		CallbackID: "original", UpstreamClientID: new(provider.ClientID), UpstreamClientRevision: provider.ClientIdentityRevision,
		EncryptedAccessToken: []byte("access-ciphertext"), EncryptedRefreshToken: []byte("refresh-ciphertext"), TokenType: "Bearer",
		AccessTokenExpiresAt: new(now.Add(time.Hour)), Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
		InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	_, err := repo.CreateIfAbsent(context.Background(), session)
	require.NoError(t, err)
	persisted, err := repo.Get(context.Background(), session.ID)
	require.NoError(t, err)
	return persisted
}
