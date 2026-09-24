package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US4: Authorization Code Flow with PKCE (local mode)", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
		clientSecret   string
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()

		// Create a third-party service (required for grant delegation)
		githubService := fixtures.GitHubService()
		Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())

		// Create agent with redirect_uris
		agent = fixtures.LocalAgent()
		agent.RedirectURIs = []string{"http://localhost:9999/callback"}
		Expect(testStorage.Agents().Create(ctx, agent)).ToNot(HaveOccurred())

		// Create a grant for the principal so consent is satisfied.
		// Grant must include at least one GrantedPermissionSetEntry.
		grant := fixtures.ActiveGrant("test@example.com", agent.ID.String(), githubService.ID.String(), []string{"repo", "user"})
		Expect(testStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())

		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

		// Generate credentials
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
			"application/json", nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var creds map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&creds)).ToNot(HaveOccurred())
		clientSecret = creds["client_secret"].(string)
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 4.1 and 4.5 from specs/025-oauth2-server/spec.md
	It("full auth code flow with PKCE", func() {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		// Step 1: Authorize request (don't follow redirects)
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"state":                 {"test-state"},
			"scope":                 {"read"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Should redirect with code
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		location := resp.Header.Get("Location")
		Expect(location).To(ContainSubstring("code="))

		// Extract code from redirect
		locURL, _ := url.Parse(location)
		code := locURL.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Step 2: Exchange code for token
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"code":          {code},
			"redirect_uri":  {"http://localhost:9999/callback"},
			"code_verifier": {verifier},
		}
		tokenResp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))

		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		Expect(tokenBody).To(HaveKey("access_token"))
		Expect(tokenBody["token_type"]).To(Equal("Bearer"))
	})

	// Scenario 4.1 and 4.5 from specs/025-oauth2-server/spec.md
	Context("offline_access refresh tokens", func() {
		var client *http.Client

		BeforeEach(func() {
			client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}}
		})

		// requestToken runs a full authorization-code + PKCE flow for the given scope and
		// returns the decoded token endpoint response the client observes.
		requestToken := func(scope string) map[string]interface{} {
			verifier := helpers.PKCEVerifier()
			challenge := helpers.GenerateCodeChallenge(verifier)

			authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
				"response_type":         {"code"},
				"client_id":             {agent.ID.String()},
				"redirect_uri":          {"http://localhost:9999/callback"},
				"state":                 {"offline-state"},
				"scope":                 {scope},
				"code_challenge":        {challenge},
				"code_challenge_method": {"S256"},
			}.Encode()

			req, _ := http.NewRequest("GET", authURL, nil)
			req.Header.Set("X-Remote-User", "test@example.com")
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusFound))

			locURL, _ := url.Parse(resp.Header.Get("Location"))
			code := locURL.Query().Get("code")
			Expect(code).ToNot(BeEmpty())

			form := url.Values{
				"grant_type":    {"authorization_code"},
				"client_id":     {agent.ID.String()},
				"client_secret": {clientSecret},
				"code":          {code},
				"redirect_uri":  {"http://localhost:9999/callback"},
				"code_verifier": {verifier},
			}
			tokenResp, err := helpers.HTTPClient().Post(
				enduserServer.BaseURL()+"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(form.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = tokenResp.Body.Close() }()
			Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))

			var tokenBody map[string]interface{}
			Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
			return tokenBody
		}

		// exchangeRefreshToken performs a refresh_token grant at the token endpoint.
		exchangeRefreshToken := func(refreshToken string) *http.Response {
			refreshForm := url.Values{
				"grant_type":    {"refresh_token"},
				"client_id":     {agent.ID.String()},
				"client_secret": {clientSecret},
				"refresh_token": {refreshToken},
			}
			resp, err := helpers.HTTPClient().Post(
				enduserServer.BaseURL()+"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(refreshForm.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			return resp
		}

		Context("when offline_access is requested", func() {
			var tokenBody map[string]interface{}

			BeforeEach(func() {
				tokenBody = requestToken("read offline_access")
			})

			It("issues a refresh token alongside the access token", func() {
				Expect(tokenBody).To(HaveKey("access_token"))
				refreshToken, ok := tokenBody["refresh_token"].(string)
				Expect(ok).To(BeTrue())
				Expect(refreshToken).ToNot(BeEmpty())
			})

			It("rotates the refresh token and lets the client use the rotated one", func() {
				refreshToken := tokenBody["refresh_token"].(string)

				refreshResp := exchangeRefreshToken(refreshToken)
				defer func() { _ = refreshResp.Body.Close() }()
				Expect(refreshResp.StatusCode).To(Equal(http.StatusOK))

				var refreshedBody map[string]interface{}
				Expect(json.NewDecoder(refreshResp.Body).Decode(&refreshedBody)).ToNot(HaveOccurred())
				Expect(refreshedBody).To(HaveKey("access_token"))
				newRefreshToken, ok := refreshedBody["refresh_token"].(string)
				Expect(ok).To(BeTrue())
				Expect(newRefreshToken).ToNot(BeEmpty())
				Expect(newRefreshToken).ToNot(Equal(refreshToken))

				// The rotated refresh token is itself usable by the client.
				secondResp := exchangeRefreshToken(newRefreshToken)
				defer func() { _ = secondResp.Body.Close() }()
				Expect(secondResp.StatusCode).To(Equal(http.StatusOK))
			})

			It("rejects reuse of a rotated refresh token", func() {
				refreshToken := tokenBody["refresh_token"].(string)

				firstResp := exchangeRefreshToken(refreshToken)
				Expect(firstResp.StatusCode).To(Equal(http.StatusOK))
				_ = firstResp.Body.Close()

				reuseResp := exchangeRefreshToken(refreshToken)
				defer func() { _ = reuseResp.Body.Close() }()
				Expect(reuseResp.StatusCode).To(Equal(http.StatusBadRequest))

				var reuseBody map[string]interface{}
				Expect(json.NewDecoder(reuseResp.Body).Decode(&reuseBody)).ToNot(HaveOccurred())
				Expect(reuseBody["error"]).To(Equal("invalid_grant"))
			})
		})

		Context("when offline_access is not requested", func() {
			var tokenBody map[string]interface{}

			BeforeEach(func() {
				tokenBody = requestToken("read")
			})

			It("omits the refresh token", func() {
				Expect(tokenBody).To(HaveKey("access_token"))
				Expect(tokenBody).NotTo(HaveKey("refresh_token"))
			})
		})
	})

	// Scenario 4.2 from specs/025-oauth2-server/spec.md
	It("invalid redirect URI rejected", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://evil.example.com/callback"},
			"code_challenge":        {"test"},
			"code_challenge_method": {"S256"},
		}.Encode()

		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	// Scenario 4.3 from specs/025-oauth2-server/spec.md
	It("missing code_challenge rejected", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type": {"code"},
			"client_id":     {agent.ID.String()},
			"redirect_uri":  {"http://localhost:9999/callback"},
		}.Encode()

		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		// Should reject without PKCE
		Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusBadRequest), Equal(http.StatusFound)))
	})

	// Scenario 4.4 from specs/025-oauth2-server/spec.md
	It("consent redirect for new user", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		// Use a different principal that has no grant
		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "newuser@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		// Should redirect to the root-mounted agent view.
		Expect(resp.StatusCode).To(Equal(http.StatusFound))
		location := resp.Header.Get("Location")
		Expect(location).To(ContainSubstring("/agents/"))
	})

	// Scenario 4.6 from specs/025-oauth2-server/spec.md
	It("PKCE mismatch rejected on token exchange", func() {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		// Get code
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"state":                 {"s"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()
		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()

		locURL, _ := url.Parse(resp.Header.Get("Location"))
		code := locURL.Query().Get("code")

		// Exchange with wrong verifier
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"code":          {code},
			"redirect_uri":  {"http://localhost:9999/callback"},
			"code_verifier": {"wrong-verifier-value"},
		}
		tokenResp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	// Scenario 4.7 from specs/025-oauth2-server/spec.md
	It("code replay rejected", func() {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		// Get code
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"state":                 {"s"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()
		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		_ = resp.Body.Close()

		locURL, _ := url.Parse(resp.Header.Get("Location"))
		code := locURL.Query().Get("code")

		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"code":          {code},
			"redirect_uri":  {"http://localhost:9999/callback"},
			"code_verifier": {verifier},
		}

		// First exchange succeeds
		tokenResp, _ := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		_ = tokenResp.Body.Close()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))

		// Replay fails
		tokenResp2, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp2.Body.Close() }()
		Expect(tokenResp2.StatusCode).To(Equal(http.StatusBadRequest))
	})

})

