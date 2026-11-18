package grantcontext

import (
	"context"
	"fmt"
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

func newTestResolver(t *testing.T, evaluator Evaluator, clock *fakeClock) *PermitResolver {
	t.Helper()
	r := NewPermitResolver(evaluator, time.Minute)
	// Join the sweep worker before replacing the clock so it never reads r.now concurrently;
	// tests drive sweep directly.
	r.Shutdown()
	r.now = clock.Now
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
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(evaluator.release) }) }
	t.Cleanup(func() {
		release()
		wg.Wait()
	})
	joined := make(chan struct{}, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, hit, err := r.Resolve(newWaitSignalContext(context.Background(), joined), "s", "https://a")
			if err == nil && !hit {
				results <- result
			}
		}()
	}
	for range callers {
		awaitSignal(t, joined, "every caller waits on the shared flight before the evaluation is released")
	}
	require.Eventually(t, func() bool { return evaluator.calls.Load() == 1 }, time.Second, time.Millisecond)
	clock.Advance(10 * time.Second)
	release()
	wg.Wait()
	close(results)
	count := 0
	for result := range results {
		count++
		assert.Equal(t, deadline, result.Deadline, "followers receive the original deadline")
	}
	assert.Equal(t, callers, count, "every caller receives the shared evaluation, not a later cache hit")
	assert.Equal(t, int32(1), evaluator.calls.Load())
}

// waitSignalContext reports the first Done call, which Resolve makes only once it waits on
// the shared flight after joining it.
type waitSignalContext struct {
	context.Context
	once    sync.Once
	waiting chan<- struct{}
}

func newWaitSignalContext(parent context.Context, waiting chan<- struct{}) *waitSignalContext {
	return &waitSignalContext{Context: parent, waiting: waiting}
}

func (c *waitSignalContext) Done() <-chan struct{} {
	c.once.Do(func() { c.waiting <- struct{}{} })
	return c.Context.Done()
}

func awaitSignal(t *testing.T, signal <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
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
	var evaluationCtxErr atomic.Value
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) { return permitUntil(clock.Now().Add(time.Minute), "alice"), nil }}
	r := newTestResolver(t, evaluatorFunc(func(ctx context.Context, subject, resource string) (Result, error) {
		evaluationCtxErr.Store(fmt.Sprint(ctx.Err()))
		return evaluator.Evaluate(ctx, subject, resource)
	}), clock)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := r.Resolve(ctx, "s", "https://a")

	require.ErrorIs(t, err, context.Canceled)
	require.Eventually(t, func() bool {
		_, hit, err := r.Resolve(context.Background(), "s", "https://a")
		return err == nil && hit
	}, time.Second, time.Millisecond, "the shared evaluation completes and caches the permit")
	assert.Equal(t, "<nil>", evaluationCtxErr.Load())
	assert.Equal(t, int32(1), evaluator.calls.Load())
}

type evaluatorFunc func(ctx context.Context, subject, resource string) (Result, error)

func (f evaluatorFunc) Evaluate(ctx context.Context, subject, resource string) (Result, error) {
	return f(ctx, subject, resource)
}

func TestResolver_CallerStopsWaitingWhileOthersReceiveSharedResult(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	deadline := clock.Now().Add(time.Minute)
	evaluator := &scriptedEvaluator{release: make(chan struct{}), respond: func(string, string) (Result, error) {
		return permitUntil(deadline, "alice"), nil
	}}
	r := newTestResolver(t, evaluator, clock)
	defer func() {
		select {
		case <-evaluator.release:
		default:
			close(evaluator.release)
		}
	}()

	waiting := make(chan error, 1)
	go func() {
		_, _, err := r.Resolve(context.Background(), "s", "https://a")
		waiting <- err
	}()
	require.Eventually(t, func() bool { return evaluator.calls.Load() == 1 }, time.Second, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	followerWaiting := make(chan struct{}, 1)
	canceled := make(chan error, 1)
	go func() {
		_, _, err := r.Resolve(newWaitSignalContext(ctx, followerWaiting), "s", "https://a")
		canceled <- err
	}()
	awaitSignal(t, followerWaiting, "the follower must wait on the active flight")
	cancel()
	select {
	case err := <-canceled:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("a canceled caller must stop waiting for the shared evaluation")
	}

	close(evaluator.release)
	require.NoError(t, <-waiting, "an active caller still receives the shared result")
	assert.Equal(t, int32(1), evaluator.calls.Load())
}

func TestResolver_RechecksCacheBeforeEvaluating(t *testing.T) {
	t.Parallel()
	clock := &fakeClock{now: time.Unix(1_000, 0)}
	evaluator := &scriptedEvaluator{respond: func(string, string) (Result, error) {
		return Result{}, newError(CategoryUnavailable, nil)
	}}
	r := newTestResolver(t, evaluator, clock)
	key := cacheKey{subjectToken: "s", resourceURI: "https://a"}
	fresh := permitUntil(clock.Now().Add(time.Minute), "alice")
	r.entries[key] = permitUntil(clock.Now(), "alice")
	var refilled atomic.Bool
	// The first clock read happens after the stale entry was observed: another caller's
	// evaluation stores a fresh permit before this caller starts its own evaluation.
	r.now = func() time.Time {
		if refilled.CompareAndSwap(false, true) {
			r.mu.Lock()
			r.entries[key] = fresh
			r.mu.Unlock()
		}
		return clock.Now()
	}

	result, hit, err := r.Resolve(context.Background(), "s", "https://a")

	require.NoError(t, err, "a live permit stored by another caller is reused instead of a failing evaluation")
	assert.True(t, hit)
	assert.Equal(t, fresh, result)
	assert.Zero(t, evaluator.calls.Load())
}
