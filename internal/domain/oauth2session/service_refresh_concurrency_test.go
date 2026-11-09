package oauth2session_test

import (
	"context"
	"encoding/json"
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

type observedSessionReads struct {
	ports.UserSessionRepository
	reads chan<- struct{}
}

func (r observedSessionReads) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	session, err := r.UserSessionRepository.FindByPrincipalAndService(ctx, principal, serviceID)
	if err == nil && session != nil {
		r.reads <- struct{}{}
	}
	return session, err
}

func TestGetValidAccessToken_ConcurrentJoinersDecryptRotatingRefresh(t *testing.T) {
	ctx := context.Background()
	_, _, sessions, _, _, providers, transactions := setupServiceWithTransactions(t, nil, &http.Client{})
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
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
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		close(firstRequest)
		<-releaseFirst
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
	}))
	defer provider.Close()
	release := sync.OnceFunc(func() { close(releaseFirst) })
	defer release()

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
	reads := make(chan struct{}, callers)
	service := oauth2session.NewOAuth2SessionService(
		providers, observedSessionReads{UserSessionRepository: sessions, reads: reads},
		sessions.(ports.UserSessionRefreshRepository), nil, nil, enc, &http.Client{}, nil,
		oauth2session.DefaultConfig(), slog.Default(), ledgerfixture.NewRecorder(), transactions,
	)
	type result struct {
		session *storage.UserSession
		token   string
		err     error
	}
	results := make(chan result, callers)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		current, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
		results <- result{current, token, err}
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
			current, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
			results <- result{current, token, err}
		}()
	}
	deadline := time.After(5 * time.Second)
	for range callers {
		select {
		case <-reads:
		case <-deadline:
			t.Fatal("not every concurrent caller read the expired session before the provider responded")
		}
	}
	release()
	workers.Wait()
	close(results)
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.session)
		assert.Equal(t, session.ID, result.session.ID)
		assert.NotEqual(t, "rotated-access", string(result.session.EncryptedAccessToken))
		decryptedAccess, err := service.DecryptAccessToken(ctx, result.session)
		require.NoError(t, err)
		assert.Equal(t, "rotated-access", decryptedAccess)
		assert.Equal(t, decryptedAccess, result.token)
		decryptedRefresh, err := service.DecryptRefreshToken(ctx, result.session)
		require.NoError(t, err)
		assert.Equal(t, "rotated-refresh", decryptedRefresh)
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
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(-time.Hour), nil, nil,
		func(repo ports.UserSessionRefreshRepository) ports.UserSessionRefreshRepository {
			return failedCommitRefreshRepo{underlying: repo}
		})
	_, _, err := fixture.service.GetValidAccessToken(ctx, fixture.principal, fixture.serviceID)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailPersistenceFailed)
	assert.EqualValues(t, 1, calls.Load())
	assert.Zero(t, fixture.logs.eventCount("session.oauth2.token_refreshed"), "an uncommitted refresh must never be audited as success")
	assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "ERROR", oauth2session.RefreshTriggerOnDemand))
	assertRefreshFailureMetadata(t, fixture, oauth2session.DetailPersistenceFailed, oauth2session.KindInfrastructure, oauth2session.DependencySessionRepository)
	persisted, err := fixture.sessions.FindByPrincipalAndService(ctx, fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
	assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
}

type refreshCommitWitness struct {
	ports.UserSessionRefreshRepository
	committed atomic.Bool
}

// UpdateRefreshedSession marks the witness only once the owning transaction commits.
func (r *refreshCommitWitness) UpdateRefreshedSession(ctx context.Context, previous, current *storage.UserSession) error {
	if err := r.UserSessionRefreshRepository.UpdateRefreshedSession(ctx, previous, current); err != nil {
		return err
	}
	effects, active := ports.StorageTransactionEffectsFromContext(ctx)
	if !active {
		return errors.New("refreshed session written outside a transaction")
	}
	return effects.AfterCommit(func() { r.committed.Store(true) })
}

type recordedRefreshLog struct {
	entry     []byte
	committed bool
}

type refreshLogSink struct {
	mu      sync.Mutex
	commit  *refreshCommitWitness
	notify  chan struct{}
	entries []recordedRefreshLog
}

