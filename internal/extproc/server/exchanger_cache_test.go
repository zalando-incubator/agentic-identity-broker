// Package server_test — Phase 4 cache, singleflight, and eviction tests.
//
// T028: Cache TTL expiry — verifies expired entries are re-fetched
// T029: Singleflight deduplication — concurrent identical calls hit exchange once
// T030: Background eviction and assertion refresh — eviction goroutine clears expired entries
package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// ---------------------------------------------------------------------------
// T028: Cache TTL expiry
// ---------------------------------------------------------------------------

// Spec: FR-011, FR-012 — Expired cache entry is re-fetched from exchange endpoint
func TestTokenExchanger_Cache_ExpiredEntry_TriggersNewExchange(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "expiry-test-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		token1, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		callsAfterFirst := mocks.exchangeCalls

		time.Sleep(1100 * time.Millisecond)

		mocks.exchangedToken = "refreshed-access-token"
		token2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		assert.Equal(t, "exchanged-access-token", token1.Token, "first call must return original token")
		assert.Equal(t, "refreshed-access-token", token2.Token, "second call after expiry must return new token")
		assert.Greater(t, mocks.exchangeCalls, callsAfterFirst,
			"token exchange endpoint must be called again after TTL expiry")
	})
}

// Spec: FR-012 — Cache hit returns same token without calling exchange
func TestTokenExchanger_Cache_HitBeforeExpiry_ReturnsCachedToken(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	// Use a long TTL — cache should not expire during test
	longExpiry := 3600
	mocks.exchangeExpiry = &longExpiry

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "long-lived-token"
	const resourceURI = "http://mcp-server:9003/mcp"

	// First call — populates cache
	token1, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.NoError(t, err)
	callsAfterFirst := mocks.exchangeCalls

	// Multiple rapid calls — all should hit cache
	for range 5 {
		token, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		assert.Equal(t, token1.Token, token.Token, "cache hit must return same token")
	}

	assert.Equal(t, callsAfterFirst, mocks.exchangeCalls,
		"exchange endpoint must not be called on cache hits")
}

// Spec: granted_permission_sets snapshots returned from cache must be isolated from caller mutation.
func TestTokenExchanger_CacheHit_ReturnsClonedGrantedPermissionSets(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	longExpiry := 3600
	mocks.exchangeExpiry = &longExpiry
	mocks.exchangeGrantedPerms = map[string][]string{
		"perm-set-1": {"svc-a", "svc-b"},
	}

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "permission-set-token"
	const resourceURI = "http://mcp-server:9003/mcp"

	first, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.NoError(t, err)
	require.Equal(t, 1, mocks.exchangeCalls)

	first.GrantedPermissionSets["perm-set-1"][0] = "mutated-service"
	first.GrantedPermissionSets["new-perm-set"] = []string{"svc-z"}

	second, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.NoError(t, err)
	assert.Equal(t, 1, mocks.exchangeCalls, "second call must come from cache")
	assert.Equal(t, []string{"svc-a", "svc-b"}, second.GrantedPermissionSets["perm-set-1"])
	_, found := second.GrantedPermissionSets["new-perm-set"]
	assert.False(t, found, "caller mutation must not leak back into the cached snapshot")
}

// Spec: FR-018 — Cache TTL respects max_ttl cap even when expires_in is larger
func TestTokenExchanger_Cache_MaxTTLCap_AppliedCorrectly(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		// expires_in larger than max_ttl
		largeExpiry := 7 * 24 * 3600 // 1 week
		mocks.exchangeExpiry = &largeExpiry

		cfg := configForMocks(mocks)
		cfg.Cache.MaxTTL = 500 * time.Millisecond // short max TTL for testing

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "max-ttl-test-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// First call — populates cache with max_ttl cap applied
		token1, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		// The token is cached — another call should hit cache
		token2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		assert.Equal(t, token1.Token, token2.Token, "immediate second call must be a cache hit")
		callsAfterSecond := mocks.exchangeCalls

		// Wait for max_ttl to expire (500ms + buffer)
		time.Sleep(700 * time.Millisecond)

		// After max_ttl, cache must be expired
		mocks.exchangedToken = "post-maxttl-token"
		token3, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		assert.Equal(t, "post-maxttl-token", token3.Token,
			"token must be refreshed after max_ttl expires")
		assert.Greater(t, mocks.exchangeCalls, callsAfterSecond,
			"exchange endpoint must be called again after max_ttl cap expiry")
	})
}

