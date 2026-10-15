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
	"sync/atomic"
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

func setupUserSessionTestDBAtVersion(t *testing.T, version int) (*Adapter, func()) {
	t.Helper()
	connString, cleanup := setupDatabaseFromTemplate(t, fmt.Sprintf("session_state_%d", version), func(t *testing.T, dbName string) {
		applyMigrationsUpToDatabase(t, requireSharedTestContainer(t), dbName, version)
	})
	adapter, err := NewAdapter(testStorageConfig(connString))
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, adapter.Initialize(ctx))
	return adapter, func() {
		require.NoError(t, adapter.Close(ctx))
		cleanup()
	}
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
	t.Run("summaries and active selectors reflect observed session validity", func(t *testing.T) {
		principal := id.Principal("user-summary@example.com")
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
			serviceID := id.NewServiceID()
			insertTestService(t, adapter, serviceID)
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
				EncryptedAccessToken: []byte("encrypted-token"), TokenType: "Bearer",
				AccessTokenExpiresAt: tc.expiry, RefreshTokenRejected: tc.rejected,
				Scope: []string{"repo", "email"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
				InitiatedAt: time.Now().UTC(), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}
			if tc.hasRefreshToken {
				session.EncryptedRefreshToken = []byte("encrypted-refresh")
			}
			require.NoError(t, repo.Create(ctx, session))
			expected[serviceID] = i
			if !tc.requiresReauthentication {
				expectedActiveIDs = append(expectedActiveIDs, serviceID)
			}
		}
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
			assert.Equal(t, []string{"repo", "email"}, summary.Scope)
			assert.Equal(t, tc.hasRefreshToken, summary.HasRefreshToken, tc.name)
			assert.Equal(t, tc.requiresReauthentication, summary.RequiresReauthentication, tc.name)
			assert.Equal(t, tc.expiry == &past, summary.AccessTokenExpired, tc.name)
		}
	})
}

