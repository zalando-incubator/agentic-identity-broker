package e2e_test

import (
	"encoding/json"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD Authorization Server Metadata", func() {
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		testStorage    *storageadapter.Adapter
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US4.1 from specs/20260421-120000-cimd-support/spec.md
	// Given CIMD is enabled in configuration,
	// When the metadata endpoint is queried,
	// Then the response includes client_id_metadata_document_supported: true.
	Describe("when CIMD is enabled", func() {
		It("includes client_id_metadata_document_supported: true in metadata", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			Expect(body).To(HaveKey("client_id_metadata_document_supported"))
			Expect(body["client_id_metadata_document_supported"]).To(BeTrue())
		})
	})

	// Scenario US4.2 from specs/20260421-120000-cimd-support/spec.md
	// Given CIMD is disabled in configuration,
	// When the metadata endpoint is queried,
	// Then client_id_metadata_document_supported is absent from the response.
	Describe("when CIMD is disabled", func() {
		It("omits client_id_metadata_document_supported from metadata", func() {
			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
			// CIMD disabled by default (Enabled: false)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			_, hasField := body["client_id_metadata_document_supported"]
			Expect(hasField).To(BeFalse(), "client_id_metadata_document_supported must be absent when CIMD is disabled")
		})
	})
})
