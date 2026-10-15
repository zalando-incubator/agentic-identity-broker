//go:build integration
// +build integration

package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupUserSessionTestDB creates an isolated migrated test database backed by the shared PostgreSQL container.
func setupUserSessionTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

// insertTestService inserts a minimal thirdparty_oauth2_services row to satisfy the
// user_sessions FK constraint without going through the full provider adapter.
func insertTestService(t *testing.T, adapter *Adapter, serviceID id.ServiceID) {
	t.Helper()
	ctx := context.Background()
	_, err := adapter.db.ExecContext(ctx, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, oauth2_flavor,
			 issuer_uri, enable_discovery, token_endpoint, authorize_endpoint, scopes)
		VALUES
			($1, 'Test Service', $2, '\x00', 'standard',
			 'https://example.com', false, 'https://example.com/token',
			 'https://example.com/authorize', '[]')
	`, serviceID, "test-client-"+serviceID.String()[:8])
	require.NoError(t, err)
}

func TestUserSessionRepository(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()

	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()

	t.Run("FindByPrincipalAndService scopes scan", func(t *testing.T) {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		principal := id.Principal("user-find-one@example.com")

		session := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal,
			ServiceID:            serviceID,
			EncryptedAccessToken: []byte("encrypted-token"),
			TokenType:            "Bearer",
			Scope:                []string{"genie"},
			EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:          time.Now().UTC(),
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		err := repo.Create(ctx, session)
		require.NoError(t, err, "Create should succeed")

		found, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
		require.NoError(t, err, "FindByPrincipalAndService must not return a scan error")
		require.NotNil(t, found)
		assert.Equal(t, []string{"genie"}, found.Scope, "scopes must round-trip through TEXT[]")
		assert.Equal(t, session.TokenType, found.TokenType)
		assert.Equal(t, session.Principal, found.Principal)
	})

	t.Run("FindByPrincipalAndService multiple scopes", func(t *testing.T) {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		principal := id.Principal("user-find-many@example.com")

		session := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal,
			ServiceID:            serviceID,
			EncryptedAccessToken: []byte("encrypted-token"),
			TokenType:            "Bearer",
			Scope:                []string{"sql-warehouses", "genie", "clusters"},
			EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:          time.Now().UTC(),
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		require.NoError(t, repo.Create(ctx, session))

		found, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.ElementsMatch(t, []string{"sql-warehouses", "genie", "clusters"}, found.Scope)
	})

	t.Run("Get scopes scan", func(t *testing.T) {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)

		session := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            id.Principal("user-get@example.com"),
			ServiceID:            serviceID,
			EncryptedAccessToken: []byte("encrypted-token"),
			TokenType:            "Bearer",
			Scope:                []string{"genie"},
			EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:          time.Now().UTC(),
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		require.NoError(t, repo.Create(ctx, session))

		found, err := repo.Get(ctx, session.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, []string{"genie"}, found.Scope)
	})

	t.Run("ListByPrincipal scopes scan", func(t *testing.T) {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		principal := id.Principal("user-list@example.com")

		session := &storage.UserSession{
			ID:                   id.NewSessionID(),
			Principal:            principal,
			ServiceID:            serviceID,
			EncryptedAccessToken: []byte("encrypted-token"),
			TokenType:            "Bearer",
			Scope:                []string{"genie"},
			EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt:          time.Now().UTC(),
			CreatedAt:            time.Now().UTC(),
			UpdatedAt:            time.Now().UTC(),
		}

		require.NoError(t, repo.Create(ctx, session))

		sessions, err := repo.ListByPrincipal(ctx, principal)
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, []string{"genie"}, sessions[0].Scope)
	})
	t.Run("summary and expiry reads preserve session visibility without tokens", func(t *testing.T) {
		principal := id.Principal("user-summary@example.com")
		activeServiceID, expiredServiceID := id.NewServiceID(), id.NewServiceID()
		insertTestService(t, adapter, activeServiceID)
		insertTestService(t, adapter, expiredServiceID)
		past := time.Now().Add(-time.Minute)
		for _, serviceID := range []id.ServiceID{activeServiceID, expiredServiceID} {
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
				EncryptedAccessToken: []byte("encrypted-token"), EncryptedRefreshToken: []byte("encrypted-refresh"),
				TokenType: "Bearer", Scope: []string{"repo", "email"},
				EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
				InitiatedAt:       time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			if serviceID == expiredServiceID {
				session.RefreshTokenExpiresAt = &past
			}
			require.NoError(t, repo.Create(ctx, session))
		}
		activeIDs, err := repo.ListActiveServiceIDsByPrincipal(ctx, principal)
		require.NoError(t, err)
		assert.Equal(t, []id.ServiceID{activeServiceID}, activeIDs)
		summaries, err := repo.ListSummariesByPrincipal(ctx, principal)
		require.NoError(t, err)
		require.Len(t, summaries, 2)
		byService := map[id.ServiceID]*storage.UserSessionSummary{}
		for _, summary := range summaries {
			byService[summary.ServiceID] = summary
		}
		assert.Equal(t, []string{"repo", "email"}, byService[activeServiceID].Scope)
		assert.True(t, byService[activeServiceID].HasRefreshToken)
		assert.False(t, byService[activeServiceID].IsExpired)
		assert.True(t, byService[expiredServiceID].IsExpired)
	})
}

func TestUserSessionRefreshLocksAcrossRepositories(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	ctx := context.Background()
	firstRepo := &PostgresUserSessionRepository{adapter: adapter}
	secondRepo := &PostgresUserSessionRepository{adapter: adapter}
	serviceID := id.NewServiceID()
	principal := id.Principal("refresh-user@example.com")
	insertTestService(t, adapter, serviceID)
	expired := time.Now().Add(-time.Hour)
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  []byte("old-access"),
		EncryptedRefreshToken: []byte("old-refresh"),
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &expired,
		Scope:                 []string{"repo"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           time.Now().UTC(),
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	require.NoError(t, firstRepo.Create(ctx, session))

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
	}()
	secondEntered := make(chan struct{}, 1)
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := firstRepo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			close(firstEntered)
			<-releaseFirst
			if string(current.EncryptedRefreshToken) != "old-refresh" {
				return false, errors.New("unexpected initial refresh token")
			}
			expires := time.Now().Add(time.Hour)
			current.EncryptedAccessToken = []byte("new-access")
			current.EncryptedRefreshToken = []byte("new-refresh")
			current.AccessTokenExpiresAt = &expires
			current.UpdatedAt = time.Now()
			return true, nil
		})
		firstDone <- err
	}()
	select {
	case <-firstEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not acquire the session")
	}
	go func() {
		_, err := secondRepo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			secondEntered <- struct{}{}
			if string(current.EncryptedRefreshToken) != "new-refresh" || !current.HasValidAccessToken() {
				return false, errors.New("second refresh observed a stale session")
			}
			return false, nil
		})
		secondDone <- err
	}()
	select {
	case <-secondEntered:
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)

	persisted, err := firstRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	assert.Equal(t, []byte("new-refresh"), persisted.EncryptedRefreshToken)
	assert.Equal(t, []byte("new-access"), persisted.EncryptedAccessToken)

	rejection := errors.New("upstream rejected refresh")
	_, err = firstRepo.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		current.EncryptedRefreshToken = []byte("discarded-refresh")
		return true, rejection
	})
	require.ErrorIs(t, err, rejection)
	persisted, err = secondRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-refresh"), persisted.EncryptedRefreshToken)
}

func TestUserSessionRefreshDoesNotWaitForProviderConnectionInsideTransaction(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	ctx := context.Background()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.Secret = model.NewAbsentSecret()
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.ProtectedResources = nil
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, NewPostgresThirdpartyOAuth2ProviderRepository(adapter).Create(ctx, provider))

	encryption := testutil.NewTestEncryptionAdapter(t)
	encryptionContext := domainencryption.NewServiceBranchKeySubject(provider.ID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("expired-access"), encryptionContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("old-refresh"), encryptionContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	now := time.Now().UTC()
	sessions := NewUserSessionRepository(adapter)
	principal := id.Principal("pool-refresh@example.com")
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: provider.ID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
		InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
	}))
	service := oauth2session.NewOAuth2SessionService(
		thirdparty.NewThirdpartyOAuth2ProviderService(NewPostgresThirdpartyOAuth2ProviderRepository(adapter), encryption, &noop.BranchKeyManager{}, nil, false, slog.Default()),
		sessions, sessions, nil, nil, encryption, &http.Client{Timeout: time.Second}, nil, oauth2session.DefaultConfig(), slog.Default(),
	)
	adapter.db.SetMaxOpenConns(1)
	refreshCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, token, err := service.GetValidAccessToken(refreshCtx, principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "new-access", token)
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
	require.NoError(t, err)
	rotated, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", rotated)
}

func TestUserSessionRefreshAllowsUpstreamWorkLongerThanStorageWriteTimeout(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	ctx := context.Background()
	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	principal := id.Principal("slow-refresh@example.com")
	expired := time.Now().Add(-time.Hour)
	now := time.Now().UTC()
	repo := NewUserSessionRepository(adapter)
	require.NoError(t, repo.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"), TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
	}))
	adapter.timeouts.Write = 250 * time.Millisecond
	refreshCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := repo.WithLockedSession(refreshCtx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
		select {
		case <-time.After(350 * time.Millisecond):
		case <-ctx.Done():
			return false, ctx.Err()
		}
		current.EncryptedAccessToken = []byte("new-access")
		current.EncryptedRefreshToken = []byte("new-refresh")
		current.UpdatedAt = time.Now()
		current.AccessTokenExpiresAt = nil
		return true, nil
	})
	require.NoError(t, err)
	persisted, err := repo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-refresh"), persisted.EncryptedRefreshToken)
}

func expiryTestID(n int) id.SessionID {
	return id.MustParseSessionID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

func seedExpiryTestSession(t *testing.T, adapter *Adapter, serviceID id.ServiceID, sessionID id.SessionID, expiry *time.Time) *storage.UserSession {
	t.Helper()
	now := time.Date(2030, time.January, 1, 12, 0, 0, 0, time.UTC)
	session := &storage.UserSession{
		ID:                    sessionID,
		Principal:             id.Principal(sessionID.String() + "@example.com"),
		ServiceID:             serviceID,
		EncryptedAccessToken:  []byte{0, 255, 17, 128},
		EncryptedRefreshToken: []byte{128, 1, 0, 254},
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  expiry,
		Scope:                 []string{"repo", "email"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	require.NoError(t, NewUserSessionRepository(adapter).Create(context.Background(), session))
	return session
}

func expirySessionIDs(sessions []*storage.UserSession) []id.SessionID {
	ids := make([]id.SessionID, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID
	}
	return ids
}

func expiryCursor(session *storage.UserSession) storage.SessionExpiryCursor {
	return storage.SessionExpiryCursor{AccessTokenExpiresAt: *session.AccessTokenExpiresAt, ID: session.ID}
}

func TestUserSessionListExpiringSessions(t *testing.T) {
	threshold := time.Date(2030, time.January, 1, 12, 0, 0, 0, time.UTC)
	due := threshold.Add(-time.Hour)
	future := threshold.Add(time.Second)
	ctx := context.Background()

	t.Run("E1 inclusive boundary and null exclusion", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(4), nil)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(3), &future)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &threshold)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &due)
		page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 10)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(2), expiryTestID(1)}, expirySessionIDs(page))
	})

	t.Run("E2 expiry then UUID keyset pages have no gaps or duplicates", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		early := due.Add(-time.Minute)
		late := due.Add(time.Minute)
		for _, item := range []struct {
			id     int
			expiry time.Time
		}{{5, due}, {4, early}, {3, due}, {2, early}, {1, late}} {
			seedExpiryTestSession(t, adapter, serviceID, expiryTestID(item.id), &item.expiry)
		}
		expected := []id.SessionID{expiryTestID(2), expiryTestID(4), expiryTestID(3), expiryTestID(5), expiryTestID(1)}
		all, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1000)
		require.NoError(t, err)
		require.Equal(t, expected, expirySessionIDs(all))
		for _, pageSize := range []int{1, 2} {
			t.Run(fmt.Sprintf("page_size_%d", pageSize), func(t *testing.T) {
				var combined []id.SessionID
				cursor := storage.SessionExpiryCursor{}
				for len(combined) < len(expected) {
					page, err := repo.ListExpiringSessions(ctx, threshold, cursor, pageSize)
					require.NoError(t, err)
					require.NotEmpty(t, page, "page ended before all due sessions were returned")
					combined = append(combined, expirySessionIDs(page)...)
					cursor = expiryCursor(page[len(page)-1])
				}
				assert.Equal(t, expected, combined)
				last, err := repo.ListExpiringSessions(ctx, threshold, cursor, pageSize)
				require.NoError(t, err)
				assert.Empty(t, last)
			})
		}
	})

	t.Run("E3 deleting or refreshing an earlier row does not skip later rows", func(t *testing.T) {
		for _, mutation := range []string{"delete", "refresh"} {
			t.Run(mutation, func(t *testing.T) {
				adapter, cleanup := setupUserSessionTestDB(t)
				defer cleanup()
				serviceID := id.NewServiceID()
				insertTestService(t, adapter, serviceID)
				repo := NewUserSessionRepository(adapter)
				early := due.Add(-time.Minute)
				seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
				seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &early)
				seedExpiryTestSession(t, adapter, serviceID, expiryTestID(3), &early)
				seedExpiryTestSession(t, adapter, serviceID, expiryTestID(4), &threshold)
				first, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
				require.NoError(t, err)
				require.Equal(t, []id.SessionID{expiryTestID(2)}, expirySessionIDs(first))
				if mutation == "delete" {
					_, err = adapter.db.ExecContext(ctx, `DELETE FROM user_sessions WHERE id = $1`, first[0].ID)
				} else {
					_, err = adapter.db.ExecContext(ctx, `UPDATE user_sessions SET access_token_expires_at = $1 WHERE id = $2`, future, first[0].ID)
				}
				require.NoError(t, err)
				cursor := expiryCursor(first[0])
				var remaining []id.SessionID
				for range 3 {
					page, err := repo.ListExpiringSessions(ctx, threshold, cursor, 1)
					require.NoError(t, err)
					require.Len(t, page, 1)
					remaining = append(remaining, page[0].ID)
					cursor = expiryCursor(page[0])
				}
				assert.Equal(t, []id.SessionID{expiryTestID(3), expiryTestID(1), expiryTestID(4)}, remaining)
				last, err := repo.ListExpiringSessions(ctx, threshold, cursor, 1)
				require.NoError(t, err)
				assert.Empty(t, last)
			})
		}
	})

	t.Run("E4 inserting at an earlier expiry never revisits the new row", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		early := due.Add(-time.Minute)
		late := due.Add(time.Minute)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &early)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(3), &due)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(5), &late)
		first, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 2)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{expiryTestID(1), expiryTestID(3)}, expirySessionIDs(first))
		// UUID 4 is after the cursor's UUID 3 but its expiry is behind the tuple.
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(4), &early)
		second, err := repo.ListExpiringSessions(ctx, threshold, expiryCursor(first[1]), 2)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(5)}, expirySessionIDs(second))
	})

	t.Run("E5 invalid page sizes and expiry-only cursors fail before database access", func(t *testing.T) {
		repo := NewUserSessionRepository(&Adapter{})
		for _, limit := range []int{-1, 0, 1001} {
			t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
				_, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, limit)
				require.Error(t, err, "limit %d must be rejected without a database connection", limit)
				var validation *storage.StorageError
				require.ErrorAs(t, err, &validation)
				assert.Equal(t, storage.ErrorKindValidation, validation.Kind)
			})
		}
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		partialRepo := NewUserSessionRepository(adapter)
		_, err := partialRepo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{AccessTokenExpiresAt: due}, 1)
		require.Error(t, err, "expiry without a session ID must be rejected")
		var validation *storage.StorageError
		require.ErrorAs(t, err, &validation)
		assert.Equal(t, storage.ErrorKindValidation, validation.Kind)
	})

	t.Run("E6 returned sessions preserve full ciphertext snapshots", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		original := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
		page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, original.ID, page[0].ID)
		assert.Equal(t, original.Principal, page[0].Principal)
		assert.Equal(t, original.ServiceID, page[0].ServiceID)
		assert.Equal(t, original.EncryptedAccessToken, page[0].EncryptedAccessToken)
		assert.Equal(t, original.EncryptedRefreshToken, page[0].EncryptedRefreshToken)
		assert.Equal(t, original.TokenType, page[0].TokenType)
		require.NotNil(t, page[0].AccessTokenExpiresAt)
		assert.True(t, original.AccessTokenExpiresAt.Equal(*page[0].AccessTokenExpiresAt))
		assert.Nil(t, page[0].RefreshTokenExpiresAt)
		assert.Equal(t, original.Scope, page[0].Scope)
		assert.Equal(t, original.EncryptionContext, page[0].EncryptionContext)
		assert.True(t, original.InitiatedAt.Equal(page[0].InitiatedAt))
		assert.True(t, original.CreatedAt.Equal(page[0].CreatedAt))
		assert.True(t, original.UpdatedAt.Equal(page[0].UpdatedAt))
	})

	t.Run("E7 UUID byte order breaks equal-expiry ties", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		ids := []id.SessionID{
			id.MustParseSessionID("f0000000-0000-4000-8000-000000000001"),
			id.MustParseSessionID("0f000000-0000-4000-8000-000000000001"),
			id.MustParseSessionID("10000000-0000-4000-8000-000000000001"),
			id.MustParseSessionID("0f000000-0000-4000-8000-0000000000ff"),
			id.MustParseSessionID("00000000-0000-4000-8000-000000000001"),
		}
		for _, sessionID := range ids {
			seedExpiryTestSession(t, adapter, serviceID, sessionID, &due)
		}
		sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })
		page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, len(ids))
		require.NoError(t, err)
		assert.Equal(t, ids, expirySessionIDs(page))
	})

	t.Run("E8 zero expiry is a due tuple that later pages continue past", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		zero := time.Time{}
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &zero)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
		first, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{expiryTestID(2)}, expirySessionIDs(first))
		require.NotNil(t, first[0].AccessTokenExpiresAt)
		assert.True(t, first[0].AccessTokenExpiresAt.IsZero())
		second, err := repo.ListExpiringSessions(ctx, threshold, expiryCursor(first[0]), 1)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(1)}, expirySessionIDs(second))
		last, err := repo.ListExpiringSessions(ctx, threshold, expiryCursor(second[0]), 1)
		require.NoError(t, err)
		assert.Empty(t, last)
	})
}

type expiryExplainPlan struct {
	NodeType         string              `json:"Node Type"`
	IndexName        string              `json:"Index Name"`
	IndexCond        string              `json:"Index Cond"`
	ActualRows       float64             `json:"Actual Rows"`
	SharedHitBlocks  int                 `json:"Shared Hit Blocks"`
	SharedReadBlocks int                 `json:"Shared Read Blocks"`
	Plans            []expiryExplainPlan `json:"Plans"`
}

func expiryPlanUsesIndex(plan expiryExplainPlan, indexName string) bool {
	if plan.NodeType == "Sort" || plan.NodeType == "Incremental Sort" {
		return false
	}
	if plan.IndexName == indexName && (plan.NodeType == "Index Scan" || plan.NodeType == "Index Only Scan") {
		return true
	}
	for _, child := range plan.Plans {
		if expiryPlanUsesIndex(child, indexName) {
			return true
		}
	}
	return false
}

func expiryIndexScan(plan *expiryExplainPlan, indexName string) *expiryExplainPlan {
	if plan.IndexName == indexName && (plan.NodeType == "Index Scan" || plan.NodeType == "Index Only Scan") {
		return plan
	}
	for i := range plan.Plans {
		if found := expiryIndexScan(&plan.Plans[i], indexName); found != nil {
			return found
		}
	}
	return nil
}

func TestUserSessionExpiryQueryUsesValidIndex(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	ctx := context.Background()
	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	threshold := time.Date(2030, time.January, 1, 12, 0, 0, 0, time.UTC)
	const indexName = "idx_user_sessions_access_token_expires_at"
	var valid bool
	var keyCount int
	var firstColumn, secondColumn, indexMethod string
	require.NoError(t, adapter.db.QueryRowxContext(ctx, `
		SELECT i.indisvalid, i.indnkeyatts,
			pg_get_indexdef(i.indexrelid, 1, true), pg_get_indexdef(i.indexrelid, 2, true), am.amname
		FROM pg_index AS i
		JOIN pg_class AS idx ON idx.oid = i.indexrelid
		JOIN pg_am AS am ON am.oid = idx.relam
		WHERE i.indexrelid = to_regclass($1)
	`, indexName).Scan(&valid, &keyCount, &firstColumn, &secondColumn, &indexMethod))
	require.True(t, valid, "%s must be valid", indexName)
	require.Equal(t, 2, keyCount, "%s must have two key columns", indexName)
	require.Equal(t, "access_token_expires_at", firstColumn)
	require.Equal(t, "id", secondColumn)
	require.Equal(t, "btree", indexMethod)

	// One sparse due range with distinct expiry tiers among 5000 sessions.
	_, err := adapter.db.ExecContext(ctx, `
		INSERT INTO user_sessions
			(principal, service_id, encrypted_access_token, encrypted_refresh_token, access_token_expires_at)
		SELECT 'planner-' || n || '@example.com', $1, $3::bytea, $4::bytea,
			CASE WHEN n <= 60 THEN $2::timestamptz - interval '1 day' + n * interval '1 minute'
			     WHEN n <= 110 THEN NULL
			     ELSE $2::timestamptz + interval '1 day' END
		FROM generate_series(1, 5000) AS n
	`, serviceID, threshold, []byte{0, 255, 17}, []byte{128, 1, 0})
	require.NoError(t, err)
	var total, dueCount, nullCount int
	require.NoError(t, adapter.db.QueryRowxContext(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE access_token_expires_at <= $1),
			COUNT(*) FILTER (WHERE access_token_expires_at IS NULL)
		FROM user_sessions
	`, threshold).Scan(&total, &dueCount, &nullCount))
	require.Equal(t, 5000, total)
	require.Equal(t, 60, dueCount)
	require.Equal(t, 50, nullCount)
	_, err = adapter.db.ExecContext(ctx, `ANALYZE user_sessions`)
	require.NoError(t, err)

	// Select real middle/late expiry tuples so each page starts inside the due range.
	var middleCursor, lateCursor storage.SessionExpiryCursor
	for _, page := range []struct {
		offset int
		cursor *storage.SessionExpiryCursor
	}{{24, &middleCursor}, {49, &lateCursor}} {
		require.NoError(t, adapter.db.QueryRowxContext(ctx, `
			SELECT access_token_expires_at, id FROM user_sessions WHERE access_token_expires_at <= $1
			ORDER BY access_token_expires_at, id OFFSET $2 LIMIT 1
		`, threshold, page.offset).Scan(&page.cursor.AccessTokenExpiresAt, &page.cursor.ID))
	}
	for _, page := range []struct {
		name   string
		cursor storage.SessionExpiryCursor
	}{{"first", storage.SessionExpiryCursor{}}, {"middle", middleCursor}, {"late", lateCursor}} {
		t.Run(page.name, func(t *testing.T) {
			var raw []byte
			query := listExpiringSessionsFirstPageQuery
			args := []any{threshold, 2}
			if !page.cursor.ID.IsZero() {
				query = listExpiringSessionsAfterCursorQuery
				args = []any{threshold, page.cursor.AccessTokenExpiresAt, page.cursor.ID, 2}
			}
			require.NoError(t, adapter.db.GetContext(ctx, &raw, `EXPLAIN (FORMAT JSON) `+query, args...))
			var explain []struct {
				Plan expiryExplainPlan `json:"Plan"`
			}
			require.NoError(t, json.Unmarshal(raw, &explain))
			require.Len(t, explain, 1)
			assert.True(t, expiryPlanUsesIndex(explain[0].Plan, indexName), "normal planner did not use %s for ordered %s page; plan: %s", indexName, page.name, raw)
			require.NoError(t, adapter.db.GetContext(ctx, &raw, `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) `+query, args...))
			require.NoError(t, json.Unmarshal(raw, &explain))
			require.Len(t, explain, 1)
			scan := expiryIndexScan(&explain[0].Plan, indexName)
			require.NotNil(t, scan, "actual plan must use the named index: %s", raw)
			assert.Equal(t, float64(2), scan.ActualRows, "each page returns only the requested rows: %s", raw)
			if !page.cursor.ID.IsZero() {
				assert.Contains(t, scan.IndexCond, "ROW(access_token_expires_at, id) > ROW(", "later pages must seek past the full cursor tuple")
			}
			t.Logf("%s page: index rows=%.0f, shared hit blocks=%d, shared read blocks=%d, condition=%s", page.name, scan.ActualRows, scan.SharedHitBlocks, scan.SharedReadBlocks, scan.IndexCond)
		})
	}
}

