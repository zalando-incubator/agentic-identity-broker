package server_test

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// Regression for specs/20260223-120000-extproc-token-exchange/spec.md FR-011: a cache TTL taken verbatim
// from expires_in serves a token until the exact second the provider rejects it, so requests
// started shortly before expiry reach the upstream with a dead token and the agent observes a
// 401 even though the broker could have refreshed it.
func TestTokenExchanger_Cache_StopsServingBeforeProviderExpiry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		expiry int
		hitAt  time.Duration
		missAt time.Duration
	}{
		{"minimum margin", 60, 49 * time.Second, 51 * time.Second},
		{"percentage margin", 200, 179 * time.Second, 181 * time.Second},
		{"one-second lifetime", 1, 250 * time.Millisecond, 750 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mocks := newMockServers()
			defer mocks.Close()

			synctest.Test(t, func(t *testing.T) {
				mocks.exchangeExpiry = &tc.expiry

				cfg := configForMocks(mocks)
				exchanger, err := server.NewTokenExchanger(cfg, testLogger())
				require.NoError(t, err)
				defer exchanger.Shutdown()

				const subjectToken = "expiry-margin-token"
				const resourceURI = "http://mcp-server:9003/mcp"

				first, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
				require.NoError(t, err)
				callsAfterFirst := mocks.exchangeCalls
				mocks.exchangedToken = "refreshed-before-expiry"

				time.Sleep(tc.hitAt)
				cached, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
				require.NoError(t, err)
				assert.Equal(t, first.Token, cached.Token)
				assert.Equal(t, callsAfterFirst, mocks.exchangeCalls, "token before the expiry safety margin must be served from cache")

				time.Sleep(tc.missAt - tc.hitAt)
				second, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
				require.NoError(t, err)

				assert.Equal(t, "exchanged-access-token", first.Token)
				assert.Equal(t, "refreshed-before-expiry", second.Token, "a token inside the expiry safety margin must be re-exchanged")
				assert.Equal(t, callsAfterFirst+1, mocks.exchangeCalls, "broker must be called again before the provider expiry")
			})
		})
	}
}

// FR-011 floor: a short-lived token is still cached for half its lifetime instead of being
// re-exchanged on every request.
func TestTokenExchanger_Cache_ShortLivedTokenKeepsHalfItsLifetime(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		expiry := 8
		mocks.exchangeExpiry = &expiry

		exchanger, err := server.NewTokenExchanger(configForMocks(mocks), testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "short-lived-token"
		const resourceURI = "http://mcp-server:9003/mcp"
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		calls := mocks.exchangeCalls

		time.Sleep(3 * time.Second)
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		assert.Equal(t, calls, mocks.exchangeCalls, "token must be served from cache for half its lifetime")

		time.Sleep(1500 * time.Millisecond)
		_, err = exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		assert.Greater(t, mocks.exchangeCalls, calls, "token past half its lifetime must be re-exchanged")
	})
}

func TestTokenExchanger_Cache_DistinguishesUnknownAndExhaustedExpiry(t *testing.T) {
	negative := -1
	for _, tc := range []struct {
		name       string
		expiry     *int
		wantCached bool
	}{
		{name: "unknown", wantCached: true},
		{name: "zero", expiry: new(int)},
		{name: "negative", expiry: &negative},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mocks := newMockServers()
			defer mocks.Close()
			synctest.Test(t, func(t *testing.T) {
				exchanger, err := server.NewTokenExchanger(configForMocks(mocks), testLogger())
				require.NoError(t, err)
				defer exchanger.Shutdown()
				if !tc.wantCached {
					previousExpiry := 1
					mocks.exchangeExpiry = &previousExpiry
					_, err = exchanger.Exchange(context.Background(), "subject", "https://resource.example")
					require.NoError(t, err)
					time.Sleep(750 * time.Millisecond)
				}
				mocks.exchangeExpiry = tc.expiry
				_, err = exchanger.Exchange(context.Background(), "subject", "https://resource.example")
				require.NoError(t, err)
				mocks.exchangedToken = "next-token"
				second, err := exchanger.Exchange(context.Background(), "subject", "https://resource.example")
				require.NoError(t, err)
				if tc.wantCached {
					assert.Equal(t, "exchanged-access-token", second.Token)
				} else {
					assert.Equal(t, "next-token", second.Token)
					mocks.exchangeStatus = http.StatusServiceUnavailable
					_, err = exchanger.Exchange(context.Background(), "subject", "https://resource.example")
					require.Error(t, err, "an exhausted replacement must remove the previous token from outage fallback")
				}
			})
		})
	}
}
