package approval

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type pollCall struct {
	sessions []string
	etag     string
	timeout  time.Duration
}

type pollerStub struct {
	mu    sync.Mutex
	calls []pollCall
	poll  func(context.Context, []string, string, time.Duration) ([]Pair, string, bool, error)
}

func (p *pollerStub) Poll(ctx context.Context, sessions []string, etag string, timeout time.Duration) ([]Pair, string, bool, error) {
	p.mu.Lock()
	p.calls = append(p.calls, pollCall{sessions: append([]string(nil), sessions...), etag: etag, timeout: timeout})
	p.mu.Unlock()
	return p.poll(ctx, sessions, etag, timeout)
}

func (p *pollerStub) recordedCalls() []pollCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]pollCall(nil), p.calls...)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSyncerBootstrapStoresFullSnapshotAndETag(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
	cache.Replace([]Pair{{
		Identity:  Identity{Principal: "stale", AgentID: "agent"},
		Approvals: []Record{approvedRecord("stale", "tool", map[string]string{}, "permanent", now)},
	}}, `"v1"`)
	poller := &pollerStub{poll: func(_ context.Context, sessions []string, etag string, timeout time.Duration) ([]Pair, string, bool, error) {
		assert.Empty(t, sessions)
		assert.Empty(t, etag)
		assert.Zero(t, timeout)
		return []Pair{{
			Identity:  Identity{Principal: "alice", AgentID: "agent"},
			Approvals: []Record{approvedRecord("approval", "tool", map[string]string{}, "permanent", now)},
		}}, `"v3"`, false, nil
	}}

	NewSyncer(cache, poller, time.Second, discardLogger()).Bootstrap(context.Background())
	_, matched := cache.Match(Identity{Principal: "alice", AgentID: "agent"}, "", "tool", map[string]any{})
	require.True(t, matched)
	_, matched = cache.Match(Identity{Principal: "stale", AgentID: "agent"}, "", "tool", map[string]any{})
	assert.False(t, matched, "a bootstrap snapshot must remove records absent from the broker response")
	assert.Equal(t, `"v3"`, cache.ETag())
	require.Len(t, poller.recordedCalls(), 1)
}

