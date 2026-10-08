package oauth2session_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetValidAccessToken_ConcurrentRotatingRefresh(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
	duplicateRequest := make(chan struct{}, 1)
	var refreshes atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("refresh_token") != "initial-refresh" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		if refreshes.Add(1) != 1 {
			select {
			case duplicateRequest <- struct{}{}:
			default:
			}
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		close(firstRequest)
		<-releaseFirst
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
	}))
	defer provider.Close()

	upstream := createTestService(serviceID)
	upstream.Endpoints.TokenEndpoint = provider.URL
	require.NoError(t, providers.Create(ctx, upstream))

	enc := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	encryptedAccess, err := enc.Encrypt(ctx, []byte("expired-access"), encContext)
	require.NoError(t, err)
	encryptedRefresh, err := enc.Encrypt(ctx, []byte("initial-refresh"), encContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  encryptedAccess,
		EncryptedRefreshToken: encryptedRefresh,
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &expired,
		Scope:                 []string{"repo"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
	}
	require.NoError(t, sessions.Create(ctx, session))

	const callers = 8
	type result struct {
		token string
		err   error
	}
	results := make(chan result, callers)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
		results <- result{token, err}
	}()
	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not reach provider")
	}
	for range callers - 1 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
			results <- result{token, err}
		}()
	}
	select {
	case <-duplicateRequest:
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	workers.Wait()
	close(results)
	for result := range results {
		assert.NoError(t, result.err)
		assert.Equal(t, "rotated-access", result.token)
	}
	assert.EqualValues(t, 1, refreshes.Load(), "rotating provider must only receive one refresh")

	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	refreshToken, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "rotated-refresh", refreshToken)
}

