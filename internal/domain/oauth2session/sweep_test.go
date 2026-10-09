package oauth2session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type sweepListCall struct {
	threshold time.Time
	cursor    storage.SessionExpiryCursor
	limit     int
}

type observedSweepExpiry struct {
	base      ports.UserSessionExpiryRepository
	calls     []sweepListCall
	afterList func(int, []*storage.UserSession)
	list      func(context.Context, time.Time, storage.SessionExpiryCursor, int) ([]*storage.UserSession, error)
}

func (r *observedSweepExpiry) ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int) ([]*storage.UserSession, error) {
	r.calls = append(r.calls, sweepListCall{threshold, cursor, limit})
	if r.list != nil {
		return r.list(ctx, threshold, cursor, limit)
	}
	rows, err := r.base.ListExpiringSessions(ctx, threshold, cursor, limit)
	if err == nil && r.afterList != nil {
		r.afterList(len(r.calls), rows)
	}
	return rows, err
}

type observedSweepRefresh struct {
	base   ports.UserSessionRefreshRepository
	locks  int
	invoke func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error)
}

func (r *observedSweepRefresh) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, callback func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	r.locks++
	if r.invoke != nil {
		return r.invoke(ctx, principal, serviceID, callback)
	}
	return r.base.WithLockedSession(ctx, principal, serviceID, callback)
}

type unavailableSweepProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository
	cause error
}

func (r unavailableSweepProviderRepository) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, storage.NewStorageError("Get", storage.ErrorKindConnection, r.cause, "provider repository unavailable")
}

type countingSweepProviderRepository struct {
	ports.ThirdpartyOAuth2ProviderRepository
	gets map[id.ServiceID]int
}

func (r *countingSweepProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.gets[serviceID]++
	return r.ThirdpartyOAuth2ProviderRepository.Get(ctx, serviceID)
}

type sweepFixture struct {
	t              *testing.T
	repo           *memory.InMemoryUserSessionRepository
	expiry         *observedSweepExpiry
	refresh        *observedSweepRefresh
	providers      *thirdparty.ThirdpartyOAuth2ProviderService
	seedEncryption ports.EncryptionPort
	service        *oauth2session.OAuth2SessionService
	serviceID      id.ServiceID
	logs           *strings.Builder
}

func newSweepFixture(t *testing.T) *sweepFixture {
	t.Helper()
	_, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	repo := sessions.(*memory.InMemoryUserSessionRepository)
	f := &sweepFixture{
		t: t, repo: repo, providers: providers, serviceID: id.NewServiceID(),
		seedEncryption: newTestEncryption(t), logs: &strings.Builder{},
		expiry:  &observedSweepExpiry{base: repo},
		refresh: &observedSweepRefresh{base: repo},
	}
	f.resetService(oauth2session.DefaultConfig(), f.seedEncryption)
	return f
}

func (f *sweepFixture) resetService(config oauth2session.Config, encryption ports.EncryptionPort) {
	f.service = oauth2session.NewOAuth2SessionService(
		f.providers, f.repo, f.refresh, nil, nil, encryption,
		&http.Client{Timeout: 10 * time.Second}, nil, config,
		slog.New(slog.NewJSONHandler(f.logs, nil)),
	)
}

func (f *sweepFixture) sweeper() *oauth2session.SessionSweepService {
	return oauth2session.NewSessionSweepService(f.expiry, f.service, slog.New(slog.NewJSONHandler(f.logs, nil)))
}

func sweepSessionID(n int) id.SessionID {
	return id.MustParseSessionID(fmt.Sprintf("00000000-0000-4000-8000-%012x", n))
}

func (f *sweepFixture) seed(n int, accessExpiry, refreshExpiry *time.Time, hasRefresh bool) *storage.UserSession {
	f.t.Helper()
	ctx := context.Background()
	encryptionContext := domainencryption.NewServiceBranchKeySubject(f.serviceID).EncryptionContext()
	access, err := f.seedEncryption.Encrypt(ctx, []byte(fmt.Sprintf("old-access-%d", n)), encryptionContext)
	require.NoError(f.t, err)
	var refresh []byte
	if hasRefresh {
		refresh, err = f.seedEncryption.Encrypt(ctx, []byte(fmt.Sprintf("refresh-%d", n)), encryptionContext)
		require.NoError(f.t, err)
	}
	session := &storage.UserSession{
		ID: sweepSessionID(n), Principal: id.Principal(fmt.Sprintf("user-%d@example.com", n)), ServiceID: f.serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: accessExpiry, RefreshTokenExpiresAt: refreshExpiry,
		EncryptionContext: storage.EncryptionContext{ServiceID: f.serviceID}, InitiatedAt: time.Now(), CreatedAt: time.Now(),
	}
	require.NoError(f.t, f.repo.Create(ctx, session))
	return session
}

