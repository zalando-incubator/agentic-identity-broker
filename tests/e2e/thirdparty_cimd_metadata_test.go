package e2e_test

import (
	"context"
	"log/slog"
	"net/http"
	"time"

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

var _ = Describe("Third-Party CIMD Metadata", func() {
	var (
		logger           *slog.Logger
		storageFactory   *bootstrap.StorageFactory
		testStorage      *storageadapter.Adapter
		enduserServer    *bootstrap.TestServer
		brokerHTTPClient *http.Client
	)

	seedUsableCIMDKey := func() string {
		kid := seedCIMDClientAuthenticationKey(testStorage)
		_, err := testStorage.SigningKeys().SetCurrentInDomain(
			context.Background(),
			domainstorage.KeyDomainCIMDClientAuthentication,
			id.NewKeyID(kid),
			time.Now().UTC(),
		)
		Expect(err).NotTo(HaveOccurred())
		return kid
	}

	getBrokerDocument := func(url string) *http.Response {
		response, err := brokerHTTPClient.Get(url)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(fixtures.CIMDLocalConfig(), logger)
		enduserServer, brokerHTTPClient, err = bootstrap.NewCIMDClientEndUserTestServer(
			testStorage,
			serverFactory,
			fixtures.CIMDEndUserPublicURL,
			logger,
		)
		Expect(err).NotTo(HaveOccurred())
		brokerHTTPClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("when a CIMD confidential service has a usable published key", func() {
		var (
			cimdService *model.ThirdpartyOAuth2ProviderEntity
		)

		BeforeEach(func() {
			cimdService = fixtures.CIMDConfidentialService()
			Expect(testStorage.Services().Create(context.Background(), cimdService)).To(Succeed())
			seedUsableCIMDKey()
		})

		// US2-S1 from specs/046-cimd-upstream-client/spec.md
		It("should return a client ID equal to its URL", Label("cimd-upstream-client"), func() {
			clientIDURL := fixtures.CIMDClientIDURL(cimdService.ID)

			response := getBrokerDocument(clientIDURL)
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			document := decodeJSON[map[string]any](response)

			Expect(document).To(HaveKeyWithValue("client_id", clientIDURL))
		})

		// US2-S2 from specs/046-cimd-upstream-client/spec.md
		It("should publish the required callback and authentication fields", Label("cimd-upstream-client"), func() {
			response := getBrokerDocument(fixtures.CIMDClientIDURL(cimdService.ID))
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			document := decodeJSON[map[string]any](response)

			Expect(document).To(HaveLen(7))
			Expect(document).To(HaveKeyWithValue("redirect_uris", []any{fixtures.CIMDCallbackURL(cimdService.ID)}))
			Expect(document).To(HaveKeyWithValue("grant_types", []any{"authorization_code", "refresh_token"}))
			Expect(document).To(HaveKeyWithValue("response_types", []any{"code"}))
			Expect(document).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(document).To(HaveKeyWithValue("token_endpoint_auth_signing_alg", "ES256"))
			Expect(document).To(HaveKeyWithValue("jwks_uri", fixtures.CIMDJWKSURL(cimdService.ID)))
		})

		// US2-S3 from specs/046-cimd-upstream-client/spec.md
		It("should expose only public metadata and JWK material", Label("cimd-upstream-client"), func() {
			metadataResponse := getBrokerDocument(fixtures.CIMDClientIDURL(cimdService.ID))
			Expect(metadataResponse.StatusCode).To(Equal(http.StatusOK))
			metadata := decodeJSON[map[string]any](metadataResponse)

			for _, field := range []string{
				"client_secret", "private_key", "private_key_encrypted", "authorization_code",
				"access_token", "refresh_token", "user_info",
			} {
				Expect(metadata).NotTo(HaveKey(field))
			}

			jwksResponse := getBrokerDocument(fixtures.CIMDJWKSURL(cimdService.ID))
			Expect(jwksResponse.StatusCode).To(Equal(http.StatusOK))
			jwks := decodeJSON[map[string]any](jwksResponse)
			keys, ok := jwks["keys"].([]any)
			Expect(ok).To(BeTrue())
			Expect(keys).NotTo(BeEmpty())
			for _, key := range keys {
				publicKey, ok := key.(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(publicKey).To(HaveKeyWithValue("kty", "EC"))
				Expect(publicKey).To(HaveKeyWithValue("crv", "P-256"))
				Expect(publicKey).To(HaveKeyWithValue("alg", "ES256"))
				Expect(publicKey).To(HaveKeyWithValue("use", "sig"))
				Expect(publicKey).To(HaveKey("kid"))
				Expect(publicKey).To(HaveKey("x"))
				Expect(publicKey).To(HaveKey("y"))
				for _, privateField := range []string{"d", "p", "q", "dp", "dq", "qi", "k"} {
					Expect(publicKey).NotTo(HaveKey(privateField))
				}
			}
		})

		// US2-S5 from specs/046-cimd-upstream-client/spec.md
		It("should return JSON documents with five-minute cache headers", Label("cimd-upstream-client"), func() {
			metadataResponse := getBrokerDocument(fixtures.CIMDClientIDURL(cimdService.ID))
			Expect(metadataResponse.StatusCode).To(Equal(http.StatusOK))
			Expect(metadataResponse.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(metadataResponse.Header.Get("Cache-Control")).To(Equal("public, max-age=300"))
			Expect(metadataResponse.Body.Close()).To(Succeed())

			jwksResponse := getBrokerDocument(fixtures.CIMDJWKSURL(cimdService.ID))
			Expect(jwksResponse.StatusCode).To(Equal(http.StatusOK))
			Expect(jwksResponse.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(jwksResponse.Header.Get("Cache-Control")).To(Equal("public, max-age=300"))
			Expect(jwksResponse.Body.Close()).To(Succeed())
		})
	})

	Context("when a service uses public or static authentication", func() {
		var (
			publicService *model.ThirdpartyOAuth2ProviderEntity
			staticService *model.ThirdpartyOAuth2ProviderEntity
		)

		BeforeEach(func() {
			publicService = fixtures.CIMDPublicService()
			staticService = fixtures.CIMDStaticConfidentialService()
			Expect(testStorage.Services().Create(context.Background(), publicService)).To(Succeed())
			Expect(testStorage.Services().Create(context.Background(), staticService)).To(Succeed())
		})

		// US2-S4 from specs/046-cimd-upstream-client/spec.md
		It("should not expose a CIMD identity for non-CIMD services", Label("cimd-upstream-client"), func() {
			for _, service := range []*model.ThirdpartyOAuth2ProviderEntity{publicService, staticService} {
				response := getBrokerDocument(fixtures.CIMDClientIDURL(service.ID))
				Expect(response.StatusCode).To(Equal(http.StatusNotFound))
				errorResponse := decodeJSON[map[string]any](response)

				Expect(errorResponse).To(HaveKey("error"))
				Expect(errorResponse).NotTo(HaveKey("client_id"))
				Expect(errorResponse).NotTo(HaveKey("jwks_uri"))
			}
		})
	})

	Context("when a requested CIMD document is unavailable", func() {
		var unavailableService *model.ThirdpartyOAuth2ProviderEntity

		BeforeEach(func() {
			deletedService := fixtures.CIMDConfidentialService()
			Expect(testStorage.Services().Create(context.Background(), deletedService)).To(Succeed())
			Expect(testStorage.Services().Delete(context.Background(), deletedService.ID)).To(Succeed())

			publicService := fixtures.CIMDPublicService()
			staticService := fixtures.CIMDStaticConfidentialService()
			Expect(testStorage.Services().Create(context.Background(), publicService)).To(Succeed())
			Expect(testStorage.Services().Create(context.Background(), staticService)).To(Succeed())

			unavailableService = fixtures.CIMDConfidentialService().Copy()
			unavailableService.ID = id.NewServiceID()
			unavailableService.ClientID = id.ClientID(fixtures.CIMDClientIDURL(unavailableService.ID))
			Expect(testStorage.Services().Create(context.Background(), unavailableService)).To(Succeed())
		})

		// US2-S6 from specs/046-cimd-upstream-client/spec.md
		It("should return canonical JSON 404 responses for unavailable services", Label("cimd-upstream-client"), func() {
			serviceIDs := []id.ServiceID{
				id.NewServiceID(),
				fixtures.CIMDConfidentialService().ID,
				fixtures.CIMDPublicService().ID,
				fixtures.CIMDStaticConfidentialService().ID,
				unavailableService.ID,
			}

			for _, serviceID := range serviceIDs {
				for _, url := range []string{fixtures.CIMDClientIDURL(serviceID), fixtures.CIMDJWKSURL(serviceID)} {
					response := getBrokerDocument(url)
					Expect(response.StatusCode).To(Equal(http.StatusNotFound))
					Expect(response.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
					errorResponse := decodeJSON[map[string]any](response)

					Expect(errorResponse).To(HaveKey("error"))
					Expect(errorResponse["error"]).To(BeAssignableToTypeOf(""))
					Expect(errorResponse["error"]).NotTo(BeEmpty())
					Expect(errorResponse).NotTo(HaveKey("client_id"))
					Expect(errorResponse).NotTo(HaveKey("jwks_uri"))
					Expect(errorResponse).NotTo(HaveKey("keys"))
				}
			}
		})
	})

	Context("when CIMD and token-signing key domains are provisioned", func() {
		var (
			cimdService *model.ThirdpartyOAuth2ProviderEntity
			cimdKID     string
			adminServer *bootstrap.TestServer
		)

		BeforeEach(func() {
			cimdService = fixtures.CIMDConfidentialService()
			Expect(testStorage.Services().Create(context.Background(), cimdService)).To(Succeed())
			cimdKID = seedUsableCIMDKey()

			var err error
			adminServer, err = bootstrap.NewAdminTestServer(enduserServer.App(), logger)
			Expect(err).NotTo(HaveOccurred())
			Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())
		})

		AfterEach(func() {
			if adminServer != nil {
				adminServer.Close()
			}
		})

		// US2-S7 from specs/046-cimd-upstream-client/spec.md
		It("should publish disjoint CIMD and token key identifiers", Label("cimd-upstream-client"), func() {
			cimdResponse := getBrokerDocument(fixtures.CIMDJWKSURL(cimdService.ID))
			Expect(cimdResponse.StatusCode).To(Equal(http.StatusOK))
			cimdJWKS := decodeJSON[map[string]any](cimdResponse)
			cimdKids := extractKids(cimdJWKS)
			Expect(cimdKids).To(ContainElement(cimdKID))

			tokenResponse, err := enduserServer.PublicGET("/oauth2/jwks.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenResponse.StatusCode).To(Equal(http.StatusOK))
			tokenJWKS := decodeJSON[map[string]any](tokenResponse)
			tokenKids := extractKids(tokenJWKS)
			Expect(tokenKids).NotTo(BeEmpty())

			for _, tokenKID := range tokenKids {
				Expect(cimdKids).NotTo(ContainElement(tokenKID))
			}
			Expect(tokenKids).NotTo(ContainElement(cimdKID))
		})
	})
})
