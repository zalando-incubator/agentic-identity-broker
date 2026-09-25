package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US1: Client Credential Management (local mode)", func() {
	var (
		adminServer    *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		agent = fixtures.ValidAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 1.1 from specs/025-oauth2-server/spec.md
	It("generates credentials for an agent", func() {
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("client_id"))
		Expect(body).To(HaveKey("client_secret"))
		Expect(body).To(HaveKey("created_at"))
	})

	// Scenario 1.2 from specs/025-oauth2-server/spec.md
	It("rotates existing credentials", func() {
		resp, _ := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		_ = resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("client_id"))
		Expect(body).To(HaveKey("client_secret"))
	})

	// Scenario 1.3 from specs/025-oauth2-server/spec.md
	It("returns 404 for missing agent", func() {
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/00000000-0000-0000-0000-000000000099/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	// Scenario 1.4 from specs/025-oauth2-server/spec.md
	It("gets credential metadata without secret", func() {
		resp, _ := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		_ = resp.Body.Close()

		resp, err := helpers.HTTPClient().Get(
			adminServer.BaseURL() + "/api/agents/" + agent.ID.String() + "/client-credentials",
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("client_id"))
		Expect(body).NotTo(HaveKey("client_secret"))
	})
})
