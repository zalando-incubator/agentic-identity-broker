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
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &threshold)
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
		page, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, 10)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(1), expiryTestID(2)}, expirySessionIDs(page))
	})

	t.Run("E2 keyset pages have no gaps or duplicates", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		for n := 5; n >= 1; n-- {
			seedExpiryTestSession(t, adapter, serviceID, expiryTestID(n), &due)
		}
		all, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, 1000)
		require.NoError(t, err)
		expected := []id.SessionID{expiryTestID(1), expiryTestID(2), expiryTestID(3), expiryTestID(4), expiryTestID(5)}
		require.Equal(t, expected, expirySessionIDs(all))
		var combined []id.SessionID
		cursor := id.SessionID{}
		for _, wantSize := range []int{2, 2, 1} {
			page, err := repo.ListExpiringSessions(ctx, threshold, cursor, 2)
			require.NoError(t, err)
			require.Len(t, page, wantSize)
			combined = append(combined, expirySessionIDs(page)...)
			cursor = page[len(page)-1].ID
		}
		assert.Equal(t, expected, combined)
		last, err := repo.ListExpiringSessions(ctx, threshold, cursor, 2)
		require.NoError(t, err)
		assert.Empty(t, last)
	})

	t.Run("E3 moving a returned row past the threshold does not skip later rows", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		for n := 1; n <= 4; n++ {
			seedExpiryTestSession(t, adapter, serviceID, expiryTestID(n), &due)
		}
		first, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, 2)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{expiryTestID(1), expiryTestID(2)}, expirySessionIDs(first))
		_, err = adapter.db.ExecContext(ctx, `UPDATE user_sessions SET access_token_expires_at = $1 WHERE id = $2`, future, first[0].ID)
		require.NoError(t, err)
		second, err := repo.ListExpiringSessions(ctx, threshold, first[1].ID, 2)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(3), expiryTestID(4)}, expirySessionIDs(second))
	})

	t.Run("E4 inserting behind the cursor never revisits that row", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		for _, n := range []int{1, 3, 5} {
			seedExpiryTestSession(t, adapter, serviceID, expiryTestID(n), &due)
		}
		first, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, 2)
		require.NoError(t, err)
		require.Equal(t, []id.SessionID{expiryTestID(1), expiryTestID(3)}, expirySessionIDs(first))
		seedExpiryTestSession(t, adapter, serviceID, expiryTestID(2), &due)
		second, err := repo.ListExpiringSessions(ctx, threshold, first[1].ID, 2)
		require.NoError(t, err)
		assert.Equal(t, []id.SessionID{expiryTestID(5)}, expirySessionIDs(second))
	})

	t.Run("E5 invalid page sizes fail before database access", func(t *testing.T) {
		repo := NewUserSessionRepository(&Adapter{})
		for _, limit := range []int{-1, 0, 1001} {
			t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
				_, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, limit)
				require.Error(t, err, "limit %d must be rejected without a database connection", limit)
				var validation *storage.StorageError
				require.ErrorAs(t, err, &validation)
				assert.Equal(t, storage.ErrorKindValidation, validation.Kind)
			})
		}
	})

	t.Run("E6 returned tokens are unchanged ciphertext", func(t *testing.T) {
		adapter, cleanup := setupUserSessionTestDB(t)
		defer cleanup()
		serviceID := id.NewServiceID()
		insertTestService(t, adapter, serviceID)
		repo := NewUserSessionRepository(adapter)
		original := seedExpiryTestSession(t, adapter, serviceID, expiryTestID(1), &due)
		page, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, 1)
		require.NoError(t, err)
		require.Len(t, page, 1)
		assert.Equal(t, original.EncryptedAccessToken, page[0].EncryptedAccessToken)
		assert.Equal(t, original.EncryptedRefreshToken, page[0].EncryptedRefreshToken)
		assert.Equal(t, original.EncryptionContext, page[0].EncryptionContext)
		assert.Equal(t, original.Scope, page[0].Scope)
	})

	t.Run("E7 UUID byte order determines the returned order", func(t *testing.T) {
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
		page, err := repo.ListExpiringSessions(ctx, threshold, id.SessionID{}, len(ids))
		require.NoError(t, err)
		assert.Equal(t, ids, expirySessionIDs(page))
	})
}

type expiryExplainPlan struct {
	NodeType  string              `json:"Node Type"`
	IndexName string              `json:"Index Name"`
	Plans     []expiryExplainPlan `json:"Plans"`
}

func expiryPlanUsesIndex(plan expiryExplainPlan, indexName string) bool {
	if plan.IndexName == indexName && (plan.NodeType == "Index Scan" || plan.NodeType == "Index Only Scan" || plan.NodeType == "Bitmap Index Scan") {
		return true
	}
	for _, child := range plan.Plans {
		if expiryPlanUsesIndex(child, indexName) {
			return true
		}
	}
	return false
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
	require.NoError(t, adapter.db.GetContext(ctx, &valid, `
		SELECT indisvalid FROM pg_index
		WHERE indexrelid = to_regclass('idx_user_sessions_access_token_expires_at')
	`))
	require.True(t, valid, "%s must be valid", indexName)

	// Generate a representative table in one statement: 1% due, 1% NULL, 98% healthy.
	_, err := adapter.db.ExecContext(ctx, `
		INSERT INTO user_sessions
			(principal, service_id, encrypted_access_token, encrypted_refresh_token, access_token_expires_at)
		SELECT 'planner-' || n || '@example.com', $1, $3::bytea, $4::bytea,
			CASE WHEN n <= 50 THEN $2::timestamptz - interval '1 minute'
			     WHEN n <= 100 THEN NULL
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
	require.LessOrEqual(t, dueCount*100, total*2)
	require.Positive(t, nullCount)
	_, err = adapter.db.ExecContext(ctx, `ANALYZE user_sessions`)
	require.NoError(t, err)

	var raw []byte
	require.NoError(t, adapter.db.GetContext(ctx, &raw, `EXPLAIN (FORMAT JSON) `+listExpiringSessionsQuery, threshold, id.SessionID{}, 1000))
	var explain []struct {
		Plan expiryExplainPlan `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(raw, &explain))
	require.Len(t, explain, 1)
	assert.True(t, expiryPlanUsesIndex(explain[0].Plan, indexName), "normal planner did not use %s for production query; plan: %s", indexName, raw)
}
