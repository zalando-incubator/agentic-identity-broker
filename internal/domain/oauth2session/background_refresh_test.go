package oauth2session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// A separate record lock models the storage port's update-only row lock without
// holding a global lock across an upstream request.
type backgroundRecord struct {
	mu      sync.Mutex
	session *storage.UserSession
}

type backgroundStore struct {
	mu      sync.RWMutex
	records map[string]*backgroundRecord
}

func backgroundKey(principal id.Principal, serviceID id.ServiceID) string {
	return principal.String() + "|" + serviceID.String()
}

func newBackgroundStore(sessions ...*storage.UserSession) *backgroundStore {
	store := &backgroundStore{records: make(map[string]*backgroundRecord, len(sessions))}
	for _, session := range sessions {
		store.records[backgroundKey(session.Principal, session.ServiceID)] = &backgroundRecord{session: session}
	}
	return store
}

func (r *backgroundStore) record(principal id.Principal, serviceID id.ServiceID) *backgroundRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.records[backgroundKey(principal, serviceID)]
}

func (r *backgroundStore) find(_ context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	record := r.record(principal, serviceID)
	if record == nil {
		return nil, nil
	}
	record.mu.Lock()
	defer record.mu.Unlock()
	copy := *record.session
	copy.EncryptedAccessToken = bytes.Clone(record.session.EncryptedAccessToken)
	copy.EncryptedRefreshToken = bytes.Clone(record.session.EncryptedRefreshToken)
	return &copy, nil
}

func (r *backgroundStore) withLocked(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	record := r.record(principal, serviceID)
	if record == nil {
		return nil, nil
	}
	record.mu.Lock()
	defer record.mu.Unlock()
	current := *record.session
	current.EncryptedAccessToken = bytes.Clone(record.session.EncryptedAccessToken)
	current.EncryptedRefreshToken = bytes.Clone(record.session.EncryptedRefreshToken)
	changed, err := refresh(ctx, &current)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record.session = &current
	}
	return &current, nil
}

func (r *backgroundStore) delete(principal id.Principal, serviceID id.ServiceID) {
	key := backgroundKey(principal, serviceID)
	r.mu.Lock()
	delete(r.records, key)
	r.mu.Unlock()
}

type backgroundLog struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *backgroundLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *backgroundLog) events(t *testing.T, event string) []map[string]any {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	var found []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(l.b.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		if entry["event"] == event {
			found = append(found, entry)
		}
	}
	return found
}

func (l *backgroundLog) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func backgroundSession(serviceID id.ServiceID, principal id.Principal) *storage.UserSession {
	expiry := time.Now().Add(2 * time.Minute)
	return &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken:  []byte("sealed:old-" + principal.String()),
		EncryptedRefreshToken: []byte("sealed:refresh-" + principal.String()),
		TokenType:             "Bearer", AccessTokenExpiresAt: &expiry,
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
	}
}

func newBackgroundFixture(t *testing.T, sessions ...*storage.UserSession) (*OAuth2SessionService, *backgroundStore, *backgroundLog) {
	t.Helper()
	logs := new(backgroundLog)
	service, provider, _ := newOperationTestService(t, logs)
	provider.ID = sessions[0].ServiceID
	store := newBackgroundStore(sessions...)
	service.sessionRepo = operationSessionRepository{find: store.find}
	service.refreshRepo = operationRefreshRepository(store.withLocked)
	service.encryption = operationEncryption{
		encrypt: func(_ context.Context, data []byte, binding map[string]string) ([]byte, error) {
			require.Equal(t, sessions[0].ServiceID.String(), binding["service_id"])
			return []byte("sealed:" + string(data)), nil
		},
		decrypt: func(_ context.Context, data []byte, binding map[string]string) ([]byte, error) {
			require.Equal(t, sessions[0].ServiceID.String(), binding["service_id"])
			if !bytes.HasPrefix(data, []byte("sealed:")) {
				return nil, errors.New("expected encrypted token")
			}
			return bytes.Clone(data[len("sealed:"):]), nil
		},
	}
	service.logger = slog.New(slog.NewJSONHandler(logs, nil))
	service.config.MaxRetries = 1
	service.config.RefreshLookahead = 5 * time.Minute
	return service, store, logs
}

func backgroundMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previous)
		assert.NoError(t, provider.Shutdown(context.Background()))
	})
	return reader
}