func TestUserSessionRepository_HealthyOperationsAcrossOnlinePhases(t *testing.T) {
	for _, version := range []int{38, 39} {
		t.Run(fmt.Sprintf("migration %d", version), func(t *testing.T) {
			adapter, cleanup := setupUserSessionTestDBAtVersion(t, version)
			defer cleanup()
			ctx := context.Background()
			repo := NewUserSessionRepository(adapter)
			principal := id.Principal("online-session@example.com")
			now := time.Now().UTC().Truncate(time.Microsecond)
			past, future := now.Add(-time.Hour), now.Add(time.Hour)
			sessions := []*storage.UserSession{
				{
					ID: id.NewSessionID(), Principal: principal, ServiceID: id.NewServiceID(),
					EncryptedAccessToken: []byte{0, 255, 17}, EncryptedRefreshToken: []byte{128, 1, 0},
					TokenType: "Bearer", AccessTokenExpiresAt: &past, Scope: []string{"repo", "email"},
					InitiatedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now,
				},
				{
					ID: id.NewSessionID(), Principal: principal, ServiceID: id.NewServiceID(),
					EncryptedAccessToken: []byte("access-only"), TokenType: "Bearer",
					AccessTokenExpiresAt: &future, Scope: []string{},
					InitiatedAt: now.Add(-time.Hour), CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
				},
			}
			for i, session := range sessions {
				insertTestService(t, adapter, session.ServiceID)
				session.EncryptionContext = storage.EncryptionContext{ServiceID: session.ServiceID}
				require.NoError(t, repo.Create(ctx, session))
				stored, err := repo.Get(ctx, session.ID)
				require.NoError(t, err)
				require.NotNil(t, stored.AccessTokenExpiresAt)
				require.True(t, session.AccessTokenExpiresAt.Equal(*stored.AccessTokenExpiresAt))
				require.True(t, session.InitiatedAt.Equal(stored.InitiatedAt))
				require.True(t, session.CreatedAt.Equal(stored.CreatedAt))
				require.True(t, session.UpdatedAt.Equal(stored.UpdatedAt))
				expected := *session
				expected.AccessTokenExpiresAt, expected.InitiatedAt = stored.AccessTokenExpiresAt, stored.InitiatedAt
				expected.CreatedAt, expected.UpdatedAt = stored.CreatedAt, stored.UpdatedAt
				assert.Equal(t, &expected, stored, "explicit view projection preserves all session fields")
				sessions[i] = stored
				found, err := repo.FindByPrincipalAndService(ctx, principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, stored, found)
			}
			other := *sessions[0]
			other.ID = id.NewSessionID()
			other.Principal = id.Principal("other-online-session@example.com")
			other.AccessTokenExpiresAt = nil
			require.NoError(t, repo.Create(ctx, &other))

			listed, err := repo.ListByPrincipal(ctx, principal)
			require.NoError(t, err)
			assert.Equal(t, []*storage.UserSession{sessions[1], sessions[0]}, listed)
			active, err := repo.ListActiveByPrincipal(ctx, principal)
			require.NoError(t, err)
			assert.Equal(t, listed, active)
			activeIDs, err := repo.ListActiveServiceIDsByPrincipal(ctx, principal)
			require.NoError(t, err)
			assert.Equal(t, []id.ServiceID{sessions[1].ServiceID, sessions[0].ServiceID}, activeIDs)
			summaries, err := repo.ListSummariesByPrincipal(ctx, principal)
			require.NoError(t, err)
			require.Len(t, summaries, 2)
			assert.Equal(t, sessions[1].ID, summaries[0].ID)
			assert.False(t, summaries[0].HasRefreshToken)
			assert.False(t, summaries[0].RequiresReauthentication)
			assert.Equal(t, sessions[0].ID, summaries[1].ID)
			assert.True(t, summaries[1].AccessTokenExpired)
			assert.True(t, summaries[1].HasRefreshToken)
			assert.False(t, summaries[1].RequiresReauthentication, "token-free projection must use SQL refresh presence")

			firstPage, err := repo.ListExpiringSessions(ctx, future, storage.SessionExpiryCursor{}, 1)
			require.NoError(t, err)
			require.Equal(t, []*storage.UserSession{sessions[0]}, firstPage)
			secondPage, err := repo.ListExpiringSessions(ctx, future, expiryCursor(firstPage[0]), 1)
			require.NoError(t, err)
			assert.Equal(t, []*storage.UserSession{sessions[1]}, secondPage)

			callbackErr := errors.New("discard state change")
			_, err = repo.WithLockedSession(ctx, principal, sessions[0].ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				current.EncryptedAccessToken = []byte("discarded")
				return true, callbackErr
			})
			require.ErrorIs(t, err, callbackErr)
			_, err = repo.WithLockedSession(ctx, principal, sessions[0].ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				current.RefreshTokenRejected = true
				return false, nil
			})
			require.NoError(t, err, "unchanged callbacks must not invoke a write function")
			stored, err := repo.Get(ctx, sessions[0].ID)
			require.NoError(t, err)
			assert.Equal(t, sessions[0], stored, "failed and unchanged callbacks must not persist")
			missing, err := repo.WithLockedSession(ctx, id.Principal("missing@example.com"), sessions[0].ServiceID, func(context.Context, *storage.UserSession) (bool, error) {
				t.Fatal("missing sessions must not invoke the callback or insert a row")
				return false, nil
			})
			require.NoError(t, err)
			assert.Nil(t, missing)

			newExpiry := future.Add(time.Hour)
			updated, err := repo.WithLockedSession(ctx, principal, sessions[0].ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				assert.Equal(t, sessions[0], current)
				_, lockErr := adapter.db.ExecContext(ctx, `SELECT id FROM user_sessions WHERE id = $1 FOR UPDATE NOWAIT`, current.ID)
				require.Error(t, lockErr, "view FOR UPDATE must hold the original row lock")
				var stateError interface{ SQLState() string }
				require.ErrorAs(t, lockErr, &stateError)
				assert.Equal(t, "55P03", stateError.SQLState(), "the original row remains locked in every phase")
				current.EncryptedAccessToken = []byte("new-access")
				current.EncryptedRefreshToken = []byte("new-refresh")
				current.AccessTokenExpiresAt = &newExpiry
				current.UpdatedAt = now.Add(time.Minute)
				return true, nil
			})
			require.NoError(t, err)
			require.NotNil(t, updated)
			stored, err = repo.Get(ctx, sessions[0].ID)
			require.NoError(t, err)
			require.NotNil(t, stored)
			require.NotNil(t, stored.AccessTokenExpiresAt)
			require.True(t, updated.AccessTokenExpiresAt.Equal(*stored.AccessTokenExpiresAt))
			updated.AccessTokenExpiresAt = stored.AccessTokenExpiresAt
			updated.UpdatedAt = stored.UpdatedAt // The existing table trigger records the write time.
			assert.Equal(t, updated, stored, "locked function update preserves untouched metadata")

			reauthorized := *stored
			reauthorized.ID = id.NewSessionID()
			reauthorized.EncryptedAccessToken = []byte("reauthorized-access")
			reauthorized.Scope = []string{"new-scope"}
			reauthorized.InitiatedAt = now
			reauthorized.CreatedAt = now
			require.NoError(t, repo.Create(ctx, &reauthorized))
			stored, err = repo.FindByPrincipalAndService(ctx, principal, sessions[0].ServiceID)
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, sessions[0].ID, stored.ID)
			assert.Equal(t, sessions[0].InitiatedAt, stored.InitiatedAt)
			assert.Equal(t, sessions[0].CreatedAt, stored.CreatedAt)
			assert.Equal(t, reauthorized.EncryptedAccessToken, stored.EncryptedAccessToken)
			assert.Equal(t, reauthorized.Scope, stored.Scope)
			assert.False(t, stored.RefreshTokenRejected)

			require.NoError(t, repo.Delete(ctx, sessions[1].ID))
			require.NoError(t, repo.DeleteByPrincipalAndService(ctx, principal, sessions[0].ServiceID))
			count, err := repo.CountByService(ctx, sessions[0].ServiceID)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "termination must not delete another principal's row")
			require.NoError(t, repo.Delete(ctx, other.ID))
			count, err = repo.CountByService(ctx, sessions[0].ServiceID)
			require.NoError(t, err)
			assert.Zero(t, count)
		})
	}
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
		current.RefreshTokenRejected = true
		return true, rejection
	})
	require.ErrorIs(t, err, rejection)
	persisted, err = secondRepo.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("new-refresh"), persisted.EncryptedRefreshToken)
	assert.False(t, persisted.RefreshTokenRejected)
}

