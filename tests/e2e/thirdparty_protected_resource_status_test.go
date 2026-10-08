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

var _ = Describe("Protected resource discovery status and refresh", func() {
	var (
		logger         *slog.Logger
		principal      string
		config         *ports.Config
		scenario       fixtures.ProtectedResourceScenario
		provider       *helpers.MockProtectedResourceProvider
		providerClient *http.Client
		hosts          []string
		storageFactory *bootstrap.StorageFactory
		store          *storageadapter.Adapter
		postgres       *bootstrap.PostgresFixture
		usePostgres    bool
		admin          *bootstrap.TestServer
		enduser        *bootstrap.TestServer
		manualUpstream *helpers.MockUpstreamOAuth2Server
	)

	requestJSON := func(method, path string, body map[string]any) *http.Response {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		response, err := admin.DirectRequest(method, path, principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		return response
	}
	create := func(body map[string]any) map[string]any {
		response := requestJSON(http.MethodPost, "/api/services", body)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[map[string]any](response)
	}
	serviceRequest := func(resource string) map[string]any {
		return map[string]any{
			"display_name": "Files MCP",
			"discovery": map[string]any{
				"enable_discovery": true,
				"resource_url":     resource,
			},
		}
	}
	readService := func(serviceID string) (map[string]any, string) {
		response, err := admin.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		etag := response.Header.Get("ETag")
		Expect(etag).NotTo(BeEmpty())
		return decodeJSON[map[string]any](response), etag
	}
	readStatus := func(serviceID string) map[string]any {
		before := len(provider.Calls())
		registrations := provider.RegistrationCount()
		response, err := admin.AuthenticatedGET("/api/services/"+serviceID+"/discovery-status", principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		status := decodeJSON[map[string]any](response)
		Expect(status).NotTo(HaveKey("client_secret"))
		Expect(status).NotTo(HaveKey("registration_access_token"))
		encoded, err := json.Marshal(status)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("fixture-provider-secret"))
		Expect(string(encoded)).NotTo(ContainSubstring("fixture-refresh-token"))
		Expect(len(provider.Calls())).To(Equal(before), "status reads must not contact the provider")
		Expect(provider.RegistrationCount()).To(Equal(registrations))
		return status
	}
	update := func(serviceID string, body map[string]any) *http.Response {
		return requestJSON(http.MethodPut, "/api/services/"+serviceID, body)
	}
	refresh := func(serviceID string) {
		response, err := enduser.AuthenticatedPOST("/api/third-party/"+serviceID+"/session/refresh", principal, "application/json", nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(response.Body.Close()).To(Succeed())
	}
	readSession := func(serviceID string) map[string]any {
		response, err := enduser.AuthenticatedGET("/api/third-party/"+serviceID+"/session", principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)
	}
	connect := func(serviceID string, client *http.Client) {
		response, err := enduser.AuthenticatedGET(
			"/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/sessions"), principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())

		authorization, err := client.Get(location)
		Expect(err).NotTo(HaveOccurred())
		Expect(authorization.StatusCode).To(Equal(http.StatusFound))
		callbackURL, err := url.Parse(authorization.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(authorization.Body.Close()).To(Succeed())
		Expect(callbackURL.Path).To(Equal("/api/third-party/" + serviceID + "/oauth2/callback"))

		callback, err := enduser.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(callback.StatusCode).To(Equal(http.StatusFound))
		Expect(callback.Body.Close()).To(Succeed())
		Expect(readSession(serviceID)).NotTo(BeEmpty())
	}
	assertReady := func(status map[string]any, resource, issuer, method string) time.Time {
		Expect(status).To(HaveKeyWithValue("status", "ready"))
		Expect(status).To(HaveKeyWithValue("resource_url", resource))
		Expect(status).To(HaveKeyWithValue("issuer_uri", issuer))
		Expect(status).To(HaveKeyWithValue("client_method", method))
		Expect(status).To(HaveKeyWithValue("failure_reason", BeNil()))
		Expect(status["last_attempt_at"]).NotTo(BeNil())
		Expect(status["last_attempt_at"]).To(Equal(status["last_success_at"]))
		when, err := time.Parse(time.RFC3339Nano, status["last_success_at"].(string))
		Expect(err).NotTo(HaveOccurred())
		return when
	}
	assertFailed := func(status map[string]any, success time.Time) {
		Expect(status).To(HaveKeyWithValue("status", "failed"))
		Expect(status).To(HaveKeyWithValue("resource_url", scenario.ResourceURL))
		Expect(status).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[0]))
		Expect(status).To(HaveKeyWithValue("client_method", "dcr"))
		Expect(status).To(HaveKeyWithValue("failure_reason", "authorization_server_metadata_invalid"))
		lastSuccess, err := time.Parse(time.RFC3339Nano, status["last_success_at"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(lastSuccess).To(Equal(success))
		lastAttempt, err := time.Parse(time.RFC3339Nano, status["last_attempt_at"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(lastAttempt).To(BeTemporally(">=", lastSuccess))
	}
	breakIssuerMetadata := func() {
		issuer, err := url.Parse(scenario.IssuerURLs[0])
		Expect(err).NotTo(HaveOccurred())
		provider.SetReply(fixtures.ProtectedResourceReply{
			Host: issuer.Host, Method: http.MethodGet, Path: "/.well-known/oauth-authorization-server", Status: http.StatusOK,
			Headers: http.Header{"Content-Type": {"application/json"}},
			Body:    `{"issuer":"` + scenario.IssuerURLs[0] + `","authorization_endpoint":"` + scenario.IssuerURLs[0] + `/authorize","sensitive_remote_field":"do-not-leak"}`,
		})
	}
	failedUpdate := func(serviceID string) {
		breakIssuerMetadata()
		response := update(serviceID, serviceRequest(scenario.ResourceURL))
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		body := decodeJSON[map[string]any](response)
		Expect(body).To(HaveKeyWithValue("message", "authorization_server_metadata_invalid"))
		Expect(body).NotTo(HaveKey("authorization_servers"))
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(encoded)).NotTo(ContainSubstring("do-not-leak"))
	}
	assertRefreshCall := func(resource, clientID string) {
		calls := provider.Calls()
		var refreshCalls []helpers.ProtectedResourceRequest
		for _, call := range calls {
			if call.Method == http.MethodPost && call.Form.Get("grant_type") == "refresh_token" {
				refreshCalls = append(refreshCalls, call)
			}
		}
		Expect(refreshCalls).To(HaveLen(1))
		call := refreshCalls[0]
		Expect(call.Host).To(Equal("login.example.test"))
		Expect(call.Path).To(Equal("/token"))
		Expect(call.Form["resource"]).To(Equal([]string{resource}))
		Expect(call.AuthMethod).To(Equal("client_secret_basic"))
		Expect(call.BasicAuthPresent).To(BeTrue())
		Expect(call.Form).NotTo(HaveKey("client_secret"))
		Expect(call.Form).NotTo(HaveKey("client_id"))
		Expect(clientID).To(Equal("dcr-client-123"))
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		principal = fixtures.DefaultPrincipal().String()
		config = fixtures.CIMDLocalConfig()
		scenario = fixtures.ConfidentialDCRProtectedResource()
		config.ThirdPartyOAuth2.ClientName = scenario.ClientName
		storageFactory = bootstrap.NewStorageFactory(logger)
		usePostgres = false
	})

	JustBeforeEach(func() {
		provider = helpers.NewMockProtectedResourceProvider(scenario)
		seen := make(map[string]bool)
		for _, rawURL := range append(append([]string{scenario.ResourceURL}, scenario.IssuerURLs...), scenario.RegistrationURL) {
			if rawURL == "" {
				continue
			}
			parsed, err := url.Parse(rawURL)
			Expect(err).NotTo(HaveOccurred())
			if !seen[parsed.Hostname()] {
				hosts = append(hosts, parsed.Hostname())
				seen[parsed.Hostname()] = true
			}
		}
		providerClient = bootstrap.NewProtectedResourceHTTPClient(provider.Server, hosts...)
		var err error
		if usePostgres {
			Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed())
			postgres, err = bootstrap.NewPostgresFixture(context.Background())
			Expect(err).NotTo(HaveOccurred())
			config.Storage = ports.StorageConfig{
				Backend:  "postgres",
				Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL},
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
			}
			store, err = storageadapter.NewAdapter(&config.Storage)
		} else {
			store, err = storageFactory.NewTestStorage()
		}
		Expect(err).NotTo(HaveOccurred())
		application, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(store, provider.Server, hosts...)
		Expect(err).NotTo(HaveOccurred())
		admin, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		enduser, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if enduser != nil {
			enduser.Close()
		}
		if admin != nil {
			admin.Close()
		}
		if store != nil {
			Expect(storageFactory.CloseStorage(store)).To(Succeed())
		}
		if providerClient != nil {
			providerClient.CloseIdleConnections()
		}
		if provider != nil {
			provider.Close()
		}
		if manualUpstream != nil {
			manualUpstream.Close()
		}
		if postgres != nil {
			Expect(postgres.Close(context.Background())).To(Succeed())
		}
		hosts = nil
		providerClient = nil
		store = nil
		admin = nil
		enduser = nil
		provider = nil
		postgres = nil
		manualUpstream = nil
	})

	Context("when discovery creates a ready DCR service", func() {
		var (
			service  map[string]any
			override string
		)

		BeforeEach(func() { override = "" })
		JustBeforeEach(func() {
			request := serviceRequest(scenario.ResourceURL)
			if override != "" {
				request["authorization_params"] = map[string]string{"resource": override}
			}
			service = create(request)
		})

		// US4-S1 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should report credential-free ready status without provider traffic", Label("protected-resource-discovery"), func() {
			serviceID := service["id"].(string)
			callsBefore := len(provider.Calls())
			unauthenticated, err := admin.DirectRequest(http.MethodGet, "/api/services/"+serviceID+"/discovery-status", "", nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(unauthenticated.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(unauthenticated.Body.Close()).To(Succeed())
			Expect(provider.Calls()).To(HaveLen(callsBefore))
			assertReady(readStatus(serviceID), scenario.ResourceURL, scenario.IssuerURLs[0], "dcr")
			Expect(service).NotTo(HaveKey("client_secret"))
			Expect(provider.RegistrationCount()).To(Equal(1))
			unknown := id.NewServiceID().String()
			before := len(provider.Calls())
			response, err := admin.AuthenticatedGET("/api/services/"+unknown+"/discovery-status", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
			Expect(response.Body.Close()).To(Succeed())
			Expect(provider.Calls()).To(HaveLen(before))

			deleted, err := admin.DirectRequest(http.MethodDelete, "/api/services/"+serviceID, principal, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(deleted.StatusCode).To(Equal(http.StatusNoContent))
			Expect(deleted.Body.Close()).To(Succeed())
			response, err = admin.AuthenticatedGET("/api/services/"+serviceID+"/discovery-status", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusNotFound))
			Expect(response.Body.Close()).To(Succeed())
			Expect(provider.Calls()).To(HaveLen(before))
		})

		// US4-S2 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should preserve active settings, ETag and sessions after failed refresh", Label("protected-resource-discovery"), func() {
			serviceID := service["id"].(string)
			connect(serviceID, providerClient)
			beforeService, etag := readService(serviceID)
			beforeSession := readSession(serviceID)
			success := assertReady(readStatus(serviceID), scenario.ResourceURL, scenario.IssuerURLs[0], "dcr")
			failedUpdate(serviceID)
			assertFailed(readStatus(serviceID), success)
			afterService, afterETag := readService(serviceID)
			Expect(afterETag).To(Equal(etag))
			Expect(afterService).To(Equal(beforeService))
			Expect(readSession(serviceID)).To(Equal(beforeSession))
			Expect(afterService).NotTo(HaveKey("client_secret"))
			Expect(provider.RegistrationCount()).To(Equal(1))
			refresh(serviceID)
			assertRefreshCall(scenario.ResourceURL, beforeService["client_id"].(string))
		})

		Context("and the issuer republishes its endpoints", func() {
			// US4-S4 from specs/050-oauth2-protected-resource-discovery/spec.md
			It("should refresh endpoints while retaining the same client identity", Label("protected-resource-discovery"), func() {
				serviceID := service["id"].(string)
				before, _ := readService(serviceID)
				issuer := scenario.IssuerURLs[0]
				newAuthorize, newToken := issuer+"/authorize-next", issuer+"/token-next"
				provider.SetReply(fixtures.ProtectedResourceReply{
					Host: "login.example.test", Method: http.MethodGet, Path: "/.well-known/oauth-authorization-server", Status: http.StatusOK,
					Headers: http.Header{"Content-Type": {"application/json"}},
					Body:    `{"issuer":"` + issuer + `","authorization_endpoint":"` + newAuthorize + `","token_endpoint":"` + newToken + `","registration_endpoint":"` + scenario.RegistrationURL + `","token_endpoint_auth_methods_supported":["client_secret_basic"],"code_challenge_methods_supported":["S256"]}`,
				})
				provider.SetReply(fixtures.ProtectedResourceReply{Host: "login.example.test", Method: http.MethodGet, Path: "/authorize-next", Status: http.StatusFound})
				provider.SetReply(fixtures.ProtectedResourceReply{Host: "login.example.test", Method: http.MethodPost, Path: "/token-next", Status: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}}, Body: `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`})
				response := update(serviceID, serviceRequest(scenario.ResourceURL))
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				updated := decodeJSON[map[string]any](response)
				Expect(updated["endpoints"]).To(HaveKeyWithValue("authorize_endpoint", newAuthorize))
				Expect(updated["endpoints"]).To(HaveKeyWithValue("token_endpoint", newToken))
				Expect(updated["client_id"]).To(Equal(before["client_id"]))
				Expect(updated["token_endpoint_auth_method"]).To(Equal(before["token_endpoint_auth_method"]))
				Expect(updated["discovery"]).To(Equal(before["discovery"]))
				Expect(provider.RegistrationCount()).To(Equal(1))
				connect(serviceID, providerClient)
				calls := provider.Calls()
				Expect(calls).To(ContainElement(And(HaveField("Path", "/authorize-next"), HaveField("PKCE", true))))
				Expect(calls).To(ContainElement(And(HaveField("Path", "/token-next"), HaveField("AuthMethod", "client_secret_basic"))))
			})
		})

		Context("and a failed refresh is followed by a PostgreSQL restart", func() {
			BeforeEach(func() { usePostgres = true })

			// US4-S5 from specs/050-oauth2-protected-resource-discovery/spec.md
			It("should retain failed status and renew with the persisted credential", Label("protected-resource-discovery", "docker"), func() {
				serviceID := service["id"].(string)
				connect(serviceID, providerClient)
				beforeService, etag := readService(serviceID)
				success := assertReady(readStatus(serviceID), scenario.ResourceURL, scenario.IssuerURLs[0], "dcr")
				failedUpdate(serviceID)
				assertFailed(readStatus(serviceID), success)
				enduser.Close()
				admin.Close()
				Expect(storageFactory.CloseStorage(store)).To(Succeed())
				store = nil

				var err error
				store, err = storageadapter.NewAdapter(&config.Storage)
				Expect(err).NotTo(HaveOccurred())
				application, err := bootstrap.NewServerFactory(config, logger).BuildAppWithProtectedResource(store, provider.Server, hosts...)
				Expect(err).NotTo(HaveOccurred())
				admin, err = bootstrap.NewAdminTestServer(application, logger)
				Expect(err).NotTo(HaveOccurred())
				enduser, err = bootstrap.NewEndUserTestServer(application, logger)
				Expect(err).NotTo(HaveOccurred())

				assertFailed(readStatus(serviceID), success)
				afterService, afterETag := readService(serviceID)
				Expect(afterService).To(Equal(beforeService))
				Expect(afterETag).To(Equal(etag))
				Expect(readSession(serviceID)).NotTo(BeEmpty())
				refresh(serviceID)
				assertRefreshCall(scenario.ResourceURL, beforeService["client_id"].(string))
				Expect(provider.RegistrationCount()).To(Equal(1))
			})
		})

		Context("and the resource audience was derived", func() {
			// US4-S6 from specs/050-oauth2-protected-resource-discovery/spec.md
			It("should replace the derived audience with a newly verified resource", Label("protected-resource-discovery"), func() {
				serviceID := service["id"].(string)
				newResource := strings.TrimSuffix(scenario.ResourceURL, "/mcp") + "/mcp-v2"
				provider.SetReply(fixtures.ProtectedResourceReply{Host: "files.example.test", Method: http.MethodGet, Path: "/mcp-v2", Status: http.StatusUnauthorized, Headers: http.Header{"WWW-Authenticate": {`Bearer resource_metadata="https://files.example.test/.well-known/oauth-protected-resource/mcp-v2"`}}})
				provider.SetReply(fixtures.ProtectedResourceReply{Host: "files.example.test", Method: http.MethodGet, Path: "/.well-known/oauth-protected-resource/mcp-v2", Status: http.StatusOK, Headers: http.Header{"Content-Type": {"application/json"}}, Body: `{"resource":"` + newResource + `","authorization_servers":["` + scenario.IssuerURLs[0] + `"]}`})
				response := update(serviceID, serviceRequest(newResource))
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				updated := decodeJSON[map[string]any](response)
				Expect(updated["authorization_params"]).To(HaveKeyWithValue("resource", newResource))
				Expect(updated["discovery"]).To(HaveKeyWithValue("resource_url", newResource))
				Expect(readStatus(serviceID)).To(HaveKeyWithValue("resource_url", newResource))
				Expect(provider.Calls()).To(ContainElement(And(HaveField("Host", "files.example.test"), HaveField("Path", "/.well-known/oauth-protected-resource/mcp-v2"))))
				connect(serviceID, providerClient)
				for _, call := range provider.Calls() {
					if call.Path == "/authorize" {
						Expect(call.Query["resource"]).To(Equal([]string{newResource}))
					}
					if call.Path == "/token" {
						Expect(call.Form["resource"]).To(Equal([]string{newResource}))
					}
				}
				Expect(provider.RegistrationCount()).To(Equal(1))
			})
		})

		Context("and an administrator chose a different token audience", func() {
			BeforeEach(func() { override = "https://files.example.test/api" })

			// US4-S7 from specs/050-oauth2-protected-resource-discovery/spec.md
			It("should retain the explicit audience when refreshing without an override", Label("protected-resource-discovery"), func() {
				serviceID := service["id"].(string)
				response := update(serviceID, serviceRequest(scenario.ResourceURL))
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				updated := decodeJSON[map[string]any](response)
				Expect(updated["authorization_params"]).To(HaveKeyWithValue("resource", override))
				Expect(updated["discovery"]).To(HaveKeyWithValue("resource_url", scenario.ResourceURL))
				connect(serviceID, providerClient)
				for _, call := range provider.Calls() {
					if call.Path == "/authorize" {
						Expect(call.Query["resource"]).To(Equal([]string{override}))
					}
					if call.Path == "/token" {
						Expect(call.Form["resource"]).To(Equal([]string{override}))
					}
				}
				refresh(serviceID)
				assertRefreshCall(override, updated["client_id"].(string))
			})

			// US4-S8 from specs/050-oauth2-protected-resource-discovery/spec.md
			It("should restore the verified resource when replacement parameters omit it", Label("protected-resource-discovery"), func() {
				serviceID := service["id"].(string)
				request := serviceRequest(scenario.ResourceURL)
				request["authorization_params"] = map[string]string{"prompt": "consent"}
				response := update(serviceID, request)
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				updated := decodeJSON[map[string]any](response)
				Expect(updated["authorization_params"]).To(HaveKeyWithValue("resource", scenario.ResourceURL))
				Expect(updated["authorization_params"]).To(HaveKeyWithValue("prompt", "consent"))
				read, _ := readService(serviceID)
				Expect(read["authorization_params"]).To(Equal(updated["authorization_params"]))
				connect(serviceID, providerClient)
				for _, call := range provider.Calls() {
					if call.Path == "/authorize" {
						Expect(call.Query["resource"]).To(Equal([]string{scenario.ResourceURL}))
					}
					if call.Path == "/token" {
						Expect(call.Form["resource"]).To(Equal([]string{scenario.ResourceURL}))
					}
				}
			})
		})
	})

	Context("when an existing manual service is read", func() {
		BeforeEach(func() {
			manualUpstream = helpers.NewMockUpstreamOAuth2Server().WithStrictPublicClientMode().WithSuccessfulTokenResponse()
			config.Security.SkipThirdpartyHTTPSValidation = true
		})

		// US4-S3 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should report not applicable without changing manual account connections", Label("protected-resource-discovery"), func() {
			fixture := fixtures.PublicClientService()
			request := map[string]any{
				"display_name":               fixture.DisplayName,
				"client_id":                  fixture.ClientID.String(),
				"token_endpoint_auth_method": "none",
				"issuer_uri":                 manualUpstream.URL(),
				"discovery":                  map[string]any{"enable_discovery": false},
				"endpoints": map[string]string{
					"authorize_endpoint": manualUpstream.URL() + "/oauth/authorize",
					"token_endpoint":     manualUpstream.URL() + "/oauth/token",
				},
			}
			service := create(request)
			serviceID := service["id"].(string)
			status := readStatus(serviceID)
			Expect(status).To(HaveKeyWithValue("status", "not_applicable"))
			for _, field := range []string{"resource_url", "issuer_uri", "client_method", "last_attempt_at", "last_success_at", "failure_reason"} {
				Expect(status).To(HaveKeyWithValue(field, BeNil()))
			}
			Expect(provider.Calls()).To(BeEmpty())
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			connect(serviceID, client)
			refresh(serviceID)
			Expect(manualUpstream.GetTokenRequests()).To(HaveLen(2))
			Expect(provider.Calls()).To(BeEmpty())
			Expect(provider.RegistrationCount()).To(Equal(0))
		})
	})
})