func TestUserSessionListAdminSummaries(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()

	now := time.Now().UTC()
	refreshExpiry := now.Add(-time.Hour)
	for _, tc := range []struct {
		principal string
		refresh   []byte
	}{
		{"admin-list-b@example.com", nil},
		{"admin-list-a@example.com", []byte("refresh")},
		{"admin-list-a@example.com", []byte("refresh")},
	} {
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		session := &storage.UserSession{
			ID: id.NewSessionID(), Principal: id.Principal(tc.principal), ServiceID: serviceID,
			EncryptedAccessToken: []byte("access"), EncryptedRefreshToken: tc.refresh, TokenType: "Bearer",
			Scope: []string{"repo"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
			InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if tc.refresh != nil {
			session.RefreshTokenExpiresAt = &refreshExpiry
		}
		require.NoError(t, repo.Create(ctx, session))
	}

	all, err := repo.ListAdminSummaries(ctx, "", storage.SessionAdminCursor{}, 100)
	require.NoError(t, err)
	require.Len(t, all, 3, "setupMigratedAdapter returns an isolated database")
	assert.Equal(t, id.Principal("admin-list-a@example.com"), all[0].Principal)
	assert.Equal(t, id.Principal("admin-list-a@example.com"), all[1].Principal)
	assert.Equal(t, id.Principal("admin-list-b@example.com"), all[2].Principal)
	assert.True(t, all[0].ServiceID.String() < all[1].ServiceID.String(), "ties on principal order by service_id")
	assert.True(t, all[0].HasRefreshToken)
	assert.True(t, all[0].IsExpired)
	assert.False(t, all[2].HasRefreshToken)
	assert.Equal(t, []string{"repo"}, all[0].Scope)

	filtered, err := repo.ListAdminSummaries(ctx, "admin-list-a@example.com", storage.SessionAdminCursor{}, 100)
	require.NoError(t, err)
	require.Len(t, filtered, 2)
	assert.True(t, filtered[0].ServiceID.String() < filtered[1].ServiceID.String())

	caseMismatch, err := repo.ListAdminSummaries(ctx, "ADMIN-LIST-A@example.com", storage.SessionAdminCursor{}, 100)
	require.NoError(t, err)
	assert.Empty(t, caseMismatch)

	limited, err := repo.ListAdminSummaries(ctx, "", storage.SessionAdminCursor{}, 1)
	require.NoError(t, err)
	assert.Len(t, limited, 1)

	_, err = repo.ListAdminSummaries(ctx, "", storage.SessionAdminCursor{}, 0)
	require.Error(t, err)

	var walked []id.SessionID
	cursor := storage.SessionAdminCursor{}
	for range 5 {
		page, err := repo.ListAdminSummaries(ctx, "", cursor, 1)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		walked = append(walked, page[0].ID)
		cursor = storage.SessionAdminCursor{Principal: page[0].Principal, ServiceID: page[0].ServiceID}
	}
	require.Len(t, walked, 3)
	for i := range all {
		assert.Equal(t, all[i].ID, walked[i], "walking with a one-row keyset page matches the full listing")
	}

	afterFirstA, err := repo.ListAdminSummaries(ctx, "admin-list-a@example.com", storage.SessionAdminCursor{Principal: all[0].Principal, ServiceID: all[0].ServiceID}, 100)
	require.NoError(t, err)
	require.Len(t, afterFirstA, 1)
	assert.Equal(t, all[1].ID, afterFirstA[0].ID)

	afterLast, err := repo.ListAdminSummaries(ctx, "", storage.SessionAdminCursor{Principal: all[2].Principal, ServiceID: all[2].ServiceID}, 100)
	require.NoError(t, err)
	assert.Empty(t, afterLast)
}

func TestUserSessionGetAdminSummary(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()

	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	now := time.Now().UTC()
	accessExpiry := now.Add(-time.Minute)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: "admin-get@example.com", ServiceID: serviceID,
		EncryptedAccessToken: []byte("access"), EncryptedRefreshToken: []byte("refresh"), TokenType: "Bearer",
		AccessTokenExpiresAt: &accessExpiry, Scope: []string{},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, session))

	got, err := repo.GetAdminSummary(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, got.ID)
	assert.Equal(t, id.Principal("admin-get@example.com"), got.Principal)
	assert.Equal(t, serviceID, got.ServiceID)
	assert.True(t, got.HasRefreshToken)
	assert.True(t, got.AccessTokenExpired)
	assert.False(t, got.IsExpired)
	assert.NotNil(t, got.Scope)

	for _, missing := range []id.SessionID{id.NewSessionID(), {}} {
		_, err = repo.GetAdminSummary(ctx, missing)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	}
}

