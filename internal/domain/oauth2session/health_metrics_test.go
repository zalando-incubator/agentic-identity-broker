package oauth2session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type healthRepoCall struct {
	threshold, now time.Time
	deadline       time.Time
	hasDeadline    bool
}

type fakeHealthRepo struct {
	mu    sync.Mutex
	calls []healthRepoCall
	count func(ctx context.Context) ([]storage.SessionExpiryGroup, error)
}

func (f *fakeHealthRepo) CountSessionsByExpiry(ctx context.Context, threshold, now time.Time) ([]storage.SessionExpiryGroup, error) {
	deadline, ok := ctx.Deadline()
	f.mu.Lock()
	f.calls = append(f.calls, healthRepoCall{threshold: threshold, now: now, deadline: deadline, hasDeadline: ok})
	f.mu.Unlock()
	return f.count(ctx)
}

func (f *fakeHealthRepo) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func installHealthReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	previous := otel.GetMeterProvider()
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(previous); _ = provider.Shutdown(context.Background()) })
	return reader
}

type healthPoint struct {
	service, state string
	value          int64
	keys           []string
}

func collectHealth(t *testing.T, reader *sdkmetric.ManualReader) ([]healthPoint, bool) {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	for _, scope := range collected.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Name != "oauth2_sessions" {
				continue
			}
			assert.Equal(t, "{session}", measurement.Unit)
			assert.NotEmpty(t, measurement.Description)
			gauge, ok := measurement.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "oauth2_sessions must be an Int64 gauge, got %T", measurement.Data)
			points := make([]healthPoint, 0, len(gauge.DataPoints))
			for _, dp := range gauge.DataPoints {
				service, _ := dp.Attributes.Value("service_id")
				state, _ := dp.Attributes.Value("state")
				var keys []string
				for _, kv := range dp.Attributes.ToSlice() {
					keys = append(keys, string(kv.Key))
				}
				points = append(points, healthPoint{service: service.AsString(), state: state.AsString(), value: dp.Value, keys: keys})
			}
			return points, true
		}
	}
	return nil, false
}

func healthLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

func healthWarnings(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == "session health query failed" {
			records = append(records, record)
		}
	}
	return records
}

