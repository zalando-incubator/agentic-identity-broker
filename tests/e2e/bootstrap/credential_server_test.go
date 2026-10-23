package bootstrap

import (
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/stretchr/testify/require"
)

func TestCredentialServerKeepsPortsReservedUntilClose(t *testing.T) {
	document := fixtures.CredentialConfigurationDocument("http://127.0.0.1:19000", map[string]any{}, 8000, 14000)
	document["oauth2_authorization_server"] = map[string]any{"mode": "local"}
	delete(document, "token_exchange")
	server, err := prepareCredentialServer(CredentialServerOptions{
		Directory: t.TempDir(),
		Document:  document,
		Principal: fixtures.DefaultPrincipal().String(),
	})
	require.NoError(t, err)
	t.Cleanup(server.Close)

	addresses := make([]string, 0, 2)
	for _, baseURL := range []string{server.AdminURL, server.EndUserURL} {
		parsed, err := url.Parse(baseURL)
		require.NoError(t, err)
		addresses = append(addresses, parsed.Host)
		listener, err := net.Listen("tcp", parsed.Host)
		if listener != nil {
			require.NoError(t, listener.Close())
		}
		require.Error(t, err, "another server must not acquire a prepared credential server's port")
	}

	cfg, err := LoadCredentialConfiguration(server.ConfigPath, nil, nil)
	require.NoError(t, err)
	require.NoError(t, server.start(cfg))
	for _, baseURL := range []string{server.AdminURL, server.EndUserURL} {
		response, err := server.client.Get(baseURL + "/health")
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode)
	}

	server.Close()
	for _, address := range addresses {
		listener, err := net.Listen("tcp", address)
		require.NoError(t, err, "closing the credential server must release both ports")
		require.NoError(t, listener.Close())
	}
}