func assertBackgroundMetrics(t *testing.T, reader *sdkmetric.ManualReader, expected map[string]int64) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	got := make(map[string]int64)
	for _, scope := range collected.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if !strings.HasPrefix(instrument.Name, "proactive_refresh_") {
				continue
			}
			sum, ok := instrument.Data.(metricdata.Sum[int64])
			require.True(t, ok, "%s must be an Int64Counter", instrument.Name)
			for _, point := range sum.DataPoints {
				attributes := point.Attributes.ToSlice()
				require.Len(t, attributes, 1, "session ID, service ID, and principal must not be metric labels")
				assert.Equal(t, "triggered_by", string(attributes[0].Key))
				assert.Equal(t, "background", attributes[0].Value.AsString())
				got[instrument.Name] += point.Value
			}
		}
	}
	for name, value := range expected {
		assert.Equal(t, value, got[name], "counter %s", name)
	}
}

func receiveBackground[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("background operation did not reach its expected synchronization point")
		var zero T
		return zero
	}
}

func submitBackground(t *testing.T, pool *backgroundRefresher, ctx context.Context, session *storage.UserSession) {
	t.Helper()
	pool.service.background = pool
	returned := make(chan struct{})
	go func() { pool.submit(ctx, session, 5*time.Minute); close(returned) }()
	receiveBackground(t, returned)
}

func TestBackgroundRefresh_CreatesRootSpanLinkedToSubmittingRequest(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "linked-user")
	service, _, _ := newBackgroundFixture(t, session)
	service.httpClient.Transport = operationTransport(func(_ *http.Request) (*http.Response, error) {
		return operationTokenResponse(http.StatusOK, `{"access_token":"renewed-access","refresh_token":"renewed-refresh","expires_in":3600}`), nil
	})
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	requestCtx, requestSpan := provider.Tracer("exchange").Start(context.Background(), "token_exchange")
	pool := newBackgroundRefresher(service, 1)
	pool.tracer = provider.Tracer("oauth2session")
	submitBackground(t, pool, requestCtx, session)
	require.NoError(t, pool.Close(context.Background()))
	requestSpan.End()

	var background sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "oauth2session.background_refresh" {
			background = span
		}
	}
	require.NotNil(t, background)
	assert.False(t, background.Parent().IsValid(), "background refresh must start a separate trace")
	require.Len(t, background.Links(), 1)
	assert.Equal(t, requestSpan.SpanContext(), background.Links()[0].SpanContext)
	assert.NotEqual(t, requestSpan.SpanContext().TraceID(), background.SpanContext().TraceID())
}

func TestBackgroundRefresh_SubmitIsNonblockingDeduplicatedAndDropsOnlyDistinctSaturatedWork(t *testing.T) {
	reader := backgroundMetrics(t)
	serviceID := id.NewServiceID()
	first := backgroundSession(serviceID, "first-user")
	second := backgroundSession(serviceID, "second-user")
	third := backgroundSession(serviceID, "third-user")
	service, store, logs := newBackgroundFixture(t, first, second, third)
	service.sessionRepo = operationSessionRepository{find: func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
		if principal == first.Principal {
			stale := *first
			return &stale, nil
		}
		return store.find(ctx, principal, serviceID)
	}}
	started := make(chan string, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	service.httpClient.Transport = operationTransport(func(r *http.Request) (*http.Response, error) {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		principal := strings.TrimPrefix(r.Form.Get("refresh_token"), "refresh-")
		started <- principal
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return operationTokenResponse(200, fmt.Sprintf(`{"access_token":"new-%s","refresh_token":"rotated-%s","expires_in":3600}`, principal, principal)), nil
	})
	pool := newBackgroundRefresher(service, 2)
	request, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	submitBackground(t, pool, request, first)
	assert.Equal(t, "first-user", receiveBackground(t, started))
	cancelRequest() // The admitted refresh must outlive its HTTP caller.
	submitBackground(t, pool, context.Background(), first)
	submitBackground(t, pool, context.Background(), second)
	assert.Equal(t, "second-user", receiveBackground(t, started), "duplicate first-user must not take the second slot")
	submitBackground(t, pool, context.Background(), third)

	current, token, err := service.GetValidAccessToken(context.Background(), first.Principal, first.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, first.ID, current.ID)
	assert.Equal(t, "old-first-user", token, "the caller must receive its current valid token before provider completion")
	assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 2, "proactive_refresh_dropped_total": 1, "proactive_refresh_failed_total": 0})
	drops := logs.events(t, "session.oauth2.proactive_refresh_dropped")
	require.Len(t, drops, 1)
	assert.Equal(t, "WARN", drops[0]["level"])
	assert.Equal(t, third.ID.String(), drops[0]["session_id"])
	assert.Equal(t, serviceID.String(), drops[0]["service_id"])
	assert.NotContains(t, logs.text(), first.Principal.String())

	releaseOnce.Do(func() { close(release) })
	require.NoError(t, pool.Close(context.Background()))
	for _, session := range []*storage.UserSession{first, second} {
		persisted, err := store.find(context.Background(), session.Principal, session.ServiceID)
		require.NoError(t, err)
		newAccess, err := service.DecryptAccessToken(context.Background(), persisted)
		require.NoError(t, err)
		assert.NotEqual(t, newAccess, string(persisted.EncryptedAccessToken))
		assert.Equal(t, "new-"+session.Principal.String(), newAccess)
		newRefresh, err := service.DecryptRefreshToken(context.Background(), persisted)
		require.NoError(t, err)
		assert.Equal(t, "rotated-"+session.Principal.String(), newRefresh)
	}
	unmodified, err := store.find(context.Background(), third.Principal, third.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, third.EncryptedAccessToken, unmodified.EncryptedAccessToken)
	submitBackground(t, pool, context.Background(), third)
	require.NoError(t, pool.Close(context.Background()))
	assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 2, "proactive_refresh_dropped_total": 1, "proactive_refresh_failed_total": 0})
	select {
	case unexpected := <-started:
		t.Fatalf("closed pool made another upstream request: %s", unexpected)
	default:
	}
}

