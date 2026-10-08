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

func TestUserSessionUpsertReturnsRetainedIdentityAndTimestamps(t *testing.T) {
	for _, commit := range []bool{false, true} {
		name := "rollback"
		if commit {
			name = "commit"
		}
		t.Run(name, func(t *testing.T) {
			transactions := NewTransactionManager()
			repo := NewInMemoryUserSessionRepository(transactions)
			ctx := context.Background()
			first := refreshTestSession("reauthorizing-user", id.NewServiceID())
			first.InitiatedAt = time.Now().UTC().Add(-2 * time.Hour)
			first.CreatedAt = first.InitiatedAt.Add(time.Minute)
			first.UpdatedAt = first.CreatedAt
			require.NoError(t, repo.Create(ctx, first))

			replacement := refreshTestSession(first.Principal, first.ServiceID)
			discardedID := replacement.ID
			replacement.EncryptedAccessToken = []byte("replacement-access")
			replacement.EncryptedRefreshToken = []byte("replacement-refresh")
			replacement.UpdatedAt = time.Now().UTC().Add(time.Hour)
			beforeWrite := time.Now().UTC()
			owner, err := transactions.BeginTX(ctx)
			require.NoError(t, err)
			defer func() { _ = transactions.Rollback(owner) }()
			require.NoError(t, repo.Create(owner, replacement))
			require.Equal(t, first.ID, replacement.ID)
			require.Equal(t, first.InitiatedAt, replacement.InitiatedAt)
			require.Equal(t, first.CreatedAt, replacement.CreatedAt)
			require.False(t, replacement.UpdatedAt.Before(beforeWrite))
			require.False(t, replacement.UpdatedAt.After(time.Now().UTC()))
			stored, err := repo.Get(owner, replacement.ID)
			require.NoError(t, err)
			require.Equal(t, replacement, stored)
			_, err = repo.Get(owner, discardedID)
			require.Error(t, err)

			expected := first
			if commit {
				require.NoError(t, transactions.Commit(owner))
				expected = replacement
			} else {
				require.NoError(t, transactions.Rollback(owner))
			}
			stored, err = repo.FindByPrincipalAndService(ctx, first.Principal, first.ServiceID)
			require.NoError(t, err)
			require.Equal(t, expected, stored)
		})
	}
}

func TestWithLockedSessionDoesNotBlockOtherSessions(t *testing.T) {
	repo := NewInMemoryUserSessionRepository(NewTransactionManager())
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
		_, err := repo.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) error {
			previous := copyUserSession(session)
			close(entered)
			<-release
			session.EncryptedAccessToken = []byte("new-access")
			tx, err := repo.transactions.BeginTX(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = repo.transactions.Rollback(tx) }()
			if err := repo.UpdateRefreshedSession(tx, previous, session); err != nil {
				return err
			}
			return repo.transactions.Commit(tx)
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
	repo := NewInMemoryUserSessionRepository(NewTransactionManager())
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
		_, err := repo.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) error {
			previous := copyUserSession(session)
			close(entered)
			<-release
			session.EncryptedAccessToken = []byte("new-access")
			tx, err := repo.transactions.BeginTX(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = repo.transactions.Rollback(tx) }()
			if err := repo.UpdateRefreshedSession(tx, previous, session); err != nil {
				return err
			}
			return repo.transactions.Commit(tx)
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

func TestWithLockedSessionRollbackRestoresRotatingTokens(t *testing.T) {
	transactions := NewTransactionManager()
	repo := NewInMemoryUserSessionRepository(transactions)
	ctx := context.Background()
	principal := id.Principal("rollback-user@example.com")
	serviceID := id.NewServiceID()
	session := refreshTestSession(principal, serviceID)
	require.NoError(t, repo.Create(ctx, session))

	txCtx, err := transactions.BeginTX(ctx)
	require.NoError(t, err)
	_, err = repo.WithLockedSession(txCtx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) error {
		previous := copyUserSession(current)
		current.EncryptedAccessToken = []byte("new-access")
		current.EncryptedRefreshToken = []byte("rotated-refresh")
		return repo.UpdateRefreshedSession(ctx, previous, current)
	})
	require.NoError(t, err)
	require.NoError(t, transactions.Rollback(txCtx))

	byPair, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	byID, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	for _, current := range []*storage.UserSession{byPair, byID} {
		assert.Equal(t, []byte("old-access"), current.EncryptedAccessToken)
		assert.Equal(t, []byte("old-refresh"), current.EncryptedRefreshToken)
	}
}
