package e2e_test

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var serviceAccountBackends = []string{"memory", "postgres"}

func serviceAccountAdminServer(st *storageadapter.Adapter) *bootstrap.TestServer {
	logger := bootstrap.TestLogger(slog.LevelWarn)
	application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(st)
	Expect(err).To(Succeed())
	server, err := bootstrap.NewAdminTestServer(application, logger)
	Expect(err).To(Succeed())
	return server
}

// withServiceAccountBackend runs action against a fresh admin server on the named store. The
// postgres case uses a migrated PostgreSQL container and needs a container runtime.
func withServiceAccountBackend(backend string, action func(*bootstrap.TestServer, *storageadapter.Adapter)) {
	By("running on the " + backend + " store")
	ctx := context.Background()
	var st *storageadapter.Adapter
	if backend == "postgres" {
		fixture, err := bootstrap.NewPostgresFixture(ctx)
		Expect(err).To(Succeed())
		defer func() { Expect(fixture.Close(ctx)).To(Succeed()) }()
		cfg := ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: fixture.ConnectionURL}, Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}}
		st, err = storageadapter.NewAdapter(&cfg, slog.New(slog.DiscardHandler))
		Expect(err).To(Succeed())
	} else {
		var err error
		st, err = bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelWarn)).NewTestStorage()
		Expect(err).To(Succeed())
	}
	defer func() { Expect(st.Close(ctx)).To(Succeed()) }()
	server := serviceAccountAdminServer(st)
	defer server.Close()
	action(server, st)
}

func seedServiceAccountService(st *storageadapter.Adapter, canonicalID string) *model.ThirdpartyOAuth2ProviderEntity {
	service := fixtures.NamedPlatformService(canonicalID)
	Expect(st.Services().Create(context.Background(), service)).To(Succeed())
	return service
}

func serviceAccountCall(server *bootstrap.TestServer, method, path, ifMatch string, body any) helpers.CapturedResponse {
	response, err := helpers.ServiceAccountRequest(server, method, path, fixtures.AdminPrincipal().String(), ifMatch, body)
	Expect(err).To(Succeed())
	return response
}

func createServiceAccount(server *bootstrap.TestServer, serviceRef, canonicalID string, scopes []string, jwt string) helpers.ServiceAccount {
	response := serviceAccountCall(server, http.MethodPost, helpers.ServiceAccountsPath(serviceRef), "", fixtures.ServiceAccountCreateBody(canonicalID, scopes, jwt))
	Expect(response.StatusCode).To(Equal(http.StatusCreated), string(response.Body))
	account, err := response.ServiceAccount()
	Expect(err).To(Succeed())
	return account
}

func listServiceAccounts(server *bootstrap.TestServer, serviceRef string) []helpers.ServiceAccount {
	response := serviceAccountCall(server, http.MethodGet, helpers.ServiceAccountsPath(serviceRef), "", nil)
	Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
	accounts, err := response.ServiceAccounts()
	Expect(err).To(Succeed())
	return accounts
}

func expectValidationFailure(response helpers.CapturedResponse, message string) {
	Expect(response.StatusCode).To(Equal(http.StatusBadRequest), string(response.Body))
	envelope, err := response.Error()
	Expect(err).To(Succeed())
	Expect(envelope).To(Equal(helpers.ServiceAccountError{Error: "validation failed", Message: message}))
}

func expectNoJWTMaterial(jwt string, responses ...helpers.CapturedResponse) {
	for _, response := range responses {
		Expect(response).NotTo(helpers.ContainJWTMaterial(jwt))
	}
}