func TestBackgroundRefresh_OnDemandJoinDecryptsSharedCiphertext(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "joined-user")
	service, store, _ := newBackgroundFixture(t, session)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var requests atomic.Int32
	service.httpClient.Transport = operationTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return operationTokenResponse(200, `{"access_token":"renewed-access","refresh_token":"renewed-refresh","expires_in":3600}`), nil
	})
	pool := newBackgroundRefresher(service, 1)
	submitBackground(t, pool, context.Background(), session)
	receiveBackground(t, started)
	read := make(chan struct{}, 1)
	service.sessionRepo = operationSessionRepository{find: func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
		stale := *session
		expired := time.Now().Add(-time.Minute)
		stale.AccessTokenExpiresAt = &expired
		read <- struct{}{}
		return &stale, nil
	}}
	type result struct {
		session *storage.UserSession
		token   string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		current, token, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
		done <- result{current, token, err}
	}()
	receiveBackground(t, read)
	select {
	case <-done:
		t.Fatal("expired caller returned before its shared background flight completed")
	default:
	}
	close(release)
	joined := receiveBackground(t, done)
	require.NoError(t, joined.err)
	require.NotNil(t, joined.session)
	assert.Equal(t, "renewed-access", joined.token)
	assert.NotEqual(t, joined.token, string(joined.session.EncryptedAccessToken), "singleflight must return ciphertext, not a shared plaintext token")
	stored, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, stored.EncryptedAccessToken, joined.session.EncryptedAccessToken)
	require.NoError(t, pool.Close(context.Background()))
	assert.EqualValues(t, 1, requests.Load())
}

func TestBackgroundRefresh_ShortLivedOnDemandRenewalIsNotRepeated(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "short-lived-user")
	expired := time.Now().Add(-time.Minute)
	session.AccessTokenExpiresAt = &expired
	service, store, _ := newBackgroundFixture(t, session)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var requests atomic.Int32
	service.httpClient.Transport = operationTransport(func(r *http.Request) (*http.Response, error) {
		call := requests.Add(1)
		if call == 1 {
			started <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return nil, r.Context().Err()
			}
		}
		return operationTokenResponse(http.StatusOK, fmt.Sprintf(`{"access_token":"renewed-%d","refresh_token":"rotated-%d","expires_in":60}`, call, call)), nil
	})
	caller := make(chan error, 1)
	go func() {
		_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
		caller <- err
	}()
	receiveBackground(t, started)
	flightChannel := service.refreshGroup.DoChan(backgroundKey(session.Principal, session.ServiceID), func() (any, error) {
		t.Error("on-demand flight ended before the shared result was observed")
		return nil, nil
	})
	close(release)
	require.NoError(t, receiveBackground(t, caller))
	flight := receiveBackground(t, flightChannel)
	require.NoError(t, flight.Err)
	shared := flight.Val.(refreshFlightResult)
	require.Equal(t, RefreshTriggerOnDemand, shared.trigger)
	require.True(t, shared.refreshed)
	require.True(t, shared.session.AccessTokenExpiresBy(time.Now().Add(5*time.Minute)))
	pool := newBackgroundRefresher(service, 1)
	continued, err := pool.continueSharedRefresh(context.Background(), session.Principal, session.ServiceID, 5*time.Minute, shared)
	require.NoError(t, err)
	require.True(t, continued.refreshed)
	require.NoError(t, pool.Close(context.Background()))
	assert.EqualValues(t, 1, requests.Load(), "the joined background request must not rotate a freshly issued token again")
	stored, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("sealed:renewed-1"), stored.EncryptedAccessToken)
}

