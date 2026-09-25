package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// Hybrid Mode E2E Tests
//
// Maps to US2 (scenarios 1–3, 7–11) and US3 (scenarios 3–4) from
// specs/030-hybrid-oauth-modes/spec.md. US1 and US2 scenarios 4–6 are
// covered by config unit tests; US2.7–8 are covered in mode_configuration_e2e_test.go.

var _ = Describe("US2: Hybrid Mode Agent Coexistence", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		config := fixtures.HybridConfig(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(adminSrv.Close)
		Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
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

	// Scenario US2.1 from specs/030-hybrid-oauth-modes/spec.md
	It("accepts proxy agent requests (proxy path) in hybrid mode", func() {
		now := time.Now()
		proxyAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientID:       ptr.To(id.ClientID("upstream-client-abc")),
			DisplayName:    "Proxy Agent",
			Description:    "Agent with upstream ClientID — proxy path",
			RedirectURIs:   []string{"https://example.com/cb"},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), proxyAgent)).To(Succeed())

		// Without a grant, proxy mode redirects to consent — proves the proxy agent
		// was resolved and accepted by the hybrid mode strategy.
		resp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", proxyAgent.ID.String()),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring("/agents/" + proxyAgent.ID.String()))
	})

	// Scenario US2.2 from specs/030-hybrid-oauth-modes/spec.md
	It("accepts local agent requests (local path) in hybrid mode", func() {
		localAgent := fixtures.LocalAgent()
		localAgent.RedirectURIs = []string{"https://example.com/cb"}
		Expect(testStorage.Agents().Create(context.Background(), localAgent)).To(Succeed())

		resp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", localAgent.ID.String()),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring("/agents/" + localAgent.ID.String()))
	})

	// Scenario US2.10 from specs/030-hybrid-oauth-modes/spec.md
	It("serves JWKS endpoint in hybrid mode", func() {
		resp, err := helpers.HTTPClient().Get(server.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var jwks map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
		keys, ok := jwks["keys"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(len(keys)).To(BeNumerically(">=", 1))
	})

	// Scenario US2.11 from specs/030-hybrid-oauth-modes/spec.md
	It("exposes discovery metadata with JWKS URI in hybrid mode", func() {
		resp, err := helpers.HTTPClient().Get(server.BaseURL() + "/.well-known/oauth-authorization-server")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("token_endpoint"))
		Expect(body).To(HaveKey("jwks_uri"))
	})
})

