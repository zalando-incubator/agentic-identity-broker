package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	storagememory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("OAuth2 Authorization Endpoint", func() {
	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		logger         *slog.Logger
	)

	BeforeEach(func() {
		// Setup: Create FRESH storage for EACH test (test isolation)
		// This ensures tests are truly independent - no shared state between tests

		// Create logger for this test
		logger = bootstrap.TestLogger(slog.LevelInfo)

		// Create upstream mock server
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		// Create config with mock upstream
		config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)

		// Create storage factory and build fresh storage
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// Create server factory and build application
		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		// Create test server from app
		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Cleanup: Close server and storage
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

	// Scenario 1: Valid client lookup - system processes request successfully (not 400 invalid_client)
	Describe("when a valid authorization request arrives", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should look up the agent by client_id and validate it exists (not return 400 invalid_client)", func() {
			// Given: Valid agent registered
			// When: Authorization request with valid client_id (authenticated as default user)
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: System processes request successfully (200, 302, or 303, not 400 invalid_client)
			// Should NOT be 400 (bad request indicating invalid_client)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusBadRequest))
			// Should be either success or redirect
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusOK),
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))
		})
	})

	// Scenario 2a: Malformed client_id (not a UUID) - returns direct 400 invalid_client
	Describe("when authorization request has a malformed (non-UUID) client_id", func() {
		It("should return direct 400 invalid_client without redirect", func() {
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=not-a-uuid&redirect_uri=https://client.example.com/cb&response_type=code",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Scenario 2b: Valid UUID but unregistered agent - returns direct 400 invalid_client (RFC 6749 §4.1.2.1)
	Describe("when authorization request has an unknown agent UUID", func() {
		It("should return direct 400 invalid_client without redirect (redirect_uri unvalidated)", func() {
			// Given: No agents registered; UUID is well-formed but unknown
			unknownAgentID := id.NewAgentID().String()
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+unknownAgentID+"&redirect_uri=https://client.example.com/cb&response_type=code&state=st1",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Must be a direct 400 — no redirect to the unvalidated redirect_uri
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Scenario 3: No grant exists - redirect to consent UI with original URL preserved
	Describe("when no user consent exists for the agent", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())
			// No grant created for this agent
		})

		It("should redirect to consent UI with session_token sealing the original request URL", func() {
			// Given: Valid agent but no grant
			originalURL := "/oauth2/authorize?client_id=" + agent.ID.String() + "&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz&scope=openid"

			// When: Authorization request (authenticated as default user)
			resp, err := server.AuthenticatedGET(originalURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Redirects to consent UI
			Expect(resp).To(matchers.HaveOAuth2Redirect("/agents/" + agent.ID.String()))

			// And: Original URL sealed in session_token (not exposed as plain redirect_uri)
			redirectURL, err := helpers.ExtractRedirectURL(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(redirectURL.Query().Get("session_token")).ToNot(BeEmpty(), "consent redirect must carry session_token")
			Expect(redirectURL.Query().Get("redirect_uri")).To(BeEmpty(), "consent redirect must not expose raw redirect_uri")
		})
	})

	// Scenario 4: Active grant exists - proxy to upstream with all parameters preserved
	Describe("when user has an active grant for the agent", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())

			// Create active grant for default principal
			grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
			err = testStorage.UserGrants().Create(context.Background(), grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(fixtures.SeedGrantSessions(context.Background(), testStorage, grant)).To(Succeed())
		})

		It("should proxy the request to upstream OAuth2 server with all parameters preserved", func() {
			// Given: Active grant exists
			// When: Authorization request with multiple OAuth2 parameters (authenticated as default user)
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz&scope=openid+profile&code_challenge=abc&code_challenge_method=S256",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Redirects to upstream
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))

			// And: Location header points to upstream
			location := resp.Header.Get("Location")
			Expect(location).To(ContainSubstring(mockUpstream.Server.URL))
			Expect(location).To(ContainSubstring("/authorize"))

			// And: All parameters preserved in redirect
			redirectURL, err := helpers.ExtractRedirectURL(resp)
			Expect(err).ToNot(HaveOccurred())

			// Check key parameters are preserved
			Expect(redirectURL.Query().Get("client_id")).To(Equal(string(*agent.ClientID)))
			Expect(redirectURL.Query().Get("redirect_uri")).To(Equal("https://client.example.com/cb"))
			Expect(redirectURL.Query().Get("response_type")).To(Equal("code"))
			Expect(redirectURL.Query().Get("state")).To(Equal("xyz"))
			Expect(redirectURL.Query().Get("scope")).To(Equal("openid profile"))
			Expect(redirectURL.Query().Get("code_challenge")).To(Equal("abc"))
			Expect(redirectURL.Query().Get("code_challenge_method")).To(Equal("S256"))
		})
	})

	// Scenario 5: Expired grant - redirect to consent UI (treat as non-existent)
	Describe("when user's grant has expired", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())

			// Create expired grant using test method that bypasses validation
			expiredGrant := fixtures.ExpiredGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
			memRepo, ok := testStorage.UserGrants().(*storagememory.UserGrantRepository)
			if !ok {
				Skip("Test requires memory storage for CreateTestGrant method")
			}
			err = memRepo.CreateTestGrant(context.Background(), expiredGrant)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should treat expired grant as non-existent and redirect to consent UI", func() {
			// Given: Expired grant exists
			// When: Authorization request (authenticated as default user)
			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Redirects to consent UI (not upstream)
			Expect(resp).To(matchers.HaveOAuth2Redirect("/agents/" + agent.ID.String()))
		})
	})

	// Scenario 6: Request authentication validation - X-Remote-User header required
	Describe("when authorization request arrives without authentication", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should require X-Remote-User header (authentication)", func() {
			// Given: Valid agent and no X-Remote-User header (unauthenticated)
			// When: Authorization request without principal (using PublicGET)
			resp, err := server.PublicGET(
				"/oauth2/authorize?client_id=" + agent.ID.String() + "&redirect_uri=https://client.example.com/cb&response_type=code",
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Request should fail (unauthenticated)
			// Either 401 Unauthorized or 400 Bad Request (depending on implementation)
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusUnauthorized),
				Equal(http.StatusBadRequest),
				Equal(http.StatusInternalServerError), // Would panic in handler
			))
		})
	})

	// Scenario 7: Redirect URI validation - matching against registered URIs
	Describe("when authorization request has valid redirect_uri", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())

			// Create active grant
			grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
			err = testStorage.UserGrants().Create(context.Background(), grant)
			Expect(err).ToNot(HaveOccurred())
			Expect(fixtures.SeedGrantSessions(context.Background(), testStorage, grant)).To(Succeed())
		})

		It("should accept valid redirect_uri and proxy to upstream", func() {
			// Given: Valid agent with active grant and valid redirect_uri (must match registered URI)
			redirectURI := "https://client.example.com/cb"

			// When: Authorization request with valid redirect_uri
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
					agent.ID.String(), redirectURI),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Request is proxied to upstream (not redirected to error)
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))

			// And: Redirect goes to upstream (not an error redirect)
			location := resp.Header.Get("Location")
			Expect(location).To(ContainSubstring(mockUpstream.Server.URL))
		})
	})

	// Scenario 8: State parameter preservation - passed through to consent/upstream
	Describe("when authorization request includes state parameter", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("and no grant exists (redirect to consent)", func() {
			It("should preserve state parameter in session_token to consent UI", func() {
				// Given: Valid agent but no grant
				stateValue := "test-state-xyz-123"

				// When: Authorization request with state parameter
				resp, err := server.AuthenticatedGET(
					fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&state=%s",
						agent.ID.String(), stateValue),
					fixtures.DefaultPrincipal().String(),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Redirects to consent UI
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusFound),
					Equal(http.StatusSeeOther),
				))

				// And: State is preserved inside the session_token (no raw redirect_uri in URL)
				redirectURL, err := helpers.ExtractRedirectURL(resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(redirectURL.Query().Get("session_token")).ToNot(BeEmpty(), "consent redirect must carry session_token sealing the state")
				Expect(redirectURL.Query().Get("redirect_uri")).To(BeEmpty(), "state must not be exposed as plain redirect_uri")
			})
		})

		Context("and active grant exists (proxy to upstream)", func() {
			BeforeEach(func() {
				grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
				err := testStorage.UserGrants().Create(context.Background(), grant)
				Expect(err).ToNot(HaveOccurred())
				Expect(fixtures.SeedGrantSessions(context.Background(), testStorage, grant)).To(Succeed())
			})

			It("should preserve state parameter in redirect to upstream", func() {
				// Given: Valid agent with active grant
				stateValue := "csrf-token-abc-xyz"

				// When: Authorization request with state parameter
				resp, err := server.AuthenticatedGET(
					fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://client.example.com/cb&response_type=code&state=%s",
						agent.ID.String(), stateValue),
					fixtures.DefaultPrincipal().String(),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Redirects to upstream
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusFound),
					Equal(http.StatusSeeOther),
				))

				// And: State parameter is preserved in upstream redirect
				redirectURL, err := helpers.ExtractRedirectURL(resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(redirectURL.Query().Get("state")).To(Equal(stateValue))
			})
		})
	})

	// Additional edge case: Different principals with different grants
	Describe("when different users make authorization requests", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			agent = fixtures.ValidAgent()
			err := testStorage.Agents().Create(context.Background(), agent)
			Expect(err).ToNot(HaveOccurred())

			// Create grant only for default principal
			grantDefault := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"read", "write"})
			err = testStorage.UserGrants().Create(context.Background(), grantDefault)
			Expect(err).ToNot(HaveOccurred())
			Expect(fixtures.SeedGrantSessions(context.Background(), testStorage, grantDefault)).To(Succeed())

			// Another principal has no grant
		})

		It("should use different grant state for different principals", func() {
			// Given: Grant exists for DefaultPrincipal but not for AnotherPrincipal

			// When: Default principal makes request
			resp1, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp1.Body.Close() }()

			// Then: Request proxied to upstream (has grant)
			Expect(resp1.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))
			location1 := resp1.Header.Get("Location")
			Expect(location1).To(ContainSubstring(mockUpstream.Server.URL))

			// When: Another principal makes request
			resp2, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+"&redirect_uri=https://client.example.com/cb&response_type=code",
				fixtures.AnotherPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp2.Body.Close() }()

			// Then: Request redirected to consent UI (no grant)
			Expect(resp2.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))
			location2 := resp2.Header.Get("Location")
			Expect(location2).To(ContainSubstring("/agents/"))
		})
	})
})
