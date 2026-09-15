// Package server_test — circuit_breaker_test.go covers the circuit breaker
// protecting token exchange calls to the identity broker.
//
// Tests verify:
//   - Circuit opens after max_failures consecutive errors
//   - Open circuit rejects calls immediately with ErrCircuitOpen
//   - Half-open state allows a single probe after reset_timeout
//   - Successful probe closes the circuit
//   - Failed probe re-opens the circuit
//   - Cache hits bypass the circuit breaker (no backend call needed)
//   - Circuit breaker is thread-safe under concurrent access
package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// eventuallyTimeout is the extra buffer added to resetTimeout when waiting for
// circuit state transitions under test. 500ms gives ample slack for CI scheduling
// jitter without making tests noticeably slow.
const eventuallyTimeout = 500 * time.Millisecond

// eventuallyPollInterval is the polling cadence for require.Eventually calls that
// probe circuit breaker state transitions. 20ms balances test speed with CPU waste.
const eventuallyPollInterval = 20 * time.Millisecond

// circuitBreakerConfig returns a config with a low threshold for fast circuit breaker testing.
func circuitBreakerConfig(m *mockServers, maxFailures int, resetTimeout time.Duration) *extprocconfig.Config {
	cfg := configForMocks(m)
	cfg.CircuitBreaker = extprocconfig.CircuitBreakerConfig{
		Enabled:      true,
		MaxFailures:  maxFailures,
		ResetTimeout: resetTimeout,
	}
	return cfg
}

// ---------------------------------------------------------------------------
// Circuit breaker: open after max_failures
// ---------------------------------------------------------------------------

// Circuit opens after max_failures consecutive exchange errors.
func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"

	const maxFailures = 3
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Trigger max_failures consecutive errors (each with a unique key to avoid singleflight caching errors)
	for i := range maxFailures {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d should fail with backend error, not circuit open", i)
	}

	// Next call should be rejected by the circuit breaker
	_, err = exchanger.Exchange(context.Background(), "token-after-trip", "http://resource.example.com/after")
	assert.ErrorIs(t, err, server.ErrCircuitOpen,
		"circuit must be open after %d consecutive failures", maxFailures)
}

// ---------------------------------------------------------------------------
// Circuit breaker: fast-fail when open
// ---------------------------------------------------------------------------

// Open circuit rejects calls immediately without hitting the backend.
func TestCircuitBreaker_OpenCircuit_RejectsImmediately(t *testing.T) {
	var backendCalls int64

	exchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&backendCalls, 1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, `{"error":"server_error"}`)
	}))
	defer exchServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 1*time.Minute)
	cfg.OAuth2.TokenEndpoint = exchServer.URL + "/oauth2/token"

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Trip the circuit
	for i := range maxFailures {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
	}
	callsAfterTrip := atomic.LoadInt64(&backendCalls)

	// Additional calls must NOT hit the backend
	for i := range 5 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("token-open-%d", i), fmt.Sprintf("http://resource.example.com/open-%d", i))
		assert.ErrorIs(t, err, server.ErrCircuitOpen)
	}

	assert.Equal(t, callsAfterTrip, atomic.LoadInt64(&backendCalls),
		"no backend calls should be made while circuit is open")
}

// ---------------------------------------------------------------------------
// Circuit breaker: half-open → probe succeeds → closed
// ---------------------------------------------------------------------------

// After reset_timeout, a successful probe closes the circuit.
func TestCircuitBreaker_HalfOpen_SuccessfulProbe_ClosesCircuit(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	const resetTimeout = 200 * time.Millisecond
	cfg := circuitBreakerConfig(mocks, maxFailures, resetTimeout)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Trip the circuit with failures
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
	}

	// Verify circuit is open
	_, err = exchanger.Exchange(context.Background(), "token-check", "http://resource.example.com/check")
	assert.ErrorIs(t, err, server.ErrCircuitOpen, "circuit must be open")

	// Restore backend and wait until the probe succeeds (circuit moves through half-open → closed).
	// Using Eventually avoids a fixed sleep that would be flaky under CI load.
	mocks.exchangeStatus = http.StatusOK
	mocks.exchangedToken = "recovered-token"

	var token string
	require.Eventually(
		t,
		func() bool {
			tok, err := exchanger.Exchange(context.Background(), "token-probe", "http://resource.example.com/probe")
			if err != nil {
				// Circuit may still be open; keep retrying until reset_timeout expires.
				return false
			}
			token = tok.Token
			return token == "recovered-token"
		},
		resetTimeout+eventuallyTimeout,
		eventuallyPollInterval,
		"half-open probe should succeed with recovered backend",
	)

	// Subsequent calls should work normally (circuit is closed)
	result2, err := exchanger.Exchange(context.Background(), "token-normal", "http://resource.example.com/normal")
	require.NoError(t, err, "circuit should be closed after successful probe")
	assert.Equal(t, "recovered-token", result2.Token)
}