func TestBackgroundRefresh_OnDemandNoOpStillRefreshesInsideLookahead(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "noop-user")
	service, store, _ := newBackgroundFixture(t, session)
	var requests atomic.Int32
	service.httpClient.Transport = operationTransport(func(_ *http.Request) (*http.Response, error) {
		requests.Add(1)
		return operationTokenResponse(http.StatusOK, `{"access_token":"renewed-access","refresh_token":"renewed-refresh","expires_in":3600}`), nil
	})
	current, changed, _, err := service.refreshDueSession(context.Background(), session.Principal, session.ServiceID, time.Now(), RefreshTriggerOnDemand)
	require.NoError(t, err)
	require.False(t, changed)
	require.EqualValues(t, 0, requests.Load())
	pool := newBackgroundRefresher(service, 1)
	continued, err := pool.continueSharedRefresh(context.Background(), session.Principal, session.ServiceID, 5*time.Minute,
		refreshFlightResult{session: current, refreshed: changed, trigger: RefreshTriggerOnDemand})
	require.NoError(t, err)
	require.True(t, continued.refreshed)
	require.NoError(t, pool.Close(context.Background()))
	assert.EqualValues(t, 1, requests.Load())
	stored, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("sealed:renewed-access"), stored.EncryptedAccessToken)
}

// Done is first consulted after DoChan registers the on-demand caller with singleflight.
type joinedFlightContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *joinedFlightContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

func TestBackgroundRefresh_OnDemandJoinReportsSynchronousFailure(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "joined-failure-user")
	service, store, logs := newBackgroundFixture(t, session)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var requests atomic.Int32
	service.httpClient.Transport = operationTransport(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		return operationTokenResponse(http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
	})
	pool := newBackgroundRefresher(service, 1)
	submitBackground(t, pool, context.Background(), session)
	receiveBackground(t, started)
	service.sessionRepo = operationSessionRepository{find: func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
		stale := *session
		expired := time.Now().Add(-time.Minute)
		stale.AccessTokenExpiresAt = &expired
		return &stale, nil
	}}
	caller := &joinedFlightContext{Context: context.Background(), joined: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, _, err := service.GetValidAccessToken(caller, session.Principal, session.ServiceID)
		done <- err
	}()
	receiveBackground(t, caller.joined)
	close(release)
	require.Error(t, receiveBackground(t, done))
	require.NoError(t, pool.Close(context.Background()))
	assert.EqualValues(t, 1, requests.Load())
	stored, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, session.EncryptedAccessToken, stored.EncryptedAccessToken)
	assert.Equal(t, session.EncryptedRefreshToken, stored.EncryptedRefreshToken)
	events := logs.events(t, "session.oauth2.refresh_failed")
	require.Len(t, events, 2, "both failed background refresh and synchronous caller need an event")
	triggers := make(map[string]string, len(events))
	for _, event := range events {
		triggers[event["triggered_by"].(string)] = event["level"].(string)
		assert.Equal(t, true, event["public_client"], "known provider type must survive the shared flight")
	}
	assert.Equal(t, "WARN", triggers[string(RefreshTriggerBackground)])
	assert.Equal(t, "ERROR", triggers[string(RefreshTriggerOnDemand)])
	assert.NotContains(t, logs.text(), session.Principal.String())
}

func TestBackgroundRefresh_CloseDrainsCommittedTokenBeforeCancelingAndRejectsLateWork(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "drain-user")
	service, store, _ := newBackgroundFixture(t, session)
	committing := make(chan struct{}, 1)
	releaseCommit := make(chan struct{})
	defer func() {
		select {
		case <-releaseCommit:
		default:
			close(releaseCommit)
		}
	}()
	service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
		return store.withLocked(ctx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
			changed, err := refresh(ctx, current)
			if changed && err == nil {
				committing <- struct{}{}
				select {
				case <-releaseCommit:
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
			return changed, err
		})
	})
	pool := newBackgroundRefresher(service, 1)
	submitBackground(t, pool, context.Background(), session)
	receiveBackground(t, committing)
	closed := make(chan error, 1)
	go func() { closed <- pool.Close(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before committing the rotated token: %v", err)
	default:
	}
	close(releaseCommit)
	require.NoError(t, receiveBackground(t, closed))
	persisted, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, []byte("sealed:new-access"), persisted.EncryptedAccessToken)
	assert.Equal(t, []byte("sealed:new-refresh"), persisted.EncryptedRefreshToken)
	require.NoError(t, pool.Close(context.Background()))
}