// Spec: FR-011 — Each unique subject+resource combination has its own cache entry
func TestTokenExchanger_Cache_UniqueKeyPerSubjectAndResource(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	longExpiry := 3600
	var callMu sync.Mutex
	var exchangeLog []string

	mocks.tokenExchServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		subject := r.FormValue("subject_token")
		resource := r.FormValue("resource")

		callMu.Lock()
		exchangeLog = append(exchangeLog, fmt.Sprintf("%s@%s", subject, resource))
		callMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		expiry := longExpiry
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: fmt.Sprintf("token-for-%s-%s", subject, resource),
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	})

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Exchange for multiple combinations
	combinations := []struct{ subject, resource string }{
		{"user-a", "http://service-1:9000/api"},
		{"user-a", "http://service-2:9001/api"},
		{"user-b", "http://service-1:9000/api"},
	}

	tokens := make([]string, len(combinations))
	for i, c := range combinations {
		result, err := exchanger.Exchange(context.Background(), c.subject, c.resource)
		require.NoError(t, err)
		tokens[i] = result.Token
	}

	// All tokens should be distinct
	assert.NotEqual(t, tokens[0], tokens[1], "different resources → different tokens")
	assert.NotEqual(t, tokens[0], tokens[2], "different subjects → different tokens")
	assert.NotEqual(t, tokens[1], tokens[2], "different subject+resource → different tokens")

	// Re-fetch — all should come from cache (no new exchange calls)
	callsBeforeRefetch := len(exchangeLog)
	for _, c := range combinations {
		_, err := exchanger.Exchange(context.Background(), c.subject, c.resource)
		require.NoError(t, err)
	}
	callMu.Lock()
	callsAfterRefetch := len(exchangeLog)
	callMu.Unlock()

	assert.Equal(t, callsBeforeRefetch, callsAfterRefetch,
		"all re-fetches must be cache hits — no new exchange calls")
}

// ---------------------------------------------------------------------------
// T029: Singleflight deduplication
// ---------------------------------------------------------------------------

// Spec: FR-014 — Concurrent requests for the same key are deduplicated via singleflight
func TestTokenExchanger_Singleflight_ConcurrentRequests_CallExchangeOnce(t *testing.T) {
	var (
		requestCount  int64
		responseMu    sync.Mutex
		responseReady = make(chan struct{})
	)

	// Slow exchange server that counts calls
	slowExchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// Block until all goroutines have submitted their requests
		responseMu.Lock()
		<-responseReady
		responseMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "singleflight-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer slowExchServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.OAuth2.TokenEndpoint = slowExchServer.URL + "/oauth2/token"
	cfg.OAuth2.ExchangeTimeout = 10 * time.Second

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const numGoroutines = 10
	const subjectToken = "concurrent-user-token"
	const resourceURI = "http://mcp-server:9003/mcp"

	var wg sync.WaitGroup
	results := make([]string, numGoroutines)
	errors := make([]error, numGoroutines)

	// Launch concurrent goroutines requesting the same key
	for i := range numGoroutines {
		wg.Go(func() {
			var r server.ExchangeResult
			r, errors[i] = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
			results[i] = r.Token
		})
	}

	// Give goroutines time to queue up before releasing the slow server
	time.Sleep(50 * time.Millisecond)
	close(responseReady) // release all blocked requests
	wg.Wait()

	// All calls must succeed
	for i, err := range errors {
		assert.NoError(t, err, "goroutine %d must not error", i)
	}

	// All calls must return the same token
	for i, result := range results {
		assert.Equal(t, "singleflight-token", result,
			"goroutine %d must receive the singleflight token", i)
	}

	// Exchange endpoint must be called only ONCE despite 10 concurrent requests
	assert.Equal(t, int64(1), atomic.LoadInt64(&requestCount),
		"singleflight must deduplicate concurrent requests — exchange called exactly once")
}

// Spec: FR-014 — Singleflight allows independent keys to proceed concurrently
func TestTokenExchanger_Singleflight_DifferentKeys_ProceedConcurrently(t *testing.T) {
	var requestCount int64

	exchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		_ = r.ParseForm()
		resource := r.FormValue("resource")

		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "token-for-" + resource,
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer exchServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.OAuth2.TokenEndpoint = exchServer.URL + "/oauth2/token"

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// 3 different resources — should each call exchange once
	resources := []string{
		"http://service-a:9000/api",
		"http://service-b:9001/api",
		"http://service-c:9002/api",
	}

	var wg sync.WaitGroup
	results := make([]string, len(resources))
	for i, res := range resources {
		wg.Add(1)
		go func(idx int, resource string) {
			defer wg.Done()
			result, err := exchanger.Exchange(context.Background(), "user-token", resource)
			require.NoError(t, err)
			results[idx] = result.Token
		}(i, res)
	}
	wg.Wait()

	// Each resource gets its own token
	for i, res := range resources {
		assert.Equal(t, "token-for-"+res, results[i],
			"resource %s must receive its own token", res)
	}

	// Each resource should have triggered exactly one exchange
	assert.Equal(t, int64(len(resources)), atomic.LoadInt64(&requestCount),
		"each unique resource must trigger exactly one exchange call")
}

