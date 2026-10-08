package e2e_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Protected resource discovery with hosted CIMD", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		provider       *helpers.MockProtectedResourceProvider
		providerClient *http.Client
		scenario       fixtures.ProtectedResourceScenario
		overrides      []fixtures.ProtectedResourceReply
		principal      string
	)

	host := func(rawURL string) string {
		parsed, err := url.Parse(rawURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.Hostname()).NotTo(BeEmpty())
		return parsed.Hostname()
	}

	fixtureReply := func(targetHost, method, path string) fixtures.ProtectedResourceReply {
		for _, reply := range scenario.Replies {
			if reply.Host == targetHost && reply.Method == method && reply.Path == path {
				return reply
			}
		}
		Fail("protected resource fixture has no reply for " + method + " " + targetHost + path)
		return fixtures.ProtectedResourceReply{}
	}

	callRoutes := func() []string {
		calls := provider.Calls()
		routes := make([]string, 0, len(calls))
		for _, call := range calls {
			routes = append(routes, call.Method+" "+call.Host+call.Path)
		}
		return routes
	}

	createService := func(issuer string) map[string]any {
		request := map[string]any{
			"display_name": "Example MCP",
			"discovery": map[string]any{
				"enable_discovery": true,
				"resource_url":     scenario.ResourceURL,
			},
		}
		if issuer != "" {
			request["issuer_uri"] = issuer
		}
		body, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		response, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[map[string]any](response)
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		principal = fixtures.AdminPrincipal().String()
		scenario = fixtures.CIMDOnlyProtectedResource()
		overrides = nil
	})

	JustBeforeEach(func() {
		provider = helpers.NewMockProtectedResourceProvider(scenario)
		for _, reply := range overrides {
			provider.SetReply(reply)
		}
		hosts := []string{host(scenario.ResourceURL)}
		for _, issuer := range scenario.IssuerURLs {
			hosts = append(hosts, host(issuer))
		}
		if scenario.RegistrationURL != "" {
			hosts = append(hosts, host(scenario.RegistrationURL))
		}
		providerClient = bootstrap.NewProtectedResourceHTTPClient(provider.Server, hosts...)
		config := fixtures.CIMDLocalConfig()
		config.ThirdPartyOAuth2.ClientName = scenario.ClientName
		application, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(testStorage, provider.Server, hosts...)
		Expect(err).NotTo(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())

		keyResponse, err := adminServer.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(keyResponse.StatusCode).To(Equal(http.StatusCreated))
		Expect(keyResponse.Body.Close()).To(Succeed())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if providerClient != nil {
			providerClient.CloseIdleConnections()
		}
		if provider != nil {
			provider.Close()
		}
		if testStorage != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	Context("when a resource advertises one path-qualified issuer", func() {
		// US1-S1 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should save the single issuer and its discovered endpoints", Label("protected-resource-discovery"), func() {
			service := createService("")
			resourceHost, issuerHost := host(scenario.ResourceURL), host(scenario.IssuerURLs[0])
			Expect(callRoutes()).To(Equal([]string{
				"GET " + resourceHost + "/mcp",
				"GET " + resourceHost + "/.well-known/oauth-protected-resource/mcp",
				"GET " + issuerHost + "/.well-known/oauth-authorization-server/tenant",
			}))
			Expect(service).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
			Expect(service["endpoints"]).To(And(
				HaveKeyWithValue("authorize_endpoint", scenario.IssuerURLs[0]+"/authorize"),
				HaveKeyWithValue("token_endpoint", scenario.IssuerURLs[0]+"/token"),
			))
		})
	})

	Context("when the issuer supports hosted CIMD and dynamic registration", func() {
		BeforeEach(func() {
			scenario = fixtures.CIMDAndDCRProtectedResource()
		})

		// US1-S2 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should choose hosted CIMD without registering another client", Label("protected-resource-discovery"), func() {
			service := createService("")
			Expect(service["discovery"]).To(HaveKeyWithValue("client_method", "cimd"))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(service).To(HaveKeyWithValue("client_id", fixtures.CIMDEndUserPublicURL+"/.well-known/oauth-client/"+service["id"].(string)))
			Expect(service).NotTo(HaveKey("client_secret"))
			Expect(provider.RegistrationCount()).To(BeZero())
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
			}))
		})

		// US1-S3 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should connect with S256 PKCE and one resource on each request", Label("protected-resource-discovery"), func() {
			service := createService("")
			serviceID := service["id"].(string)
			start, err := enduserServer.AuthenticatedGET(
				"/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/done"),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(start.StatusCode).To(Equal(http.StatusFound))
			authorizeURL := start.Header.Get("Location")
			Expect(start.Body.Close()).To(Succeed())

			providerResponse, err := providerClient.Get(authorizeURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(providerResponse.StatusCode).To(Equal(http.StatusFound))
			callbackURL, err := url.Parse(providerResponse.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(providerResponse.Body.Close()).To(Succeed())
			callback, err := enduserServer.AuthenticatedGET(callbackURL.RequestURI(), fixtures.DefaultPrincipal().String())
			Expect(err).NotTo(HaveOccurred())
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Header.Get("Location")).To(ContainSubstring("success=true"))
			Expect(callback.Body.Close()).To(Succeed())

			calls := provider.Calls()
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
				"GET " + host(scenario.IssuerURLs[0]) + "/tenant/authorize",
				"POST " + host(scenario.IssuerURLs[0]) + "/tenant/token",
			}))
			Expect(calls[3].Query["resource"]).To(Equal([]string{scenario.ResourceURL}))
			Expect(calls[3].Query["code_challenge_method"]).To(Equal([]string{"S256"}))
			Expect(calls[3].PKCEChallengePresent).To(BeTrue())
			Expect(calls[3].PKCE).To(BeTrue())
			Expect(calls[4].Form["resource"]).To(Equal([]string{scenario.ResourceURL}))
			Expect(calls[4].Form["grant_type"]).To(Equal([]string{"authorization_code"}))
			Expect(calls[4].PKCEVerifierPresent).To(BeTrue())
			Expect(calls[4].PKCE).To(BeTrue())
			Expect(calls[4].AuthMethod).To(Equal("private_key_jwt"))
			Expect(provider.RegistrationCount()).To(BeZero())
		})
	})

	Context("when a resource advertises two issuers", func() {
		BeforeEach(func() {
			scenario = fixtures.MultiIssuerProtectedResource()
		})

		// US1-S4 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should discover only the explicitly selected issuer", Label("protected-resource-discovery"), func() {
			selected := scenario.IssuerURLs[1]
			service := createService(selected)
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(selected) + "/.well-known/oauth-authorization-server",
			}))
			Expect(service).To(HaveKeyWithValue("issuer_uri", selected))
			Expect(service["endpoints"]).To(And(
				HaveKeyWithValue("authorize_endpoint", selected+"/authorize"),
				HaveKeyWithValue("token_endpoint", selected+"/token"),
			))
		})
	})

	Context("when a DPoP challenge names root resource metadata", func() {
		BeforeEach(func() {
			scenario = fixtures.CIMDAndDCRProtectedResource()
			resourceHost := host(scenario.ResourceURL)
			challenge := fixtureReply(resourceHost, http.MethodGet, "/mcp")
			challenge.Headers.Set("WWW-Authenticate", `DPoP resource_metadata="https://`+resourceHost+`/.well-known/oauth-protected-resource"`)
			root := fixtureReply(resourceHost, http.MethodGet, "/.well-known/oauth-protected-resource/mcp")
			root.Path = "/.well-known/oauth-protected-resource"
			overrides = []fixtures.ProtectedResourceReply{challenge, root}
		})

		// US1-S5 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should follow the one challenge URL before well-known alternatives", Label("protected-resource-discovery"), func() {
			service := createService("")
			Expect(service).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
			}))
		})
	})

	Context("when a path issuer has both OpenID metadata locations", func() {
		BeforeEach(func() {
			issuerHost := host(scenario.IssuerURLs[0])
			oauth := fixtureReply(issuerHost, http.MethodGet, "/.well-known/oauth-authorization-server/tenant")
			inserted := oauth
			inserted.Path = "/.well-known/openid-configuration/tenant"
			appended := oauth
			appended.Path = "/tenant/.well-known/openid-configuration"
			oauth.Status, oauth.Body = http.StatusNotFound, ""
			overrides = []fixtures.ProtectedResourceReply{oauth, inserted, appended}
		})

		// US1-S6 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should select path-inserted OpenID before path-appended OpenID", Label("protected-resource-discovery"), func() {
			service := createService("")
			Expect(service["endpoints"]).To(HaveKeyWithValue("token_endpoint", scenario.IssuerURLs[0]+"/token"))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/openid-configuration/tenant",
			}))
		})
	})

	Context("when only path-appended OpenID metadata is available", func() {
		BeforeEach(func() {
			issuerHost := host(scenario.IssuerURLs[0])
			oauth := fixtureReply(issuerHost, http.MethodGet, "/.well-known/oauth-authorization-server/tenant")
			appended := oauth
			appended.Path = "/tenant/.well-known/openid-configuration"
			oauth.Status, oauth.Body = http.StatusNotFound, ""
			overrides = []fixtures.ProtectedResourceReply{
				oauth,
				{Host: issuerHost, Method: http.MethodGet, Path: "/.well-known/openid-configuration/tenant", Status: http.StatusNotFound},
				appended,
			}
		})

		// US1-S7 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should use path-appended OpenID after two missing locations", Label("protected-resource-discovery"), func() {
			service := createService("")
			Expect(service["endpoints"]).To(And(
				HaveKeyWithValue("authorize_endpoint", scenario.IssuerURLs[0]+"/authorize"),
				HaveKeyWithValue("token_endpoint", scenario.IssuerURLs[0]+"/token"),
			))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/openid-configuration/tenant",
				"GET " + host(scenario.IssuerURLs[0]) + "/tenant/.well-known/openid-configuration",
			}))
		})
	})

	Context("when a root issuer has no OAuth authorization-server metadata", func() {
		BeforeEach(func() {
			scenario = fixtures.MultiIssuerProtectedResource()
			issuerHost := host(scenario.IssuerURLs[0])
			oauth := fixtureReply(issuerHost, http.MethodGet, "/.well-known/oauth-authorization-server")
			openid := oauth
			openid.Path = "/.well-known/openid-configuration"
			oauth.Status, oauth.Body = http.StatusNotFound, ""
			overrides = []fixtures.ProtectedResourceReply{oauth, openid}
		})

		// US1-S8 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should obtain root issuer endpoints from OpenID metadata", Label("protected-resource-discovery"), func() {
			service := createService(scenario.IssuerURLs[0])
			Expect(service["endpoints"]).To(And(
				HaveKeyWithValue("authorize_endpoint", scenario.IssuerURLs[0]+"/authorize"),
				HaveKeyWithValue("token_endpoint", scenario.IssuerURLs[0]+"/token"),
			))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/openid-configuration",
			}))
		})
	})

	Context("when a path resource has no metadata challenge", func() {
		BeforeEach(func() {
			resourceHost := host(scenario.ResourceURL)
			challenge := fixtureReply(resourceHost, http.MethodGet, "/mcp")
			challenge.Headers.Set("WWW-Authenticate", `Bearer realm="mcp"`)
			root := fixtureReply(resourceHost, http.MethodGet, "/.well-known/oauth-protected-resource/mcp")
			root.Path = "/.well-known/oauth-protected-resource"
			root.Body = `{"resource":"https://wrong.example.test/mcp","authorization_servers":["https://wrong-issuer.example.test"]}`
			overrides = []fixtures.ProtectedResourceReply{challenge, root}
		})

		// US1-S9 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should prefer path-specific metadata over root metadata", Label("protected-resource-discovery"), func() {
			service := createService("")
			Expect(service).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
			}))
		})
	})

	Context("when an administrator reads a discovered service", func() {
		// US1-S10 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should show the verified URL as its effective resource", Label("protected-resource-discovery"), func() {
			created := createService("")
			response, err := adminServer.AuthenticatedGET("/api/services/"+created["id"].(string), principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			read := decodeJSON[map[string]any](response)
			Expect(read["discovery"]).To(HaveKeyWithValue("resource_url", scenario.ResourceURL))
			Expect(read["authorization_params"]).To(HaveKeyWithValue("resource", scenario.ResourceURL))
			Expect(callRoutes()).To(Equal([]string{
				"GET " + host(scenario.ResourceURL) + "/mcp",
				"GET " + host(scenario.ResourceURL) + "/.well-known/oauth-protected-resource/mcp",
				"GET " + host(scenario.IssuerURLs[0]) + "/.well-known/oauth-authorization-server/tenant",
			}))
		})
	})
})