func (f *sweepFixture) provider(handler http.HandlerFunc) {
	f.t.Helper()
	server := httptest.NewServer(handler)
	f.t.Cleanup(server.Close)
	provider := createTestService(f.serviceID)
	provider.Endpoints.TokenEndpoint = server.URL
	require.NoError(f.t, f.providers.Create(context.Background(), provider))
}

func sweepSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
}

func assertSweepCounts(t *testing.T, got oauth2session.SweepResult, refreshed, skipped, failed int, dryRun bool) {
	t.Helper()
	assert.Equal(t, refreshed, got.Refreshed)
	assert.Equal(t, skipped, got.Skipped)
	assert.Equal(t, failed, got.Failed)
	assert.Equal(t, refreshed+skipped+failed, got.TotalEvaluated)
	assert.Equal(t, dryRun, got.DryRun)
}

func TestSessionSweepRequestBoundsAndConfiguredDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    oauth2session.SweepRequest
		config oauth2session.Config
	}{
		{"negative lookahead", oauth2session.SweepRequest{Lookahead: -time.Nanosecond}, oauth2session.DefaultConfig()},
		{"negative page size", oauth2session.SweepRequest{PageSize: -1}, oauth2session.DefaultConfig()},
		{"page size over maximum", oauth2session.SweepRequest{PageSize: 1001}, oauth2session.DefaultConfig()},
		{"invalid configured lookahead", oauth2session.SweepRequest{}, oauth2session.Config{RefreshLookahead: -time.Minute, SweepDefaultPageSize: 100}},
		{"invalid configured page size", oauth2session.SweepRequest{}, oauth2session.Config{RefreshLookahead: time.Minute, SweepDefaultPageSize: 1001}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSweepFixture(t)
			f.resetService(tc.config, f.seedEncryption)
			_, err := f.sweeper().Sweep(context.Background(), tc.req)
			require.ErrorIs(t, err, oauth2session.ErrInvalidSweepRequest)
			assert.Empty(t, f.expiry.calls, "invalid input must not query storage")
			assert.Zero(t, f.refresh.locks)
		})
	}
	for _, tc := range []struct {
		name      string
		config    oauth2session.Config
		lookahead time.Duration
		pageSize  int
	}{
		{"built-in defaults", oauth2session.DefaultConfig(), 5 * time.Minute, 100},
		{"configured defaults", oauth2session.Config{RefreshLookahead: 17 * time.Minute, SweepDefaultPageSize: 7}, 17 * time.Minute, 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSweepFixture(t)
			f.resetService(tc.config, f.seedEncryption)
			before := time.Now()
			result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true})
			after := time.Now()
			require.NoError(t, err)
			assertSweepCounts(t, result, 0, 0, 0, true)
			require.Len(t, f.expiry.calls, 1, "even an empty sweep must query once")
			call := f.expiry.calls[0]
			assert.Equal(t, tc.pageSize, call.limit)
			assert.Equal(t, storage.SessionExpiryCursor{}, call.cursor)
			assert.False(t, call.threshold.Before(before.Add(tc.lookahead)))
			assert.False(t, call.threshold.After(after.Add(tc.lookahead)))
		})
	}
}

func TestSessionSweepReportsEffectiveConfiguredControls(t *testing.T) {
	f := newSweepFixture(t)
	f.resetService(oauth2session.Config{RefreshLookahead: 17 * time.Minute, SweepDefaultPageSize: 7}, f.seedEncryption)

	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, 17*time.Minute, result.EffectiveLookahead)
	assert.Equal(t, 7, result.EffectivePageSize)
	require.Len(t, f.expiry.calls, 1)
	assert.Equal(t, 7, f.expiry.calls[0].limit, "the reported page size must be the one used to query sessions")
}

func TestSessionSweepRequestOverridesBothConfiguredDefaults(t *testing.T) {
	f := newSweepFixture(t)
	f.resetService(oauth2session.Config{RefreshLookahead: time.Minute, SweepDefaultPageSize: 100}, f.seedEncryption)
	inside, outside := time.Now().Add(6*time.Minute), time.Now().Add(time.Hour)
	f.seed(1, &inside, nil, true)
	f.seed(2, &outside, nil, true)
	before := time.Now()
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{
		Lookahead: 10 * time.Minute, PageSize: 1, DryRun: true,
	})
	after := time.Now()
	require.NoError(t, err)
	assertSweepCounts(t, result, 1, 0, 0, true)
	require.Len(t, f.expiry.calls, 2)
	for _, call := range f.expiry.calls {
		assert.Equal(t, 1, call.limit)
		assert.False(t, call.threshold.Before(before.Add(10*time.Minute)))
		assert.False(t, call.threshold.After(after.Add(10*time.Minute)))
	}
	assert.Equal(t, storage.SessionExpiryCursor{AccessTokenExpiresAt: inside, ID: sweepSessionID(1)}, f.expiry.calls[1].cursor)
}

