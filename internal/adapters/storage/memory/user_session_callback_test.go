package memory

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateIfAbsentKeepsConcurrentCallbackWinner(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	principal, serviceID := id.Principal("callbacks@example.com"), id.NewServiceID()
	type result struct {
		session *storage.UserSession
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, token := range []string{"first-account", "second-account"} {
		candidate := refreshTestSession(principal, serviceID)
		candidate.EncryptedAccessToken = []byte(token)
		candidate.CallbackID = token
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
	persisted, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, first.session.CallbackID, persisted.CallbackID)
	assert.Equal(t, first.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
}

func TestCreateIfAbsentReturnsIsolatedSnapshots(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	candidate := refreshTestSession("snapshot@example.com", id.NewServiceID())
	identity := id.ClientID("established-client")
	candidate.UpstreamClientID = &identity
	candidate.CallbackID = "original-generation"
	inserted, err := repo.CreateIfAbsent(ctx, candidate)
	require.NoError(t, err)
	winner, err := repo.CreateIfAbsent(ctx, refreshTestSession(candidate.Principal, candidate.ServiceID))
	require.NoError(t, err)
	for _, snapshot := range []*storage.UserSession{candidate, inserted, winner} {
		*snapshot.UpstreamClientID = "mutated-client"
		snapshot.EncryptedAccessToken[0] = 'X'
		snapshot.CallbackID = "mutated-generation"
	}
	persisted, err := repo.Get(ctx, candidate.ID)
	require.NoError(t, err)
	assert.Equal(t, id.ClientID("established-client"), *persisted.UpstreamClientID)
	assert.Equal(t, []byte("old-access"), persisted.EncryptedAccessToken)
	assert.Equal(t, "original-generation", persisted.CallbackID)
}

func TestLockedReconnectReplacesUpstreamIdentity(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	initial := refreshTestSession("reconnect@example.com", id.NewServiceID())
	initial.UpstreamClientID = new(id.ClientID("initial-client"))
	initial.CallbackID = "initial-generation"
	require.NoError(t, repo.Create(ctx, initial))
	_, err := repo.WithLockedSession(ctx, initial.Principal, initial.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		current.UpstreamClientID = new(id.ClientID("reconnected-client"))
		current.CallbackID = "reconnected-generation"
		current.EncryptedAccessToken = []byte("reconnected-access")
		return true, nil
	})
	require.NoError(t, err)
	persisted, err := repo.Get(ctx, initial.ID)
	require.NoError(t, err)
	assert.Equal(t, id.ClientID("reconnected-client"), *persisted.UpstreamClientID)
	assert.Equal(t, "reconnected-generation", persisted.CallbackID)
	assert.Equal(t, []byte("reconnected-access"), persisted.EncryptedAccessToken)
}

func TestSessionSummariesReturnIsolatedSnapshots(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	session := refreshTestSession("summary@example.com", id.NewServiceID())
	session.RefreshTokenExpiresAt = &session.CreatedAt
	require.NoError(t, repo.Create(ctx, session))
	summaries, err := repo.ListSummariesByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	summaries[0].Scope[0] = "mutated-scope"
	*summaries[0].RefreshTokenExpiresAt = session.CreatedAt.AddDate(1, 0, 0)
	persisted, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.Scope, persisted.Scope)
	assert.Equal(t, session.RefreshTokenExpiresAt, persisted.RefreshTokenExpiresAt)
}