func (s *refreshLogSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, recordedRefreshLog{entry: append([]byte(nil), p...), committed: s.commit.committed.Load()})
	if s.notify != nil {
		select {
		case s.notify <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func (s *refreshLogSink) event(t *testing.T, event string, principal id.Principal) (map[string]any, bool) {
	t.Helper()
	s.mu.Lock()
	entries := append([]recordedRefreshLog(nil), s.entries...)
	s.mu.Unlock()
	var found []recordedRefreshLog
	for _, entry := range entries {
		var fields map[string]any
		require.NoError(t, json.Unmarshal(entry.entry, &fields))
		if fields["event"] == event {
			found = append(found, entry)
			if event == "session.oauth2.sweep_aborted" {
				assert.NotContains(t, fields, "principal", "aggregate abort has no session owner")
			} else {
				assert.Equal(t, principal.String(), fields["principal"], "refresh events identify the session owner")
			}
		}
	}
	require.Len(t, found, 1, "exactly one event per refresh attempt")
	var fields map[string]any
	require.NoError(t, json.Unmarshal(found[0].entry, &fields))
	return fields, found[0].committed
}

func (s *refreshLogSink) eventCount(event string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, entry := range s.entries {
		if strings.Contains(string(entry.entry), `"event":"`+event+`"`) {
			count++
		}
	}
	return count
}

func (s *refreshLogSink) waitForEvent(t *testing.T, event string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		s.mu.Lock()
		found := false
		for _, entry := range s.entries {
			found = found || strings.Contains(string(entry.entry), `"event":"`+event+`"`)
		}
		s.mu.Unlock()
		if found {
			return
		}
		select {
		case <-s.notify:
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", event)
		}
	}
}

type loggedRefreshFixture struct {
	service   *oauth2session.OAuth2SessionService
	sessions  ports.UserSessionRepository
	session   *storage.UserSession
	principal id.Principal
	serviceID id.ServiceID
	logs      *refreshLogSink
}

func newLoggedRefreshFixture(t *testing.T, endpoint string, accessExpiry time.Time, refreshRejectedAt *time.Time, configure func(*oauth2session.Config), wrap ...func(ports.UserSessionRefreshRepository) ports.UserSessionRefreshRepository) loggedRefreshFixture {
	t.Helper()
	_, _, sessions, grants, agents, providers, transactions := setupServiceWithTransactions(t, nil, &http.Client{})
	ctx := context.Background()
	principal := id.Principal("private-refresh-principal@example.com")
	serviceID := id.NewServiceID()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = endpoint
	require.NoError(t, providers.Create(ctx, provider))

	encryption := newTestEncryption(t)
	encryptionContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("old-access"), encryptionContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("old-refresh"), encryptionContext)
	require.NoError(t, err)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &accessExpiry, RefreshRejectedAt: refreshRejectedAt,
		Scope: []string{"repo"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(t, sessions.Create(ctx, session))
	refreshRepo := sessions.(ports.UserSessionRefreshRepository)
	if len(wrap) > 0 {
		refreshRepo = wrap[0](refreshRepo)
	}
	witness := &refreshCommitWitness{UserSessionRefreshRepository: refreshRepo}
	logs := &refreshLogSink{commit: witness, notify: make(chan struct{}, 8)}
	config := oauth2session.DefaultConfig()
	if configure != nil {
		configure(&config)
	}
	service := oauth2session.NewOAuth2SessionService(providers, sessions, witness, grants, agents, encryption,
		&http.Client{}, nil, config, slog.New(slog.NewJSONHandler(logs, nil)), ledgerfixture.NewRecorder(), transactions)
	return loggedRefreshFixture{service: service, sessions: sessions, session: session, principal: principal, serviceID: serviceID, logs: logs}
}

func assertRefreshEvent(t *testing.T, fixture loggedRefreshFixture, event, level string, trigger oauth2session.RefreshTrigger) bool {
	t.Helper()
	fields, committed := fixture.logs.event(t, event, fixture.principal)
	assert.Equal(t, level, fields["level"])
	assert.Equal(t, fixture.session.ID.String(), fields["session_id"])
	assert.Equal(t, fixture.serviceID.String(), fields["service_id"])
	assert.Equal(t, string(trigger), fields["triggered_by"])
	assert.Equal(t, fixture.principal.String(), fields["principal"])
	return committed
}

func assertRefreshFailureMetadata(t *testing.T, fixture loggedRefreshFixture, detail oauth2session.ErrorDetail, kind oauth2session.ErrorKind, dependency oauth2session.Dependency) {
	t.Helper()
	fields, _ := fixture.logs.event(t, "session.oauth2.refresh_failed", fixture.principal)
	metadata, ok := fields["oauth2_session"].(map[string]any)
	require.True(t, ok, "failure event needs bounded error metadata")
	assert.Equal(t, string(oauth2session.OperationRefresh), metadata["operation"])
	assert.Equal(t, string(detail), metadata["failure_detail"])
	assert.Equal(t, string(kind), metadata["error_kind"])
	assert.Equal(t, string(dependency), metadata["dependency"])
	if detail == oauth2session.DetailRefreshRejected {
		assert.EqualValues(t, http.StatusBadRequest, metadata["http_status_code"])
		assert.Equal(t, "invalid_grant", metadata["oauth_error_code"])
	}
}

func TestRefreshSuccessEvent_FollowsCommittedUpdateForOnDemandPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		force bool
	}{
		{name: "expired token read"},
		{name: "forced refresh of valid token", force: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
			}))
			defer upstream.Close()
			expiry := time.Now().Add(-time.Minute)
			if tc.force {
				expiry = time.Now().Add(time.Hour)
			}
			fixture := newLoggedRefreshFixture(t, upstream.URL, expiry, nil, nil)
			ctx := context.Background()
			if tc.force {
				summary, err := fixture.service.ForceRefreshSession(ctx, fixture.principal, fixture.serviceID)
				require.NoError(t, err)
				require.NotNil(t, summary)
			} else {
				_, token, err := fixture.service.GetValidAccessToken(ctx, fixture.principal, fixture.serviceID)
				require.NoError(t, err)
				assert.Equal(t, "new-access", token)
			}
			assert.EqualValues(t, 1, calls.Load())
			assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.token_refreshed", "INFO", oauth2session.RefreshTriggerOnDemand), "success must be logged after the locked update commits")
			persisted, err := fixture.sessions.FindByPrincipalAndService(ctx, fixture.principal, fixture.serviceID)
			require.NoError(t, err)
			refresh, err := fixture.service.DecryptRefreshToken(ctx, persisted)
			require.NoError(t, err)
			assert.Equal(t, "new-refresh", refresh)
		})
	}
}

