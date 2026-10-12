package e2e_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// jwtPreauthAPIResponse represents the /api/me response for JWT pre-auth tests.
type jwtPreauthAPIResponse struct {
	Data struct {
		Principal   string  `json:"principal"`
		DisplayName string  `json:"displayName"`
		PictureURL  *string `json:"pictureUrl,omitempty"`
		Email       *string `json:"email,omitempty"`
	} `json:"data"`
}

var _ = Describe("JWT Pre-Authentication", func() {
	var (
		serverFactory  *bootstrap.ServerFactory
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		testStorage    *storageadapter.Adapter
		enduserServer  *bootstrap.TestServer
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		jwksServer     *helpers.MockJWKSServer
		config         *ports.Config
		principal      string
	)

	// ============================================================================
	// User Story 1: Accept Signed JWTs for Pre-Authentication (Priority: P1)
	// ============================================================================
	Describe("Signed JWT Authentication (US1)", func() {

		BeforeEach(func() {
			logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			}))

			jwksServer = helpers.NewMockJWKSServer()
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()

			config = fixtures.SignedJWTConfig(jwksServer.JWKSURL())
			config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
			config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
			config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

			storageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())

			serverFactory = bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).NotTo(HaveOccurred())

			enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())

			principal = fixtures.DefaultPrincipal().String()
		})

		AfterEach(func() {
			if enduserServer != nil {
				enduserServer.Close()
			}
			if mockUpstream != nil {
				mockUpstream.Close()
			}
			if jwksServer != nil {
				jwksServer.Close()
			}
			if storageFactory != nil && testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// Scenario 1.1 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should extract principal from valid signed JWT via CEL expression", func() {
			claims := helpers.NewJWTClaims().
				WithSubject(principal).
				WithName("Test User").
				WithEmail("user@example.com").
				WithPicture("https://example.com/avatar.jpg").
				Build()

			signedJWT, err := jwksServer.SignJWT(claims)
			Expect(err).NotTo(HaveOccurred())

			resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
				"Authorization": "Bearer " + signedJWT,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			var apiResp jwtPreauthAPIResponse
			err = json.Unmarshal(body, &apiResp)
			Expect(err).NotTo(HaveOccurred())
			Expect(apiResp.Data.Principal).To(Equal(principal))
		})

		// Scenario 1.2 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should reject JWT with invalid signature with 401", func() {
			differentPrivateKey, _, err := helpers.GenerateTestRSAKeyPair()
			Expect(err).NotTo(HaveOccurred())

			claims := helpers.NewJWTClaims().
				WithSubject(principal).
				Build()

			invalidJWT, err := helpers.SignTestJWT(claims, differentPrivateKey)
			Expect(err).NotTo(HaveOccurred())

			resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
				"Authorization": "Bearer " + invalidJWT,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario 1.3 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should reject expired JWT with 401", func() {
			claims := helpers.NewJWTClaims().
				WithSubject(principal).
				WithExpired().
				Build()

			expiredJWT, err := jwksServer.SignJWT(claims)
			Expect(err).NotTo(HaveOccurred())

			resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
				"Authorization": "Bearer " + expiredJWT,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario 1.4 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should reject request without JWT header on protected route with 401", func() {
			resp, err := enduserServer.PublicGET("/api/me")
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario 1.5 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should reject JWT with wrong audience with 401", func() {
			claims := helpers.NewJWTClaims().
				WithSubject(principal).
				WithAudience("wrong-audience").
				Build()

			wrongAudJWT, err := jwksServer.SignJWT(claims)
			Expect(err).NotTo(HaveOccurred())

			resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
				"Authorization": "Bearer " + wrongAudJWT,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		// Scenario 1.6 from specs/016-jwt-preauth/spec.md (User Story 1)
		It("should reject JWT with wrong issuer with 401", func() {
			claims := helpers.NewJWTClaims().
				WithSubject(principal).
				WithIssuer("https://wrong-issuer.example.com").
				Build()

			wrongIssJWT, err := jwksServer.SignJWT(claims)
			Expect(err).NotTo(HaveOccurred())

			resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
				"Authorization": "Bearer " + wrongIssJWT,
			}, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	// ============================================================================
	// User Story 2: Accept Unsigned (Pre-Authenticated) JWTs (Priority: P2)
	// ============================================================================
	Describe("Unsigned JWT Authentication (US2)", func() {

		Context("when verification is none", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.UnsignedJWTConfig()
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
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

			// Scenario 2.1 from specs/016-jwt-preauth/spec.md (User Story 2)
			It("should accept unsigned JWT when verification is none", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					WithClaim("preferred_username", "testuser").
					WithEmail("user@example.com").
					Build()

				unsignedJWT, err := helpers.CreateUnsignedJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"X-JWT-Claims": unsignedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(apiResp.Data.Principal).To(Equal(principal))
			})

			// Scenario 2.3 from specs/016-jwt-preauth/spec.md (User Story 2)
			It("should accept signed JWT without checking signature when verification is none", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					Build()

				privateKey, _, err := helpers.GenerateTestRSAKeyPair()
				Expect(err).NotTo(HaveOccurred())

				signedJWT, err := helpers.SignTestJWT(claims, privateKey)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"X-JWT-Claims": signedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			// Scenario: JWT without exp accepted when verification is none
			It("should accept JWT without exp claim when verification is none", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					WithoutExpiry().
					Build()

				tokenWithoutExp, err := helpers.CreateUnsignedJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"X-JWT-Claims": tokenWithoutExp,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})

			// Scenario 2.4 from specs/016-jwt-preauth/spec.md (User Story 2)
			It("should reject expired JWT even when verification is none", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					WithExpired().
					Build()

				expiredJWT, err := helpers.CreateUnsignedJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"X-JWT-Claims": expiredJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("when verification is jwks (default)", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				jwksServer = helpers.NewMockJWKSServer()
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.SignedJWTConfig(jwksServer.JWKSURL())
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
				if jwksServer != nil {
					jwksServer.Close()
				}
				if storageFactory != nil && testStorage != nil {
					_ = storageFactory.CloseStorage(testStorage)
				}
			})

			// Scenario 2.2 from specs/016-jwt-preauth/spec.md (User Story 2)
			It("should reject unsigned JWT when verification is jwks", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					Build()

				unsignedJWT, err := helpers.CreateUnsignedJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"Authorization": "Bearer " + unsignedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("when verification none and jwks_uri both present", func() {
			// Scenario 2.5 from specs/016-jwt-preauth/spec.md (User Story 2)
			It("should fail startup when verification none and jwks_uri both present", func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				mockUpstream = helpers.NewMockUpstreamOAuth2Server()
				defer mockUpstream.Close()

				invalidConfig := fixtures.MutuallyExclusiveJWTConfig("http://localhost:9999/.well-known/jwks.json")
				invalidConfig.Security.SkipThirdpartyHTTPSValidation = true
				invalidConfig.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				invalidConfig.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				invalidConfig.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				testStorage, err := storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = storageFactory.CloseStorage(testStorage) }()

				serverFactory = bootstrap.NewServerFactory(invalidConfig, logger)
				_, err = serverFactory.BuildApp(testStorage)

				Expect(err).To(HaveOccurred(), "Expected startup to fail with mutually exclusive JWT config")
			})
		})
	})

	// ============================================================================
	// User Story 3: Extract Profile Attributes from JWT Using CEL (Priority: P2)
	// ============================================================================
	Describe("Profile Extraction via CEL (US3)", func() {

		Context("when all profile CEL expressions are configured", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				jwksServer = helpers.NewMockJWKSServer()
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.SignedJWTConfig(jwksServer.JWKSURL())
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
				if jwksServer != nil {
					jwksServer.Close()
				}
				if storageFactory != nil && testStorage != nil {
					_ = storageFactory.CloseStorage(testStorage)
				}
			})

			// Scenario 3.1 from specs/016-jwt-preauth/spec.md (User Story 3)
			It("should return display name, email, and picture URL from JWT claims in /api/me", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					WithName("Jane Doe").
					WithEmail("jane@example.com").
					WithPicture("https://example.com/jane.jpg").
					Build()

				signedJWT, err := jwksServer.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"Authorization": "Bearer " + signedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(principal))
				Expect(apiResp.Data.DisplayName).To(Equal("Jane Doe"))
				Expect(apiResp.Data.Email).NotTo(BeNil())
				Expect(*apiResp.Data.Email).To(Equal("jane@example.com"))
				Expect(apiResp.Data.PictureURL).NotTo(BeNil())
				Expect(*apiResp.Data.PictureURL).To(Equal("https://example.com/jane.jpg"))
			})

			// Scenario 3.2 from specs/016-jwt-preauth/spec.md (User Story 3)
			It("should omit missing profile attributes from /api/me response", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					Build()

				signedJWT, err := jwksServer.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"Authorization": "Bearer " + signedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(principal))
				// DisplayName should fall back to principal when no name expression matches
				Expect(apiResp.Data.DisplayName).To(Equal(principal))
				Expect(apiResp.Data.Email).To(BeNil())
				Expect(apiResp.Data.PictureURL).To(BeNil())
			})
		})

		Context("when no profile expressions are configured", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				jwksServer = helpers.NewMockJWKSServer()
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.SignedJWTConfigMinimal(jwksServer.JWKSURL())
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
				if jwksServer != nil {
					jwksServer.Close()
				}
				if storageFactory != nil && testStorage != nil {
					_ = storageFactory.CloseStorage(testStorage)
				}
			})

			// Scenario 3.3 from specs/016-jwt-preauth/spec.md (User Story 3)
			It("should derive display name from principal when no display name expression configured", func() {
				claims := helpers.NewJWTClaims().
					WithSubject(principal).
					WithName("This Name Should Not Appear").
					Build()

				signedJWT, err := jwksServer.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "", map[string]string{
					"Authorization": "Bearer " + signedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(principal))
				Expect(apiResp.Data.DisplayName).To(Equal(principal))
				Expect(apiResp.Data.Email).To(BeNil())
				Expect(apiResp.Data.PictureURL).To(BeNil())
			})
		})

		Context("when using plain header preauth (no JWT)", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.NoJWTConfig()
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
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

			// Scenario 3.4 from specs/016-jwt-preauth/spec.md (User Story 3)
			It("should return principal-only profile for plain header preauth", func() {
				resp, err := enduserServer.AuthenticatedGET("/api/me", principal)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(principal))
				Expect(apiResp.Data.DisplayName).To(Equal(principal))
				Expect(apiResp.Data.Email).To(BeNil())
				Expect(apiResp.Data.PictureURL).To(BeNil())
			})
		})
	})

	// ============================================================================
	// User Story 5: Backward-Compatible Configuration (Priority: P1)
	// ============================================================================
	Describe("Backward-Compatible Configuration (US5)", func() {

		Context("when only plain header preauth is configured", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.NoJWTConfig()
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
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

			// Scenario 5.1 from specs/016-jwt-preauth/spec.md (User Story 5)
			It("should work with plain header preauth only config (no JWT block)", func() {
				resp, err := enduserServer.AuthenticatedGET("/api/me", principal)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(principal))
			})
		})

		Context("when both JWT and plain header are configured", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				jwksServer = helpers.NewMockJWKSServer()
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.SignedJWTConfig(jwksServer.JWKSURL())
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())

				principal = fixtures.DefaultPrincipal().String()
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
				if jwksServer != nil {
					jwksServer.Close()
				}
				if storageFactory != nil && testStorage != nil {
					_ = storageFactory.CloseStorage(testStorage)
				}
			})

			// Scenario 5.2 from specs/016-jwt-preauth/spec.md (User Story 5)
			It("should prefer JWT over plain header when both configured and JWT present", func() {
				jwtPrincipal := "jwt-user@example.com"
				claims := helpers.NewJWTClaims().
					WithSubject(jwtPrincipal).
					Build()

				signedJWT, err := jwksServer.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "plain-header-user@example.com", map[string]string{
					"Authorization": "Bearer " + signedJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())

				var apiResp jwtPreauthAPIResponse
				err = json.Unmarshal(body, &apiResp)
				Expect(err).NotTo(HaveOccurred())

				Expect(apiResp.Data.Principal).To(Equal(jwtPrincipal))
			})

			// Scenario 5.3 from specs/016-jwt-preauth/spec.md (User Story 5)
			It("should reject with 401 when JWT is configured but JWT header is absent", func() {
				plainPrincipal := "plain-user@example.com"

				resp, err := enduserServer.AuthenticatedGET("/api/me", plainPrincipal)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	// ============================================================================
	// Edge Cases
	// ============================================================================
	Describe("Edge Cases", func() {

		Context("when JWT present but invalid and plain header also present", func() {
			BeforeEach(func() {
				logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				}))

				jwksServer = helpers.NewMockJWKSServer()
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()

				config = fixtures.SignedJWTConfig(jwksServer.JWKSURL())
				config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = mockUpstream.URL()
				config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
				config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = mockUpstream.URL() + "/oauth/token"

				storageFactory = bootstrap.NewStorageFactory(logger)
				var err error
				testStorage, err = storageFactory.NewTestStorage()
				Expect(err).NotTo(HaveOccurred())

				serverFactory = bootstrap.NewServerFactory(config, logger)
				app, err := serverFactory.BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())

				enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
				if jwksServer != nil {
					jwksServer.Close()
				}
				if storageFactory != nil && testStorage != nil {
					_ = storageFactory.CloseStorage(testStorage)
				}
			})

			// Edge case from specs/016-jwt-preauth/spec.md
			// FR-013: System MUST NOT silently fall back to plain header when JWT is invalid
			It("should reject request when JWT present but invalid, not falling back to plain header", func() {
				claims := helpers.NewJWTClaims().
					WithSubject("jwt-user@example.com").
					WithExpired().
					Build()

				expiredJWT, err := jwksServer.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())

				resp, err := enduserServer.DirectRequest("GET", "/api/me", "valid-fallback@example.com", map[string]string{
					"Authorization": "Bearer " + expiredJWT,
				}, nil)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})
})
