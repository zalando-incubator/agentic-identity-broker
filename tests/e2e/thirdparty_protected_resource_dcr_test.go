package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func dcrDiscoveryRequest(scenario fixtures.ProtectedResourceScenario, displayName, resource string) map[string]any {
	request := map[string]any{
		"display_name": displayName,
		"discovery": map[string]any{
			"enable_discovery": true,
			"resource_url":     scenario.ResourceURL,
		},
	}
	if resource != "" {
		request["authorization_params"] = map[string]any{"resource": resource}
	}
	return request
}

func dcrPostService(server *bootstrap.TestServer, principal string, request map[string]any) map[string]any {
	encoded, err := json.Marshal(request)
	Expect(err).NotTo(HaveOccurred())
	response, err := server.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(encoded))
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusCreated))
	service := decodeJSON[map[string]any](response)
	Expect(service).NotTo(HaveKey("client_secret"))
	return service
}

func dcrReadService(server *bootstrap.TestServer, principal, serviceID string) map[string]any {
	response, err := server.AuthenticatedGET("/api/services/"+serviceID, principal)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	service := decodeJSON[map[string]any](response)
	Expect(service).NotTo(HaveKey("client_secret"))
	return service
}

func dcrConnect(server *bootstrap.TestServer, client *http.Client, principal, serviceID string) {
	response, err := server.AuthenticatedGET(
		"/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/done"),
		principal,
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	location := response.Header.Get("Location")
	Expect(response.Body.Close()).To(Succeed())
	Expect(location).NotTo(BeEmpty())

	providerResponse, err := client.Get(location)
	Expect(err).NotTo(HaveOccurred())
	Expect(providerResponse.StatusCode).To(Equal(http.StatusFound))
	callback, err := url.Parse(providerResponse.Header.Get("Location"))
	Expect(err).NotTo(HaveOccurred())
	Expect(providerResponse.Body.Close()).To(Succeed())
	Expect(callback.Path).To(Equal("/api/third-party/" + serviceID + "/oauth2/callback"))

	response, err = server.AuthenticatedGET(callback.RequestURI(), principal)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	Expect(response.Body.Close()).To(Succeed())
}

func dcrExpireSession(storage *storageadapter.Adapter, principal, serviceID string) {
	ctx := context.Background()
	parsedID := id.MustParseServiceID(serviceID)
	session, err := storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), parsedID)
	Expect(err).NotTo(HaveOccurred())
	Expect(session).NotTo(BeNil())
	provider, err := storage.Services().Get(ctx, parsedID)
	Expect(err).NotTo(HaveOccurred())
	session.ExpectedIssuerURI = provider.IssuerURI
	expired := time.Now().Add(-time.Minute)
	session.AccessTokenExpiresAt = &expired
	Expect(storage.UserSessions().Create(ctx, session)).To(Succeed())
}

