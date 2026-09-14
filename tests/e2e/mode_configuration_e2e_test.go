package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// This test validates behavioral differences across the three operational modes:
// - Proxy mode: delegates token issuance to upstream, republishes upstream JWKS at broker endpoint
// - local mode (no CIMD): mints tokens locally, agents resolved by client_id string
// - local mode (with CIMD): mints tokens locally, agents resolved by URL-based client_id via CIMD fetch

var _ = Describe("Mode Configuration: Proxy vs Local vs Local+CIMD", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
	})

	Describe("Proxy mode", func() {
		var (
			testStorage  *storageadapter.Adapter
			mockUpstream *helpers.MockUpstreamOAuth2Server
			server       *bootstrap.TestServer
		)

		BeforeEach(func() {
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()
			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)

			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if mockUpstream != nil {
				mockUpstream.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// Scenario 1.2 from specs/032-aggregated-jwks/spec.md
		It("exposes aggregated JWKS endpoint in proxy mode", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))
		})

		// Scenario 2.2 from specs/032-aggregated-jwks/spec.md
		It("advertises jwks_uri in discovery metadata in proxy mode", func() {
			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
			Expect(body).To(HaveKey("jwks_uri"))
		})

		It("redirects authorization to consent page (proxy mode)", func() {
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientID:       ptr.To(id.ClientID("test-proxy-agent")),
				DisplayName:    "Proxy Agent",
				Description:    "Test agent for proxy mode",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", agent.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Without an existing grant, proxy mode redirects to consent
			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/" + agent.ID.String()))
		})

		It("rejects URL-format client_id since CIMD is not available in proxy mode", func() {
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://agent.example.com/client&redirect_uri=https://example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	Describe("local mode (no CIMD)", func() {
		var (
			testStorage *storageadapter.Adapter
			server      *bootstrap.TestServer
		)

		BeforeEach(func() {
			config := fixtures.LocalConfig()

			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
			DeferCleanup(adminSrv.Close)
			Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		It("exposes JWKS endpoint with signing keys", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			keys := jwks["keys"].([]interface{})
			Expect(len(keys)).To(BeNumerically(">=", 1))
		})

		It("exposes discovery metadata with token endpoint", func() {
			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
			Expect(body).To(HaveKey("token_endpoint"))
			Expect(body).To(HaveKey("jwks_uri"))
		})

		It("redirects authorization to local consent page", func() {
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Local Agent",
				Description:    "Test agent for local mode",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", agent.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/" + agent.ID.String()))
		})

		It("rejects URL-format client_id when CIMD is disabled", func() {
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://agent.example.com/client&redirect_uri=https://example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	Describe("local mode (with CIMD)", func() {
		var (
			testStorage *storageadapter.Adapter
			cimdServer  *httptest.Server
			server      *bootstrap.TestServer
			clientURL   string
			redirectURI string
		)

		const fakeHost = "cimd-mode-test.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "max-age=300")
				w.WriteHeader(http.StatusOK)
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "CIMD Mode Test Agent",
					"redirect_uris": []string{redirectURI},
				}
				_ = json.NewEncoder(w).Encode(doc)
			}))

			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			config := fixtures.OAuth2ConfigWithCIMD("http://unused-upstream.invalid")
			serverFactory := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			app, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if cimdServer != nil {
				cimdServer.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		It("exposes JWKS endpoint (local mode feature)", func() {
			resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("exposes discovery metadata (local mode feature)", func() {
			resp, err := http.Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("resolves URL-format client_id via CIMD fetch", func() {
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "CIMD Agent",
				Description:    "Agent with URL client_id",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
					url.QueryEscape(clientURL), url.QueryEscape(redirectURI),
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// CIMD-enabled: resolves agent and redirects to consent
			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/" + agent.ID.String()))
		})

		It("still resolves plain string client_id for registered agents", func() {
			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Plain Agent",
				Description:    "Local-mode agent in CIMD mode, resolved by UUID",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", agent.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/" + agent.ID.String()))
		})
	})
})
