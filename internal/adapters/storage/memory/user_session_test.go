package memory

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
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

func expiryTestID(prefix byte) id.SessionID {
	return id.MustParseSessionID(fmt.Sprintf("%02x000000-0000-4000-8000-000000000001", prefix))
}

func createExpiringTestSession(t *testing.T, repo *InMemoryUserSessionRepository, prefix byte, expiresAt *time.Time) *storage.UserSession {
	t.Helper()
	session := refreshTestSession(id.Principal(fmt.Sprintf("session-%02x@example.com", prefix)), id.NewServiceID())
	session.ID = expiryTestID(prefix)
	session.AccessTokenExpiresAt = expiresAt
	require.NoError(t, repo.Create(context.Background(), session))
	return session
}

func expiryTestCursor(session *storage.UserSession) storage.SessionExpiryCursor {
	return storage.SessionExpiryCursor{AccessTokenExpiresAt: *session.AccessTokenExpiresAt, ID: session.ID}
}

func TestListExpiringSessionsThresholdAndCiphertext(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	before, after := threshold.Add(-time.Hour), threshold.Add(time.Second)
	refreshExpiry := threshold.Add(24 * time.Hour)
	earlier := refreshTestSession(id.Principal("ciphertext@example.com"), id.NewServiceID())
	earlier.ID = expiryTestID(0x80)
	earlier.AccessTokenExpiresAt = &before
	earlier.RefreshTokenExpiresAt = &refreshExpiry
	earlier.EncryptedAccessToken = []byte{0, 0xff, 0x01, 0x80}
	earlier.EncryptedRefreshToken = []byte{0xfe, 0, 0xfd}
	require.NoError(t, repo.Create(ctx, earlier))
	atBoundary := createExpiringTestSession(t, repo, 0x02, &threshold)
	createExpiringTestSession(t, repo, 0x10, &after)
	createExpiringTestSession(t, repo, 0xf0, nil)

	got, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []id.SessionID{earlier.ID, atBoundary.ID}, []id.SessionID{got[0].ID, got[1].ID})
	assert.Equal(t, earlier, got[0], "the candidate must include its complete encrypted session")
	assert.NotSame(t, earlier, got[0], "the candidate must be a snapshot")
	assert.Equal(t, atBoundary, got[1])

	empty, err := repo.ListExpiringSessions(ctx, before.Add(-time.Second), storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	assert.Empty(t, empty)

	// Changing stored data after listing must not change the encrypted page snapshot.
	earlier.EncryptedAccessToken[0] = 0x42
	earlier.EncryptedRefreshToken[0] = 0x42
	earlier.Scope[0] = "changed"
	*earlier.AccessTokenExpiresAt = after
	*earlier.RefreshTokenExpiresAt = after
	assert.Equal(t, []byte{0, 0xff, 0x01, 0x80}, got[0].EncryptedAccessToken)
	assert.Equal(t, []byte{0xfe, 0, 0xfd}, got[0].EncryptedRefreshToken)
	assert.Equal(t, []string{"repo"}, got[0].Scope)
	assert.Equal(t, threshold.Add(-time.Hour), *got[0].AccessTokenExpiresAt)
	assert.Equal(t, threshold.Add(24*time.Hour), *got[0].RefreshTokenExpiresAt)
}

func TestListExpiringSessionsKeysetPages(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	firstExpiry := threshold.Add(-3 * time.Hour)
	sharedExpiry := threshold.Add(-2 * time.Hour)
	laterExpiry := threshold.Add(-time.Hour)
	for _, item := range []struct {
		prefix byte
		expiry *time.Time
	}{
		{0x80, &sharedExpiry}, {0x10, &threshold}, {0xf0, &firstExpiry},
		{0x40, &laterExpiry}, {0x02, &sharedExpiry},
	} {
		createExpiringTestSession(t, repo, item.prefix, item.expiry)
	}
	expected := []id.SessionID{
		expiryTestID(0xf0), expiryTestID(0x02), expiryTestID(0x80),
		expiryTestID(0x40), expiryTestID(0x10),
	}

	all, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	require.Len(t, all, len(expected))
	single, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
	require.NoError(t, err)
	require.Len(t, single, 1)
	assert.Equal(t, expected[0], single[0].ID)

	var cursor storage.SessionExpiryCursor
	var pages []int
	var paged []id.SessionID
	for {
		page, err := repo.ListExpiringSessions(ctx, threshold, cursor, 2)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		pages = append(pages, len(page))
		for _, session := range page {
			paged = append(paged, session.ID)
		}
		cursor = expiryTestCursor(page[len(page)-1])
		if len(page) < 2 {
			break
		}
	}
	assert.Equal(t, []int{2, 2, 1}, pages)
	assert.Equal(t, expected, paged)
	allIDs := make([]id.SessionID, 0, len(all))
	for _, session := range all {
		allIDs = append(allIDs, session.ID)
	}
	assert.Equal(t, expected, allIDs)
}

