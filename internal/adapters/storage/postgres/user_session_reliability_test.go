//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Locked token updates previously dropped session metadata, so later reads evaluated stale state.
func TestUserSessionRefreshPersistsRefreshExpiryAndScope(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()

	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	principal := id.Principal("refresh-metadata@example.com")
	expired := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, repo.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"),
		TokenType: "Bearer", AccessTokenExpiresAt: &expired, Scope: []string{"repo", "user"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))

	newAccessExpiry := time.Now().UTC().Add(time.Hour).Round(time.Millisecond)
	newRefreshExpiry := time.Now().UTC().Add(14 * 24 * time.Hour).Round(time.Millisecond)
	_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
		session.EncryptedAccessToken = []byte("new-access")
		session.EncryptedRefreshToken = []byte("new-refresh")
		session.TokenType = "DPoP"
		session.AccessTokenExpiresAt = &newAccessExpiry
		session.RefreshTokenExpiresAt = &newRefreshExpiry
		session.Scope = []string{"repo"}
		session.UpdatedAt = time.Now().UTC()
		return true, nil
	})
	require.NoError(t, err)

	persisted, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, []byte("new-refresh"), persisted.EncryptedRefreshToken)
	assert.Equal(t, "DPoP", persisted.TokenType)
	require.NotNil(t, persisted.RefreshTokenExpiresAt, "refresh_token_expires_at must be committed by the refresh transaction")
	assert.WithinDuration(t, newRefreshExpiry, *persisted.RefreshTokenExpiresAt, time.Second)
	assert.Equal(t, []string{"repo"}, persisted.Scope, "scope narrowed on refresh must be committed")
}

func TestUserSessionReadsAfterSchemaExpansion(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()
	now := time.Now().Truncate(time.Microsecond)
	expires := now.Add(time.Hour)
	principal := id.Principal("schema-expansion@example.com")
	sessions := make([]*storage.UserSession, 2)
	for i := range sessions {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		sessions[i] = &storage.UserSession{
			ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
			CallbackID: "original-callback", EncryptedAccessToken: []byte("old-access"),
			EncryptedRefreshToken: []byte("old-refresh"), TokenType: "Bearer",
			AccessTokenExpiresAt: &expires, RefreshTokenExpiresAt: &expires,
			Scope: []string{"profile"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		require.NoError(t, repo.Create(ctx, sessions[i]))
	}
	_, err := adapter.db.ExecContext(ctx, `ALTER TABLE user_sessions ADD COLUMN upstream_client_id TEXT`)
	require.NoError(t, err)
	_, err = adapter.db.ExecContext(ctx, `UPDATE user_sessions SET upstream_client_id = 'established-client' WHERE id = $1`, sessions[0].ID)
	require.NoError(t, err)

	t.Run("Get", func(t *testing.T) {
		for _, expected := range sessions {
			got, err := repo.Get(ctx, expected.ID)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, expected.ID, got.ID)
			assert.Equal(t, expected.ServiceID, got.ServiceID)
			assert.Equal(t, expected.Principal, got.Principal)
			assert.Equal(t, expected.CallbackID, got.CallbackID)
			assert.Equal(t, expected.EncryptedAccessToken, got.EncryptedAccessToken)
			assert.Equal(t, expected.EncryptedRefreshToken, got.EncryptedRefreshToken)
			assert.Equal(t, expected.TokenType, got.TokenType)
			assert.Equal(t, expected.Scope, got.Scope)
			assert.Equal(t, expected.EncryptionContext, got.EncryptionContext)
		}
	})
	t.Run("FindByPrincipalAndService", func(t *testing.T) {
		got, err := repo.FindByPrincipalAndService(ctx, principal, sessions[0].ServiceID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, sessions[0].ID, got.ID)
	})
	for name, list := range map[string]func(context.Context, id.Principal) ([]*storage.UserSession, error){
		"ListByPrincipal": repo.ListByPrincipal, "ListActiveByPrincipal": repo.ListActiveByPrincipal,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := list(ctx, principal)
			require.NoError(t, err)
			assert.ElementsMatch(t, expirySessionIDs(sessions), expirySessionIDs(got))
		})
	}
	t.Run("ListExpiringSessions", func(t *testing.T) {
		first, err := repo.ListExpiringSessions(ctx, expires, storage.SessionExpiryCursor{}, 1)
		require.NoError(t, err)
		require.Len(t, first, 1)
		cursor := storage.SessionExpiryCursor{AccessTokenExpiresAt: *first[0].AccessTokenExpiresAt, ID: first[0].ID}
		second, err := repo.ListExpiringSessions(ctx, expires, cursor, 1)
		require.NoError(t, err)
		assert.ElementsMatch(t, expirySessionIDs(sessions), expirySessionIDs(append(first, second...)))
	})
	t.Run("CreateIfAbsent preserves existing connection", func(t *testing.T) {
		candidate := *sessions[0]
		candidate.ID = id.NewSessionID()
		candidate.EncryptedAccessToken = []byte("competing-access")
		got, err := repo.CreateIfAbsent(ctx, &candidate)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, sessions[0].ID, got.ID)
		assert.Equal(t, sessions[0].EncryptedAccessToken, got.EncryptedAccessToken)
	})
	t.Run("WithLockedSession persists refresh without changing new column", func(t *testing.T) {
		expected := *sessions[0]
		expected.EncryptedAccessToken = []byte("renewed-access")
		expected.EncryptedRefreshToken = []byte("renewed-refresh")
		_, err := repo.WithLockedSession(ctx, principal, expected.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			assert.Equal(t, sessions[0].ID, current.ID)
			assert.Equal(t, sessions[0].EncryptedRefreshToken, current.EncryptedRefreshToken)
			current.EncryptedAccessToken = expected.EncryptedAccessToken
			current.EncryptedRefreshToken = expected.EncryptedRefreshToken
			return true, nil
		})
		require.NoError(t, err)
		got, err := repo.Get(ctx, expected.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, expected.EncryptedAccessToken, got.EncryptedAccessToken)
		assert.Equal(t, expected.EncryptedRefreshToken, got.EncryptedRefreshToken)
		var established string
		require.NoError(t, adapter.db.GetContext(ctx, &established, `SELECT upstream_client_id FROM user_sessions WHERE id = $1`, expected.ID))
		assert.Equal(t, "established-client", established)
	})
}
