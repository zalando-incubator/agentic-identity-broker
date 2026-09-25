package bootstrap_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
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