func TestBackgroundRefresh_CloseDeadlineCancelsActiveRequest(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "deadline-user")
	service, store, _ := newBackgroundFixture(t, session)
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	service.httpClient.Transport = operationTransport(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
		return nil, r.Context().Err()
	})
	pool := newBackgroundRefresher(service, 1)
	submitBackground(t, pool, context.Background(), session)
	receiveBackground(t, started)
	shutdown, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- pool.Close(shutdown) }()
	assert.ErrorIs(t, receiveBackground(t, result), context.DeadlineExceeded)
	receiveBackground(t, canceled)
	persisted, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Equal(t, session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
}

func TestBackgroundRefresh_DeletedSessionIsNotRecreated(t *testing.T) {
	session := backgroundSession(id.NewServiceID(), "deleted-user")
	service, store, logs := newBackgroundFixture(t, session)
	beforeLock := make(chan struct{}, 1)
	releaseLock := make(chan struct{})
	defer func() {
		select {
		case <-releaseLock:
		default:
			close(releaseLock)
		}
	}()
	service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
		beforeLock <- struct{}{}
		<-releaseLock
		return store.withLocked(ctx, principal, serviceID, refresh)
	})
	var requests atomic.Int32
	service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return operationTokenResponse(200, `{"access_token":"resurrected","expires_in":3600}`), nil
	})
	pool := newBackgroundRefresher(service, 1)
	submitBackground(t, pool, context.Background(), session)
	receiveBackground(t, beforeLock)
	store.delete(session.Principal, session.ServiceID)
	close(releaseLock)
	require.NoError(t, pool.Close(context.Background()))
	found, err := store.find(context.Background(), session.Principal, session.ServiceID)
	require.NoError(t, err)
	assert.Nil(t, found)
	assert.Zero(t, requests.Load())
	assert.Empty(t, logs.events(t, "session.oauth2.refresh_failed"), "a deleted session is not a background failure")
}

func TestBackgroundRefresh_ClassifiesProviderAndEncryptionErrorsWithFixedCardinalityCounters(t *testing.T) {
	for _, tc := range []struct {
		name      string
		level     string
		configure func(*OAuth2SessionService)
	}{
		{"provider rejection", "WARN", func(service *OAuth2SessionService) {
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				return operationTokenResponse(http.StatusBadRequest, `{"error":"invalid_grant","error_description":"private-provider-body"}`), nil
			})
		}},
		{"encryption outage", "ERROR", func(service *OAuth2SessionService) {
			service.encryption = operationEncryption{
				decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
					return bytes.Clone(bytes.TrimPrefix(data, []byte("sealed:"))), nil
				},
				encrypt: func(context.Context, []byte, map[string]string) ([]byte, error) {
					return nil, errors.New("private-encryption-cause")
				},
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := backgroundMetrics(t)
			session := backgroundSession(id.NewServiceID(), "private-user")
			service, store, logs := newBackgroundFixture(t, session)
			tc.configure(service)
			pool := newBackgroundRefresher(service, 1)
			submitBackground(t, pool, context.Background(), session)
			require.NoError(t, pool.Close(context.Background()))
			unchanged, err := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, err)
			assert.Equal(t, session.EncryptedAccessToken, unchanged.EncryptedAccessToken)
			assert.Equal(t, session.EncryptedRefreshToken, unchanged.EncryptedRefreshToken)
			failures := logs.events(t, "session.oauth2.refresh_failed")
			require.Len(t, failures, 1, "a background attempt must classify its failure exactly once")
			assert.Equal(t, tc.level, failures[0]["level"])
			assert.Equal(t, session.ID.String(), failures[0]["session_id"])
			assert.Equal(t, "background", failures[0]["triggered_by"])
			assert.Equal(t, true, failures[0]["public_client"], "known provider type must reach background failure logs")
			assert.NotContains(t, logs.text(), "private-provider-body")
			assert.NotContains(t, logs.text(), "private-encryption-cause")
			assert.NotContains(t, logs.text(), session.Principal.String())
			assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 1, "proactive_refresh_dropped_total": 0, "proactive_refresh_failed_total": 1})
		})
	}
}