var _ = Describe("US4b: Authorization Code Flow — LocalClient as public client (no credentials)", func() {
	var (
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()

		githubService := fixtures.GitHubService()
		Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())

		// LocalAgent with no credentials registered — public client.
		agent = fixtures.LocalAgent()
		agent.RedirectURIs = []string{"http://localhost:9999/callback"}
		Expect(testStorage.Agents().Create(ctx, agent)).ToNot(HaveOccurred())

		grant := fixtures.ActiveGrant("test@example.com", agent.ID.String(), githubService.ID.String(), []string{"repo", "user"})
		Expect(testStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

		// No client-credentials endpoint call — agent has no secret.
		serverFactory := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminSrv, err := bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(adminSrv.Close)
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminSrv.BaseURL())).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 4.5 from specs/025-oauth2-server/spec.md
	It("full auth code flow without client secret", func() {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"state":                 {"test-state"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()

		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusFound))

		locURL, _ := url.Parse(resp.Header.Get("Location"))
		code := locURL.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		// Token exchange with no client_secret.
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {agent.ID.String()},
			"code":          {code},
			"redirect_uri":  {"http://localhost:9999/callback"},
			"code_verifier": {verifier},
		}
		tokenResp, err := helpers.HTTPClient().Post(
			enduserServer.BaseURL()+"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResp.Body.Close() }()
		Expect(tokenResp.StatusCode).To(Equal(http.StatusOK))

		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(tokenResp.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		Expect(tokenBody).To(HaveKey("access_token"))
		Expect(tokenBody["token_type"]).To(Equal("Bearer"))
	})

	// Scenario 4.3 from specs/025-oauth2-server/spec.md
	It("authorize without code_challenge rejected", func() {
		client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}

		authURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type": {"code"},
			"client_id":     {agent.ID.String()},
			"redirect_uri":  {"http://localhost:9999/callback"},
		}.Encode()

		req, _ := http.NewRequest("GET", authURL, nil)
		req.Header.Set("X-Remote-User", "test@example.com")
		resp, err := client.Do(req)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusBadRequest), Equal(http.StatusFound)))
		if resp.StatusCode == http.StatusFound {
			locURL, _ := url.Parse(resp.Header.Get("Location"))
			Expect(locURL.Query().Get("error")).ToNot(BeEmpty(), "expected OAuth2 error in redirect")
			Expect(locURL.Query().Get("code")).To(BeEmpty(), "expected no authorization code to be issued")
		}
	})
})

