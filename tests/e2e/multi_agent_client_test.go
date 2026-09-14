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
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Multi-Agent Client Delegation", func() {
	// Shared infrastructure for all multi-agent tests.
	// Each test group (US1, US2, US3) builds its own server with appropriate config.
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// ──────────────────────────────────────────────────────────────────
	// User Story 1: Multiple Agents Share One Upstream OAuth2 Client
	// ──────────────────────────────────────────────────────────────────
	Describe("US1: Multiple Agents Share One Upstream OAuth2 Client", func() {
		var (
			alpha *storagedomain.Agent
			beta  *storagedomain.Agent
		)

		BeforeEach(func() {
			alpha = fixtures.MultiAgentAlpha()
			beta = fixtures.MultiAgentBeta()
			ctx := context.Background()
			Expect(testStorage.Agents().Create(ctx, alpha)).ToNot(HaveOccurred())
			Expect(testStorage.Agents().Create(ctx, beta)).ToNot(HaveOccurred())
		})

		Context("when multi-agent client sharing is enabled", func() {
			var server *bootstrap.TestServer

			BeforeEach(func() {
				ctx := context.Background()
				// Register GitHub service and create an active session for the default principal.
				// US1 S4 (claim mismatch) requires an active session to reach claim validation.
				githubService := fixtures.GitHubService()
				Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())
				session := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				Expect(testStorage.UserSessions().Create(ctx, session)).ToNot(HaveOccurred())

				config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())
				sf := bootstrap.NewServerFactory(config, logger)
				appInstance, err := sf.BuildApp(testStorage)
				Expect(err).ToNot(HaveOccurred())
				server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				if server != nil {
					server.Close()
				}
			})

			// US1 Scenario 1 from specs/021-multi-agent-clientid/spec.md
			It("should append agent ID param to upstream authorize URL", Label("US1"), func() {
				// Given: Feature enabled, agent has active grant
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					alpha.ID.String(),
					fixtures.GitHubService().ID.String(),
					[]string{"read"},
				)
				Expect(testStorage.UserGrants().Create(context.Background(), grant)).ToNot(HaveOccurred())

				// When: Authorization request using agent internal ID as client_id
				resp, err := server.AuthenticatedGET(
					"/oauth2/authorize?client_id="+alpha.ID.String()+
						"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz",
					fixtures.DefaultPrincipal().String(),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Upstream redirect URL contains agent ID param (x_agent_id=<agent.id>)
				redirectURL, err := helpers.ExtractRedirectURL(resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(redirectURL.Query().Get("x_agent_id")).To(Equal(alpha.ID.String()))
			})

			// US1 Scenario 2 from specs/021-multi-agent-clientid/spec.md
			It("should resolve client_id against agent.id not agent.client_id", Label("US1"), func() {
				// Given: Agent registered with client_id="shared-upstream" but internal ID is a UUID
				// Active grant for the default principal
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					alpha.ID.String(),
					fixtures.GitHubService().ID.String(),
					[]string{"read"},
				)
				Expect(testStorage.UserGrants().Create(context.Background(), grant)).ToNot(HaveOccurred())

				// When: Authorization request uses agent.ID (UUID) as client_id
				resp, err := server.AuthenticatedGET(
					"/oauth2/authorize?client_id="+alpha.ID.String()+
						"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz",
					fixtures.DefaultPrincipal().String(),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Agent is found and request proceeds (proxied to upstream, not 400 invalid_client)
				Expect(resp.StatusCode).ToNot(Equal(http.StatusBadRequest))
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusFound),
					Equal(http.StatusSeeOther),
				))
				location := resp.Header.Get("Location")
				Expect(location).To(ContainSubstring(mockUpstream.URL()))
			})

			// US1 Scenario 3 from specs/021-multi-agent-clientid/spec.md
			It("should proxy token after verifying agent ID claim", Label("US1"), func() {
				// Given: Mock upstream returns JWT token containing x_agent_id=alpha.ID
				mockUpstream.WithSuccessfulTokenResponse().
					ReturnTokenWithClaim("x_agent_id", alpha.ID.String())

				// When: Token request with alpha agent's ID as client_id
				formData := url.Values{
					"grant_type":   []string{"authorization_code"},
					"code":         []string{"mock-auth-code-123"},
					"client_id":    []string{alpha.ID.String()},
					"redirect_uri": []string{"https://client.example.com/cb"},
				}
				resp, err := server.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Token is returned (broker verified the claim and forwarded the token)
				Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
				Expect(resp).To(matchers.ContainAgentIDClaim("x_agent_id", alpha.ID.String()))
			})

			// US1 Scenario 3b: Verify upstream receives agent.ClientID, not the broker UUID
			// (review comment r2995594898)
			It("should forward the agent upstream client_id to upstream, not the broker agent UUID", Label("US1"), func() {
				// Given: Mock upstream is configured to return a valid token response with
				// the x_agent_id claim so the verifier passes.
				mockUpstream.WithSuccessfulTokenResponse().
					ReturnTokenWithClaim("x_agent_id", alpha.ID.String())

				// When: Token request using alpha's broker UUID as client_id
				formData := url.Values{
					"grant_type":   []string{"authorization_code"},
					"code":         []string{"mock-auth-code-123"},
					"client_id":    []string{alpha.ID.String()}, // broker-internal UUID
					"redirect_uri": []string{"https://client.example.com/cb"},
				}
				resp, err := server.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				// Then: Upstream received the agent's upstream client_id, NOT the broker UUID.
				// alpha.ClientID == fixtures.SharedUpstreamClientID, not alpha.ID (broker UUID).
				lastReq := mockUpstream.GetLastRequest()
				Expect(lastReq).NotTo(BeNil())
				Expect(lastReq.FormValue("client_id")).To(Equal(fixtures.SharedUpstreamClientID))
				Expect(lastReq.FormValue("client_id")).NotTo(Equal(alpha.ID.String()))
			})

			// US1 Scenario 4 from specs/021-multi-agent-clientid/spec.md
			It("should verify claim value matches initiating agent ID", Label("US1"), func() {
				// Given: Mock upstream returns JWT with x_agent_id=BETA (wrong agent!)
				mockUpstream.WithSuccessfulTokenResponse().
					ReturnTokenWithClaim("x_agent_id", beta.ID.String())

				// When: Token request using ALPHA agent's ID as client_id (alpha initiated)
				formData := url.Values{
					"grant_type":   []string{"authorization_code"},
					"code":         []string{"mock-auth-code-123"},
					"client_id":    []string{alpha.ID.String()},
					"redirect_uri": []string{"https://client.example.com/cb"},
				}
				resp, err := server.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Claim mismatch — broker returns OAuth2 error and withholds token
				Expect(resp).To(matchers.HaveOAuth2Error("server_error"))
			})

			// Bug regression – no scenario number yet; to be assigned when the fix lands.
			// When two agents share the same upstream ClientID and one is updated to use a new
			// ClientID, the in-memory byClientID index incorrectly removes the shared entry,
			// making the other agent invisible via GetByClientID.
			It("should keep all agents accessible after one shared-client agent has its client_id updated", Label("US1"), func() {
				ctx := context.Background()

				// Build a dedicated admin + end-user server pair for this test so we can issue
				// an admin PUT without touching the shared `server` created in BeforeEach.
				config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())
				sf := bootstrap.NewServerFactory(config, logger)
				appInstance, err := sf.BuildApp(testStorage)
				Expect(err).ToNot(HaveOccurred())

				adminSrv, err := bootstrap.NewAdminTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
				defer adminSrv.Close()

				enduserSrv, err := bootstrap.NewEndUserTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
				defer enduserSrv.Close()

				// Given: alpha and beta both have SharedUpstreamClientID (created in outer BeforeEach).
				// Seed a permission set so the PUT update satisfies the FR-006 non-empty requirement.
				ps := &storagedomain.PermissionSet{
					ID:          fixtures.PlaceholderPermissionSetID,
					Name:        "Multi-Agent Test PS",
					Description: "Permission set for multi-agent client update regression test",
					ServiceScopes: []storagedomain.ServiceScope{
						{ServiceID: fixtures.PlaceholderServiceID, Scopes: []string{"read"}, RequirementType: storagedomain.RequirementTypeOptional},
					},
				}
				Expect(testStorage.PermissionSets().Create(ctx, ps)).ToNot(HaveOccurred())

				// When: Admin changes alpha's client_id to a unique value.
				updatePayload := map[string]interface{}{
					"client_id":    "other-unique-client", // new, distinct from SharedUpstreamClientID
					"display_name": alpha.DisplayName,
					"description":  alpha.Description,
					"permission_sets": []map[string]interface{}{
						{"permission_set_id": fixtures.PlaceholderPermissionSetID.String(), "requirement_type": "mandatory"},
					},
				}
				body, err := json.Marshal(updatePayload)
				Expect(err).ToNot(HaveOccurred())

				putResp, err := adminSrv.DirectRequest(
					"PUT", "/api/agents/"+alpha.ID.String(), "admin@example.com",
					map[string]string{"Content-Type": "application/json"},
					bytes.NewReader(body),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = putResp.Body.Close() }()
				Expect(putResp.StatusCode).To(Equal(http.StatusOK))

				// Then: GET /api/agents/{beta.ID} still returns beta with the original client_id.
				getResp, err := adminSrv.DirectRequest(
					"GET", "/api/agents/"+beta.ID.String(), "admin@example.com",
					nil, nil,
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = getResp.Body.Close() }()
				Expect(getResp.StatusCode).To(Equal(http.StatusOK))

				var betaJSON map[string]interface{}
				Expect(json.NewDecoder(getResp.Body).Decode(&betaJSON)).ToNot(HaveOccurred())
				Expect(betaJSON["client_id"]).To(Equal(fixtures.SharedUpstreamClientID))

				// Then: beta must still be resolvable via its SharedUpstreamClientID.
				// Bug: delete(byClientID["shared"]) wipes the entry when alpha's client_id changes,
				// so GetByClientID("shared") returns not-found even though beta is unchanged.
				// This assertion FAILS on the current buggy implementation.
				betaByClientID, err := testStorage.Agents().GetByClientID(ctx, id.ClientID(fixtures.SharedUpstreamClientID))
				Expect(err).ToNot(HaveOccurred(), "beta should still be findable by SharedUpstreamClientID after alpha's client_id was updated")
				Expect(betaByClientID.ID).To(Equal(beta.ID))

				// Also: beta's full authorize->token flow must succeed.
				mockUpstream.WithSuccessfulTokenResponse().
					ReturnTokenWithClaim("x_agent_id", beta.ID.String())

				formData := url.Values{
					"grant_type":   []string{"authorization_code"},
					"code":         []string{"mock-auth-code-123"},
					"client_id":    []string{beta.ID.String()},
					"redirect_uri": []string{"https://client.example.com/cb"},
				}
				tokenResp, err := enduserSrv.DirectRequest(
					"POST", "/oauth2/token", "",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = tokenResp.Body.Close() }()
				Expect(tokenResp).To(matchers.HaveStatusCode(http.StatusOK))
			})

			// US1 Scenario 5 from specs/021-multi-agent-clientid/spec.md
			It("should return error and withhold token when agent ID claim absent", Label("US1"), func() {
				// Given: Mock upstream returns a plain non-JWT token (no x_agent_id claim)
				mockUpstream.WithSuccessfulTokenResponse().
					WithAccessToken("plain-opaque-token-without-any-claim")

				// When: Token request — broker expects claim verification
				formData := url.Values{
					"grant_type":   []string{"authorization_code"},
					"code":         []string{"mock-auth-code-123"},
					"client_id":    []string{alpha.ID.String()},
					"redirect_uri": []string{"https://client.example.com/cb"},
				}
				resp, err := server.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Claim absent — broker fails closed and does NOT forward the token
				Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			})
		})

		Context("when multi-agent client sharing is disabled", func() {
			var server *bootstrap.TestServer

			BeforeEach(func() {
				config := fixtures.MultiAgentDisabledConfig(mockUpstream.URL())
				sf := bootstrap.NewServerFactory(config, logger)
				appInstance, err := sf.BuildApp(testStorage)
				Expect(err).ToNot(HaveOccurred())
				server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
			})

			AfterEach(func() {
				if server != nil {
					server.Close()
				}
			})

			// US1 Scenario 6 from specs/021-multi-agent-clientid/spec.md
			It("should not inject param or verify claim when feature is disabled", Label("US1"), func() {
				// Given: Feature disabled; alpha has an active grant
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					alpha.ID.String(),
					fixtures.GitHubService().ID.String(),
					[]string{"read"},
				)
				Expect(testStorage.UserGrants().Create(context.Background(), grant)).ToNot(HaveOccurred())

				// When: Authorization request using agent.ID as client_id (still UUID-based)
				resp, err := server.AuthenticatedGET(
					"/oauth2/authorize?client_id="+alpha.ID.String()+
						"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz",
					fixtures.DefaultPrincipal().String(),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Proxied to upstream without x_agent_id param (feature is disabled)
				redirectURL, err := helpers.ExtractRedirectURL(resp)
				Expect(err).ToNot(HaveOccurred())
				Expect(redirectURL.Query().Get("x_agent_id")).To(BeEmpty())
				// And: Redirect goes to upstream normally
				Expect(redirectURL.String()).To(ContainSubstring(mockUpstream.URL()))
			})
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// User Story 2: Token Exchange Resolves Agent from Token Claim
	// ──────────────────────────────────────────────────────────────────
	Describe("US2: Token Exchange Resolves Agent from Token Claim", func() {
		var (
			alpha         *storagedomain.Agent
			enduserServer *bootstrap.TestServer
		)

		AfterEach(func() {
			if enduserServer != nil {
				enduserServer.Close()
			}
		})

		Context("when multi-agent client sharing is enabled", func() {
			var clientAssertion string

			BeforeEach(func() {
				alpha = fixtures.MultiAgentAlpha()
				ctx := context.Background()
				Expect(testStorage.Agents().Create(ctx, alpha)).ToNot(HaveOccurred())

				// Register GitHub as the third-party service for resource-based lookup.
				// Token endpoint is pointed at the mock upstream so exchange requests reach it.
				githubService := fixtures.GitHubService()
				githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
				Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())
				Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)).To(Succeed())

				// Grant alpha agent access to GitHub service (required by token exchange flow).
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					alpha.ID.String(),
					githubService.ID.String(),
					[]string{"repo", "user"},
				)
				Expect(testStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

				// Create active session for default principal — token exchange requires an active session.
				session := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				Expect(testStorage.UserSessions().Create(ctx, session)).ToNot(HaveOccurred())

				// Generate client assertion for alpha agent — authenticates the requesting agent.
				// sub = alpha.ID (broker UUID) since in multi-agent mode each agent has its own UUID.
				now := time.Now()
				assertionClaims := map[string]interface{}{
					"sub": alpha.ID.String(),
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				var err error
				clientAssertion, err = helpers.SignTestJWT(assertionClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())
				sf := bootstrap.NewServerFactory(config, logger)
				appInstance, err := sf.BuildApp(testStorage)
				Expect(err).ToNot(HaveOccurred())
				enduserServer, err = bootstrap.NewEndUserTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
			})

			// US2 Scenario 1 from specs/021-multi-agent-clientid/spec.md
			It("should resolve agent from token claim when feature enabled", Label("US2"), func() {
				// Given: Subject token contains x_agent_id=alpha.ID, CEL reads it directly.
				// Full RFC 8693 claims required: iss must match upstream for issuer validation.
				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub":        fixtures.DefaultPrincipal().String(),
					"x_agent_id": alpha.ID.String(),
					"azp":        string(*alpha.ClientID),
					"iss":        mockUpstream.URL(),
					"aud":        "token-exchange-broker",
					"exp":        now.Add(1 * time.Hour).Unix(),
					"iat":        now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				// When: Token exchange with subject token containing agent ID claim
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				mockUpstream.WithSuccessfulTokenResponse()
				resp, err := enduserServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Token exchange succeeds — agent resolved from claim
				Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			})

			// US2 Scenario 2 from specs/021-multi-agent-clientid/spec.md
			It("should fail token exchange when agent ID claim absent from subject token", Label("US2"), func() {
				// Given: Subject token WITHOUT x_agent_id claim; CEL policy expects it.
				// Full RFC 8693 claims required to pass issuer validation before claim check.
				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub": fixtures.DefaultPrincipal().String(),
					"azp": string(*alpha.ClientID),
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				// When: Token exchange with subject token missing the agent ID claim
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				resp, err := enduserServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: CEL expression fails (claim absent) — token exchange returns error
				Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			})

			// US2 Scenario 4 from specs/021-multi-agent-clientid/spec.md
			It("should reject token exchange when agent ID claim does not match any registered agent", Label("US2"), func() {
				// Given: Subject token has x_agent_id claim with UUID not matching any registered agent.
				// Full RFC 8693 claims required to pass issuer validation before agent lookup.
				unknownAgentID := "00000000-0000-0000-0000-000000000099"
				now := time.Now()
				subjectTokenClaims := map[string]interface{}{
					"sub":        fixtures.DefaultPrincipal().String(),
					"x_agent_id": unknownAgentID,
					"iss":        mockUpstream.URL(),
					"aud":        "token-exchange-broker",
					"exp":        now.Add(1 * time.Hour).Unix(),
					"iat":        now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(subjectTokenClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				// When: Token exchange with subject token referencing an unregistered agent
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertion},
				}
				resp, err := enduserServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: Unknown agent ID — exchange is rejected
				Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			})
		})

		Context("when multi-agent client sharing is disabled", func() {
			var clientAssertionDisabled string

			BeforeEach(func() {
				alpha = fixtures.MultiAgentAlpha()
				ctx := context.Background()
				Expect(testStorage.Agents().Create(ctx, alpha)).ToNot(HaveOccurred())

				// Register GitHub as the third-party service for resource-based lookup.
				githubService := fixtures.GitHubService()
				githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
				Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())
				Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)).To(Succeed())

				// Grant alpha agent access to GitHub service.
				grant := fixtures.ActiveGrant(
					fixtures.DefaultPrincipal().String(),
					alpha.ID.String(),
					githubService.ID.String(),
					[]string{"repo", "user"},
				)
				Expect(testStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

				// Create active session for default principal — token exchange requires an active session.
				session := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
				Expect(testStorage.UserSessions().Create(ctx, session)).ToNot(HaveOccurred())

				// Generate client assertion for alpha agent.
				now := time.Now()
				assertionClaims := map[string]interface{}{
					"sub": alpha.ID.String(),
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				var err error
				clientAssertionDisabled, err = helpers.SignTestJWT(assertionClaims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				config := fixtures.MultiAgentDisabledConfig(mockUpstream.URL())
				sf := bootstrap.NewServerFactory(config, logger)
				appInstance, err := sf.BuildApp(testStorage)
				Expect(err).ToNot(HaveOccurred())
				enduserServer, err = bootstrap.NewEndUserTestServer(appInstance, logger)
				Expect(err).ToNot(HaveOccurred())
			})

			// US2 Scenario 3 from specs/021-multi-agent-clientid/spec.md
			It("should resolve agent via resolveAgentIdByClientId CEL function when disabled", Label("US2"), func() {
				// Given: Feature disabled; CEL policy uses resolveAgentIdByClientId(subject_token.azp)
				// Subject token has azp=alpha.ClientID plus full RFC 8693 claims (iss, aud, exp, iat).
				now := time.Now()
				claims := map[string]interface{}{
					"sub": fixtures.DefaultPrincipal().String(),
					"azp": string(*alpha.ClientID),
					"iss": mockUpstream.URL(),
					"aud": "token-exchange-broker",
					"exp": now.Add(1 * time.Hour).Unix(),
					"iat": now.Unix(),
				}
				subjectToken, err := helpers.SignTestJWT(claims, mockUpstream.GetPrivateKeyPEM())
				Expect(err).ToNot(HaveOccurred())

				// When: Token exchange where CEL calls resolveAgentIdByClientId(azp)
				formData := url.Values{
					"grant_type":            []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
					"subject_token":         []string{subjectToken},
					"subject_token_type":    []string{"urn:ietf:params:oauth:token-type:access_token"},
					"requested_token_type":  []string{"urn:ietf:params:oauth:token-type:access_token"},
					"resource":              []string{"https://api.github.com"},
					"client_assertion_type": []string{"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
					"client_assertion":      []string{clientAssertionDisabled},
				}
				mockUpstream.WithSuccessfulTokenResponse()
				resp, err := enduserServer.DirectRequest(
					"POST",
					"/oauth2/token",
					"",
					map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
					strings.NewReader(formData.Encode()),
				)
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				// Then: resolveAgentIdByClientId maps upstream client_id to agent.id → exchange succeeds
				Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			})
		})
	})

	// ──────────────────────────────────────────────────────────────────
	// User Story 3: Operator Configures Multi-Agent Client Sharing
	// ──────────────────────────────────────────────────────────────────
	// US3 tests validate server startup behavior — they create their own
	// server instances inside each It() block to test config validation.
	Describe("US3: Operator Configures Multi-Agent Client Sharing", func() {
		// US3 Scenario 1 from specs/021-multi-agent-clientid/spec.md
		It("should start successfully with valid multi_agent_client configuration", Label("US3"), func() {
			// Given: Config with feature enabled and both required fields present
			config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())

			// When: Building the application
			sf := bootstrap.NewServerFactory(config, logger)
			appInstance, err := sf.BuildApp(testStorage)

			// Then: Startup succeeds — no validation error
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			srv, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer srv.Close()
		})

		// US3 Scenario 2 from specs/021-multi-agent-clientid/spec.md
		It("should fail to start when enabled but agent_id_param_name is absent", Label("US3"), func() {
			// Given: Config with feature enabled but agent_id_param_name missing
			config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())
			config.OAuth2AuthServer.MultiAgentClient.AgentIDParamName = ""

			// When: Building the application
			sf := bootstrap.NewServerFactory(config, logger)
			_, err := sf.BuildApp(testStorage)

			// Then: Startup fails with a clear error about the missing param name
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agent_id_param_name"))
		})

		// US3 Scenario 3 from specs/021-multi-agent-clientid/spec.md
		It("should fail to start when enabled but agent_id_claim_name is absent", Label("US3"), func() {
			// Given: Config with feature enabled but agent_id_claim_name missing
			config := fixtures.MultiAgentEnabledConfig(mockUpstream.URL())
			config.OAuth2AuthServer.MultiAgentClient.AgentIDClaimName = ""

			// When: Building the application
			sf := bootstrap.NewServerFactory(config, logger)
			_, err := sf.BuildApp(testStorage)

			// Then: Startup fails with a clear error about the missing claim name
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agent_id_claim_name"))
		})

		// US3 Scenario 4 from specs/021-multi-agent-clientid/spec.md
		It("should operate in single-agent mode when multi_agent_client is disabled", Label("US3"), func() {
			// Given: Config with feature explicitly disabled
			config := fixtures.MultiAgentDisabledConfig(mockUpstream.URL())

			// When: Building and starting the application
			sf := bootstrap.NewServerFactory(config, logger)
			appInstance, err := sf.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			srv, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer srv.Close()

			// Then: Broker starts normally and serves requests (single-agent mode, no behavioral change)
			agent := fixtures.ValidAgent()
			Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

			grant := fixtures.ActiveGrant(
				fixtures.DefaultPrincipal().String(),
				agent.ID.String(),
				fixtures.GitHubService().ID.String(),
				[]string{"read"},
			)
			Expect(testStorage.UserGrants().Create(context.Background(), grant)).ToNot(HaveOccurred())

			// Authorize with agent.ID (UUID) — should proxy to upstream normally
			resp, err := srv.AuthenticatedGET(
				"/oauth2/authorize?client_id="+agent.ID.String()+
					"&redirect_uri=https://client.example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Single-agent mode: proxies normally, no x_agent_id param injected
			Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusFound),
				Equal(http.StatusSeeOther),
			))
			redirectURL, err := helpers.ExtractRedirectURL(resp)
			Expect(err).ToNot(HaveOccurred())
			Expect(redirectURL.Query().Get("x_agent_id")).To(BeEmpty())
		})
	})
})
