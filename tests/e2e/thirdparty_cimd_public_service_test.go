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

var _ = Describe("CIMD public third-party service registration", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
		principal      string
		provider       *httptest.Server
		traffic        atomic.Int32
		config         *ports.Config
	)

	publicRequest := func() map[string]any {
		return map[string]any{
			"display_name":               "CIMD public service",
			"oauth2_flavor":              "standard",
			"client_registration":        "cimd",
			"token_endpoint_auth_method": "none",
			"issuer_uri":                 "https://public-provider.example.test",
			"discovery":                  map[string]any{"enable_discovery": false},
			"endpoints": map[string]any{
				"authorize_endpoint": "https://public-provider.example.test/authorize",
				"token_endpoint":     "https://public-provider.example.test/token",
			},
			"scopes": []map[string]any{{"scope_value": "profile", "description": "Read profile"}},
		}
	}

	serviceJSON := func(method, path string, request map[string]any) *http.Response {
		GinkgoHelper()
		body, err := json.Marshal(request)
		Expect(err).NotTo(HaveOccurred())
		response, err := adminServer.DirectRequest(method, path, principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	readService := func(serviceID string) map[string]any {
		GinkgoHelper()
		response, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return decodeJSON[map[string]any](response)
	}

	keyList := func() []any {
		GinkgoHelper()
		response, err := adminServer.AuthenticatedGET("/api/cimd-client-keys", principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body := decodeJSON[map[string]any](response)
		Expect(body).To(HaveKey("items"))
		items, ok := body["items"].([]any)
		Expect(ok).To(BeTrue())
		return items
	}

	provisionKey := func() {
		GinkgoHelper()
		response, err := adminServer.AuthenticatedPOST("/api/cimd-client-keys", principal, "application/json", bytes.NewBufferString(`{"algorithm":"ES256"}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(response.Body.Close()).To(Succeed())
	}

	createPublic := func() map[string]any {
		GinkgoHelper()
		response := serviceJSON(http.MethodPost, "/api/services", publicRequest())
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		return decodeJSON[map[string]any](response)
	}

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelWarn)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		principal = fixtures.DefaultPrincipal().String()
		config = fixtures.CIMDLocalConfig()
		traffic.Store(0)
	})

	JustBeforeEach(func() {
		application, err := bootstrap.NewServerFactory(config, logger).BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(application, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.CloseListener()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if provider != nil {
			provider.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// US6-S1 from specs/20260917-120000-cimd-upstream-client/spec.md
	It("should assign a hosted secretless client identity on creation", Label("cimd-upstream-client", "cimd-public-client"), func() {
		created := createPublic()
		serviceID := created["id"].(string)
		clientID := fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + serviceID
		Expect(created).To(HaveKeyWithValue("client_registration", "cimd"))
		Expect(created).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
		Expect(created).To(HaveKeyWithValue("client_id", clientID))
		Expect(created).NotTo(HaveKey("client_secret"))
		stored := readService(serviceID)
		Expect(stored).To(HaveKeyWithValue("client_registration", "cimd"))
		Expect(stored).To(HaveKeyWithValue("client_id", clientID))
		Expect(stored).NotTo(HaveKey("client_secret"))
	})

	// US6-S2 from specs/20260917-120000-cimd-upstream-client/spec.md
	It("should allow public create and replacement without generating a key", Label("cimd-upstream-client", "cimd-public-client"), func() {
		Expect(keyList()).To(BeEmpty())
		created := createPublic()
		serviceID := created["id"].(string)
		replacement := publicRequest()
		replacement["display_name"] = "Replaced keyless CIMD public service"
		updated := serviceJSON(http.MethodPut, "/api/services/"+serviceID, replacement)
		Expect(updated.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[map[string]any](updated)).To(HaveKeyWithValue("client_id", created["client_id"]))
		Expect(keyList()).To(BeEmpty())

		confidential := publicRequest()
		confidential["token_endpoint_auth_method"] = "private_key_jwt"
		rejected := serviceJSON(http.MethodPost, "/api/services", confidential)
		Expect(rejected.StatusCode).To(BeNumerically(">=", http.StatusBadRequest))
		Expect(rejected.Body.Close()).To(Succeed())
		Expect(keyList()).To(BeEmpty())
		listed, err := adminServer.AuthenticatedGET("/api/services", principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(decodeJSON[[]map[string]any](listed)).To(HaveLen(1))
		Expect(readService(serviceID)).To(HaveKeyWithValue("display_name", "Replaced keyless CIMD public service"))
	})

	Context("when the provider exposes a discovery endpoint", func() {
		BeforeEach(func() {
			provider = httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				traffic.Add(1)
			}))
			config.Security.SkipThirdpartyHTTPSValidation = true
		})

		// US6-S3 from specs/20260917-120000-cimd-upstream-client/spec.md
		It("should reject supplied identifiers and secrets before provider traffic", Label("cimd-upstream-client", "cimd-public-client"), func() {
			created := createPublic()
			serviceID := created["id"].(string)
			for _, field := range []string{"client_id", "client_secret"} {
				request := publicRequest()
				request[field] = "caller-controlled-credential"
				request["issuer_uri"] = provider.URL
				request["discovery"] = map[string]any{"enable_discovery": true, "metadata_url": provider.URL + "/.well-known/oauth-authorization-server"}
				delete(request, "endpoints")
				for _, method := range []string{http.MethodPost, http.MethodPut} {
					path := "/api/services"
					if method == http.MethodPut {
						path += "/" + serviceID
					}
					response := serviceJSON(method, path, request)
					Expect(response.StatusCode).To(Equal(http.StatusBadRequest), field, method)
					body := decodeJSON[map[string]any](response)
					Expect(body).To(HaveKeyWithValue("error", "validation failed"))
					Expect(body["message"]).To(ContainSubstring(field))
					Expect(traffic.Load()).To(BeZero())
					Expect(readService(serviceID)).To(HaveKeyWithValue("client_id", created["client_id"]))
				}
			}
			Expect(readService(serviceID)).To(HaveKeyWithValue("display_name", created["display_name"]))
		})
	})

	// US6-S4 from specs/20260917-120000-cimd-upstream-client/spec.md
	It("should reject CIMD registration for Google and filesystem while allowing static", Label("cimd-upstream-client", "cimd-public-client"), func() {
		google := fixtures.ValidGoogleServiceRequest()
		google["client_registration"] = "cimd"
		response := serviceJSON(http.MethodPost, "/api/services", google)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		googleError := decodeJSON[map[string]any](response)
		Expect(googleError).To(HaveKeyWithValue("error", "validation failed"))
		Expect(googleError["message"]).To(ContainSubstring("google"))
		google["client_registration"] = "static"
		response = serviceJSON(http.MethodPost, "/api/services", google)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(decodeJSON[map[string]any](response)).To(HaveKeyWithValue("client_registration", "static"))

		filesystem := fixtures.CredentialServiceRequest("keyless-filesystem", "https://public-provider.example.test", "filesystem")
		filesystem["client_registration"] = "cimd"
		response = serviceJSON(http.MethodPost, "/api/services", filesystem)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(decodeJSON[map[string]any](response)["message"]).To(ContainSubstring("filesystem"))
		filesystem["client_registration"] = "static"
		response = serviceJSON(http.MethodPost, "/api/services", filesystem)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		Expect(decodeJSON[map[string]any](response)).To(HaveKeyWithValue("client_registration", "static"))
	})

	// US6-S5 from specs/20260917-120000-cimd-upstream-client/spec.md
	It("should retain its URL across authentication-method switches and require a confidential key", Label("cimd-upstream-client", "cimd-public-client"), func() {
		created := createPublic()
		serviceID := created["id"].(string)
		confidential := publicRequest()
		confidential["token_endpoint_auth_method"] = "private_key_jwt"
		rejected := serviceJSON(http.MethodPut, "/api/services/"+serviceID, confidential)
		Expect(rejected.StatusCode).To(BeNumerically(">=", http.StatusBadRequest))
		Expect(rejected.Body.Close()).To(Succeed())
		Expect(readService(serviceID)).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
		provisionKey()
		response := serviceJSON(http.MethodPut, "/api/services/"+serviceID, confidential)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		updated := decodeJSON[map[string]any](response)
		Expect(updated).To(HaveKeyWithValue("client_id", created["client_id"]))
		Expect(updated).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
		Expect(updated).NotTo(HaveKey("client_secret"))
		metadata, err := enduserServer.PublicGET("/.well-known/oauth-client/" + serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[map[string]any](metadata)).To(HaveKeyWithValue("token_endpoint_auth_method", "private_key_jwt"))
		response = serviceJSON(http.MethodPut, "/api/services/"+serviceID, publicRequest())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		updated = decodeJSON[map[string]any](response)
		Expect(updated).To(HaveKeyWithValue("client_id", created["client_id"]))
		Expect(updated).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
		Expect(updated).NotTo(HaveKey("client_secret"))
		metadata, err = enduserServer.PublicGET("/.well-known/oauth-client/" + serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(metadata.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[map[string]any](metadata)).To(HaveKeyWithValue("token_endpoint_auth_method", "none"))
	})

	// US6-S6 from specs/20260917-120000-cimd-upstream-client/spec.md
	It("should change identity only after validating static and CIMD replacements", Label("cimd-upstream-client", "cimd-public-client"), func() {
		static := publicRequest()
		static["client_registration"] = "static"
		delete(static, "token_endpoint_auth_method")
		static["client_id"] = "operator-confidential-client"
		static["client_secret"] = "operator-confidential-secret"
		response := serviceJSON(http.MethodPost, "/api/services", static)
		Expect(response.StatusCode).To(Equal(http.StatusCreated))
		created := decodeJSON[map[string]any](response)
		serviceID := created["id"].(string)
		Expect(created).To(HaveKeyWithValue("client_secret", "REDACTED"))

		invalid := publicRequest()
		invalid["client_secret"] = "must-not-be-retained"
		response = serviceJSON(http.MethodPut, "/api/services/"+serviceID, invalid)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(decodeJSON[map[string]any](response)["message"]).To(ContainSubstring("client_secret"))
		unchanged := readService(serviceID)
		Expect(unchanged).To(HaveKeyWithValue("client_id", "operator-confidential-client"))
		Expect(unchanged).To(HaveKeyWithValue("client_secret", "REDACTED"))

		response = serviceJSON(http.MethodPut, "/api/services/"+serviceID, publicRequest())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		converted := decodeJSON[map[string]any](response)
		clientID := fixtures.CIMDEndUserPublicURL + "/.well-known/oauth-client/" + serviceID
		Expect(converted).To(HaveKeyWithValue("client_id", clientID))
		Expect(converted).To(HaveKeyWithValue("client_registration", "cimd"))
		Expect(converted).NotTo(HaveKey("client_secret"))
		published, err := enduserServer.PublicGET("/.well-known/oauth-client/" + serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(published.StatusCode).To(Equal(http.StatusOK))
		Expect(decodeJSON[map[string]any](published)).To(HaveKeyWithValue("client_id", clientID))

		staticPublic := publicRequest()
		staticPublic["client_registration"] = "static"
		response = serviceJSON(http.MethodPut, "/api/services/"+serviceID, staticPublic)
		Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
		Expect(decodeJSON[map[string]any](response)["message"]).To(ContainSubstring("client_id"))
		Expect(readService(serviceID)).To(HaveKeyWithValue("client_id", clientID))

		staticPublic["client_id"] = "operator-public-client"
		response = serviceJSON(http.MethodPut, "/api/services/"+serviceID, staticPublic)
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		restored := decodeJSON[map[string]any](response)
		Expect(restored).To(HaveKeyWithValue("client_registration", "static"))
		Expect(restored).To(HaveKeyWithValue("client_id", "operator-public-client"))
		Expect(restored).NotTo(HaveKey("client_secret"))
		unpublished, err := enduserServer.PublicGET("/.well-known/oauth-client/" + serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unpublished.StatusCode).To(Equal(http.StatusNotFound))
		Expect(decodeJSON[map[string]any](unpublished)).NotTo(HaveKey("client_id"))
	})
})