func TestUserSessionRefreshDoesNotWaitForProviderConnectionInsideTransaction(t *testing.T) {
	for _, version := range []int{38, 39} {
		t.Run(fmt.Sprintf("migration %d", version), func(t *testing.T) {
			adapter, cleanup := setupUserSessionTestDBAtVersion(t, version)
			defer cleanup()
			ctx := context.Background()
			provider := newTestEntity()
			provider.ID = id.NewServiceID()
			provider.Secret = model.NewAbsentSecret()
			provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			provider.ProtectedResources = nil
			var rejectRefresh atomic.Bool
			var providerCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				providerCalls.Add(1)
				if rejectRefresh.Load() {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
					return
				}
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

			rejectRefresh.Store(true)
			summary, err := service.ForceRefreshSession(refreshCtx, principal, provider.ID)
			require.Error(t, err)
			assert.Nil(t, summary)
			var operationError *oauth2session.OperationError
			require.ErrorAs(t, err, &operationError)
			if version == 38 {
				assert.Equal(t, oauth2session.DetailPersistenceFailed, operationError.Metadata().Detail())
				assert.Equal(t, oauth2session.DependencySessionRepository, operationError.Metadata().Dependency())
				assert.NotErrorIs(t, err, oauth2session.ErrRefreshRejected, "preparation cannot claim the provider rejection was persisted")
				assert.NotErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
			} else {
				assert.Equal(t, oauth2session.DetailRefreshRejected, operationError.Metadata().Detail())
				assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
				assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
			}
			after, err := sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
			require.NoError(t, err)
			if version == 38 {
				assert.Equal(t, persisted, after, "failed rejection writes preserve ciphertext, access expiry, and all row metadata")
			} else {
				assert.True(t, after.RefreshTokenRejected)
				expected := *persisted
				expected.RefreshTokenRejected = true
				expected.UpdatedAt = after.UpdatedAt
				assert.Equal(t, &expected, after, "activation persists only observed rejection and its update timestamp")
			}
			_, token, err = service.GetValidAccessToken(refreshCtx, principal, provider.ID)
			if version == 38 {
				require.NoError(t, err)
				assert.Equal(t, "new-access", token, "the preparation row remains usable because rejection did not commit")
			} else {
				require.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
				assert.Empty(t, token, "activation must stop issuance even though access has not expired")
			}
			assert.Equal(t, int32(2), providerCalls.Load(), "acquisition must not make another provider request in either phase")
		})
	}
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
		atBoundary := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &threshold)
		atBoundary.EncryptedRefreshToken = nil
		require.NoError(t, repo.Create(ctx, atBoundary))
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &due)
		page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 10)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{expiryTestID(2), expiryTestID(1)}, expirySessionIDs(page))
		assert.Empty(t, page[1].EncryptedRefreshToken, "missing refresh credentials must remain actionable sweep candidates")
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
			id       int
			expiry   time.Time
			rejected bool
		}{
			{5, due, false}, {4, early, false}, {3, due, false}, {2, early, false}, {1, late, false},
			{0, early, true}, {6, early, true}, {7, threshold, true},
		} {
			session := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(item.id), &item.expiry)
			if item.rejected {
				session.RefreshTokenRejected = true
				require.NoError(t, repo.Create(ctx, session))
			}
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

	t.Run("E9 rejection between pages excludes the row without hiding missing refresh credentials", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		first := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
		rejected := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &due)
		last := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(3), &due)
		last.EncryptedRefreshToken = nil
		require.NoError(t, repo.Create(ctx, last))

		page, err := repo.ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{first.ID}, expirySessionIDs(page))
		cursor := expiryCursor(page[0])
		_, err = repo.WithLockedSession(ctx, rejected.Principal, rejected.ServiceID, func(_ context.Context, session *storage.UserSession) (bool, error) {
			session.RefreshTokenRejected = true
			return true, nil
		})
		require.NoError(t, err)

		page, err = repo.ListExpiringSessions(ctx, threshold, cursor, 2)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{last.ID}, expirySessionIDs(page))
		assert.Empty(t, page[0].EncryptedRefreshToken)
		page, err = repo.ListExpiringSessions(ctx, threshold, expiryCursor(page[0]), 2)
		require.NoError(t, err)
		assert.Empty(t, page)
	})
}

