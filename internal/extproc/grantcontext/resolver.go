package grantcontext

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
)

// Evaluator performs one broker evaluation.
type Evaluator interface {
	Evaluate(ctx context.Context, subjectToken, resourceURI string) (Result, error)
}

// cacheKey is a struct so subject and resource values cannot collide through separators.
type cacheKey struct {
	subjectToken string
	resourceURI  string
}

// PermitResolver caches permits only, each until its original Deadline. Denials, errors and
// unusable responses are never cached, and an expired permit is never served (no stale
// fallback). It is independent of the token-exchange cache (ADR 012 boundary).
type PermitResolver struct {
	evaluator Evaluator
	now       func() time.Time

	mu      sync.RWMutex
	entries map[cacheKey]Result
	group   singleflight.Group

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// NewPermitResolver starts a resolver whose background sweep runs every max(maxStaleness/2, 1s).
func NewPermitResolver(evaluator Evaluator, maxStaleness time.Duration) *PermitResolver {
	r := &PermitResolver{
		evaluator: evaluator,
		now:       time.Now,
		entries:   make(map[cacheKey]Result),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go r.sweepLoop(max(maxStaleness/2, time.Second))
	return r
}

// Resolve returns a cached permit while now < Deadline, or evaluates once per key across
// concurrent callers. The returned Result keeps its original Deadline; the caller must
// check it again before every policy use. hit reports a cache hit.
func (r *PermitResolver) Resolve(ctx context.Context, subjectToken, resourceURI string) (result Result, hit bool, err error) {
	ctx, span := otel.Tracer("extproc").Start(ctx, "extproc.grant_context.resolve", trace.WithSpanKind(trace.SpanKindInternal))
	defer func() {
		cache := "miss"
		if hit {
			cache = "hit"
		}
		span.SetAttributes(attribute.String("grant_context.cache", cache))
		observeResolution(span, err)
		span.End()
	}()
	key := cacheKey{subjectToken: subjectToken, resourceURI: resourceURI}
	if cached, ok := r.live(key); ok {
		return cached, true, nil
	}

	type resolved struct {
		result Result
		hit    bool
	}
	flightKey := strconv.Itoa(len(subjectToken)) + ":" + subjectToken + resourceURI
	// The evaluation is detached from any one caller; each caller stops waiting on its own
	// cancellation while the others still receive the shared result.
	flight := r.group.DoChan(flightKey, func() (any, error) {
		if cached, ok := r.live(key); ok {
			return resolved{result: cached, hit: true}, nil
		}
		evaluated, evalErr := r.evaluator.Evaluate(context.WithoutCancel(ctx), subjectToken, resourceURI)
		if evalErr != nil {
			return resolved{}, evalErr
		}
		if r.now().Before(evaluated.Deadline) {
			r.mu.Lock()
			r.entries[key] = evaluated
			r.mu.Unlock()
		}
		return resolved{result: evaluated}, nil
	})
	select {
	case <-ctx.Done():
		return Result{}, false, transportError(ctx.Err(), CategoryUnavailable)
	case res := <-flight:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{}, false, transportError(ctxErr, CategoryUnavailable)
		}
		if res.Err != nil {
			return Result{}, false, res.Err
		}
		value := res.Val.(resolved)
		return value.result, value.hit, nil
	}
}

// live returns the cached permit for key while now < Deadline.
func (r *PermitResolver) live(key cacheKey) (Result, bool) {
	r.mu.RLock()
	cached, ok := r.entries[key]
	r.mu.RUnlock()
	if ok && r.now().Before(cached.Deadline) {
		return cached, true
	}
	return Result{}, false
}

// Shutdown stops the sweep goroutine. It is idempotent.
func (r *PermitResolver) Shutdown() {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
}

func (r *PermitResolver) sweepLoop(interval time.Duration) {
	defer close(r.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.sweep()
		}
	}
}

func (r *PermitResolver) sweep() {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, entry := range r.entries {
		if !now.Before(entry.Deadline) {
			delete(r.entries, key)
		}
	}
}
