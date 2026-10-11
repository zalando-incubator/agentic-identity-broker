package e2e_test

import (
	"context"
	"encoding/json"
	"io"
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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// Regression journeys for recurring third-party "logout" reports.
var _ = Describe("Third-Party Session Reliability", func() {
	var (
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		enduserServer  *bootstrap.TestServer
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		config         *ports.Config
		principal      string
		agent          *storagedomain.Agent
		tokenFixtures  *TokenFixtures
		githubID       id.ServiceID
	)

	BeforeEach(func() {
		logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}))
		mockUpstream = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
		config = fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL())

		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		app, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()
		principal = fixtures.DefaultPrincipal().String()
		agent = fixtures.ValidAgent()
		Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())
		tokenFixtures = generateTokenFixtures(mockUpstream, principal, agent)

		githubService := fixtures.GitHubService()
		githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
		githubService.Endpoints.AuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
		githubID = githubService.ID
		Expect(testStorage.Services().Create(ctx, githubService)).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubID)).To(Succeed())
		Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), githubID.String(), []string{"repo", "user"}))).To(Succeed())
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

	exchange := func() (*http.Response, map[string]any) {
		data := url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {tokenFixtures.SubjectToken},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {tokenFixtures.ClientAssertion},
			"resource":              {"https://api.github.com"},
		}
		resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return resp, body
	}

	completeConnect := func() {
		resp, err := enduserServer.AuthenticatedGET(
			"/api/third-party/"+githubID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape("http://localhost:8000/sessions"),
			principal,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		authorizeURL := resp.Header.Get("Location")
		Expect(resp.Body.Close()).To(Succeed())

		client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
		upstreamResp, err := client.Get(authorizeURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreamResp.StatusCode).To(Equal(http.StatusFound))
		callbackURL, err := url.Parse(upstreamResp.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(upstreamResp.Body.Close()).To(Succeed())

		callbackResp, err := enduserServer.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(callbackResp.StatusCode).To(Equal(http.StatusFound))
		Expect(callbackResp.Header.Get("Location")).To(ContainSubstring("success=true"))
		Expect(callbackResp.Body.Close()).To(Succeed())
	}

	// US5-S6 from specs/013-token-exchange/spec.md
	It("[US5-S6] survives a transient provider outage during refresh without demanding re-authentication", func() {
		ctx := context.Background()
		Expect(testStorage.UserSessions().Create(ctx, fixtures.ExpiredGitHubSessionForPrincipal(principal))).To(Succeed())
		mockUpstream.WithTransientTokenFailures(http.StatusServiceUnavailable, 1).WithAccessToken("recovered-after-outage")

		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("recovered-after-outage"))
		Expect(body).NotTo(HaveKey("error_uri"))
	})

	// US2-S10 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("[US2-S10] keeps the stored refresh token when the provider omits it on re-connect", func() {
		ctx := context.Background()
		mockUpstream.WithAccessToken("first-access").WithRefreshToken("long-lived-refresh")
		completeConnect()

		mockUpstream.WithAccessToken("second-access").WithRefreshToken("")
		completeConnect()

		// Force the next exchange through the refresh path and observe which refresh token is sent.
		session, err := testStorage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), githubID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session).NotTo(BeNil())
		Expect(session.EncryptedRefreshToken).NotTo(BeEmpty(), "re-connect must not drop the stored refresh token")
		expired := time.Now().Add(-time.Minute)
		session.AccessTokenExpiresAt = &expired
		Expect(testStorage.UserSessions().Create(ctx, session)).To(Succeed())

		mockUpstream.WithAccessToken("refreshed-access").WithRefreshToken("long-lived-refresh")
		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("refreshed-access"))
		form, err := url.ParseQuery(mockUpstream.GetLastBody())
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal("refresh_token"))
		Expect(form.Get("refresh_token")).To(Equal("long-lived-refresh"))
	})
})
