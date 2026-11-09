package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/dns/dnsmessage"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type protectedResourceRejectionEnvironment struct {
	storage   *storageadapter.Adapter
	admin     *bootstrap.TestServer
	enduser   *bootstrap.TestServer
	provider  *helpers.MockProtectedResourceProvider
	browser   *http.Client
	principal string
}

func useLoopbackDiscoveryDNS(hostname string) (func(), *atomic.Int32) {
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	done := make(chan struct{})
	var lookups atomic.Int32
	go func() {
		defer close(done)
		buffer := make([]byte, 512)
		for {
			n, peer, readErr := listener.ReadFrom(buffer)
			if readErr != nil {
				return
			}
			var query dnsmessage.Message
			if query.Unpack(buffer[:n]) != nil || len(query.Questions) != 1 {
				continue
			}
			question := query.Questions[0]
			reply := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			if strings.TrimSuffix(question.Name.String(), ".") == hostname && question.Type == dnsmessage.TypeA {
				lookups.Add(1)
				reply.Answers = []dnsmessage.Resource{{
					Header: dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET},
					Body:   &dnsmessage.AResource{A: [4]byte{127, 0, 0, 1}},
				}}
			}
			wire, packErr := reply.Pack()
			if packErr == nil {
				_, _ = listener.WriteTo(wire, peer)
			}
		}
	}()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", listener.LocalAddr().String())
	}}
	return func() {
		net.DefaultResolver = previous
		_ = listener.Close()
		<-done
	}, &lookups
}

