package oauth2session

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestExchangeCodeWithRetry_UsesConfiguredClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(150 * time.Millisecond):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"late","token_type":"Bearer"}`)
		}
	}))
	defer server.Close()

	service := &OAuth2SessionService{
		httpClient: &http.Client{Timeout: 20 * time.Millisecond},
		config:     Config{MaxRetries: 1},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	cfg := &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	token, err := service.exchangeCodeWithRetry(context.Background(), cfg, "code", "verifier", nil)
	require.Nil(t, token)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestExchangeCodeWithRetry_DoesNotRetryNonTransientResponses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		code   string
	}{
		{name: "invalid grant", status: http.StatusBadRequest, code: "invalid_grant"},
		{name: "invalid client", status: http.StatusUnauthorized, code: "invalid_client"},
		{name: "invalid grant despite server error", status: http.StatusInternalServerError, code: "invalid_grant"},
		{name: "rate limited", status: http.StatusTooManyRequests, code: "slow_down"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"error":"`+tt.code+`"}`)
			}))
			defer server.Close()

			service := &OAuth2SessionService{
				httpClient: server.Client(),
				config:     Config{MaxRetries: 3, RetryBaseDelay: time.Millisecond},
				logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
			cfg := &oauth2.Config{
				ClientID: "client",
				Endpoint: oauth2.Endpoint{
					TokenURL:  server.URL,
					AuthStyle: oauth2.AuthStyleInParams,
				},
			}

			token, err := service.exchangeCodeWithRetry(context.Background(), cfg, "code", "verifier", nil)
			require.Nil(t, token)
			require.ErrorIs(t, err, ErrTokenExchange)
			require.Equal(t, int32(1), requests.Load())
		})
	}
}

func TestExchangeCodeWithRetry_RetriesNetworkFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack first connection: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"retried","token_type":"Bearer"}`)
	}))
	defer server.Close()

	service := &OAuth2SessionService{
		httpClient: server.Client(),
		config:     Config{MaxRetries: 2, RetryBaseDelay: time.Millisecond},
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	cfg := &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{
			TokenURL:  server.URL,
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}

	token, err := service.exchangeCodeWithRetry(context.Background(), cfg, "code", "verifier", nil)
	require.NoError(t, err)
	require.Equal(t, "retried", token.AccessToken)
	require.Equal(t, int32(2), requests.Load())
}
