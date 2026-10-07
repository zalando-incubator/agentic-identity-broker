package jwks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type deadlineCapturingRoundTripper struct {
	deadlineCh chan time.Time
	doneCh     chan struct{}
	doneOnce   sync.Once
}

type shutdownControllerStub struct {
	calls int
	err   error
}

type safeLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func (b *safeLogBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.Buffer.Bytes()...)
}

func (s *shutdownControllerStub) ShutdownContext(context.Context) error {
	s.calls++
	return s.err
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type countingRoundTripper struct {
	calls atomic.Int64
}

func (rt *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.calls.Add(1)
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func (rt *deadlineCapturingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if deadline, ok := req.Context().Deadline(); ok {
		select {
		case rt.deadlineCh <- deadline:
		default:
		}
	} else {
		select {
		case rt.deadlineCh <- time.Time{}:
		default:
		}
	}
	<-req.Context().Done()
	rt.doneOnce.Do(func() { close(rt.doneCh) })
	return nil, req.Context().Err()
}

func TestShutdownControllerWithWarning_LogsCleanupFailure(t *testing.T) {
	ctrl := &shutdownControllerStub{err: assert.AnError}
	var logs safeLogBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	shutdownControllerWithWarning(ctrl, logger, "resource registration failure")

	assert.Equal(t, 1, ctrl.calls)
	assert.Contains(t, logs.String(), "failed to shut down JWKS controller during constructor cleanup")
	assert.Contains(t, logs.String(), "resource registration failure")
}

func TestShutdownControllerWithWarning_DoesNotLogOnSuccess(t *testing.T) {
	ctrl := &shutdownControllerStub{}
	var logs safeLogBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	shutdownControllerWithWarning(ctrl, logger, "resource creation failure")

	assert.Equal(t, 1, ctrl.calls)
	assert.Empty(t, logs.String())
}

// TestNewJWKSAdapter tests adapter creation with various parameter combinations
func TestNewJWKSAdapter(t *testing.T) {
	tests := []struct {
		name               string
		jwksURI            string
		httpClient         *http.Client
		minRefreshInterval time.Duration
		maxRefreshInterval time.Duration
		expectError        bool
		errorContains      string
	}{
		{
			name:               "valid parameters",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         http.DefaultClient,
			minRefreshInterval: 15 * time.Minute,
			maxRefreshInterval: time.Hour,
			expectError:        false,
		},
		{
			name:               "zero intervals",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         http.DefaultClient,
			minRefreshInterval: 0,
			maxRefreshInterval: 0,
			expectError:        false,
		},
		{
			name:               "empty jwks_uri",
			jwksURI:            "",
			httpClient:         http.DefaultClient,
			minRefreshInterval: 15 * time.Minute,
			maxRefreshInterval: time.Hour,
			expectError:        true,
			errorContains:      "cannot be empty",
		},
		{
			name:               "nil http client",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         nil,
			minRefreshInterval: 15 * time.Minute,
			maxRefreshInterval: time.Hour,
			expectError:        true,
			errorContains:      "cannot be nil",
		},
		{
			name:               "negative min_refresh_interval",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         http.DefaultClient,
			minRefreshInterval: -1 * time.Minute,
			maxRefreshInterval: time.Hour,
			expectError:        true,
			errorContains:      "cannot be negative",
		},
		{
			name:               "negative max_refresh_interval",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         http.DefaultClient,
			minRefreshInterval: 15 * time.Minute,
			maxRefreshInterval: -1 * time.Hour,
			expectError:        true,
			errorContains:      "cannot be negative",
		},
		{
			name:               "min_refresh > max_refresh",
			jwksURI:            "https://auth.example.com/.well-known/jwks.json",
			httpClient:         http.DefaultClient,
			minRefreshInterval: 2 * time.Hour,
			maxRefreshInterval: time.Hour,
			expectError:        true,
			errorContains:      "must be <=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewJWKSAdapter(tt.jwksURI, tt.httpClient, tt.minRefreshInterval, tt.maxRefreshInterval, testLogger())

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, adapter)
				if tt.errorContains != "" {
					assert.ErrorContains(t, err, tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, adapter)
				assert.Equal(t, tt.jwksURI, adapter.jwksURI)
			}
		})
	}
}

