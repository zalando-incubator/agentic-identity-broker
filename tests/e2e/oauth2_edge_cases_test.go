package e2e_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OAuth2 Edge Cases and Error Scenarios", func() {
	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		ctx            context.Context
		storageFactory *bootstrap.StorageFactory
		testAgent      *storage.Agent // registered agent used in token endpoint tests
	)

	BeforeEach(func() {
		// Create logger for diagnostics
		logger = bootstrap.TestLogger(slog.LevelInfo)

		// Initialize test storage factory
		storageFactory = bootstrap.NewStorageFactory(logger)

		// Create fresh storage for this test
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// Create context for test operations
		ctx = context.Background()

		// Register a shared test agent so token endpoint tests can resolve
		// the broker-internal UUID to an upstream client_id.
		testAgent = fixtures.ValidAgent()
		Expect(testStorage.Agents().Create(ctx, testAgent)).ToNot(HaveOccurred())

		// Build and start test server with default config
		config := fixtures.DefaultOAuth2Config()
		factory := bootstrap.NewServerFactory(config, logger)
		built, err := factory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(built, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Close server gracefully if created
		if server != nil {
			server.Close()
		}
		// Close storage if created
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// GROUP 1: Authentication validation
	Describe("authentication failures", func() {
		It("should return 401 Unauthorized when X-Remote-User header is missing", func() {
			// Given: Server requires authentication
			// When: Request without X-Remote-User header
			resp, err := server.PublicGET("/oauth2/authorize?client_id=client-1&redirect_uri=https://client.example.com/cb&response_type=code")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 401 Unauthorized
			// Specification: missing authentication header must return 401
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should return 400 Bad Request when X-Remote-User header exceeds max length", func() {
			// Given: Oversized principal (>200 chars per spec)
			oversizedPrincipal := fixtures.OversizedPrincipal().String()

			// When: Request with oversized principal
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=client-1&redirect_uri=https://client.example.com/cb&response_type=code",
				oversizedPrincipal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 Bad Request
			// Specification: principal exceeding max length must be rejected
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// GROUP 2: OAuth2 parameter validation
	Describe("OAuth2 parameter validation", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			// Shared setup for all parameter validation tests
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(ctx, agent)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle missing client_id parameter", func() {
			// When: Request missing client_id
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?redirect_uri=https://client.example.com/cb&response_type=code",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns error
			// Specification: client_id is required parameter
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest), // Direct validation error
				Equal(http.StatusFound),      // Redirect with error parameter
			))
		})

		It("should handle missing response_type parameter", func() {
			// When: Request without response_type
			path := fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb",
				agent.ID.String())

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns error
			// Specification: response_type is required parameter
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))
		})

		It("should reject unsupported response_type values", func() {
			// When: Request with unsupported response_type (e.g., "token" instead of "code")
			path := fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=token",
				agent.ID.String())

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns unsupported_response_type error
			// Specification: only "code" flow is supported, others must be rejected
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))
		})

		It("should reject requests for non-existent client_id", func() {
			// When: Request for non-existent client
			path := "/oauth2/authorize?client_id=non-existent-client&redirect_uri=https://client.example.com/cb&response_type=code&state=state123"

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns invalid_client error
			// Specification: request with unknown client must be rejected
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))
		})

		It("should validate and reject malformed redirect_uri", func() {
			// When: Request with malformed redirect_uri (not a valid URL)
			path := fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=not-a-valid-url&response_type=code",
				agent.ID.String())

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Either rejects with 400 or redirects with error
			// Specification: malformed redirect_uri must be validated
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))
		})

		It("should handle redirect_uri parameter variations safely", func() {
			// When: Request with redirect_uri containing query parameters
			pathWithSpecialChars := "/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb?query=1&response_type=code&state=state123"
			path := fmt.Sprintf(pathWithSpecialChars, agent.ID.String())

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Handles gracefully without panic
			// Specification: system must safely handle special characters in parameters
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))
		})
	})

	// GROUP 3: Request body validation
	Describe("malformed request bodies", func() {
		It("should reject incorrect Content-Type for form endpoint", func() {
			// Given: Server expects application/x-www-form-urlencoded
			// When: POST to token endpoint with wrong Content-Type
			resp, err := server.DirectRequest(
				"POST",
				"/oauth2/token",
				fixtures.DefaultPrincipal().String(),
				map[string]string{
					"Content-Type": "application/json",
				},
				strings.NewReader(`{"grant_type":"authorization_code","code":"test"}`),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 Bad Request
			// Specification: token endpoint requires form-urlencoded body
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should handle empty POST body gracefully", func() {
			// Given: Server requires form parameters
			// When: POST to token endpoint with empty body
			resp, err := server.DirectRequest(
				"POST",
				"/oauth2/token",
				fixtures.DefaultPrincipal().String(),
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(""),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 Bad Request (missing required parameters)
			// Specification: empty body must be rejected with 400
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})

	// GROUP 4: Upstream server failures - unreachable scenario
	Describe("upstream server failures - unreachable", func() {
		var (
			agent          *storage.Agent
			upstreamServer *bootstrap.TestServer
		)

		BeforeEach(func() {
			// Reconfigure server to point to non-existent upstream
			config := fixtures.DefaultOAuth2Config()
			config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = "http://localhost:19999/authorize"
			config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = "http://localhost:19999/token"
			config.OAuth2AuthServer.Proxy.UpstreamTimeout = 100 * time.Millisecond // Short timeout to fail fast

			// Rebuild server with unreachable upstream
			factory := bootstrap.NewServerFactory(config, logger)
			built, err := factory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			// Close default server, will use this one instead
			if server != nil {
				server.Close()
			}

			upstreamServer, err = bootstrap.NewEndUserTestServer(built, logger)
			Expect(err).ToNot(HaveOccurred())
			server = upstreamServer

			// Create test agent
			agent = fixtures.ValidAgent()
			err = testStorage.Agents().Create(ctx, agent)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should handle upstream server unavailability gracefully", func() {
			// Given: Upstream URL points to non-existent server
			// When: Authorization request to unreachable upstream
			path := fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code",
				agent.ID.String())
			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns error (either 502/503 or redirect with error)
			// Specification: upstream unavailability must be handled gracefully
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadGateway),         // 502 - upstream connection error
				Equal(http.StatusServiceUnavailable), // 503 - upstream unavailable
				Equal(http.StatusFound),              // Redirect with error parameter
			))
		})
	})

	// GROUP 5: Upstream server failures - timeout scenario
	Describe("upstream server failures - timeout handling", func() {
		BeforeEach(func() {
			mockUpstream := helpers.NewMockUpstreamOAuth2Server().WithTokenHangUntilCanceled()
			DeferCleanup(mockUpstream.Close)

			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())
			config.OAuth2AuthServer.Proxy.UpstreamTimeout = time.Second

			factory := bootstrap.NewServerFactory(config, logger)
			built, err := factory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			if server != nil {
				server.Close()
			}

			server, err = bootstrap.NewEndUserTestServer(built, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should gracefully handle upstream timeout errors", func() {
			// Given: Upstream configured with short timeout
			// When: POST to token endpoint with valid form data
			// client_id must be a valid agent UUID (broker-internal identifier)
			resp, err := server.DirectRequest(
				"POST",
				"/oauth2/token",
				fixtures.DefaultPrincipal().String(),
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader("grant_type=authorization_code&code=test&client_id="+testAgent.ID.String()+"&redirect_uri=https://client.example.com/cb"),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns error response when upstream times out
			// Specification: timeout errors must be handled gracefully
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusGatewayTimeout),     // 504 - timeout
				Equal(http.StatusServiceUnavailable), // 503 - unavailable
				Equal(http.StatusBadGateway),         // 502 - proxy error
			))
		})
	})

	// GROUP 6: PKCE parameter forwarding to upstream
	Describe("PKCE parameter forwarding to upstream", func() {
		var (
			agent        *storage.Agent
			grant        *storage.UserGrant
			mockUpstream *helpers.MockUpstreamOAuth2Server
		)

		BeforeEach(func() {
			// Create mock upstream server for PKCE tests
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()

			// Build server config with mock upstream
			config := fixtures.DefaultOAuth2Config()
			config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"

			factory := bootstrap.NewServerFactory(config, logger)
			built, err := factory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			// Close default server and use mock upstream server
			if server != nil {
				server.Close()
			}

			server, err = bootstrap.NewEndUserTestServer(built, logger)
			Expect(err).ToNot(HaveOccurred())

			// Create test agent and grant for PKCE scenario
			agent = fixtures.ValidAgent()
			err = testStorage.Agents().Create(ctx, agent)
			Expect(err).ToNot(HaveOccurred())

			grant = fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
			err = testStorage.UserGrants().Create(ctx, grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(fixtures.SeedGrantSessions(ctx, testStorage, grant)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up mock upstream
			if mockUpstream != nil {
				mockUpstream.Close()
			}
		})

		It("should forward code_challenge and code_challenge_method parameters to upstream", func() {
			// Given: Mock upstream server configured
			// When: Authorization request with PKCE parameters
			pkceChallenge := "E9Mrozoa2owUonx4Z_p4gUzyQYISTuYnxlCMCkxo4dQ"
			path := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&code_challenge=%s&code_challenge_method=S256&state=state123",
				agent.ID.String(), pkceChallenge)

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Parameters are forwarded to upstream
			// Verify redirect happens
			Expect(resp.StatusCode).To(Equal(http.StatusFound))

			// Get the redirect location
			location := resp.Header.Get("Location")
			Expect(location).ToNot(BeEmpty())

			// Verify redirect is to upstream (not to consent page)
			Expect(location).To(ContainSubstring(mockUpstream.URL()))

			// Make request to the upstream endpoint directly (without following 2nd redirect)
			// to check that upstream received the PKCE parameters
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse // Don't follow redirects
				},
			}
			followResp, err := client.Get(location)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = followResp.Body.Close() }()

			// Check upstream received the request
			Expect(mockUpstream.GetAuthorizeCalled()).To(BeTrue())
			lastReq := mockUpstream.GetLastRequest()
			Expect(lastReq).ToNot(BeNil())

			// Verify PKCE parameters were forwarded in the URL
			queryParams := lastReq.URL.Query()
			Expect(queryParams.Get("code_challenge")).To(Equal(pkceChallenge))
			Expect(queryParams.Get("code_challenge_method")).To(Equal("S256"))
		})
	})

	// GROUP 7: Security validation
	Describe("security validations", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			// Create test agent for security validation tests
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(ctx, agent)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should safely handle XSS attempts in query parameters", func() {
			// Given: Attacker attempts to inject XSS payload
			// When: Request with XSS payload in state parameter
			xssPayload := fixtures.XSSPayload()
			encodedPayload := url.QueryEscape(xssPayload)
			path := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&state=%s",
				agent.ID.String(), encodedPayload)

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should safely handle the payload without executing script
			// Read response body to verify it doesn't contain unescaped script
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			// Verify script tag is not present unescaped
			// Specification: XSS payloads must be sanitized/escaped
			Expect(bodyStr).ToNot(ContainSubstring("<script>alert"))
		})

		It("should safely handle SQL injection attempts in query parameters", func() {
			// Given: Attacker attempts SQL injection
			// When: Request with SQL injection attempt in client_id
			sqlInjectionPayload := fixtures.SQLInjectionPayload()
			path := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code",
				url.QueryEscape(sqlInjectionPayload))

			resp, err := server.AuthenticatedGET(path, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns error but doesn't expose database details
			// Specification: injection attempts must be rejected safely
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusBadRequest),
				Equal(http.StatusFound),
			))

			// Verify response doesn't contain SQL error messages
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)
			Expect(bodyStr).ToNot(ContainSubstring("SQL"))
			Expect(bodyStr).ToNot(ContainSubstring("syntax"))
		})
	})

	// GROUP 8: Concurrent request handling
	Describe("concurrent request handling", func() {
		var (
			agent1 *storage.Agent
			agent2 *storage.Agent
		)

		BeforeEach(func() {
			// Create multiple agents for concurrent scenario
			agent1 = fixtures.ValidAgent()
			err := testStorage.Agents().Create(ctx, agent1)
			Expect(err).ToNot(HaveOccurred())

			agent2 = fixtures.AnotherAgent()
			err = testStorage.Agents().Create(ctx, agent2)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should prevent state pollution between concurrent requests", func() {
			// Given: Server is running with multiple agents
			// When: Make requests from different agents with different states
			resp1, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&state=state1",
					agent1.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp1.Body.Close() }()

			resp2, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&state=state2",
					agent2.ID.String()),
				fixtures.AnotherPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp2.Body.Close() }()

			// Then: Both requests complete with valid responses
			// Specification: concurrent requests must not interfere with each other
			Expect(resp1.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
				Equal(http.StatusOK),
			))
			Expect(resp2.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
				Equal(http.StatusOK),
			))

			// Verify no cross-contamination (state values should differ)
			// This ensures responses are not mixed between requests
			loc1 := resp1.Header.Get("Location")
			loc2 := resp2.Header.Get("Location")
			Expect(loc1).ToNot(Equal(loc2))
		})
	})

	// GROUP 9: Public endpoints (no auth required)
	Describe("public endpoints", func() {
		It("should serve metadata endpoint publicly without authentication", func() {
			// Given: Server is running
			// When: Request metadata endpoint without authentication
			resp, err := server.PublicGET("/.well-known/oauth-authorization-server")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 200 OK with JSON metadata
			// Specification: metadata endpoint must be publicly accessible
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
		})

		It("should serve health endpoint publicly", func() {
			// Given: Server is running
			// When: Request health endpoint without authentication
			resp, err := server.PublicGET("/health")
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 200 OK
			// Specification: health endpoint must be publicly accessible
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
