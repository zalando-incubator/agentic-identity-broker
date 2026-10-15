//go:build integration

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

func TestCreateIfAbsentKeepsConcurrentCallbackWinner(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()
	principal, serviceID := id.Principal("callbacks@example.com"), id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	type result struct {
		session *storage.UserSession
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, token := range []string{"first-account", "second-account"} {
		now := time.Now().UTC()
		candidate := &storage.UserSession{
			ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, CallbackID: token,
			UpstreamClientID:     new(id.ClientID(token)),
			EncryptedAccessToken: []byte(token), TokenType: "Bearer", Scope: []string{"repo"},
			EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
		}
		go func() {
			<-start
			session, err := repo.CreateIfAbsent(ctx, candidate)
			results <- result{session, err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.session)
	require.NotNil(t, second.session)
	assert.Equal(t, first.session.ID, second.session.ID)
	assert.Equal(t, first.session.EncryptedAccessToken, second.session.EncryptedAccessToken)
	assert.Equal(t, first.session.UpstreamClientID, second.session.UpstreamClientID)
	_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
		session.CallbackID = "reconnect-generation"
		session.EncryptedAccessToken = []byte("reconnected-account")
		session.UpstreamClientID = new(id.ClientID("reconnected-client"))
		return true, nil
	})
	require.NoError(t, err)
	persisted, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, "reconnect-generation", persisted.CallbackID)
	assert.Equal(t, []byte("reconnected-account"), persisted.EncryptedAccessToken)
	assert.Equal(t, new(id.ClientID("reconnected-client")), persisted.UpstreamClientID)
	_, err = repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
		*session.UpstreamClientID = "unexpected-refresh-client"
		session.EncryptedAccessToken = []byte("refreshed-access")
		return true, nil
	})
	require.NoError(t, err)
	persisted, err = repo.Get(ctx, persisted.ID)
	require.NoError(t, err)
	assert.Equal(t, new(id.ClientID("reconnected-client")), persisted.UpstreamClientID)
	assert.Equal(t, "reconnect-generation", persisted.CallbackID)
	assert.Equal(t, []byte("refreshed-access"), persisted.EncryptedAccessToken)
}
