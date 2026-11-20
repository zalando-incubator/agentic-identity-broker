package retention

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type retentionRepository func(context.Context, time.Time, int) (int64, error)

func (f retentionRepository) DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	return f(ctx, cutoff, limit)
}

func (f retentionRepository) StorageTime(context.Context) (time.Time, error) {
	return time.Now(), nil
}

type retentionStorageClock struct {
	retentionRepository
	now func() (time.Time, error)
}

func (r retentionStorageClock) StorageTime(context.Context) (time.Time, error) { return r.now() }

func TestRetentionFreezesStorageCutoff(t *testing.T) {
	const period = 24 * time.Hour
	synctest.Test(t, func(t *testing.T) {
		storageNow := time.Now().Add(-time.Hour)
		cutoff := storageNow.Add(-period)
		rows := []time.Time{cutoff.Add(-2 * time.Microsecond), cutoff.Add(-time.Microsecond), cutoff, cutoff.Add(time.Microsecond)}
		reads := 0
		repo := retentionStorageClock{
			now: func() (time.Time, error) {
				reads++
				return storageNow, nil
			},
			retentionRepository: func(_ context.Context, before time.Time, _ int) (int64, error) {
				assert.Equal(t, cutoff, before)
				storageNow = storageNow.Add(time.Hour)
				if len(rows) > 0 && rows[0].Before(before) {
					rows = rows[1:]
					return 1, nil
				}
				return 0, nil
			},
		}
		service, err := NewService(repo, ports.BusinessEventsRetentionConfig{Period: period, Interval: time.Second, BatchSize: 1}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
		service.sweep(t.Context())
		assert.Equal(t, 1, reads)
		assert.Equal(t, []time.Time{cutoff, cutoff.Add(time.Microsecond)}, rows)
	})
}

func TestRetentionSkipsDeleteWhenStorageClockFails(t *testing.T) {
	var logs bytes.Buffer
	repo := retentionStorageClock{
		now: func() (time.Time, error) { return time.Time{}, context.DeadlineExceeded },
		retentionRepository: func(context.Context, time.Time, int) (int64, error) {
			t.Fatal("retention must not delete without storage time")
			return 0, nil
		},
	}
	service, err := NewService(repo, ports.BusinessEventsRetentionConfig{Period: 24 * time.Hour, Interval: time.Second, BatchSize: 1}, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, err)
	service.sweep(t.Context())
	assert.Contains(t, logs.String(), `"event":"business_event.ledger.retention_failed"`)
	assert.Contains(t, logs.String(), `"error_kind":"timeout"`)
}

type retentionLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *retentionLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *retentionLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *retentionLogBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer.Reset()
}

func TestRetentionSchedulingAndRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		rows := []time.Time{start.Add(-48 * time.Hour), start.Add(-47 * time.Hour), start.Add(-24*time.Hour + time.Second), start}
		var logs retentionLogBuffer
		var mu sync.Mutex
		calls := 0
		fail, competingShort := false, false
		var cutoffs []time.Time
		repo := retentionRepository(func(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
			mu.Lock()
			defer mu.Unlock()
			calls++
			cutoffs = append(cutoffs, cutoff)
			if fail {
				return 0, errors.New("private storage detail")
			}
			if competingShort {
				competingShort = false
				return 0, nil
			}
			var deleted int64
			kept := rows[:0]
			for _, recorded := range rows {
				if recorded.Before(cutoff) && deleted < int64(limit) {
					deleted++
				} else {
					kept = append(kept, recorded)
				}
			}
			rows = kept
			return deleted, nil
		})
		cfg := ports.BusinessEventsRetentionConfig{Period: 24 * time.Hour, Interval: 10 * time.Second, BatchSize: 1}
		service, err := NewService(repo, cfg, slog.New(slog.NewJSONHandler(&logs, nil)))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go service.Run(ctx)
		synctest.Wait()
		mu.Lock()
		require.Equal(t, 3, calls, "startup must drain full batches then stop on a short batch")
		assert.Equal(t, []time.Time{start.Add(-24*time.Hour + time.Second), start}, rows)
		for _, cutoff := range cutoffs {
			assert.True(t, cutoff.Equal(start.Add(-24*time.Hour)))
		}
		assert.Empty(t, logs.String(), "successful deletion emits no log")
		mu.Unlock()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, []time.Time{start}, rows, "a row expiring just after the first cutoff must disappear at the next run")

		rows = append(rows, start.Add(-72*time.Hour), start.Add(-71*time.Hour), start.Add(-70*time.Hour))
		fail = true
		mu.Unlock()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		assert.Len(t, rows, 4)
		assert.Contains(t, logs.String(), `"event":"business_event.ledger.retention_failed"`)
		assert.Contains(t, logs.String(), `"error_kind":"unknown"`)
		assert.NotContains(t, logs.String(), "private storage detail")
		assert.NotContains(t, logs.String(), "principal")
		fail = false
		logs.Reset()
		mu.Unlock()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, []time.Time{start}, rows, "the next successful recovery run must drain the expired backlog")
		assert.Empty(t, logs.String())

		rows = append(rows, start.Add(-72*time.Hour))
		competingShort = true
		mu.Unlock()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		assert.Len(t, rows, 2, "a concurrent short delete may leave a backlog")
		mu.Unlock()
		time.Sleep(10 * time.Second)
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, []time.Time{start}, rows, "the next scheduled run resumes cleanup")
		mu.Unlock()
		cancel()
		synctest.Wait()
		stoppedCalls := calls
		rows = append(rows, start.Add(-72*time.Hour))
		time.Sleep(10 * time.Second)
		synctest.Wait()
		assert.Equal(t, stoppedCalls, calls)
		ctx, cancelAgain := context.WithCancel(t.Context())
		defer cancelAgain()
		go service.Run(ctx)
		synctest.Wait()
		mu.Lock()
		assert.Equal(t, []time.Time{start}, rows, "re-enable performs immediate catch-up")
		mu.Unlock()
	})
}

func TestRetentionCancellationWaitsForDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		calls := 0
		repo := retentionRepository(func(context.Context, time.Time, int) (int64, error) {
			calls++
			close(entered)
			<-release
			return 1, nil
		})
		service, err := NewService(repo, ports.BusinessEventsRetentionConfig{Period: 24 * time.Hour, Interval: time.Second, BatchSize: 1}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		go func() { service.Run(ctx); close(done) }()
		synctest.Wait()
		select {
		case <-entered:
		default:
			cancel()
			t.Fatal("startup did not delete expired rows")
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
			t.Fatal("Run returned before the active delete finished")
		default:
		}
		close(release)
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("Run did not finish after delete returned")
		}
		assert.Equal(t, 1, calls)
	})
}
