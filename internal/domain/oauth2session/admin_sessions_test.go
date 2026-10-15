package oauth2session_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const maxPrincipalRunes = 200

func newAdminSessionsService(t *testing.T, principals ...string) (*oauth2session.SessionAdminService, []*storage.UserSession) {
	t.Helper()
	sessions := memory.NewInMemoryUserSessionRepository()
	created := make([]*storage.UserSession, 0, len(principals))
	for _, principal := range principals {
		session := &storage.UserSession{
			ID: id.NewSessionID(), Principal: id.Principal(principal), ServiceID: id.NewServiceID(),
			EncryptedAccessToken: []byte("ciphertext"), TokenType: "Bearer",
		}
		require.NoError(t, sessions.Create(context.Background(), session))
		created = append(created, session)
	}
	return oauth2session.NewSessionAdminService(sessions), created
}

func TestListSessions(t *testing.T) {
	svc, _ := newAdminSessionsService(t, "a@example.com", "a@example.com", "b@example.com")
	ctx := context.Background()

	list, err := svc.ListSessions(ctx, oauth2session.SessionListQuery{})
	require.NoError(t, err)
	assert.Len(t, list.Items, 3)
	assert.False(t, list.Truncated)
	assert.Empty(t, list.NextCursor)

	list, err = svc.ListSessions(ctx, oauth2session.SessionListQuery{Principal: "a@example.com"})
	require.NoError(t, err)
	assert.Len(t, list.Items, 2)

	list, err = svc.ListSessions(ctx, oauth2session.SessionListQuery{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, list.Items, 2)
	assert.True(t, list.Truncated)
	assert.NotEmpty(t, list.NextCursor)

	list, err = svc.ListSessions(ctx, oauth2session.SessionListQuery{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, list.Items, 3)
	assert.False(t, list.Truncated)
	assert.Empty(t, list.NextCursor)

	list, err = svc.ListSessions(ctx, oauth2session.SessionListQuery{Principal: "nobody@example.com"})
	require.NoError(t, err)
	assert.NotNil(t, list.Items)
	assert.Empty(t, list.Items)
}

func collectPages(t *testing.T, svc *oauth2session.SessionAdminService, query oauth2session.SessionListQuery) (all []*storage.SessionAdminSummary, pages int) {
	t.Helper()
	for {
		list, err := svc.ListSessions(context.Background(), query)
		require.NoError(t, err)
		all = append(all, list.Items...)
		pages++
		require.Less(t, pages, 50, "paging must terminate")
		if !list.Truncated {
			assert.Empty(t, list.NextCursor)
			return all, pages
		}
		require.NotEmpty(t, list.NextCursor)
		query.Cursor = list.NextCursor
	}
}

func TestListSessionsPagesThroughEverySessionOnce(t *testing.T) {
	svc, _ := newAdminSessionsService(t, "c@example.com", "a@example.com", "b@example.com", "a@example.com", "c@example.com", "d@example.com", "b@example.com")

	full, pages := collectPages(t, svc, oauth2session.SessionListQuery{Limit: 100})
	require.Len(t, full, 7)
	assert.Equal(t, 1, pages)

	for _, limit := range []int{1, 2, 3, 6, 7} {
		paged, _ := collectPages(t, svc, oauth2session.SessionListQuery{Limit: limit})
		require.Len(t, paged, len(full), "limit %d", limit)
		for i := range full {
			assert.Equal(t, full[i].ID, paged[i].ID, "limit %d position %d", limit, i)
		}
	}

	filtered, pages := collectPages(t, svc, oauth2session.SessionListQuery{Principal: "c@example.com", Limit: 1})
	require.Len(t, filtered, 2)
	assert.Equal(t, 2, pages)
	for _, item := range filtered {
		assert.Equal(t, id.Principal("c@example.com"), item.Principal)
	}
}

func TestListSessionsCursorSurvivesConcurrentChanges(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewInMemoryUserSessionRepository()
	create := func(principal string) *storage.UserSession {
		session := &storage.UserSession{
			ID: id.NewSessionID(), Principal: id.Principal(principal), ServiceID: id.NewServiceID(),
			EncryptedAccessToken: []byte("ciphertext"), TokenType: "Bearer",
		}
		require.NoError(t, repo.Create(ctx, session))
		return session
	}
	firstUser, secondUser := create("b@example.com"), create("c@example.com")
	svc := oauth2session.NewSessionAdminService(repo)

	first, err := svc.ListSessions(ctx, oauth2session.SessionListQuery{Limit: 1})
	require.NoError(t, err)
	require.True(t, first.Truncated)
	require.Equal(t, firstUser.ID, first.Items[0].ID)

	require.NoError(t, repo.Delete(ctx, firstUser.ID))
	inserted := create("a@example.com")
	later := create("d@example.com")

	rest, err := svc.ListSessions(ctx, oauth2session.SessionListQuery{Limit: 10, Cursor: first.NextCursor})
	require.NoError(t, err)
	require.Len(t, rest.Items, 2)
	assert.Equal(t, secondUser.ID, rest.Items[0].ID)
	assert.Equal(t, later.ID, rest.Items[1].ID)
	for _, item := range rest.Items {
		assert.NotEqual(t, inserted.ID, item.ID, "a session inserted before the cursor is not shown")
	}
}

func TestListSessionsRejectsInvalidQuery(t *testing.T) {
	svc, _ := newAdminSessionsService(t, "a@example.com")
	validCursor := func() string {
		svc, _ := newAdminSessionsService(t, "a@example.com", "b@example.com")
		list, err := svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Limit: 1})
		require.NoError(t, err)
		require.NotEmpty(t, list.NextCursor)
		return list.NextCursor
	}()
	for name, tc := range map[string]oauth2session.SessionListQuery{
		"negative limit":          {Limit: -1},
		"limit too big":           {Limit: oauth2session.MaxSessionListLimit + 1},
		"long principal":          {Principal: id.Principal(strings.Repeat("x", 201))},
		"cursor not base64":       {Cursor: "***"},
		"cursor too short":        {Cursor: "YQ"},
		"cursor too long":         {Cursor: strings.Repeat("A", 2000)},
		"cursor without a uuid":   {Cursor: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 40)))},
		"cursor for other filter": {Principal: "zzz@example.com", Cursor: validCursor},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ListSessions(context.Background(), tc)
			require.ErrorIs(t, err, oauth2session.ErrInvalidSessionQuery)
		})
	}

	_, err := svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Limit: oauth2session.MaxSessionListLimit + 1})
	require.ErrorIs(t, err, oauth2session.ErrInvalidSessionLimit)
}