func TestSessionSweepDryRunClassifiesSnapshotsWithoutSideEffects(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); _ = meterProvider.Shutdown(context.Background()) })

	f := newSweepFixture(t)
	var upstreamCalls atomic.Int32
	f.provider(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		sweepSuccess(w)
	})
	soon, expired := time.Now().Add(time.Minute), time.Now().Add(-time.Minute)
	later := time.Now().Add(time.Hour)
	f.seed(1, &soon, nil, true)
	f.seed(2, &expired, nil, false)
	f.seed(3, &expired, &expired, true)
	f.seed(4, &later, nil, true)
	f.seed(5, nil, nil, true)
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true})
	require.NoError(t, err)
	assertSweepCounts(t, result, 1, 0, 2, true)
	assert.Zero(t, f.refresh.locks, "dry run must not acquire any session locks")
	assert.Zero(t, upstreamCalls.Load(), "dry run must not contact the upstream provider")
	assert.NotContains(t, f.logs.String(), "token_refreshed")
	assert.Empty(t, sweepMetrics(t, reader), "dry run must not emit either sweep counter")
	for _, n := range []int{1, 2, 3} {
		persisted, err := f.repo.Get(context.Background(), sweepSessionID(n))
		require.NoError(t, err)
		plain, err := f.service.DecryptAccessToken(context.Background(), persisted)
		require.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("old-access-%d", n), plain)
	}
}

func TestSessionSweepFixedThresholdAndCompositeCursorAcrossChangingPages(t *testing.T) {
	f := newSweepFixture(t)
	soon := time.Now().Add(time.Minute)
	for n := 1; n <= 5; n++ {
		f.seed(n, &soon, nil, true)
	}
	f.expiry.afterList = func(page int, _ []*storage.UserSession) {
		if page == 1 {
			// Deleting a row behind the cursor must not skip later expiry/ID tuples.
			require.NoError(t, f.repo.Delete(context.Background(), sweepSessionID(1)))
		}
	}
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true, PageSize: 2})
	require.NoError(t, err)
	assertSweepCounts(t, result, 5, 0, 0, true)
	require.Len(t, f.expiry.calls, 3)
	for i, call := range f.expiry.calls {
		assert.Equal(t, 2, call.limit)
		assert.Equal(t, f.expiry.calls[0].threshold, call.threshold, "the sweep uses its start-time threshold for every page")
		if i > 0 {
			assert.Equal(t, storage.SessionExpiryCursor{AccessTokenExpiresAt: soon, ID: sweepSessionID(i * 2)}, call.cursor, "the cursor is the previous page's final expiry and ID")
		}
	}
}

func TestSessionSweepEvaluatesMovedSessionOnce(t *testing.T) {
	f := newSweepFixture(t)
	var calls atomic.Int32
	f.provider(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.NoError(t, r.ParseForm())
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("refresh_token") {
		case "refresh-1":
			_, _ = io.WriteString(w, `{"access_token":"renewed-1","refresh_token":"rotated-1","token_type":"Bearer","expires_in":3600}`)
		case "refresh-2":
			_, _ = io.WriteString(w, `{"access_token":"renewed-2","refresh_token":"rotated-2","token_type":"Bearer","expires_in":7200}`)
		default:
			_, _ = io.WriteString(w, `{"access_token":"duplicate-refresh","refresh_token":"rotated-1","token_type":"Bearer","expires_in":7200}`)
		}
	})
	firstExpiry, secondExpiry := time.Now().Add(-time.Minute), time.Now().Add(time.Minute)
	f.seed(1, &firstExpiry, nil, true)
	f.seed(2, &secondExpiry, nil, true)
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{Lookahead: 90 * time.Minute, PageSize: 1})
	require.NoError(t, err)
	assertSweepCounts(t, result, 2, 0, 0, false)
	assert.EqualValues(t, 2, calls.Load(), "each session gets one upstream refresh")
	assert.Equal(t, 2, f.refresh.locks, "a moved row must not acquire a second lock")
	assert.Equal(t, 2, strings.Count(f.logs.String(), `"event":"session.oauth2.token_refreshed"`))
	first, err := f.repo.Get(context.Background(), sweepSessionID(1))
	require.NoError(t, err)
	access, err := f.service.DecryptAccessToken(context.Background(), first)
	require.NoError(t, err)
	assert.Equal(t, "renewed-1", access)
	require.Len(t, f.expiry.calls, 4, "a duplicate-only full page must still advance to an empty page")
	assert.Equal(t, storage.SessionExpiryCursor{AccessTokenExpiresAt: *first.AccessTokenExpiresAt, ID: first.ID}, f.expiry.calls[3].cursor)
}