var _ = Describe("US2+US3: Hybrid Mode with CIMD", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		cimdServer     *httptest.Server
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		clientURL      string
		redirectURI    string
	)

	const fakeHost = "cimd-hybrid-test.test.invalid"

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		clientURL = "https://" + fakeHost + "/client"
		redirectURI = "https://" + fakeHost + "/callback"

		cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			w.WriteHeader(http.StatusOK)
			doc := map[string]any{
				"client_id":     clientURL,
				"client_name":   "Hybrid CIMD Agent",
				"redirect_uris": []string{redirectURI},
			}
			_ = json.NewEncoder(w).Encode(doc)
		}))

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		config := fixtures.HybridConfigWithCIMD(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)

		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
		Expect(err).ToNot(HaveOccurred())

		app, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
		Expect(err).ToNot(HaveOccurred())

		adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(adminSrv.Close)
		Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if cimdServer != nil {
			cimdServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US3.3 from specs/030-hybrid-oauth-modes/spec.md
	It("accepts hybrid+CIMD configuration and starts successfully", func() {
		// Builder succeeded (server is non-nil) — hybrid+CIMD config is accepted.
		// The JWKS endpoint confirms local token signing is wired.
		resp, err := helpers.HTTPClient().Get(server.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario US2.3 from specs/030-hybrid-oauth-modes/spec.md
	It("resolves CIMD agent via URL-format client_id in hybrid mode", func() {
		now := time.Now()
		cimdAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientURL},
			DisplayName:    "CIMD Agent",
			Description:    "CIMD agent in hybrid mode",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), cimdAgent)).To(Succeed())

		resp, err := server.AuthenticatedGET(
			fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
				clientURL, redirectURI,
			),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// CIMD agent resolved via URL → accepted by hybrid mode → consent redirect
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring("/agents/" + cimdAgent.ID.String()))
	})

	// Scenario US3.4 from specs/030-hybrid-oauth-modes/spec.md
	It("proxy agent bypasses CIMD in hybrid mode — resolved by UUID without CIMD lookup", func() {
		now := time.Now()
		proxyAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientID:       ptr.To(id.ClientID("upstream-client-xyz")),
			DisplayName:    "Proxy Agent",
			Description:    "Proxy agent in hybrid+CIMD mode — resolved by UUID",
			RedirectURIs:   []string{"https://example.com/cb"},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), proxyAgent)).To(Succeed())

		// UUID client_id → opaque path → no CIMD fetch → proxy agent accepted
		resp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", proxyAgent.ID.String()),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp).To(matchers.HaveStatusCode(http.StatusFound))
		Expect(resp.Header.Get("Location")).To(ContainSubstring("/agents/" + proxyAgent.ID.String()))
	})

	// Scenario US2.4: full CIMD agent authorization code journey in hybrid mode.
	// Exercises the GetAuthorizeCodeSession path that was broken — it used AgentID
	// for GetClient, but CIMD resolvers reject UUID lookups, returning invalid_client.
	It("CIMD agent in hybrid mode completes full authorization code flow with PKCE", func() {
		ctx := context.Background()
		principal := fixtures.DefaultPrincipal().String()
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		now := time.Now()
		cimdAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientURL},
			DisplayName:    "CIMD Auth Code Agent",
			Description:    "CIMD agent for full auth code flow test",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(ctx, cimdAgent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(ctx, testStorage, id.Principal(principal))).To(Succeed())

		// Step 1: Authorize — no grant → consent redirect.
		authResp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=cimd-state&code_challenge=%s&code_challenge_method=S256",
				url.QueryEscape(clientURL), url.QueryEscape(redirectURI), challenge),
			principal,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = authResp.Body.Close() }()
		Expect(authResp.StatusCode).To(Equal(http.StatusFound))
		consentLoc, _ := url.Parse(authResp.Header.Get("Location"))
		sessionToken := consentLoc.Query().Get("session_token")
		Expect(sessionToken).ToNot(BeEmpty())

		// Step 2: Submit grant for the agent's declared permission set.
		grantBody, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
		grantResp, err := server.AuthenticatedPOST(
			fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", cimdAgent.ID, url.QueryEscape(sessionToken)),
			principal, "application/json", bytes.NewReader(grantBody),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = grantResp.Body.Close() }()
		Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
		var grantRespBody map[string]any
		Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
		reAuthorizeURL, _ := grantRespBody["redirect_url"].(string)
		Expect(reAuthorizeURL).ToNot(BeEmpty())

		// Step 3: Re-authorize — grant satisfied → CIMD local path issues authorization code.
		codeResp, err := server.AuthenticatedGET(reAuthorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = codeResp.Body.Close() }()
		Expect(codeResp.StatusCode).To(Equal(http.StatusFound))
		parsedCodeLoc, _ := url.Parse(codeResp.Header.Get("Location"))
		Expect(parsedCodeLoc.String()).To(HavePrefix(redirectURI))
		code := parsedCodeLoc.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Step 4: Exchange code for token — no client_secret (CIMD clients are public).
		// This step previously failed because GetAuthorizeCodeSession used agentID.String()
		// for GetClient, which the CIMD resolver rejects with invalid_client.
		tokenResp, err := helpers.HTTPClient().Post(
			server.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {clientURL},
				"code":          {code},
				"redirect_uri":  {redirectURI},
				"code_verifier": {verifier},
			}.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]any
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).To(Succeed())
		Expect(tokenBody).To(HaveKey("access_token"))
		Expect(tokenBody["token_type"]).To(Equal("Bearer"))
	})
})