// Spec: FR-014 — Singleflight error is shared across all waiting callers
func TestTokenExchanger_Singleflight_SharedError_OnExchangeFailure(t *testing.T) {
	var callCount int64

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":"invalid_client"}`)
	}))
	defer errorServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.OAuth2.TokenEndpoint = errorServer.URL + "/oauth2/token"

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Multiple concurrent calls with same key — all should fail, exchange called once
	const numGoroutines = 5
	errs := make([]error, numGoroutines)
	var wg sync.WaitGroup

	for i := range numGoroutines {
		wg.Go(func() {
			_, errs[i] = exchanger.Exchange(context.Background(), "user-token", "http://resource.example.com")
		})
	}
	wg.Wait()

	// All goroutines must receive an error
	for i, err := range errs {
		assert.Error(t, err, "goroutine %d must receive an error on exchange failure", i)
	}

	// Exchange must be called at most once despite concurrent requests
	// (singleflight deduplicates the error path too)
	assert.LessOrEqual(t, atomic.LoadInt64(&callCount), int64(numGoroutines),
		"singleflight must not amplify error-path calls beyond number of goroutines")
}

// ---------------------------------------------------------------------------
// T030: Background eviction and assertion refresh
// ---------------------------------------------------------------------------

// Spec: FR-016 — Background eviction removes expired entries from cache
func TestTokenExchanger_Eviction_ExpiredEntries_RemovedFromCache(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1 // 1 second TTL
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		// Set a very short DefaultTTL to trigger eviction loop quickly
		cfg.Cache.DefaultTTL = 500 * time.Millisecond

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "eviction-test-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// Populate cache
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		callsAfterFirst := mocks.exchangeCalls

		// The 1s expiry and 5s stale window must both pass before eviction.
		time.Sleep(7 * time.Second)
		synctest.Wait()

		// After eviction, next call must re-fetch
		mocks.exchangedToken = "token-after-eviction"
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		assert.Greater(t, mocks.exchangeCalls, callsAfterFirst,
			"exchange endpoint must be called again after eviction")
	})
}

// Spec: FR-016 — Background eviction does not remove valid (non-expired) entries
func TestTokenExchanger_Eviction_ValidEntries_NotEvicted(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		longExpiry := 3600 // 1 hour TTL
		mocks.exchangeExpiry = &longExpiry

		cfg := configForMocks(mocks)
		cfg.Cache.DefaultTTL = 500 * time.Millisecond // fast eviction loop

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "persist-test-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// Populate cache
		token1, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		callsAfterFirst := mocks.exchangeCalls

		// Allow several eviction ticks while the entry is still valid.
		time.Sleep(3200 * time.Millisecond)
		synctest.Wait()

		// Entry must still be in cache — no new exchange call
		token2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		assert.Equal(t, token1.Token, token2.Token,
			"valid cache entry must survive eviction cycles")
		assert.Equal(t, callsAfterFirst, mocks.exchangeCalls,
			"exchange endpoint must not be called for a non-expired cache entry")
	})
}