func TestRefreshFailureEvent_ClassifiesOnDemandAttemptsWithoutChangingCiphertext(t *testing.T) {
	for _, tc := range []struct {
		name            string
		force           bool
		refreshRejected bool
		detail          oauth2session.ErrorDetail
		providerCalls   int32
	}{
		{name: "provider rejects read-path refresh", detail: oauth2session.DetailRefreshRejected, providerCalls: 1},
		{name: "provider rejects forced refresh", force: true, detail: oauth2session.DetailRefreshRejected, providerCalls: 1},
		{name: "stored refresh token rejected on read path", refreshRejected: true, detail: oauth2session.DetailRefreshRejected},
		{name: "stored refresh token rejected on forced refresh", force: true, refreshRejected: true, detail: oauth2session.DetailRefreshRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"private-refresh-principal@example.com"}`)
			}))
			defer upstream.Close()
			expiry := time.Now().Add(-time.Minute)
			if tc.force {
				expiry = time.Now().Add(time.Hour)
			}
			var rejectedAt *time.Time
			if tc.refreshRejected {
				earlier := time.Now().Add(-time.Minute)
				rejectedAt = &earlier
			}
			fixture := newLoggedRefreshFixture(t, upstream.URL, expiry, rejectedAt, nil)
			ctx := context.Background()
			var err error
			if tc.force {
				_, err = fixture.service.ForceRefreshSession(ctx, fixture.principal, fixture.serviceID)
			} else {
				_, _, err = fixture.service.GetValidAccessToken(ctx, fixture.principal, fixture.serviceID)
			}
			metadata := assertOperationMetadata(t, err, oauth2session.OperationRefresh, tc.detail)
			assert.EqualValues(t, tc.providerCalls, calls.Load())
			assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
			switch {
			case tc.providerCalls > 0:
				assert.Equal(t, http.StatusBadRequest, metadata.StatusCode())
				assert.Equal(t, "invalid_grant", metadata.OAuthCode())
				assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "ERROR", oauth2session.RefreshTriggerOnDemand), "a provider rejection commits before it is reported")
				assertRefreshFailureMetadata(t, fixture, tc.detail, oauth2session.KindProvider, oauth2session.DependencyProvider)
			case tc.force:
				assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
				assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "ERROR", oauth2session.RefreshTriggerOnDemand), "a recorded rejection commits nothing")
				assert.Zero(t, metadata.StatusCode())
			default:
				assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
				assert.Zero(t, fixture.logs.eventCount("session.oauth2.refresh_failed"), "a recorded rejection refuses issuance without a refresh attempt")
			}
			persisted, findErr := fixture.sessions.FindByPrincipalAndService(ctx, fixture.principal, fixture.serviceID)
			require.NoError(t, findErr)
			assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
		})
	}
}

func TestBackgroundRefreshSuccessEvent_FollowsCommitWithoutDelayingValidToken(t *testing.T) {
	requested := make(chan struct{}, 1)
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		requested <- struct{}{}
		<-gate
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	defer release()
	fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Minute), nil, func(config *oauth2session.Config) {
		config.RefreshLookahead = 5 * time.Minute
		config.BackgroundRefreshWorkers = 1
	})
	type outcome struct {
		token string
		err   error
	}
	returned := make(chan outcome, 1)
	go func() {
		_, token, err := fixture.service.GetValidAccessToken(context.Background(), fixture.principal, fixture.serviceID)
		returned <- outcome{token: token, err: err}
	}()
	select {
	case result := <-returned:
		require.NoError(t, result.err)
		assert.Equal(t, "old-access", result.token, "the current token must not await the provider")
	case <-time.After(5 * time.Second):
		t.Fatal("a valid token request waited for background refresh")
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("near-expiry session did not reach the background provider")
	}
	release()
	fixture.logs.waitForEvent(t, "session.oauth2.token_refreshed")
	assert.EqualValues(t, 1, calls.Load())
	assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.token_refreshed", "INFO", oauth2session.RefreshTriggerBackground), "background event must follow persistence")
	persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	refresh, err := fixture.service.DecryptRefreshToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", refresh)
}

func TestBackgroundRefreshFailureEvent_ClassifiesRejectionWithoutChangingCiphertext(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"private-refresh-principal@example.com"}`)
	}))
	defer upstream.Close()
	fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Minute), nil, func(config *oauth2session.Config) {
		config.RefreshLookahead = 5 * time.Minute
		config.BackgroundRefreshWorkers = 1
	})
	_, token, err := fixture.service.GetValidAccessToken(context.Background(), fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	assert.Equal(t, "old-access", token)
	fixture.logs.waitForEvent(t, "session.oauth2.refresh_failed")
	assert.EqualValues(t, 1, calls.Load())
	assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "WARN", oauth2session.RefreshTriggerBackground), "a provider rejection commits, to record the terminal refresh rejection")
	assertRefreshFailureMetadata(t, fixture, oauth2session.DetailRefreshRejected, oauth2session.KindProvider, oauth2session.DependencyProvider)
	persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
	assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
}

