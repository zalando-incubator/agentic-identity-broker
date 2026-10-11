package server_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/server"
)

// Regression for specs/015-extproc-token-exchange/spec.md FR-011: a cache TTL taken verbatim
// from expires_in serves a token until the exact second the provider rejects it, so requests
// started shortly before expiry reach the upstream with a dead token and the agent observes a
// 401 even though the broker could have refreshed it.
func TestTokenExchanger_Cache_StopsServingBeforeProviderExpiry(t *testing.T) {
	mocks := newMockServers()
	defer mocks.Close()

	synctest.Test(t, func(t *testing.T) {
		expiry := 60
		mocks.exchangeExpiry = &expiry

		cfg := configForMocks(mocks)
		exchanger, err := server.NewTokenExchanger(cfg, testLogger())
		require.NoError(t, err)
		defer exchanger.Shutdown()

		const subjectToken = "expiry-margin-token"
		const resourceURI = "http://mcp-server:9003/mcp"

		first, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)
		callsAfterFirst := mocks.exchangeCalls

		// Ten seconds before the provider expiry the cached token is no longer safe to hand out.
		time.Sleep(51 * time.Second)

		mocks.exchangedToken = "refreshed-before-expiry"
		second, err := exchanger.Exchange(context.Background(), subjectToken, resourceURI)
		require.NoError(t, err)

		assert.Equal(t, "exchanged-access-token", first.Token)
		assert.Equal(t, "refreshed-before-expiry", second.Token, "a token inside the expiry safety margin must be re-exchanged")
		assert.Greater(t, mocks.exchangeCalls, callsAfterFirst, "broker must be called again before the provider expiry")
	})
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