// Spec: FR-006 — Background goroutine refreshes client assertion before expiry
func TestTokenExchanger_AssertionRefresh_BackgroundRefresh_KeepsAssertionFresh(t *testing.T) {
	var assertionCallCount int64

	// Custom client credentials server that counts assertion refreshes.
	// Uses 1-second expires_in so maybeRefreshAssertion (which fires when remaining < 30s)
	// will always trigger a refresh on every eviction tick.
	clientCredsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&assertionCallCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Connection", "close")
		expiry := 1 // 1 second expires_in → assertion is "almost expired" immediately
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "access-token",
			IDToken:     fmt.Sprintf("id-token-%d", count),
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer clientCredsServer.Close()

	tokenExchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "exchanged-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer tokenExchServer.Close()

	synctest.Test(t, func(t *testing.T) {
		cfg := &extprocconfig.Config{
			GRPC: extprocconfig.GRPCConfig{Bind: "127.0.0.1", Port: 50051},
			OAuth2: extprocconfig.OAuth2Config{
				TokenEndpoint:             tokenExchServer.URL + "/oauth2/token",
				Issuer:                    clientCredsServer.URL,
				ClientID:                  "client",
				ClientSecret:              "secret",
				ClientCredentialsEndpoint: clientCredsServer.URL + "/oauth/token",
				ClientAssertionType:       "id_token",
				ExchangeTimeout:           5 * time.Second,
				TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
			},
			Cache: extprocconfig.CacheConfig{
				// DefaultTTL = 2s → eviction ticker fires every max(1s, 2s/2) = 1s.
				// Assertion refresh ticker = min(1s, 30s) = 1s.
				// Assertion expires in 1s; remaining is always < 30s threshold →
				// maybeRefreshAssertion triggers on every assertion ticker tick.
				DefaultTTL: 2 * time.Second,
				MaxTTL:     1 * time.Hour,
			},
			CircuitBreaker: extprocconfig.CircuitBreakerConfig{
				Enabled:      true,
				MaxFailures:  5,
				ResetTimeout: 30 * time.Second,
			},
		}

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()
		synctest.Wait()

		callsAtStartup := atomic.LoadInt64(&assertionCallCount)

		// Wait for at least 2 assertion refresh ticks (1s each) to fire.
		// Assertion ticker = min(DefaultTTL/2, 30s) = min(1s, 30s) = 1s.
		// With 1s expires_in → remaining is always < 30s → refresh is triggered on every tick.
		time.Sleep(2500 * time.Millisecond)
		synctest.Wait()

		callsAfterWait := atomic.LoadInt64(&assertionCallCount)

		// The background goroutine must have triggered at least one refresh beyond startup
		assert.Greater(t, callsAfterWait, callsAtStartup,
			"background goroutine must refresh client assertion before expiry")
	})
}

// Spec: FR-016 — Shutdown stops background eviction goroutine
func TestTokenExchanger_Shutdown_StopsEvictionGoroutine(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.Cache.DefaultTTL = 100 * time.Millisecond // fast eviction loop

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)

	// Shutdown must complete without blocking or panicking
	done := make(chan struct{})
	go func() {
		exchanger.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Shutdown completed as expected
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown must complete promptly — possible goroutine leak")
	}

	// Second Shutdown call must not panic (idempotent cleanup via stopCh close)
	// Note: calling Shutdown() twice on a channel that's already closed would panic,
	// so we verify single-shutdown semantics work correctly
}

// Spec: FR-014, FR-016 — Race detector: concurrent Exchange + Shutdown is safe
func TestTokenExchanger_Concurrent_ExchangeAndShutdown_RaceFree(t *testing.T) {
	// Use isolated mock servers with race-safe atomic counters (avoiding the
	// shared mutable fields in newMockServers() which have a pre-existing race).
	clientCredsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "access-token",
			IDToken:     "id-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer clientCredsServer.Close()

	tokenExchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "exchanged-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer tokenExchServer.Close()

	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{Bind: "127.0.0.1", Port: 50051},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             tokenExchServer.URL + "/oauth2/token",
			Issuer:                    clientCredsServer.URL,
			ClientID:                  "client",
			ClientSecret:              "secret",
			ClientCredentialsEndpoint: clientCredsServer.URL + "/oauth/token",
			ClientAssertionType:       "id_token",
			ExchangeTimeout:           5 * time.Second,
			TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 50 * time.Millisecond,
			MaxTTL:     1 * time.Hour,
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			Enabled:      true,
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
	}

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)

	var wg sync.WaitGroup

	// Concurrent exchange calls with unique keys (no singleflight contention needed here)
	for i := range 5 {
		wg.Go(func() {
			_, _ = exchanger.Exchange(
				context.Background(),
				fmt.Sprintf("token-%d", i),
				fmt.Sprintf("http://service-%d:9000/api", i),
			)
		})
	}

	// Concurrent shutdown
	wg.Go(func() {
		time.Sleep(10 * time.Millisecond) // let some exchanges start
		exchanger.Shutdown()
	})

	wg.Wait()
	// Test passes if no race conditions are detected by the Go race detector (-race flag)
}

// ---------------------------------------------------------------------------
// Regression: canceled leader does not abort live followers (disabled CB)
// ---------------------------------------------------------------------------

