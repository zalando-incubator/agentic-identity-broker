package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lestrrat-go/jwx/v4/jwk"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// newE2EJWETokenService returns a JWE token service backed by the same key as DefaultOAuth2Config.
// Used to create test tokens (including expired ones) for E2E rejection scenarios.
func newE2EJWETokenService() *domjwe.TokenService {
	jweKey, err := jwk.Import[jwk.Key]([]byte("test-32-byte-key-must-be-exact-x"))
	if err != nil {
		panic("newE2EJWETokenService: failed to import key: " + err.Error())
	}
	return domjwe.New(jweKey)
}

// newExpiredE2ESessionToken creates a JWE session token whose TTL has already elapsed,
// using the same key as the test server.
func newExpiredE2ESessionToken(agentID id.AgentID, principalVal string) string {
	ts := newE2EJWETokenService()
	past := time.Now().Add(-time.Hour)
	claims := &sessiontoken.AuthorizationSessionClaims{
		AgentID:     agentID,
		Principal:   id.Principal(principalVal),
		OriginalURL: "https://broker.example.com/oauth2/authorize?client_id=" + agentID.String(),
		IssuedAt:    past,
		ExpiresAt:   past,
	}
	token, err := ts.Encrypt(claims)
	if err != nil {
		panic("newExpiredE2ESessionToken: " + err.Error())
	}
	return token
}

