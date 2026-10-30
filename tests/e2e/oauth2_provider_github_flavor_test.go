package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("OAuth2 Provider GitHub Flavor Support", func() {
	var (
		adminServer    *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		testStorage    *storageadapter.Adapter
		principal      string
	)

	createService := func(body map[string]interface{}) *http.Response {
		b, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(b))
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	getService := func(serviceID string) *http.Response {
		resp, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	decodeJSON := func(resp *http.Response) map[string]interface{} {
		defer func() { _ = resp.Body.Close() }()
		var result map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&result)
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(GinkgoWriter, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))

		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		config := fixtures.DefaultOAuth2Config()
		serverFactory := bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())

		adminServer, err = bootstrap.NewAdminTestServer(appInstance, logger)
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

	Describe("GitHub Flavor Registration", func() {
		// Scenario 1.2 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should accept explicit github flavor and create service successfully", func() {
			req := fixtures.ValidGitHubServiceRequest()

			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("github"))
			Expect(body["client_id"]).To(Equal("Iv1.1234567890abcdef"))
			Expect(body["client_secret"]).To(Equal("REDACTED"))
		})

		// Scenario 1.2 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should return github flavor in GET response after creation", func() {
			req := fixtures.ValidGitHubServiceRequest()
			createResp := createService(req)
			Expect(createResp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(createResp)
			serviceID := created["id"].(string)

			resp := getService(serviceID)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("github"))
			Expect(body["client_id"]).To(Equal("Iv1.1234567890abcdef"))

			endpoints := body["endpoints"].(map[string]interface{})
			Expect(endpoints["token_endpoint"]).To(Equal("https://github.com/login/oauth/access_token"))
			Expect(endpoints["authorize_endpoint"]).To(Equal("https://github.com/login/oauth/authorize"))
		})

		// Scenario 3.1 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 3)
		It("should validate github flavor same as standard (require client_id, client_secret, issuer_uri)", func() {
			req := fixtures.ValidGitHubServiceRequest()
			delete(req, "client_id")

			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("client_id"))
		})
	})

	Describe("GitHub Flavor Auto-Detection", func() {
		// Scenario 1.1 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should auto-detect github flavor when token endpoint contains github.com and flavor is omitted", func() {
			req := map[string]interface{}{
				"display_name":  "Auto GitHub Service",
				"client_id":     "Iv1.auto1234567890",
				"client_secret": "ghp_autosecret1234567890",
				"issuer_uri":    "https://github.com",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://github.com/login/oauth/access_token",
					"authorize_endpoint": "https://github.com/login/oauth/authorize",
				},
				"scopes": []map[string]interface{}{{"scope_value": "repo", "description": "Repository access"}},
			}

			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("github"))
		})

		// Scenario 1.1 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should default to standard flavor when token endpoint does not contain github.com and flavor is omitted", func() {
			req := map[string]interface{}{
				"display_name":  "Generic Service",
				"client_id":     "generic-client",
				"client_secret": "generic-secret",
				"issuer_uri":    "https://auth.example.com",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://auth.example.com/oauth/token",
					"authorize_endpoint": "https://auth.example.com/oauth/authorize",
				},
				"scopes": []map[string]interface{}{{"scope_value": "read", "description": "Read access"}},
			}

			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("standard"))
		})

		// Scenario 1.1 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should default to standard flavor when non-github endpoints are provided and flavor is omitted", func() {
			req := map[string]interface{}{
				"display_name":  "No Endpoints Service",
				"client_id":     "no-ep-client",
				"client_secret": "no-ep-secret",
				"issuer_uri":    "https://auth.example.com",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://auth.example.com/oauth/token",
					"authorize_endpoint": "https://auth.example.com/oauth/authorize",
				},
				"scopes": []map[string]interface{}{{"scope_value": "read", "description": "Read"}},
			}

			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("standard"))
		})
	})

	Describe("GitHub Flavor Scope Separator", func() {
		// Scenario 1.4 from specs/20260312-120000-oauth2-provider-flavors/spec.md (User Story 1)
		It("should reject unrecognized flavor values including near-misses", func() {
			req := fixtures.ValidGitHubServiceRequest()
			req["oauth2_flavor"] = "GitHub" // capital G - invalid

			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("github"))
		})
	})
})