type adminListPlanShape struct {
	ordered      bool
	compositeIdx bool
	serviceIdx   bool
	sorts        bool
	seqScan      bool
}

func inspectAdminListPlan(plan expiryExplainPlan, shape *adminListPlanShape) {
	isComposite := plan.IndexName == "user_sessions_principal_service_unique" || plan.IndexName == "idx_user_sessions_principal_service"
	switch plan.NodeType {
	case "Sort", "Incremental Sort":
		shape.sorts = true
	case "Seq Scan":
		shape.seqScan = true
	case "Index Scan", "Index Only Scan":
		shape.ordered = shape.ordered || isComposite
		shape.compositeIdx = shape.compositeIdx || isComposite
		shape.serviceIdx = shape.serviceIdx || plan.IndexName == "idx_user_sessions_service_id"
	case "Bitmap Index Scan":
		shape.compositeIdx = shape.compositeIdx || isComposite
		shape.serviceIdx = shape.serviceIdx || plan.IndexName == "idx_user_sessions_service_id"
	}
	for _, child := range plan.Plans {
		inspectAdminListPlan(child, shape)
	}
}

// Listings across all users must read the (principal, service_id) index in order without a sort.
// Listings for one principal must be bounded by that principal in the composite index. A sort there
// covers at most one row per service.
func TestUserSessionAdminListQueriesUseIndex(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	ctx := context.Background()

	serviceIDs := make([]id.ServiceID, 5)
	for i := range serviceIDs {
		serviceIDs[i] = id.MustParseServiceID(fmt.Sprintf("b0000000-0000-4000-8000-%012d", i+1))
		insertTestService(t, adapter, serviceIDs[i])
		_, err := adapter.db.ExecContext(ctx, `
			INSERT INTO user_sessions (principal, service_id, encrypted_access_token)
			SELECT 'planner-' || n || '@example.com', $1, $2::bytea
			FROM generate_series(1, 1000) AS n
		`, serviceIDs[i], []byte{1})
		require.NoError(t, err)
	}
	_, err := adapter.db.ExecContext(ctx, `ANALYZE user_sessions`)
	require.NoError(t, err)

	midPrincipal := id.Principal("planner-500@example.com")
	midCursor := storage.SessionAdminCursor{Principal: midPrincipal, ServiceID: serviceIDs[2]}
	for _, tc := range []struct {
		name        string
		principal   id.Principal
		after       storage.SessionAdminCursor
		want        string
		wantOrdered bool
		wantNoSort  bool
	}{
		{"unfiltered", "", storage.SessionAdminCursor{}, listAdminSummariesAllQuery, true, true},
		{"unfiltered after cursor", "", midCursor, listAdminSummariesAllAfterQuery, true, true},
		{"by principal", midPrincipal, storage.SessionAdminCursor{}, listAdminSummariesByPrincipalQuery, false, false},
		{"by principal after cursor", midPrincipal, midCursor, listAdminSummariesByPrincipalAfterQuery, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query, args := adminListStatement(tc.principal, tc.after, 101)
			require.Equal(t, tc.want, query)
			var raw []byte
			require.NoError(t, adapter.db.GetContext(ctx, &raw, `EXPLAIN (FORMAT JSON) `+query, args...))
			var explain []struct {
				Plan expiryExplainPlan `json:"Plan"`
			}
			require.NoError(t, json.Unmarshal(raw, &explain))
			require.Len(t, explain, 1)
			var shape adminListPlanShape
			inspectAdminListPlan(explain[0].Plan, &shape)
			assert.True(t, shape.compositeIdx, "plan must use the (principal, service_id) index: %s", raw)
			assert.False(t, shape.serviceIdx, "plan must not drive from the service_id index: %s", raw)
			assert.False(t, shape.seqScan, "plan must not scan the whole table: %s", raw)
			if tc.wantOrdered {
				assert.True(t, shape.ordered, "plan must read the index in order: %s", raw)
			}
			if tc.wantNoSort {
				assert.False(t, shape.sorts, "plan must not sort: %s", raw)
			}
		})
	}
}