type expiryExplainPlan struct {
	NodeType         string              `json:"Node Type"`
	RelationName     string              `json:"Relation Name"`
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
			(principal, service_id, encrypted_access_token, encrypted_refresh_token, access_token_expires_at, refresh_token_rejected)
		SELECT 'planner-' || n || '@example.com', $1, $3::bytea, $4::bytea,
			CASE WHEN n <= 60 THEN $2::timestamptz - interval '1 day' + n * interval '1 minute'
			     WHEN n <= 110 THEN NULL
			     ELSE $2::timestamptz + interval '1 day' END,
			n <= 60 AND n % 10 = 0
		FROM generate_series(1, 5000) AS n
	`, serviceID, threshold, []byte{0, 255, 17}, []byte{128, 1, 0})
	require.NoError(t, err)
	var total, dueCount, nullCount, rejectedCount int
	require.NoError(t, adapter.db.QueryRowxContext(ctx, `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE access_token_expires_at <= $1),
			COUNT(*) FILTER (WHERE access_token_expires_at IS NULL),
			COUNT(*) FILTER (WHERE refresh_token_rejected)
		FROM user_sessions
	`, threshold).Scan(&total, &dueCount, &nullCount, &rejectedCount))
	require.Equal(t, 5000, total)
	require.Equal(t, 60, dueCount)
	require.Equal(t, 50, nullCount)
	require.Equal(t, 6, rejectedCount)
	candidates, err := NewUserSessionRepository(adapter).ListExpiringSessions(ctx, threshold, storage.SessionExpiryCursor{}, 1000)
	require.NoError(t, err)
	require.Len(t, candidates, dueCount-rejectedCount)
	for _, session := range candidates {
		require.False(t, session.RefreshTokenRejected, "the indexed view query must exclude recorded rejection")
	}
	_, err = adapter.db.ExecContext(ctx, `ANALYZE user_sessions`)
	require.NoError(t, err)

	// Select real middle/late expiry tuples so each page starts inside the due range.
	var middleCursor, lateCursor storage.SessionExpiryCursor
	for _, page := range []struct {
		offset int
		cursor *storage.SessionExpiryCursor
	}{{24, &middleCursor}, {49, &lateCursor}} {
		require.NoError(t, adapter.db.QueryRowxContext(ctx, `
			SELECT access_token_expires_at, id FROM user_sessions
			WHERE access_token_expires_at <= $1 AND NOT refresh_token_rejected
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
			assert.Equal(t, "user_sessions", scan.RelationName, "stable view must retain the original table's index scan")
			assert.Equal(t, float64(2), scan.ActualRows, "each page returns only the requested rows: %s", raw)
			if !page.cursor.ID.IsZero() {
				assert.Contains(t, scan.IndexCond, "ROW(access_token_expires_at, id) > ROW(", "later pages must seek past the full cursor tuple")
			}
			t.Logf("%s page: index rows=%.0f, shared hit blocks=%d, shared read blocks=%d, condition=%s", page.name, scan.ActualRows, scan.SharedHitBlocks, scan.SharedReadBlocks, scan.IndexCond)
		})
	}
}

