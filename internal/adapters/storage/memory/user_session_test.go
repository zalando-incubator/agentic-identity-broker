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

func TestUserSessionRepository_ListingAndActiveSelectionReflectReauthentication(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	principal := id.Principal("user@example.com")
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name                     string
		expiry                   *time.Time
		hasRefreshToken          bool
		rejected                 bool
		requiresReauthentication bool
	}{
		{name: "expired access with refresh", expiry: &past, hasRefreshToken: true},
		{name: "rejected refresh with valid access", expiry: &future, hasRefreshToken: true, rejected: true, requiresReauthentication: true},
		{name: "valid access only", expiry: &future},
		{name: "expired access only", expiry: &past, requiresReauthentication: true},
		{name: "unknown access expiry without refresh"},
		{name: "rejected refresh without recorded access expiry", hasRefreshToken: true, rejected: true, requiresReauthentication: true},
	}
	expected := make(map[id.ServiceID]int, len(tests))
	var expectedActiveIDs []id.ServiceID
	for i, tc := range tests {
		session := refreshTestSession(principal, id.NewServiceID())
		session.AccessTokenExpiresAt = tc.expiry
		session.RefreshTokenRejected = tc.rejected
		if !tc.hasRefreshToken {
			session.EncryptedRefreshToken = nil
		}
		require.NoError(t, repo.Create(ctx, session))
		expected[session.ServiceID] = i
		if !tc.requiresReauthentication {
			expectedActiveIDs = append(expectedActiveIDs, session.ServiceID)
		}
	}
	require.NoError(t, repo.Create(ctx, refreshTestSession(id.Principal("other@example.com"), id.NewServiceID())))

	activeIDs, err := repo.ListActiveServiceIDsByPrincipal(ctx, principal)
	require.NoError(t, err)
	assert.ElementsMatch(t, expectedActiveIDs, activeIDs)
	activeSessions, err := repo.ListActiveByPrincipal(ctx, principal)
	require.NoError(t, err)
	var activeSessionIDs []id.ServiceID
	for _, session := range activeSessions {
		activeSessionIDs = append(activeSessionIDs, session.ServiceID)
	}
	assert.ElementsMatch(t, expectedActiveIDs, activeSessionIDs)
	sessions, err := repo.ListByPrincipal(ctx, principal)
	require.NoError(t, err)
	assert.Len(t, sessions, len(tests), "rejected and exhausted sessions remain visible")
	summaries, err := repo.ListSummariesByPrincipal(ctx, principal)
	require.NoError(t, err)
	require.Len(t, summaries, len(tests))
	for _, summary := range summaries {
		i, ok := expected[summary.ServiceID]
		require.True(t, ok)
		tc := tests[i]
		assert.Equal(t, tc.hasRefreshToken, summary.HasRefreshToken, tc.name)
		assert.Equal(t, tc.requiresReauthentication, summary.RequiresReauthentication, tc.name)
		assert.Equal(t, tc.expiry == &past, summary.AccessTokenExpired, tc.name)
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
	earlier := refreshTestSession(id.Principal("ciphertext@example.com"), id.NewServiceID())
	earlier.ID = expiryTestID(0x80)
	earlier.AccessTokenExpiresAt = &before
	earlier.EncryptedAccessToken = []byte{0, 0xff, 0x01, 0x80}
	earlier.EncryptedRefreshToken = []byte{0xfe, 0, 0xfd}
	require.NoError(t, repo.Create(ctx, earlier))
	atBoundary := createExpiringTestSession(t, repo, 0x02, &threshold)
	atBoundary.EncryptedRefreshToken = nil
	require.NoError(t, repo.Create(ctx, atBoundary))
	createExpiringTestSession(t, repo, 0x10, &after)
	createExpiringTestSession(t, repo, 0xf0, nil)

	got, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, []id.SessionID{earlier.ID, atBoundary.ID}, []id.SessionID{got[0].ID, got[1].ID})
	assert.Equal(t, earlier, got[0], "the candidate must include its complete encrypted session")
	assert.NotSame(t, earlier, got[0], "the candidate must be a snapshot")
	assert.Equal(t, atBoundary, got[1])
	assert.Empty(t, got[1].EncryptedRefreshToken, "missing refresh credentials must remain actionable sweep candidates")

	empty, err := repo.ListExpiringSessions(ctx, before.Add(-time.Second), storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	assert.Empty(t, empty)

	// Changing stored data after listing must not change the encrypted page snapshot.
	earlier.EncryptedAccessToken[0] = 0x42
	earlier.EncryptedRefreshToken[0] = 0x42
	earlier.Scope[0] = "changed"
	*earlier.AccessTokenExpiresAt = after
	assert.Equal(t, []byte{0, 0xff, 0x01, 0x80}, got[0].EncryptedAccessToken)
	assert.Equal(t, []byte{0xfe, 0, 0xfd}, got[0].EncryptedRefreshToken)
	assert.Equal(t, []string{"repo"}, got[0].Scope)
	assert.Equal(t, threshold.Add(-time.Hour), *got[0].AccessTokenExpiresAt)
}