func TestSyncerBootstrapHonorsContextAndRetainsCacheOnFailure(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
	identity := Identity{Principal: "alice", AgentID: "agent"}
	cache.Replace([]Pair{{Identity: identity, Approvals: []Record{approvedRecord("cached", "tool", map[string]string{}, "permanent", now)}}}, `"v1"`)
	deadlineObserved := make(chan struct{})
	poller := &pollerStub{poll: func(ctx context.Context, _ []string, _ string, _ time.Duration) ([]Pair, string, bool, error) {
		_, hasDeadline := ctx.Deadline()
		if hasDeadline {
			close(deadlineObserved)
		}
		<-ctx.Done()
		return nil, "", false, ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	NewSyncer(cache, poller, time.Second, discardLogger()).Bootstrap(ctx)
	select {
	case <-deadlineObserved:
	case <-time.After(time.Second):
		t.Fatal("bootstrap did not pass the caller deadline to the poller")
	}
	_, matched := cache.Match(identity, "", "tool", map[string]any{})
	assert.True(t, matched, "failed bootstrap must retain the last known good snapshot")
	assert.Equal(t, `"v1"`, cache.ETag())
}

func TestSyncerRunRepollsAfter304AndReplacesSnapshot(t *testing.T) {
	now := time.Now().UTC()
	cache := NewCache(time.Minute, time.Minute)
	stale := Identity{Principal: "stale", AgentID: "agent"}
	fresh := Identity{Principal: "alice", AgentID: "agent"}
	cache.Replace([]Pair{{Identity: stale, Approvals: []Record{approvedRecord("stale", "tool", map[string]string{}, "permanent", now)}}}, `"v1"`)
	cache.RecordSession("active-session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	poller := &pollerStub{poll: func(_ context.Context, sessions []string, etag string, timeout time.Duration) ([]Pair, string, bool, error) {
		calls++
		require.Equal(t, []string{"active-session"}, sessions)
		if calls <= 2 {
			require.Equal(t, `"v1"`, etag)
		} else {
			require.Equal(t, `"v2"`, etag)
		}
		require.Equal(t, time.Second, timeout)
		switch calls {
		case 1:
			return nil, "", true, nil
		case 2:
			return []Pair{{Identity: fresh, Approvals: []Record{approvedRecord("fresh", "tool", map[string]string{}, "permanent", now)}}}, `"v2"`, false, nil
		default:
			cancel()
			return nil, "", false, errors.New("stop")
		}
	}}

	NewSyncer(cache, poller, time.Second, discardLogger()).Run(ctx)
	assert.Len(t, poller.recordedCalls(), 3)
	_, matched := cache.Match(stale, "", "tool", map[string]any{})
	assert.False(t, matched, "a later full snapshot must remove revoked records after a 304")
	record, matched := cache.Match(fresh, "", "tool", map[string]any{})
	require.True(t, matched)
	assert.Equal(t, "fresh", record.ID)
	assert.Equal(t, `"v2"`, cache.ETag())
}

func TestSyncerRetriesWithBoundedExponentialBackoff(t *testing.T) {
	cache := NewCache(time.Minute, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var starts []time.Time
	calls := 0
	poller := &pollerStub{poll: func(_ context.Context, _ []string, _ string, _ time.Duration) ([]Pair, string, bool, error) {
		starts = append(starts, time.Now())
		calls++
		if calls == 3 {
			cancel()
		}
		return nil, "", false, errors.New("broker unavailable")
	}}
	syncer := NewSyncer(cache, poller, time.Second, discardLogger())
	syncer.initialBackoff = 10 * time.Millisecond
	syncer.maximumBackoff = 20 * time.Millisecond

	syncer.Run(ctx)
	require.Len(t, starts, 3)
	assert.GreaterOrEqual(t, starts[1].Sub(starts[0]), 8*time.Millisecond)
	assert.GreaterOrEqual(t, starts[2].Sub(starts[1]), 18*time.Millisecond)
}

func TestSyncerCompletesBackoffWhenEvictionTickerFires(t *testing.T) {
	const (
		idleTTL           = 2 * time.Millisecond
		backoff           = 500 * time.Millisecond
		observationWindow = backoff / 2
	)

	cache := NewCache(idleTTL, time.Minute)
	cache.RecordSession("idle-session")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstAttempt := make(chan struct{})
	releaseFirstAttempt := make(chan struct{})
	firstFailureReturned := make(chan struct{}, 1)
	secondAttempt := make(chan struct{}, 1)
	calls := 0
	poller := &pollerStub{poll: func(_ context.Context, _ []string, _ string, _ time.Duration) ([]Pair, string, bool, error) {
		calls++
		if calls == 1 {
			close(firstAttempt)
			<-releaseFirstAttempt
			firstFailureReturned <- struct{}{}
		} else {
			select {
			case secondAttempt <- struct{}{}:
			default:
			}
		}
		return nil, "", false, errors.New("broker unavailable")
	}}
	syncer := NewSyncer(cache, poller, time.Second, discardLogger())
	syncer.initialBackoff = backoff
	syncer.maximumBackoff = backoff

	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()
	<-firstAttempt

	close(releaseFirstAttempt)
	<-firstFailureReturned
	require.Eventually(t, func() bool {
		return len(cache.ActiveSessions()) == 0
	}, observationWindow, time.Millisecond, "eviction ticker did not fire during retry backoff")

	select {
	case <-secondAttempt:
		t.Fatal("syncer retried before the retry backoff elapsed")
	case <-time.After(observationWindow):
	}
	assert.Len(t, poller.recordedCalls(), 1, "eviction must not end the retry backoff")

	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("syncer did not stop after cancellation")
	}
}

func TestSyncerStopsImmediatelyWhenCancelledDuringBackoff(t *testing.T) {
	cache := NewCache(time.Minute, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	firstAttempt := make(chan struct{})
	poller := &pollerStub{poll: func(_ context.Context, _ []string, _ string, _ time.Duration) ([]Pair, string, bool, error) {
		close(firstAttempt)
		return nil, "", false, errors.New("broker unavailable")
	}}
	syncer := NewSyncer(cache, poller, time.Second, discardLogger())
	syncer.initialBackoff = time.Second
	done := make(chan struct{})
	go func() {
		syncer.Run(ctx)
		close(done)
	}()
	<-firstAttempt
	cancel()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("syncer did not stop while waiting for retry backoff")
	}
	assert.Len(t, poller.recordedCalls(), 1)
}

func TestSyncerPublishesApprovalSyncAgeGauge(t *testing.T) {
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	previousMeterProvider := otel.GetMeterProvider()
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() {
		otel.SetMeterProvider(previousMeterProvider)
		_ = meterProvider.Shutdown(context.Background())
	})

	cache := NewCache(time.Minute, time.Minute)
	expectedAge := cache.SyncAgeSeconds()
	NewSyncer(cache, &pollerStub{}, time.Second, discardLogger())

	var resourceMetrics metricdata.ResourceMetrics
	require.NoError(t, metricReader.Collect(context.Background(), &resourceMetrics))
	var found bool
	for _, scopeMetrics := range resourceMetrics.ScopeMetrics {
		for _, metric := range scopeMetrics.Metrics {
			if metric.Name != "extproc.approval.sync_age" {
				continue
			}
			found = true
			assert.Equal(t, "s", metric.Unit)
			assert.Equal(t, "Seconds since the last successful approval sync", metric.Description)
			gauge, ok := metric.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "approval sync age must be an Int64 observable gauge")
			require.Len(t, gauge.DataPoints, 1)
			assert.Equal(t, expectedAge, gauge.DataPoints[0].Value)
		}
	}
	assert.True(t, found, "approval sync-age gauge must be exported with its exact name")
}