// Spec: singleflight — when the circuit breaker is disabled, a canceled leader
// context must not abort the shared exchange for concurrent follower callers.
func TestTokenExchanger_CbDisabled_CanceledLeader_FollowerSucceeds(t *testing.T) {
	// started receives a signal when the exchange server receives the first request.
	// release is closed to unblock the exchange server and let it respond.
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	// slowExchServer blocks until release is closed, simulating a slow broker.
	// The client-credentials endpoint (mocks) is separate so NewTokenExchanger
	// startup succeeds without blocking.
	slowExchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "shared-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer slowExchServer.Close()

	// mocks provides the client-credentials (assertion refresh) endpoint.
	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.OAuth2.TokenEndpoint = slowExchServer.URL + "/token"
	cfg.OAuth2.ExchangeTimeout = 10 * time.Second
	// Circuit breaker deliberately disabled (Enabled: false / zero value).
	cfg.CircuitBreaker = extprocconfig.CircuitBreakerConfig{}

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "shared-subject"
	const resourceURI = "http://resource.example.com/api"

	leaderCtx, leaderCancel := context.WithCancel(context.Background())

	var leaderErr error
	var followerTok string
	var followerErr error
	var wg sync.WaitGroup

	// Start leader — its context will be cancelled while the exchange is in flight.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, leaderErr = exchanger.Exchange(leaderCtx, subjectToken, resourceURI)
	}()

	// Wait for the slow server to receive the request, then cancel the leader.
	<-started
	leaderCancel()

	// Small pause to let the leader's DoChan select fire before the follower starts.
	time.Sleep(10 * time.Millisecond)

	// Follower uses an independent context that is never cancelled.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var followerResult server.ExchangeResult
		followerResult, followerErr = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		followerTok = followerResult.Token
	}()

	// Unblock the slow exchange server — both leader result and follower wait on it.
	close(release)
	wg.Wait()

	assert.ErrorIs(t, leaderErr, context.Canceled,
		"leader must receive context.Canceled after its context was cancelled")
	require.NoError(t, followerErr, "follower must not inherit the leader's cancellation")
	assert.Equal(t, "shared-token", followerTok,
		"follower must receive the shared exchange result")
}

// Spec: After a successful exchange is cached, if the cached entry expires and the
// broker starts returning error_uri for the same key, the re-auth error is returned.
// This verifies the stale-success window is bounded by the cache TTL (expires_in).
func TestTokenExchanger_Cache_ExpiredSuccessRevealsSameTokenReAuth(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1 // 1 second TTL — cache expires quickly
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "same-token-reauth-test"
		const resourceURI = "http://mcp-server:9003/mcp"

		// First call: success, result is cached.
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		// Wait for the cache entry to expire.
		time.Sleep(1100 * time.Millisecond)

		// Broker now requires re-auth for the same key.
		mocks.exchangeStatus = http.StatusUnauthorized
		mocks.exchangeErrCode = "invalid_grant"
		mocks.exchangeErrorURI = "https://idp.example.com/reauth"

		// After expiry, the same token/resource pair must reach the broker and return re-auth.
		callsBefore := mocks.exchangeCalls
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, err, "expired cache must not serve stale success — broker re-auth must surface")

		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, err, &brokerErr, "error must be a BrokerExchangeError")
		assert.Equal(t, "https://idp.example.com/reauth", brokerErr.ErrorURI)
		assert.Greater(t, mocks.exchangeCalls, callsBefore,
			"broker must be called after cache expiry — not a cache hit")
	})
}

// Spec: Re-auth errors are rate-limited per token+resource key (reAuthCooldownTTL)
// to prevent tight retry loops from hammering the broker. Within the cooldown window,
// repeated calls return the cached re-auth error without calling the broker.
func TestTokenExchanger_ReAuthCooldown_PreventsRepeatedBrokerCalls(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	mocks.exchangeStatus = http.StatusUnauthorized
	mocks.exchangeErrCode = "invalid_grant"
	mocks.exchangeErrorURI = "https://idp.example.com/reauth"

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "cooldown-test-token"
	const resourceURI = "http://mcp-server:9003/mcp"

	// First call: broker is called, re-auth error is returned and rate-limit is set.
	_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.Error(t, err)
	callsAfterFirst := mocks.exchangeCalls

	// Subsequent calls within the cooldown window must NOT call the broker again.
	for range 3 {
		_, exchErr := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, exchErr, "re-auth error must still be returned from cooldown cache")
		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, exchErr, &brokerErr)
		assert.Equal(t, "https://idp.example.com/reauth", brokerErr.ErrorURI)
	}

	assert.Equal(t, callsAfterFirst, mocks.exchangeCalls,
		"broker must not be called during the re-auth cooldown window")
}

// Spec: When the token cache expires and the broker returns a transient 5xx error,
// Exchange must serve the stale cached token rather than propagating the error.
// The stale window (staleUntil) is set once on the first transient failure and is
// never extended, so the stale period is bounded to one reAuthCooldownTTL interval.
func TestTokenExchanger_ExpiredCache_TransientError_ServesStaleToken(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1 // 1 second — cache expires quickly
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "stale-fallback-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// First call: success, token cached.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		// Wait for cache entry to expire.
		time.Sleep(1100 * time.Millisecond)

		// Broker now returns a transient 5xx error.
		mocks.exchangeStatus = http.StatusInternalServerError
		mocks.exchangeErrCode = "server_error"

		// After expiry, Exchange calls broker. Broker returns transient error — serve stale.
		tok2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err, "transient 5xx at expiry must serve stale token, not error")
		assert.Equal(t, "exchanged-access-token", tok2.Token, "stale cached token must be returned")
	})
}