func TestListSessionsCountsPrincipalCharactersNotBytes(t *testing.T) {
	svc, _ := newAdminSessionsService(t)
	_, err := svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Principal: id.Principal(strings.Repeat("é", 200))})
	require.NoError(t, err)
	_, err = svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Principal: id.Principal(strings.Repeat("é", 201))})
	require.ErrorIs(t, err, oauth2session.ErrInvalidSessionQuery)
}

type fixedItemsRepo struct {
	ports.UserSessionAdminRepository
	items []*storage.SessionAdminSummary
}

func (r fixedItemsRepo) ListAdminSummaries(_ context.Context, _ id.Principal, after storage.SessionAdminCursor, _ int) ([]*storage.SessionAdminSummary, error) {
	if after.IsZero() {
		return r.items, nil
	}
	return r.items[1:], nil
}

func TestListSessionsCursorRoundTripsMaximumLengthPrincipals(t *testing.T) {
	prefix := strings.Repeat("𝒜", maxPrincipalRunes-1)
	first, second := prefix+"a", prefix+"b"
	svc := oauth2session.NewSessionAdminService(fixedItemsRepo{items: []*storage.SessionAdminSummary{
		{ID: id.NewSessionID(), Principal: id.Principal(first), ServiceID: id.NewServiceID()},
		{ID: id.NewSessionID(), Principal: id.Principal(second), ServiceID: id.NewServiceID()},
	}})

	page, err := svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.True(t, page.Truncated)
	assert.Equal(t, id.Principal(first), page.Items[0].Principal)

	page, err = svc.ListSessions(context.Background(), oauth2session.SessionListQuery{Limit: 1, Cursor: page.NextCursor})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, id.Principal(second), page.Items[0].Principal)
	assert.False(t, page.Truncated)
}

func TestListSessionsRejectsPrincipalsTheStoreCannotHold(t *testing.T) {
	svc, _ := newAdminSessionsService(t, "a@example.com")
	cursorFor := func(principal string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(id.NewServiceID().String() + principal))
	}
	for name, tc := range map[string]oauth2session.SessionListQuery{
		"filter with NUL":                {Principal: id.Principal("a\x00b")},
		"filter that is only NUL":        {Principal: id.Principal("\x00")},
		"filter with invalid UTF-8":      {Principal: id.Principal("a\xffb")},
		"cursor principal with NUL":      {Cursor: cursorFor("a\x00b")},
		"cursor principal invalid UTF-8": {Cursor: cursorFor("a\xffb")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.ListSessions(context.Background(), tc)
			require.ErrorIs(t, err, oauth2session.ErrInvalidSessionQuery)
		})
	}
}

type nilItemsRepo struct {
	ports.UserSessionAdminRepository
}

func (nilItemsRepo) ListAdminSummaries(context.Context, id.Principal, storage.SessionAdminCursor, int) ([]*storage.SessionAdminSummary, error) {
	return nil, nil
}

func TestListSessionsNeverReturnsNilItems(t *testing.T) {
	list, err := oauth2session.NewSessionAdminService(nilItemsRepo{}).ListSessions(context.Background(), oauth2session.SessionListQuery{})
	require.NoError(t, err)
	assert.NotNil(t, list.Items)
	assert.Empty(t, list.Items)
}

func TestGetSession(t *testing.T) {
	svc, created := newAdminSessionsService(t, "a@example.com")

	got, err := svc.GetSession(context.Background(), created[0].ID)
	require.NoError(t, err)
	assert.Equal(t, created[0].ID, got.ID)
	assert.Equal(t, id.Principal("a@example.com"), got.Principal)

	_, err = svc.GetSession(context.Background(), id.NewSessionID())
	require.ErrorIs(t, err, oauth2session.ErrSessionNotFound)

	_, err = svc.GetSession(context.Background(), id.SessionID{})
	require.ErrorIs(t, err, oauth2session.ErrSessionNotFound)
}