func TestClassifySessionHealth(t *testing.T) {
	tests := []struct {
		name          string
		accessExpiry  storage.AccessExpiryClass
		refreshUsable bool
		want          oauth2session.SessionHealthState
	}{
		{"unknown expiry with usable refresh", storage.AccessExpiryUnknown, true, oauth2session.SessionHealthUnknownExpiry},
		{"unknown expiry without refresh", storage.AccessExpiryUnknown, false, oauth2session.SessionHealthUnknownExpiry},
		{"after threshold with usable refresh", storage.AccessExpiryAfterThreshold, true, oauth2session.SessionHealthValid},
		{"after threshold without refresh", storage.AccessExpiryAfterThreshold, false, oauth2session.SessionHealthValid},
		{"due with usable refresh", storage.AccessExpiryDue, true, oauth2session.SessionHealthExpiringRefreshable},
		{"due without refresh", storage.AccessExpiryDue, false, oauth2session.SessionHealthExpiredUnrefreshable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := oauth2session.ClassifySessionHealth(storage.SessionExpiryGroup{
				ServiceID: id.NewServiceID(), AccessExpiry: tt.accessExpiry, RefreshUsable: tt.refreshUsable, Count: 1,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	t.Run("invalid class", func(t *testing.T) {
		_, err := oauth2session.ClassifySessionHealth(storage.SessionExpiryGroup{AccessExpiry: "later", Count: 1})
		assert.Error(t, err)
	})
}

func TestSessionHealthMetricsObservesZeroFilledStatesPerService(t *testing.T) {
	reader := installHealthReader(t)
	calendar, tickets := id.NewServiceID(), id.NewServiceID()
	repo := &fakeHealthRepo{count: func(context.Context) ([]storage.SessionExpiryGroup, error) {
		return []storage.SessionExpiryGroup{
			{ServiceID: calendar, AccessExpiry: storage.AccessExpiryDue, RefreshUsable: true, Count: 2},
			{ServiceID: calendar, AccessExpiry: storage.AccessExpiryAfterThreshold, RefreshUsable: true, Count: 1},
			{ServiceID: calendar, AccessExpiry: storage.AccessExpiryAfterThreshold, RefreshUsable: false, Count: 3},
			{ServiceID: tickets, AccessExpiry: storage.AccessExpiryUnknown, RefreshUsable: true, Count: 1},
		}, nil
	}}
	logger, _ := healthLogger()
	metrics, err := oauth2session.NewSessionHealthMetrics(repo, 5*time.Minute, time.Second, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = metrics.Close() })

	before := time.Now()
	points, ok := collectHealth(t, reader)
	after := time.Now()
	require.True(t, ok, "oauth2_sessions must be exported")
	got := map[string]int64{}
	for _, p := range points {
		assert.ElementsMatch(t, []string{"service_id", "state"}, p.keys)
		got[p.service+"/"+p.state] = p.value
	}
	assert.Equal(t, map[string]int64{
		calendar.String() + "/valid": 4, calendar.String() + "/expiring_refreshable": 2,
		calendar.String() + "/expired_unrefreshable": 0, calendar.String() + "/unknown_expiry": 0,
		tickets.String() + "/valid": 0, tickets.String() + "/expiring_refreshable": 0,
		tickets.String() + "/expired_unrefreshable": 0, tickets.String() + "/unknown_expiry": 1,
	}, got)

	require.Equal(t, 1, repo.callCount())
	call := repo.calls[0]
	assert.Equal(t, 5*time.Minute, call.threshold.Sub(call.now))
	assert.False(t, call.now.Before(before))
	assert.False(t, call.now.After(after))
	require.True(t, call.hasDeadline, "the query must run under the configured timeout")
	assert.LessOrEqual(t, time.Until(call.deadline), time.Second)
}

func TestSessionHealthMetricsReportsNoPointsWhenQueryFails(t *testing.T) {
	tests := []struct {
		name     string
		count    func(ctx context.Context) ([]storage.SessionExpiryGroup, error)
		wantKind string
	}{
		{"storage error", func(context.Context) ([]storage.SessionExpiryGroup, error) {
			return nil, storage.NewStorageError("CountSessionsByExpiry", storage.ErrorKindTimeout, context.DeadlineExceeded, "timeout")
		}, "timeout"},
		{"timeout while blocked", func(ctx context.Context) ([]storage.SessionExpiryGroup, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, "unknown"},
		{"invalid group", func(context.Context) ([]storage.SessionExpiryGroup, error) {
			return []storage.SessionExpiryGroup{{ServiceID: id.NewServiceID(), AccessExpiry: storage.AccessExpiryDue, Count: 0}}, nil
		}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := installHealthReader(t)
			logger, logs := healthLogger()
			metrics, err := oauth2session.NewSessionHealthMetrics(&fakeHealthRepo{count: tt.count}, 5*time.Minute, 20*time.Millisecond, logger)
			require.NoError(t, err)
			t.Cleanup(func() { _ = metrics.Close() })

			started := time.Now()
			points, _ := collectHealth(t, reader)
			assert.Less(t, time.Since(started), time.Second)
			assert.Empty(t, points)
			warnings := healthWarnings(t, logs)
			require.Len(t, warnings, 1)
			assert.Equal(t, "WARN", warnings[0]["level"])
			assert.Equal(t, tt.wantKind, warnings[0]["error_kind"])
		})
	}
}

func TestSessionHealthMetricsCloseStopsQueries(t *testing.T) {
	reader := installHealthReader(t)
	repo := &fakeHealthRepo{count: func(context.Context) ([]storage.SessionExpiryGroup, error) { return nil, nil }}
	logger, _ := healthLogger()
	metrics, err := oauth2session.NewSessionHealthMetrics(repo, 5*time.Minute, time.Second, logger)
	require.NoError(t, err)
	collectHealth(t, reader)
	require.Equal(t, 1, repo.callCount())

	require.NoError(t, metrics.Close())
	require.NoError(t, metrics.Close())
	collectHealth(t, reader)
	assert.Equal(t, 1, repo.callCount())
}