// ---------------------------------------------------------------------------
// Circuit breaker: half-open → probe fails → re-opens
// ---------------------------------------------------------------------------

// After reset_timeout, a failed probe re-opens the circuit.
func TestCircuitBreaker_HalfOpen_FailedProbe_ReopensCircuit(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	const resetTimeout = 200 * time.Millisecond
	cfg := circuitBreakerConfig(mocks, maxFailures, resetTimeout)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Trip the circuit
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
	}

	// Wait until a probe is allowed (circuit transitions to half-open after reset_timeout).
	// Using Eventually avoids a fixed sleep that can be flaky under CI scheduling pressure.
	// The backend is still failing, so the probe should fail and re-open the circuit.
	require.Eventually(
		t,
		func() bool {
			_, err := exchanger.Exchange(context.Background(), "token-probe", "http://resource.example.com/probe")
			// The probe either reached the backend (non-circuit error) or the circuit is open.
			// We stop polling as soon as we get a non-circuit-open error — that means a probe was attempted.
			return err != nil && !errors.Is(err, server.ErrCircuitOpen)
		},
		resetTimeout+eventuallyTimeout,
		eventuallyPollInterval,
		"probe should be attempted after reset_timeout with backend still down",
	)

	// Circuit should be re-opened — immediate calls should be rejected
	_, err = exchanger.Exchange(context.Background(), "token-after-probe", "http://resource.example.com/after-probe")
	assert.ErrorIs(t, err, server.ErrCircuitOpen,
		"circuit must re-open after failed probe")
}

// ---------------------------------------------------------------------------
// Circuit breaker: cache hits bypass circuit breaker
// ---------------------------------------------------------------------------

// Cached tokens are returned even when the circuit is open.
func TestCircuitBreaker_CacheHit_BypassesCircuitBreaker(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 1*time.Minute)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Cache a successful token first
	const cachedKey = "cached-token"
	const cachedResource = "http://resource.example.com/cached"
	token1, err := exchanger.Exchange(context.Background(), cachedKey, cachedResource)
	require.NoError(t, err)

	// Trip the circuit with failures on different keys
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("fail-token-%d", i), fmt.Sprintf("http://resource.example.com/fail-%d", i))
	}

	// Circuit is now open — but cached entry should still work
	token2, err := exchanger.Exchange(context.Background(), cachedKey, cachedResource)
	require.NoError(t, err, "cache hit must succeed even when circuit is open")
	assert.Equal(t, token1.Token, token2.Token, "cached token must be returned")
}

// ---------------------------------------------------------------------------
// Circuit breaker: success resets failure count
// ---------------------------------------------------------------------------

// A successful exchange resets the consecutive failure counter.
func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 3
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// 2 failures (under threshold)
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures - 1 {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
	}

	// 1 success — resets failure count
	mocks.exchangeStatus = http.StatusOK
	mocks.exchangedToken = "success-token"
	_, err = exchanger.Exchange(context.Background(), "token-success", "http://resource.example.com/success")
	require.NoError(t, err)

	// 2 more failures — should NOT trip because count was reset
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures - 1 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("token-after-%d", i), fmt.Sprintf("http://resource.example.com/after-%d", i))
		assert.Error(t, err)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"circuit should not be open — failure count was reset by success")
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: concurrent access is safe
// ---------------------------------------------------------------------------

// Concurrent exchanges must not cause a data race on the circuit breaker.
func TestCircuitBreaker_ConcurrentAccess_RaceFree(t *testing.T) {
	// Use isolated race-safe servers (atomic counters, no shared mutable state)
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

	var failCount int64
	exchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt64(&failCount, 1)
		if count <= 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"error":"server_error"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		expiry := 3600
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "exchanged-token",
			TokenType:   "Bearer",
			ExpiresIn:   &expiry,
		})
	}))
	defer exchServer.Close()

	cfg := &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{Bind: "127.0.0.1", Port: 50051},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:             exchServer.URL + "/oauth2/token",
			Issuer:                    clientCredsServer.URL,
			ClientID:                  "client",
			ClientSecret:              "secret",
			ClientCredentialsEndpoint: clientCredsServer.URL + "/oauth/token",
			ClientAssertionType:       "id_token",
			ExchangeTimeout:           5 * time.Second,
			TLS:                       extprocconfig.TLSConfig{AllowHTTP: true},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			Enabled:      true,
			MaxFailures:  10, // high threshold to avoid opening during test
			ResetTimeout: 30 * time.Second,
		},
	}

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = exchanger.Exchange(
				context.Background(),
				fmt.Sprintf("token-%d", idx),
				fmt.Sprintf("http://service-%d:9000/api", idx),
			)
		}(i)
	}
	wg.Wait()
	// Test passes if no race conditions are detected by the Go race detector (-race flag)
}

