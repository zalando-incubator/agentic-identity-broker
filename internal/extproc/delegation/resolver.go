package delegation

import (
	"context"
	"strconv"
	"sync"
	"time"

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

// Resolver caches permits only, each until its original Deadline. Denials, errors and
// unusable responses are never cached, and an expired permit is never served (no stale
// fallback). It is independent of the token-exchange cache (ADR 012 boundary).
type Resolver struct {
	evaluator Evaluator
	now       func() time.Time

	mu      sync.RWMutex
	entries map[cacheKey]Result
	group   singleflight.Group

	stop     chan struct{}
	stopOnce sync.Once
	done     chan struct{}
}

// NewResolver starts a resolver whose background sweep runs every max(maxStaleness/2, 1s).
func NewResolver(evaluator Evaluator, maxStaleness time.Duration) *Resolver {
	r := &Resolver{
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
func (r *Resolver) Resolve(ctx context.Context, subjectToken, resourceURI string) (result Result, hit bool, err error) {
	key := cacheKey{subjectToken: subjectToken, resourceURI: resourceURI}
	r.mu.RLock()
	cached, ok := r.entries[key]
	r.mu.RUnlock()
	if ok && r.now().Before(cached.Deadline) {
		return cached, true, nil
	}

	flightKey := strconv.Itoa(len(subjectToken)) + ":" + subjectToken + resourceURI
	value, err, _ := r.group.Do(flightKey, func() (any, error) {
		evaluated, evalErr := r.evaluator.Evaluate(context.WithoutCancel(ctx), subjectToken, resourceURI)
		if evalErr != nil {
			return Result{}, evalErr
		}
		if r.now().Before(evaluated.Deadline) {
			r.mu.Lock()
			r.entries[key] = evaluated
			r.mu.Unlock()
		}
		return evaluated, nil
	})
	if err != nil {
		return Result{}, false, err
	}
	return value.(Result), false, nil
}

// Shutdown stops the sweep goroutine. It is idempotent.
func (r *Resolver) Shutdown() {
	r.stopOnce.Do(func() { close(r.stop) })
	<-r.done
}

func (r *Resolver) sweepLoop(interval time.Duration) {
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

func (r *Resolver) sweep() {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, entry := range r.entries {
		if !now.Before(entry.Deadline) {
			delete(r.entries, key)
		}
	}
}
