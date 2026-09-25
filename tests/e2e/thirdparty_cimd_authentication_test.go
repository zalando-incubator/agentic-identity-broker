package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD third-party client authentication", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	seedUsableCIMDKey := func() string {
		keyID := seedCIMDClientAuthenticationKey(testStorage)
		_, err := testStorage.SigningKeys().SetCurrentInDomain(
			context.Background(),
			domainstorage.KeyDomainCIMDClientAuthentication,
			id.NewKeyID(keyID),
			time.Now().UTC(),
		)
		Expect(err).NotTo(HaveOccurred())
		return keyID
	}

	initiateConnect := func(server *bootstrap.TestServer, principal string, serviceID id.ServiceID) *url.URL {
		response, err := server.AuthenticatedGET(
			"/api/third-party/"+serviceID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/done"),
			principal,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())

		authorizationURL, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		return authorizationURL
	}

	visitProviderAuthorization := func(authorizationURL *url.URL) *url.URL {
		providerClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		response, err := providerClient.Get(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())

		callbackURL, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		return callbackURL
	}

	completeConnect := func(server *bootstrap.TestServer, principal string, serviceID id.ServiceID) *http.Response {
		callbackURL := visitProviderAuthorization(initiateConnect(server, principal, serviceID))
		response, err := server.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	refreshSession := func(server *bootstrap.TestServer, principal string, serviceID id.ServiceID) *http.Response {
		response, err := server.AuthenticatedPOST(
			"/api/third-party/"+serviceID.String()+"/session/refresh",
			principal,
			"application/json",
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	Context("when the broker operates in local mode", func() {
		var (
			enduserServer *bootstrap.TestServer
			principal     string
			service       *model.ThirdpartyOAuth2ProviderEntity
			upstream      *helpers.MockCIMDUpstream
		)

		BeforeEach(func() {
			principal = fixtures.DefaultPrincipal().String()
			serverFactory := bootstrap.NewServerFactory(fixtures.CIMDLocalConfig(), logger)

			var brokerHTTPClient *http.Client
			var err error
			enduserServer, brokerHTTPClient, err = bootstrap.NewCIMDClientEndUserTestServer(
				testStorage,
				serverFactory,
				fixtures.CIMDEndUserPublicURL,
				logger,
			)
			Expect(err).NotTo(HaveOccurred())

			upstream = helpers.NewMockCIMDUpstream(helpers.WithCIMDUpstreamHTTPClient(brokerHTTPClient))
			service = fixtures.CIMDConfidentialService()
			service.Endpoints.AuthorizeEndpoint = upstream.AuthorizationEndpoint()
			service.Endpoints.TokenEndpoint = upstream.TokenEndpoint()
			Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())
			seedUsableCIMDKey()
		})

		AfterEach(func() {
			if enduserServer != nil {
				enduserServer.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
		})

		// US3-S1 from specs/046-cimd-upstream-client/spec.md
		It("identifies the broker with its metadata URL and S256 PKCE", Label("cimd-upstream-client"), func() {
			authorizationURL := initiateConnect(enduserServer, principal, service.ID)

			Expect(authorizationURL.Query().Get("client_id")).To(Equal(service.ClientID.String()))
			Expect(authorizationURL.Query().Get("code_challenge")).NotTo(BeEmpty())
			Expect(authorizationURL.Query().Get("code_challenge_method")).To(Equal("S256"))

			visitProviderAuthorization(authorizationURL)
			observations := upstream.Observations()
			Expect(observations.AuthorizationRequests).To(HaveLen(1))
			Expect(observations.AuthorizationRequests[0].MetadataValidated).To(BeTrue())
			Expect(observations.AuthorizationRequests[0].PKCEValidated).To(BeTrue())
		})

		// US3-S2 from specs/046-cimd-upstream-client/spec.md
		It("exchanges the authorization code with a signed client assertion", Label("cimd-upstream-client"), func() {
			response := completeConnect(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			observations := upstream.Observations()
			Expect(observations.TokenRequests).To(HaveLen(1))
			request := observations.TokenRequests[0]
			Expect(request.GrantType).To(Equal("authorization_code"))
			Expect(request.ClientIDValidated).To(BeTrue())
			Expect(request.AssertionValidated).To(BeTrue())
			Expect(request.PKCEValidated).To(BeTrue())
			Expect(request.SecretAuthenticationReject).To(BeFalse())
		})

		// US3-S3 from specs/046-cimd-upstream-client/spec.md
		It("refreshes with a signed assertion and replaces encrypted tokens", Label("cimd-upstream-client"), func() {
			response := completeConnect(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			before, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), service.ID)
			Expect(err).NotTo(HaveOccurred())
			previousAccessToken := append([]byte(nil), before.EncryptedAccessToken...)
			previousRefreshToken := append([]byte(nil), before.EncryptedRefreshToken...)
			expired := time.Now().Add(-time.Minute)
			before.AccessTokenExpiresAt = &expired
			Expect(testStorage.UserSessions().Create(context.Background(), before)).To(Succeed())

			response = refreshSession(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())

			after, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), service.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(after.EncryptedAccessToken).NotTo(Equal(previousAccessToken))
			Expect(after.EncryptedRefreshToken).NotTo(Equal(previousRefreshToken))

			observations := upstream.Observations()
			Expect(observations.TokenRequests).To(HaveLen(2))
			refreshRequest := observations.TokenRequests[1]
			Expect(refreshRequest.GrantType).To(Equal("refresh_token"))
			Expect(refreshRequest.AssertionValidated).To(BeTrue())
			Expect(refreshRequest.SecretAuthenticationReject).To(BeFalse())
		})

		// US3-S4 from specs/046-cimd-upstream-client/spec.md
		It("fails closed when the provider rejects its client assertion", Label("cimd-upstream-client"), func() {
			upstream.WithTokenError("invalid_client")

			response := completeConnect(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			failureURL, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(failureURL.Path).To(Equal("/sessions"))
			Expect(failureURL.Query().Get("error")).To(Equal("callback_failed"))

			observations := upstream.Observations()
			Expect(observations.TokenRequests).To(HaveLen(1))
			request := observations.TokenRequests[0]
			Expect(request.AssertionValidated).To(BeTrue())
			Expect(request.SecretAuthenticationReject).To(BeFalse())

			session, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), service.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeNil())
		})

		Context("and existing public and static confidential services connect", func() {
			var (
				publicService  *model.ThirdpartyOAuth2ProviderEntity
				publicUpstream *helpers.MockUpstreamOAuth2Server
				staticService  *model.ThirdpartyOAuth2ProviderEntity
				staticUpstream *helpers.MockUpstreamOAuth2Server
			)

			expireSession := func(serviceID id.ServiceID) {
				session, err := testStorage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), serviceID)
				Expect(err).NotTo(HaveOccurred())
				expired := time.Now().Add(-time.Minute)
				session.AccessTokenExpiresAt = &expired
				Expect(testStorage.UserSessions().Create(context.Background(), session)).To(Succeed())
			}

			BeforeEach(func() {
				staticUpstream = helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
				staticService = fixtures.CIMDStaticConfidentialService()
				staticService.Endpoints.AuthorizeEndpoint = staticUpstream.URL() + "/oauth/authorize"
				staticService.Endpoints.TokenEndpoint = staticUpstream.URL() + "/oauth/token"
				Expect(testStorage.Services().Create(context.Background(), staticService)).To(Succeed())

				publicUpstream = helpers.NewMockUpstreamOAuth2Server().
					WithStrictPublicClientMode().
					WithSuccessfulTokenResponse()
				publicService = fixtures.CIMDPublicService()
				publicService.Endpoints.AuthorizeEndpoint = publicUpstream.URL() + "/oauth/authorize"
				publicService.Endpoints.TokenEndpoint = publicUpstream.URL() + "/oauth/token"
				Expect(testStorage.Services().Create(context.Background(), publicService)).To(Succeed())
			})

			AfterEach(func() {
				if staticUpstream != nil {
					staticUpstream.Close()
				}
				if publicUpstream != nil {
					publicUpstream.Close()
				}
			})

			// US3-S5 from specs/046-cimd-upstream-client/spec.md
			It("keeps public and static client behavior unchanged", Label("cimd-upstream-client"), func() {
				response := completeConnect(enduserServer, principal, staticService.ID)
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				Expect(response.Body.Close()).To(Succeed())
				expireSession(staticService.ID)
				response = refreshSession(enduserServer, principal, staticService.ID)
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())

				expectedAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte("static-confidential-cimd-fixture-client:fixture-static-confidential-credential"))
				staticRequests := staticUpstream.GetTokenRequests()
				Expect(staticRequests).To(HaveLen(2))
				Expect(staticRequests[0].Header.Get("Authorization")).To(Equal(expectedAuthorization))
				Expect(staticRequests[1].Header.Values("Authorization")).To(BeEmpty())
				refreshForm, err := url.ParseQuery(staticRequests[1].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(refreshForm.Get("client_id")).To(Equal(staticService.ClientID.String()))
				Expect(refreshForm.Get("client_secret")).To(Equal("fixture-static-confidential-credential"))

				response = completeConnect(enduserServer, principal, publicService.ID)
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				Expect(response.Body.Close()).To(Succeed())
				expireSession(publicService.ID)
				response = refreshSession(enduserServer, principal, publicService.ID)
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Body.Close()).To(Succeed())

				publicRequests := publicUpstream.GetTokenRequests()
				Expect(publicRequests).To(HaveLen(2))
				for _, request := range publicRequests {
					Expect(request.Header.Values("Authorization")).To(BeEmpty())
					form, err := url.ParseQuery(request.Body)
					Expect(err).NotTo(HaveOccurred())
					Expect(form.Get("client_id")).To(Equal(publicService.ClientID.String()))
					Expect(form).NotTo(HaveKey("client_secret"))
				}
			})
		})

		// US3-S6 from specs/046-cimd-upstream-client/spec.md
		It("sends exactly one valid JWT-bearer assertion pair", Label("cimd-upstream-client"), func() {
			response := completeConnect(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			observations := upstream.Observations()
			Expect(observations.TokenRequests).To(HaveLen(1))
			request := observations.TokenRequests[0]
			Expect(request.ClientIDValidated).To(BeTrue())
			Expect(request.ClientAssertionTypeCount).To(Equal(1))
			Expect(request.ClientAssertionCount).To(Equal(1))
			Expect(request.JWTBearerAssertionType).To(BeTrue())
			Expect(request.AssertionKeyID).NotTo(BeEmpty())
			Expect(request.AssertionValidated).To(BeTrue())
			Expect(request.SecretAuthenticationReject).To(BeFalse())
		})

		Context("and local token issuance is configured", func() {
			var (
				adminServer  *bootstrap.TestServer
				clientSecret string
				localAgentID id.AgentID
			)

			BeforeEach(func() {
				var err error
				adminServer, err = bootstrap.NewAdminTestServer(enduserServer.App(), logger)
				Expect(err).NotTo(HaveOccurred())

				agent := fixtures.LocalAgent()
				Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
				localAgentID = agent.ID
				Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())

				response, err := http.Post(
					adminServer.BaseURL()+"/api/agents/"+agent.ID.String()+"/client-credentials",
					"application/json",
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusCreated))
				var credentials map[string]any
				Expect(json.NewDecoder(response.Body).Decode(&credentials)).To(Succeed())
				Expect(response.Body.Close()).To(Succeed())
				var ok bool
				clientSecret, ok = credentials["client_secret"].(string)
				Expect(ok).To(BeTrue())
				Expect(clientSecret).NotTo(BeEmpty())
			})

			AfterEach(func() {
				if adminServer != nil {
					adminServer.Close()
				}
			})

			// US3-S8 from specs/046-cimd-upstream-client/spec.md
			It("keeps CIMD and token signing keys in separate domains", Label("cimd-upstream-client"), func() {
				response := completeConnect(enduserServer, principal, service.ID)
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				Expect(response.Body.Close()).To(Succeed())

				observations := upstream.Observations()
				Expect(observations.TokenRequests).To(HaveLen(1))
				assertionKeyID := observations.TokenRequests[0].AssertionKeyID
				Expect(assertionKeyID).NotTo(BeEmpty())
				Expect(observations.CachedKeyIDs).To(ContainElement(assertionKeyID))

				form := url.Values{
					"grant_type":    {"client_credentials"},
					"client_id":     {localAgentID.String()},
					"client_secret": {clientSecret},
				}
				response, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				var tokenResponse map[string]any
				Expect(json.NewDecoder(response.Body).Decode(&tokenResponse)).To(Succeed())
				Expect(response.Body.Close()).To(Succeed())
				accessToken, ok := tokenResponse["access_token"].(string)
				Expect(ok).To(BeTrue())
				Expect(accessToken).NotTo(BeEmpty())

				message, err := jws.Parse([]byte(accessToken))
				Expect(err).NotTo(HaveOccurred())
				Expect(message.Signatures()).To(HaveLen(1))
				accessTokenKeyID, ok := message.Signatures()[0].ProtectedHeaders().KeyID()
				Expect(ok).To(BeTrue())
				Expect(accessTokenKeyID).NotTo(BeEmpty())
				Expect(accessTokenKeyID).NotTo(Equal(assertionKeyID))
				Expect(observations.CachedKeyIDs).NotTo(ContainElement(accessTokenKeyID))

				jwksResponse, err := enduserServer.PublicGET("/oauth2/jwks.json")
				Expect(err).NotTo(HaveOccurred())
				Expect(jwksResponse.StatusCode).To(Equal(http.StatusOK))
				tokenJWKS, err := jwk.ParseReader(jwksResponse.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(jwksResponse.Body.Close()).To(Succeed())
				_, exposesAssertionKey := tokenJWKS.LookupKeyID(assertionKeyID)
				Expect(exposesAssertionKey).To(BeFalse())
				_, exposesAccessTokenKey := tokenJWKS.LookupKeyID(accessTokenKeyID)
				Expect(exposesAccessTokenKey).To(BeTrue())
			})
		})
	})

	Context("when the broker operates in proxy mode", func() {
		var (
			enduserServer *bootstrap.TestServer
			principal     string
			proxyUpstream *helpers.MockUpstreamOAuth2Server
			service       *model.ThirdpartyOAuth2ProviderEntity
			upstream      *helpers.MockCIMDUpstream
		)

		BeforeEach(func() {
			principal = fixtures.DefaultPrincipal().String()
			proxyUpstream = helpers.NewMockUpstreamOAuth2Server()
			serverFactory := bootstrap.NewServerFactory(fixtures.CIMDProxyConfig(proxyUpstream.URL()), logger)

			var brokerHTTPClient *http.Client
			var err error
			enduserServer, brokerHTTPClient, err = bootstrap.NewCIMDClientEndUserTestServer(
				testStorage,
				serverFactory,
				fixtures.CIMDEndUserPublicURL,
				logger,
			)
			Expect(err).NotTo(HaveOccurred())

			upstream = helpers.NewMockCIMDUpstream(helpers.WithCIMDUpstreamHTTPClient(brokerHTTPClient))
			service = fixtures.CIMDConfidentialService()
			service.Endpoints.AuthorizeEndpoint = upstream.AuthorizationEndpoint()
			service.Endpoints.TokenEndpoint = upstream.TokenEndpoint()
			Expect(testStorage.Services().Create(context.Background(), service)).To(Succeed())
			seedUsableCIMDKey()
		})

		AfterEach(func() {
			if enduserServer != nil {
				enduserServer.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
			if proxyUpstream != nil {
				proxyUpstream.Close()
			}
		})

		// US3-S7 from specs/046-cimd-upstream-client/spec.md
		It("uses its dedicated CIMD identity and keys in proxy mode", Label("cimd-upstream-client"), func() {
			response := completeConnect(enduserServer, principal, service.ID)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())

			observations := upstream.Observations()
			Expect(observations.AuthorizationRequests).To(HaveLen(1))
			Expect(observations.AuthorizationRequests[0].MetadataValidated).To(BeTrue())
			Expect(observations.TokenRequests).To(HaveLen(1))
			request := observations.TokenRequests[0]
			Expect(request.AssertionValidated).To(BeTrue())
			Expect(request.AssertionKeyID).NotTo(BeEmpty())
			Expect(observations.CachedKeyIDs).To(ContainElement(request.AssertionKeyID))
			Expect(proxyUpstream.GetAuthorizeCalled()).To(BeFalse())
			Expect(proxyUpstream.GetTokenRequests()).To(BeEmpty())
		})
	})
})