func TestListExpiringSessionsAdvancesAcrossMutations(t *testing.T) {
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	firstExpiry := threshold.Add(-4 * time.Hour)
	secondExpiry := threshold.Add(-3 * time.Hour)
	thirdExpiry := threshold.Add(-2 * time.Hour)
	fourthExpiry := threshold.Add(-time.Hour)
	behindExpiry := firstExpiry.Add(-time.Minute)
	aheadExpiry := secondExpiry.Add(time.Minute)
	for _, tc := range []struct {
		name      string
		pageSize  int
		refresh   bool
		firstIDs  []id.SessionID
		remaining []id.SessionID
	}{
		{
			name: "one_row_deletes_previous_candidate", pageSize: 1,
			firstIDs:  []id.SessionID{expiryTestID(0x90)},
			remaining: []id.SessionID{expiryTestID(0x30), expiryTestID(0x60), expiryTestID(0x10), expiryTestID(0x70), expiryTestID(0xf0)},
		},
		{
			name: "one_row_refreshes_previous_candidate", pageSize: 1, refresh: true,
			firstIDs:  []id.SessionID{expiryTestID(0x90)},
			remaining: []id.SessionID{expiryTestID(0x30), expiryTestID(0x60), expiryTestID(0x10), expiryTestID(0x70), expiryTestID(0xf0)},
		},
		{
			name: "two_rows_refresh_and_delete_previous_candidates", pageSize: 2, refresh: true,
			firstIDs:  []id.SessionID{expiryTestID(0x90), expiryTestID(0x30)},
			remaining: []id.SessionID{expiryTestID(0x60), expiryTestID(0x10), expiryTestID(0x70), expiryTestID(0xf0)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewInMemoryUserSessionRepository()
			ctx := context.Background()
			for _, item := range []struct {
				prefix byte
				expiry *time.Time
			}{
				{0x70, &fourthExpiry}, {0x90, &firstExpiry}, {0x10, &thirdExpiry},
				{0xf0, &fourthExpiry}, {0x30, &secondExpiry},
			} {
				createExpiringTestSession(t, repo, item.prefix, item.expiry)
			}
			first, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, tc.pageSize)
			require.NoError(t, err)
			require.Len(t, first, tc.pageSize)
			firstIDs := make([]id.SessionID, 0, len(first))
			for _, session := range first {
				firstIDs = append(firstIDs, session.ID)
			}
			assert.Equal(t, tc.firstIDs, firstIDs)
			cursor := expiryTestCursor(first[len(first)-1])

			if tc.refresh {
				updated := *first[0]
				noLongerDue := threshold.Add(time.Second)
				updated.AccessTokenExpiresAt = &noLongerDue
				require.NoError(t, repo.Create(ctx, &updated))
			}
			if !tc.refresh || tc.pageSize == 2 {
				require.NoError(t, repo.Delete(ctx, first[len(first)-1].ID))
			}
			createExpiringTestSession(t, repo, 0x20, &behindExpiry)
			createExpiringTestSession(t, repo, 0x60, &aheadExpiry)

			var remaining []id.SessionID
			for {
				page, err := repo.ListExpiringSessions(ctx, threshold, cursor, tc.pageSize)
				require.NoError(t, err)
				if len(page) == 0 {
					break
				}
				for _, session := range page {
					remaining = append(remaining, session.ID)
				}
				cursor = expiryTestCursor(page[len(page)-1])
				if len(page) < tc.pageSize {
					break
				}
			}
			assert.Equal(t, tc.remaining, remaining, "no remaining candidate may be skipped or repeated")
		})
	}
}

func TestListExpiringSessionsLimitValidation(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	for _, limit := range []int{-1, 0, 1001} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			got, err := repo.ListExpiringSessions(context.Background(), time.Now(), storage.SessionExpiryCursor{}, limit)
			assert.Empty(t, got)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	}
}

