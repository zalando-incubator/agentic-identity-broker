package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenExchanger_AssertionExpiryUsesWallClock(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"assertion","token_type":"Bearer","expires_in":3600}`)
	}))
	defer provider.Close()

	cfg := &extprocconfig.Config{OAuth2: extprocconfig.OAuth2Config{
		ClientCredentialsEndpoint: provider.URL,
		ClientID:                  "client",
		ClientSecret:              "secret",
		ClientAssertionType:       "access_token",
		ExchangeTimeout:           time.Second,
	}}
	exchanger := &TokenExchanger{cfg: cfg, client: provider.Client(), logger: slog.New(slog.DiscardHandler)}
	require.NoError(t, exchanger.refreshClientAssertion())
	state := exchanger.assertion.Load()
	require.NotNil(t, state)
	require.True(t, state.expiresAt.After(time.Now().Add(50*time.Minute)))
	// A suspended monotonic clock must not keep a wall-clock-expired JWT valid.
	assert.Equal(t, state.expiresAt.Round(0), state.expiresAt)
}