func TestSweepRefreshSuccessEvent_FollowsCommittedUpdate(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Minute), nil, nil)
	sweeper := oauth2session.NewSessionSweepService(fixture.sessions.(ports.UserSessionExpiryRepository), fixture.service, slog.New(slog.NewJSONHandler(fixture.logs, nil)))
	result, err := sweeper.Sweep(context.Background(), oauth2session.SweepRequest{Lookahead: 5 * time.Minute, PageSize: 2})
	require.NoError(t, err)
	assertSweepCounts(t, result, 1, 0, 0, false)
	assert.EqualValues(t, 1, calls.Load())
	assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.token_refreshed", "INFO", oauth2session.RefreshTriggerSweep), "sweep event must follow persistence")
	persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	refresh, err := fixture.service.DecryptRefreshToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", refresh)
}

func TestSweepRefreshFailureEvent_ClassifiesEachFailedCandidate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		calls      int32
		detail     oauth2session.ErrorDetail
		kind       oauth2session.ErrorKind
		dependency oauth2session.Dependency
	}{
		{name: "provider rejection", calls: 1, detail: oauth2session.DetailRefreshRejected, kind: oauth2session.KindProvider, dependency: oauth2session.DependencyProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":"invalid_grant","error_description":"private-refresh-principal@example.com"}`)
			}))
			defer upstream.Close()
			fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Minute), nil, nil)
			sweeper := oauth2session.NewSessionSweepService(fixture.sessions.(ports.UserSessionExpiryRepository), fixture.service, slog.New(slog.NewJSONHandler(fixture.logs, nil)))
			result, err := sweeper.Sweep(context.Background(), oauth2session.SweepRequest{Lookahead: 5 * time.Minute, PageSize: 2})
			require.NoError(t, err)
			assertSweepCounts(t, result, 0, 0, 1, false)
			assert.EqualValues(t, tc.calls, calls.Load())
			assert.True(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "WARN", oauth2session.RefreshTriggerSweep), "a provider rejection commits before it is reported")
			assertRefreshFailureMetadata(t, fixture, tc.detail, tc.kind, tc.dependency)
			persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
			require.NoError(t, err)
			assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
		})
	}
}

