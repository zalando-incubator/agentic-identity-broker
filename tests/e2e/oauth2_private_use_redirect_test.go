package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

func issueCodeWithActiveGrant(server *bootstrap.TestServer, principal, clientID, redirectURI string) (code, verifier string) {
	verifier = helpers.PKCEVerifier()
	response, err := server.AuthenticatedGET("/oauth2/authorize?"+url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"state":                 {"native-app-state"},
		"code_challenge":        {helpers.GenerateCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}.Encode(), principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = response.Body.Close() }()
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	Expect(response.Header.Get("Location")).To(HavePrefix(redirectURI + "?"))
	callback, err := url.Parse(response.Header.Get("Location"))
	Expect(err).ToNot(HaveOccurred())
	Expect(callback.Scheme).To(Equal("com.example.app"))
	Expect(callback.Path).To(Equal("/oauth2redirect/example-provider"))
	Expect(callback.Query().Get("state")).To(Equal("native-app-state"))
	code = callback.Query().Get("code")
	Expect(code).ToNot(BeEmpty())
	return code, verifier
}

var _ = Describe("US7: Native App Private-Use Redirect URIs (local mode)", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		agent          *domainstorage.Agent
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		app, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())
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

	Context("when an operator writes redirect_uris through the Admin API", func() {
		BeforeEach(func() {
			Expect(fixtures.SeedPlaceholderGrantData(context.Background(), testStorage)).To(Succeed())
		})

		payload := func(uris []string) []byte {
			body, err := json.Marshal(map[string]any{
				"display_name":  "Example Desktop App",
				"description":   "Desktop assistant that signs in through its native callback",
				"redirect_uris": uris,
				"permission_sets": []map[string]string{{
					"permission_set_id": fixtures.PlaceholderPermissionSetID.String(),
					"requirement_type":  "mandatory",
				}},
			})
			Expect(err).ToNot(HaveOccurred())
			return body
		}

		// Scenario 7.1 from specs/025-oauth2-server/spec.md
		It("stores and returns create and update callbacks unchanged and in order", func() {
			uris := []string{"https://agent.example.com/callback", "http://127.0.0.1/callback", fixtures.GitHubAppCallback, fixtures.NativeAppCallback}
			response, err := adminServer.AuthenticatedPOST("/api/agents", "admin@example.com", "application/json", bytes.NewReader(payload(uris)))
			Expect(err).ToNot(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			var created struct {
				ID string `json:"id"`
			}
			Expect(json.NewDecoder(response.Body).Decode(&created)).To(Succeed())
			_ = response.Body.Close()
			Expect(created.ID).ToNot(BeEmpty())

			readRedirects := func() []string {
				resp, err := adminServer.AuthenticatedGET("/api/agents/"+created.ID, "admin@example.com")
				Expect(err).ToNot(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				var agentBody struct {
					RedirectURIs []string `json:"redirect_uris"`
				}
				Expect(json.NewDecoder(resp.Body).Decode(&agentBody)).To(Succeed())
				return agentBody.RedirectURIs
			}
			Expect(readRedirects()).To(Equal(uris))

			reversed := []string{fixtures.NativeAppCallback, fixtures.GitHubAppCallback, "http://127.0.0.1/callback", "https://agent.example.com/callback"}
			response, err = adminServer.DirectRequest(http.MethodPut, "/api/agents/"+created.ID, "admin@example.com", map[string]string{"Content-Type": "application/json"}, bytes.NewReader(payload(reversed)))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(readRedirects()).To(Equal(reversed))
		})

		// Scenario 7.2 from specs/025-oauth2-server/spec.md
		It("rejects unsafe, malformed and noncanonical callbacks at their index", func() {
			for _, uri := range []string{
				"javascript://oauth/callback", "data:text/html,callback", "file:///etc/passwd",
				"github-app://oauth/callback#fragment", "GitHub-App://oauth/callback",
				"github-app:oauth/callback", "github-app://user@oauth/callback",
				"github-app://oauth/../callback", "github-app://oauth/%63allback",
			} {
				By(uri)
				response, err := adminServer.AuthenticatedPOST("/api/agents", "admin@example.com", "application/json", bytes.NewReader(payload([]string{uri})))
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				var body struct{ Error, Message string }
				Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
				_ = response.Body.Close()
				Expect(body.Error).To(Equal("validation failed"))
				Expect(body.Message).To(ContainSubstring("redirect_uris[0]"))
			}
		})
	})

	Context("when a public local agent registers com.example.app:/oauth2redirect/example-provider", func() {
		BeforeEach(func() {
			ctx := context.Background()
			githubService := fixtures.GitHubService()
			Expect(testStorage.Services().Create(ctx, githubService)).To(Succeed())
			agent = fixtures.LocalAgent()
			agent.RedirectURIs = []string{fixtures.NativeAppCallback}
			Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())
			Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant("test@example.com", agent.ID.String(), githubService.ID.String(), []string{"repo", "user"}))).To(Succeed())
		})

		// Scenario 7.3 from specs/025-oauth2-server/spec.md
		It("issues a code to the native callback and exchanges it without client credentials", func() {
			code, verifier := issueCodeWithActiveGrant(enduserServer, "test@example.com", agent.ID.String(), fixtures.NativeAppCallback)
			response := exchangeAuthorizationCode(enduserServer, agent.ID.String(), code, fixtures.NativeAppCallback, verifier)
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			var token map[string]any
			Expect(json.NewDecoder(response.Body).Decode(&token)).To(Succeed())
			Expect(token["token_type"]).To(Equal("Bearer"))
			Expect(token["access_token"]).To(BeAssignableToTypeOf(""))
			Expect(token["access_token"]).ToNot(BeEmpty())
		})

		// Scenario 7.4 from specs/025-oauth2-server/spec.md
		It("rejects each look-alike authorization callback without redirecting", func() {
			for _, uri := range []string{
				"com.example.app:/oauth2redirect/other", "COM.EXAMPLE.APP:/oauth2redirect/example-provider",
				"com.example.app://oauth2redirect/example-provider", "com.example.app:/oauth2redirect/example-provider/",
				"com.example.app:/oauth2redirect/example-provider?x=1",
			} {
				By(uri)
				verifier := helpers.PKCEVerifier()
				response, err := enduserServer.AuthenticatedGET("/oauth2/authorize?"+url.Values{
					"response_type": {"code"}, "client_id": {agent.ID.String()}, "redirect_uri": {uri},
					"state": {"native-app-state"}, "code_challenge": {helpers.GenerateCodeChallenge(verifier)},
					"code_challenge_method": {"S256"},
				}.Encode(), "test@example.com")
				Expect(err).ToNot(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
				Expect(response).To(matchers.HaveOAuth2Error("invalid_redirect_uri"))
				Expect(response.Header.Get("Location")).To(BeEmpty())
				_ = response.Body.Close()
			}
		})

		// Scenario 7.5 from specs/025-oauth2-server/spec.md
		It("rejects a code exchange with a different native callback", func() {
			code, verifier := issueCodeWithActiveGrant(enduserServer, "test@example.com", agent.ID.String(), fixtures.NativeAppCallback)
			response := exchangeAuthorizationCode(enduserServer, agent.ID.String(), code, "com.example.app:/oauth2redirect/other", verifier)
			defer func() { _ = response.Body.Close() }()
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(response).To(matchers.HaveOAuth2Error("invalid_grant"))
		})
	})
})
