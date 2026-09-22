package e2e_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type cimdKeyMetadata struct {
	KID         string `json:"kid"`
	Algorithm   string `json:"algorithm"`
	IsCurrent   bool   `json:"is_current"`
	ActivatesAt string `json:"activates_at"`
	CreatedAt   string `json:"created_at"`
}

type cimdKeyList struct {
	Items []cimdKeyMetadata `json:"items"`
}

type cimdMetadataDocument struct {
	ClientID string `json:"client_id"`
	JWKSURI  string `json:"jwks_uri"`
}

type cimdPublicJWK struct {
	KID       string `json:"kid"`
	Algorithm string `json:"alg"`
}

type cimdPublicJWKS struct {
	Keys []cimdPublicJWK `json:"keys"`
}

var _ = Describe("CIMD client-authentication key rotation", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		adminPrincipal string
		principal      string
		service        = fixtures.CIMDConfidentialService()
		upstream       *helpers.MockCIMDUpstream
		initialKeyID   string
	)

	createCIMDKey := func(server *bootstrap.TestServer, algorithm string) cimdKeyMetadata {
		body := strings.NewReader(`{}`)
		if algorithm != "" {
			body = strings.NewReader(`{"algorithm":"` + algorithm + `"}`)
		}
		response, err := server.AuthenticatedPOST(
			"/api/cimd-client-keys",
			adminPrincipal,
			"application/json",
			body,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[cimdKeyMetadata](response)
	}

	listCIMDKeys := func(server *bootstrap.TestServer) []cimdKeyMetadata {
		response, err := server.AuthenticatedGET("/api/cimd-client-keys", adminPrincipal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[cimdKeyList](response).Items
	}

	listTokenSigningKeys := func(server *bootstrap.TestServer) []cimdKeyMetadata {
		response, err := server.AuthenticatedGET("/api/oauth2-server/signing-keys", adminPrincipal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[cimdKeyList](response).Items
	}

	promoteCIMDKey := func(server *bootstrap.TestServer, kid string) cimdKeyMetadata {
		response, err := server.DirectRequest(
			http.MethodPut,
			"/api/cimd-client-keys/"+url.PathEscape(kid)+"/current",
			adminPrincipal,
			nil,
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[cimdKeyMetadata](response)
	}

	readCIMDMetadata := func() cimdMetadataDocument {
		response, err := enduserServer.PublicGET("/.well-known/oauth-client/" + service.ID.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[cimdMetadataDocument](response)
	}

	readCIMDJWKS := func(jwksURI string) cimdPublicJWKS {
		parsed, err := url.Parse(jwksURI)
		Expect(err).NotTo(HaveOccurred())
		response, err := enduserServer.PublicGET(parsed.RequestURI())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[cimdPublicJWKS](response)
	}

	keyIDs := func(jwks cimdPublicJWKS) []string {
		kids := make([]string, 0, len(jwks.Keys))
		for _, key := range jwks.Keys {
			kids = append(kids, key.KID)
		}
		return kids
	}

	completeConnect := func() *http.Response {
		start, err := enduserServer.AuthenticatedGET(
			"/api/third-party/"+service.ID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(fixtures.CIMDEndUserPublicURL+"/done"),
			principal,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(start.StatusCode).To(Equal(http.StatusFound))
		authorizationURL, err := url.Parse(start.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(start.Body.Close()).To(Succeed())

		client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
		authorizationResponse, err := client.Get(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationResponse.StatusCode).To(Equal(http.StatusFound))
		callbackURL, err := url.Parse(authorizationResponse.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationResponse.Body.Close()).To(Succeed())

		response, err := enduserServer.AuthenticatedGET(callbackURL.RequestURI(), principal)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	refreshSession := func() *http.Response {
		response, err := enduserServer.AuthenticatedPOST(
			"/api/third-party/"+service.ID.String()+"/session/refresh",
			principal,
			"application/json",
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

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

	setupCIMDService := func(withUsableKey bool) {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(fixtures.CIMDLocalConfig(), logger)
		var brokerHTTPClient *http.Client
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
		if withUsableKey {
			initialKeyID = seedUsableCIMDKey()
		}

		adminServer, err = bootstrap.NewAdminTestServer(enduserServer.App(), logger)
		Expect(err).NotTo(HaveOccurred())
		adminPrincipal = fixtures.AdminPrincipal().String()
		principal = fixtures.DefaultPrincipal().String()
	}

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
		adminServer = nil
		enduserServer = nil
		upstream = nil
		testStorage = nil
		storageFactory = nil
	})

	Context("when rotating a usable CIMD key", func() {
		BeforeEach(func() {
			setupCIMDService(true)
		})

		// US4-S1 from specs/046-cimd-upstream-client/spec.md
		It("should keep the public key location stable before use", Label("cimd-upstream-client"), func() {
			metadataBefore := readCIMDMetadata()
			rotatedKey := createCIMDKey(adminServer, "ES256")
			metadataAfter := readCIMDMetadata()
			publicKeys := readCIMDJWKS(metadataAfter.JWKSURI)

			Expect(metadataAfter.JWKSURI).To(Equal(metadataBefore.JWKSURI))
			Expect(keyIDs(publicKeys)).To(ContainElement(rotatedKey.KID))
			Expect(upstream.Observations().TokenRequests).To(BeEmpty())

			promoteCIMDKey(adminServer, rotatedKey.KID)
			callback := completeConnect()
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())
			Expect(upstream.Observations().TokenRequests).To(ContainElement(
				HaveField("AssertionKeyID", rotatedKey.KID),
			))
		})

		// US4-S2 from specs/046-cimd-upstream-client/spec.md
		It("should refresh cached public keys for a newly active assertion", Label("cimd-upstream-client"), func() {
			Expect(upstream.RefreshClientMetadata(context.Background(), service.ClientID.String())).To(Succeed())
			Expect(upstream.Observations().CachedKeyIDs).To(ContainElement(initialKeyID))

			rotatedKey := createCIMDKey(adminServer, "ES256")
			promoteCIMDKey(adminServer, rotatedKey.KID)
			Expect(upstream.Observations().CachedKeyIDs).NotTo(ContainElement(rotatedKey.KID))

			Expect(upstream.RefreshClientMetadata(context.Background(), service.ClientID.String())).To(Succeed())
			Expect(upstream.Observations().CachedKeyIDs).To(ContainElement(rotatedKey.KID))
			callback := completeConnect()
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())
			Expect(upstream.Observations().TokenRequests).To(ContainElement(
				HaveField("AssertionKeyID", rotatedKey.KID),
			))
		})

		// US4-S4 from specs/046-cimd-upstream-client/spec.md
		It("should publish a grace key while the previous key signs", Label("cimd-upstream-client"), func() {
			metadata := readCIMDMetadata()
			previousKeyIDs := keyIDs(readCIMDJWKS(metadata.JWKSURI))
			pendingKey := createCIMDKey(adminServer, "ES256")
			publishedKeyIDs := keyIDs(readCIMDJWKS(metadata.JWKSURI))

			Expect(publishedKeyIDs).To(ContainElement(pendingKey.KID))
			callback := completeConnect()
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())
			observations := upstream.Observations()
			Expect(observations.TokenRequests).To(HaveLen(1))
			Expect(previousKeyIDs).To(ContainElement(observations.TokenRequests[0].AssertionKeyID))
		})

		// US4-S5 from specs/046-cimd-upstream-client/spec.md
		It("should sign with an immediately promoted key", Label("cimd-upstream-client"), func() {
			candidate := createCIMDKey(adminServer, "ES256")
			Expect(upstream.RefreshClientMetadata(context.Background(), service.ClientID.String())).To(Succeed())
			Expect(upstream.Observations().CachedKeyIDs).To(ContainElement(candidate.KID))

			promoted := promoteCIMDKey(adminServer, candidate.KID)
			Expect(promoted.ActivatesAt).ToNot(BeEmpty())
			callback := completeConnect()
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())
			Expect(upstream.Observations().TokenRequests).To(ContainElement(
				HaveField("AssertionKeyID", candidate.KID),
			))
		})
	})

	Context("when no usable CIMD key exists", func() {
		BeforeEach(func() {
			setupCIMDService(false)
			session := fixtures.SessionForService(principal, service.ID.String())
			Expect(testStorage.UserSessions().Create(context.Background(), session)).To(Succeed())
		})

		// US4-S3 from specs/046-cimd-upstream-client/spec.md
		It("should fail closed without sending a token request", Label("cimd-upstream-client"), func() {
			Expect(listCIMDKeys(adminServer)).To(BeEmpty())

			response := refreshSession()
			Expect(response.StatusCode).NotTo(Equal(http.StatusOK))
			Expect(response.Body.Close()).To(Succeed())
			Expect(upstream.Observations().TokenRequests).To(BeEmpty())
		})

		// US4-S6 from specs/046-cimd-upstream-client/spec.md
		It("should activate the first generated key immediately", Label("cimd-upstream-client"), func() {
			Expect(listCIMDKeys(adminServer)).To(BeEmpty())
			firstKey := createCIMDKey(adminServer, "ES256")
			metadata := readCIMDMetadata()

			Expect(keyIDs(readCIMDJWKS(metadata.JWKSURI))).To(ContainElement(firstKey.KID))
			callback := completeConnect()
			Expect(callback.StatusCode).To(Equal(http.StatusFound))
			Expect(callback.Body.Close()).To(Succeed())
			Expect(upstream.Observations().TokenRequests).To(ContainElement(
				HaveField("AssertionKeyID", firstKey.KID),
			))
		})
	})

	Context("when inspecting every OAuth server mode", func() {
		type modeServer struct {
			name           string
			adminServer    *bootstrap.TestServer
			storageFactory *bootstrap.StorageFactory
			testStorage    *storageadapter.Adapter
			upstream       *helpers.MockUpstreamOAuth2Server
		}

		var modeServers []*modeServer

		startModeServer := func(name string, config *ports.Config, modeUpstream *helpers.MockUpstreamOAuth2Server) *modeServer {
			modeLogger := bootstrap.TestLogger(slog.LevelWarn)
			modeStorageFactory := bootstrap.NewStorageFactory(modeLogger)
			modeStorage, err := modeStorageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			application, err := bootstrap.NewServerFactory(config, modeLogger).BuildApp(modeStorage)
			Expect(err).NotTo(HaveOccurred())
			modeAdminServer, err := bootstrap.NewAdminTestServer(application, modeLogger)
			Expect(err).NotTo(HaveOccurred())
			return &modeServer{
				name:           name,
				adminServer:    modeAdminServer,
				storageFactory: modeStorageFactory,
				testStorage:    modeStorage,
				upstream:       modeUpstream,
			}
		}

		BeforeEach(func() {
			adminPrincipal = fixtures.AdminPrincipal().String()
			proxyUpstream := helpers.NewMockUpstreamOAuth2Server()
			hybridUpstream := helpers.NewMockUpstreamOAuth2Server()
			modeServers = []*modeServer{
				startModeServer("proxy", fixtures.CIMDProxyConfig(proxyUpstream.URL()), proxyUpstream),
				startModeServer("local", fixtures.CIMDLocalConfig(), nil),
				startModeServer("hybrid", fixtures.CIMDHybridConfig(hybridUpstream.URL()), hybridUpstream),
			}
		})

		AfterEach(func() {
			for _, mode := range modeServers {
				if mode.adminServer != nil {
					mode.adminServer.Close()
				}
				if mode.upstream != nil {
					mode.upstream.Close()
				}
				if mode.storageFactory != nil && mode.testStorage != nil {
					_ = mode.storageFactory.CloseStorage(mode.testStorage)
				}
			}
		})

		// US4-S7 from specs/046-cimd-upstream-client/spec.md
		It("should isolate CIMD key routes in every server mode", Label("cimd-upstream-client"), func() {
			for _, mode := range modeServers {
				var tokenSigningKeysBefore []cimdKeyMetadata
				if mode.name != "proxy" {
					tokenSigningKeysBefore = listTokenSigningKeys(mode.adminServer)
				}
				firstKey := createCIMDKey(mode.adminServer, "ES256")
				Expect(listCIMDKeys(mode.adminServer)).To(ContainElement(
					HaveField("KID", firstKey.KID),
				))
				secondKey := createCIMDKey(mode.adminServer, "ES256")
				promoteCIMDKey(mode.adminServer, secondKey.KID)

				response, err := mode.adminServer.DirectRequest(
					http.MethodDelete,
					"/api/cimd-client-keys/"+url.PathEscape(firstKey.KID),
					adminPrincipal,
					nil,
					nil,
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusNoContent), mode.name)
				Expect(response.Body.Close()).To(Succeed())

				if mode.name == "proxy" {
					response, err = mode.adminServer.AuthenticatedGET(
						"/api/oauth2-server/signing-keys",
						adminPrincipal,
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(response.StatusCode).To(Equal(http.StatusNotFound))
					Expect(response.Body.Close()).To(Succeed())
				} else {
					Expect(listTokenSigningKeys(mode.adminServer)).To(Equal(tokenSigningKeysBefore))
				}
			}
		})
	})

	Context("when an operator requests a non-ES256 key", func() {
		BeforeEach(func() {
			setupCIMDService(true)
		})

		// US4-S8 from specs/046-cimd-upstream-client/spec.md
		It("should reject non-ES256 generation without publishing a key", Label("cimd-upstream-client"), func() {
			keysBefore := listCIMDKeys(adminServer)
			response, err := adminServer.AuthenticatedPOST(
				"/api/cimd-client-keys",
				adminPrincipal,
				"application/json",
				strings.NewReader(`{"algorithm":"RS256"}`),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorResponse := decodeJSON[map[string]any](response)
			Expect(errorResponse).To(HaveKey("error"))

			keysAfter := listCIMDKeys(adminServer)
			Expect(keysAfter).To(HaveLen(len(keysBefore)))
			for _, key := range keysAfter {
				Expect(key.Algorithm).To(Equal("ES256"))
			}
			metadata := readCIMDMetadata()
			for _, key := range readCIMDJWKS(metadata.JWKSURI).Keys {
				Expect(key.Algorithm).To(Equal("ES256"))
			}
		})
	})
})