var _ = Describe("Protected resource discovery rejection", func() {
	var (
		scenario               fixtures.ProtectedResourceScenario
		clientName             string
		useProductionTransport bool
		env                    *protectedResourceRejectionEnvironment
	)

	start := func(s fixtures.ProtectedResourceScenario, name string, productionTransport bool) *protectedResourceRejectionEnvironment {
		logger := bootstrap.TestLogger(slog.LevelWarn)
		provider := helpers.NewMockProtectedResourceProvider(s)
		DeferCleanup(provider.Close)
		factory := bootstrap.NewStorageFactory(logger)
		storage, err := factory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(factory.CloseStorage(storage)).To(Succeed()) })

		hosts := make([]string, 0, len(s.IssuerURLs)+2)
		for _, raw := range append(append([]string{s.ResourceURL}, s.IssuerURLs...), s.RegistrationURL) {
			if raw == "" {
				continue
			}
			parsed, parseErr := url.Parse(raw)
			Expect(parseErr).NotTo(HaveOccurred())
			hosts = append(hosts, parsed.Hostname())
		}
		config := fixtures.CIMDLocalConfig()
		config.ThirdPartyOAuth2.ClientName = name
		serverFactory := bootstrap.NewServerFactory(config, logger)
		var built *app.App
		if productionTransport {
			// Do not inject either discovery or token transport in the private-IP test.
			built, err = serverFactory.BuildApp(storage)
		} else {
			built, err = serverFactory.BuildAppWithProtectedResource(storage, provider.Server, hosts...)
		}
		Expect(err).NotTo(HaveOccurred())
		admin, err := bootstrap.NewAdminTestServer(built, logger)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(admin.Close)
		enduser, err := bootstrap.NewEndUserTestServer(built, logger)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(enduser.Close)
		browser := bootstrap.NewProtectedResourceHTTPClient(provider.Server, hosts...)
		DeferCleanup(browser.CloseIdleConnections)
		return &protectedResourceRejectionEnvironment{
			storage: storage, admin: admin, enduser: enduser,
			provider: provider, browser: browser, principal: fixtures.DefaultPrincipal().String(),
		}
	}

	request := func(_ *protectedResourceRejectionEnvironment, resource string) map[string]any {
		return map[string]any{
			"display_name": "Protected MCP service",
			"discovery":    map[string]any{"enable_discovery": true, "resource_url": resource},
		}
	}
	post := func(e *protectedResourceRejectionEnvironment, body map[string]any) *http.Response {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		response, err := e.admin.AuthenticatedPOST("/api/services", e.principal, "application/json", bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		return response
	}
	failure := func(response *http.Response, status int, category, code string) map[string]any {
		Expect(response.StatusCode).To(Equal(status))
		body := decodeJSON[map[string]any](response)
		Expect(body).To(HaveKeyWithValue("error", category))
		Expect(body).To(HaveKeyWithValue("message", code))
		if code != "issuer_selection_required" {
			Expect(body).NotTo(HaveKey("authorization_servers"))
		}
		return body
	}
	noServices := func(e *protectedResourceRejectionEnvironment) {
		response, err := e.admin.AuthenticatedGET("/api/services", e.principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[[]map[string]any](response)).To(BeEmpty())
	}
	provisionKey := func(e *protectedResourceRejectionEnvironment) {
		response, err := e.admin.AuthenticatedPOST("/api/cimd-client-keys", e.principal, "application/json", strings.NewReader(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(response.Body.Close()).To(Succeed())
	}
	setJSON := func(e *protectedResourceRejectionEnvironment, host, method, path string, status int, value map[string]any) {
		encoded, err := json.Marshal(value)
		Expect(err).NotTo(HaveOccurred())
		e.provider.SetReply(fixtures.ProtectedResourceReply{
			Host: host, Method: method, Path: path, Status: status,
			Headers: http.Header{"Content-Type": {"application/json"}}, Body: string(encoded),
		})
	}

	BeforeEach(func() {
		scenario = fixtures.CIMDOnlyProtectedResource()
		clientName = scenario.ClientName
		useProductionTransport = false
	})
	JustBeforeEach(func() { env = start(scenario, clientName, useProductionTransport) })

	Context("when protected-resource metadata is absent", func() {
		// US3-S1 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject missing metadata rather than accept manual endpoints", Label("protected-resource-discovery"), func() {
			env.provider.SetReply(fixtures.ProtectedResourceReply{Host: "cimd-only.example.test", Method: http.MethodGet, Path: "/.well-known/oauth-protected-resource/mcp", Status: http.StatusNotFound})
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "discovery failed", "resource_metadata_not_found")
			callsBeforeFallback := len(env.provider.Calls())
			manualFallback := request(env, scenario.ResourceURL)
			manualFallback["endpoints"] = map[string]any{"authorize_endpoint": scenario.IssuerURLs[0] + "/authorize", "token_endpoint": scenario.IssuerURLs[0] + "/token"}
			response := post(env, manualFallback)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(decodeJSON[map[string]any](response)).To(HaveKeyWithValue("error", "validation failed"))
			Expect(env.provider.Calls()).To(HaveLen(callsBeforeFallback))
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)
		})
	})

	Context("when the resource document identifies another resource", func() {
		// US3-S2 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject mismatched resource identity before choosing a client", Label("protected-resource-discovery"), func() {
			setJSON(env, "cimd-only.example.test", http.MethodGet, "/.well-known/oauth-protected-resource/mcp", http.StatusOK,
				map[string]any{"resource": "https://other.example.test/mcp", "authorization_servers": scenario.IssuerURLs})
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "discovery failed", "resource_mismatch")
			Expect(env.provider.Calls()).NotTo(ContainElement(HaveField("Host", "cimd-issuer.example.test")))
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)
		})
	})

	Context("when a target is a loopback address", func() {
		BeforeEach(func() {
			useProductionTransport = true
		})
		// US3-S3 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should block a private destination before connecting", Serial, Label("protected-resource-discovery"), func() {
			failure(post(env, request(env, env.provider.Server.URL+"/mcp")), http.StatusBadRequest, "discovery failed", "unsafe_destination")
			const hostname = "private-discovery.example.test"
			restoreDNS, lookups := useLoopbackDiscoveryDNS(hostname)
			DeferCleanup(restoreDNS)
			const dialTarget = "https://" + hostname + "/mcp"
			Expect(model.ValidatePublicHTTPSURL(dialTarget)).To(Succeed(), "the source must pass URL validation before DNS resolves it to loopback")
			failure(post(env, request(env, dialTarget)), http.StatusBadRequest, "discovery failed", "unsafe_destination")
			Expect(lookups.Load()).To(BeNumerically(">", 0), "the production dialer must resolve the hostname before it rejects loopback")
			Expect(env.provider.Calls()).To(BeEmpty())
			noServices(env)
		})
	})

	Context("when the issuer supports neither CIMD nor DCR", func() {
		// US3-S4 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should refuse to invent a static client identity", Label("protected-resource-discovery"), func() {
			setJSON(env, "cimd-issuer.example.test", http.MethodGet, "/.well-known/oauth-authorization-server/tenant", http.StatusOK,
				map[string]any{"issuer": scenario.IssuerURLs[0], "authorization_endpoint": scenario.IssuerURLs[0] + "/authorize", "token_endpoint": scenario.IssuerURLs[0] + "/token", "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}})
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "client registration failed", "no_compatible_client_method")
			Expect(env.provider.RegistrationCount()).To(BeZero())
			Expect(env.provider.Calls()).To(ContainElement(HaveField("Path", "/.well-known/oauth-authorization-server/tenant")))
			noServices(env)
		})
	})

	Context("when hosted CIMD is selected but its key is unavailable", func() {
		BeforeEach(func() { scenario = fixtures.CIMDAndDCRProtectedResource(); clientName = scenario.ClientName })
		// US3-S5 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should fail without retrying registration by DCR", Label("protected-resource-discovery"), func() {
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "client registration failed", "cimd_unavailable")
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)
		})
	})

	Context("when protected-resource metadata advertises two issuers", func() {
		BeforeEach(func() { scenario = fixtures.MultiIssuerProtectedResource() })
		// US3-S6 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should return validated choices and contact only the selected issuer on retry", Label("protected-resource-discovery"), func() {
			provisionKey(env)
			choices := failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "discovery failed", "issuer_selection_required")
			Expect(choices["authorization_servers"]).To(Equal([]any{scenario.IssuerURLs[0], scenario.IssuerURLs[1]}))
			for _, call := range env.provider.Calls() {
				Expect(call.Host).NotTo(BeElementOf("login-a.example.test", "login-b.example.test"))
			}
			noServices(env)
			selected := request(env, scenario.ResourceURL)
			selected["issuer_uri"] = scenario.IssuerURLs[1]
			response := post(env, selected)
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			created := decodeJSON[map[string]any](response)
			Expect(created).To(HaveKeyWithValue("issuer_uri", scenario.IssuerURLs[1]))
			Expect(created["discovery"]).To(HaveKeyWithValue("client_method", "cimd"))
			Expect(env.provider.Calls()).To(ContainElement(And(HaveField("Host", "login-b.example.test"), HaveField("Path", "/.well-known/oauth-authorization-server"))))
			Expect(env.provider.Calls()).NotTo(ContainElement(HaveField("Host", "login-a.example.test")))
			Expect(env.provider.RegistrationCount()).To(BeZero())
		})
	})

	Context("when the first issuer document reports another issuer", func() {
		// US3-S7 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject issuer mismatch without trying lower-priority locations", Label("protected-resource-discovery"), func() {
			provisionKey(env)
			setJSON(env, "cimd-issuer.example.test", http.MethodGet, "/.well-known/oauth-authorization-server/tenant", http.StatusOK,
				map[string]any{"issuer": "https://other-issuer.example.test/tenant", "authorization_endpoint": scenario.IssuerURLs[0] + "/authorize", "token_endpoint": scenario.IssuerURLs[0] + "/token", "code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": true, "token_endpoint_auth_methods_supported": []string{"private_key_jwt"}, "token_endpoint_auth_signing_alg_values_supported": []string{"ES256"}})
			setJSON(env, "cimd-issuer.example.test", http.MethodGet, "/.well-known/openid-configuration/tenant", http.StatusOK,
				map[string]any{"issuer": scenario.IssuerURLs[0], "authorization_endpoint": scenario.IssuerURLs[0] + "/authorize", "token_endpoint": scenario.IssuerURLs[0] + "/token", "code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": true, "token_endpoint_auth_methods_supported": []string{"private_key_jwt"}, "token_endpoint_auth_signing_alg_values_supported": []string{"ES256"}})
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "discovery failed", "issuer_mismatch")
			Expect(env.provider.Calls()).To(ContainElement(HaveField("Path", "/.well-known/oauth-authorization-server/tenant")))
			Expect(env.provider.Calls()).NotTo(ContainElement(HaveField("Path", "/.well-known/openid-configuration/tenant")))
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)
		})
	})

	Context("when the token endpoint rejects the bound resource", func() {
		BeforeEach(func() { scenario = fixtures.ConfidentialDCRProtectedResource(); clientName = scenario.ClientName })
		// US3-S8 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should fail connection without making an unbound token request", Label("protected-resource-discovery"), func() {
			created := post(env, request(env, scenario.ResourceURL))
			Expect(created.StatusCode).To(Equal(http.StatusCreated))
			service := decodeJSON[map[string]any](created)
			serviceID := service["id"].(string)
			setJSON(env, "login.example.test", http.MethodPost, "/token", http.StatusBadRequest, map[string]any{"error": "invalid_target"})

			begin, err := env.enduser.AuthenticatedGET("/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/sessions"), env.principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(begin.StatusCode).To(Equal(http.StatusFound))
			providerURL := begin.Header.Get("Location")
			Expect(begin.Body.Close()).To(Succeed())
			visit, err := env.browser.Get(providerURL)
			Expect(err).NotTo(HaveOccurred())
			Expect(visit.StatusCode).To(Equal(http.StatusFound))
			callbackURL, err := url.Parse(visit.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(visit.Body.Close()).To(Succeed())
			callback, err := env.enduser.AuthenticatedGET(callbackURL.RequestURI(), env.principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			failureURL, err := url.Parse(callback.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(failureURL.Query().Get("error")).To(Equal("callback_failed"))
			Expect(callback.Body.Close()).To(Succeed())

			var tokens []helpers.ProtectedResourceRequest
			for _, call := range env.provider.Calls() {
				if call.Method == http.MethodPost && call.Path == "/token" {
					tokens = append(tokens, call)
				}
			}
			Expect(tokens).To(HaveLen(1))
			Expect(tokens[0].Form["resource"]).To(Equal([]string{scenario.ResourceURL}))
			Expect(tokens[0].Form["grant_type"]).To(Equal([]string{"authorization_code"}))
			Expect(tokens[0].AuthMethod).To(Equal("client_secret_basic"))
			Expect(tokens[0].PKCEVerifierPresent).To(BeTrue())
			session, err := env.storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(env.principal), id.MustParseServiceID(serviceID))
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeNil())
			status, err := env.admin.AuthenticatedGET("/api/services/"+serviceID+"/discovery-status", env.principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(status.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](status)).To(HaveKeyWithValue("status", "ready"))
		})
	})

	Context("when an explicit authorization resource is invalid", func() {
		// US3-S9 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject an invalid audience before creating a service", Label("protected-resource-discovery"), func() {
			body := request(env, scenario.ResourceURL)
			body["authorization_params"] = map[string]string{"resource": "/relative-audience"}
			response := post(env, body)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			problem := decodeJSON[map[string]any](response)
			Expect(problem).To(HaveKeyWithValue("error", "validation failed"))
			Expect(problem["message"]).To(ContainSubstring("resource"))
			Expect(env.provider.Calls()).To(BeEmpty())
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)
		})
	})

	Context("when both protected-resource and direct metadata sources are specified", func() {
		// US3-S10 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject conflicting discovery sources before fetching metadata", Label("protected-resource-discovery"), func() {
			body := request(env, scenario.ResourceURL)
			body["discovery"].(map[string]any)["metadata_url"] = scenario.IssuerURLs[0] + "/.well-known/oauth-authorization-server/tenant"
			response := post(env, body)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			problem := decodeJSON[map[string]any](response)
			Expect(problem).To(HaveKeyWithValue("error", "validation failed"))
			Expect(problem["message"]).To(ContainSubstring("metadata_url"))
			Expect(env.provider.Calls()).To(BeEmpty())
			noServices(env)
		})
	})

	Context("when confidential registration is refused", func() {
		BeforeEach(func() { scenario = fixtures.RejectedProtectedResource(); clientName = "Example Platform" })
		// US3-S11 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should not retry rejected confidential registration as public", Label("protected-resource-discovery"), func() {
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "client registration failed", "client_registration_rejected")
			Expect(env.provider.RegistrationCount()).To(Equal(1))
			Expect(env.provider.Calls()).To(ContainElement(And(HaveField("Path", "/register"), HaveField("Form", HaveKeyWithValue("token_endpoint_auth_method", []string{"client_secret_basic"})))))
			noServices(env)
		})
	})

	Context("when a second service receives the same issuer and DCR client ID", func() {
		BeforeEach(func() { scenario = fixtures.ConfidentialDCRProtectedResource(); clientName = scenario.ClientName })
		// US3-S12 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should preserve the first service credential and reject the duplicate", Label("protected-resource-discovery"), func() {
			first := post(env, request(env, scenario.ResourceURL))
			Expect(first.StatusCode).To(Equal(http.StatusCreated))
			created := decodeJSON[map[string]any](first)
			firstID := id.MustParseServiceID(created["id"].(string))
			stored, err := env.storage.Services().Get(context.Background(), firstID)
			Expect(err).NotTo(HaveOccurred())
			originalSecret, err := stored.Secret.GetCiphertext()
			Expect(err).NotTo(HaveOccurred())
			originalSecret = bytes.Clone(originalSecret)

			secondResource := "https://files.example.test/another-mcp"
			env.provider.SetReply(fixtures.ProtectedResourceReply{Host: "files.example.test", Method: http.MethodGet, Path: "/another-mcp", Status: http.StatusUnauthorized,
				Headers: http.Header{"Www-Authenticate": {`Bearer resource_metadata="https://files.example.test/.well-known/oauth-protected-resource/another-mcp"`}}})
			setJSON(env, "files.example.test", http.MethodGet, "/.well-known/oauth-protected-resource/another-mcp", http.StatusOK,
				map[string]any{"resource": secondResource, "authorization_servers": scenario.IssuerURLs})
			setJSON(env, "login.example.test", http.MethodPost, "/register", http.StatusCreated,
				map[string]any{"client_id": created["client_id"], "client_secret": "different-provider-secret", "client_secret_expires_at": 0, "token_endpoint_auth_method": "client_secret_basic", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "redirect_uris": []string{}})
			second := request(env, secondResource)
			second["display_name"] = "Another MCP service"
			failure(post(env, second), http.StatusConflict, "conflict", "duplicate_client_identity")
			Expect(env.provider.RegistrationCount()).To(Equal(2))
			after, err := env.storage.Services().Get(context.Background(), firstID)
			Expect(err).NotTo(HaveOccurred())
			ciphertext, err := after.Secret.GetCiphertext()
			Expect(err).NotTo(HaveOccurred())
			Expect(ciphertext).To(Equal(originalSecret))
			Expect(after.ClientID).To(Equal(stored.ClientID))
			listed, err := env.admin.AuthenticatedGET("/api/services", env.principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[[]map[string]any](listed)).To(HaveLen(1))
		})
	})

	Context("when DCR has no configured broker client name", func() {
		BeforeEach(func() { scenario = fixtures.ConfidentialDCRProtectedResource(); clientName = "" })
		// US3-S13 from specs/050-oauth2-protected-resource-discovery/spec.md
		It("should reject absent and whitespace-only names without blocking other modes", Label("protected-resource-discovery"), func() {
			failure(post(env, request(env, scenario.ResourceURL)), http.StatusBadRequest, "client registration failed", "client_name_unconfigured")
			Expect(env.provider.RegistrationCount()).To(BeZero())
			noServices(env)

			providerCallsBeforeManual := len(env.provider.Calls())
			manual := map[string]any{
				"display_name": "Manual MCP", "oauth2_flavor": "standard", "issuer_uri": scenario.IssuerURLs[0],
				"client_id": "manually-registered-client", "client_secret": "manually-registered-secret",
				"discovery": map[string]any{"enable_discovery": false},
				"endpoints": map[string]any{"authorize_endpoint": scenario.IssuerURLs[0] + "/authorize", "token_endpoint": scenario.IssuerURLs[0] + "/token"},
			}
			manualCreated := post(env, manual)
			Expect(manualCreated.StatusCode).To(Equal(http.StatusCreated))
			Expect(decodeJSON[map[string]any](manualCreated)).To(HaveKeyWithValue("client_id", "manually-registered-client"))
			Expect(env.provider.Calls()).To(HaveLen(providerCallsBeforeManual))
			Expect(env.provider.RegistrationCount()).To(BeZero())

			cimdScenario := fixtures.CIMDOnlyProtectedResource()
			cimdEnv := start(cimdScenario, "", false)
			provisionKey(cimdEnv)
			cimdCreated := post(cimdEnv, request(cimdEnv, cimdScenario.ResourceURL))
			Expect(cimdCreated.StatusCode).To(Equal(http.StatusCreated))
			Expect(decodeJSON[map[string]any](cimdCreated)).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(cimdEnv.provider.RegistrationCount()).To(BeZero())

			blankEnv := start(fixtures.ConfidentialDCRProtectedResource(), " \t ", false)
			failure(post(blankEnv, request(blankEnv, scenario.ResourceURL)), http.StatusBadRequest, "client registration failed", "client_name_unconfigured")
			Expect(blankEnv.provider.RegistrationCount()).To(BeZero())
			noServices(blankEnv)
		})
	})
})