func TestForceRefreshSession_ConcurrentRotatingRefresh(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
	duplicateRequest := make(chan struct{}, 1)
	var refreshes atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("refresh_token") {
		case "initial-refresh":
			if refreshes.Add(1) != 1 {
				duplicateRequest <- struct{}{}
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			close(firstRequest)
			<-releaseFirst
			_, _ = fmt.Fprint(w, `{"access_token":"first-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
		case "rotated-refresh":
			refreshes.Add(1)
			_, _ = fmt.Fprint(w, `{"access_token":"second-access","token_type":"Bearer","expires_in":3600,"refresh_token":"final-refresh"}`)
		default:
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		}
	}))
	defer provider.Close()
	upstream := createTestService(serviceID)
	upstream.Endpoints.TokenEndpoint = provider.URL
	require.NoError(t, providers.Create(ctx, upstream))

	enc := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	encryptedAccess, err := enc.Encrypt(ctx, []byte("initial-access"), encContext)
	require.NoError(t, err)
	encryptedRefresh, err := enc.Encrypt(ctx, []byte("initial-refresh"), encContext)
	require.NoError(t, err)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  encryptedAccess,
		EncryptedRefreshToken: encryptedRefresh,
		TokenType:             "Bearer",
		Scope:                 []string{"repo"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
	}))

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := service.ForceRefreshSession(ctx, principal, serviceID)
		firstDone <- err
	}()
	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("first force-refresh did not reach provider")
	}
	go func() {
		_, err := service.ForceRefreshSession(ctx, principal, serviceID)
		secondDone <- err
	}()
	select {
	case <-duplicateRequest:
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	assert.EqualValues(t, 2, refreshes.Load())

	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	refreshToken, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "final-refresh", refreshToken)
	_, accessToken, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "second-access", accessToken)
}

type failedCommitRefreshRepo struct {
	underlying ports.UserSessionRefreshRepository
}

func (r failedCommitRefreshRepo) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) error) (*storage.UserSession, error) {
	return r.underlying.WithLockedSession(ctx, principal, serviceID, refresh)
}

func (r failedCommitRefreshRepo) UpdateRefreshedSession(context.Context, *storage.UserSession, *storage.UserSession) error {
	return errors.New("failed to commit refreshed session")
}

func TestGetValidAccessToken_DoesNotAuditRefreshSuccessBeforePersistence(t *testing.T) {
	ctx := context.Background()
	_, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("audit-user@example.com")
	serviceID := id.NewServiceID()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	encryption := newTestEncryption(t)
	encryptionContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("old-access"), encryptionContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("old-refresh"), encryptionContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       time.Now(), CreatedAt: time.Now(),
	}))
	var logs strings.Builder
	store := &ledgerfixture.Store{}
	service := oauth2session.NewOAuth2SessionService(
		providers, sessions, failedCommitRefreshRepo{sessions.(ports.UserSessionRefreshRepository)},
		nil, nil, encryption, &http.Client{}, nil, oauth2session.DefaultConfig(), slog.New(slog.NewJSONHandler(&logs, nil)),
		store.Recorder(t), store,
	)
	_, _, err = service.GetValidAccessToken(ctx, principal, serviceID)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailPersistenceFailed)
	assert.NotContains(t, logs.String(), "session.oauth2.token_refreshed")
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	currentRefresh, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "old-refresh", currentRefresh)
}

type failingRefreshEvents struct {
	ports.BusinessEventRepository
	failed error
}

func (r failingRefreshEvents) Append(ctx context.Context, event *model.BusinessEvent, queue bool) error {
	if event.Type == model.BusinessEventTypePrefix+"session-refreshed" {
		return r.failed
	}
	return r.BusinessEventRepository.Append(ctx, event, queue)
}

func realMemoryRefreshService(t *testing.T, eventsFailure error) (*oauth2session.OAuth2SessionService, *memory.InMemoryUserSessionRepository, *memory.InMemoryThirdpartyOAuth2ProviderRepository, *memory.TransactionManager, *memory.BusinessEventRepository) {
	t.Helper()
	transactions := memory.NewTransactionManager()
	sessions := memory.NewInMemoryUserSessionRepository(transactions)
	providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository(transactions)
	encryption := newTestEncryption(t)
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	events := memory.NewBusinessEventRepository(transactions, registry)
	var eventStore ports.BusinessEventRepository = events
	if eventsFailure != nil {
		eventStore = failingRefreshEvents{BusinessEventRepository: events, failed: eventsFailure}
	}
	recorder := ledger.NewService(registry, eventStore, nil, transactions, false)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(providers, encryption, newNoopBranchKeyManager(), nil, false, slog.Default())
	service := oauth2session.NewOAuth2SessionService(providerService, sessions, sessions, nil, nil, encryption,
		&http.Client{Timeout: 3 * time.Second}, nil, oauth2session.DefaultConfig(), slog.Default(), recorder, transactions)
	return service, sessions, providers, transactions, events
}

func memoryExpiredRefreshSession(t *testing.T, sessions *memory.InMemoryUserSessionRepository, serviceID id.ServiceID, principal id.Principal, refreshToken string) *storage.UserSession {
	t.Helper()
	ctx := context.Background()
	encryption := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("old-access"), encContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte(refreshToken), encContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Minute)
	now := time.Now().UTC()
	session := &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID}, InitiatedAt: now, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, sessions.Create(ctx, session))
	return session
}

func queryRefreshEvents(t *testing.T, repo *memory.BusinessEventRepository, principal id.Principal, eventName string) []*model.BusinessEvent {
	t.Helper()
	events, err := repo.Query(context.Background(), model.BusinessEventQuery{Subject: model.BusinessEventSubject{Principal: principal},
		Type: model.BusinessEventTypePrefix + eventName, Start: time.Now().Add(-time.Hour).UTC(), End: time.Now().Add(time.Hour).UTC()})
	require.NoError(t, err)
	return events
}

func TestMemoryServiceRefreshDoesNotGateUnrelatedSessionsOrAdminReads(t *testing.T) {
	ctx := context.Background()
	service, sessions, providers, _, events := realMemoryRefreshService(t, nil)
	serviceID := id.NewServiceID()
	slowPrincipal, fastPrincipal := id.Principal("slow-refresh@example.com"), id.Principal("fast-refresh@example.com")
	entered := make(chan struct{})
	release := make(chan struct{})
	releaseProvider := sync.OnceFunc(func() { close(release) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		if r.Form.Get("refresh_token") == "slow-refresh" {
			close(entered)
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
	}))
	defer upstream.Close()
	defer releaseProvider()
	provider := createTestService(serviceID)
	provider.Secret = model.NewAbsentSecret()
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))
	memoryExpiredRefreshSession(t, sessions, serviceID, slowPrincipal, "slow-refresh")
	memoryExpiredRefreshSession(t, sessions, serviceID, fastPrincipal, "fast-refresh")
	slowDone := make(chan error, 1)
	go func() { _, _, err := service.GetValidAccessToken(ctx, slowPrincipal, serviceID); slowDone <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("slow refresh never reached provider")
	}
	otherDone := make(chan error, 1)
	go func() {
		if _, err := providers.Get(ctx, serviceID); err != nil {
			otherDone <- err
			return
		}
		if _, err := sessions.FindByPrincipalAndService(ctx, fastPrincipal, serviceID); err != nil {
			otherDone <- err
			return
		}
		_, token, err := service.GetValidAccessToken(ctx, fastPrincipal, serviceID)
		if err == nil && token != "rotated-access" {
			err = fmt.Errorf("unexpected other session token")
		}
		otherDone <- err
	}()
	select {
	case err := <-otherDone:
		require.NoError(t, err, "unrelated session and admin work must finish during the provider wait")
	case <-time.After(900 * time.Millisecond):
		t.Fatal("slow upstream refresh held the global storage gate")
	}
	releaseProvider()
	require.NoError(t, <-slowDone)
	require.Len(t, queryRefreshEvents(t, events, slowPrincipal, "session-refreshed"), 1)
	require.Len(t, queryRefreshEvents(t, events, fastPrincipal, "session-refreshed"), 1)
}

func TestMemoryServiceRefreshEventFailureRollsBackRotatedToken(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("ledger append refused refreshed token")
	service, sessions, providers, _, events := realMemoryRefreshService(t, failure)
	serviceID := id.NewServiceID()
	principal := id.Principal("atomic-refresh@example.com")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Secret = model.NewAbsentSecret()
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))
	before := memoryExpiredRefreshSession(t, sessions, serviceID, principal, "original-refresh")
	_, _, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.ErrorIs(t, err, failure)
	after, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.Equal(t, before.EncryptedAccessToken, after.EncryptedAccessToken)
	require.Equal(t, before.EncryptedRefreshToken, after.EncryptedRefreshToken)
	require.Empty(t, queryRefreshEvents(t, events, principal, "session-refreshed"))
}

func TestMemoryServiceRefreshCannotReviveLogoutOrOverwriteReauthorization(t *testing.T) {
	for _, action := range []string{"logout", "reauthorize"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			service, sessions, providers, transactions, events := realMemoryRefreshService(t, nil)
			serviceID := id.NewServiceID()
			principal := id.Principal("raced-refresh@example.com")
			entered := make(chan struct{})
			release := make(chan struct{})
			releaseProvider := sync.OnceFunc(func() { close(release) })
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				close(entered)
				<-release
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"access_token":"obsolete-access","token_type":"Bearer","expires_in":3600,"refresh_token":"obsolete-refresh"}`)
			}))
			defer upstream.Close()
			defer releaseProvider()
			provider := createTestService(serviceID)
			provider.Secret = model.NewAbsentSecret()
			provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
			provider.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, providers.Create(ctx, provider))
			before := memoryExpiredRefreshSession(t, sessions, serviceID, principal, "original-refresh")
			refreshDone := make(chan error, 1)
			go func() { _, _, err := service.GetValidAccessToken(ctx, principal, serviceID); refreshDone <- err }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("refresh did not reach provider")
			}
			mutationCtx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
			defer cancel()
			txCtx, err := transactions.BeginTX(mutationCtx)
			require.NoError(t, err, "logout and reauthorization must commit while provider waits")
			switch action {
			case "logout":
				err = sessions.DeleteByPrincipalAndService(txCtx, principal, serviceID)
			case "reauthorize":
				reauthorized := *before
				reauthorized.EncryptedAccessToken = []byte("reauthorized-access")
				reauthorized.EncryptedRefreshToken = []byte("reauthorized-refresh")
				reauthorized.UpdatedAt = time.Now().Add(time.Minute).UTC()
				err = sessions.Create(txCtx, &reauthorized)
			}
			if err != nil {
				_ = transactions.Rollback(txCtx)
				t.Fatal(err)
			}
			require.NoError(t, transactions.Commit(txCtx))
			releaseProvider()
			select {
			case err := <-refreshDone:
				require.Error(t, err, "stale provider result must lose the conditional write")
			case <-time.After(3 * time.Second):
				t.Fatal("refresh did not finish")
			}
			after, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			if action == "logout" {
				require.Nil(t, after)
			} else {
				require.NotNil(t, after)
				require.Equal(t, []byte("reauthorized-access"), after.EncryptedAccessToken)
				require.Equal(t, []byte("reauthorized-refresh"), after.EncryptedRefreshToken)
			}
			require.Empty(t, queryRefreshEvents(t, events, principal, "session-refreshed"))
		})
	}
}
