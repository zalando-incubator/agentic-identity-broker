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

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
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
	_, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
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
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
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
		oauth2session.DefaultConfig(), slog.Default(),
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

func (r failedCommitRefreshRepo) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	return r.underlying.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
		updated, err := refresh(ctx, session)
		if err != nil || !updated {
			return updated, err
		}
		return false, errors.New("failed to commit refreshed session")
	})
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

func (r *refreshCommitWitness) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	updated := false
	current, err := r.UserSessionRefreshRepository.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
		changed, err := refresh(ctx, session)
		updated = changed
		return changed, err
	})
	if updated && err == nil && current != nil {
		r.committed.Store(true)
	}
	return current, err
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
			assert.NotContains(t, string(entry.entry), principal.String(), "refresh events must not disclose the user principal")
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

func newLoggedRefreshFixture(t *testing.T, endpoint string, accessExpiry time.Time, refreshExpiry *time.Time, configure func(*oauth2session.Config), wrap ...func(ports.UserSessionRefreshRepository) ports.UserSessionRefreshRepository) loggedRefreshFixture {
	t.Helper()
	_, _, sessions, grants, agents, providers := setupServiceWithConfig(t, nil)
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
		AccessTokenExpiresAt: &accessExpiry, RefreshTokenExpiresAt: refreshExpiry,
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
		&http.Client{}, nil, config, slog.New(slog.NewJSONHandler(logs, nil)))
	return loggedRefreshFixture{service: service, sessions: sessions, session: session, principal: principal, serviceID: serviceID, logs: logs}
}

func assertRefreshEvent(t *testing.T, fixture loggedRefreshFixture, event, level string, trigger oauth2session.RefreshTrigger) bool {
	t.Helper()
	fields, committed := fixture.logs.event(t, event, fixture.principal)
	assert.Equal(t, level, fields["level"])
	assert.Equal(t, fixture.session.ID.String(), fields["session_id"])
	assert.Equal(t, fixture.serviceID.String(), fields["service_id"])
	assert.Equal(t, string(trigger), fields["triggered_by"])
	assert.NotContains(t, fields, "principal")
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
		name           string
		force          bool
		refreshExpired bool
		detail         oauth2session.ErrorDetail
		providerCalls  int32
	}{
		{name: "provider rejects read-path refresh", detail: oauth2session.DetailRefreshRejected, providerCalls: 1},
		{name: "provider rejects forced refresh", force: true, detail: oauth2session.DetailRefreshRejected, providerCalls: 1},
		{name: "stored refresh token expired", refreshExpired: true, detail: oauth2session.DetailRefreshTokenExpired},
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
			var refreshExpiry *time.Time
			if tc.refreshExpired {
				refreshExpiry = &expiry
			}
			fixture := newLoggedRefreshFixture(t, upstream.URL, expiry, refreshExpiry, nil)
			ctx := context.Background()
			var err error
			if tc.force {
				_, err = fixture.service.ForceRefreshSession(ctx, fixture.principal, fixture.serviceID)
			} else {
				_, _, err = fixture.service.GetValidAccessToken(ctx, fixture.principal, fixture.serviceID)
			}
			metadata := assertOperationMetadata(t, err, oauth2session.OperationRefresh, tc.detail)
			assert.EqualValues(t, tc.providerCalls, calls.Load())
			if tc.providerCalls > 0 {
				assert.Equal(t, http.StatusBadRequest, metadata.StatusCode())
				assert.Equal(t, "invalid_grant", metadata.OAuthCode())
				assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
			} else {
				assert.ErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
			}
			assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "ERROR", oauth2session.RefreshTriggerOnDemand), "failed refresh must not commit")
			if tc.providerCalls > 0 {
				assertRefreshFailureMetadata(t, fixture, tc.detail, oauth2session.KindProvider, oauth2session.DependencyProvider)
			} else {
				assertRefreshFailureMetadata(t, fixture, tc.detail, oauth2session.KindSession, oauth2session.DependencySessionRepository)
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
	assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "WARN", oauth2session.RefreshTriggerBackground))
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
		name           string
		refreshExpired bool
		calls          int32
		detail         oauth2session.ErrorDetail
		kind           oauth2session.ErrorKind
		dependency     oauth2session.Dependency
	}{
		{name: "provider rejection", calls: 1, detail: oauth2session.DetailRefreshRejected, kind: oauth2session.KindProvider, dependency: oauth2session.DependencyProvider},
		{name: "expired refresh token", refreshExpired: true, detail: oauth2session.DetailRefreshTokenExpired, kind: oauth2session.KindSession, dependency: oauth2session.DependencySessionRepository},
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
			var refreshExpiry *time.Time
			if tc.refreshExpired {
				expired := time.Now().Add(-time.Minute)
				refreshExpiry = &expired
			}
			fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Minute), refreshExpiry, nil)
			sweeper := oauth2session.NewSessionSweepService(fixture.sessions.(ports.UserSessionExpiryRepository), fixture.service, slog.New(slog.NewJSONHandler(fixture.logs, nil)))
			result, err := sweeper.Sweep(context.Background(), oauth2session.SweepRequest{Lookahead: 5 * time.Minute, PageSize: 2})
			require.NoError(t, err)
			assertSweepCounts(t, result, 0, 0, 1, false)
			assert.EqualValues(t, tc.calls, calls.Load())
			assert.False(t, assertRefreshEvent(t, fixture, "session.oauth2.refresh_failed", "WARN", oauth2session.RefreshTriggerSweep))
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