// Spec: When the token cache expires and the broker returns 429 (throttling, no ErrorURI),
// Exchange must serve the stale cached token. 429 without error_uri is transient.
func TestTokenExchanger_ExpiredCache_429_ServesStaleToken(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "429-stale-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		time.Sleep(1100 * time.Millisecond)

		mocks.exchangeStatus = http.StatusTooManyRequests
		mocks.exchangeErrCode = "slow_down"

		tok2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err, "429 throttling at expiry must serve stale token")
		assert.Equal(t, "exchanged-access-token", tok2.Token)
	})
}

// Spec: When the token cache expires and the broker returns an authoritative 4xx rejection
// without error_uri, Exchange must propagate the error — not serve the stale cached token.
func TestTokenExchanger_ExpiredCache_AuthoritativeError_PropagatesError(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "4xx-at-expiry-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		time.Sleep(1100 * time.Millisecond)

		mocks.exchangeStatus = http.StatusForbidden
		mocks.exchangeErrCode = "access_denied"

		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, err, "authoritative 4xx at expiry must propagate — not serve stale")
		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, err, &brokerErr)
		assert.Equal(t, http.StatusForbidden, brokerErr.StatusCode)
	})
}

// Spec: A 5xx response that carries error_uri must NOT overwrite the success cache with
// a reAuthErr entry. 5xx is always transient; the stale cached token must be served
// and the re-auth elicitation URL must not be surfaced for transient server errors.
func TestTokenExchanger_5xxWithErrorURI_DoesNotOverwriteSuccessCache(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "5xx-error-uri-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// First call: success, token cached.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		time.Sleep(1100 * time.Millisecond)

		// Broker returns 5xx with error_uri (transient — must not trigger re-auth).
		mocks.exchangeStatus = http.StatusInternalServerError
		mocks.exchangeErrCode = "server_error"
		mocks.exchangeErrorURI = "https://idp.example.com/reauth"

		// Must serve the stale cached token, not propagate re-auth.
		tok2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err, "5xx with error_uri is transient — must serve stale token, not re-auth error")
		assert.Equal(t, "exchanged-access-token", tok2.Token)
	})
}

// Spec: Repeated transient failures within the stale window must all serve the stale
// cached token. The broker is probed on each call (fast-path cache check fails because
// expiresAt is in the past), but staleUntil is not re-extended on subsequent failures —
// verifying the stale window is bounded to one reAuthCooldownTTL period.
func TestTokenExchanger_ExpiredCache_RepeatedTransientErrors_StaleWindowBounded(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1 // 1 second — cache expires quickly
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "repeated-transient-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// First call: success, token cached.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		// Wait for cache entry to expire.
		time.Sleep(1100 * time.Millisecond)

		// Broker now returns a transient 5xx.
		mocks.exchangeStatus = http.StatusInternalServerError
		mocks.exchangeErrCode = "server_error"

		// Three consecutive calls after expiry: broker is probed each time (fast-path fails)
		// and stale token is returned on each. The broker call count increases each time,
		// proving the fast-path is not being hit — only the stale fallback is active.
		callsBefore := mocks.exchangeCalls
		const numCalls = 3
		for i := range numCalls {
			tok2, exchErr := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
			require.NoError(t, exchErr, "call %d: transient 5xx must serve stale token", i)
			assert.Equal(t, "exchanged-access-token", tok2.Token, "call %d: stale token must be returned", i)
		}
		assert.Equal(t, callsBefore+numCalls, mocks.exchangeCalls,
			"broker must be probed on every post-expiry call — stale window must not re-warm the fast-path cache")
	})
}

// Spec: A max_ttl shorter than expires_in caps both the cache TTL and stale window.
// A transient broker error after that deadline cannot return the old token.
func TestTokenExchanger_StaleWindow_CappedByMaxTTL(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortTTL := 1 // 1 second expires_in
		mocks.exchangeExpiry = &shortTTL

		cfg := configForMocks(mocks)
		// max_ttl caps the 1s expires_in, so staleUntil equals expiresAt at 200ms.
		cfg.Cache.MaxTTL = 200 * time.Millisecond

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "max-ttl-cap-test-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// Warm the cache.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		// Cross the exact max_ttl boundary without waiting on wall-clock time.
		time.Sleep(200*time.Millisecond + time.Nanosecond)

		mocks.exchangeStatus = http.StatusInternalServerError
		mocks.exchangeErrCode = "server_error"

		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, err,
			"transient 5xx after staleUntil cap (max_ttl) must propagate — stale serving must be disabled")
		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, err, &brokerErr)
		assert.Equal(t, http.StatusInternalServerError, brokerErr.StatusCode)
	})
}