var _ = Describe("US2: Hybrid Mode — Local Agent Full Authorization Code Journey", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		testStorage    *storageadapter.Adapter
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		agent          *storage.Agent
		clientSecret   string
	)

	const localAgentRedirectURI = "http://localhost:9999/callback"

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		agent = fixtures.LocalAgent()
		agent.RedirectURIs = []string{localAgentRedirectURI}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		config := fixtures.HybridConfig(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())

		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

		credResp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = credResp.Body.Close() }()
		Expect(credResp.StatusCode).To(Equal(http.StatusCreated))
		var creds map[string]interface{}
		Expect(json.NewDecoder(credResp.Body).Decode(&creds)).ToNot(HaveOccurred())
		clientSecret = creds["client_secret"].(string)
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US2.2 (full journey) from specs/030-hybrid-oauth-modes/spec.md
	It("local agent in hybrid mode issues JWT via full authorization code flow with PKCE", func() {
		principal := fixtures.DefaultPrincipal().String()
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)
		authorizeURL := "/oauth2/authorize?" + url.Values{
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {localAgentRedirectURI},
			"response_type":         {"code"},
			"state":                 {"hybrid-local-state"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		// Step 1: Authorize — no grant → consent redirect.
		// Consent URL contains session_token (spec 031 unified session token).
		authResp, err := enduserServer.AuthenticatedGET(authorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = authResp.Body.Close() }()
		Expect(authResp.StatusCode).To(Equal(http.StatusFound))
		consentLoc, err := url.Parse(authResp.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		sessionToken := consentLoc.Query().Get("session_token")
		Expect(sessionToken).ToNot(BeEmpty())

		// Step 2: Submit grant for the agent's declared permission set.
		grantBodyBytes, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
		grantResp, err := enduserServer.AuthenticatedPOST(
			fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, url.QueryEscape(sessionToken)),
			principal, "application/json", bytes.NewReader(grantBodyBytes),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = grantResp.Body.Close() }()
		Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
		var grantRespBody map[string]any
		Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
		reAuthorizeURL, _ := grantRespBody["redirect_url"].(string)
		Expect(reAuthorizeURL).ToNot(BeEmpty())

		// Step 3: Re-authorize — grant satisfied → local path issues authorization code.
		codeResp, err := enduserServer.AuthenticatedGET(reAuthorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = codeResp.Body.Close() }()
		Expect(codeResp.StatusCode).To(Equal(http.StatusFound))
		codeLoc := codeResp.Header.Get("Location")
		Expect(codeLoc).To(HavePrefix(localAgentRedirectURI))
		Expect(codeLoc).To(ContainSubstring("state=hybrid-local-state"))
		parsedCodeLoc, _ := url.Parse(codeLoc)
		code := parsedCodeLoc.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Step 4: Exchange code for locally-issued JWT access token.
		tokenResp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {agent.ID.String()},
				"client_secret": {clientSecret},
				"code":          {code},
				"redirect_uri":  {localAgentRedirectURI},
				"code_verifier": {verifier},
			}.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		Expect(tokenBody).To(HaveKey("access_token"))
		Expect(tokenBody["token_type"]).To(Equal("Bearer"))
	})

	// PKCE enforcement: omitting code_verifier when code_challenge was set must fail.
	It("rejects token exchange without code_verifier when code_challenge was set (PKCE enforcement)", func() {
		principal := fixtures.DefaultPrincipal().String()
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)
		authorizeURL := "/oauth2/authorize?" + url.Values{
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {localAgentRedirectURI},
			"response_type":         {"code"},
			"state":                 {"pkce-enforce"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		// Step 1: Authorize — get consent redirect.
		authResp, err := enduserServer.AuthenticatedGET(authorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = authResp.Body.Close() }()
		Expect(authResp.StatusCode).To(Equal(http.StatusFound))
		consentLoc, err := url.Parse(authResp.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		sessionToken := consentLoc.Query().Get("session_token")
		Expect(sessionToken).ToNot(BeEmpty())

		// Step 2: Submit grant.
		grantBodyBytes, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
		grantResp, err := enduserServer.AuthenticatedPOST(
			fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, url.QueryEscape(sessionToken)),
			principal, "application/json", bytes.NewReader(grantBodyBytes),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = grantResp.Body.Close() }()
		Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
		var grantRespBody map[string]any
		Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
		reAuthorizeURL, _ := grantRespBody["redirect_url"].(string)
		Expect(reAuthorizeURL).ToNot(BeEmpty())

		// Step 3: Re-authorize — obtain authorization code.
		codeResp, err := enduserServer.AuthenticatedGET(reAuthorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = codeResp.Body.Close() }()
		Expect(codeResp.StatusCode).To(Equal(http.StatusFound))
		parsedCodeLoc, _ := url.Parse(codeResp.Header.Get("Location"))
		code := parsedCodeLoc.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Step 4: Exchange code WITHOUT code_verifier — must be rejected.
		tokenResp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {agent.ID.String()},
				"client_secret": {clientSecret},
				"code":          {code},
				"redirect_uri":  {localAgentRedirectURI},
				// code_verifier intentionally omitted
			}.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()

		Expect(tokenResp.StatusCode).To(Equal(http.StatusBadRequest))
		var tokenBody map[string]string
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		Expect(tokenBody["error"]).To(Equal("invalid_grant"))
	})
})

