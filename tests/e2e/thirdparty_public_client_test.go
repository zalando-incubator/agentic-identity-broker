package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Public Client Support for Third-Party OAuth2 Services", func() {
	serviceRequest := func(upstreamURL string) map[string]any {
		if upstreamURL == "" {
			upstreamURL = "https://provider.example.test"
		}
		return map[string]any{
			"display_name":  "Public Client Test Service",
			"client_id":     "public-client-id",
			"client_secret": "confidential-secret",
			"issuer_uri":    upstreamURL,
			"discovery":     map[string]any{"enable_discovery": false},
			"endpoints": map[string]any{
				"token_endpoint":     upstreamURL + "/oauth/token",
				"authorize_endpoint": upstreamURL + "/oauth/authorize",
			},
			"scopes": []map[string]any{{"scope_value": "profile", "description": "Profile"}},
		}
	}

	postJSON := func(server *bootstrap.TestServer, path, principal string, body map[string]any) *http.Response {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		response, err := server.AuthenticatedPOST(path, principal, "application/json", bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	putJSON := func(server *bootstrap.TestServer, path, principal string, body map[string]any) *http.Response {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		response, err := server.DirectRequest(http.MethodPut, path, principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(encoded))
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	upstreamForm := func(upstream *helpers.MockUpstreamOAuth2Server) url.Values {
		form, err := url.ParseQuery(upstream.GetLastBody())
		Expect(err).NotTo(HaveOccurred())
		return form
	}

	credentialFreeTokenRequests := func(upstream *helpers.MockUpstreamOAuth2Server) {
		requests := upstream.GetTokenRequests()
		Expect(requests).NotTo(BeEmpty())
		for _, request := range requests {
			Expect(request.Header.Values("Authorization")).To(BeEmpty())
			form, err := url.ParseQuery(request.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(form).NotTo(HaveKey("client_secret"))
		}
	}
	initiateFlow := func(server *bootstrap.TestServer, principal string, serviceID id.ServiceID) *url.URL {
		response, err := server.AuthenticatedGET(
			"/api/third-party/"+serviceID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape("http://localhost:8000/done"),
			principal,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())
		parsed, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		return parsed
	}

	visitUpstreamAuthorize := func(authorizationURL *url.URL) *url.URL {
		client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		response, err := client.Get(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())
		callbackURL, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		return callbackURL
	}

	completeFlow := func(server *bootstrap.TestServer, principal string, serviceID id.ServiceID) *http.Response {
		callbackURL := visitUpstreamAuthorize(initiateFlow(server, principal, serviceID))
		response, err := server.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	Context("User Story 1: register a public third-party service", func() {
		var (
			adminServer    *bootstrap.TestServer
			storageFactory *bootstrap.StorageFactory
			testStorage    *storageadapter.Adapter
			logger         *slog.Logger
			principal      string
		)

		BeforeEach(func() {
			logger = bootstrap.TestLogger(slog.LevelWarn)
			storageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			application, err := bootstrap.NewServerFactory(fixtures.DefaultOAuth2Config(), logger).BuildApp(testStorage)
			Expect(err).NotTo(HaveOccurred())
			adminServer, err = bootstrap.NewAdminTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			principal = fixtures.DefaultPrincipal().String()
		})

		AfterEach(func() {
			if adminServer != nil {
				adminServer.Close()
			}
			if storageFactory != nil && testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// US1-S1 from specs/042-thirdparty-public-pkce/spec.md
		It("creates the service and reports method none with no credential", func() {
			request := serviceRequest("")
			request["token_endpoint_auth_method"] = "none"
			delete(request, "client_secret")

			response := postJSON(adminServer, "/api/services", principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			service := decodeJSON[map[string]any](response)
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(service).NotTo(HaveKey("client_secret"))
		})

		// US1-S2 from specs/042-thirdparty-public-pkce/spec.md
		It("rejects the contradiction and creates no service", func() {
			request := serviceRequest("")
			request["token_endpoint_auth_method"] = "none"
			request["client_secret"] = "must-not-be-stored"

			response := postJSON(adminServer, "/api/services", principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorBody := decodeJSON[map[string]any](response)
			Expect(errorBody).To(HaveKeyWithValue("error", "validation failed"))
			Expect(errorBody).To(HaveKeyWithValue("message", "client_secret must not have a non-empty value when token_endpoint_auth_method is \"none\""))

			listed, err := adminServer.AuthenticatedGET("/api/services", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[[]map[string]any](listed)).To(BeEmpty())
		})

		// US1-S3 from specs/042-thirdparty-public-pkce/spec.md
		It("creates a confidential service with unchanged credential requirements", func() {
			response := postJSON(adminServer, "/api/services", principal, serviceRequest(""))
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			service := decodeJSON[map[string]any](response)
			Expect(service).To(HaveKey("token_endpoint_auth_method"))
			Expect(service["token_endpoint_auth_method"]).To(BeNil())
			Expect(service).To(HaveKeyWithValue("client_secret", "REDACTED"))
		})

		// US1-S4 from specs/042-thirdparty-public-pkce/spec.md
		It("rejects a confidential service with no client secret exactly as today", func() {
			request := serviceRequest("")
			request["client_secret"] = ""

			response := postJSON(adminServer, "/api/services", principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorBody := decodeJSON[map[string]any](response)
			Expect(errorBody).To(HaveKeyWithValue("error", "validation failed"))
			Expect(errorBody).To(HaveKeyWithValue("message", "client_secret is required"))
		})

		// US1-S5 from specs/042-thirdparty-public-pkce/spec.md
		It("rejects none for the google variant, naming it", func() {
			request := fixtures.ValidGoogleServiceRequest()
			request["token_endpoint_auth_method"] = "none"
			delete(request, "client_secret")

			response := postJSON(adminServer, "/api/services", principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorBody := decodeJSON[map[string]any](response)
			Expect(errorBody).To(HaveKeyWithValue("error", "validation failed"))
			Expect(errorBody).To(HaveKeyWithValue("message", "token_endpoint_auth_method \"none\" is not supported for the google flavor: the client identifier is derived from the credential document"))
		})

		// US1-S6 from specs/042-thirdparty-public-pkce/spec.md
		It("reports the method for every entry and omits the credential for public ones", func() {
			publicRequest := serviceRequest("")
			publicRequest["display_name"] = "Public catalogue service"
			publicRequest["token_endpoint_auth_method"] = "none"
			delete(publicRequest, "client_secret")
			publicResponse := postJSON(adminServer, "/api/services", principal, publicRequest)
			Expect(publicResponse.StatusCode).To(Equal(http.StatusCreated))
			publicService := decodeJSON[map[string]any](publicResponse)

			confidentialRequest := serviceRequest("")
			confidentialRequest["display_name"] = "Confidential catalogue service"
			confidentialResponse := postJSON(adminServer, "/api/services", principal, confidentialRequest)
			Expect(confidentialResponse.StatusCode).To(Equal(http.StatusCreated))
			confidentialService := decodeJSON[map[string]any](confidentialResponse)

			response, err := adminServer.AuthenticatedGET("/api/services", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			services := decodeJSON[[]map[string]any](response)
			var publicEntry, confidentialEntry map[string]any
			for _, service := range services {
				switch service["id"] {
				case publicService["id"]:
					publicEntry = service
				case confidentialService["id"]:
					confidentialEntry = service
				}
			}
			Expect(publicEntry).NotTo(BeNil())
			Expect(publicEntry).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(publicEntry).NotTo(HaveKey("client_secret"))
			Expect(confidentialEntry).NotTo(BeNil())
			Expect(confidentialEntry).To(HaveKey("token_endpoint_auth_method"))
			Expect(confidentialEntry["token_endpoint_auth_method"]).To(BeNil())
			Expect(confidentialEntry).To(HaveKeyWithValue("client_secret", "REDACTED"))
		})
	})

	Context("User Story 2: authorize a user against a public-client provider", func() {
		var (
			enduserServer  *bootstrap.TestServer
			storageFactory *bootstrap.StorageFactory
			testStorage    *storageadapter.Adapter
			logger         *slog.Logger
			principal      string
			publicService  *model.ThirdpartyOAuth2ProviderEntity
			upstream       *helpers.MockUpstreamOAuth2Server
		)

		BeforeEach(func() {
			logger = bootstrap.TestLogger(slog.LevelWarn)
			upstream = helpers.NewMockUpstreamOAuth2Server().WithStrictPublicClientMode().WithSuccessfulTokenResponse()
			storageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			application, err := bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithUpstream(upstream.URL()), logger).BuildApp(testStorage)
			Expect(err).NotTo(HaveOccurred())
			enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			principal = fixtures.DefaultPrincipal().String()
			publicService = fixtures.PublicClientService()
			publicService.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
			publicService.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
			Expect(testStorage.Services().Create(context.Background(), publicService)).To(Succeed())
		})

		AfterEach(func() {
			if enduserServer != nil {
				enduserServer.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
			if storageFactory != nil && testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// US2-S1 from specs/042-thirdparty-public-pkce/spec.md
		It("sends code_challenge and code_challenge_method S256 upstream", func() {
			service := publicService
			authorizationURL := initiateFlow(enduserServer, principal, service.ID)
			visitUpstreamAuthorize(authorizationURL)

			Expect(upstream.GetAuthorizeCalled()).To(BeTrue())
			Expect(authorizationURL.Query().Get("code_challenge")).NotTo(BeEmpty())
			Expect(authorizationURL.Query().Get("code_challenge_method")).To(Equal("S256"))
		})

		// US2-S2 from specs/042-thirdparty-public-pkce/spec.md
		It("sends client_id and code_verifier with no client_secret in the body", func() {
			service := publicService
			response := completeFlow(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			credentialFreeTokenRequests(upstream)

			form := upstreamForm(upstream)
			Expect(form.Get("client_id")).To(Equal(service.ClientID.String()))
			Expect(form.Get("code_verifier")).NotTo(BeEmpty())
			Expect(form).NotTo(HaveKey("client_secret"))
		})

		// US2-S3 from specs/042-thirdparty-public-pkce/spec.md
		It("sends no authorization header, including none with an empty password", func() {
			service := publicService
			response := completeFlow(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			credentialFreeTokenRequests(upstream)
		})
		// US2-S4 from specs/042-thirdparty-public-pkce/spec.md
		It("stores an encrypted session exactly as for a confidential service", func() {
			service := publicService
			response := completeFlow(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			session, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), service.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).NotTo(BeNil())
			Expect(session.EncryptedAccessToken).NotTo(BeEmpty())
			Expect(session.EncryptedRefreshToken).NotTo(BeEmpty())
		})

		// US2-S5 from specs/042-thirdparty-public-pkce/spec.md
		It("fails closed without any retry adding a credential", func() {
			service := publicService
			upstream.WithErrorResponse("temporarily_unavailable")

			response := completeFlow(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location := response.Header.Get("Location")
			Expect(response.Body.Close()).To(Succeed())
			callbackFailure, err := url.Parse(location)
			Expect(err).NotTo(HaveOccurred())
			Expect(callbackFailure.Path).To(Equal("/sessions"))
			Expect(callbackFailure.Query().Get("error")).To(Equal("callback_failed"))
			credentialFreeTokenRequests(upstream)
		})

		// US2-S6 from specs/042-thirdparty-public-pkce/spec.md
		It("leaves the confidential connect flow unchanged", func() {
			confidentialUpstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
			DeferCleanup(confidentialUpstream.Close)
			service := fixtures.ServiceWithID(id.NewServiceID().String())
			service.ClientID = id.ClientID("confidential-client")
			service.Secret = fixtures.EncryptedSecret(service.ID.String(), "confidential-secret")
			service.Endpoints.TokenEndpoint = confidentialUpstream.URL() + "/oauth/token"
			service.Endpoints.AuthorizeEndpoint = confidentialUpstream.URL() + "/oauth/authorize"
			Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())

			response := completeFlow(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			request := confidentialUpstream.GetLastRequest()
			Expect(request).NotTo(BeNil())
			expectedAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("confidential-client:confidential-secret"))
			Expect(request.Header.Get("Authorization")).To(Equal(expectedAuthorization))
		})
	})

	Context("User Story 3: keep a public-client session alive", func() {
		var (
			adminServer    *bootstrap.TestServer
			enduserServer  *bootstrap.TestServer
			storageFactory *bootstrap.StorageFactory
			testStorage    *storageadapter.Adapter
			logger         *slog.Logger
			principal      string
			serviceID      id.ServiceID
			upstream       *helpers.MockUpstreamOAuth2Server
		)

		BeforeEach(func() {
			logger = bootstrap.TestLogger(slog.LevelWarn)
			upstream = helpers.NewMockUpstreamOAuth2Server().
				WithStrictPublicClientMode().
				WithSuccessfulTokenResponse().
				WithAccessToken("initial-public-access-token").
				WithRefreshToken("initial-public-refresh-token")
			storageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			application, err := bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithUpstream(upstream.URL()), logger).BuildApp(testStorage)
			Expect(err).NotTo(HaveOccurred())
			adminServer, err = bootstrap.NewAdminTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			principal = fixtures.DefaultPrincipal().String()

			request := serviceRequest(upstream.URL())
			request["token_endpoint_auth_method"] = "none"
			delete(request, "client_secret")
			created := postJSON(adminServer, "/api/services", principal, request)
			Expect(created.StatusCode).To(Equal(http.StatusCreated))
			serviceID = id.MustParseServiceID(decodeJSON[map[string]any](created)["id"].(string))

			callback := completeFlow(enduserServer, principal, serviceID)
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())

			session, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			expired := time.Now().Add(-time.Minute)
			session.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), session)).To(Succeed())
		})

		AfterEach(func() {
			if adminServer != nil {
				adminServer.Close()
			}
			if enduserServer != nil {
				enduserServer.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
			if storageFactory != nil && testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		refresh := func(serviceID id.ServiceID) *http.Response {
			response, err := enduserServer.AuthenticatedPOST("/api/third-party/"+serviceID.String()+"/session/refresh", principal, "application/json", nil)
			Expect(err).NotTo(HaveOccurred())
			return response
		}

		// US3-S1 from specs/042-thirdparty-public-pkce/spec.md
		It("refreshes with client_id and refresh_token and no client_secret", func() {
			response := refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())

			form := upstreamForm(upstream)
			Expect(form.Get("client_id")).To(Equal("public-client-id"))
			Expect(form.Get("refresh_token")).To(Equal("initial-public-refresh-token"))
			Expect(form).NotTo(HaveKey("client_secret"))
			credentialFreeTokenRequests(upstream)
		})

		// US3-S2 from specs/042-thirdparty-public-pkce/spec.md
		It("encrypts and persists the refreshed tokens, replacing the previous ones", func() {
			before, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			previousAccess := append([]byte(nil), before.EncryptedAccessToken...)
			previousRefresh := append([]byte(nil), before.EncryptedRefreshToken...)
			upstream.WithAccessToken("refreshed-public-access-token").WithRefreshToken("refreshed-public-refresh-token")

			response := refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			after, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.EncryptedAccessToken).NotTo(Equal(previousAccess))
			Expect(after.EncryptedRefreshToken).NotTo(Equal(previousRefresh))

			expired := time.Now().Add(-time.Minute)
			after.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), after)).To(Succeed())
			response = refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			Expect(upstreamForm(upstream).Get("refresh_token")).To(Equal("refreshed-public-refresh-token"))
		})

		// US3-S3 from specs/042-thirdparty-public-pkce/spec.md
		It("leaves the confidential refresh request unchanged", func() {
			confidentialUpstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
			DeferCleanup(confidentialUpstream.Close)
			service := fixtures.ServiceWithID(id.NewServiceID().String())
			service.ClientID = id.ClientID("confidential-client")
			service.Secret = fixtures.EncryptedSecret(service.ID.String(), "confidential-secret")
			service.Endpoints.TokenEndpoint = confidentialUpstream.URL() + "/oauth/token"
			service.Endpoints.AuthorizeEndpoint = confidentialUpstream.URL() + "/oauth/authorize"
			Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())
			session := fixtures.SessionForService(principal, service.ID.String())
			expired := time.Now().Add(-time.Minute)
			session.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), session)).To(Succeed())

			response := refresh(service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			form := upstreamForm(confidentialUpstream)
			Expect(form.Get("client_id")).To(Equal("confidential-client"))
			Expect(form.Get("client_secret")).To(Equal("confidential-secret"))
			Expect(form.Get("refresh_token")).To(Equal("refresh-" + service.ID.String()))
			Expect(confidentialUpstream.GetLastRequest().Header.Get("Authorization")).To(BeEmpty())
		})

		// US3-S4 from specs/042-thirdparty-public-pkce/spec.md
		It("surfaces the failure without retrying with a credential or downgrading", func() {
			before, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			previousAccess := append([]byte(nil), before.EncryptedAccessToken...)
			previousRefresh := append([]byte(nil), before.EncryptedRefreshToken...)
			upstream.WithErrorResponse("invalid_grant")

			response := refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(response.Body.Close()).To(Succeed())
			credentialFreeTokenRequests(upstream)
			after, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.EncryptedAccessToken).To(Equal(previousAccess))
			Expect(after.EncryptedRefreshToken).To(Equal(previousRefresh))
		})
	})

	Context("User Story 4: change an existing service between public and confidential", func() {
		var (
			adminServer    *bootstrap.TestServer
			enduserServer  *bootstrap.TestServer
			storageFactory *bootstrap.StorageFactory
			testStorage    *storageadapter.Adapter
			logger         *slog.Logger
			principal      string
			upstream       *helpers.MockUpstreamOAuth2Server
		)

		create := func(request map[string]any) map[string]any {
			response := postJSON(adminServer, "/api/services", principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusCreated))
			return decodeJSON[map[string]any](response)
		}

		get := func(serviceID string) map[string]any {
			response, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			return decodeJSON[map[string]any](response)
		}

		refresh := func(serviceID id.ServiceID) *http.Response {
			response, err := enduserServer.AuthenticatedPOST("/api/third-party/"+serviceID.String()+"/session/refresh", principal, "application/json", nil)
			Expect(err).NotTo(HaveOccurred())
			return response
		}

		BeforeEach(func() {
			logger = bootstrap.TestLogger(slog.LevelWarn)
			upstream = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
			storageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			application, err := bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithUpstream(upstream.URL()), logger).BuildApp(testStorage)
			Expect(err).NotTo(HaveOccurred())
			adminServer, err = bootstrap.NewAdminTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
			Expect(err).NotTo(HaveOccurred())
			principal = fixtures.DefaultPrincipal().String()
		})

		AfterEach(func() {
			if adminServer != nil {
				adminServer.Close()
			}
			if enduserServer != nil {
				enduserServer.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
			if storageFactory != nil && testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// US4-S1 from specs/042-thirdparty-public-pkce/spec.md
		It("removes the stored credential when the service becomes public", func() {
			created := create(serviceRequest(upstream.URL()))
			request := serviceRequest(upstream.URL())
			request["token_endpoint_auth_method"] = "none"
			delete(request, "client_secret")

			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, request)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			updated := decodeJSON[map[string]any](response)
			Expect(updated).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(updated).NotTo(HaveKey("client_secret"))
			stored, err := testStorage.Services().Get(context.Background(), id.MustParseServiceID(created["id"].(string)))
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.Secret.IsAbsent()).To(BeTrue())
		})

		// US4-S2 from specs/042-thirdparty-public-pkce/spec.md
		It("rejects the update and leaves the public service intact", func() {
			publicRequest := serviceRequest(upstream.URL())
			publicRequest["token_endpoint_auth_method"] = "none"
			delete(publicRequest, "client_secret")
			created := create(publicRequest)
			invalidUpdate := serviceRequest(upstream.URL())
			delete(invalidUpdate, "client_secret")

			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, invalidUpdate)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorBody := decodeJSON[map[string]any](response)
			Expect(errorBody).To(HaveKeyWithValue("message", "client_secret is required"))
			stored := get(created["id"].(string))
			Expect(stored).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(stored).NotTo(HaveKey("client_secret"))
		})

		// US4-S3 from specs/042-thirdparty-public-pkce/spec.md
		It("makes the service confidential and sends that credential upstream", func() {
			publicRequest := serviceRequest(upstream.URL())
			publicRequest["token_endpoint_auth_method"] = "none"
			delete(publicRequest, "client_secret")
			created := create(publicRequest)
			confidentialRequest := serviceRequest(upstream.URL())
			confidentialRequest["client_secret"] = "restored-secret"

			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, confidentialRequest)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			updated := decodeJSON[map[string]any](response)
			Expect(updated).To(HaveKey("token_endpoint_auth_method"))
			Expect(updated["token_endpoint_auth_method"]).To(BeNil())
			Expect(updated).To(HaveKeyWithValue("client_secret", "REDACTED"))
			serviceID := id.MustParseServiceID(created["id"].(string))
			flow := completeFlow(enduserServer, principal, serviceID)
			Expect(flow.StatusCode).To(Equal(http.StatusFound))
			Expect(flow.Body.Close()).To(Succeed())
			expectedAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("public-client-id:restored-secret"))
			Expect(upstream.GetLastRequest().Header.Get("Authorization")).To(Equal(expectedAuthorization))
		})

		// US4-S4 from specs/042-thirdparty-public-pkce/spec.md
		It("keeps the service public with no credential", func() {
			publicRequest := serviceRequest(upstream.URL())
			publicRequest["token_endpoint_auth_method"] = "none"
			delete(publicRequest, "client_secret")
			created := create(publicRequest)
			renameRequest := serviceRequest(upstream.URL())
			renameRequest["display_name"] = "Renamed public service"
			renameRequest["token_endpoint_auth_method"] = "none"
			delete(renameRequest, "client_secret")

			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, renameRequest)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			updated := decodeJSON[map[string]any](response)
			Expect(updated).To(HaveKeyWithValue("display_name", "Renamed public service"))
			Expect(updated).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
			Expect(updated).NotTo(HaveKey("client_secret"))
		})

		// US4-S5 from specs/042-thirdparty-public-pkce/spec.md
		It("keeps stored sessions valid and uses the new setting on the next refresh", func() {
			created := create(serviceRequest(upstream.URL()))
			serviceID := id.MustParseServiceID(created["id"].(string))
			flow := completeFlow(enduserServer, principal, serviceID)
			Expect(flow.StatusCode).To(Equal(http.StatusFound))
			Expect(flow.Body.Close()).To(Succeed())
			before, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			previousID := before.ID
			previousAccess := append([]byte(nil), before.EncryptedAccessToken...)
			previousRefresh := append([]byte(nil), before.EncryptedRefreshToken...)

			publicRequest := serviceRequest(upstream.URL())
			publicRequest["token_endpoint_auth_method"] = "none"
			delete(publicRequest, "client_secret")
			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, publicRequest)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())

			afterPublicUpdate, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterPublicUpdate.ID).To(Equal(previousID))
			Expect(afterPublicUpdate.EncryptedAccessToken).To(Equal(previousAccess))
			Expect(afterPublicUpdate.EncryptedRefreshToken).To(Equal(previousRefresh))

			upstream.WithStrictPublicClientMode().
				WithAccessToken("post-public-transition-access-token").
				WithRefreshToken("post-public-transition-refresh-token")
			upstream.Reset()
			expired := time.Now().Add(-time.Minute)
			afterPublicUpdate.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), afterPublicUpdate)).To(Succeed())
			response = refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			credentialFreeTokenRequests(upstream)

			afterPublicRefresh, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			publicAccess := append([]byte(nil), afterPublicRefresh.EncryptedAccessToken...)
			publicRefresh := append([]byte(nil), afterPublicRefresh.EncryptedRefreshToken...)

			confidentialUpstream := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse().WithAccessToken("post-confidential-transition-access-token")
			DeferCleanup(confidentialUpstream.Close)
			confidentialRequest := serviceRequest(confidentialUpstream.URL())
			confidentialRequest["client_secret"] = "restored-secret"
			response = putJSON(adminServer, "/api/services/"+created["id"].(string), principal, confidentialRequest)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())

			afterConfidentialUpdate, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
			Expect(err).NotTo(HaveOccurred())
			Expect(afterConfidentialUpdate.ID).To(Equal(previousID))
			Expect(afterConfidentialUpdate.EncryptedAccessToken).To(Equal(publicAccess))
			Expect(afterConfidentialUpdate.EncryptedRefreshToken).To(Equal(publicRefresh))

			expired = time.Now().Add(-time.Minute)
			afterConfidentialUpdate.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), afterConfidentialUpdate)).To(Succeed())
			response = refresh(serviceID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			form := upstreamForm(confidentialUpstream)
			Expect(form.Get("grant_type")).To(Equal("refresh_token"))
			Expect(form.Get("client_id")).To(Equal("public-client-id"))
			Expect(form).To(HaveKey("client_secret"))
			Expect(form.Get("client_secret")).To(Equal("restored-secret"))
			Expect(form.Get("refresh_token")).To(Equal("post-public-transition-refresh-token"))
			Expect(confidentialUpstream.GetLastRequest().Header.Get("Authorization")).To(BeEmpty())
		})

		// US4-S6 from specs/042-thirdparty-public-pkce/spec.md
		It("accepts the null token_endpoint_auth_method a read returned and keeps the service confidential", func() {
			created := create(serviceRequest(upstream.URL()))
			read := get(created["id"].(string))
			Expect(read).To(HaveKey("token_endpoint_auth_method"))
			Expect(read["token_endpoint_auth_method"]).To(BeNil())
			read["display_name"] = "Read-modify-write confidential service"
			read["client_secret"] = "confidential-secret"

			response := putJSON(adminServer, "/api/services/"+created["id"].(string), principal, read)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			updated := decodeJSON[map[string]any](response)
			Expect(updated).To(HaveKeyWithValue("display_name", "Read-modify-write confidential service"))
			Expect(updated).To(HaveKey("token_endpoint_auth_method"))
			Expect(updated["token_endpoint_auth_method"]).To(BeNil())
			Expect(updated).To(HaveKeyWithValue("client_secret", "REDACTED"))
		})
	})
})
