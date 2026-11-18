package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// The stale fallback belongs to the flight that failed: a later flight that starts once the
// singleflight key is released must not take the fallback away from the first flight's waiters.
func TestExchange_StaleFallbackIsCapturedWithTheFailedFlight(t *testing.T) {
	const subject, resource = "SUBJECT", "https://resource.example"
	entered, release := make(chan struct{}), make(chan struct{})
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
	}))
	t.Cleanup(broker.Close)
	te := &TokenExchanger{
		cfg: &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{TokenEndpoint: broker.URL, ExchangeTimeout: 5 * time.Second},
			Cache: extprocconfig.CacheConfig{DefaultTTL: time.Minute, MaxTTL: time.Hour}},
		client: broker.Client(), logger: slog.New(slog.DiscardHandler), cache: make(map[tokenCacheKey]*cachedToken),
	}
	te.assertion.Store(&assertionState{value: "ASSERTION"})
	key := tokenCacheKey{subjectToken: subject, resourceURI: resource}
	te.cache[key] = &cachedToken{accessToken: "STALE", expiresAt: time.Now().Add(-time.Second), staleUntil: time.Now().Add(time.Minute)}

	type outcome struct {
		result ExchangeResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := te.Exchange(context.Background(), subject, resource)
		done <- outcome{result, err}
	}()
	<-entered

	// Hold the cache so no fallback read can complete, then let the broker fail.
	te.cacheMu.Lock()
	close(release)
	var secondFlightRan atomic.Bool
	probe := func() (any, error) {
		secondFlightRan.Store(true)
		return ExchangeResult{}, nil
	}
	sfKey := subject + "\x00" + resource
	for deadline := time.Now().Add(300 * time.Millisecond); !secondFlightRan.Load() && time.Now().Before(deadline); {
		te.sfGroup.DoChan(sfKey, probe)
		time.Sleep(5 * time.Millisecond)
	}
	if secondFlightRan.Load() {
		// A second flight started before the first flight's waiter read its fallback; a
		// zero-lifetime result there removes the entry.
		delete(te.cache, key)
	}
	te.cacheMu.Unlock()

	got := <-done
	require.NoError(t, got.err, "the failed flight's waiter keeps the fallback it was eligible for")
	assert.Equal(t, "STALE", got.result.Token)
}