var _ = Describe("US2: Hybrid Mode — Proxy Agent Full Authorization Code Journey", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		proxyAgent     *storage.Agent
	)

	const proxyRedirectURI = "https://example.com/cb"

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		now := time.Now()
		proxyAgent = &storage.Agent{
			ID:             id.NewAgentID(),
			ClientID:       ptr.To(id.ClientID("upstream-client-abc")),
			DisplayName:    "Proxy Journey Agent",
			Description:    "Proxy agent for full authorization code journey test",
			RedirectURIs:   []string{proxyRedirectURI},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), proxyAgent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		config := fixtures.HybridConfig(mockUpstream.Server.URL)
		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
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

	// Scenario US2.1 (full journey) from specs/030-hybrid-oauth-modes/spec.md
	It("proxy agent in hybrid mode forwards authorize to upstream and proxies token response", func() {
		principal := fixtures.DefaultPrincipal().String()
		authorizeURL := "/oauth2/authorize?" + url.Values{
			"client_id":     {proxyAgent.ID.String()},
			"redirect_uri":  {proxyRedirectURI},
			"response_type": {"code"},
			"state":         {"hybrid-proxy-state"},
		}.Encode()

		// Step 1: Authorize — no grant → consent redirect.
		// Consent URL contains session_token (spec 031 unified session token).
		authResp, err := server.AuthenticatedGET(authorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = authResp.Body.Close() }()
		Expect(authResp.StatusCode).To(Equal(http.StatusFound))
		consentLoc, err := url.Parse(authResp.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		sessionToken := consentLoc.Query().Get("session_token")
		Expect(sessionToken).ToNot(BeEmpty())

		// Step 2: Submit grant.
		grantBodyBytes, _ := json.Marshal(map[string]any{"granted_permission_sets": map[string][]string{fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()}}})
		grantResp, err := server.AuthenticatedPOST(
			fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", proxyAgent.ID, url.QueryEscape(sessionToken)),
			principal, "application/json", bytes.NewReader(grantBodyBytes),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = grantResp.Body.Close() }()
		Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))
		var grantRespBody map[string]any
		Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
		reAuthorizeURL, _ := grantRespBody["redirect_url"].(string)
		Expect(reAuthorizeURL).ToNot(BeEmpty())
		proxyResp, err := server.AuthenticatedGET(reAuthorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = proxyResp.Body.Close() }()
		Expect(proxyResp.StatusCode).To(Equal(http.StatusFound))
		upstreamAuthorizeURL := proxyResp.Header.Get("Location")
		Expect(upstreamAuthorizeURL).To(ContainSubstring(mockUpstream.Server.URL + "/oauth/authorize"))
		Expect(upstreamAuthorizeURL).To(ContainSubstring("client_id=upstream-client-abc"))

		// Step 4: Follow redirect to mock upstream — upstream returns code at client redirect_uri.
		noFollow := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		upstreamResp, err := noFollow.Get(upstreamAuthorizeURL)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = upstreamResp.Body.Close() }()
		Expect(upstreamResp.StatusCode).To(Equal(http.StatusFound))
		parsedCodeLoc, err := url.Parse(upstreamResp.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		Expect(parsedCodeLoc.String()).To(HavePrefix(proxyRedirectURI))
		code := parsedCodeLoc.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Step 5: Exchange code at broker — broker proxies request to upstream token endpoint.
		tokenResp, err := helpers.HTTPClient().Post(
			server.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(url.Values{
				"grant_type":   {"authorization_code"},
				"client_id":    {proxyAgent.ID.String()},
				"code":         {code},
				"redirect_uri": {proxyRedirectURI},
			}.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		Expect(tokenBody["access_token"]).To(Equal("mock-access-token"))
		Expect(mockUpstream.GetTokenCalled()).To(BeTrue())
	})
})