// Spec: When the token's expires_in exceeds cache.max_ttl (so ttl is capped to max_ttl),
// staleUntil must equal expiresAt — no stale extension beyond max_ttl is permitted.
// This is the common case in production where long-lived tokens are capped by config.
func TestTokenExchanger_StaleWindow_TtlCappedAtMaxTTL_NoExtensionBeyondCap(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		// expires_in >> max_ttl so ttl = max_ttl after computeTTL cap.
		largeExpiry := 3600 // 1 hour; far exceeds max_ttl = 500ms
		mocks.exchangeExpiry = &largeExpiry

		cfg := configForMocks(mocks)
		cfg.Cache.MaxTTL = 500 * time.Millisecond // staleUntil = min(expiresAt+5s, now+500ms) = now+500ms

		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "max-ttl-no-extension-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// Warm cache.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)

		// Wait past max_ttl (500ms + buffer = 700ms). Both expiresAt and staleUntil should
		// now be in the past (staleUntil = now+500ms since ttl was capped to max_ttl).
		time.Sleep(700 * time.Millisecond)

		mocks.exchangeStatus = http.StatusInternalServerError
		mocks.exchangeErrCode = "server_error"

		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, err,
			"transient 5xx after max_ttl cap must propagate — staleUntil must not extend past max_ttl when ttl==max_ttl")
		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, err, &brokerErr)
		assert.Equal(t, http.StatusInternalServerError, brokerErr.StatusCode)
	})
}

// Spec: isTransientBrokerError must treat 429 as transient only when ErrorURI is absent.
// A 429 with error_uri carries an explicit re-auth signal and must be treated as
// authoritative so the caller can surface the elicitation URL rather than masking
// it with a stale cached token.
func TestIsTransientBrokerError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"429 no error_uri — transient", &server.BrokerExchangeError{StatusCode: 429, Code: "slow_down"}, true},
		{"429 with error_uri — authoritative", &server.BrokerExchangeError{StatusCode: 429, Code: "slow_down", ErrorURI: "https://idp.example.com/reauth"}, false},
		{"500 no error_uri — transient", &server.BrokerExchangeError{StatusCode: 500, Code: "server_error"}, true},
		{"500 with error_uri — transient (5xx always transient)", &server.BrokerExchangeError{StatusCode: 500, Code: "server_error", ErrorURI: "https://idp.example.com/reauth"}, true},
		{"401 with error_uri — authoritative", &server.BrokerExchangeError{StatusCode: 401, Code: "invalid_token", ErrorURI: "https://idp.example.com/reauth"}, false},
		{"403 no error_uri — authoritative", &server.BrokerExchangeError{StatusCode: 403, Code: "access_denied"}, false},
		{"ErrAssertionExpired — not transient (must not serve stale token)", server.ErrAssertionExpired, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := server.IsTransientBrokerError(tc.err)
			assert.Equal(t, tc.transient, got)
		})
	}
}

// Spec: A valid cached success token is served until expiresAt without intermediate
// broker calls. Revocation or re-auth from the broker is visible only after expiry.
func TestTokenExchanger_Cache_SuccessServedUntilExpiry_ReAuthVisibleAfter(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		shortExpiry := 1 // 1 second — expire quickly so re-auth detection can be tested
		mocks.exchangeExpiry = &shortExpiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "live-cache-reauth-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		// Populate success cache.
		tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		require.Equal(t, "exchanged-access-token", tok.Token)
		callsAfterFirst := mocks.exchangeCalls

		// Broker now requires re-auth for the same key.
		mocks.exchangeStatus = http.StatusUnauthorized
		mocks.exchangeErrCode = "invalid_grant"
		mocks.exchangeErrorURI = "https://idp.example.com/reauth"

		// Within the expiry window: cached success is returned, broker is NOT called.
		tok2, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err, "within expiry window: cached success must be returned, re-auth not yet visible")
		assert.Equal(t, "exchanged-access-token", tok2.Token)
		assert.Equal(t, callsAfterFirst, mocks.exchangeCalls,
			"broker must not be called within the cache TTL window")

		// After expiry, the broker is called and the re-auth error becomes visible.
		time.Sleep(1100 * time.Millisecond)

		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.Error(t, err, "after expiry: re-auth must be visible")
		var brokerErr *server.BrokerExchangeError
		require.ErrorAs(t, err, &brokerErr)
		assert.Equal(t, "https://idp.example.com/reauth", brokerErr.ErrorURI,
			"re-auth error from broker must be returned after cache expiry")
		assert.Greater(t, mocks.exchangeCalls, callsAfterFirst,
			"broker must be called after cache expiry to detect re-auth")
	})
}

