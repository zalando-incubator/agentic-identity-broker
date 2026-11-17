package delegation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type scriptedEvaluator struct {
	calls   atomic.Int32
	release chan struct{}
	respond func(subject, resource string) (Result, error)
}

func (e *scriptedEvaluator) Evaluate(_ context.Context, subject, resource string) (Result, error) {
	e.calls.Add(1)
	if e.release != nil {
		<-e.release
	}
	return e.respond(subject, resource)
}

func newTestResolver(t *testing.T, evaluator Evaluator, clock *fakeClock) *Resolver {
	t.Helper()
	r := NewResolver(evaluator, time.Minute)
	r.now = clock.Now
	t.Cleanup(r.Shutdown)
	return r
}

func permitUntil(deadline time.Time, principal string) Result {
	return Result{Context: Context{Principal: principal}, Deadline: deadline}
}

func TestResolver_CachesPermitUntilOriginalDeadline(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	deadline := clock.Now().Add(10 * time.Second)
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) { return permitUntil(deadline, "alice"), nil }}
	r := newTestResolver(t, evaluator, clock)

	first, hit, err := r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err)
	assert.False(t, hit)
	clock.Advance(9 * time.Second)
	second, hit, err := r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, first.Deadline, second.Deadline, "a hit returns the stored deadline unchanged")
	assert.Equal(t, int32(1), evaluator.calls.Load())

	clock.Advance(time.Second)
	_, hit, err = r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err)
	assert.False(t, hit, "no hit at the deadline")
	assert.Equal(t, int32(2), evaluator.calls.Load())
}

func TestResolver_NeverCachesDenialsOrErrorsAndHasNoStaleFallback(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	var outcome atomic.Int32
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) {
		switch outcome.Load() {
		case 0:
			return Result{}, &DeniedError{Code: "grant_missing", ConsentURL: "https://b/agents/x"}
		case 1:
			return permitUntil(clock.Now().Add(5*time.Second), "alice"), nil
		default:
			return Result{}, &Error{Category: CategoryUnavailable}
		}
	}}
	r := newTestResolver(t, evaluator, clock)

	_, _, err := r.Resolve(context.Background(), "s", "https://a")
	var denied *DeniedError
	require.ErrorAs(t, err, &denied)

	outcome.Store(1)
	result, hit, err := r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err, "consent is visible immediately because denials are not cached")
	assert.False(t, hit)
	assert.Equal(t, "alice", result.Context.Principal)

	outcome.Store(2)
	clock.Advance(5 * time.Second)
	_, _, err = r.Resolve(context.Background(), "s", "https://a")
	var resolveErr *Error
	require.ErrorAs(t, err, &resolveErr, "an expired permit is never served when the broker fails")
	_, _, err = r.Resolve(context.Background(), "s", "https://a")
	require.Error(t, err, "errors are not cached either")
	assert.Equal(t, int32(4), evaluator.calls.Load())
}

func TestResolver_DelayedResponsePastDeadlineIsReturnedButNotCached(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	deadline := clock.Now().Add(-time.Second)
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) { return permitUntil(deadline, "alice"), nil }}
	r := newTestResolver(t, evaluator, clock)

	result, _, err := r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err)
	assert.Equal(t, deadline, result.Deadline, "the original deadline is kept; the caller rejects it before policy")
	_, _, _ = r.Resolve(context.Background(), "s", "https://a")
	assert.Equal(t, int32(2), evaluator.calls.Load())
}

func TestResolver_IsolatesKeysWithoutSeparatorCollisions(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	evaluator := &scriptedEvaluator{respond: func(subject, resource string) (Result, error) {
		return permitUntil(clock.Now().Add(time.Minute), subject+"|"+resource), nil
	}}
	r := newTestResolver(t, evaluator, clock)

	a, _, err := r.Resolve(context.Background(), "ab", "c")
	require.NoError(t, err)
	b, _, err := r.Resolve(context.Background(), "a", "bc")
	require.NoError(t, err)
	assert.Equal(t, "ab|c", a.Context.Principal)
	assert.Equal(t, "a|bc", b.Context.Principal)
	assert.Equal(t, int32(2), evaluator.calls.Load())
}

func TestResolver_SingleflightSharesOneEvaluationAndDeadline(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	deadline := clock.Now().Add(30 * time.Second)
	evaluator := &scriptedEvaluator{release: make(chan struct{}), respond: func(string, string) (Result, error) {
		return permitUntil(deadline, "alice"), nil
	}}
	r := newTestResolver(t, evaluator, clock)

	const callers = 8
	results := make(chan Result, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := r.Resolve(context.Background(), "s", "https://a")
			if err == nil {
				results <- result
			}
		}()
	}
	require.Eventually(t, func() bool { return evaluator.calls.Load() == 1 }, time.Second, time.Millisecond)
	clock.Advance(10 * time.Second)
	close(evaluator.release)
	wg.Wait()
	close(results)
	count := 0
	for result := range results {
		count++
		assert.Equal(t, deadline, result.Deadline, "followers receive the original deadline")
	}
	assert.Equal(t, callers, count)
	assert.Equal(t, int32(1), evaluator.calls.Load())
}

func TestResolver_SweepEvictsExpiredEntriesAndShutdownIsIdempotent(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) {
		return permitUntil(clock.Now().Add(time.Second), "alice"), nil
	}}
	r := newTestResolver(t, evaluator, clock)
	_, _, err := r.Resolve(context.Background(), "s", "https://a")
	require.NoError(t, err)

	clock.Advance(time.Second)
	r.sweep()
	r.mu.RLock()
	assert.Empty(t, r.entries)
	r.mu.RUnlock()

	r.Shutdown()
	r.Shutdown()
}

func TestResolver_DoesNotForwardCallerCancellationToSharedEvaluation(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) { return permitUntil(clock.Now().Add(time.Minute), "alice"), nil }}
	r := newTestResolver(t, evaluator, clock)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := r.Resolve(ctx, "s", "https://a")
	require.NoError(t, err)
}
