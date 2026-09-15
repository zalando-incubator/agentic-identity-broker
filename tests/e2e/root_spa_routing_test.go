package e2e_test

import (
	"io"
	"log/slog"
	"net/http"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Root-Mounted SPA Routing", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		application, err := bootstrap.NewServerFactory(fixtures.DefaultOAuth2Config(), logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	// Browser route delivery and SPA response security contract from ADRs 035 and 005.
	It("serves canonical browser views for GET and HEAD", func() {
		for _, path := range []string{
			"/",
			"/delegations",
			"/agents/agent-123",
			"/sessions",
			"/approvals",
			"/approvals/approval-123",
		} {
			response, err := server.DirectRequest(http.MethodGet, path, "", nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK), path)
			Expect(response.Body.Close()).To(Succeed())
		}

		getResponse, err := server.DirectRequest(http.MethodGet, "/agents/agent-123", "", nil, nil)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { _ = getResponse.Body.Close() })
		Expect(getResponse.StatusCode).To(Equal(http.StatusOK))

		headResponse, err := server.DirectRequest(http.MethodHead, "/agents/agent-123", "", nil, nil)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { _ = headResponse.Body.Close() })
		Expect(headResponse.StatusCode).To(Equal(http.StatusOK))
		for header, expected := range map[string]string{
			"X-Frame-Options":         "DENY",
			"X-Content-Type-Options":  "nosniff",
			"Content-Security-Policy": "frame-ancestors 'none'",
		} {
			Expect(getResponse.Header.Values(header)).To(Equal([]string{expected}), header)
			Expect(headResponse.Header.Values(header)).To(Equal([]string{expected}), header)
		}
		for _, header := range []string{"Content-Type", "Content-Length", "Last-Modified", "Accept-Ranges"} {
			Expect(headResponse.Header.Values(header)).To(Equal(getResponse.Header.Values(header)), header)
		}
		body, err := io.ReadAll(headResponse.Body)
		Expect(err).ToNot(HaveOccurred())
		Expect(body).To(BeEmpty())
	})

	// Reserved protocol namespace contract from ADR 035.
	It("keeps protocol namespaces outside the SPA route", func() {
		for _, path := range []string{"/api/not-a-route", "/oauth2/not-a-route", "/.well-known/not-a-route", "/health/not-a-route"} {
			response, err := server.DirectRequest(http.MethodGet, path, fixtures.DefaultPrincipal().String(), nil, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound), path)
			Expect(response.Body.Close()).To(Succeed())
		}
	})

	// Unsupported SPA method contract from ADR 035.
	It("does not serve unsupported methods through the SPA route", func() {
		response, err := server.DirectRequest(http.MethodPost, "/agents/agent-123", fixtures.DefaultPrincipal().String(), nil, nil)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() { _ = response.Body.Close() })
		Expect(response.StatusCode).To(Equal(http.StatusMethodNotAllowed))
	})
})
