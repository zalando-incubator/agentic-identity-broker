package oauth2session

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestRefreshErrorBodyTimeoutIsNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, provider, _ := newOperationTestService(t, io.Discard)
				service.httpClient.Timeout = time.Second
				attempts := 0
				service.httpClient.Transport = operationTransport(func(req *http.Request) (*http.Response, error) {
					attempts++
					body := operationReadFunc(func([]byte) (int, error) {
						<-req.Context().Done()
						return 0, req.Context().Err()
					})
					return &http.Response{StatusCode: status, Body: io.NopCloser(body)}, nil
				})
				ctx := context.Background()
				start := time.Now()
				_, err := service.RefreshAccessToken(ctx, provider, "refresh", nil)
				requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
				assert.ErrorIs(t, err, context.DeadlineExceeded)
				assert.NoError(t, ctx.Err())
				assert.Equal(t, 1, attempts)
				assert.Equal(t, time.Second, time.Since(start))
			})
		})
	}
}

func TestRefreshIncompleteErrorResponseIsNotRetried(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, provider, _ := newOperationTestService(t, io.Discard)
				attempts := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					attempts++
					return &http.Response{StatusCode: status, Body: io.NopCloser(operationFailingReader{cause: io.ErrUnexpectedEOF})}, nil
				})
				_, err := service.RefreshAccessToken(context.Background(), provider, "refresh", nil)
				metadata := requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
				assert.Equal(t, status, metadata.StatusCode())
				assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
				assert.Equal(t, 1, attempts)
			})
		})
	}
}

func TestRefreshErrorBodyLimitControlsRetries(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		bodyBytes    int
		wantAttempts int
	}{
		{"429 at limit", http.StatusTooManyRequests, maxThirdpartyErrorBodyBytes, 3},
		{"429 over limit", http.StatusTooManyRequests, maxThirdpartyErrorBodyBytes + 1, 1},
		{"503 at limit", http.StatusServiceUnavailable, maxThirdpartyErrorBodyBytes, 3},
		{"503 over limit", http.StatusServiceUnavailable, maxThirdpartyErrorBodyBytes + 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, provider, _ := newOperationTestService(t, io.Discard)
				payload := `{"error":"temporarily_unavailable"}`
				body := payload + strings.Repeat(" ", tc.bodyBytes-len(payload))
				attempts := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					attempts++
					return operationTokenResponse(tc.status, body), nil
				})
				_, err := service.RefreshAccessToken(context.Background(), provider, "refresh", nil)
				metadata := requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
				assert.Equal(t, tc.status, metadata.StatusCode())
				assert.Equal(t, tc.wantAttempts, attempts)
			})
		})
	}
}

func TestRefreshRetryScheduling(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryAfter string
		budget     time.Duration
		want       []time.Duration
	}{
		{name: "exponential backoff", want: []time.Duration{0, time.Second, 3 * time.Second}},
		{name: "positive Retry-After", retryAfter: "2", want: []time.Duration{0, 2 * time.Second, 4 * time.Second}},
		{name: "Retry-After cutoff is inclusive", retryAfter: "10", want: []time.Duration{0, 10 * time.Second, 20 * time.Second}},
		{name: "Retry-After above cutoff", retryAfter: "11", want: []time.Duration{0}},
		{name: "Retry-After duration overflow", retryAfter: "9223372037", want: []time.Duration{0}},
		{name: "Retry-After integer overflow", retryAfter: "9223372036854775808", want: []time.Duration{0}},
		{name: "insufficient backoff budget", budget: 500 * time.Millisecond, want: []time.Duration{0}},
		{name: "insufficient Retry-After budget", retryAfter: "2", budget: 1500 * time.Millisecond, want: []time.Duration{0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, provider, _ := newOperationTestService(t, io.Discard)
				service.config.RetryBaseDelay = time.Second
				start := time.Now()
				var attempts []time.Duration
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					attempts = append(attempts, time.Since(start))
					response := operationTokenResponse(http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`)
					response.Header = http.Header{"Retry-After": []string{tc.retryAfter}}
					return response, nil
				})
				ctx := context.Background()
				if tc.budget > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.budget)
					defer cancel()
				}
				_, err := service.RefreshAccessToken(ctx, provider, "refresh", nil)
				requireOperationError(t, err, OperationRefresh, DetailProviderUnavailable)
				assert.Equal(t, tc.want, attempts)
				assert.Equal(t, tc.want[len(tc.want)-1], time.Since(start))
				assert.NoError(t, ctx.Err())
			})
		})
	}
}

func TestRefreshBackoffCancellationPreservesProviderError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, provider, _ := newOperationTestService(t, io.Discard)
		service.config.RetryBaseDelay = time.Second
		attempts := 0
		service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
			attempts++
			return operationTokenResponse(http.StatusServiceUnavailable, `{"error":"invalid_request"}`), nil
		})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		finished := make(chan error, 1)
		go func() {
			_, err := service.RefreshAccessToken(ctx, provider, "refresh", nil)
			finished <- err
		}()
		synctest.Wait()
		require.Equal(t, 1, attempts)
		cancel()
		err := <-finished
		metadata := requireOperationError(t, err, OperationRefresh, DetailCallerCanceled)
		assert.Equal(t, KindCanceled, metadata.Kind())
		assert.Equal(t, DependencyProvider, metadata.Dependency())
		assert.Equal(t, http.StatusServiceUnavailable, metadata.StatusCode())
		assert.Equal(t, "invalid_request", metadata.OAuthCode())
		assert.ErrorIs(t, err, context.Canceled)
		var providerError *oauth2.RetrieveError
		require.ErrorAs(t, err, &providerError)
		assert.Equal(t, http.StatusServiceUnavailable, providerError.Response.StatusCode)
		assert.Equal(t, "invalid_request", providerError.ErrorCode)
		assert.Equal(t, 1, attempts)
	})
}

func TestMalformedRefreshSuccessIsNotRetried(t *testing.T) {
	for _, body := range []string{"invalid-json", `{}`} {
		t.Run(body, func(t *testing.T) {
			service, provider, _ := newOperationTestService(t, io.Discard)
			attempts := 0
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				attempts++
				if attempts > 1 {
					return operationTokenResponse(http.StatusOK, `{"access_token":"unexpected-retry-success"}`), nil
				}
				return operationTokenResponse(http.StatusOK, body), nil
			})
			_, err := service.RefreshAccessToken(context.Background(), provider, "refresh", nil)
			metadata := requireOperationError(t, err, OperationRefresh, DetailProviderResponseInvalid)
			assert.Equal(t, http.StatusOK, metadata.StatusCode())
			assert.Equal(t, 1, attempts)
		})
	}
}
