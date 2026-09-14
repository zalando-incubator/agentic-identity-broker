package e2e_test

// Revoke Agent Grant E2E Tests
//
// This test suite validates the complete implementation of revoke agent consent
// following Principle XIII (End-to-End Acceptance Testing & Spec Traceability).
//
// Each It() block maps to exactly ONE acceptance scenario from spec.md:
// - US1-S3, US2-S3: Successful revocation (204 response)
// - Edge: not-found (404 when no grant)
// - SR-001: Cross-principal isolation (404 for wrong principal)
// - SR-001: Auth required (401 when no principal header)
// - API validation: 400 for invalid UUID
// - US1-S4, US2-S3: Grant removed from list after revocation
// - US3-S1: Token exchange rejected after revocation
// - US3-S2: Immediate re-attempt denied after revocation

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Revoke Agent Grant", func() {
	var (
		enduserServer  *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		logger         *slog.Logger

		agent        *storagedomain.Agent
		anotherAgent *storagedomain.Agent
		principal    string
		agentPath    string
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)

		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)

		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred(), "Failed to create test storage")

		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred(), "Failed to build app instance")

		enduserServer, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred(), "Failed to create enduser test server")

		principal = fixtures.DefaultPrincipal().String()

		// Create two agents: one with an active grant, one without
		ctx := context.Background()
		agent = fixtures.ValidAgent()
		anotherAgent = fixtures.AnotherAgent()

		err = testStorage.Agents().Create(ctx, agent)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test agent")

		err = testStorage.Agents().Create(ctx, anotherAgent)
		Expect(err).ToNot(HaveOccurred(), "Failed to create another test agent")

		// Create a GitHub service fixture for the grant
		service := fixtures.GitHubService()
		err = testStorage.Services().Create(ctx, service)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test service")
		err = fixtures.SeedPlaceholderGrantData(ctx, testStorage, service.ID)
		Expect(err).ToNot(HaveOccurred(), "Failed to seed permission set coverage")

		// Create an active grant for (principal, agent) — the grant to be revoked
		grant := fixtures.ActiveGrant(principal, agent.ID.String(), service.ID.String(), []string{"repo", "user"})
		err = testStorage.UserGrants().Create(ctx, grant)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test grant")

		agentPath = "/api/consent/agents/" + agent.ID.String() + "/grants"
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US1-S3, US2-S3 from specs/022-revoke-agent-consent/spec.md
	It("returns 204 when grant exists and is owned by principal", func() {
		// Given: An active grant exists for (principal, agent)
		// When: Principal sends DELETE request for their grant
		resp, err := enduserServer.DirectRequest("DELETE", agentPath, principal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Then: Grant is permanently deleted, 204 returned
		// SEMANTIC FAILURE IN RED PHASE: DELETE route not yet registered (returns 404/405)
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))
	})

	// Scenario Edge: not-found from specs/022-revoke-agent-consent/spec.md
	It("returns 404 when no grant exists for the principal+agent", func() {
		// Given: anotherAgent has no grant for this principal
		// When: Principal sends DELETE for a non-existent grant
		path := "/api/consent/agents/" + anotherAgent.ID.String() + "/grants"
		resp, err := enduserServer.DirectRequest("DELETE", path, principal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Then: 404 Not Found is returned
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	// Scenario SR-001, Edge from specs/022-revoke-agent-consent/spec.md
	It("returns 404 when grant exists for different principal", func() {
		// Given: The grant is owned by DefaultPrincipal
		// When: AnotherPrincipal sends DELETE for the same agent
		anotherPrincipal := fixtures.AnotherPrincipal().String()
		resp, err := enduserServer.DirectRequest("DELETE", agentPath, anotherPrincipal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Then: 404 Not Found (principal-scoped query leaks no existence information per SR-001)
		Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
	})

	// Scenario SR-001 from specs/022-revoke-agent-consent/spec.md
	It("returns 401 when no principal header is present", func() {
		// Given: No X-Remote-User header is included
		// When: Unauthenticated DELETE request is sent
		// Principal check MUST happen before UUID validation
		resp, err := enduserServer.DirectRequest("DELETE", agentPath, "", nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Then: 401 Unauthorized
		Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
	})

	// Scenario API validation from specs/022-revoke-agent-consent/spec.md
	It("returns 400 when agent-id is not a valid UUID", func() {
		// Given: An authenticated principal
		// When: DELETE request is sent with a non-UUID agent-id
		// Principal check happens before UUID parse, but 400 returned for bad format
		path := "/api/consent/agents/not-a-valid-uuid/grants"
		resp, err := enduserServer.DirectRequest("DELETE", path, principal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Then: 400 Bad Request
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	// Scenario US1-S4, US2-S3 from specs/022-revoke-agent-consent/spec.md
	It("removes grant from list after successful revocation", func() {
		// Given: An active grant exists for (principal, agent)

		// When: Principal revokes the grant
		resp, err := enduserServer.DirectRequest("DELETE", agentPath, principal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// SEMANTIC FAILURE IN RED PHASE: DELETE route not yet registered
		Expect(resp.StatusCode).To(Equal(http.StatusNoContent))

		// Then: GET /api/consent/agent/{agent-id}/grants returns {"data": null}
		// The endpoint returns 200 with a null data field (not 404) because the agent
		// still exists — only the grant was deleted. Response shape is {"data": GrantResponse|null}.
		listResp, err := enduserServer.AuthenticatedGET(agentPath, principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = listResp.Body.Close() }()
		Expect(listResp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		err = json.NewDecoder(listResp.Body).Decode(&body)
		Expect(err).NotTo(HaveOccurred())
		// data field must be null (not an array — the endpoint returns a single nullable grant object)
		Expect(body["data"]).To(BeNil(), "grant data should be null after revocation")
	})

	// US3: Token exchange enforcement after revocation
	Context("US3: Token exchange is blocked after grant revocation", func() {
		var (
			tokenExchangeServer *bootstrap.TestServer
			tokenFixtures       *TokenFixtures // defined in token_exchange_test.go (same package)
		)

		BeforeEach(func() {
			ctx := context.Background()

			// Add an OAuth2 session so token exchange can serve stored GitHub tokens
			Expect(testStorage.UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal))).
				ToNot(HaveOccurred(), "Failed to create user session for token exchange")

			// Build a server with token exchange config (same storage, different config)
			tokenExchangeConfig := fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.Server.URL)
			app, err := bootstrap.NewServerFactory(tokenExchangeConfig, logger).BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred(), "Failed to build token exchange app")

			tokenExchangeServer, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred(), "Failed to create token exchange server")

			// Generate real signed JWTs for the token exchange flow
			tokenFixtures = generateTokenFixtures(mockUpstream, principal, agent)
		})

		AfterEach(func() {
			if tokenExchangeServer != nil {
				tokenExchangeServer.Close()
			}
		})

		// Scenario US3-S1 from specs/022-revoke-agent-consent/spec.md
		It("token exchange is rejected after grant is revoked", func() {
			// Given: User revokes consent for the agent
			revokeResp, err := tokenExchangeServer.DirectRequest("DELETE", agentPath, principal, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = revokeResp.Body.Close() }()

			// SEMANTIC FAILURE IN RED PHASE: DELETE route not yet implemented (returns 404/405)
			Expect(revokeResp.StatusCode).To(Equal(http.StatusNoContent))

			// When: The revoked agent attempts a token exchange for the same user
			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			tokenResp, err := tokenExchangeServer.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(data.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = tokenResp.Body.Close() }()

			// Then: Authorization check fails — no grant means no token issued
			Expect(tokenResp).To(matchers.HaveStatusCode(http.StatusForbidden))
		})

		// Scenario US3-S2 from specs/022-revoke-agent-consent/spec.md
		It("immediate token exchange re-attempt is rejected", func() {
			// Given: Token exchange succeeds while grant is active
			mockUpstream.WithSuccessfulTokenResponse().WithAccessToken("github-token-pre-revoke")

			data := url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {tokenFixtures.SubjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {tokenFixtures.ClientAssertion},
				"resource":              {"https://api.github.com"},
			}

			firstResp, err := tokenExchangeServer.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(data.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = firstResp.Body.Close() }()
			Expect(firstResp).To(matchers.HaveStatusCode(http.StatusOK))

			// When: Grant is revoked
			revokeResp, err := tokenExchangeServer.DirectRequest("DELETE", agentPath, principal, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = revokeResp.Body.Close() }()

			// SEMANTIC FAILURE IN RED PHASE: DELETE route not yet implemented (returns 404/405)
			Expect(revokeResp.StatusCode).To(Equal(http.StatusNoContent))

			// Then: Immediate re-attempt after revocation is denied
			secondResp, err := tokenExchangeServer.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(data.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = secondResp.Body.Close() }()

			Expect(secondResp).To(matchers.HaveStatusCode(http.StatusForbidden))
		})
	})
})