var _ = Describe("OAuth2 Token Claims (principal profile)", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		agent          *domainstorage.Agent
		clientSecret   string
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		githubService := fixtures.GitHubService()
		Expect(testStorage.Services().Create(ctx, githubService)).ToNot(HaveOccurred())
		agent = fixtures.LocalAgent()
		agent.RedirectURIs = []string{"http://localhost:9999/callback"}
		Expect(testStorage.Agents().Create(ctx, agent)).ToNot(HaveOccurred())
		grant := fixtures.ActiveGrant("test@example.com", agent.ID.String(), githubService.ID.String(), []string{"repo", "user"})
		Expect(testStorage.UserGrants().Create(ctx, grant)).ToNot(HaveOccurred())

		app, err := bootstrap.NewServerFactory(fixtures.LocalConfigWithCEL(`{"pemail": principal.email, "pname": principal.display_name}`), logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())

		resp, err := helpers.HTTPClient().Post(adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials", "application/json", nil)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		var credentials map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&credentials)).ToNot(HaveOccurred())
		clientSecret = credentials["client_secret"].(string)
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 4.5 from specs/025-oauth2-server/spec.md
	It("should include the authenticated profile in locally issued token claims", func() {
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		authorizeURL := enduserServer.BaseURL() + "/oauth2/authorize?" + url.Values{
			"response_type":         {"code"},
			"client_id":             {agent.ID.String()},
			"redirect_uri":          {"http://localhost:9999/callback"},
			"scope":                 {"read offline_access"},
			"code_challenge":        {challenge},
			"code_challenge_method": {"S256"},
		}.Encode()
		authorizeRequest, err := http.NewRequest(http.MethodGet, authorizeURL, nil)
		Expect(err).ToNot(HaveOccurred())
		authorizeRequest.Header.Set("X-Remote-User", "test@example.com")
		authorizeResponse, err := client.Do(authorizeRequest)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = authorizeResponse.Body.Close() }()
		Expect(authorizeResponse.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(authorizeResponse.Header.Get("Location"))
		Expect(err).ToNot(HaveOccurred())
		code := location.Query().Get("code")
		Expect(code).ToNot(BeEmpty())

		tokenResponse, err := helpers.HTTPClient().Post(enduserServer.BaseURL()+"/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"code":          {code},
			"redirect_uri":  {"http://localhost:9999/callback"},
			"code_verifier": {verifier},
		}.Encode()))
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = tokenResponse.Body.Close() }()
		Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK))
		var tokenBody map[string]interface{}
		Expect(json.NewDecoder(tokenResponse.Body).Decode(&tokenBody)).ToNot(HaveOccurred())
		accessToken := tokenBody["access_token"].(string)
		parts := strings.Split(accessToken, ".")
		Expect(parts).To(HaveLen(3))
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		Expect(err).ToNot(HaveOccurred())
		var claims map[string]interface{}
		Expect(json.Unmarshal(payload, &claims)).To(Succeed())
		Expect(claims["pname"]).To(Equal("test@example.com"))
		Expect(claims["pemail"]).To(Equal(""))

		refreshToken, ok := tokenBody["refresh_token"].(string)
		Expect(ok).To(BeTrue())
		Expect(refreshToken).ToNot(BeEmpty())

		refreshResponse, err := helpers.HTTPClient().Post(enduserServer.BaseURL()+"/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {agent.ID.String()},
			"client_secret": {clientSecret},
			"refresh_token": {refreshToken},
		}.Encode()))
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = refreshResponse.Body.Close() }()
		Expect(refreshResponse.StatusCode).To(Equal(http.StatusOK))

		var refreshedBody map[string]interface{}
		Expect(json.NewDecoder(refreshResponse.Body).Decode(&refreshedBody)).ToNot(HaveOccurred())
		refreshedAccessToken := refreshedBody["access_token"].(string)
		refreshedParts := strings.Split(refreshedAccessToken, ".")
		Expect(refreshedParts).To(HaveLen(3))
		refreshedPayload, err := base64.RawURLEncoding.DecodeString(refreshedParts[1])
		Expect(err).ToNot(HaveOccurred())
		var refreshedClaims map[string]interface{}
		Expect(json.Unmarshal(refreshedPayload, &refreshedClaims)).To(Succeed())
		Expect(refreshedClaims["pname"]).To(Equal("test@example.com"))
		Expect(refreshedClaims["pemail"]).To(Equal(""))
	})
})
