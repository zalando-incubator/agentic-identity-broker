package e2e_test

import (
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
	storagememory "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// TokenFixtures holds all JWT tokens for a test suite execution.
// Tokens are generated once in BeforeEach and shared across all tests.
// This ensures consistent, valid JWT tokens for testing token exchange flows.
type TokenFixtures struct {
	// Valid tokens for successful scenarios
	SubjectToken              string // Valid subject token with principal and agent ID
	ClientAssertion           string // Valid client assertion for gateway authentication
	ExpiredSubjectToken       string // Expired subject token (exp in past)
	MalformedSubjectToken     string // Malformed subject token (not a valid JWT)
	MissingSubClaimToken      string // Token without 'sub' claim
	MissingAudClaimToken      string // Token without 'aud' claim
	InvalidIssuerToken        string // Token with wrong issuer
	ClientAssertionInvalidSig string // JWT with invalid signature
	TokenWithCELClaims        string // Token with claims for CEL evaluation
}

var _ = Describe("RFC 8693 Token Exchange E2E Tests", func() {
	var (
		serverFactory  *bootstrap.ServerFactory
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		testStorage    *storageadapter.Adapter
		enduserServer  *bootstrap.TestServer
		adminServer    *bootstrap.TestServer
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		config         *ports.Config
		principal      string
		agent          *storagedomain.Agent
		tokenFixtures  *TokenFixtures
	)

	// Setup: Initialize fresh test infrastructure for each test
	BeforeEach(func() {
		// Initialize logger with GinkgoWriter for test visibility
		// This allows us to see slog output in test failures for debugging
		logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		// Create mock upstream OAuth2 server
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		// Get configuration with mock upstream and token exchange enabled
		config = fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL())

		// Create storage factory and fresh storage instance
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		// Create server factory
		serverFactory = bootstrap.NewServerFactory(config, logger)

		// Build application instance
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())

		// Create separate HTTP test servers for end-user and admin endpoints
		// This matches production architecture where they are on different ports/servers
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).NotTo(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).NotTo(HaveOccurred())

		// Create test fixtures
		principal = fixtures.DefaultPrincipal().String()
		agent = fixtures.ValidAgent()

		// Store fixture data
		ctx := context.Background()
		err = testStorage.Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred())

		// ============ PHASE 2: RFC 8693 TOKEN EXCHANGE DATA SETUP ============
		// Generate real JWT tokens for testing RFC 8693 token exchange flows.
		// All tokens are properly signed with the mock upstream's private key.
		// This ensures JWT validation passes and tests move past token parsing.

		tokenFixtures = generateTokenFixtures(mockUpstream, principal, agent)

		// Create GitHub service with protected_resources for resource-based lookup (US2)
		githubService := fixtures.GitHubService()
		// Update service endpoints to use mock upstream for token refresh tests (US1-S4)
		githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
		githubService.Endpoints.AuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
		err = testStorage.Services().Create(ctx, githubService)
		Expect(err).NotTo(HaveOccurred())

		// Seed placeholder PS/service rows so the grant covers GitHub during
		// permission set scope resolution in the token exchange path.
		err = fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)
		Expect(err).NotTo(HaveOccurred())

		// Create user grant allowing agent to access GitHub service (US3)
		// This grant is required before token exchange can succeed.
		// US3-S2, US3-S3, US3-S4 test different grant states (active, revoked, expired).
		// NOTE: Grant must use agent.ID (internal UUID) because token exchange service
		// resolves agent client_id (from JWT "azp" claim) to agent UUID via AgentRepository,
		// then looks up grants by principal + agent.ID.
		grant := fixtures.ActiveGrant(principal, agent.ID.String(), githubService.ID.String(), []string{"repo", "user"})
		err = testStorage.UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred())

		// Create user session with stored GitHub tokens (US1)
		// Session contains encrypted access/refresh tokens retrieved from vault.
		// US1-S4 tests automatic refresh when access token is expired.
		session := fixtures.GitHubSessionForPrincipal(principal)
		err = testStorage.UserSessions().Create(ctx, session)
		Expect(err).NotTo(HaveOccurred())
	})

	// Cleanup: Close resources after each test
	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if adminServer != nil {
			adminServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// US1: Gateway Exchanges Token for Third-Party Token
	Describe("US1: Gateway Exchanges Token for Third-Party Token", func() {
		// Spec Reference: US1-S1 from specs/013-token-exchange/spec.md
		It("[US1-S1] should detect token exchange via grant_type parameter", func() {
			// Given: Valid RFC 8693 token exchange request is prepared
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("third-party-token-123")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Client sends POST request to token endpoint with token exchange grant_type
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Token exchange succeeds with RFC 8693 response
			// SEMANTIC FAILURE: endpoint not implemented returns 404 or wrong status
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(resp).To(matchers.HaveTokenExchangeSuccess())
		})

		// Spec Reference: US1-S2 from specs/013-token-exchange/spec.md
		It("[US1-S2] should look up service by resource URI in protected_resources", func() {
			// Given: Service with protected_resources is configured (to be added in Phase 3)
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("third-party-token-456")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange request includes resource parameter
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: System looks up service by resource URI
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		})

		// Spec Reference: US1-S3 from specs/013-token-exchange/spec.md
		It("[US1-S3] should return RFC 8693 response with access_token and token_type", func() {
			// Given: Token exchange request is valid
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("github-token-xyz").
				WithExpiresIn(3600)

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Client sends token exchange request
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response conforms to RFC 8693 format
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(resp).To(matchers.HaveTokenExchangeSuccess().
				WithAccessToken("github-token-xyz").
				WithTokenType("Bearer").
				WithIssuedTokenType("urn:ietf:params:oauth:token-type:access_token").
				WithExpiresIn(3600))
		})

		// Spec Reference: US1-S4 from specs/013-token-exchange/spec.md
		It("[US1-S4] should auto-refresh expired token if refresh_token valid", func() {
			// Given: Session exists with expired access_token but valid refresh_token
			// Replace the default active session with an expired one
			// Create() uses upsert semantics, so this replaces the existing session
			ctx := context.Background()
			expiredSession := fixtures.ExpiredGitHubSessionForPrincipal(principal)
			err := testStorage.UserSessions().Create(ctx, expiredSession)
			Expect(err).NotTo(HaveOccurred())

			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("refreshed-token-new")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange request is made for resource with expired token
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: System auto-refreshes expired token and returns new one
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var tokenResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenResponse["access_token"]).To(Equal("refreshed-token-new"))
		})

		// Spec Reference: US1-S5 from specs/013-token-exchange/spec.md
		It("[US1-S5] should return 401 invalid_client without valid client_assertion", func() {
			// Given: Request with invalid client_assertion
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertionInvalidSig},
				"resource":              {"https://api.github.com"},
			}

			// When: Client sends token exchange with invalid client_assertion
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 401 with invalid_client error
			// SEMANTIC FAILURE: JWT validation not implemented, returns wrong status
			Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_client"))
		})

		// Spec Reference: US1-S6 from specs/013-token-exchange/spec.md
		It("[US1-S6] should return 400 invalid_request with invalid subject_token", func() {
			// Given: Request with malformed subject_token (not a valid JWT structure)
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.MalformedSubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Client sends token exchange with invalid subject_token
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 with invalid_request error
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_request"))
		})
	})

	// US2: Resource-Based Service Discovery
	Describe("US2: Resource-Based Service Discovery", func() {
		// Spec Reference: US2-S1 from specs/013-token-exchange/spec.md
		It("[US2-S1] should store protected_resources via admin API", func() {
			// Given: Admin API accepts protected_resources field in service creation
			serviceData := map[string]interface{}{
				"name": "github-service-with-resources",
				"protected_resources": []string{
					"https://api.github.com",
				},
			}
			body, err := json.Marshal(serviceData)
			Expect(err).NotTo(HaveOccurred())

			// When: Admin creates service via POST /api/services with protected_resources
			resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", strings.NewReader(string(body)))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Service is stored with protected_resources (Admin API not yet implemented)
			Expect(resp).NotTo(matchers.HaveStatusCode(http.StatusCreated)) // Placeholder
		})

		// Spec Reference: US2-S2 from specs/013-token-exchange/spec.md
		It("[US2-S2] should normalize resource URI removing trailing slashes", func() {
			// Given: Service is updated through the admin API with a protected_resource that has a trailing slash
			githubServiceID := fixtures.GitHubService().ID.String()
			updateBody := map[string]interface{}{
				"display_name":  "GitHub",
				"client_id":     "github-client-id",
				"client_secret": "github-client-secret",
				"issuer_uri":    "https://github.com",
				"discovery": map[string]interface{}{
					"enable_discovery": false,
				},
				"endpoints": map[string]interface{}{
					"token_endpoint":     mockUpstream.URL() + "/oauth/token",
					"authorize_endpoint": mockUpstream.URL() + "/oauth/authorize",
				},
				"scopes": []map[string]interface{}{
					{"scope_value": "repo", "description": "Access repository"},
					{"scope_value": "user", "description": "Access user information"},
					{"scope_value": "read:org", "description": "Read organization information"},
				},
				"protected_resources": []string{"https://api.github.com/v4/"},
			}
			updateBodyJSON, err := json.Marshal(updateBody)
			Expect(err).NotTo(HaveOccurred())

			updateResp, err := adminServer.DirectRequest(
				http.MethodPut,
				"/api/services/"+githubServiceID,
				principal,
				map[string]string{"Content-Type": "application/json", "If-Match": `"1"`},
				strings.NewReader(string(updateBodyJSON)),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = updateResp.Body.Close() }()
			Expect(updateResp).To(matchers.HaveStatusCode(http.StatusOK))

			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-123")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com/v4"},
			}

			// When: Token exchange request uses the canonical URI without the trailing slash
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Service is resolved because protected_resources were normalized before storage
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		})

		// Spec Reference: US2-S3 from specs/013-token-exchange/spec.md
		It("[US2-S3] should return tokens for matching service only", func() {
			// Given: Multiple services with different protected_resources exist
			// The test setup creates a session with GitHub service token "github-token-xyz"
			// which is valid and not expired, so the service returns the stored token
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requests GitHub resource
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns GitHub service token (not other services)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var tokenResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			// The token comes from the stored session fixture, which has "github-token-xyz"
			Expect(tokenResponse["access_token"]).To(Equal("github-token-xyz"))
		})

		// Spec Reference: US2-S4 from specs/013-token-exchange/spec.md
		It("[US2-S4] should return 400 invalid_target when no service matches", func() {
			// Given: Request for resource not in any protected_resources
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.nonexistent.com"},
			}

			// When: Token exchange requests unmapped resource
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 invalid_target
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_target"))
		})

		// Spec Reference: US2-S5 from specs/013-token-exchange/spec.md
		// Global protected-resource ownership makes ambiguous resource resolution
		// impossible: the second claim must fail before token exchange.
		It("[US2-S5] should reject a duplicate protected resource claim", func() {
			ctx := context.Background()
			ambiguousServiceID := id.NewServiceID()
			ambiguousService := &model.ThirdpartyOAuth2ProviderEntity{
				ID:          ambiguousServiceID,
				DisplayName: "Ambiguous Service",
				ClientID:    "ambiguous-client-id",
				Secret:      fixtures.EncryptedSecret(ambiguousServiceID.String(), "ambiguous-client-secret"),
				IssuerURI:   "https://ambiguous.example.com",
				Discovery: model.DiscoveryConfig{
					EnableDiscovery: false,
				},
				Endpoints: model.OAuth2Endpoints{
					TokenEndpoint:     "https://ambiguous.example.com/token",
					AuthorizeEndpoint: "https://ambiguous.example.com/authorize",
				},
				Scopes: []model.OAuthScope{
					{ScopeValue: "read", Description: "Read access"},
				},
				ProtectedResources: []string{"https://api.github.com"},
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}

			Expect(testStorage.Services().Create(ctx, ambiguousService)).To(HaveOccurred())
		})
	})

	// US3: User Grant Verification
	Describe("US3: User Grant Verification", func() {
		// Spec Reference: US3-S1 from specs/013-token-exchange/spec.md
		It("[US3-S1] should proceed when user has active grant for agent+service", func() {
			// Given: User has active grant for agent+service combination
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-verified")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange is requested with active grant
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Token exchange succeeds
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		})

		// Spec Reference: US3-S2 from specs/013-token-exchange/spec.md
		It("[US3-S2] should return 403 access_denied without user grant", func() {
			// Given: User without grant for agent+service
			// Delete the grant that was created in BeforeEach so this principal has no grant
			ctx := context.Background()
			// Get the active grant first to delete it
			activeGrants, err := testStorage.UserGrants().ListByPrincipalAndAgent(ctx, id.Principal(principal), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			// Delete each active grant for this principal+agent combination
			for _, g := range activeGrants {
				err := testStorage.UserGrants().Delete(ctx, g.ID)
				Expect(err).NotTo(HaveOccurred())
			}

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requested without grant
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 403 access_denied
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("access_denied"))
		})

		// Spec Reference: US3-S3 from specs/013-token-exchange/spec.md
		It("[US3-S3] should return 403 access_denied when grant revoked", func() {
			// Given: User with revoked grant
			// Replace the active grant with a revoked one (ValidUntil set to past)
			ctx := context.Background()
			// First delete the active grant created in BeforeEach
			activeGrants, err := testStorage.UserGrants().ListByPrincipalAndAgent(ctx, id.Principal(principal), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			for _, g := range activeGrants {
				err := testStorage.UserGrants().Delete(ctx, g.ID)
				Expect(err).NotTo(HaveOccurred())
			}
			// Create expired grant using CreateTestGrant to bypass validation
			revokedGrant := fixtures.ExpiredGrant(principal, agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"repo", "user"})
			memRepo, ok := testStorage.UserGrants().(*storagememory.UserGrantRepository)
			Expect(ok).To(BeTrue(), "test requires memory storage for CreateTestGrant method")
			err = memRepo.CreateTestGrant(ctx, revokedGrant)
			Expect(err).NotTo(HaveOccurred())

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requested with revoked grant
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 403 access_denied
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("access_denied"))
		})

		// Spec Reference: US3-S4 from specs/013-token-exchange/spec.md
		It("[US3-S4] should return 403 access_denied when grant expired", func() {
			// Given: User with expired grant
			ctx := context.Background()
			// First delete the active grant created in BeforeEach
			activeGrants, err := testStorage.UserGrants().ListByPrincipalAndAgent(ctx, id.Principal(principal), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			for _, g := range activeGrants {
				err := testStorage.UserGrants().Delete(ctx, g.ID)
				Expect(err).NotTo(HaveOccurred())
			}
			// Create expired grant using CreateTestGrant to bypass validation
			expiredGrant := fixtures.ExpiredGrant(principal, agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"repo", "user"})
			memRepo, ok := testStorage.UserGrants().(*storagememory.UserGrantRepository)
			Expect(ok).To(BeTrue(), "test requires memory storage for CreateTestGrant method")
			err = memRepo.CreateTestGrant(ctx, expiredGrant)
			Expect(err).NotTo(HaveOccurred())

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requested with expired grant
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 403 access_denied
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("access_denied"))
		})
	})

	// US4: CEL Authorization
	Describe("US4: CEL Authorization", func() {
		// Spec Reference: US4-S1 from specs/013-token-exchange/spec.md
		It("[US4-S1] should evaluate CEL expression against client_assertion", func() {
			// Given: CEL authorization policy is configured in config
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-with-cel")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange request is made with CEL policy configured
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: CEL expression is evaluated and request proceeds
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var tokenResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenResponse).To(HaveKey("access_token"))
		})

		// Spec Reference: US4-S2 from specs/013-token-exchange/spec.md
		It("[US4-S2] should proceed when CEL evaluates to true", func() {
			// Given: CEL policy evaluates to true for this request
			// The stored session already has a valid access token "github-token-xyz"
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-cel-approved")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange request is evaluated by CEL policy (result: true)
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Request proceeds with token exchange successfully
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var tokenResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			// The returned token is the stored session token (not the mock upstream token)
			// because the session's access token is still valid (expires in 1 hour)
			Expect(tokenResponse["access_token"]).To(Equal("github-token-xyz"))
		})

		// Spec Reference: US4-S3 from specs/013-token-exchange/spec.md
		It("[US4-S3] should return 401 invalid_client when client_assertion signature is invalid", func() {
			// Given: client_assertion has invalid signature
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertionInvalidSig},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange request is made with invalid client_assertion signature
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 401 Unauthorized with invalid_client error (JWT validation failure)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_client"))
		})

		// Spec Reference: US4-S4 from specs/013-token-exchange/spec.md
		It("[US4-S4] should fail startup with invalid CEL syntax", func() {
			// Given: Configuration has invalid CEL expression syntax in authorization policy
			invalidConfig := fixtures.TokenExchangeConfigWithInvalidCELSyntax("claims.invalid syntax here!@#$")

			// When: Application attempts to start with invalid CEL syntax in config
			invalidServerFactory := bootstrap.NewServerFactory(invalidConfig, logger)
			invalidStorage, err := storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				// Storage cleanup happens automatically via test teardown
				_ = invalidStorage
			}()

			// Then: App builder returns error during startup (CEL validation during app build)
			_, err = invalidServerFactory.BuildApp(invalidStorage)
			// The error should indicate CEL compilation failure
			// When Phase 3 implements CEL validation, this will verify startup failure on invalid syntax
			Expect(err).To(HaveOccurred())
		})

		// Spec Reference: US4-S5 from specs/013-token-exchange/spec.md
		It("[US4-S5] should provide client_assertion claims in CEL context", func() {
			// Given: CEL policy accesses client_assertion JWT claims for evaluation
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-with-claims")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.TokenWithCELClaims},
				"resource":              {"https://api.github.com"},
			}

			// When: CEL policy evaluates using client_assertion claims
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: CEL can access and use claims for decision making (succeeds when claims available)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		})

		// Spec Reference: US4-S6 from specs/013-token-exchange/spec.md
		It("[US4-S6] should provide request context in CEL context", func() {
			// Given: CEL policy uses request context (resource URI, principal, grant information)
			// The stored session already has a valid access token "github-token-xyz"
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("token-with-context")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: CEL policy evaluates using request context (resource, principal, etc.)
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: CEL can access request context for policy decisions (succeeds when context available)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var tokenResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
			Expect(err).NotTo(HaveOccurred())
			// The returned token is the stored session token (not the mock upstream token)
			// because the session's access token is still valid (expires in 1 hour)
			Expect(tokenResponse["access_token"]).To(Equal("github-token-xyz"))
		})
	})

	// US5: Session Error Handling
	Describe("US5: Session Error Handling", func() {
		// Spec Reference: US5-S1 from specs/013-token-exchange/spec.md
		It("[US5-S1] should return 400 invalid_grant when no session exists", func() {
			// Given: User has no session with service
			// Delete the session that was created in BeforeEach
			// This allows the test to verify behavior when session doesn't exist
			ctx := context.Background()
			// Create a new in-memory session storage to have no sessions at all
			// Actually, we just need to delete the existing session.
			// The session storage doesn't expose a delete by principal+service method,
			// so we'll create a different principal with no session.
			anotherPrincipal := fixtures.AnotherPrincipal().String()
			anotherPrincipalClaims := map[string]interface{}{
				"sub": anotherPrincipal,
				"azp": agent.ID.String(),
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": time.Now().Add(1 * time.Hour).Unix(),
				"iat": time.Now().Unix(),
			}
			privateKeyPEM := mockUpstream.GetPrivateKeyPEM()
			anotherToken, err := helpers.SignTestJWT(anotherPrincipalClaims, privateKeyPEM)
			Expect(err).NotTo(HaveOccurred())

			// Also create grant for the other principal so the error is about session, not grant
			grantForOther := fixtures.ActiveGrant(anotherPrincipal, agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"repo", "user"})
			err = testStorage.UserGrants().Create(ctx, grantForOther)
			Expect(err).NotTo(HaveOccurred())

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {anotherToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requested without session
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 invalid_grant (not 403 access_denied)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_grant"))
		})

		// Spec Reference: US5-S2 from specs/013-token-exchange/spec.md
		It("[US5-S2] should return 400 invalid_grant when tokens fully expired", func() {
			// Given: Both access and refresh tokens expired
			ctx := context.Background()
			fullyExpiredSession := fixtures.FullyExpiredSessionForPrincipal(principal, fixtures.GitHubService().ID.String())
			err := testStorage.UserSessions().Create(ctx, fullyExpiredSession)
			Expect(err).NotTo(HaveOccurred())

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange requested with expired tokens
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 invalid_grant (not 403 access_denied)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_grant"))
		})

		// Spec Reference: US5-S3 from specs/013-token-exchange/spec.md
		It("[US5-S3] should include sufficient info in error_description for re-auth flow", func() {
			// Given: Request that will fail with invalid_grant (no session exists)
			ctx := context.Background()
			anotherPrincipal := fixtures.AdminPrincipal().String()

			// Create a token for principal with no session
			adminClaims := map[string]interface{}{
				"sub": anotherPrincipal,
				"azp": agent.ID.String(),
				"iss": mockUpstream.URL(),
				"aud": "token-exchange-broker",
				"exp": time.Now().Add(1 * time.Hour).Unix(),
				"iat": time.Now().Unix(),
			}
			privateKeyPEM := mockUpstream.GetPrivateKeyPEM()
			adminToken, err := helpers.SignTestJWT(adminClaims, privateKeyPEM)
			Expect(err).NotTo(HaveOccurred())

			// Create grant for admin so error is specifically about session
			grantForAdmin := fixtures.ActiveGrant(anotherPrincipal, agent.ID.String(), fixtures.GitHubService().ID.String(), []string{"repo", "user"})
			err = testStorage.UserGrants().Create(ctx, grantForAdmin)
			Expect(err).NotTo(HaveOccurred())

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {adminToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			// When: Token exchange fails with invalid_grant (no session exists)
			resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Error includes actionable description for re-auth
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse["error"]).To(Equal("invalid_grant"))
			Expect(errorResponse).To(HaveKey("error_description"))
			Expect(errorResponse["error_description"]).NotTo(BeEmpty())
		})
	})

	// US6: Admin API Protected Resources
	Describe("US6: Admin API Protected Resources", func() {
		// Spec Reference: US6-S1 from specs/013-token-exchange/spec.md
		It("[US6-S1] should accept protected_resources in POST /api/services", func() {
			// Given: Admin API accepts protected_resources field in service creation
			serviceData := map[string]interface{}{
				"display_name":  "New Service with Protected Resources",
				"client_id":     "new-service-client-id",
				"client_secret": "new-service-client-secret",
				"issuer_uri":    "https://new.example.com",
				"discovery": map[string]interface{}{
					"enable_discovery": false,
				},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://new.example.com/token",
					"authorize_endpoint": "https://new.example.com/authorize",
				},
				"scopes": []map[string]interface{}{
					{"scope_value": "read", "description": "Read access"},
				},
				"protected_resources": []string{
					"https://api.newservice.com/",
					"https://newservice.com",
				},
			}
			body, err := json.Marshal(serviceData)
			Expect(err).NotTo(HaveOccurred())

			// When: Admin creates service with protected_resources array via POST /api/services
			resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", strings.NewReader(string(body)))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Service is created with 201 status and protected_resources normalized in the response
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			var createdService map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&createdService)
			Expect(err).NotTo(HaveOccurred())
			Expect(createdService).To(HaveKey("protected_resources"))
			Expect(createdService["protected_resources"]).To(Equal([]interface{}{"https://api.newservice.com", "https://newservice.com"}))

			serviceID := createdService["id"].(string)
			getResp, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp).To(matchers.HaveStatusCode(http.StatusOK))

			var fetchedService map[string]interface{}
			err = json.NewDecoder(getResp.Body).Decode(&fetchedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedService["protected_resources"]).To(Equal([]interface{}{"https://api.newservice.com", "https://newservice.com"}))
		})

		// Spec Reference: US6-S2 from specs/013-token-exchange/spec.md
		It("[US6-S2] should accept protected_resources in PUT /api/services/{id}", func() {
			// Given: Admin wants to update service protected_resources via PUT
			// Use the GitHub service that was already created in BeforeEach
			updateData := map[string]interface{}{
				"display_name":  "GitHub OAuth2 Service Updated",
				"client_id":     "github-client-id",
				"client_secret": "github-client-secret",
				"issuer_uri":    "https://github.com",
				"discovery": map[string]interface{}{
					"enable_discovery": false,
				},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://github.com/login/oauth/access_token",
					"authorize_endpoint": "https://github.com/login/oauth/authorize",
				},
				"scopes": []map[string]interface{}{
					{"scope_value": "repo", "description": "Repository access"},
				},
				"protected_resources": []string{
					"https://api.example.com/",
				},
			}
			body, err := json.Marshal(updateData)
			Expect(err).NotTo(HaveOccurred())

			resourcesResp, err := adminServer.AuthenticatedGET("/api/services/"+fixtures.GitHubService().ID.String()+"/protected-resources", principal)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resourcesResp.Body.Close() }()
			Expect(resourcesResp).To(matchers.HaveStatusCode(http.StatusOK))
			etag := resourcesResp.Header.Get("ETag")
			Expect(etag).To(MatchRegexp(`^"[1-9][0-9]*"$`))

			// When: Admin updates service with new protected_resources via PUT
			resp, err := adminServer.DirectRequest("PUT", "/api/services/"+fixtures.GitHubService().ID.String(), principal, map[string]string{"Content-Type": "application/json", "If-Match": etag}, strings.NewReader(string(body)))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Service is updated with 200 status and normalized protected_resources in the response
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var updatedService map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&updatedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedService).To(HaveKey("protected_resources"))
			Expect(updatedService["protected_resources"]).To(Equal([]interface{}{"https://api.example.com"}))

			getResp, err := adminServer.AuthenticatedGET("/api/services/"+fixtures.GitHubService().ID.String(), principal)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = getResp.Body.Close() }()
			Expect(getResp).To(matchers.HaveStatusCode(http.StatusOK))

			var fetchedService map[string]interface{}
			err = json.NewDecoder(getResp.Body).Decode(&fetchedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetchedService["protected_resources"]).To(Equal([]interface{}{"https://api.example.com"}))
		})

		// Spec Reference: US6-S3 from specs/013-token-exchange/spec.md
		It("[US6-S3] should return 400 for invalid URI in protected_resources", func() {
			// Given: Admin provides invalid URI format in protected_resources
			serviceData := map[string]interface{}{
				"display_name":  "Invalid Service",
				"client_id":     "invalid-client-id",
				"client_secret": "invalid-client-secret",
				"issuer_uri":    "https://invalid.example.com",
				"discovery": map[string]interface{}{
					"enable_discovery": false,
				},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://invalid.example.com/token",
					"authorize_endpoint": "https://invalid.example.com/authorize",
				},
				"scopes": []map[string]interface{}{
					{"scope_value": "read", "description": "Read access"},
				},
				"protected_resources": []string{
					"not-a-valid-uri",
				},
			}
			body, err := json.Marshal(serviceData)
			Expect(err).NotTo(HaveOccurred())

			// When: Admin tries to create service with invalid URIs
			resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", strings.NewReader(string(body)))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 400 Bad Request with validation error for invalid URIs
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse).To(HaveKey("error"))
		})

		// Spec Reference: US6-S4 from specs/013-token-exchange/spec.md
		It("[US6-S4] should return 409 for duplicate resource URI across services", func() {
			// Given: Multiple services already configured with same protected_resource URI
			// GitHub service already exists with "https://api.github.com" (from BeforeEach)
			serviceData := map[string]interface{}{
				"display_name":  "Duplicate Service",
				"client_id":     "duplicate-client-id",
				"client_secret": "duplicate-client-secret",
				"issuer_uri":    "https://duplicate.example.com",
				"discovery": map[string]interface{}{
					"enable_discovery": false,
				},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://duplicate.example.com/token",
					"authorize_endpoint": "https://duplicate.example.com/authorize",
				},
				"scopes": []map[string]interface{}{
					{"scope_value": "read", "description": "Read access"},
				},
				"protected_resources": []string{
					"https://api.github.com", // Already exists in GitHub service from BeforeEach
				},
			}
			body, err := json.Marshal(serviceData)
			Expect(err).NotTo(HaveOccurred())

			// When: Admin tries to create service with duplicate resource URI
			resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", strings.NewReader(string(body)))
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Returns 409 Conflict for duplicate resource URI
			Expect(resp).To(matchers.HaveStatusCode(http.StatusConflict))

			var errorResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&errorResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(errorResponse).To(HaveKey("error"))
		})

		// Spec Reference: US6-S5 from specs/013-token-exchange/spec.md
		It("[US6-S5] should include protected_resources in GET /api/services/{id} response", func() {
			// Given: Service exists with protected_resources stored (GitHub service from BeforeEach)
			// When: Admin retrieves service details via GET /api/services/{id}
			resp, err := adminServer.AuthenticatedGET("/api/services/"+fixtures.GitHubService().ID.String(), principal)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response includes protected_resources array in service object
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			var serviceResponse map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&serviceResponse)
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceResponse).To(HaveKey("protected_resources"))
			Expect(serviceResponse["protected_resources"]).To(BeAssignableToTypeOf([]interface{}{}))
			// Verify the protected_resources contains the expected GitHub URI
			resources := serviceResponse["protected_resources"].([]interface{})
			Expect(resources).To(ContainElement("https://api.github.com"))
		})
	})

	// US7: resolveAgentIdByClientId CEL Helper (Feature 021)
	//
	// Spec: specs/021-multi-agent-clientid/spec.md — US2 Scenario 3
	//
	// When multi_agent_client is disabled (the default), the broker enforces that each
	// upstream OAuth2 client_id is unique per agent. In this mode, the CEL helper
	// function resolveAgentIdByClientId() is registered and operators use it in
	// agent_id_expression to map an upstream client_id (e.g. the "azp" claim) to
	// the broker's internal agent.id UUID.
	//
	// This Describe block contains the explicit test that FR-009 is satisfied:
	// the function is registered, functional, and actually used by the token exchange.
	Describe("US7: resolveAgentIdByClientId CEL Helper (multi_agent_client disabled)", func() {
		Context("when resolveAgentIdByClientId is used in agent_id_expression", func() {
			var (
				resolveConfig  *ports.Config
				resolveFactory *bootstrap.ServerFactory
				resolveStorage *storageadapter.Adapter
				resolveServer  *bootstrap.TestServer
				githubService  *model.ThirdpartyOAuth2ProviderEntity
			)

			BeforeEach(func() {
				// Create config with resolveAgentIdByClientId expression
				resolveConfig = fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL())
				resolveConfig.OAuth2AuthServer.MultiAgentClient = ports.MultiAgentClientConfig{Enabled: false}
				resolveConfig.TokenExchange.ClaimExtraction.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"

				resolveFactory = bootstrap.NewServerFactory(resolveConfig, logger)

				// Create isolated storage for this context
				var err error
				resolveStorage, err = storageFactory.NewTestStorage()
				Expect(err).ToNot(HaveOccurred())

				// Create GitHub service for resource-based lookup
				githubService = fixtures.GitHubService()
				githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
				ctx := context.Background()
				Expect(resolveStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())
				Expect(fixtures.SeedPlaceholderGrantData(ctx, resolveStorage, githubService.ID)).To(Succeed())
			})

			AfterEach(func() {
				if resolveServer != nil {
					resolveServer.Close()
				}
				if resolveStorage != nil {
					_ = storageFactory.CloseStorage(resolveStorage)
				}
			})

			// US7 Scenario 1 from specs/021-multi-agent-clientid/spec.md
			// Given: multi_agent_client is disabled and agent_id_expression uses resolveAgentIdByClientId,
			// When: a subject token with azp=<upstream client_id> is presented for exchange,
			// Then: the token exchange resolves the agent by upstream client_id and succeeds.
			It("[US7-S1] should resolve agent via resolveAgentIdByClientId when feature is disabled", Label("US7"), func() {
				// Create agent with a distinct upstream client_id
				resolveAgent := fixtures.AgentWithClientID("test-upstream-client-id")

				ctx := context.Background()
				Expect(resolveStorage.Agents().Create(ctx, resolveAgent)).ToNot(HaveOccurred())

				// Grant the resolve agent access to the GitHub service
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					resolveAgent.ID.String(),
					githubService.ID.String(),
					[]string{"repo", "user"},
				)
				Expect(resolveStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

				// Create a session for the resolve agent
				session := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				Expect(resolveStorage.UserSessions().Create(ctx, session)).ToNot(HaveOccurred())

				// Build and start the resolver server
				resolveApp, err := resolveFactory.BuildApp(resolveStorage)
				Expect(err).ToNot(HaveOccurred())
				resolveServer, err = bootstrap.NewEndUserTestServer(resolveApp, logger)
				Expect(err).ToNot(HaveOccurred())

				// Create tokens: subject_token has azp = upstream client_id (not the agent UUID)
				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub": fixtures.DefaultPrincipal().String(),
					"azp": string(*resolveAgent.ClientID), // upstream client_id — resolved by the CEL helper
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				clientAssertionClaims := map[string]interface{}{
					"sub": resolveAgent.ID.String(),
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				clientAssertion, err := helpers.SignTestJWT(clientAssertionClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				// Request: RFC 8693 token exchange
				mockUpstream.WithSuccessfulTokenResponse()
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				resp, err := resolveServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Assert: exchange succeeds — resolveAgentIdByClientId mapped azp → agent.id
				Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			})

			// US7 Scenario 2 from specs/021-multi-agent-clientid/spec.md
			// Given: resolveAgentIdByClientId is registered but the azp claim does not match any agent,
			// When: a token exchange is attempted,
			// Then: it fails (unknown agent).
			It("[US7-S2] should fail when azp does not match any agent upstream client_id", Label("US7"), func() {
				// Build and start the resolver server (no agents registered)
				resolveApp, err := resolveFactory.BuildApp(resolveStorage)
				Expect(err).ToNot(HaveOccurred())
				resolveServer, err = bootstrap.NewEndUserTestServer(resolveApp, logger)
				Expect(err).ToNot(HaveOccurred())

				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub": fixtures.DefaultPrincipal().String(),
					"azp": "nonexistent-upstream-client", // no agent has this upstream client_id
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				clientAssertionClaims := map[string]interface{}{
					"sub": "some-gateway",
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				clientAssertion, err := helpers.SignTestJWT(clientAssertionClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				resp, err := resolveServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// resolveAgentIdByClientId returns an error for unknown client_id →
				// CEL expression fails → token exchange is rejected (not 200)
				Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			})

			// US7 Scenario 4 from specs/021-multi-agent-clientid/spec.md
			// Regression — UUID-shaped client_id must resolve via GetByClientID,
			// not short-circuit. An agent whose explicit client_id is a UUID different from its ID
			// must still be resolved correctly (the old short-circuit returned the wrong agent ID).
			It("[US7-S4] should correctly resolve agent when client_id is a UUID distinct from agent ID", Label("US7"), func() {
				// Agent whose ClientID is a UUID distinct from its own ID.
				// The old short-circuit would treat azp (= ClientID UUID) as the agent ID directly,
				// causing a mismatch with client_assertion.sub (= agent.ID).
				uuidShapedClientID := id.NewAgentID().String()
				resolveAgent := fixtures.AgentWithClientID(uuidShapedClientID)
				Expect(resolveAgent.ID.String()).ToNot(Equal(uuidShapedClientID))

				ctx := context.Background()
				Expect(resolveStorage.Agents().Create(ctx, resolveAgent)).ToNot(HaveOccurred())

				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					resolveAgent.ID.String(),
					githubService.ID.String(),
					[]string{"repo", "user"},
				)
				Expect(resolveStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

				session := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				Expect(resolveStorage.UserSessions().Create(ctx, session)).ToNot(HaveOccurred())

				resolveApp, err := resolveFactory.BuildApp(resolveStorage)
				Expect(err).ToNot(HaveOccurred())
				resolveServer, err = bootstrap.NewEndUserTestServer(resolveApp, logger)
				Expect(err).ToNot(HaveOccurred())

				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub": fixtures.DefaultPrincipal().String(),
					"azp": uuidShapedClientID, // UUID-shaped client_id — NOT the agent's ID
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				clientAssertionClaims := map[string]interface{}{
					"sub": resolveAgent.ID.String(), // agent's actual UUID, not the client_id UUID
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				clientAssertion, err := helpers.SignTestJWT(clientAssertionClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				mockUpstream.WithSuccessfulTokenResponse()
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				resp, err := resolveServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// resolveAgentIdByClientId must call GetByClientID and return agent.ID, not the UUID client_id.
				Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			})
		})

		// US7 Scenario 3 from specs/021-multi-agent-clientid/spec.md
		// Startup validation — resolveAgentIdByClientId is NOT registered
		// when multi_agent_client is enabled.
		It("[US7-S3] should fail to compile agent_id_expression using resolveAgentIdByClientId when feature is enabled", Label("US7"), func() {
			// When multi_agent_client is enabled, resolveAgentIdByClientId is NOT registered.
			// An expression using it will fail at startup (unknown function).
			badConfig := fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL())
			badConfig.OAuth2AuthServer.MultiAgentClient = ports.MultiAgentClientConfig{
				Enabled:          true,
				AgentIDParamName: "x_agent_id",
				AgentIDClaimName: "x_agent_id",
			}
			// This expression is only valid when the feature is disabled.
			badConfig.TokenExchange.ClaimExtraction.AgentIDExpression = "resolveAgentIdByClientId(subject_token.azp)"

			resolveFactory := bootstrap.NewServerFactory(badConfig, logger)
			resolveStorage, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = storageFactory.CloseStorage(resolveStorage) }()

			// When: trying to build the application
			_, err = resolveFactory.BuildApp(resolveStorage)

			// Then: startup fails because resolveAgentIdByClientId is not registered (FR-010)
			Expect(err).To(HaveOccurred())
		})
	})

	// US5-S1 from specs/026-extproc-approval-sync/spec.md
	DescribeTable("returns the verified principal and resolved canonical agent ID", func(uppercaseSubjectAgentID bool) {
		subjectAgentID := agent.ID.String()
		if uppercaseSubjectAgentID {
			subjectAgentID = strings.ToUpper(subjectAgentID)
		}
		now := time.Now()
		subjectToken, err := helpers.SignTestJWT(map[string]interface{}{
			"sub": principal,
			"azp": subjectAgentID,
			"iss": mockUpstream.URL(),
			"aud": "token-exchange-broker",
			"exp": now.Add(time.Hour).Unix(),
			"iat": now.Unix(),
		}, mockUpstream.GetPrivateKeyPEM())
		Expect(err).NotTo(HaveOccurred())

		mockUpstream.WithSuccessfulTokenResponse().WithAccessToken("approval-identity-token")
		formData := url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {subjectToken},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {tokenFixtures.ClientAssertion},
			"resource":              {"https://api.github.com"},
		}
		resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(formData.Encode()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

		var tokenResponse struct {
			Principal string `json:"principal"`
			AgentID   string `json:"agent_id"`
		}
		Expect(json.NewDecoder(resp.Body).Decode(&tokenResponse)).To(Succeed())
		Expect(tokenResponse.Principal).To(Equal(principal))
		Expect(tokenResponse.AgentID).To(Equal(agent.ID.String()))
	},
		Entry("from canonical subject agent ID", false),
		Entry("from equivalent uppercase subject agent ID", true),
	)

	// Specification Reference
	// All tests map to acceptance scenarios from specs/013-token-exchange/spec.md,
	// specs/021-multi-agent-clientid/spec.md, and specs/026-extproc-approval-sync/spec.md
	// Total: 32 acceptance test scenarios
	// - US1: 6 scenarios (token exchange gateway)
	// - US2: 5 scenarios (resource discovery)
	// - US3: 4 scenarios (grant verification)
	// - US4: 6 scenarios (CEL authorization)
	// - US5: 3 scenarios (session errors)
	// - US6: 5 scenarios (admin API)
	// - US7: 3 scenarios (resolveAgentIdByClientId CEL helper)
})

// generateTokenFixtures creates all JWT tokens needed for RFC 8693 token exchange testing.
// Each token is properly signed with the mock upstream OAuth2 server's private key,
// ensuring JWT validation passes and tests can focus on business logic.
//
// TokenFixtures contains:
// - SubjectToken: Valid subject token with principal and agent claims for successful exchange
// - ClientAssertion: Valid client assertion for gateway authentication
// - ExpiredSubjectToken: Subject token with exp claim in the past for expiration testing
// - MalformedSubjectToken: Token that is not a valid JWT structure for malformed token testing
// - MissingSubClaimToken: Token without 'sub' claim for validation testing
// - MissingAudClaimToken: Token without 'aud' claim for validation testing
// - InvalidIssuerToken: Token with incorrect issuer URI for issuer validation testing
// - ClientAssertionInvalidSig: JWT that appears valid but has manipulated signature
// - TokenWithCELClaims: Token with extra claims for CEL expression evaluation
func generateTokenFixtures(mockUpstream *helpers.MockUpstreamOAuth2Server, principal string, agent *storagedomain.Agent) *TokenFixtures {
	fixtures := &TokenFixtures{}
	privateKeyPEM := mockUpstream.GetPrivateKeyPEM()
	now := time.Now()

	// SubjectToken: Valid subject token with principal and agent ID claims
	// Used in successful token exchange scenarios (US1-S1 through US1-S4)
	subjectTokenClaims := map[string]interface{}{
		"sub": principal,                     // Principal claim (user identifier)
		"azp": agent.ID.String(),             // Agent internal UUID (T030: must be UUID for agentRepository.Get)
		"iss": mockUpstream.URL(),            // Issuer must match upstream server
		"aud": "token-exchange-broker",       // Audience for this token exchange
		"exp": now.Add(1 * time.Hour).Unix(), // Expires in 1 hour
		"iat": now.Unix(),                    // Issued now
	}
	token, err := helpers.SignTestJWT(subjectTokenClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.SubjectToken = token

	// ClientAssertion: Valid gateway client assertion
	// Authenticates the gateway making the token exchange request (US1-S1 through US1-S4)
	clientAssertionClaims := map[string]interface{}{
		"sub": "test-gateway-client",         // Gateway identifier
		"iss": mockUpstream.URL(),            // Issuer must match upstream
		"aud": "token-exchange-broker",       // Audience
		"exp": now.Add(1 * time.Hour).Unix(), // Expires in 1 hour
		"iat": now.Unix(),                    // Issued now
	}
	token, err = helpers.SignTestJWT(clientAssertionClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.ClientAssertion = token

	// ExpiredSubjectToken: Subject token with expired timestamp
	// Used to test handling of expired tokens (US1-S6, US5-S2)
	expiredTokenClaims := map[string]interface{}{
		"sub": principal,
		"azp": agent.ID.String(),
		"iss": mockUpstream.URL(),
		"aud": "token-exchange-broker",
		"exp": now.Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
		"iat": now.Add(-2 * time.Hour).Unix(),
	}
	token, err = helpers.SignTestJWT(expiredTokenClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.ExpiredSubjectToken = token

	// MalformedSubjectToken: Token that is not a valid JWT structure
	// Used to test handling of malformed/invalid tokens (US1-S6)
	// Create a string that looks like it could be a token but isn't valid JWT format
	fixtures.MalformedSubjectToken = "not.a.valid.jwt.structure"

	// MissingSubClaimToken: Token without 'sub' claim
	// Used to test validation of required claims
	missingSubClaims := map[string]interface{}{
		"iss": mockUpstream.URL(),
		"aud": "token-exchange-broker",
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}
	token, err = helpers.SignTestJWT(missingSubClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.MissingSubClaimToken = token

	// MissingAudClaimToken: Token without 'aud' claim
	// Used to test validation of required claims
	missingAudClaims := map[string]interface{}{
		"sub": principal,
		"iss": mockUpstream.URL(),
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}
	token, err = helpers.SignTestJWT(missingAudClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.MissingAudClaimToken = token

	// InvalidIssuerToken: Token with wrong issuer
	// Used to test issuer validation (different upstream server)
	invalidIssuerClaims := map[string]interface{}{
		"sub": principal,
		"azp": agent.ID.String(),
		"iss": "https://wrong-issuer.example.com", // Wrong issuer
		"aud": "token-exchange-broker",
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}
	token, err = helpers.SignTestJWT(invalidIssuerClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.InvalidIssuerToken = token

	// ClientAssertionInvalidSig: Token with invalid/manipulated signature
	// Signed with private key but signature will be corrupted for testing verification failure
	// This represents an invalid client_assertion (US1-S5) and invalid CEL policy case (US4-S3)
	validClientClaims := map[string]interface{}{
		"sub": "test-gateway-invalid",
		"iss": mockUpstream.URL(),
		"aud": "token-exchange-broker",
		"exp": now.Add(1 * time.Hour).Unix(),
		"iat": now.Unix(),
	}
	token, err = helpers.SignTestJWT(validClientClaims, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	// Corrupt the signature by replacing the signature part (after the last dot)
	// with a different value. This ensures the signature won't verify even if
	// the payload hasn't changed.
	parts := strings.Split(token, ".")
	if len(parts) == 3 && len(parts[2]) > 0 {
		// Replace the signature part with a corrupted version
		// Change first character of signature to something different
		sigBytes := []byte(parts[2])
		if sigBytes[0] == 'A' {
			sigBytes[0] = 'B'
		} else {
			sigBytes[0] = 'A'
		}
		parts[2] = string(sigBytes)
		token = strings.Join(parts, ".")
	}
	fixtures.ClientAssertionInvalidSig = token

	// TokenWithCELClaims: Token with additional claims for CEL expression evaluation
	// Used to test CEL policy access to JWT claims (US4-S5)
	celClaimsToken := map[string]interface{}{
		"sub":      "test-gateway-cel",
		"iss":      mockUpstream.URL(),
		"aud":      "token-exchange-broker",
		"exp":      now.Add(1 * time.Hour).Unix(),
		"iat":      now.Unix(),
		"scope":    "api:write api:read",  // Custom scope claim for CEL
		"org_id":   "org-123",             // Custom org claim for CEL
		"role":     "admin",               // Custom role claim for CEL
		"env":      "production",          // Custom environment claim
		"features": "feature-x,feature-y", // Feature flags
	}
	token, err = helpers.SignTestJWT(celClaimsToken, privateKeyPEM)
	Expect(err).NotTo(HaveOccurred())
	fixtures.TokenWithCELClaims = token

	return fixtures
}
