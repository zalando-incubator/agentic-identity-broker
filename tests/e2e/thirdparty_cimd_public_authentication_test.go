package e2e_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD public third-party authentication", func() {
	type journey struct {
		storage  *storageadapter.Adapter
		enduser  *bootstrap.TestServer
		admin    *bootstrap.TestServer
		provider *helpers.MockCIMDUpstream
		proxy    *helpers.MockUpstreamOAuth2Server
		service  *model.ThirdpartyOAuth2ProviderEntity
	}

	principal := fixtures.DefaultPrincipal().String()
	adminPrincipal := fixtures.AdminPrincipal().String()
	logger := bootstrap.TestLogger(slog.LevelWarn)

	start := func(mode string, seededAtStartup bool) *journey {
		j := &journey{}
		storageFactory := bootstrap.NewStorageFactory(logger)
		var err error
		j.storage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(storageFactory.CloseStorage(j.storage)).To(Succeed()) })

		j.service = fixtures.CIMDPublicRegisteredService()
		if seededAtStartup {
			Expect(j.storage.Services().Create(context.Background(), j.service)).To(Succeed())
		}

		var config *ports.Config
		switch mode {
		case "local":
			config = fixtures.CIMDLocalConfig()
		case "proxy", "hybrid":
			j.proxy = helpers.NewMockUpstreamOAuth2Server()
			DeferCleanup(j.proxy.Close)
			if mode == "proxy" {
				config = fixtures.CIMDProxyConfig(j.proxy.URL())
			} else {
				config = fixtures.CIMDHybridConfig(j.proxy.URL())
			}
		default:
			Fail("unsupported OAuth server mode")
		}

		var brokerHTTPClient *http.Client
		j.enduser, brokerHTTPClient, err = bootstrap.NewCIMDClientEndUserTestServer(
			j.storage, bootstrap.NewServerFactory(config, logger), fixtures.CIMDEndUserPublicURL, logger,
		)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(j.enduser.Close)
		j.admin, err = bootstrap.NewAdminTestServer(j.enduser.App(), logger)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(j.admin.Close)

		j.provider = helpers.NewMockCIMDUpstream(
			helpers.WithCIMDUpstreamHTTPClient(brokerHTTPClient), helpers.WithPublicClientMode(),
		)
		DeferCleanup(j.provider.Close)
		j.service.Endpoints.AuthorizeEndpoint = j.provider.AuthorizationEndpoint()
		j.service.Endpoints.TokenEndpoint = j.provider.TokenEndpoint()
		if seededAtStartup {
			Expect(j.storage.Services().Update(context.Background(), j.service, nil)).To(Succeed())
		} else {
			Expect(j.storage.Services().Create(context.Background(), j.service)).To(Succeed())
		}
		return j
	}

	initiate := func(j *journey) *url.URL {
		response, err := j.enduser.AuthenticatedGET(
			"/api/third-party/"+j.service.ID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/done"),
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

	connect := func(j *journey) *http.Response {
		providerClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		authorizationURL := initiate(j)
		Expect(authorizationURL.Query().Get("client_id")).To(Equal(fixtures.CIMDClientIDURL(j.service.ID)))
		response, err := providerClient.Get(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location := response.Header.Get("Location")
		Expect(response.Body.Close()).To(Succeed())
		callbackURL, err := url.Parse(location)
		Expect(err).NotTo(HaveOccurred())
		response, err = j.enduser.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	storedSession := func(j *journey) *domainstorage.UserSession {
		session, err := j.storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(principal), j.service.ID)
		Expect(err).NotTo(HaveOccurred())
		return session
	}

	expireSession := func(j *journey) {
		session := storedSession(j)
		Expect(session).NotTo(BeNil())
		expired := time.Now().Add(-time.Minute)
		session.AccessTokenExpiresAt = &expired
		Expect(j.storage.UserSessions().Create(context.Background(), session)).To(Succeed())
	}

	refresh := func(j *journey) *http.Response {
		response, err := j.enduser.AuthenticatedPOST(
			"/api/third-party/"+j.service.ID.String()+"/session/refresh", principal, "application/json", nil,
		)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	listKeys := func(j *journey) {
		response, err := j.admin.AuthenticatedGET("/api/cimd-client-keys", adminPrincipal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[cimdKeyList](response).Items).To(BeEmpty())
	}

	assertPublicRequest := func(request helpers.CIMDTokenRequestObservation, grantType string) {
		Expect(request.GrantType).To(Equal(grantType))
		Expect(request.ClientIDValidated).To(BeTrue())
		Expect(request.ClientAssertionCount).To(BeZero())
		Expect(request.ClientAssertionTypeCount).To(BeZero())
		Expect(request.SecretAuthenticationReject).To(BeFalse())
		Expect(request.AssertionValidated).To(BeFalse())
	}

	Context("when the broker uses a keyless public registration", func() {
		var j *journey
		BeforeEach(func() { j = start("local", false) })

		// US8-S1 from specs/046-cimd-upstream-client/spec.md
		It("should publish exactly five public metadata members with cache headers", Label("cimd-upstream-client", "cimd-public-client"), func() {
			response, err := j.enduser.PublicGET("/.well-known/oauth-client/" + j.service.ID.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(response.Header.Get("Cache-Control")).To(Equal("public, max-age=300"))
			Expect(decodeJSON[map[string]any](response)).To(Equal(map[string]any{
				"client_id":                  j.service.ClientID.String(),
				"redirect_uris":              []any{fixtures.CIMDCallbackURL(j.service.ID)},
				"grant_types":                []any{"authorization_code", "refresh_token"},
				"response_types":             []any{"code"},
				"token_endpoint_auth_method": "none",
			}))
		})

		// US8-S2 from specs/046-cimd-upstream-client/spec.md
		It("should serve metadata but not keys or a public JWKS", Label("cimd-upstream-client", "cimd-public-client"), func() {
			listKeys(j)
			metadata, err := j.enduser.PublicGET("/.well-known/oauth-client/" + j.service.ID.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[map[string]any](metadata)).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))

			jwks, err := j.enduser.PublicGET("/.well-known/oauth-client/" + j.service.ID.String() + "/jwks.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(jwks.StatusCode).To(Equal(http.StatusNotFound))
			Expect(jwks.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(decodeJSON[map[string]any](jwks)).To(Equal(map[string]any{
				"error": "not found", "message": "CIMD service unavailable",
			}))
			Expect(j.provider.Observations().JWKSFetchCount).To(BeZero())
			listKeys(j)
		})

		// US8-S3 from specs/046-cimd-upstream-client/spec.md
		It("should authorize with the hosted client identity and S256 PKCE", Label("cimd-upstream-client", "cimd-public-client"), func() {
			authorizationURL := initiate(j)
			Expect(authorizationURL.Scheme + "://" + authorizationURL.Host + authorizationURL.Path).To(Equal(j.provider.AuthorizationEndpoint()))
			Expect(authorizationURL.Query().Get("client_id")).To(Equal(fixtures.CIMDClientIDURL(j.service.ID)))
			Expect(authorizationURL.Query().Get("redirect_uri")).To(Equal(fixtures.CIMDCallbackURL(j.service.ID)))
			Expect(authorizationURL.Query().Get("code_challenge_method")).To(Equal("S256"))
			Expect(authorizationURL.Query()).To(HaveKey("code_challenge"))
			Expect(authorizationURL.Query()).To(HaveKey("state"))

			providerClient := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
			response, err := providerClient.Get(authorizationURL.String())
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			observations := j.provider.Observations()
			Expect(observations.AuthorizationRequests).To(HaveLen(1))
			Expect(observations.AuthorizationRequests[0].MetadataValidated).To(BeTrue())
			Expect(observations.AuthorizationRequests[0].PKCEValidated).To(BeTrue())
			Expect(observations.JWKSFetchCount).To(BeZero())
		})

		// US8-S4 from specs/046-cimd-upstream-client/spec.md
		It("should exchange a code without credentials and store an encrypted session", Label("cimd-upstream-client", "cimd-public-client"), func() {
			response := connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			location, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(location.Query().Get("success")).To(Equal("true"))
			Expect(response.Body.Close()).To(Succeed())

			observations := j.provider.Observations()
			Expect(observations.TokenRequests).To(HaveLen(1))
			assertPublicRequest(observations.TokenRequests[0], "authorization_code")
			Expect(observations.TokenRequests[0].PKCEValidated).To(BeTrue())
			Expect(observations.JWKSFetchCount).To(BeZero())
			session := storedSession(j)
			Expect(session).NotTo(BeNil())
			Expect(session.EncryptionContext.ServiceID).To(Equal(j.service.ID))
			Expect(len(session.EncryptedAccessToken)).To(BeNumerically(">", 0))
			Expect(len(session.EncryptedRefreshToken)).To(BeNumerically(">", 0))
		})

		// US8-S5 from specs/046-cimd-upstream-client/spec.md
		It("should refresh without credentials and persist encrypted replacement tokens", Label("cimd-upstream-client", "cimd-public-client"), func() {
			response := connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			before := storedSession(j)
			Expect(before).NotTo(BeNil())
			accessBefore := append([]byte(nil), before.EncryptedAccessToken...)
			refreshBefore := append([]byte(nil), before.EncryptedRefreshToken...)
			expireSession(j)

			response = refresh(j)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			observations := j.provider.Observations()
			Expect(observations.TokenRequests).To(HaveLen(2))
			assertPublicRequest(observations.TokenRequests[1], "refresh_token")
			Expect(observations.JWKSFetchCount).To(BeZero())
			after := storedSession(j)
			Expect(after).NotTo(BeNil())
			Expect(after.EncryptionContext.ServiceID).To(Equal(j.service.ID))
			Expect(len(after.EncryptedAccessToken)).To(BeNumerically(">", 0))
			Expect(len(after.EncryptedRefreshToken)).To(BeNumerically(">", 0))
			Expect(bytes.Equal(after.EncryptedAccessToken, accessBefore)).To(BeFalse())
			Expect(bytes.Equal(after.EncryptedRefreshToken, refreshBefore)).To(BeFalse())
		})

		// US8-S6 from specs/046-cimd-upstream-client/spec.md
		It("should fail closed on exchange and refresh rejection without storing tokens", Label("cimd-upstream-client", "cimd-public-client"), func() {
			j.provider.WithTokenError("invalid_client")
			response := connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			failure, err := url.Parse(response.Header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Body.Close()).To(Succeed())
			Expect(failure.Path).To(Equal("/sessions"))
			Expect(failure.Query().Get("error")).To(Equal("callback_failed"))
			Expect(j.provider.Observations().TokenRequests).To(HaveLen(1))
			assertPublicRequest(j.provider.Observations().TokenRequests[0], "authorization_code")
			Expect(storedSession(j)).To(BeNil())

			j.provider.WithTokenError("")
			response = connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			before := storedSession(j)
			Expect(before).NotTo(BeNil())
			accessBefore := append([]byte(nil), before.EncryptedAccessToken...)
			refreshBefore := append([]byte(nil), before.EncryptedRefreshToken...)
			expireSession(j)
			j.provider.WithTokenError("invalid_grant")
			response = refresh(j)
			Expect(response.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(response.Body.Close()).To(Succeed())
			observations := j.provider.Observations()
			Expect(observations.TokenRequests).To(HaveLen(3))
			assertPublicRequest(observations.TokenRequests[2], "refresh_token")
			after := storedSession(j)
			Expect(after).NotTo(BeNil())
			Expect(bytes.Equal(after.EncryptedAccessToken, accessBefore)).To(BeTrue())
			Expect(bytes.Equal(after.EncryptedRefreshToken, refreshBefore)).To(BeTrue())
		})
	})

	// US8-S7 from specs/046-cimd-upstream-client/spec.md
	It("should connect and refresh with hosted identity in all server modes", Label("cimd-upstream-client", "cimd-public-client"), func() {
		for _, mode := range []string{"proxy", "local", "hybrid"} {
			j := start(mode, false)
			response := connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			expireSession(j)
			response = refresh(j)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			Expect(storedSession(j)).NotTo(BeNil())
			observations := j.provider.Observations()
			Expect(observations.AuthorizationRequests).To(HaveLen(1))
			Expect(observations.AuthorizationRequests[0].MetadataValidated).To(BeTrue())
			Expect(observations.TokenRequests).To(HaveLen(2))
			assertPublicRequest(observations.TokenRequests[0], "authorization_code")
			assertPublicRequest(observations.TokenRequests[1], "refresh_token")
			Expect(observations.JWKSFetchCount).To(BeZero())
			if j.proxy != nil {
				Expect(j.proxy.GetAuthorizeCalled()).To(BeFalse())
				Expect(j.proxy.GetTokenRequests()).To(BeEmpty())
			}
		}
	})

	Context("when only CIMD public services exist at startup", func() {
		var j *journey
		BeforeEach(func() { j = start("proxy", true) })

		// US8-S8 from specs/046-cimd-upstream-client/spec.md
		It("should start and complete a keyless connect and refresh", Label("cimd-upstream-client", "cimd-public-client"), func() {
			listKeys(j)
			response := connect(j)
			Expect(response.StatusCode).To(Equal(http.StatusFound))
			Expect(response.Body.Close()).To(Succeed())
			before := storedSession(j)
			Expect(before).NotTo(BeNil())
			accessBefore := append([]byte(nil), before.EncryptedAccessToken...)
			refreshBefore := append([]byte(nil), before.EncryptedRefreshToken...)
			expireSession(j)
			response = refresh(j)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			session := storedSession(j)
			Expect(session).NotTo(BeNil())
			Expect(len(session.EncryptedAccessToken)).To(BeNumerically(">", 0))
			Expect(len(session.EncryptedRefreshToken)).To(BeNumerically(">", 0))
			Expect(bytes.Equal(session.EncryptedAccessToken, accessBefore)).To(BeFalse())
			Expect(bytes.Equal(session.EncryptedRefreshToken, refreshBefore)).To(BeFalse())
			observations := j.provider.Observations()
			Expect(observations.TokenRequests).To(HaveLen(2))
			assertPublicRequest(observations.TokenRequests[0], "authorization_code")
			assertPublicRequest(observations.TokenRequests[1], "refresh_token")
			Expect(observations.JWKSFetchCount).To(BeZero())
			Expect(j.proxy.GetAuthorizeCalled()).To(BeFalse())
			Expect(j.proxy.GetTokenRequests()).To(BeEmpty())
			listKeys(j)
		})
	})
})