func TestRefreshFailureEvent_CommitFailureNeverReportsSuccess(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger oauth2session.RefreshTrigger
	}{
		{name: "forced refresh", trigger: oauth2session.RefreshTriggerOnDemand},
		{name: "background refresh", trigger: oauth2session.RefreshTriggerBackground},
		{name: "sweep refresh", trigger: oauth2session.RefreshTriggerSweep},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
			}))
			defer upstream.Close()
			expiry := time.Now().Add(time.Minute)
			if tc.trigger == oauth2session.RefreshTriggerOnDemand {
				expiry = time.Now().Add(time.Hour)
			}
			fixture := newLoggedRefreshFixture(t, upstream.URL, expiry, nil, func(config *oauth2session.Config) {
				config.RefreshLookahead = 5 * time.Minute
				config.BackgroundRefreshWorkers = 1
			}, func(repo ports.UserSessionRefreshRepository) ports.UserSessionRefreshRepository {
				return failedCommitRefreshRepo{underlying: repo}
			})
			ctx := context.Background()
			switch tc.trigger {
			case oauth2session.RefreshTriggerOnDemand:
				_, err := fixture.service.ForceRefreshSession(ctx, fixture.principal, fixture.serviceID)
				assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailPersistenceFailed)
			case oauth2session.RefreshTriggerBackground:
				_, token, err := fixture.service.GetValidAccessToken(ctx, fixture.principal, fixture.serviceID)
				require.NoError(t, err)
				assert.Equal(t, "old-access", token)
				fixture.logs.waitForEvent(t, "session.oauth2.refresh_failed")
			case oauth2session.RefreshTriggerSweep:
				sweeper := oauth2session.NewSessionSweepService(fixture.sessions.(ports.UserSessionExpiryRepository), fixture.service, slog.New(slog.NewJSONHandler(fixture.logs, nil)))
				_, err := sweeper.Sweep(ctx, oauth2session.SweepRequest{Lookahead: 5 * time.Minute, PageSize: 2})
				assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailPersistenceFailed)
			}
			assert.EqualValues(t, 1, calls.Load())
			assert.Zero(t, fixture.logs.eventCount("session.oauth2.token_refreshed"), "a failed commit cannot produce a success event")
			if tc.trigger == oauth2session.RefreshTriggerSweep {
				fields, committed := fixture.logs.event(t, "session.oauth2.sweep_aborted", fixture.principal)
				assert.False(t, committed)
				assert.Equal(t, "ERROR", fields["level"])
				assert.Equal(t, string(oauth2session.RefreshTriggerSweep), fields["triggered_by"])
				metadata, ok := fields["oauth2_session"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, string(oauth2session.DetailPersistenceFailed), metadata["failure_detail"])
				assert.Zero(t, fixture.logs.eventCount("session.oauth2.refresh_failed"))
			} else {
				assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "ERROR", tc.trigger))
				assertRefreshFailureMetadata(t, fixture, oauth2session.DetailPersistenceFailed, oauth2session.KindInfrastructure, oauth2session.DependencySessionRepository)
			}
			persisted, err := fixture.sessions.FindByPrincipalAndService(ctx, fixture.principal, fixture.serviceID)
			require.NoError(t, err)
			assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
		})
	}
}

type failingRefreshEvents struct {
	ports.BusinessEventRepository
	failed error
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

func (r failingRefreshEvents) Append(ctx context.Context, event *model.BusinessEvent, queue bool) error {
	if event.Type == model.BusinessEventTypePrefix+"session-refreshed" {
		return r.failed
	}
	return r.BusinessEventRepository.Append(ctx, event, queue)
}

func (r observedSessionReads) FindByPrincipalAndServiceForUpdate(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	return r.FindByPrincipalAndService(ctx, principal, serviceID)
}
