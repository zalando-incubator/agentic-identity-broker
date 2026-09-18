package server

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExchanger_DoExchangeUsesRequestLogger(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer broker.Close()

	var requestLogs, fallbackLogs bytes.Buffer
	requestLogger := slog.New(slog.NewTextHandler(&requestLogs, nil)).With(
		"trace_id", "0123456789abcdef0123456789abcdef",
		"actor", "anonymous",
	)
	exchanger := &TokenExchanger{
		cfg: &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint:   broker.URL,
			ExchangeTimeout: time.Second,
		}},
		client: broker.Client(),
		logger: slog.New(slog.NewTextHandler(&fallbackLogs, nil)),
	}
	exchanger.assertion.Store(&assertionState{value: "assertion", expiresAt: time.Now().Add(time.Hour)})
	ctx := context.WithValue(context.Background(), requestLoggerKey{}, requestLogger)

	_, _, err := exchanger.doExchange(ctx, "subject-token", "https://api.example.com/resource")

	require.Error(t, err)
	assert.Contains(t, requestLogs.String(), "token exchange returned broker error")
	assert.Contains(t, requestLogs.String(), "trace_id=0123456789abcdef0123456789abcdef")
	assert.Contains(t, requestLogs.String(), "actor=anonymous")
	assert.Empty(t, fallbackLogs.String())
}
