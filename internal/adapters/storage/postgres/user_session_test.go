//go:build integration
// +build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
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
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	service := oauth2session.NewOAuth2SessionService(
		thirdparty.NewThirdpartyOAuth2ProviderService(NewPostgresThirdpartyOAuth2ProviderRepository(adapter), encryption, &noop.BranchKeyManager{}, nil, false, slog.Default()),
		sessions, sessions, nil, nil, encryption, &http.Client{Timeout: time.Second}, nil, oauth2session.DefaultConfig(), slog.Default(),
		ledger.NewService(registry, NewBusinessEventRepository(adapter, registry), nil, adapter, false), adapter,
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
