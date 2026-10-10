package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func refreshTestSession(principal id.Principal, serviceID id.ServiceID) *storage.UserSession {
	now := time.Now()
	return &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"), TokenType: "Bearer",
		Scope: []string{"repo"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
}

func TestWithLockedSessionDoesNotBlockOtherSessions(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	principal := id.Principal("slow-refresh@example.com")
	serviceID := id.NewServiceID()
	other := id.Principal("another-user@example.com")
	otherService := id.NewServiceID()
	require.NoError(t, repo.Create(ctx, refreshTestSession(principal, serviceID)))
	require.NoError(t, repo.Create(ctx, refreshTestSession(other, otherService)))
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	refreshDone := make(chan error, 1)
	go func() {
		_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
			close(entered)
			<-release
			session.EncryptedAccessToken = []byte("new-access")
			return true, nil
		})
		refreshDone <- err
	}()
	<-entered

	otherDone := make(chan error, 1)
	go func() {
		found, err := repo.FindByPrincipalAndService(ctx, other, otherService)
		if err != nil || found == nil {
			otherDone <- errors.New("unrelated session lookup failed")
			return
		}
		if _, err = repo.ListByPrincipal(ctx, other); err != nil {
			otherDone <- err
			return
		}
		if err = repo.Create(ctx, refreshTestSession(id.Principal("new-user@example.com"), id.NewServiceID())); err != nil {
			otherDone <- err
			return
		}
		otherDone <- repo.DeleteByPrincipalAndService(ctx, other, otherService)
	}()
	select {
	case err := <-otherDone:
		require.NoError(t, err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("unrelated session operations waited for the provider refresh")
	}
	close(release)
	require.NoError(t, <-refreshDone)
}

func TestWithLockedSessionDoesNotResurrectDeletedSession(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	principal := id.Principal("deleted-user@example.com")
	serviceID := id.NewServiceID()
	require.NoError(t, repo.Create(ctx, refreshTestSession(principal, serviceID)))
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	refreshDone := make(chan error, 1)
	go func() {
		_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
			close(entered)
			<-release
			session.EncryptedAccessToken = []byte("new-access")
			return true, nil
		})
		refreshDone <- err
	}()
	<-entered
	deleteDone := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		deleteDone <- repo.DeleteByPrincipalAndService(ctx, principal, serviceID)
	}()
	<-started
	select {
	case err := <-deleteDone:
		t.Fatalf("deleted session before refresh committed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-refreshDone)
	require.NoError(t, <-deleteDone)
	found, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestUserSessionIdentityPersistsThroughUpsertAndLockedRefresh(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryUserSessionRepository()
	principal := id.Principal("session-identity@example.com")
	serviceID := id.NewServiceID()
	initialIdentity := id.ClientID("client-A")
	initial := refreshTestSession(principal, serviceID)
	initial.UpstreamClientID = &initialIdentity
	require.NoError(t, repo.Create(ctx, initial))
	establishedIdentity := id.ClientID("client-B")
	reconnected := refreshTestSession(principal, serviceID)
	reconnected.UpstreamClientID = &establishedIdentity
	reconnected.EncryptedAccessToken = []byte("reconnected-access")
	require.NoError(t, repo.Create(ctx, reconnected))
	assert.Equal(t, initial.ID, reconnected.ID)
	refreshed, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		require.NotNil(t, current.UpstreamClientID)
		assert.Equal(t, establishedIdentity, *current.UpstreamClientID)
		current.EncryptedAccessToken = []byte("refreshed-access")
		current.EncryptedRefreshToken = []byte("rotated-refresh")
		return true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, &establishedIdentity, refreshed.UpstreamClientID)
	persisted, err := repo.Get(ctx, initial.ID)
	require.NoError(t, err)
	assert.Equal(t, &establishedIdentity, persisted.UpstreamClientID)
	assert.Equal(t, []byte("refreshed-access"), persisted.EncryptedAccessToken)
	assert.Equal(t, []byte("rotated-refresh"), persisted.EncryptedRefreshToken)
	found, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, &establishedIdentity, found.UpstreamClientID)
	listed, err := repo.ListByPrincipal(ctx, principal)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, &establishedIdentity, listed[0].UpstreamClientID)
}

func TestUserSessionLockedIdentityMutationDoesNotLeakWithoutCommit(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "no update", true: "rejected refresh"}[rejected], func(t *testing.T) {
			ctx := context.Background()
			repo := NewInMemoryUserSessionRepository()
			principal := id.Principal("identity-rollback@example.com")
			serviceID := id.NewServiceID()
			established := id.ClientID("established-client")
			session := refreshTestSession(principal, serviceID)
			session.UpstreamClientID = &established
			require.NoError(t, repo.Create(ctx, session))
			rejectedError := errors.New("provider refresh rejected")
			_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				require.NotNil(t, current.UpstreamClientID)
				*current.UpstreamClientID = "uncommitted-client"
				if rejected {
					return true, rejectedError
				}
				return false, nil
			})
			if rejected {
				assert.ErrorIs(t, err, rejectedError)
			} else {
				require.NoError(t, err)
			}
			persisted, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			require.NotNil(t, persisted.UpstreamClientID)
			assert.Equal(t, id.ClientID("established-client"), *persisted.UpstreamClientID)
		})
	}
}

func TestUserSessionLegacyMissingIdentityRemainsMissingThroughRefresh(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryUserSessionRepository()
	principal := id.Principal("legacy-session@example.com")
	serviceID := id.NewServiceID()
	session := refreshTestSession(principal, serviceID)
	require.NoError(t, repo.Create(ctx, session))
	_, err := repo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		assert.Nil(t, current.UpstreamClientID)
		current.EncryptedAccessToken = []byte("legacy-refreshed-access")
		return true, nil
	})
	require.NoError(t, err)
	persisted, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Nil(t, persisted.UpstreamClientID)
	assert.Equal(t, []byte("legacy-refreshed-access"), persisted.EncryptedAccessToken)
}