func TestListExpiringSessionsKeysetPages(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	firstExpiry := threshold.Add(-3 * time.Hour)
	sharedExpiry := threshold.Add(-2 * time.Hour)
	laterExpiry := threshold.Add(-time.Hour)
	for _, item := range []struct {
		prefix   byte
		expiry   *time.Time
		rejected bool
	}{
		{0x80, &sharedExpiry, false}, {0x10, &threshold, false}, {0xf0, &firstExpiry, false},
		{0x40, &laterExpiry, false}, {0x02, &sharedExpiry, false},
		{0x01, &firstExpiry, true}, {0x60, &sharedExpiry, true}, {0xff, &threshold, true},
	} {
		session := createExpiringTestSession(t, repo, item.prefix, item.expiry)
		if item.rejected {
			session.RefreshTokenRejected = true
			require.NoError(t, repo.Create(ctx, session))
		}
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

func TestListExpiringSessionsExcludesRejectionBetweenPages(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	first := createExpiringTestSession(t, repo, 0x10, &threshold)
	rejected := createExpiringTestSession(t, repo, 0x20, &threshold)
	last := createExpiringTestSession(t, repo, 0x30, &threshold)
	last.EncryptedRefreshToken = nil
	require.NoError(t, repo.Create(ctx, last))

	page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, first.ID, page[0].ID)
	cursor := expiryTestCursor(page[0])
	_, err = repo.WithLockedSession(ctx, rejected.Principal, rejected.ServiceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
		session.RefreshTokenRejected = true
		return true, nil
	})
	require.NoError(t, err)

	page, err = repo.ListExpiringSessions(ctx, threshold, cursor, 2)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, last.ID, page[0].ID)
	assert.Empty(t, page[0].EncryptedRefreshToken)
	page, err = repo.ListExpiringSessions(ctx, threshold, expiryTestCursor(page[0]), 2)
	require.NoError(t, err)
	assert.Empty(t, page)
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

func TestListExpiringSessionsRejectsExpiryWithoutID(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	createExpiringTestSession(t, repo, 0x20, &threshold)
	got, err := repo.ListExpiringSessions(context.Background(), threshold, storage.SessionExpiryCursor{AccessTokenExpiresAt: threshold}, 1)
	assert.Empty(t, got)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}

func TestListExpiringSessionsContinuesAfterZeroExpiry(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	threshold := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	zero := time.Time{}
	createExpiringTestSession(t, repo, 0x20, &zero)
	createExpiringTestSession(t, repo, 0x10, &threshold)
	first, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, expiryTestID(0x20), first[0].ID)
	second, err := repo.ListExpiringSessions(ctx, threshold, expiryTestCursor(first[0]), 1)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, expiryTestID(0x10), second[0].ID)
	last, err := repo.ListExpiringSessions(ctx, threshold, expiryTestCursor(second[0]), 1)
	require.NoError(t, err)
	assert.Empty(t, last)
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
				current.RefreshTokenRejected = true
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
			assert.False(t, stored.RefreshTokenRejected)
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

func TestWithLockedSessionPersistsRejectionWithoutReplacingTokens(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	session := refreshTestSession(id.Principal("rejected@example.com"), id.NewServiceID())
	expiry := time.Now().Add(time.Hour)
	session.AccessTokenExpiresAt = &expiry
	require.NoError(t, repo.Create(ctx, session))
	expected := *session
	expected.RefreshTokenRejected = true
	expected.UpdatedAt = session.UpdatedAt.Add(time.Minute)

	returned, err := repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		assert.False(t, current.RefreshTokenRejected)
		current.RefreshTokenRejected = true
		current.UpdatedAt = expected.UpdatedAt
		return true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, &expected, returned)
	stored, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, &expected, stored)
	found, err := repo.FindByPrincipalAndService(ctx, session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, &expected, found)
	listed, err := repo.ListByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	assert.Equal(t, []*storage.UserSession{&expected}, listed)
	_, err = repo.WithLockedSession(ctx, session.Principal, session.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		assert.True(t, current.RefreshTokenRejected, "subsequent locked reads observe rejection")
		return false, nil
	})
	require.NoError(t, err)
}

func TestUserSessionRepository_UpsertResetsRecordedRejection(t *testing.T) {
	repo := NewInMemoryUserSessionRepository()
	ctx := context.Background()
	session := refreshTestSession(id.Principal("reauthorized@example.com"), id.NewServiceID())
	session.RefreshTokenRejected = true
	require.NoError(t, repo.Create(ctx, session))
	stored, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.True(t, stored.RefreshTokenRejected)

	recovered := refreshTestSession(session.Principal, session.ServiceID)
	recovered.EncryptedAccessToken = []byte("reauthorized-access")
	recovered.EncryptedRefreshToken = []byte("reauthorized-refresh")
	require.NoError(t, repo.Create(ctx, recovered))
	assert.Equal(t, session.ID, recovered.ID, "upsert keeps the existing session identity")
	stored, err = repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.False(t, stored.RefreshTokenRejected)
	assert.True(t, stored.CanRefresh())
	assert.False(t, stored.RequiresReauthentication())
	assert.Equal(t, recovered.EncryptedAccessToken, stored.EncryptedAccessToken)
	assert.Equal(t, recovered.EncryptedRefreshToken, stored.EncryptedRefreshToken)
	listed, err := repo.ListByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}