func dcrRefresh(server *bootstrap.TestServer, principal, serviceID string) {
	response, err := server.AuthenticatedPOST("/api/third-party/"+serviceID+"/session/refresh", principal, "application/json", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	Expect(response.Body.Close()).To(Succeed())
}

func dcrCallsForGrant(provider *helpers.MockProtectedResourceProvider, grant string) []helpers.ProtectedResourceRequest {
	var calls []helpers.ProtectedResourceRequest
	for _, call := range provider.Calls() {
		if call.Form.Get("grant_type") == grant {
			calls = append(calls, call)
		}
	}
	return calls
}

func dcrAuthorizeCalls(provider *helpers.MockProtectedResourceProvider) []helpers.ProtectedResourceRequest {
	var calls []helpers.ProtectedResourceRequest
	for _, call := range provider.Calls() {
		if call.Query.Get("response_type") == "code" {
			calls = append(calls, call)
		}
	}
	return calls
}

func dcrPostOnlyScenario() fixtures.ProtectedResourceScenario {
	scenario := fixtures.ConfidentialDCRProtectedResource()
	for i := range scenario.Replies {
		reply := &scenario.Replies[i]
		if reply.Path != "/.well-known/oauth-authorization-server" && reply.Path != "/register" {
			continue
		}
		var document map[string]any
		Expect(json.Unmarshal([]byte(reply.Body), &document)).To(Succeed())
		if reply.Path == "/register" {
			document["token_endpoint_auth_method"] = "client_secret_post"
		} else {
			document["token_endpoint_auth_methods_supported"] = []string{"client_secret_post"}
		}
		encoded, err := json.Marshal(document)
		Expect(err).NotTo(HaveOccurred())
		reply.Body = string(encoded)
	}
	return scenario
}

func dcrSecondIssuerScenario() fixtures.ProtectedResourceScenario {
	scenario := fixtures.ConfidentialDCRProtectedResource()
	const firstResource, secondResource = "files.example.test", "files-two.example.test"
	const firstIssuer, secondIssuer = "login.example.test", "login-two.example.test"
	scenario.ResourceURL = strings.ReplaceAll(scenario.ResourceURL, firstResource, secondResource)
	scenario.IssuerURLs[0] = strings.ReplaceAll(scenario.IssuerURLs[0], firstIssuer, secondIssuer)
	scenario.RegistrationURL = strings.ReplaceAll(scenario.RegistrationURL, firstIssuer, secondIssuer)
	for i := range scenario.Replies {
		reply := &scenario.Replies[i]
		reply.Host = strings.ReplaceAll(strings.ReplaceAll(reply.Host, firstResource, secondResource), firstIssuer, secondIssuer)
		reply.Body = strings.ReplaceAll(strings.ReplaceAll(reply.Body, firstResource, secondResource), firstIssuer, secondIssuer)
		for header, values := range reply.Headers {
			for j := range values {
				reply.Headers[header][j] = strings.ReplaceAll(strings.ReplaceAll(values[j], firstResource, secondResource), firstIssuer, secondIssuer)
			}
		}
	}
	return scenario
}

func dcrAllowedHosts(scenario fixtures.ProtectedResourceScenario) []string {
	locations := append([]string{scenario.ResourceURL, scenario.RegistrationURL}, scenario.IssuerURLs...)
	hosts := make([]string, 0, len(locations))
	for _, location := range locations {
		if location == "" {
			continue
		}
		parsed, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		hosts = append(hosts, parsed.Hostname())
	}
	return hosts
}

func dcrPostgresConfig(connectionURL string) ports.StorageConfig {
	return ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: connectionURL},
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
	}
}