func TestUserSessionRepository_RejectionPersistenceAndReauthorization(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDB(t)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()
	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(time.Hour)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal("rejected@example.com"), ServiceID: serviceID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"),
		TokenType: "Bearer", AccessTokenExpiresAt: &expiry, Scope: []string{"repo", "email"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, session))
	initial, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	expected := *initial
	expected.RefreshTokenRejected = true
	expected.UpdatedAt = initial.UpdatedAt.Add(time.Minute)

	callbackErr := errors.New("callback failed")
	for _, tc := range []struct {
		name    string
		updated bool
		err     error
	}{
		{name: "declined"},
		{name: "failed", err: callbackErr},
		{name: "failed after write requested", updated: true, err: callbackErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.WithLockedSession(ctx, session.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
				current.RefreshTokenRejected = true
				current.UpdatedAt = expected.UpdatedAt
				return tc.updated, tc.err
			})
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}
			stored, err := repo.Get(ctx, session.ID)
			require.NoError(t, err)
			assert.Equal(t, initial, stored)
		})
	}

	returned, err := repo.WithLockedSession(ctx, session.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		assert.False(t, current.RefreshTokenRejected)
		current.RefreshTokenRejected = true
		current.UpdatedAt = expected.UpdatedAt
		return true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, &expected, returned)
	stored, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.True(t, stored.UpdatedAt.After(initial.UpdatedAt), "database trigger records the persisted state change")
	expected.UpdatedAt = stored.UpdatedAt
	assert.Equal(t, &expected, stored, "rejection retains ciphertext and real token metadata")
	found, err := repo.FindByPrincipalAndService(ctx, session.Principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, &expected, found)
	listed, err := repo.ListByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	assert.Equal(t, []*storage.UserSession{&expected}, listed)
	_, err = repo.WithLockedSession(ctx, session.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		assert.True(t, current.RefreshTokenRejected, "subsequent locked reads observe rejection")
		return false, nil
	})
	require.NoError(t, err)

	recovered := *initial
	recovered.ID = id.NewSessionID()
	recovered.EncryptedAccessToken = []byte("reauthorized-access")
	recovered.EncryptedRefreshToken = []byte("reauthorized-refresh")
	recovered.InitiatedAt = now.Add(time.Minute)
	recovered.CreatedAt = recovered.InitiatedAt
	recovered.UpdatedAt = recovered.InitiatedAt
	require.NoError(t, repo.Create(ctx, &recovered))
	stored, err = repo.FindByPrincipalAndService(ctx, session.Principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, initial.ID, stored.ID)
	assert.True(t, initial.InitiatedAt.Equal(stored.InitiatedAt), "upsert preserves the existing initiation time")
	assert.True(t, initial.CreatedAt.Equal(stored.CreatedAt), "upsert preserves the existing creation time")
	assert.False(t, stored.RefreshTokenRejected)
	assert.True(t, stored.CanRefresh())
	assert.False(t, stored.RequiresReauthentication())
	assert.Equal(t, recovered.EncryptedAccessToken, stored.EncryptedAccessToken)
	assert.Equal(t, recovered.EncryptedRefreshToken, stored.EncryptedRefreshToken)
	listed, err = repo.ListByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
	activeIDs, err := repo.ListActiveServiceIDsByPrincipal(ctx, session.Principal)
	require.NoError(t, err)
	assert.Equal(t, []id.ServiceID{serviceID}, activeIDs)
}

