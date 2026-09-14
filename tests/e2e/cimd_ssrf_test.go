package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD SSRF Protection", func() {
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

	// Scenario US2.1 from specs/028-cimd-support/spec.md
	Describe("when client_id resolves to a private RFC 1918 address", func() {
		It("rejects the request before establishing any outbound connection", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			// Use production fetcher (has SSRF control) — no injected test client
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "RFC1918 Agent",
				Description:    "E2E test agent for private-IP SSRF scenario",
				ClientURIs:     []string{"https://10.0.0.1/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://10.0.0.1/client&redirect_uri=https://10.0.0.1/callback&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.2 from specs/028-cimd-support/spec.md
	Describe("when client_id resolves to a loopback address", func() {
		It("rejects the request before establishing any outbound connection", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Loopback Agent",
				Description:    "E2E test agent for loopback SSRF scenario",
				ClientURIs:     []string{"https://127.0.0.1/client"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://127.0.0.1/client&redirect_uri=https://127.0.0.1/callback&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.3 from specs/028-cimd-support/spec.md
	Describe("when client_id resolves to a link-local address (cloud metadata endpoint)", func() {
		It("rejects the request before establishing any outbound connection", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Link-Local Agent",
				Description:    "E2E test agent for link-local SSRF scenario",
				ClientURIs:     []string{"https://169.254.169.254/metadata"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://169.254.169.254/metadata&redirect_uri=https://169.254.169.254/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.4 from specs/028-cimd-support/spec.md
	Describe("when client_id uses an HTTP (non-HTTPS) scheme", func() {
		It("rejects the request immediately without any outbound connection", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			// http:// is detected as a URL-format client_id (contains "://"), but fails
			// ValidateCIMDClientURL → falls through to opaque path → UUID parse fails → invalid_client
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=http://example.com/client&redirect_uri=http://example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.5 from specs/028-cimd-support/spec.md
	Describe("when client_id URL contains dot segments", func() {
		It("rejects the request without any outbound connection", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://example.com/./client&redirect_uri=https://example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			// URL format validation failures return invalid_client (falls through to opaque path)
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.6 from specs/028-cimd-support/spec.md
	Describe("when the CIMD endpoint streams a response larger than the size limit", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-oversized.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Write 6000 bytes — exceeds the 5120-byte limit
				_, _ = w.Write(make([]byte, 6000))
			}))

			clientURL = "https://" + fakeHost + "/client"
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Oversized CIMD Agent",
				Description:    "E2E test agent for oversized CIMD response scenario",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		It("aborts the download and rejects the authorization request with invalid_client", func() {
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/cb&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})

	// Scenario US2.7 from specs/028-cimd-support/spec.md
	Describe("when the CIMD endpoint does not respond within the configured timeout", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-timeout.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Hang until the request context is canceled
				<-r.Context().Done()
			}))

			clientURL = "https://" + fakeHost + "/client"
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Timeout CIMD Agent",
				Description:    "E2E test agent for CIMD timeout scenario",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			// Very short fetch timeout to trigger the timeout scenario quickly
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			config.OAuth2AuthServer.CIMD.FetchTimeout = 50 * time.Millisecond
			serverFactory = bootstrap.NewServerFactory(config, logger)

			// Set a short timeout on the injected client so the CIMD fetch times out
			// quickly (in ~50ms), allowing the handler to return 400 well within the
			// test client's timeout window.
			tlsClient := bootstrap.CIMDTestHTTPClient(cimdServer, fakeHost)
			tlsClient.Timeout = 100 * time.Millisecond
			cimdFetcher, err := bootstrap.NewCIMDTestFetcherFromClient(tlsClient, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		It("terminates the connection and rejects the authorization request with invalid_client", func() {
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/cb&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(Equal("invalid_client"))
		})
	})
})