var _ = Describe("Protected-resource discovery through dynamic client registration", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		storage        *storageadapter.Adapter
		postgres       *bootstrap.PostgresFixture
		providers      []*helpers.MockProtectedResourceProvider
		provider       *helpers.MockProtectedResourceProvider
		admin          *bootstrap.TestServer
		enduser        *bootstrap.TestServer
		client         *http.Client
		principal      string
		scenario       fixtures.ProtectedResourceScenario
	)

	openServers := func(next *helpers.MockProtectedResourceProvider, selected fixtures.ProtectedResourceScenario) {
		config := fixtures.CIMDLocalConfig()
		config.ThirdPartyOAuth2.ClientName = selected.ClientName
		if postgres != nil {
			config.Storage = dcrPostgresConfig(postgres.ConnectionURL)
		}
		hosts := dcrAllowedHosts(selected)
		client = bootstrap.NewProtectedResourceHTTPClient(next.Server, hosts...)
		application, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(storage, next.Server, hosts...)
		Expect(err).NotTo(HaveOccurred())
		admin, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		enduser, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		provider = next
		scenario = selected
	}

	newProvider := func(selected fixtures.ProtectedResourceScenario) {
		next := helpers.NewMockProtectedResourceProvider(selected)
		providers = append(providers, next)
		openServers(next, selected)
	}

	closeServers := func() {
		if enduser != nil {
			enduser.Close()
			enduser = nil
		}
		if admin != nil {
			admin.Close()
			admin = nil
		}
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		principal = fixtures.DefaultPrincipal().String()
		storage = nil
		postgres = nil
		providers = nil
		provider = nil
		admin = nil
		enduser = nil
		client = nil
	})

	AfterEach(func() {
		closeServers()
		if storage != nil {
			Expect(storageFactory.CloseStorage(storage)).To(Succeed())
		}
		for _, started := range providers {
			started.Close()
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
		}
	})

	Context("when a DCR-only authorization server supports confidential registration", func() {
		BeforeEach(func() {
			var err error
			storage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			newProvider(fixtures.ConfidentialDCRProtectedResource())
		})

		// US2-S1 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should persist one registered identity usable for account connection", Label("protected-resource-discovery"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", ""))
			Expect(service).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
			Expect(service).To(HaveKeyWithValue("client_id", "dcr-client-123"))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "client_secret_basic"))
			Expect(service["discovery"]).To(HaveKeyWithValue("client_method", "dcr"))
			Expect(provider.RegistrationCount()).To(Equal(1))

			serviceID := service["id"].(string)
			dcrConnect(enduser, client, principal, serviceID)
			codeCalls := dcrCallsForGrant(provider, "authorization_code")
			Expect(codeCalls).To(HaveLen(1))
			Expect(codeCalls[0].AuthMethod).To(Equal("client_secret_basic"))
			Expect(dcrReadService(admin, principal, serviceID)).To(HaveKeyWithValue("client_id", service["client_id"]))
			Expect(provider.RegistrationCount()).To(Equal(1))
		})

		// US2-S5 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should send one explicit resource on authorization exchange and renewal", Label("protected-resource-discovery"), func() {
			const audience = "https://files.example.test/api"
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", audience))
			serviceID := service["id"].(string)
			dcrConnect(enduser, client, principal, serviceID)
			dcrExpireSession(storage, principal, serviceID)
			dcrRefresh(enduser, principal, serviceID)

			authorizations := dcrAuthorizeCalls(provider)
			codes := dcrCallsForGrant(provider, "authorization_code")
			renewals := dcrCallsForGrant(provider, "refresh_token")
			Expect(authorizations).To(HaveLen(1))
			Expect(codes).To(HaveLen(1))
			Expect(renewals).To(HaveLen(1))
			Expect(authorizations[0].Query["resource"]).To(Equal([]string{audience}))
			Expect(codes[0].Form["resource"]).To(Equal([]string{audience}))
			Expect(renewals[0].Form["resource"]).To(Equal([]string{audience}))
			Expect(provider.RegistrationCount()).To(Equal(1))
		})

		// US2-S6 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should read the explicit audience separately from its discovery URL", Label("protected-resource-discovery"), func() {
			const audience = "https://files.example.test/api"
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", audience))
			read := dcrReadService(admin, principal, service["id"].(string))
			Expect(read["authorization_params"]).To(HaveKeyWithValue("resource", audience))
			Expect(read["discovery"]).To(HaveKeyWithValue("resource_url", scenario.ResourceURL))
			Expect(audience).NotTo(Equal(scenario.ResourceURL))
		})

		// US2-S7 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should prefer confidential registration over advertised public registration", Label("protected-resource-discovery"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", ""))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "client_secret_basic"))
			Expect(provider.RegistrationCount()).To(Equal(1))
			var registrations []helpers.ProtectedResourceRequest
			for _, call := range provider.Calls() {
				if call.Method == http.MethodPost && call.Path == "/register" {
					registrations = append(registrations, call)
				}
			}
			Expect(registrations).To(HaveLen(1))
			Expect(registrations[0].Form["token_endpoint_auth_method"]).To(Equal([]string{"client_secret_basic"}))
		})

		// US2-S10 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should register the broker name without changing service display name", Label("protected-resource-discovery"), func() {
			Expect(scenario.ClientName).To(Equal("Example Platform"))
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", ""))
			Expect(service).To(HaveKeyWithValue("display_name", "Files MCP"))
			Expect(dcrReadService(admin, principal, service["id"].(string))).To(HaveKeyWithValue("display_name", "Files MCP"))
			var registrations []helpers.ProtectedResourceRequest
			for _, call := range provider.Calls() {
				if call.Method == http.MethodPost && call.Path == "/register" {
					registrations = append(registrations, call)
				}
			}
			Expect(registrations).To(HaveLen(1))
			Expect(registrations[0].Form["client_name"]).To(Equal([]string{"Example Platform"}))
			Expect(registrations[0].Form["client_name"]).NotTo(Equal([]string{"Files MCP"}))
		})
	})

	Context("when DCR offers only a public client", func() {
		BeforeEach(func() {
			var err error
			storage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			newProvider(fixtures.PublicDCRProtectedResource())
		})

		// US2-S2 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should connect a public client with PKCE and no secret", Label("protected-resource-discovery"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Public MCP", ""))
			dcrConnect(enduser, client, principal, service["id"].(string))
			authorizations := dcrAuthorizeCalls(provider)
			codes := dcrCallsForGrant(provider, "authorization_code")
			Expect(authorizations).To(HaveLen(1))
			Expect(codes).To(HaveLen(1))
			Expect(authorizations[0].Query.Get("code_challenge_method")).To(Equal("S256"))
			Expect(authorizations[0].PKCEChallengePresent).To(BeTrue())
			Expect(authorizations[0].PKCE).To(BeTrue())
			Expect(codes[0].PKCEVerifierPresent).To(BeTrue())
			Expect(codes[0].PKCE).To(BeTrue())
			Expect(codes[0].AuthMethod).To(Equal("none"))
			Expect(codes[0].ClientSecretPresent).To(BeFalse())
			Expect(codes[0].BasicAuthPresent).To(BeFalse())
		})

		// US2-S8 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should register a public-only client with mandatory S256 PKCE", Label("protected-resource-discovery"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Public MCP", ""))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(service["discovery"]).To(HaveKeyWithValue("client_method", "dcr"))
			Expect(provider.RegistrationCount()).To(Equal(1))
			dcrConnect(enduser, client, principal, service["id"].(string))
			authorizations := dcrAuthorizeCalls(provider)
			Expect(authorizations).To(HaveLen(1))
			Expect(authorizations[0].Query.Get("code_challenge_method")).To(Equal("S256"))
			Expect(authorizations[0].PKCE).To(BeTrue())
			var registrations []helpers.ProtectedResourceRequest
			for _, call := range provider.Calls() {
				if call.Method == http.MethodPost && call.Path == "/register" {
					registrations = append(registrations, call)
				}
			}
			Expect(registrations).To(HaveLen(1))
			Expect(registrations[0].Form["token_endpoint_auth_method"]).To(Equal([]string{"none"}))
		})
	})

	Context("when DCR selects confidential POST client authentication", func() {
		BeforeEach(func() {
			var err error
			storage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			newProvider(dcrPostOnlyScenario())
		})

		// US2-S4 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should renew for exactly its configured resource with POST authentication", Label("protected-resource-discovery"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "POST MCP", ""))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "client_secret_post"))
			serviceID := service["id"].(string)
			dcrConnect(enduser, client, principal, serviceID)
			dcrExpireSession(storage, principal, serviceID)
			dcrRefresh(enduser, principal, serviceID)
			codes := dcrCallsForGrant(provider, "authorization_code")
			renewals := dcrCallsForGrant(provider, "refresh_token")
			Expect(codes).To(HaveLen(1))
			Expect(renewals).To(HaveLen(1))
			Expect(codes[0].AuthMethod).To(Equal("client_secret_post"))
			Expect(codes[0].BasicAuthPresent).To(BeFalse())
			Expect(codes[0].ClientSecretPresent).To(BeTrue())
			Expect(renewals[0].AuthMethod).To(Equal("client_secret_post"))
			Expect(renewals[0].BasicAuthPresent).To(BeFalse())
			Expect(renewals[0].ClientSecretPresent).To(BeTrue())
			Expect(renewals[0].Form["resource"]).To(Equal([]string{scenario.ResourceURL}))
			Expect(provider.RegistrationCount()).To(Equal(1))
		})
	})

	Context("when the broker restarts with the same PostgreSQL database", func() {
		BeforeEach(func() {
			Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed())
			var err error
			postgres, err = bootstrap.NewPostgresFixture(context.Background())
			Expect(err).NotTo(HaveOccurred())
			config := dcrPostgresConfig(postgres.ConnectionURL)
			storage, err = storageadapter.NewAdapter(&config)
			Expect(err).NotTo(HaveOccurred())
			newProvider(fixtures.ConfidentialDCRProtectedResource())
		})

		// US2-S3 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should renew with its confidential identity after an actual restart", Label("protected-resource-discovery", "docker"), func() {
			service := dcrPostService(admin, principal, dcrDiscoveryRequest(scenario, "Files MCP", ""))
			serviceID := service["id"].(string)
			dcrConnect(enduser, client, principal, serviceID)
			Expect(dcrCallsForGrant(provider, "authorization_code")).To(HaveLen(1))
			dcrExpireSession(storage, principal, serviceID)

			closeServers()
			Expect(storageFactory.CloseStorage(storage)).To(Succeed())
			storage = nil
			var err error
			config := dcrPostgresConfig(postgres.ConnectionURL)
			storage, err = storageadapter.NewAdapter(&config)
			Expect(err).NotTo(HaveOccurred())
			openServers(provider, scenario)
			read := dcrReadService(admin, principal, serviceID)
			Expect(read).To(HaveKeyWithValue("client_id", service["client_id"]))
			Expect(read).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
			Expect(read).To(HaveKeyWithValue("token_endpoint_auth_method", "client_secret_basic"))
			dcrRefresh(enduser, principal, serviceID)
			renewals := dcrCallsForGrant(provider, "refresh_token")
			Expect(renewals).To(HaveLen(1))
			Expect(renewals[0].AuthMethod).To(Equal("client_secret_basic"))
			Expect(renewals[0].BasicAuthPresent).To(BeTrue())
			Expect(renewals[0].ClientSecretPresent).To(BeFalse())
			Expect(renewals[0].Form["resource"]).To(Equal([]string{scenario.ResourceURL}))
			Expect(provider.RegistrationCount()).To(Equal(1))
		})
	})

	Context("when distinct issuers return the same dynamic client ID", func() {
		BeforeEach(func() {
			var err error
			storage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			newProvider(fixtures.ConfidentialDCRProtectedResource())
		})

		// US2-S9 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should keep the issuer-scoped identities usable for both services", Label("protected-resource-discovery"), func() {
			firstScenario := scenario
			firstProvider := provider
			first := dcrPostService(admin, principal, dcrDiscoveryRequest(firstScenario, "First MCP", ""))
			closeServers()

			secondScenario := dcrSecondIssuerScenario()
			newProvider(secondScenario)
			secondProvider := provider
			second := dcrPostService(admin, principal, dcrDiscoveryRequest(secondScenario, "Second MCP", ""))
			Expect(first["client_id"]).To(Equal(second["client_id"]))
			Expect(first["issuer_uri"]).NotTo(Equal(second["issuer_uri"]))
			dcrConnect(enduser, client, principal, second["id"].(string))
			Expect(dcrCallsForGrant(secondProvider, "authorization_code")).To(HaveLen(1))
			closeServers()

			openServers(firstProvider, firstScenario)
			Expect(dcrReadService(admin, principal, first["id"].(string))).To(HaveKeyWithValue("issuer_uri", firstScenario.IssuerURLs[0]))
			Expect(dcrReadService(admin, principal, second["id"].(string))).To(HaveKeyWithValue("issuer_uri", secondScenario.IssuerURLs[0]))
			dcrConnect(enduser, client, principal, first["id"].(string))
			firstCodes := dcrCallsForGrant(firstProvider, "authorization_code")
			secondCodes := dcrCallsForGrant(secondProvider, "authorization_code")
			Expect(firstCodes).To(HaveLen(1))
			Expect(secondCodes).To(HaveLen(1))
			Expect(firstCodes[0].Host).To(Equal("login.example.test"))
			Expect(secondCodes[0].Host).To(Equal("login-two.example.test"))
			Expect(firstProvider.RegistrationCount()).To(Equal(1))
			Expect(secondProvider.RegistrationCount()).To(Equal(1))
		})
	})
})