func TestUserSessionRepository_PreparationRefusesRejectionWithoutPartialWrites(t *testing.T) {
	adapter, cleanup := setupUserSessionTestDBAtVersion(t, 38)
	defer cleanup()
	repo := NewUserSessionRepository(adapter)
	ctx := context.Background()
	serviceID := id.NewServiceID()
	insertTestService(t, adapter, serviceID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(time.Hour)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal("rejected@example.com"), ServiceID: serviceID,
		EncryptedAccessToken: []byte("old-access"), EncryptedRefreshToken: []byte("old-refresh"),
		TokenType: "Bearer", AccessTokenExpiresAt: &expiry, Scope: []string{"repo", "email"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, repo.Create(ctx, session))
	initial, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	_, err = repo.WithLockedSession(ctx, session.Principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
		current.EncryptedAccessToken = []byte("discarded-access")
		current.EncryptedRefreshToken = []byte("discarded-refresh")
		current.AccessTokenExpiresAt = nil
		current.RefreshTokenRejected = true
		current.UpdatedAt = now.Add(time.Minute)
		return true, nil
	})
	require.ErrorContains(t, err, "session rejection persistence requires migration 039")
	stored, err := repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, initial, stored, "refused locked write must not persist any session changes")
	replacement := *initial
	replacement.ID = id.NewSessionID()
	replacement.RefreshTokenRejected = true
	replacement.EncryptedAccessToken = []byte("discarded-upsert")
	replacement.Scope = []string{"discarded"}
	require.ErrorContains(t, repo.Create(ctx, &replacement), "session rejection persistence requires migration 039")
	stored, err = repo.Get(ctx, session.ID)
	require.NoError(t, err)
	assert.Equal(t, initial, stored, "refused upsert must preserve the original identity, credentials, and metadata")
	replacement.Principal = id.Principal("unsupported-rejection@example.com")
	require.ErrorContains(t, repo.Create(ctx, &replacement), "session rejection persistence requires migration 039")
	missing, err := repo.FindByPrincipalAndService(ctx, replacement.Principal, serviceID)
	require.NoError(t, err)
	assert.Nil(t, missing, "refused create must not insert a partially initialized row")
}