func TestSessionSweepDryRunSkipsMovedCandidateID(t *testing.T) {
	f := newSweepFixture(t)
	firstExpiry, movedExpiry, secondExpiry := time.Now().Add(time.Minute), time.Now().Add(2*time.Minute), time.Now().Add(3*time.Minute)
	first := f.seed(1, &firstExpiry, nil, true)
	second := f.seed(2, &secondExpiry, nil, true)
	moved := *first
	moved.AccessTokenExpiresAt = &movedExpiry
	f.expiry.list = func(_ context.Context, _ time.Time, _ storage.SessionExpiryCursor, _ int) ([]*storage.UserSession, error) {
		switch len(f.expiry.calls) {
		case 1:
			return []*storage.UserSession{first}, nil
		case 2:
			return []*storage.UserSession{&moved}, nil
		case 3:
			return []*storage.UserSession{second}, nil
		default:
			return nil, nil
		}
	}
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true, PageSize: 1})
	require.NoError(t, err)
	assertSweepCounts(t, result, 2, 0, 0, true)
	assert.Zero(t, f.refresh.locks)
	require.Len(t, f.expiry.calls, 4)
	assert.Equal(t, storage.SessionExpiryCursor{AccessTokenExpiresAt: movedExpiry, ID: first.ID}, f.expiry.calls[2].cursor)
}

func TestSessionSweepRejectsInvalidExpiryPages(t *testing.T) {
	for _, name := range []string{"nil expiry", "repeated tuple", "decreasing tuple"} {
		t.Run(name, func(t *testing.T) {
			f := newSweepFixture(t)
			firstExpiry, secondExpiry := time.Now().Add(time.Minute), time.Now().Add(2*time.Minute)
			first := f.seed(1, &firstExpiry, nil, true)
			second := f.seed(2, &secondExpiry, nil, true)
			f.expiry.list = func(_ context.Context, _ time.Time, _ storage.SessionExpiryCursor, _ int) ([]*storage.UserSession, error) {
				switch name {
				case "nil expiry":
					invalid := *first
					invalid.AccessTokenExpiresAt = nil
					return []*storage.UserSession{&invalid}, nil
				case "repeated tuple":
					if len(f.expiry.calls) <= 2 {
						return []*storage.UserSession{first}, nil
					}
				case "decreasing tuple":
					if len(f.expiry.calls) == 1 {
						return []*storage.UserSession{second, first}, nil
					}
				}
				return nil, nil
			}
			pageSize := 1
			if name == "nil expiry" || name == "decreasing tuple" {
				pageSize = 2
			}
			_, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{DryRun: true, PageSize: pageSize})
			require.Error(t, err, "invalid repository pages must abort the sweep")
			assert.NotErrorIs(t, err, oauth2session.ErrInvalidSweepRequest)
			assertSweepLogMetadata(t, f.logs.String(), "session.oauth2.sweep_aborted", "ERROR", oauth2session.DetailRepositoryUnavailable, oauth2session.KindInfrastructure, oauth2session.DependencySessionRepository)
			assert.Zero(t, f.refresh.locks)
		})
	}
}

func TestSessionSweepRealRefreshCommitsBeforeSuccessAndMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); _ = meterProvider.Shutdown(context.Background()) })

	f := newSweepFixture(t)
	var upstreamCalls atomic.Int32
	f.provider(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		sweepSuccess(w)
	})
	soon := time.Now().Add(time.Minute)
	f.seed(1, &soon, nil, true)
	f.refresh.invoke = func(ctx context.Context, p id.Principal, serviceID id.ServiceID, callback func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
		return f.repo.WithLockedSession(ctx, p, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
			changed, err := callback(ctx, session)
			if changed && err == nil {
				assert.NotContains(t, f.logs.String(), `"event":"session.oauth2.token_refreshed"`, "success is logged only after storage commits")
				assert.Empty(t, sweepMetrics(t, reader), "success counter requires a committed update")
			}
			return changed, err
		})
	}
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
	require.NoError(t, err)
	assertSweepCounts(t, result, 1, 0, 0, false)
	assert.EqualValues(t, 1, upstreamCalls.Load())
	assert.Equal(t, 1, f.refresh.locks)
	persisted, err := f.repo.Get(context.Background(), sweepSessionID(1))
	require.NoError(t, err)
	access, err := f.service.DecryptAccessToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-access", access)
	refresh, err := f.service.DecryptRefreshToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-refresh", refresh)
	assert.EqualValues(t, 1, sweepMetrics(t, reader)["session_sweep_refreshed_total"])
	assert.EqualValues(t, 0, sweepMetrics(t, reader)["session_sweep_failed_total"])
	assert.Contains(t, f.logs.String(), `"triggered_by":"sweep"`)
	assert.Contains(t, f.logs.String(), `"event":"session.oauth2.token_refreshed"`)
	assert.Contains(t, f.logs.String(), `"session_id":"`+sweepSessionID(1).String()+`"`)
	assert.NotContains(t, f.logs.String(), "user-1@example.com", "the session principal must not be logged")
	assert.NotContains(t, f.logs.String(), "new-refresh", "rotated tokens must not be logged")
}