var _ = Describe("Unified Session Token State Transport", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		adminServer    *bootstrap.TestServer
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		config := fixtures.LocalConfig()
		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("local agent", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Local Test Agent",
				Description:    "E2E test agent for unified session token scenarios",
				RedirectURIs:   []string{"https://client.example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		})

		// Scenario 1.1 from specs/031-unified-session-token/spec.md
		It("includes session_token and omits redirect_uri for local agent authorize", func() {
			principal := fixtures.DefaultPrincipal().String()
			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=state123",
				agent.ID,
				url.QueryEscape("https://client.example.com/cb"),
			)

			resp, err := server.AuthenticatedGET(authorizeURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/"))
			Expect(loc).To(ContainSubstring("session_token="), "consent URL must contain session_token")
			Expect(loc).NotTo(ContainSubstring("redirect_uri="), "consent URL must NOT contain redirect_uri")
		})

		// Scenario 1.2 from specs/031-unified-session-token/spec.md
		It("session token contains agent ID, principal, original URL, iat, exp with 10min TTL", func() {
			principal := fixtures.DefaultPrincipal().String()
			originalURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=state456",
				agent.ID,
				url.QueryEscape("https://client.example.com/cb"),
			)

			resp, err := server.AuthenticatedGET(originalURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusFound))

			loc := resp.Header.Get("Location")
			consentLoc, err := url.Parse(loc)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentLoc.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())

			ts := newE2EJWETokenService()
			var claims sessiontoken.AuthorizationSessionClaims
			Expect(ts.Decrypt(sessionToken, &claims)).To(Succeed())

			Expect(claims.AgentID).To(Equal(agent.ID))
			Expect(string(claims.Principal)).To(Equal(principal))
			Expect(claims.OriginalURL).ToNot(BeEmpty())
			Expect(claims.IssuedAt.IsZero()).To(BeFalse())
			Expect(claims.ExpiresAt.IsZero()).To(BeFalse())
			ttl := claims.ExpiresAt.Sub(claims.IssuedAt)
			Expect(ttl).To(BeNumerically("~", 10*time.Minute, 5*time.Second))
			Expect(claims.CIMDMetadata).To(BeNil(), "local agent token must not contain CIMD metadata")
		})

		// Scenario 1.3 from specs/031-unified-session-token/spec.md
		It("rejects session token when principal does not match authenticated user", func() {
			principalA := fixtures.DefaultPrincipal().String()
			principalB := fixtures.AnotherPrincipal().String()

			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=state789",
				agent.ID,
				url.QueryEscape("https://client.example.com/cb"),
			)

			// User A performs authorize — gets session_token bound to principalA
			resp, err := server.AuthenticatedGET(authorizeURL, principalA)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusFound))

			loc := resp.Header.Get("Location")
			consentLoc, err := url.Parse(loc)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentLoc.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())

			// User B tries to use principalA's session_token on the consent page
			consentPath := fmt.Sprintf("/api/consent/agents/%s?session_token=%s",
				agent.ID, url.QueryEscape(sessionToken))
			consentResp, err := server.AuthenticatedGET(consentPath, principalB)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = consentResp.Body.Close() }()
			Expect(consentResp.StatusCode).To(Equal(http.StatusBadRequest),
				"principal mismatch must be rejected with 400")
		})
	})

	Context("proxy agent", func() {
		var (
			agent        *storage.Agent
			proxyServer  *bootstrap.TestServer
			proxyStorage *storageadapter.Adapter
		)

		BeforeEach(func() {
			// Proxy agent uses the default proxy mode config
			proxyStorage, _ = storageFactory.NewTestStorage()
			proxyCfg := fixtures.DefaultOAuth2Config()
			proxyFactory := bootstrap.NewServerFactory(proxyCfg, logger)
			appInstance, err := proxyFactory.BuildApp(proxyStorage)
			Expect(err).ToNot(HaveOccurred())
			proxyServer, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())

			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				ClientID:       ptr.To(id.ClientID("proxy-test-client")),
				DisplayName:    "Proxy Test Agent",
				Description:    "E2E test agent for proxy mode unified session token scenarios",
				RedirectURIs:   []string{"https://client.example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(proxyStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		})

		AfterEach(func() {
			if proxyServer != nil {
				proxyServer.Close()
			}
			if proxyStorage != nil {
				_ = storageFactory.CloseStorage(proxyStorage)
			}
		})

		// Scenario 2.1 from specs/031-unified-session-token/spec.md
		It("includes session_token for proxy agent authorize", func() {
			principal := fixtures.DefaultPrincipal().String()
			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=proxystate",
				agent.ID,
				url.QueryEscape("https://client.example.com/cb"),
			)

			resp, err := proxyServer.AuthenticatedGET(authorizeURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("session_token="), "proxy agent consent URL must contain session_token")
			Expect(loc).NotTo(ContainSubstring("redirect_uri="), "proxy agent consent URL must NOT contain redirect_uri")
		})

		// Scenario 2.2 from specs/031-unified-session-token/spec.md
		It("rejects expired session token for proxy agent", func() {
			principal := fixtures.DefaultPrincipal().String()
			expiredToken := newExpiredE2ESessionToken(agent.ID, principal)

			grantBody, _ := json.Marshal(map[string]any{
				"delegated_oauth2_tokens": []any{},
			})
			grantPath := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s",
				agent.ID, url.QueryEscape(expiredToken))

			resp, err := proxyServer.AuthenticatedPOST(grantPath, principal, "application/json", bytes.NewReader(grantBody))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "expired session token must be rejected with 400")
		})
	})

	Context("consent handlers", func() {
		var agent *storage.Agent

		BeforeEach(func() {
			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Consent Handler Test Agent",
				Description:    "E2E test agent for consent handler unified session token scenarios",
				RedirectURIs:   []string{"https://client.example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

			// Register broker credentials so the issue_token code issuer can authenticate this client.
			resp, err := http.Post(adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials", "application/json", nil)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		})

		// Scenario 3.1 from specs/031-unified-session-token/spec.md
		It("consent handler extracts context from session token for any agent mode", func() {
			principal := fixtures.DefaultPrincipal().String()
			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=ctxstate",
				agent.ID,
				url.QueryEscape("https://client.example.com/cb"),
			)

			// Get session_token from authorize redirect
			authResp, err := server.AuthenticatedGET(authorizeURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = authResp.Body.Close() }()
			Expect(authResp.StatusCode).To(Equal(http.StatusFound))

			loc := authResp.Header.Get("Location")
			consentLoc, err := url.Parse(loc)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentLoc.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())

			// Submit grant using session_token — must succeed
			grantBody, _ := json.Marshal(map[string]any{
				"granted_permission_sets": map[string][]string{
					fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
				},
			})
			grantPath := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s",
				agent.ID, url.QueryEscape(sessionToken))

			grantResp, err := server.AuthenticatedPOST(grantPath, principal, "application/json", bytes.NewReader(grantBody))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()

			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			var body map[string]any
			Expect(json.NewDecoder(grantResp.Body).Decode(&body)).To(Succeed())
			Expect(body["redirect_url"]).ToNot(BeEmpty(), "session token claims must provide redirect_url")
		})

		// Scenario 4.1 from specs/031-unified-session-token/spec.md
		It("grant response redirect_url preserves original authorization request context", func() {
			principal := fixtures.DefaultPrincipal().String()
			state := "originalstate456"
			redirectURI := "https://client.example.com/cb"
			pkceVerifier := helpers.PKCEVerifier()
			pkceChallenge := helpers.GenerateCodeChallenge(pkceVerifier)
			originalAuthorize := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=%s&code_challenge=%s&code_challenge_method=S256",
				agent.ID,
				url.QueryEscape(redirectURI),
				state,
				pkceChallenge,
			)

			// Step 1: authorize → consent redirect with session_token
			authResp, err := server.AuthenticatedGET(originalAuthorize, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = authResp.Body.Close() }()
			Expect(authResp.StatusCode).To(Equal(http.StatusFound))

			loc := authResp.Header.Get("Location")
			consentLoc, err := url.Parse(loc)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentLoc.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())

			// Step 2: consent page load — GET /api/consent/agent/{id}?session_token=...
			consentPagePath := fmt.Sprintf("/api/consent/agents/%s?session_token=%s",
				agent.ID, url.QueryEscape(sessionToken))
			consentPageResp, err := server.AuthenticatedGET(consentPagePath, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = consentPageResp.Body.Close() }()
			Expect(consentPageResp.StatusCode).To(Equal(http.StatusOK),
				"consent page load must succeed with valid session_token")

			// Step 3: submit grant with session_token
			grantBody, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
			grantPath := fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s",
				agent.ID, url.QueryEscape(sessionToken))
			grantResp, err := server.AuthenticatedPOST(grantPath, principal, "application/json", bytes.NewReader(grantBody))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			var body map[string]any
			Expect(json.NewDecoder(grantResp.Body).Decode(&body)).To(Succeed())
			redirectURL, _ := body["redirect_url"].(string)
			Expect(redirectURL).ToNot(BeEmpty(), "grant response must include redirect_url when session_token was present")

			// Step 4: verify redirect_url preserves original authorization context
			resumeURL, err := url.Parse(redirectURL)
			Expect(err).ToNot(HaveOccurred())
			q := resumeURL.Query()
			Expect(q.Get("state")).To(Equal(state), "original state must be preserved in redirect_url")
			Expect(q.Get("redirect_uri")).To(Equal(redirectURI), "original redirect_uri must be preserved in redirect_url")
			Expect(q.Get("client_id")).To(Equal(agent.ID.String()), "original client_id must be preserved in redirect_url")

			// Step 5: verify redirect_url is a valid authorize endpoint (full journey completion)
			resumeResp, err := server.AuthenticatedGET(resumeURL.RequestURI(), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resumeResp.Body.Close() }()
			Expect(resumeResp.StatusCode).To(Equal(http.StatusFound),
				"resuming the authorize flow after consent must redirect (grant now exists)")
		})

		// Scenario 3.2 from specs/031-unified-session-token/spec.md
		// Without a session_token the grants handler operates in consent-management mode
		// (user managing grants outside the OAuth2 flow) — no redirect_url is returned.
		It("grant response omits redirect_url when no session_token is present", func() {
			principal := fixtures.DefaultPrincipal().String()
			grantBody, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
			grantPath := fmt.Sprintf("/api/consent/agents/%s/grants", agent.ID)

			resp, err := server.AuthenticatedPOST(grantPath, principal, "application/json", bytes.NewReader(grantBody))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body).ToNot(HaveKey("redirect_url"),
				"no redirect_url in consent-management mode (session_token absent)")
		})

		// Scenario 3.3 from specs/031-unified-session-token/spec.md
		// GET agent detail without session_token must return the resource normally —
		// no session context required for standalone consent-management requests.
		It("returns agent detail without session context when no session_token is present", func() {
			principal := fixtures.DefaultPrincipal().String()
			detailPath := fmt.Sprintf("/api/consent/agents/%s", agent.ID)

			resp, err := server.AuthenticatedGET(detailPath, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

			data, ok := body["data"].(map[string]any)
			Expect(ok).To(BeTrue(), "response must have a data object")
			agentData, ok := data["agent"].(map[string]any)
			Expect(ok).To(BeTrue(), "data must contain an agent object")
			Expect(agentData["agentId"]).To(Equal(agent.ID.String()), "returned agent must match requested ID")
			Expect(data["cimd_metadata"]).To(BeNil(), "cimd_metadata must be absent when no session_token is present")
		})
	})
})
