package e2e_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US032: Discovery and JWKS", func() {
	var (
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
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
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 6.1 from specs/025-oauth2-server/spec.md
	It("discovery endpoint returns metadata in local mode", func() {
		resp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/.well-known/oauth-authorization-server")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("issuer"))
		Expect(body).To(HaveKey("authorization_endpoint"))
		Expect(body).To(HaveKey("token_endpoint"))
		Expect(body).To(HaveKey("jwks_uri"))
		Expect(body).To(HaveKey("response_types_supported"))
		Expect(body).To(HaveKey("grant_types_supported"))
		Expect(body).To(HaveKey("code_challenge_methods_supported"))
	})

	// Scenario 6.2 from specs/025-oauth2-server/spec.md
	It("JWKS endpoint returns valid key set", func() {
		resp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resp.Header.Get("Cache-Control")).To(ContainSubstring("max-age=300"))

		var jwks map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
		Expect(jwks).To(HaveKey("keys"))

		keys := jwks["keys"].([]interface{})
		Expect(len(keys)).To(BeNumerically(">=", 1))

		// Verify no private key material
		for _, k := range keys {
			keyMap := k.(map[string]interface{})
			Expect(keyMap).ToNot(HaveKey("d"))
		}
	})

	Context("in proxy mode", func() {
		var (
			mockUpstream *helpers.MockUpstreamOAuth2Server
			proxyServer  *bootstrap.TestServer
			proxyStorage *storageadapter.Adapter
		)

		BeforeEach(func() {
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()
			proxyConfig := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())
			var err error
			proxyStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			proxyFactory := bootstrap.NewServerFactory(proxyConfig, logger)
			proxyApp, err := proxyFactory.BuildApp(proxyStorage)
			Expect(err).ToNot(HaveOccurred())

			proxyServer, err = bootstrap.NewEndUserTestServer(proxyApp, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if proxyServer != nil {
				proxyServer.Close()
			}
			if proxyStorage != nil {
				_ = storageFactory.CloseStorage(proxyStorage)
			}
			if mockUpstream != nil {
				mockUpstream.Close()
			}
		})

		// Scenario 2.4 from specs/032-aggregated-jwks/spec.md
		It("serves aggregated JWKS and advertises jwks_uri in discovery", func() {
			resp, err := helpers.HTTPClient().Get(proxyServer.BaseURL() + "/oauth2/jwks.json")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var jwks map[string]interface{}
			Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
			Expect(jwks).To(HaveKey("keys"))

			discResp, err := helpers.HTTPClient().Get(proxyServer.BaseURL() + "/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = discResp.Body.Close() }()
			Expect(discResp.StatusCode).To(Equal(http.StatusOK))

			var discBody map[string]interface{}
			Expect(json.NewDecoder(discResp.Body).Decode(&discBody)).ToNot(HaveOccurred())
			Expect(discBody).To(HaveKey("jwks_uri"))
		})
	})
})