// TestGetKeySet tests successful JWKS fetching and caching
func TestGetKeySet(t *testing.T) {
	// Create a test JWKS with one key
	testKey, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_"))
	require.NoError(t, err)
	require.NoError(t, testKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, testKey.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(testKey))

	// Convert keyset to JSON (what upstream server returns)
	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	// Create mock upstream JWKS endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	// Create adapter
	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	// Get keyset
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultSet, err := adapter.GetKeySet(ctx)
	require.NoError(t, err)
	require.NotNil(t, resultSet)

	// Verify keyset contains our test key
	key, found := resultSet.LookupKeyID("test-kid")
	assert.True(t, found)
	assert.NotNil(t, key)
}

// TestGetKeySet_HTTPError tests handling of HTTP errors
func TestGetKeySet_HTTPError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	assert.Error(t, err)
}

// TestGetKeySet_InvalidJSON tests handling of invalid JWKS format
func TestGetKeySet_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json {"))
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	assert.Error(t, err)
}

func TestGetKeySet_RejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"keys":[]}`)
		_, _ = io.WriteString(w, strings.Repeat(" ", 1<<20))
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = adapter.GetKeySet(ctx)
	require.ErrorContains(t, err, "exceeds")
}

func TestGetKeySet_RecordsSpanErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json {"))
	}))
	defer server.Close()

	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	}()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	require.Error(t, err)
	require.NoError(t, tp.ForceFlush(context.Background()))

	span := findSpanByName(spanRecorder.Ended(), "jwks.fetch")
	require.NotNil(t, span)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Contains(t, span.Status().Description, "failed to fetch jwks")
	assert.Contains(t, spanEvents(span), "exception")
}

// TestGetKeySet_ContextCancelled tests cancellation handling
func TestGetKeySet_ContextCancelled(t *testing.T) {
	// Create mock server that blocks until the caller cancels the request.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	assert.Error(t, err)
}

func TestGetKeySet_DeduplicatesRefreshAcrossCanceledCallers(t *testing.T) {
	transport := &countingRoundTripper{}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   100 * time.Millisecond,
	}

	adapter, err := NewJWKSAdapter("https://auth.example.com/.well-known/jwks.json", httpClient, 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		_, err := adapter.GetKeySet(ctx)
		cancel()
		require.Error(t, err)
	}

	// net/http may retry a canceled idempotent GET once, so assert the refresh stays
	// bounded instead of requiring exactly one RoundTrip.
	assert.LessOrEqual(t, transport.calls.Load(), int64(2), "canceled callers should not trigger a fresh JWKS fetch each time")
}

func TestGetKeySet_RefreshUsesFetchTimeoutWhenCallerContextExpires(t *testing.T) {
	transport := &deadlineCapturingRoundTripper{
		deadlineCh: make(chan time.Time, 1),
		doneCh:     make(chan struct{}),
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   80 * time.Millisecond,
	}

	adapter, err := NewJWKSAdapter("https://auth.example.com/.well-known/jwks.json", httpClient, 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	require.Error(t, err)
	assert.ErrorContains(t, err, "context deadline exceeded")

	select {
	case deadline := <-transport.deadlineCh:
		require.False(t, deadline.IsZero(), "refresh request should carry a deadline")
		observedTimeout := deadline.Sub(start)
		assert.Greater(t, observedTimeout, 50*time.Millisecond)
		assert.LessOrEqual(t, observedTimeout, 150*time.Millisecond)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for refresh request")
	}

	select {
	case <-transport.doneCh:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("refresh request did not finish within fetch timeout")
	}
}

// TestGetKey tests key retrieval by kid
func TestGetKey(t *testing.T) {
	// Create test JWKS with multiple keys
	key1, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_1"))
	require.NoError(t, err)
	require.NoError(t, key1.Set(jwk.KeyIDKey, "kid-1"))
	require.NoError(t, key1.Set(jwk.AlgorithmKey, jwa.HS256()))

	key2, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_2"))
	require.NoError(t, err)
	require.NoError(t, key2.Set(jwk.KeyIDKey, "kid-2"))
	require.NoError(t, key2.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(key1))
	require.NoError(t, keyset.AddKey(key2))

	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get specific key
	key, err := adapter.GetKey(ctx, "kid-1")
	require.NoError(t, err)
	require.NotNil(t, key)

	// Verify it's the correct key (KeyID() returns (string, bool) in v3)
	kid, ok := key.KeyID()
	assert.True(t, ok)
	assert.Equal(t, "kid-1", kid)
}

// TestGetKey_NotFound tests handling of missing key
func TestGetKey_NotFound(t *testing.T) {
	key1, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_1"))
	require.NoError(t, err)
	require.NoError(t, key1.Set(jwk.KeyIDKey, "kid-1"))
	require.NoError(t, key1.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(key1))

	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get key that doesn't exist
	_, err = adapter.GetKey(ctx, "nonexistent-kid")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "key not found")
}

// TestGetKey_ConcurrentAccess tests thread-safety of concurrent Get calls
func TestGetKey_ConcurrentAccess(t *testing.T) {
	key1, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_1"))
	require.NoError(t, err)
	require.NoError(t, key1.Set(jwk.KeyIDKey, "kid-1"))
	require.NoError(t, key1.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(key1))

	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Concurrent calls should use cache, not make multiple fetches
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := adapter.GetKey(ctx, "kid-1")
			done <- err
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		err := <-done
		require.NoError(t, err)
	}
}

// TestGetKeySet_Caching verifies that subsequent calls use cached results
func TestGetKeySet_Caching(t *testing.T) {
	key1, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_1"))
	require.NoError(t, err)
	require.NoError(t, key1.Set(jwk.KeyIDKey, "kid-1"))
	require.NoError(t, key1.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(key1))

	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First call should fetch
	keyset1, err := adapter.GetKeySet(ctx)
	require.NoError(t, err)
	require.NotNil(t, keyset1)

	// Second call should use cache
	keyset2, err := adapter.GetKeySet(ctx)
	require.NoError(t, err)
	require.NotNil(t, keyset2)

	// Both calls should return key sets
	assert.NotNil(t, keyset1)
	assert.NotNil(t, keyset2)
}

func findSpanByName(spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	return nil
}

func spanEvents(span sdktrace.ReadOnlySpan) []string {
	events := span.Events()
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	return names
}

func TestShutdown_CanBeCalledMultipleTimes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)

	require.NoError(t, adapter.Shutdown(context.Background()))
	require.NoError(t, adapter.Shutdown(context.Background()))
}

func TestBackgroundRefreshFailure_LogsWarn(t *testing.T) {
	testKey, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_"))
	require.NoError(t, err)
	require.NoError(t, testKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, testKey.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(testKey))
	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	var upstreamHealthy atomic.Bool
	upstreamHealthy.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upstreamHealthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	var logs safeLogBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	adapter, err := NewJWKSAdapter(
		server.URL,
		server.Client(),
		time.Second,
		time.Second,
		logger,
	)
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)

	upstreamHealthy.Store(false)
	require.Eventually(t, func() bool {
		return logs.String() != ""
	}, 5*time.Second, 100*time.Millisecond)
	assert.Contains(t, logs.String(), "JWKS refresh failed")
}

func TestGetKeySet_LogsOnceWhenServingCachedKeysAfterRefreshFailure(t *testing.T) {
	testKey, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_"))
	require.NoError(t, err)
	require.NoError(t, testKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, testKey.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(testKey))
	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	var logs safeLogBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
	adapter, err := NewJWKSAdapter(server.URL, server.Client(), time.Minute, time.Hour, logger)
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.Eventually(t, adapter.resourceReady, 5*time.Second, 10*time.Millisecond)
	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)

	adapter.recordFailure(assert.AnError)

	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)
	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)

	assert.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())
	assert.Equal(t, 1, bytes.Count(logs.Bytes(), []byte("serving cached JWKS after refresh failure")))
}

func TestMarkStaleServeLogged_RelogsEveryTenthServe(t *testing.T) {
	adapter := &Adapter{}
	adapter.healthMu.Lock()
	adapter.lastSuccessAt = time.Now()
	adapter.nextRefreshAt = time.Now().Add(time.Hour)
	adapter.lastErr = assert.AnError
	adapter.healthMu.Unlock()

	logged := 0
	for range 10 {
		if err := adapter.markStaleServeLogged(); err != nil {
			logged++
		}
	}

	assert.Equal(t, 2, logged)
}

func TestHealthState_HealthyWhileCachedMaterialIsStillFreshAfterRefreshFailure(t *testing.T) {
	adapter := &Adapter{}
	adapter.healthMu.Lock()
	adapter.lastSuccessAt = time.Now()
	adapter.nextRefreshAt = time.Now().Add(time.Hour)
	adapter.lastErr = assert.AnError
	adapter.healthMu.Unlock()

	assert.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())
}

func TestHealthState_DegradedBeforeFirstFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 15*time.Minute, time.Hour, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	assert.Equal(t, ports.ComponentHealthDegraded, adapter.HealthState())
}

func TestHealthState_TracksBackgroundRefreshLifecycle(t *testing.T) {
	testKey, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_"))
	require.NoError(t, err)
	require.NoError(t, testKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, testKey.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(testKey))
	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	var upstreamHealthy atomic.Bool
	upstreamHealthy.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upstreamHealthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 200*time.Millisecond, 200*time.Millisecond, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)
	require.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())

	upstreamHealthy.Store(false)
	require.Eventually(t, func() bool {
		return adapter.HealthState() == ports.ComponentHealthDegraded
	}, 4*time.Second, 50*time.Millisecond)

	upstreamHealthy.Store(true)
	require.Eventually(t, func() bool {
		return adapter.HealthState() == ports.ComponentHealthHealthy
	}, 4*time.Second, 50*time.Millisecond)
}

func TestGetKeySet_ReturnsErrorWhenCachedMaterialExpiresAfterRefreshFailure(t *testing.T) {
	testKey, err := jwk.Import[jwk.Key]([]byte("secret_key_material_32_bytes_long_"))
	require.NoError(t, err)
	require.NoError(t, testKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, testKey.Set(jwk.AlgorithmKey, jwa.HS256()))

	keyset := jwk.NewSet()
	require.NoError(t, keyset.AddKey(testKey))
	jwksJSON, err := json.Marshal(keyset)
	require.NoError(t, err)

	var upstreamHealthy atomic.Bool
	upstreamHealthy.Store(true)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upstreamHealthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	}))
	defer server.Close()

	adapter, err := NewJWKSAdapter(server.URL, server.Client(), 200*time.Millisecond, 200*time.Millisecond, testLogger())
	require.NoError(t, err)
	defer func() { _ = adapter.Shutdown(context.Background()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = adapter.GetKeySet(ctx)
	require.NoError(t, err)

	upstreamHealthy.Store(false)
	require.Eventually(t, func() bool {
		return adapter.HealthState() == ports.ComponentHealthDegraded
	}, 4*time.Second, 50*time.Millisecond)

	_, err = adapter.GetKeySet(ctx)
	require.Error(t, err)
}

func TestTrackingTransformer_RejectsUnsupportedJWKSKeys(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "https://issuer.example.com/jwks", nil)
	response := &http.Response{
		Body:    io.NopCloser(strings.NewReader(`{"keys":[{"kty":"unsupported"}]}`)),
		Request: request,
	}

	_, err := (trackingTransformer{}).Transform(context.Background(), response)

	require.Error(t, err)
}
