package bootstrap_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/stretchr/testify/require"
)

func TestCIMDServerRejectsIncompleteAuthorizationRequest(t *testing.T) {
	logger := bootstrap.TestLogger(slog.LevelError)
	storageFactory := bootstrap.NewStorageFactory(logger)
	storage, err := storageFactory.NewTestStorage()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storageFactory.CloseStorage(storage) })

	fetcher, err := bootstrap.NewCIMDTestFetcherFromClient(&http.Client{}, 5120)
	if err != nil {
		t.Fatal(err)
	}
	server, err := bootstrap.NewCIMDEndUserTestServer(storage,
		bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithCIMD(""), logger), fetcher, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)

	resp, err := server.AuthenticatedGET("/oauth2/authorize?response_type=code", fixtures.DefaultPrincipal().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("incomplete authorize request: got HTTP %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestNewCIMDEndUserTestServerPreservesHTTPSPublicURL(t *testing.T) {
	const publicURL = "https://broker.e2e.test"

	logger := bootstrap.TestLogger(slog.LevelError)
	storageFactory := bootstrap.NewStorageFactory(logger)
	storage, err := storageFactory.NewTestStorage()
	require.NoError(t, err)
	defer func() { require.NoError(t, storageFactory.CloseStorage(storage)) }()

	config := fixtures.LocalConfig()
	config.Server.EndUser.PublicURL = publicURL
	server, err := bootstrap.NewCIMDEndUserTestServer(storage, bootstrap.NewServerFactory(config, logger), nil, logger)
	require.NoError(t, err)
	defer server.Close()

	require.Equal(t, publicURL, server.App().Config.Server.EndUser.PublicURL)
	brokerClient, err := bootstrap.CIMDUpstreamHTTPClient(server, publicURL)
	require.NoError(t, err)

	response, err := brokerClient.Get(publicURL + "/health")
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusOK, response.StatusCode)
}