func TestSessionSweepFailedAndSkippedCandidatesDoNotStopNextRefresh(t *testing.T) {
	for _, tc := range []struct {
		name           string
		modify         func(*sweepFixture, *storage.UserSession)
		response       int
		wantFail       int
		wantSkip       int
		wantCalls      int32
		wantLevel      string
		wantDetail     oauth2session.ErrorDetail
		wantKind       oauth2session.ErrorKind
		wantDependency oauth2session.Dependency
	}{
		{name: "no refresh token", wantFail: 1, wantLevel: "WARN", wantDetail: oauth2session.DetailRefreshUnavailable, wantKind: oauth2session.KindSession, wantDependency: oauth2session.DependencySessionRepository},
		{name: "expired refresh token", wantFail: 1, wantLevel: "WARN", wantDetail: oauth2session.DetailRefreshTokenExpired, wantKind: oauth2session.KindSession, wantDependency: oauth2session.DependencySessionRepository},
		{name: "deleted after listing", modify: func(f *sweepFixture, s *storage.UserSession) {
			f.expiry.afterList = func(_ int, _ []*storage.UserSession) { require.NoError(f.t, f.repo.Delete(context.Background(), s.ID)) }
		}, wantSkip: 1},
		{name: "renewed after listing", modify: func(f *sweepFixture, s *storage.UserSession) {
			f.expiry.afterList = func(_ int, _ []*storage.UserSession) {
				later := time.Now().Add(time.Hour)
				_, err := f.repo.WithLockedSession(context.Background(), s.Principal, s.ServiceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
					current.AccessTokenExpiresAt = &later
					return true, nil
				})
				require.NoError(f.t, err)
			}
		}, wantSkip: 1},
		{name: "provider rejection", response: http.StatusBadRequest, wantFail: 1, wantCalls: 1, wantLevel: "WARN", wantDetail: oauth2session.DetailRefreshRejected, wantKind: oauth2session.KindProvider, wantDependency: oauth2session.DependencyProvider},
		{name: "provider unavailable", response: http.StatusServiceUnavailable, wantFail: 1, wantCalls: 1, wantLevel: "WARN", wantDetail: oauth2session.DetailProviderUnavailable, wantKind: oauth2session.KindInfrastructure, wantDependency: oauth2session.DependencyProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			previous := otel.GetMeterProvider()
			otel.SetMeterProvider(meterProvider)
			t.Cleanup(func() { otel.SetMeterProvider(previous); _ = meterProvider.Shutdown(context.Background()) })
			f := newSweepFixture(t)
			var calls atomic.Int32
			f.provider(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.NoError(t, r.ParseForm())
				if r.Form.Get("refresh_token") == "refresh-1" && tc.response != 0 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.response)
					_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
					return
				}
				sweepSuccess(w)
			})
			soon, past := time.Now().Add(time.Minute), time.Now().Add(-time.Minute)
			var refreshExpiry *time.Time
			hasRefresh := true
			switch tc.name {
			case "no refresh token":
				hasRefresh = false
			case "expired refresh token":
				refreshExpiry = &past
			}
			first := f.seed(1, &soon, refreshExpiry, hasRefresh)
			f.seed(2, &soon, nil, true)
			if tc.modify != nil {
				tc.modify(f, first)
			}
			result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
			require.NoError(t, err, "a single candidate must not abort the remaining sweep")
			assertSweepCounts(t, result, 1, tc.wantSkip, tc.wantFail, false)
			assert.EqualValues(t, 1+tc.wantCalls, calls.Load())
			assert.Equal(t, 2, f.refresh.locks)
			persisted, err := f.repo.Get(context.Background(), sweepSessionID(2))
			require.NoError(t, err)
			plain, err := f.service.DecryptAccessToken(context.Background(), persisted)
			require.NoError(t, err)
			assert.Equal(t, "new-access", plain)
			assert.EqualValues(t, 1, sweepMetrics(t, reader)["session_sweep_refreshed_total"])
			assert.EqualValues(t, tc.wantFail, sweepMetrics(t, reader)["session_sweep_failed_total"])
			if tc.wantFail == 1 {
				original, err := f.repo.Get(context.Background(), sweepSessionID(1))
				require.NoError(t, err)
				plain, err := f.service.DecryptAccessToken(context.Background(), original)
				require.NoError(t, err)
				assert.Equal(t, "old-access-1", plain, "failed candidates never persist partial tokens")
				assertSweepLogMetadata(t, f.logs.String(), "session.oauth2.refresh_failed", tc.wantLevel, tc.wantDetail, tc.wantKind, tc.wantDependency)
			}
		})
	}
}