// ---------------------------------------------------------------------------
// Circuit breaker: server.go integration — ErrCircuitOpen returns 503
// ---------------------------------------------------------------------------

// ErrCircuitOpen from exchanger → 503 ImmediateResponse with appropriate body.
func TestServer_Process_CircuitOpen_Returns503(t *testing.T) {
	exchanger := &mockExchanger{
		exchangeFunc: func(_ context.Context, _, _ string) (server.ExchangeResult, error) {
			return server.ExchangeResult{}, server.ErrCircuitOpen
		},
	}
	client, cleanup := startTestServer(t, exchanger)
	defer cleanup()

	resp, err := sendRequestHeaders(t, client, nil)
	require.NoError(t, err)

	immResp, ok := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse)
	require.True(t, ok, "circuit open must produce an ImmediateResponse")
	assert.Equal(t, int32(httpv3.StatusCode_ServiceUnavailable),
		int32(immResp.ImmediateResponse.Status.Code),
		"circuit open must return HTTP 503")
	assert.Equal(t, `{"error":"service_unavailable","error_description":"circuit breaker is open"}`, string(immResp.ImmediateResponse.Body))
	assert.Equal(t, "application/json", headerMutationValue(immResp.ImmediateResponse.Headers, "content-type"))
	assert.Empty(t, headerMutationValue(immResp.ImmediateResponse.Headers, "authorization"), "open circuit must not mutate authorization")
}

// ---------------------------------------------------------------------------
// Circuit breaker: 4xx client errors do NOT trip the circuit
// ---------------------------------------------------------------------------

// 4xx broker errors (e.g., invalid subject token → 400/401) are client errors and must
// NOT count towards the circuit breaker failure threshold. Only 5xx server errors trip it.
func TestCircuitBreaker_4xxErrors_DoNotTripCircuit(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Return 400 Bad Request (e.g., invalid_grant for an invalid subject token)
	mocks.exchangeStatus = http.StatusBadRequest
	mocks.exchangeErrCode = "invalid_grant"

	// Send more than maxFailures requests with 4xx errors
	for i := range maxFailures + 3 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("bad-token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d: 4xx error must NOT trip circuit breaker", i)
	}
}

// 401 Unauthorized responses (e.g., expired or revoked subject token) must not trip the circuit.
func TestCircuitBreaker_401Unauthorized_DoesNotTripCircuit(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	mocks.exchangeStatus = http.StatusUnauthorized
	mocks.exchangeErrCode = "invalid_token"

	for i := range maxFailures + 3 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("expired-token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d: 401 error must NOT trip circuit breaker", i)
	}
}

// 5xx errors still trip the circuit as before.
func TestCircuitBreaker_5xxErrors_StillTripCircuit(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"

	for i := range maxFailures {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
	}

	// Circuit should now be open
	_, err = exchanger.Exchange(context.Background(), "token-after-trip", "http://resource.example.com/after")
	assert.ErrorIs(t, err, server.ErrCircuitOpen,
		"5xx errors must still trip the circuit breaker")
}

// Mixed 4xx and 5xx: 4xx in between 5xx errors resets consecutive failure count.
func TestCircuitBreaker_4xxBetween5xx_DoesNotCountAsFailure(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 3
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// 2 consecutive 5xx errors (under threshold)
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures - 1 {
		_, _ = exchanger.Exchange(context.Background(), fmt.Sprintf("token-5xx-%d", i), fmt.Sprintf("http://resource.example.com/5xx-%d", i))
	}

	// 1 x 4xx error — counts as success for circuit breaker, resets failure counter
	mocks.exchangeStatus = http.StatusBadRequest
	mocks.exchangeErrCode = "invalid_grant"
	_, err = exchanger.Exchange(context.Background(), "token-4xx", "http://resource.example.com/4xx")
	assert.Error(t, err, "4xx should still return an error to the caller")
	assert.NotErrorIs(t, err, server.ErrCircuitOpen)

	// 2 more 5xx errors — should NOT trip because failure count was reset by 4xx
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"
	for i := range maxFailures - 1 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("token-after-%d", i), fmt.Sprintf("http://resource.example.com/after-%d", i))
		assert.Error(t, err)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"circuit should not be open — failure count was reset by 4xx success")
	}
}

