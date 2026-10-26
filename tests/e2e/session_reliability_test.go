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

	startConnect := func() *url.URL {
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
		return callbackURL
	}

	finishConnect := func(callbackURL *url.URL) *url.URL {
		callbackResp, err := enduserServer.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(callbackResp.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(callbackResp.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(callbackResp.Body.Close()).To(Succeed())
		return location
	}

	completeConnect := func() *url.URL {
		return finishConnect(startConnect())
	}

	// US5-S6 from specs/013-token-exchange/spec.md
	It("[US5-S6] survives a transient provider outage during refresh without demanding re-authentication", func() {
		ctx := context.Background()
		Expect(testStorage.UserSessions().Create(ctx, fixtures.ExpiredGitHubSessionForPrincipal(principal))).To(Succeed())
		mockUpstream.WithTransientTokenFailures(http.StatusServiceUnavailable, 1).WithAccessToken("recovered-after-outage")

		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		recoveredToken, ok := body["access_token"].(string)
		Expect(ok && recoveredToken == "recovered-after-outage").To(BeTrue())
		_, hasErrorURI := body["error_uri"]
		Expect(hasErrorURI).To(BeFalse())
	})

	// US2-S10 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("[US2-S10] accepts reconnect without a refresh token and requires login after expiry", func() {
		ctx := context.Background()
		mockUpstream.WithAccessToken("account-a-access").WithRefreshToken("account-a-refresh")
		Expect(completeConnect().Query().Get("success")).To(Equal("true"))
		before, err := testStorage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), githubID)
		Expect(err).NotTo(HaveOccurred())
		refreshExpiry := time.Now().Add(time.Hour)
		before.RefreshTokenExpiresAt = &refreshExpiry
		Expect(testStorage.UserSessions().Create(ctx, before)).To(Succeed())

		mockUpstream.WithAccessToken("account-b-access").WithOmittedRefreshToken()
		Expect(completeConnect().Query().Get("success")).To(Equal("true"))
		session, err := testStorage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), githubID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session.ID).To(Equal(before.ID))
		Expect(session.EncryptedRefreshToken).To(BeEmpty())
		Expect(session.RefreshTokenExpiresAt).To(BeNil())
		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		connectedToken, ok := body["access_token"].(string)
		Expect(ok && connectedToken == "account-b-access").To(BeTrue())
		expired := time.Now().Add(-time.Minute)
		session.AccessTokenExpiresAt = &expired
		Expect(testStorage.UserSessions().Create(ctx, session)).To(Succeed())
		resp, body = exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(body["error"]).To(Equal("invalid_grant"))
		recoveryURI := config.Server.EndUser.PublicURL + "/api/third-party/" + githubID.String() + "/oauth2/authorize?redirect_uri=" +
			url.QueryEscape(config.Server.EndUser.PublicURL+"/sessions")
		Expect(body["error_uri"]).To(Equal(recoveryURI))

		mockUpstream.WithRefreshToken("account-b-refresh")
		Expect(completeConnect().Query().Get("success")).To(Equal("true"))
		session, err = testStorage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), githubID)
		Expect(err).NotTo(HaveOccurred())
		session.AccessTokenExpiresAt = &expired
		Expect(testStorage.UserSessions().Create(ctx, session)).To(Succeed())
		mockUpstream.WithAccessToken("account-b-refreshed")
		resp, body = exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		refreshedToken, ok := body["access_token"].(string)
		Expect(ok && refreshedToken == "account-b-refreshed").To(BeTrue())
		form, err := url.ParseQuery(mockUpstream.GetLastBody())
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("refresh_token") == "account-b-refresh").To(BeTrue())
	})

	// US2-S11 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("[US2-S11] keeps the first completed callback and accepts a later deliberate reconnect", func() {
		first, second := startConnect(), startConnect()
		mockUpstream.WithAccessToken("initial-winner")
		Expect(finishConnect(second).Query().Get("success")).To(Equal("true"))
		mockUpstream.WithAccessToken("initial-loser")
		Expect(finishConnect(first).Query().Get("success")).To(Equal("true"))
		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("initial-winner"))

		first, second = startConnect(), startConnect()
		mockUpstream.WithAccessToken("reconnect-winner")
		Expect(finishConnect(first).Query().Get("success")).To(Equal("true"))
		mockUpstream.WithAccessToken("reconnect-loser")
		Expect(finishConnect(second).Query().Get("success")).To(Equal("true"))
		resp, body = exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("reconnect-winner"))
		mockUpstream.WithAccessToken("deliberate-reconnect")
		Expect(completeConnect().Query().Get("success")).To(Equal("true"))
		resp, body = exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("deliberate-reconnect"))

		pending := startConnect()
		deleted, err := enduserServer.DirectRequest(http.MethodDelete, "/api/third-party/"+githubID.String()+"/session", principal, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(deleted.StatusCode).To(Equal(http.StatusOK))
		Expect(deleted.Body.Close()).To(Succeed())
		Expect(finishConnect(pending).Query().Get("success")).NotTo(Equal("true"))
		resp, body = exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(body["error"]).To(Equal("invalid_grant"))
	})

	// US1-S8 from specs/013-token-exchange/spec.md
	It("[US1-S8] refreshes and persists a provider lifetime encoded as a JSON string", func() {
		ctx := context.Background()
		Expect(testStorage.UserSessions().Create(ctx, fixtures.ExpiredGitHubSessionForPrincipal(principal))).To(Succeed())
		mockUpstream.WithAccessToken("string-expiry-access").WithStringExpiresIn(3600)
		before := time.Now()
		resp, body := exchange()
		Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		Expect(body["access_token"]).To(Equal("string-expiry-access"))
		Expect(body["expires_in"]).To(BeNumerically("~", 3600, 2))
		session, err := testStorage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), githubID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session.AccessTokenExpiresAt).NotTo(BeNil())
		Expect(*session.AccessTokenExpiresAt).To(BeTemporally(">=", before.Add(time.Hour)))
		Expect(*session.AccessTokenExpiresAt).To(BeTemporally("<=", time.Now().Add(time.Hour)))
		requests := mockUpstream.GetTokenRequests()
		Expect(requests).To(HaveLen(1))
		form, err := url.ParseQuery(requests[0].Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal("refresh_token"))
		Expect(form.Get("refresh_token")).To(Equal("github-refresh-xyz"))
	})
})