func TestSessionSweepReportsUnrefreshableSessionOnEveryInvocationWithoutProviderLookup(t *testing.T) {
	f := newSweepFixture(t)
	providerRepo := &countingSweepProviderRepository{
		ThirdpartyOAuth2ProviderRepository: memory.NewInMemoryThirdpartyOAuth2ProviderRepository(),
		gets:                               make(map[id.ServiceID]int),
	}
	f.providers = thirdparty.NewThirdpartyOAuth2ProviderService(providerRepo, f.seedEncryption, newNoopBranchKeyManager(), nil, false, slog.Default())
	f.resetService(oauth2session.DefaultConfig(), f.seedEncryption)
	validServiceID := f.serviceID
	f.provider(func(w http.ResponseWriter, _ *http.Request) { sweepSuccess(w) })
	failedServiceID := id.NewServiceID()
	f.serviceID = failedServiceID
	accessExpiry, refreshExpiry := time.Now().Add(-time.Minute), time.Now().Add(-time.Hour)
	failed := f.seed(1, &accessExpiry, &refreshExpiry, true)
	failedAccess := bytes.Clone(failed.EncryptedAccessToken)
	failedRefresh := bytes.Clone(failed.EncryptedRefreshToken)
	f.serviceID = validServiceID
	f.seed(2, &accessExpiry, nil, true)

	for invocation := 1; invocation <= 2; invocation++ {
		result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{PageSize: 1})
		require.NoError(t, err)
		assertSweepCounts(t, result, 2-invocation, 0, 1, false)
		persisted, err := f.repo.Get(context.Background(), failed.ID)
		require.NoError(t, err)
		assert.Equal(t, failedAccess, persisted.EncryptedAccessToken)
		assert.Equal(t, failedRefresh, persisted.EncryptedRefreshToken)
		require.NotNil(t, persisted.AccessTokenExpiresAt)
		assert.True(t, accessExpiry.Equal(*persisted.AccessTokenExpiresAt))
		assert.Zero(t, providerRepo.gets[failedServiceID], "expired refresh tokens must not load provider credentials")
	}
	assert.Equal(t, 1, providerRepo.gets[validServiceID])
	assert.Equal(t, 2, strings.Count(f.logs.String(), `"event":"session.oauth2.refresh_failed"`))
}

func TestSessionSweepReauthorizedAfterListingUsesRefreshPath(t *testing.T) {
	f := newSweepFixture(t)
	var calls atomic.Int32
	f.provider(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "reauthorized-refresh", r.Form.Get("refresh_token"))
		sweepSuccess(w)
	})
	accessExpiry, oldRefreshExpiry := time.Now().Add(-time.Minute), time.Now().Add(-time.Hour)
	current := f.seed(1, &accessExpiry, &oldRefreshExpiry, true)
	refresh, err := f.seedEncryption.Encrypt(context.Background(), []byte("reauthorized-refresh"), domainencryption.NewServiceBranchKeySubject(f.serviceID).EncryptionContext())
	require.NoError(t, err)
	f.expiry.afterList = func(_ int, _ []*storage.UserSession) {
		validUntil := time.Now().Add(time.Hour)
		_, err := f.repo.WithLockedSession(context.Background(), current.Principal, current.ServiceID, func(_ context.Context, latest *storage.UserSession) (bool, error) {
			latest.EncryptedRefreshToken = refresh
			latest.RefreshTokenExpiresAt = &validUntil
			return true, nil
		})
		require.NoError(t, err)
	}
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
	require.NoError(t, err)
	assertSweepCounts(t, result, 1, 0, 0, false)
	assert.EqualValues(t, 1, calls.Load())
	assert.Equal(t, 2, f.refresh.locks, "the stale snapshot needs a read-only locked recheck before refresh")
	assert.NotContains(t, f.logs.String(), `"event":"session.oauth2.refresh_failed"`)
}