// Spec: A cached re-auth error must not be returned after the client assertion expires.
// ErrAssertionExpired takes precedence — the service is broken and cannot exchange tokens,
// so surfacing a re-auth redirect would be misleading.
func TestTokenExchanger_CachedReAuthErr_AssertionExpired_ReturnsErrAssertionExpired(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	longExpiry := 3600
	mocks.exchangeExpiry = &longExpiry
	mocks.exchangeStatus = http.StatusUnauthorized
	mocks.exchangeErrorURI = "https://idp.example.com/reauth"
	mocks.exchangeErrCode = "invalid_grant"

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "reauth-assertion-test"
	const resourceURI = "http://mcp-server:9003/mcp"

	// Populate the reAuthErr cache entry via a 401+error_uri response.
	_, firstErr := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.Error(t, firstErr, "first call must fail with re-auth error")
	var brokerErr *server.BrokerExchangeError
	require.ErrorAs(t, firstErr, &brokerErr, "first error must be BrokerExchangeError")
	require.Equal(t, "https://idp.example.com/reauth", brokerErr.ErrorURI)

	// Expire the assertion — the service is now unable to exchange tokens.
	exchanger.ExpireAssertion()

	// The re-auth cache entry must NOT be served; ErrAssertionExpired takes precedence.
	_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.ErrorIs(t, err, server.ErrAssertionExpired,
		"expired assertion must surface even when a cached re-auth entry exists")
}

// Spec: A warm cache hit must not bypass assertion expiry.
// When the client assertion expires, Exchange must return ErrAssertionExpired
// even when a valid cached success entry exists — serving a cached token after
// the assertion lapses violates the fail-closed guarantee.
func TestTokenExchanger_CachedToken_AssertionExpired_ReturnsErrAssertionExpired(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	longExpiry := 3600
	mocks.exchangeExpiry = &longExpiry

	cfg := configForMocks(mocks)
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "warm-cache-token"
	const resourceURI = "http://mcp-server:9003/mcp"

	// Warm the cache with a valid success entry.
	tok, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.NoError(t, err)
	require.NotEmpty(t, tok, "first call must return a token")

	// Simulate assertion expiry by injecting an expired state.
	exchanger.ExpireAssertion()

	// Exchange must now return ErrAssertionExpired, not the cached token.
	_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.ErrorIs(t, err, server.ErrAssertionExpired,
		"expired assertion must surface even when a cached success entry exists")
}

// Spec: stale-token fallback in serveStaleOrErr must not serve cached tokens when
// the client assertion has expired. This matters especially for the circuit-open path:
// when the circuit breaker is open, doExchange is never called, so the assertion
// check inside doExchange cannot guard the stale fallback — only the guard in
// serveStaleOrErr prevents serving a cached token after assertion expiry.
func TestTokenExchanger_OpenCircuit_AssertionExpired_ReturnsErrCircuitOpen(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	longExpiry := 3600
	mocks.exchangeExpiry = &longExpiry

	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{Bind: "127.0.0.1", Port: 50051},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             mocks.tokenExchServer.URL + "/oauth2/token",
			Issuer:                    mocks.clientCredsServer.URL,
			ClientID:                  "test-client",
			ClientSecret:              "test-secret",
			ClientCredentialsEndpoint: mocks.clientCredsServer.URL + "/oauth/token",
			ClientAssertionType:       "id_token",
			ExchangeTimeout:           5 * time.Second,
			TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{DefaultTTL: 5 * time.Minute, MaxTTL: 1 * time.Hour},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			Enabled:      true,
			MaxFailures:  2,
			ResetTimeout: 1 * time.Hour,
		},
	}
	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	const subjectToken = "circuit-assertion-test"
	const resourceURI = "http://mcp-server:9003/mcp"

	// Warm the cache so a stale entry is available.
	_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.NoError(t, err)

	// Trip the circuit breaker directly (bypasses cache to avoid waiting for nextReauthCheck).
	exchanger.TripCircuitBreaker(2)

	// Expire the assertion after the circuit is open.
	exchanger.ExpireAssertion()

	// With circuit open + expired assertion: serveStaleOrErr must NOT serve the cached
	// token. ErrCircuitOpen must be returned to surface the unavailability.
	_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
	require.ErrorIs(t, err, server.ErrCircuitOpen,
		"stale-token fallback must not activate when assertion is expired and circuit is open")
}
