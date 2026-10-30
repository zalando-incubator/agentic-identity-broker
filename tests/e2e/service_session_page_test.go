package e2e_test

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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Incremental Authorization Service Session Page", func() {
	var (
		storageFactory *bootstrap.StorageFactory
		store          *storageadapter.Adapter
		server         *bootstrap.TestServer
		provider       *helpers.MockUpstreamOAuth2Server
		config         *ports.Config
		principal      string
		service        *model.ThirdpartyOAuth2ProviderEntity
		agent          *storagedomain.Agent
		otherAgent     *storagedomain.Agent
		tokens         *TokenFixtures
	)

	BeforeEach(func() {
		logger := bootstrap.TestLogger(slog.LevelWarn)
		provider = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse().WithAccessToken("synthetic-repaired-access")
		config = fixtures.OAuth2ConfigWithTokenExchange(provider.URL())
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		store, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		application, err := bootstrap.NewServerFactory(config, logger).BuildApp(store)
		Expect(err).NotTo(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())

		ctx := context.Background()
		principal = fixtures.DefaultPrincipal().String()
		agent, otherAgent = fixtures.ValidAgent(), fixtures.AnotherAgent()
		Expect(store.Agents().Create(ctx, agent)).To(Succeed())
		Expect(store.Agents().Create(ctx, otherAgent)).To(Succeed())
		service = fixtures.GitHubService()
		canonical := "github"
		service.CanonicalID = &canonical
		service.DisplayName = "GitHub Production"
		service.Endpoints.AuthorizeEndpoint = provider.URL() + "/oauth/authorize"
		service.Endpoints.TokenEndpoint = provider.URL() + "/oauth/token"
		Expect(store.Services().Create(ctx, service)).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, store, service.ID)).To(Succeed())
		Expect(store.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), service.ID.String(), []string{"repo", "user"}))).To(Succeed())
		Expect(store.UserGrants().Create(ctx, fixtures.ActiveGrant(fixtures.AnotherPrincipal().String(), otherAgent.ID.String(), service.ID.String(), []string{"repo"}))).To(Succeed())
		Expect(store.UserSessions().Create(ctx, fixtures.SessionForService(fixtures.AnotherPrincipal().String(), service.ID.String()))).To(Succeed())
		tokens = generateTokenFixtures(provider, principal, agent)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if provider != nil {
			provider.Close()
		}
		if storageFactory != nil && store != nil {
			Expect(storageFactory.CloseStorage(store)).To(Succeed())
		}
	})

	readObject := func(response *http.Response) map[string]any {
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
		var body map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		return body
	}

	detail := func(identifier string) map[string]any {
		response, err := server.AuthenticatedGET("/api/third-party/sessions/"+identifier, principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var body struct {
			Data map[string]any `json:"data"`
		}
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		Expect(body.Data).To(HaveKey("session"))
		return body.Data
	}

	connect := func(action string) *url.URL {
		response, err := server.AuthenticatedGET(action, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		authorizationURL := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())
		callback, err := helpers.ProviderAuthorizationCallback(authorizationURL)
		Expect(err).NotTo(HaveOccurred())
		response, err = server.AuthenticatedGET(callback.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { Expect(response.Body.Close()).To(Succeed()) }()
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("service_id")).To(Equal(service.ID.String()))
		return location
	}

	exchange := func() (*http.Response, map[string]any) {
		form := url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {tokens.SubjectToken},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {tokens.ClientAssertion},
			"resource":              {"https://api.github.com"},
		}
		response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		return response, readObject(response)
	}

	Context("when a known service has no session", func() {
		// US2 S1 from specs/055-incremental-service-authorization/spec.md.
		It("shows the service with no session and returns to the same page after provider sign-in", func() {
			for _, identifier := range []string{"github", service.ID.String()} {
				before := len(provider.GetTokenRequests())
				data := detail(identifier)
				Expect(data["session"]).To(BeNil())
				Expect(data["service"]).To(HaveKeyWithValue("id", service.ID.String()))
				Expect(data["service"]).To(HaveKeyWithValue("display_name", "GitHub Production"))
				Expect(data["service"]).To(HaveKeyWithValue("scope_url", config.Server.EndUser.PublicURL+"/sessions/github"))
				Expect(data["dependent_agents"]).To(HaveLen(1))
				Expect(data["dependent_agents"]).To(ContainElement(HaveKeyWithValue("agent_id", agent.ID.String())))
				Expect(len(provider.GetTokenRequests())).To(Equal(before))

				returnURL := config.Server.EndUser.PublicURL + "/sessions/" + identifier
				location := connect("/api/third-party/" + service.ID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(returnURL))
				Expect(location.Scheme + "://" + location.Host + location.Path).To(Equal(returnURL))
				data = detail(identifier)
				Expect(data["session"]).To(HaveKeyWithValue("is_expired", false))
				Expect(data["session"]).To(HaveKeyWithValue("has_refresh_token", true))
				Expect(data["session"]).To(HaveKeyWithValue("scope", ContainElement("repo")))

				response, err := server.DirectRequest(http.MethodDelete, "/api/third-party/"+service.ID.String()+"/session", principal, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())
				Expect(detail(identifier)["session"]).To(BeNil())
			}
		})

		// US2 S3 from specs/055-incremental-service-authorization/spec.md.
		It("requires authentication and returns not found for an unknown service", func() {
			for _, identifier := range []string{"github", service.ID.String(), "unknown-service", id.NewServiceID().String()} {
				response, err := server.PublicGET("/api/third-party/sessions/" + identifier)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusUnauthorized))
				Expect(response.Body.Close()).To(Succeed())
			}
			for _, identifier := range []string{"unknown-service", id.NewServiceID().String()} {
				response, err := server.AuthenticatedGET("/api/third-party/sessions/"+identifier, principal)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNotFound))
				body := readObject(response)
				Expect(body).To(HaveKeyWithValue("error", "not_found"))
				Expect(body).To(HaveKeyWithValue("message", "service not found"))
			}
			Expect(detail("github")["session"]).To(BeNil())
			Expect(provider.GetTokenRequests()).To(BeEmpty())
		})

		// US2 S4 from specs/055-incremental-service-authorization/spec.md.
		It("returns from the recovery link to the service page and the next exchange succeeds without agent authorization", func() {
			ctx := context.Background()
			grant, err := store.UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(principal), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			response, body := exchange()
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_grant"))
			Expect(body["scope"]).To(Equal(config.Server.EndUser.PublicURL + "/sessions/github"))
			recovery, err := url.Parse(body["error_uri"].(string))
			Expect(err).NotTo(HaveOccurred())
			Expect(recovery.Scheme + "://" + recovery.Host).To(Equal(config.Server.EndUser.PublicURL))
			Expect(recovery.Path).To(Equal("/api/third-party/" + service.ID.String() + "/oauth2/authorize"))
			Expect(recovery.Query().Get("redirect_uri")).To(Equal(config.Server.EndUser.PublicURL + "/sessions/github"))
			location := connect(recovery.RequestURI())
			Expect(location.Path).To(Equal("/sessions/github"))
			Expect(detail("github")["session"]).To(HaveKeyWithValue("is_expired", false))
			response, body = exchange()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(body["access_token"]).To(Equal("synthetic-repaired-access"))
			unchanged, err := store.UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(principal), agent.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(unchanged).To(Equal(grant))
			Expect(provider.GetAuthorizationRequestCount()).To(Equal(1))
		})
	})
})