func TestSessionSweepRepositoryFailuresAbortInsteadOfMiscounting(t *testing.T) {
	for _, tc := range []struct {
		name         string
		listFails    bool
		providerRepo bool
		terminal     bool
	}{
		{"expiry listing unavailable", true, false, false},
		{"locked update unavailable", false, false, false},
		{"locked terminal recheck unavailable", false, false, true},
		{"provider repository unavailable", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSweepFixture(t)
			soon := time.Now().Add(time.Minute)
			f.seed(1, &soon, nil, !tc.terminal)
			f.seed(2, &soon, nil, true)
			failure := errors.New("session repository disconnected")
			if tc.listFails {
				f.expiry.list = func(context.Context, time.Time, storage.SessionExpiryCursor, int) ([]*storage.UserSession, error) {
					return nil, storage.NewStorageError("ListExpiringSessions", storage.ErrorKindConnection, failure, "session repository unavailable")
				}
			} else if tc.providerRepo {
				f.providers = thirdparty.NewThirdpartyOAuth2ProviderService(
					unavailableSweepProviderRepository{memory.NewInMemoryThirdpartyOAuth2ProviderRepository(), failure},
					f.seedEncryption, newNoopBranchKeyManager(), nil, false, slog.Default(),
				)
				f.resetService(oauth2session.DefaultConfig(), f.seedEncryption)
			} else {
				f.refresh.invoke = func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
					return nil, storage.NewStorageError("WithLockedSession", storage.ErrorKindConnection, failure, "session repository unavailable")
				}
			}
			result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
			require.Error(t, err)
			assert.ErrorIs(t, err, failure)
			assert.NotErrorIs(t, err, oauth2session.ErrInvalidSweepRequest)
			assert.Zero(t, result.TotalEvaluated, "a repository outage cannot count a session failure")
			if tc.listFails {
				assert.Zero(t, f.refresh.locks)
			} else {
				assert.Equal(t, 1, f.refresh.locks, "do not process the next candidate after storage fails")
			}
			dependency := oauth2session.DependencySessionRepository
			if tc.providerRepo {
				dependency = oauth2session.DependencyProviderRepository
			}
			assertSweepLogMetadata(t, f.logs.String(), "session.oauth2.sweep_aborted", "ERROR", oauth2session.DetailRepositoryUnavailable, oauth2session.KindInfrastructure, dependency)
		})
	}
}

func TestSessionSweepCommitFailureDoesNotRecordSuccess(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); _ = meterProvider.Shutdown(context.Background()) })

	f := newSweepFixture(t)
	f.provider(func(w http.ResponseWriter, _ *http.Request) { sweepSuccess(w) })
	soon := time.Now().Add(time.Minute)
	f.seed(1, &soon, nil, true)
	f.seed(2, &soon, nil, true)
	commitFailure := errors.New("failed to commit encrypted tokens")
	f.refresh.invoke = func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, callback func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
		return f.repo.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
			updated, err := callback(ctx, session)
			if updated && err == nil {
				return false, storage.NewStorageError("WithLockedSession", storage.ErrorKindConnection, commitFailure, "failed to commit encrypted tokens")
			}
			return updated, err
		})
	}
	_, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
	require.ErrorIs(t, err, commitFailure)
	assert.Equal(t, 1, f.refresh.locks, "a commit failure aborts before evaluating the next candidate")
	assert.NotContains(t, f.logs.String(), `"event":"session.oauth2.token_refreshed"`)
	assert.Empty(t, sweepMetrics(t, reader), "a failed commit must not increment a sweep counter")
	persisted, err := f.repo.Get(context.Background(), sweepSessionID(1))
	require.NoError(t, err)
	plain, err := f.service.DecryptAccessToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "old-access-1", plain)
}

type sweepFailEncrypt struct {
	ports.EncryptionPort
}

func (e sweepFailEncrypt) Encrypt(ctx context.Context, data []byte, aad map[string]string) ([]byte, error) {
	if string(data) == "new-access" {
		return nil, errors.New("encryption backend unavailable")
	}
	return e.EncryptionPort.Encrypt(ctx, data, aad)
}

