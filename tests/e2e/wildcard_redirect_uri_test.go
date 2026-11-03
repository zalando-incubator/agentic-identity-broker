package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

const (
	wildcardRedirectPattern = "https://preview*.app-*.example.com/api/oidc/callback"
	wildcardRedirectUser    = "wildcard-redirect@example.test"
)

var _ = Describe("Wildcard Redirect URI", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		wildcardAgent  string
		registerAgent  func([]string) (*http.Response, error)
		authorizeRaw   func(string, string, string, string) (*http.Response, error)
		authorize      func(string, string, string) (*http.Response, error)
		expectRedirect func(*http.Response, string, string)
		expectRejected func(*http.Response)
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.NewPrincipal(wildcardRedirectUser))).To(Succeed())

		registerAgent = func(redirectURIs []string) (*http.Response, error) {
			payload, marshalErr := json.Marshal(map[string]any{
				"display_name":  "Wildcard Redirect Test Agent",
				"description":   "Agent used to verify bounded redirect URI matching",
				"redirect_uris": redirectURIs,
				"permission_sets": []map[string]string{{
					"permission_set_id": fixtures.PlaceholderPermissionSetID.String(),
					"requirement_type":  "mandatory",
				}},
			})
			Expect(marshalErr).ToNot(HaveOccurred())
			return adminServer.AuthenticatedPOST("/api/agents", "admin@example.test", "application/json", bytes.NewReader(payload))
		}

		authorizeRaw = func(agentID, redirectURI, state, verifier string) (*http.Response, error) {
			query := url.Values{
				"response_type":         {"code"},
				"client_id":             {agentID},
				"redirect_uri":          {redirectURI},
				"state":                 {state},
				"code_challenge":        {helpers.GenerateCodeChallenge(verifier)},
				"code_challenge_method": {"S256"},
			}
			return enduserServer.AuthenticatedGET("/oauth2/authorize?"+query.Encode(), wildcardRedirectUser)
		}

		authorize = func(agentID, redirectURI, state string) (*http.Response, error) {
			verifier := helpers.PKCEVerifier()
			response, requestErr := authorizeRaw(agentID, redirectURI, state, verifier)
			if requestErr != nil {
				return nil, requestErr
			}
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location, parseErr := url.Parse(response.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			if sessionToken := location.Query().Get("session_token"); sessionToken != "" {
				_ = response.Body.Close()
				grantBody, marshalErr := json.Marshal(map[string]any{
					"granted_permission_sets": map[string][]string{
						fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
					},
				})
				Expect(marshalErr).ToNot(HaveOccurred())
				grantResponse, grantErr := enduserServer.AuthenticatedPOST(
					"/api/consent/agents/"+agentID+"/grants?session_token="+url.QueryEscape(sessionToken),
					wildcardRedirectUser,
					"application/json",
					bytes.NewReader(grantBody),
				)
				Expect(grantErr).ToNot(HaveOccurred())
				Expect(grantResponse.StatusCode).To(Equal(http.StatusCreated))
				var grantResult map[string]any
				Expect(json.NewDecoder(grantResponse.Body).Decode(&grantResult)).To(Succeed())
				_ = grantResponse.Body.Close()
				reAuthorizeURL, ok := grantResult["redirect_url"].(string)
				Expect(ok).To(BeTrue())
				Expect(reAuthorizeURL).ToNot(BeEmpty())
				response, requestErr = enduserServer.AuthenticatedGET(reAuthorizeURL, wildcardRedirectUser)
				Expect(requestErr).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusFound))
			}

			location, parseErr = url.Parse(response.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			code := location.Query().Get("code")
			Expect(code).ToNot(BeEmpty())
			tokenResponse, tokenErr := helpers.HTTPClient().Post(
				enduserServer.BaseURL()+"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(url.Values{
					"grant_type":    {"authorization_code"},
					"client_id":     {agentID},
					"code":          {code},
					"redirect_uri":  {redirectURI},
					"code_verifier": {verifier},
				}.Encode()),
			)
			Expect(tokenErr).ToNot(HaveOccurred())
			Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK))
			var tokenBody map[string]any
			Expect(json.NewDecoder(tokenResponse.Body).Decode(&tokenBody)).To(Succeed())
			_ = tokenResponse.Body.Close()
			Expect(tokenBody).To(HaveKey("access_token"))
			Expect(tokenBody["token_type"]).To(Equal("Bearer"))
			return response, nil
		}

		expectRedirect = func(response *http.Response, redirectURI, state string) {
			defer func() { _ = response.Body.Close() }()
			body, readErr := io.ReadAll(response.Body)
			Expect(readErr).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound), string(body))

			location, parseErr := url.Parse(response.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			target, parseErr := url.Parse(redirectURI)
			Expect(parseErr).ToNot(HaveOccurred())
			Expect(location.Scheme).To(Equal(target.Scheme))
			Expect(location.Host).To(Equal(target.Host))
			Expect(location.Path).To(Equal(target.Path))
			Expect(location.Query().Get("state")).To(Equal(state))
			Expect(location.Query().Get("code")).ToNot(BeEmpty())
		}

		expectRejected = func(response *http.Response) {
			defer func() { _ = response.Body.Close() }()
			Expect(response).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(response).To(matchers.HaveOAuth2Error("invalid_redirect_uri"))
			Expect(response.Header.Get("Location")).To(BeEmpty())
		}

		response, err := registerAgent([]string{wildcardRedirectPattern})
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		var created struct {
			ID string `json:"id"`
		}
		Expect(json.NewDecoder(response.Body).Decode(&created)).To(Succeed())
		_ = response.Body.Close()
		Expect(created.ID).ToNot(BeEmpty())
		wildcardAgent = created.ID
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if adminServer != nil {
			adminServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// US1-S1 from specs/055-wildcard-redirect-uri/spec.md
	It("redirects to the first matching pull-request preview callback", func() {
		const callback = "https://preview-pr-123.app-test.example.com/api/oidc/callback"
		response, err := authorize(wildcardAgent, callback, "pr-123")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(response, callback, "pr-123")
	})

	// US1-S2 from specs/055-wildcard-redirect-uri/spec.md
	It("redirects to a second matching pull-request preview callback", func() {
		const callback = "https://preview-pr-456.app-test.example.com/api/oidc/callback"
		response, err := authorize(wildcardAgent, callback, "pr-456")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(response, callback, "pr-456")
	})

	// US1-S3 from specs/055-wildcard-redirect-uri/spec.md
	It("accepts the stable test callback host", func() {
		const callback = "https://preview.app-test.example.com/api/oidc/callback"
		response, err := authorize(wildcardAgent, callback, "stable-test")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(response, callback, "stable-test")
	})

	// US1-S4 from specs/055-wildcard-redirect-uri/spec.md
	It("accepts the sandbox callback host", func() {
		const callback = "https://preview.app-sandbox.example.com/api/oidc/callback"
		response, err := authorize(wildcardAgent, callback, "sandbox")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(response, callback, "sandbox")
	})

	// US1-S5 from specs/055-wildcard-redirect-uri/spec.md
	It("accepts the staging callback host", func() {
		const callback = "https://preview.app-staging.example.com/api/oidc/callback"
		response, err := authorize(wildcardAgent, callback, "staging")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(response, callback, "staging")
	})

	// US2-S1 from specs/055-wildcard-redirect-uri/spec.md
	It("rejects callbacks outside the registered domain", func() {
		validCallback := "https://preview-pr-123.app-test.example.com/api/oidc/callback"
		validResponse, err := authorize(wildcardAgent, validCallback, "valid-control")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(validResponse, validCallback, "valid-control")

		response, err := authorizeRaw(wildcardAgent, "https://preview-pr-123.app-test.example.net/api/oidc/callback", "outside-domain", helpers.PKCEVerifier())
		Expect(err).ToNot(HaveOccurred())
		expectRejected(response)
	})

	// US2-S2 from specs/055-wildcard-redirect-uri/spec.md
	It("rejects scheme and path mismatches", func() {
		validCallback := "https://preview-pr-123.app-test.example.com/api/oidc/callback"
		validResponse, err := authorize(wildcardAgent, validCallback, "valid-control")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(validResponse, validCallback, "valid-control")

		for _, callback := range []string{
			"http://preview-pr-123.app-test.example.com/api/oidc/callback",
			"https://preview-pr-123.app-test.example.com/other/callback",
		} {
			response, requestErr := authorizeRaw(wildcardAgent, callback, "out-of-scope", helpers.PKCEVerifier())
			Expect(requestErr).ToNot(HaveOccurred())
			expectRejected(response)
		}
	})

	// US2-S3 from specs/055-wildcard-redirect-uri/spec.md
	It("rejects patterns that wildcard the registrable domain or full host", func() {
		for _, pattern := range []string{
			"https://preview*.app-*.example.*/api/oidc/callback",
			"https://preview*.app-*.*.com/api/oidc/callback",
			"https://*/api/oidc/callback",
		} {
			By("registering " + pattern)
			response, err := registerAgent([]string{pattern})
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			var body struct {
				Error string `json:"error"`
			}
			Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
			_ = response.Body.Close()
			Expect(body.Error).To(Equal("validation failed"))
		}
	})

	// US2-S4 from specs/055-wildcard-redirect-uri/spec.md
	It("continues to accept an existing exact callback registration", func() {
		const callback = "https://exact.example.com/api/oidc/callback"
		response, err := registerAgent([]string{callback})
		Expect(err).ToNot(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		var created struct {
			ID string `json:"id"`
		}
		Expect(json.NewDecoder(response.Body).Decode(&created)).To(Succeed())
		_ = response.Body.Close()
		Expect(created.ID).ToNot(BeEmpty())
		authorization, err := authorize(created.ID, callback, "exact-callback")
		Expect(err).ToNot(HaveOccurred())
		expectRedirect(authorization, callback, "exact-callback")
	})
})