func TestListExpiringSessionsRejectsPartialCursor(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	createExpiringTestSession(t, repo, 0x20, &threshold)
	for _, tc := range []struct {
		name   string
		cursor storage.SessionExpiryCursor
	}{
		{"expiry_without_ID", storage.SessionExpiryCursor{AccessTokenExpiresAt: threshold}},
		{"ID_without_expiry", storage.SessionExpiryCursor{ID: expiryTestID(0x10)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ListExpiringSessions(context.Background(), threshold, tc.cursor, 1)
			assert.Empty(t, got)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	}
}

func TestListExpiringSessionsOrdersUUIDBytes(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	prefixes := []byte{0xff, 0x7f, 0x00, 0x80, 0x01, 0xf0}
	expected := make([]id.SessionID, 0, len(prefixes))
	for _, prefix := range prefixes {
		session := createExpiringTestSession(t, repo, prefix, &threshold)
		expected = append(expected, session.ID)
	}
	sort.Slice(expected, func(i, j int) bool {
		return bytes.Compare(expected[i][:], expected[j][:]) < 0
	})
	got, err := repo.ListExpiringSessions(context.Background(), threshold, storage.SessionExpiryCursor{}, len(expected))
	require.NoError(t, err)
	require.Len(t, got, len(expected))
	for i, session := range got {
		assert.Equal(t, expected[i], session.ID)
	}
}

func TestWithLockedSessionSeesLatestCommittedTokens(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	session := refreshTestSession(id.Principal("latest@example.com"), id.NewServiceID())
	require.NoError(t, repo.Create(ctx, session))
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			close(entered)
			<-release
			current.EncryptedAccessToken = []byte("committed-access")
			return true, nil
		})
		firstDone <- err
	}()
	<-entered
	seen := make(chan []byte, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			seen <- current.EncryptedAccessToken
			return false, nil
		})
		secondDone <- err
	}()
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	assert.Equal(t, []byte("committed-access"), <-seen)
}

func TestWithLockedSessionMissingDoesNotInvokeCallback(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	principal := id.Principal("missing@example.com")
	serviceID := id.NewServiceID()
	called := false
	got, err := repo.WithLockedSession(ctx, principal, serviceID, func(context.Context, *storage.UserSession) (bool, error) {
		called = true
		return true, nil
	})
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.False(t, called)
	stored, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Nil(t, stored)
}

func TestWithLockedSessionDoesNotWriteWhenDeclinedOrFailed(t *testing.T) {
	sentinel := errors.New("refresh failed")
	for _, tc := range []struct {
		name    string
		updated bool
		err     error
	}{
		{name: "declined"},
		{name: "failed", err: sentinel},
		{name: "failed_after_update_requested", updated: true, err: sentinel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewInMemoryUserSessionRepository()
			ctx := context.Background()
			session := refreshTestSession(id.Principal("declined@example.com"), id.NewServiceID())
			require.NoError(t, repo.Create(ctx, session))
			newExpiry := session.UpdatedAt.Add(time.Hour)
			returned, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				current.EncryptedAccessToken = []byte("uncommitted-access")
				current.EncryptedRefreshToken = []byte("uncommitted-refresh")
				current.AccessTokenExpiresAt = &newExpiry
				current.UpdatedAt = newExpiry
				return tc.updated, tc.err
			})
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				assert.Nil(t, returned)
			} else {
				require.NoError(t, err)
			}
			stored, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, session.EncryptedAccessToken, stored.EncryptedAccessToken)
			assert.Equal(t, session.EncryptedRefreshToken, stored.EncryptedRefreshToken)
			assert.Nil(t, stored.AccessTokenExpiresAt)
			assert.Equal(t, session.UpdatedAt, stored.UpdatedAt)
		})
	}
}

func TestWithLockedSessionPersistsTokenUpdateAtomically(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	session := refreshTestSession(id.Principal("atomic@example.com"), id.NewServiceID())
	require.NoError(t, repo.Create(ctx, session))
	newExpiry := session.UpdatedAt.Add(time.Hour)
	returned, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		current.EncryptedAccessToken = []byte("new-access")
		current.EncryptedRefreshToken = []byte("new-refresh")
		current.AccessTokenExpiresAt = &newExpiry
		current.UpdatedAt = newExpiry
		beforeCommit, err := repo.Get(ctx, session.ID)
		require.NoError(t, err)
		assert.Equal(t, session.EncryptedAccessToken, beforeCommit.EncryptedAccessToken)
		assert.Equal(t, session.EncryptedRefreshToken, beforeCommit.EncryptedRefreshToken)
		assert.Nil(t, beforeCommit.AccessTokenExpiresAt)
		assert.Equal(t, session.UpdatedAt, beforeCommit.UpdatedAt)
		return true, nil
	})
	require.NoError(t, err)
	require.NotNil(t, returned)
	stored, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-access"), stored.EncryptedAccessToken)
	assert.Equal(t, []byte("new-refresh"), stored.EncryptedRefreshToken)
	require.NotNil(t, stored.AccessTokenExpiresAt)
	assert.Equal(t, newExpiry, *stored.AccessTokenExpiresAt)
	assert.Equal(t, newExpiry, stored.UpdatedAt)
}