func TestSessionSweepEncryptionFailureIsRecordedAndNextSessionRuns(t *testing.T) {
	f := newSweepFixture(t)
	f.resetService(oauth2session.DefaultConfig(), sweepFailEncrypt{f.seedEncryption})
	var upstreamCalls atomic.Int32
	f.provider(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		sweepSuccess(w)
	})
	soon := time.Now().Add(time.Minute)
	f.seed(1, &soon, nil, true)
	f.seed(2, &soon, nil, false)
	result, err := f.sweeper().Sweep(context.Background(), oauth2session.SweepRequest{})
	require.NoError(t, err)
	assertSweepCounts(t, result, 0, 0, 2, false)
	assert.Equal(t, 2, f.refresh.locks)
	assert.EqualValues(t, 1, upstreamCalls.Load(), "the second failed candidate has no refresh token")
	assertSweepLogMetadata(t, f.logs.String(), "session.oauth2.refresh_failed", "ERROR", oauth2session.DetailEncryptionFailed, oauth2session.KindInfrastructure, oauth2session.DependencyEncryption)
	original, err := f.repo.Get(context.Background(), sweepSessionID(1))
	require.NoError(t, err)
	plain, err := f.service.DecryptAccessToken(context.Background(), original)
	require.NoError(t, err)
	assert.Equal(t, "old-access-1", plain)
}

func TestSessionSweepCancellationWaitsForAdmittedRefreshAndStopsBeforeNext(t *testing.T) {
	f := newSweepFixture(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var upstreamCalls atomic.Int32
	f.provider(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
			sweepSuccess(w)
		case <-r.Context().Done():
			// A caller disconnect must not cancel a refresh after admission.
		}
	})
	soon := time.Now().Add(time.Minute)
	f.seed(1, &soon, nil, true)
	f.seed(2, &soon, nil, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		result oauth2session.SweepResult
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		result, err := f.sweeper().Sweep(ctx, oauth2session.SweepRequest{})
		finished <- outcome{result, err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first candidate never reached the upstream token endpoint")
	}
	cancel()
	select {
	case early := <-finished:
		t.Fatalf("sweep returned before its admitted refresh could commit: %+v", early)
	default:
	}
	once.Do(func() { close(release) })
	select {
	case got := <-finished:
		require.ErrorIs(t, got.err, context.Canceled)
		assertSweepCounts(t, got.result, 1, 0, 0, false)
	case <-time.After(5 * time.Second):
		t.Fatal("sweep did not stop after the admitted refresh completed")
	}
	assert.EqualValues(t, 1, upstreamCalls.Load())
	assert.Equal(t, 1, f.refresh.locks)
	persisted, err := f.repo.Get(context.Background(), sweepSessionID(1))
	require.NoError(t, err)
	plain, err := f.service.DecryptAccessToken(context.Background(), persisted)
	require.NoError(t, err)
	assert.Equal(t, "new-access", plain, "refresh-token rotation persists despite client disconnect")
}

func assertSweepLogMetadata(t *testing.T, logs, event, level string, detail oauth2session.ErrorDetail, kind oauth2session.ErrorKind, dependency oauth2session.Dependency) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var entry struct {
			Event        string `json:"event"`
			Level        string `json:"level"`
			PublicClient *bool  `json:"public_client"`
			Failure      struct {
				Detail     string `json:"failure_detail"`
				Kind       string `json:"error_kind"`
				Dependency string `json:"dependency"`
			} `json:"oauth2_session"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Event != event || entry.Level != level {
			continue
		}
		assert.Equal(t, string(detail), entry.Failure.Detail)
		assert.Equal(t, string(kind), entry.Failure.Kind)
		assert.Equal(t, string(dependency), entry.Failure.Dependency)
		if event == "session.oauth2.refresh_failed" {
			if kind == oauth2session.KindSession {
				assert.Nil(t, entry.PublicClient, "terminal candidates must not fetch provider credentials")
			} else {
				require.NotNil(t, entry.PublicClient, "known provider type must reach provider failure logs")
				assert.False(t, *entry.PublicClient)
			}
		}
		assert.NotContains(t, line, "user-1@example.com", "session logs never contain the user principal")
		assert.NotContains(t, line, "refresh-1", "session logs never contain refresh tokens")
		return
	}
	t.Errorf("missing %s log at %s level with classified metadata: %s", event, level, logs)
}

func sweepMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	counts := make(map[string]int64)
	for _, scope := range collected.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if !strings.HasPrefix(measurement.Name, "session_sweep_") {
				continue
			}
			sum, ok := measurement.Data.(metricdata.Sum[int64])
			require.True(t, ok, "sweep instruments must be Int64 counters")
			for _, point := range sum.DataPoints {
				assert.Len(t, point.Attributes.ToSlice(), 1, "only the bounded trigger label is permitted")
				for _, attr := range point.Attributes.ToSlice() {
					assert.Equal(t, "triggered_by", string(attr.Key))
					assert.Equal(t, "sweep", attr.Value.AsString())
				}
				counts[measurement.Name] += point.Value
			}
		}
	}
	return counts
}
