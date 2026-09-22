package e2e_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

var _ = Describe("CIMD client authentication for third-party OAuth2 services", func() {
	var (
		adminServer     *bootstrap.TestServer
		storageFactory  *bootstrap.StorageFactory
		testStorage     *storageadapter.Adapter
		logger          *slog.Logger
		principal       string
		config          *ports.Config
		provider        *httptest.Server
		providerTraffic atomic.Int32
	)

	serviceRequest := func() map[string]any {
		return map[string]any{
			"display_name":               "CIMD confidential service",
			"oauth2_flavor":              "standard",
			"token_endpoint_auth_method": "private_key_jwt",
			"issuer_uri":                 "https://cimd-provider.example.test",
			"discovery":                  map[string]any{"enable_discovery": false},
			"endpoints": map[string]any{
				"token_endpoint":     "https://cimd-provider.example.test/oauth/token",
				"authorize_endpoint": "https://cimd-provider.example.test/oauth/authorize",
			},
			"scopes": []map[string]any{{"scope_value": "profile", "description": "Read the signed-in profile"}},
		}
	}

	postService := func(request map[string]any) *http.Response {
		body, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())

		response, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	provisionCIMDKey := func() {
		response, err := adminServer.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(response.Body.Close()).To(Succeed())
	}

	createCIMDService := func() map[string]any {
		provisionCIMDKey()
		response := postService(serviceRequest())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[map[string]any](response)
	}

	readService := func(serviceID string) map[string]any {
		response, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		config = fixtures.CIMDLocalConfig()
		principal = fixtures.DefaultPrincipal().String()
		provider = nil
		providerTraffic.Store(0)
	})

	JustBeforeEach(func() {
		application, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if provider != nil {
			provider.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("when a usable CIMD key has been provisioned", func() {
		JustBeforeEach(func() {
			provisionCIMDKey()
		})

		// US1-S1 from specs/046-cimd-upstream-client/spec.md
		It("creates a secretless service with broker-hosted identity", Label("cimd-upstream-client"), func() {
			created := postService(serviceRequest())
			Expect(created.StatusCode).To(Equal(http.StatusCreated))
			service := decodeJSON[map[string]any](created)
			expectedClientID := fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + service["id"].(string)
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(service).To(HaveKeyWithValue("client_id", expectedClientID))
			Expect(service).NotTo(HaveKey("client_secret"))

			read := readService(service["id"].(string))
			Expect(read).To(HaveKeyWithValue("client_id", expectedClientID))
			Expect(read).NotTo(HaveKey("client_secret"))
		})
	})

	Context("when registration supplies contradictory caller credentials", func() {
		BeforeEach(func() {
			provider = httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				providerTraffic.Add(1)
			}))
			config.Security.SkipThirdpartyHTTPSValidation = true
		})

		// US1-S2 from specs/046-cimd-upstream-client/spec.md
		It("rejects caller credentials before provider discovery", Label("cimd-upstream-client"), func() {
			request := serviceRequest()
			request["token_endpoint_auth_method"] = "private_key_jwt"
			request["client_id"] = "caller-supplied-client-id"
			request["client_secret"] = "caller-supplied-secret"
			request["issuer_uri"] = provider.URL
			request["discovery"] = map[string]any{"enable_discovery": true, "metadata_url": provider.URL + "/.well-known/oauth-authorization-server"}
			delete(request, "endpoints")

			response := postService(request)
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			errorBody := decodeJSON[map[string]any](response)
			Expect(errorBody).To(HaveKeyWithValue("error", "validation failed"))
			Expect(errorBody["message"]).To(ContainSubstring("client_id"))
			Expect(providerTraffic.Load()).To(Equal(int32(0)))

			services, err := adminServer.AuthenticatedGET("/api/services", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(services.StatusCode).To(Equal(http.StatusOK))
			Expect(decodeJSON[[]map[string]any](services)).To(BeEmpty())
		})
	})

	// US1-S3 from specs/046-cimd-upstream-client/spec.md
	It("preserves static confidential registration", Label("cimd-upstream-client"), func() {
		request := serviceRequest()
		delete(request, "token_endpoint_auth_method")
		request["client_id"] = "static-confidential-client"
		request["client_secret"] = "static-confidential-secret"

		response := postService(request)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		service := decodeJSON[map[string]any](response)
		Expect(service).To(HaveKey("token_endpoint_auth_method"))
		Expect(service["token_endpoint_auth_method"]).To(BeNil())
		Expect(service).To(HaveKeyWithValue("client_secret", "REDACTED"))
	})

	// US1-S4 from specs/046-cimd-upstream-client/spec.md
	It("preserves public registration", Label("cimd-upstream-client"), func() {
		request := serviceRequest()
		request["client_id"] = "public-client"
		request["token_endpoint_auth_method"] = "none"

		response := postService(request)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		service := decodeJSON[map[string]any](response)
		Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
		Expect(service).NotTo(HaveKey("client_secret"))
	})

	// US1-S5 from specs/046-cimd-upstream-client/spec.md
	It("rejects private key JWT for the google flavor", Label("cimd-upstream-client"), func() {
		request := fixtures.ValidGoogleServiceRequest()
		request["token_endpoint_auth_method"] = "private_key_jwt"
		delete(request, "client_secret")

		response := postService(request)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		errorBody := decodeJSON[map[string]any](response)
		Expect(errorBody).To(HaveKeyWithValue("error", "validation failed"))
		Expect(errorBody["message"]).To(ContainSubstring("google"))
	})

	Context("when all authentication postures are registered", func() {
		var cimdService map[string]any

		JustBeforeEach(func() {
			staticRequest := serviceRequest()
			delete(staticRequest, "token_endpoint_auth_method")
			staticRequest["client_id"] = "static-posture-client"
			staticRequest["client_secret"] = "static-posture-secret"
			Expect(postService(staticRequest).StatusCode).To(Equal(http.StatusCreated))

			publicRequest := serviceRequest()
			publicRequest["client_id"] = "public-posture-client"
			publicRequest["token_endpoint_auth_method"] = "none"
			Expect(postService(publicRequest).StatusCode).To(Equal(http.StatusCreated))

			cimdService = createCIMDService()
		})

		// US5-S1 from specs/046-cimd-upstream-client/spec.md
		It("lists static, public, and CIMD authentication postures", Label("cimd-upstream-client"), func() {
			response, err := adminServer.AuthenticatedGET("/api/services", principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			services := decodeJSON[[]map[string]any](response)
			Expect(services).To(HaveLen(3))

			methods := make(map[any]map[string]any, len(services))
			for _, service := range services {
				methods[service["token_endpoint_auth_method"]] = service
			}
			Expect(methods).To(HaveKey(BeNil()))
			Expect(methods).To(HaveKey("none"))
			Expect(methods).To(HaveKey("private_key_jwt"))
			Expect(methods["private_key_jwt"]).To(HaveKeyWithValue("id", cimdService["id"]))
			Expect(methods["private_key_jwt"]).NotTo(HaveKey("client_secret"))
		})
	})

	Context("when reading a CIMD confidential service", func() {
		var cimdService map[string]any

		JustBeforeEach(func() {
			cimdService = createCIMDService()
		})

		// US5-S2 from specs/046-cimd-upstream-client/spec.md
		It("reads the immutable CIMD client ID without a shared secret", Label("cimd-upstream-client"), func() {
			service := readService(cimdService["id"].(string))
			Expect(service).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(service["client_id"]).To(Equal(fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + cimdService["id"].(string)))
			Expect(service).NotTo(HaveKey("client_secret"))
		})
	})

	Context("when replacing static confidential authentication with CIMD", func() {
		var updatedService map[string]any

		JustBeforeEach(func() {
			staticRequest := serviceRequest()
			delete(staticRequest, "token_endpoint_auth_method")
			staticRequest["client_id"] = "static-to-cimd-client"
			staticRequest["client_secret"] = "static-to-cimd-secret"
			created := postService(staticRequest)
			Expect(created.StatusCode).To(Equal(http.StatusCreated))
			staticService := decodeJSON[map[string]any](created)

			provisionCIMDKey()
			cimdRequest := serviceRequest()
			cimdRequest["token_endpoint_auth_method"] = "private_key_jwt"
			body, err := json.Marshal(cimdRequest)
			Expect(err).NotTo(HaveOccurred())
			response, err := adminServer.DirectRequest(http.MethodPut, "/api/services/"+staticService["id"].(string), principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			updatedService = decodeJSON[map[string]any](response)
		})

		// US5-S3 from specs/046-cimd-upstream-client/spec.md
		It("replaces static secret state with CIMD authentication", Label("cimd-upstream-client"), func() {
			Expect(updatedService).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
			Expect(updatedService).NotTo(HaveKey("client_secret"))
			Expect(updatedService["client_id"]).To(HavePrefix(fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/"))
		})
	})
})