// isServerError classifies errors for circuit-breaker purposes.
// These tests verify the exact boundary conditions, including the zero-value case.
func TestIsServerError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"generic non-broker error", errors.New("network timeout"), true},
		{"status 0 (zero-value BrokerExchangeError)", &server.BrokerExchangeError{StatusCode: 0}, false},
		{"status 200", &server.BrokerExchangeError{StatusCode: 200}, false},
		{"status 400", &server.BrokerExchangeError{StatusCode: 400}, false},
		{"status 401", &server.BrokerExchangeError{StatusCode: 401}, false},
		{"status 404", &server.BrokerExchangeError{StatusCode: 404}, false},
		{"status 499", &server.BrokerExchangeError{StatusCode: 499}, false},
		{"status 500", &server.BrokerExchangeError{StatusCode: 500}, true},
		{"status 503", &server.BrokerExchangeError{StatusCode: 503}, true},
		{"wrapped broker error 400", fmt.Errorf("wrapped: %w", &server.BrokerExchangeError{StatusCode: 400}), false},
		{"wrapped broker error 500", fmt.Errorf("wrapped: %w", &server.BrokerExchangeError{StatusCode: 500}), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, server.IsServerError(tc.err))
		})
	}
}

// ---------------------------------------------------------------------------
// Circuit breaker: disabled mode bypasses circuit entirely
// ---------------------------------------------------------------------------

// When circuit_breaker.enabled is false, exchanges bypass the circuit breaker.
// Backend failures never trigger ErrCircuitOpen.
func TestCircuitBreaker_Disabled_NeverTrips(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.CircuitBreaker = extprocconfig.CircuitBreakerConfig{
		Enabled: false,
		// MaxFailures and ResetTimeout are irrelevant when disabled.
	}

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// Return 500 for all requests
	mocks.exchangeStatus = http.StatusInternalServerError
	mocks.exchangeErrCode = "server_error"

	// Many consecutive failures should NOT produce ErrCircuitOpen
	for i := range 20 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail with backend error", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d: disabled circuit breaker must never produce ErrCircuitOpen", i)
	}
}

// When circuit_breaker.enabled is false, successful exchanges still work normally.
func TestCircuitBreaker_Disabled_SuccessfulExchangeWorks(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	cfg := configForMocks(mocks)
	cfg.CircuitBreaker = extprocconfig.CircuitBreakerConfig{
		Enabled: false,
	}

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	result, err := exchanger.Exchange(context.Background(), "valid-token", "http://resource.example.com/api")
	require.NoError(t, err)
	assert.Equal(t, "exchanged-access-token", result.Token)
}

// ---------------------------------------------------------------------------
// Circuit breaker: 4xx without RFC 8693 error body do NOT trip the circuit
// ---------------------------------------------------------------------------

// When a broker responds with 4xx but no parseable RFC 8693 error body (empty
// "error" field or completely empty/non-JSON body), the exchange returns a typed
// BrokerExchangeError with the HTTP status code. isServerError classifies these
// as client errors, so the circuit must NOT trip.

// 4xx with an empty "error" field in the response body (no RFC 8693 error code).
func TestCircuitBreaker_4xxEmptyErrorField_DoesNotTripCircuit(t *testing.T) {
	// Serve 4xx with body `{"error":""}` — errBody.Code will be "" after parse,
	// which triggers the fallback BrokerExchangeError path in doExchange.
	exchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":""}`)
	}))
	defer exchServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)
	cfg.OAuth2.TokenEndpoint = exchServer.URL + "/oauth2/token"

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	// More than maxFailures 4xx requests — circuit must stay closed.
	for i := range maxFailures + 3 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("bad-token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d: 4xx with empty error field must NOT trip circuit breaker", i)
	}
}

// 4xx with a completely empty response body does NOT trip the circuit.
func TestCircuitBreaker_4xxEmptyBody_DoesNotTripCircuit(t *testing.T) {
	exchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Write nothing — the broker returned a 401 with no body.
	}))
	defer exchServer.Close()

	mocks := newMockServers()
	defer mocks.Close()

	const maxFailures = 2
	cfg := circuitBreakerConfig(mocks, maxFailures, 30*time.Second)
	cfg.OAuth2.TokenEndpoint = exchServer.URL + "/oauth2/token"

	exchanger, err := server.NewTokenExchanger(cfg, testLogger())
	require.NoError(t, err)
	defer exchanger.Shutdown()

	for i := range maxFailures + 3 {
		_, err := exchanger.Exchange(context.Background(), fmt.Sprintf("expired-token-%d", i), fmt.Sprintf("http://resource.example.com/%d", i))
		assert.Error(t, err, "exchange %d should fail", i)
		assert.NotErrorIs(t, err, server.ErrCircuitOpen,
			"exchange %d: 4xx with empty body must NOT trip circuit breaker", i)
	}
}
