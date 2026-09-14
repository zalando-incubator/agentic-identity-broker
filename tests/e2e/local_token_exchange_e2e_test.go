package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Local Mode Token Exchange Validation", func() {
	var (
		logger          *slog.Logger
		storageFactory  *bootstrap.StorageFactory
		mockUpstream    *helpers.MockUpstreamOAuth2Server
		testStorage     *storageadapter.Adapter
		adminServer     *bootstrap.TestServer
		enduserServer   *bootstrap.TestServer
		principal       string
		agent           *domainstorage.Agent
		clientSecret    string
		tokenFixtures   *TokenFixtures
		githubServiceID string
	)

	const localAgentRedirectURI = "http://localhost:9999/callback"

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		principal = fixtures.DefaultPrincipal().String()

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		agent = fixtures.LocalAgent()
		agent.RedirectURIs = []string{localAgentRedirectURI}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

		ctx := context.Background()

		githubService := fixtures.GitHubService()
		githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
		githubService.Endpoints.AuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
		Expect(testStorage.Services().Create(ctx, githubService)).To(Succeed())
		githubServiceID = githubService.ID.String()
		Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)).To(Succeed())

		grant := fixtures.ActiveGrant(principal, agent.ID.String(), githubService.ID.String(), []string{"repo", "user"})
		Expect(testStorage.UserGrants().Create(ctx, grant)).To(Succeed())

		session := fixtures.GitHubSessionForPrincipal(principal)
		Expect(testStorage.UserSessions().Create(ctx, session)).To(Succeed())

		config := fixtures.LocalConfig()
		config.OAuth2AuthServer.Local.TokenClaimsExpression = `{"azp": agent.id, "aud": "token-exchange-broker"}`
		config.TokenExchange = ports.TokenExchangeConfig{
			ClientAssertion: ports.ClientAssertionTrustConfig{
				IssuerURI: mockUpstream.Server.URL,
			},
			ClaimExtraction: ports.ClaimExtractionConfig{
				PrincipalExpression: "subject_token.sub",
				AgentIDExpression:   "subject_token.azp",
			},
			Authorization: ports.AuthorizationConfig{
				Type: "cel",
				CEL: ports.CELAuthorizationConfig{
					Expression:        "true",
					EvaluationTimeout: 100 * time.Millisecond,
				},
			},
		}

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())

		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

		credResp, err := http.Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json",
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = credResp.Body.Close() }()
		Expect(credResp.StatusCode).To(Equal(http.StatusCreated))

		var creds map[string]any
		Expect(json.NewDecoder(credResp.Body).Decode(&creds)).To(Succeed())
		secret, ok := creds["client_secret"].(string)
		Expect(ok).To(BeTrue())
		Expect(secret).ToNot(BeEmpty())
		clientSecret = secret

		tokenFixtures = generateTokenFixtures(mockUpstream, principal, agent)
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

	mintLocalSubjectToken := func() string {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)
		authorizeURL := "/oauth2/authorize?" + url.Values{
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {localAgentRedirectURI},
			"response_type":         {"code"},
			"state":                 {"local-token-exchange-state"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		resp, err := enduserServer.AuthenticatedGET(authorizeURL, principal)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusFound))

		location := resp.Header.Get("Location")
		Expect(location).ToNot(BeEmpty())

		var code string
		if strings.HasPrefix(location, localAgentRedirectURI) {
			redirectURL, err := url.Parse(location)
			Expect(err).ToNot(HaveOccurred())
			code = redirectURL.Query().Get("code")
		} else {
			consentURL, err := url.Parse(location)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentURL.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty())

			grantBody, err := json.Marshal(map[string]any{
				"granted_permission_sets": map[string][]string{
					fixtures.PlaceholderPermissionSetID.String(): {githubServiceID},
				},
			})
			Expect(err).ToNot(HaveOccurred())

			grantResp, err := enduserServer.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, url.QueryEscape(sessionToken)),
				principal,
				"application/json",
				bytes.NewReader(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			var grantRespBody map[string]any
			Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
			reAuthorizeURL, _ := grantRespBody["redirect_url"].(string)
			Expect(reAuthorizeURL).ToNot(BeEmpty())

			codeResp, err := enduserServer.AuthenticatedGET(reAuthorizeURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = codeResp.Body.Close() }()
			Expect(codeResp.StatusCode).To(Equal(http.StatusFound))

			codeLocation, err := url.Parse(codeResp.Header.Get("Location"))
			Expect(err).ToNot(HaveOccurred())
			code = codeLocation.Query().Get("code")
		}

		Expect(code).ToNot(BeEmpty())

		tokenResp, err := http.Post(
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

		var tokenBody map[string]any
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).To(Succeed())
		accessToken, ok := tokenBody["access_token"].(string)
		Expect(ok).To(BeTrue())
		Expect(accessToken).ToNot(BeEmpty())

		return accessToken
	}

	postTokenExchange := func(subjectToken, clientAssertion string) *http.Response {
		resp, err := enduserServer.PublicPOST(
			"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(url.Values{
				"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
				"subject_token":         {subjectToken},
				"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
				"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
				"client_assertion":      {clientAssertion},
				"resource":              {"https://api.github.com"},
			}.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		return resp
	}

	// Scenario US7-S1 from specs/013-token-exchange/spec.md
	It("should accept a locally-issued subject token with an external client assertion", func() {
		resp := postTokenExchange(mintLocalSubjectToken(), tokenFixtures.ClientAssertion)
		defer func() { _ = resp.Body.Close() }()

		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body["access_token"]).To(Equal("github-token-xyz"))
	})

	// Scenario US7-S2 from specs/013-token-exchange/spec.md
	It("should reject a locally-issued client assertion", func() {
		resp := postTokenExchange(mintLocalSubjectToken(), mintLocalSubjectToken())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))

		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		Expect(body["error"]).To(Equal("invalid_client"))
	})
})
