package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// oauth2FlavorE2ESuite contains shared infrastructure for OAuth2 flavor E2E tests.
// Uses bootstrap.NewAdminTestServer() following tests/e2e/README.md patterns.
var _ = Describe("OAuth2 Provider Flavor Support", func() {
	var (
		adminServer    *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		testStorage    *storageadapter.Adapter
		principal      string
	)

	// Standard valid service request (no oauth2_flavor - defaults to standard)
	standardServiceRequest := func() map[string]interface{} {
		return map[string]interface{}{
			"display_name":  "Standard Test Service",
			"client_id":     "std-client-123",
			"client_secret": "std-secret-abc",
			"issuer_uri":    "https://issuer.example.com",
			"discovery":     map[string]interface{}{"enable_discovery": false},
			"endpoints": map[string]interface{}{
				"token_endpoint":     "https://issuer.example.com/token",
				"authorize_endpoint": "https://issuer.example.com/authorize",
			},
			"scopes": []map[string]interface{}{
				{"scope_value": "read", "description": "Read access"},
			},
		}
	}

	// Helper: post a service creation request
	createService := func(body map[string]interface{}) *http.Response {
		b, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		resp, err := adminServer.AuthenticatedPOST("/api/services", principal, "application/json", bytes.NewReader(b))
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	// Helper: PUT to update a service
	updateService := func(serviceID string, body map[string]interface{}) *http.Response {
		b, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		resp, err := adminServer.DirectRequest("PUT", "/api/services/"+serviceID, principal, map[string]string{"Content-Type": "application/json"}, bytes.NewReader(b))
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	// Helper: GET a service by ID
	getService := func(serviceID string) *http.Response {
		resp, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	// Helper: GET all services
	listServices := func() *http.Response {
		resp, err := adminServer.AuthenticatedGET("/api/services", principal)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	// Helper: decode JSON body to map
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

	// US1: Configure Third-Party Service with Explicit OAuth2 Flavor

	Describe("US1: Configure Third-Party Service with Explicit OAuth2 Flavor", func() {
		// Spec Reference: US1.S1 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US1.S1] should assign default standard flavor when oauth2_flavor is omitted", func() {
			// Given: A service creation request with no oauth2_flavor field
			req := standardServiceRequest()
			delete(req, "oauth2_flavor") // ensure field is absent

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			// Then: Response contains oauth2_flavor: standard
			body := decodeJSON(resp)
			Expect(body).To(HaveKey("oauth2_flavor"))
			Expect(body["oauth2_flavor"]).To(Equal("standard"))

			// And: GET confirms standard flavor
			serviceID := body["id"].(string)
			getResp := getService(serviceID)
			Expect(getResp).To(matchers.HaveStatusCode(http.StatusOK))
			getBody := decodeJSON(getResp)
			Expect(getBody["oauth2_flavor"]).To(Equal("standard"))
		})

		// Spec Reference: US1.S2 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US1.S2] should accept explicit standard flavor and validate credential as plain string", func() {
			// Given: A service creation request with oauth2_flavor: standard
			req := standardServiceRequest()
			req["oauth2_flavor"] = "standard"

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			// Then: Response contains oauth2_flavor: standard
			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("standard"))
		})

		// Spec Reference: US1.S3 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US1.S3] should accept google flavor and validate credential as service account JSON", func() {
			// Given: A service creation request with oauth2_flavor: google and valid SA JSON
			req := fixtures.ValidGoogleServiceRequest()

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			// Then: Response contains oauth2_flavor: google
			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("google"))
		})

		// Spec Reference: US1.S4 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US1.S4] should reject unrecognized flavor values with HTTP 400 listing valid values", func() {
			// Given: A service creation request with an invalid oauth2_flavor
			req := standardServiceRequest()
			req["oauth2_flavor"] = "azure"

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 with error message mentioning valid values
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("azure"))
		})

		// Spec Reference: US1.S5 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US1.S5] should re-validate credential against new flavor when flavor is updated", func() {
			// Given: An existing standard service
			req := standardServiceRequest()
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(resp)
			serviceID := created["id"].(string)

			// When: Update with google flavor but plain-text credential (not JSON)
			updateReq := map[string]interface{}{
				"display_name":  "Updated Service",
				"oauth2_flavor": "google",
				"client_secret": "plain-text-not-json",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "read", "description": "Read"}},
			}
			updateResp := updateService(serviceID, updateReq)
			defer func() { _ = updateResp.Body.Close() }()

			// Then: HTTP 400 because google flavor requires valid JSON
			Expect(updateResp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		})
	})

	// US2: Configure Google OAuth2 Service with Service Account Credentials

	Describe("US2: Configure Google OAuth2 Service with Service Account Credentials", func() {
		// Spec Reference: US2.S1 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S1] should store service account JSON encrypted and extract client_id from JSON", func() {
			// Given: A valid google service request
			req := fixtures.ValidGoogleServiceRequest()

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			// Then: client_id is extracted from JSON's client_id field (112233445566778899001)
			body := decodeJSON(resp)
			Expect(body["client_id"]).To(Equal("112233445566778899001"))
			Expect(body["oauth2_flavor"]).To(Equal("google"))
		})

		// Spec Reference: US2.S2 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S2] should return service config with client_id populated and credential redacted", func() {
			// Given: A valid google service created
			req := fixtures.ValidGoogleServiceRequest()
			createResp := createService(req)
			Expect(createResp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(createResp)
			serviceID := created["id"].(string)

			// When: Admin retrieves the service
			resp := getService(serviceID)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			// Then: client_id is populated from JSON, credential is REDACTED
			body := decodeJSON(resp)
			Expect(body["client_id"]).To(Equal("112233445566778899001"))
			Expect(body["client_secret"]).To(Equal("REDACTED"))
			Expect(body["oauth2_flavor"]).To(Equal("google"))
		})

		// Spec Reference: US2.S3 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S3] should reject google credential with missing private_key field with HTTP 400", func() {
			// Given: Google service account JSON without private_key
			saJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA2a2rwplBQLf0kTmkGp5RJFBpJOFBBhfJmLO0YjCGSLCuoP7\noc5RfakePrivateKeyDataForTestingPurposesOnlyNotRealCryptographicKey\n-----END RSA PRIVATE KEY-----\n"`,
				`"private_key": ""`)

			req := map[string]interface{}{
				"display_name":  "Invalid Google Service",
				"oauth2_flavor": "google",
				"client_secret": saJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 with message mentioning private_key
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("private_key"))
		})

		// Spec Reference: US2.S4 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S4] should reject google credential with type != service_account with HTTP 400", func() {
			// Given: Google JSON with wrong type
			saJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"type": "service_account"`,
				`"type": "authorized_user"`)

			req := map[string]interface{}{
				"display_name":  "Wrong Type Service",
				"oauth2_flavor": "google",
				"client_secret": saJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 with message mentioning service_account
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("service_account"))
		})

		// Spec Reference: US2.S5 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S5] should return oauth2_flavor: google and redacted credential in GET response", func() {
			// Given: A google service created
			req := fixtures.ValidGoogleServiceRequest()
			createResp := createService(req)
			Expect(createResp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(createResp)
			serviceID := created["id"].(string)

			// When: Admin retrieves the service
			resp := getService(serviceID)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			// Then: oauth2_flavor is google, credential is redacted
			body := decodeJSON(resp)
			Expect(body["oauth2_flavor"]).To(Equal("google"))
			Expect(body["client_secret"]).To(Equal("REDACTED"))
		})

		// Spec Reference: US2.S6 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S6] should reject issuer_uri whose host differs from token_uri in JSON with HTTP 400", func() {
			// Given: A google service request with mismatched issuer_uri host
			req := fixtures.ValidGoogleServiceRequest()
			req["issuer_uri"] = "https://accounts.google.com" // different from token_uri host (oauth2.googleapis.com)

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 mentioning the host mismatch
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).NotTo(BeEmpty())
		})

		// Spec Reference: US2.S7 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US2.S7] should use token_uri from JSON as token endpoint when issuer_uri is omitted", func() {
			// Given: A google service request without issuer_uri
			req := fixtures.ValidGoogleServiceRequest()
			delete(req, "issuer_uri") // omit issuer_uri

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))

			// Then: token_endpoint is set from JSON's token_uri
			body := decodeJSON(resp)
			endpoints := body["endpoints"].(map[string]interface{})
			Expect(endpoints["token_endpoint"]).To(Equal("https://oauth2.googleapis.com/token"))
		})
	})

	// US3: Validate Credentials According to OAuth2 Flavor

	Describe("US3: Validate Credentials According to OAuth2 Flavor", func() {
		// Spec Reference: US3.S1 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S1] should accept non-empty string credential for standard flavor", func() {
			// Given: A standard service with a plain string credential
			req := standardServiceRequest()
			req["client_secret"] = "any-non-empty-string"

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))
		})

		// Spec Reference: US3.S2 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S2] should reject empty/whitespace-only credential for standard flavor with HTTP 400", func() {
			// Given: A standard service with empty client_secret
			req := standardServiceRequest()
			req["client_secret"] = ""

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		})

		// Spec Reference: US3.S3 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S3] should accept valid google service account JSON for google flavor", func() {
			// Given: A google service with valid SA JSON
			req := fixtures.ValidGoogleServiceRequest()

			// When: Admin creates the service
			resp := createService(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusCreated))
		})

		// Spec Reference: US3.S4 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S4] should reject non-JSON credential for google flavor with HTTP 400", func() {
			// Given: A google service with a plain string (not JSON) credential
			req := map[string]interface{}{
				"display_name":  "Bad Google Service",
				"oauth2_flavor": "google",
				"client_secret": "not-a-json-string",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		})

		// Spec Reference: US3.S5 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S5] should reject google JSON missing required fields with HTTP 400 identifying each missing field", func() {
			// Given: Google service account JSON missing client_id
			saJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"client_id": "112233445566778899001"`,
				`"client_id": ""`)

			req := map[string]interface{}{
				"display_name":  "Missing ClientID Service",
				"oauth2_flavor": "google",
				"client_secret": saJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 mentioning client_id field
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("client_id"))
		})

		// Spec Reference: US3.S6 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US3.S6] should reject plain string (not JSON) for google flavor with HTTP 400", func() {
			// Given: A google service request with a plain string as the credential
			req := map[string]interface{}{
				"display_name":  "Bad Credential Service",
				"oauth2_flavor": "google",
				"client_secret": "ghp_notjson",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		})
	})

	// US4: List and Filter Third-Party Services by Flavor

	Describe("US4: List and Filter Third-Party Services by Flavor", func() {
		// Spec Reference: US4.S1 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US4.S1] should include oauth2_flavor in list response for all services", func() {
			// Given: Two services — one standard, one google
			stdReq := standardServiceRequest()
			stdResp := createService(stdReq)
			Expect(stdResp).To(matchers.HaveStatusCode(http.StatusCreated))
			decodeJSON(stdResp) // drain body

			googleReq := fixtures.ValidGoogleServiceRequest()
			googleResp := createService(googleReq)
			Expect(googleResp).To(matchers.HaveStatusCode(http.StatusCreated))
			decodeJSON(googleResp) // drain body

			// When: Admin lists services
			resp := listServices()
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			defer func() { _ = resp.Body.Close() }()

			// Then: All services have oauth2_flavor field
			var services []map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&services)
			Expect(err).NotTo(HaveOccurred())
			Expect(services).To(HaveLen(2))
			for _, svc := range services {
				Expect(svc).To(HaveKey("oauth2_flavor"))
				Expect(svc["oauth2_flavor"]).To(Or(Equal("standard"), Equal("google")))
			}
		})

		// Spec Reference: US4.S2 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[US4.S2] should include oauth2_flavor in single service GET response", func() {
			// Given: A standard service created
			req := standardServiceRequest()
			createResp := createService(req)
			Expect(createResp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(createResp)
			serviceID := created["id"].(string)

			// When: Admin retrieves the service
			resp := getService(serviceID)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))

			// Then: Response includes oauth2_flavor field
			body := decodeJSON(resp)
			Expect(body).To(HaveKey("oauth2_flavor"))
			Expect(body["oauth2_flavor"]).To(Equal("standard"))
		})
	})

	// Edge Cases

	Describe("Edge Cases", func() {
		// Spec Reference: EC1 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[EC1] should allow flavor change from google to standard when providing a valid plain-text credential", func() {
			// Given: A google service already created
			googleReq := fixtures.ValidGoogleServiceRequest()
			createResp := createService(googleReq)
			Expect(createResp).To(matchers.HaveStatusCode(http.StatusCreated))
			created := decodeJSON(createResp)
			serviceID := created["id"].(string)

			// When: Admin updates the service to standard flavor with a plain-text credential
			// Note: standard flavor accepts any non-empty string as client_secret
			updateReq := map[string]interface{}{
				"display_name":  "Updated to Standard",
				"client_id":     "my-client-id",
				"oauth2_flavor": "standard",
				"client_secret": "new-plain-secret",
				"issuer_uri":    "https://issuer.example.com",
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"endpoints": map[string]interface{}{
					"token_endpoint":     "https://issuer.example.com/token",
					"authorize_endpoint": "https://issuer.example.com/authorize",
				},
				"scopes": []map[string]interface{}{{"scope_value": "read", "description": "Read"}},
			}
			updateResp := updateService(serviceID, updateReq)
			defer func() { _ = updateResp.Body.Close() }()

			// Then: Update succeeds - standard flavor accepts non-empty plain string credentials
			Expect(updateResp).To(matchers.HaveStatusCode(http.StatusOK))
			var body map[string]interface{}
			err := json.NewDecoder(updateResp.Body).Decode(&body)
			Expect(err).NotTo(HaveOccurred())
			Expect(body["oauth2_flavor"]).To(Equal("standard"))
		})

		// Spec Reference: EC2 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[EC2] should reject google service account JSON exceeding 32 KB with HTTP 400", func() {
			// Given: A google service request with a credential exceeding 32 KB
			largePadding := strings.Repeat("x", 33*1024)
			oversizedJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"project_id": "test-project-123"`,
				`"project_id": "`+largePadding+`"`)

			req := map[string]interface{}{
				"display_name":  "Oversized Service",
				"oauth2_flavor": "google",
				"client_secret": oversizedJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 mentioning size limit
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("32 KB"))
		})

		// Spec Reference: EC3 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[EC3] should reject google JSON where client_id field is missing with HTTP 400", func() {
			// Given: Google service account JSON with missing client_id
			saJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"client_id": "112233445566778899001"`,
				`"client_id": ""`)

			req := map[string]interface{}{
				"display_name":  "No ClientID Service",
				"oauth2_flavor": "google",
				"client_secret": saJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 mentioning client_id
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("client_id"))
		})

		// Spec Reference: EC4 from specs/20260312-120000-oauth2-provider-flavors/spec.md
		It("[EC4] should reject google JSON where private_key is present but empty with HTTP 400", func() {
			// Given: Google service account JSON with empty private_key value
			saJSON := strings.ReplaceAll(fixtures.GoogleServiceAccountFixtureJSON(),
				`"private_key": "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA2a2rwplBQLf0kTmkGp5RJFBpJOFBBhfJmLO0YjCGSLCuoP7\noc5RfakePrivateKeyDataForTestingPurposesOnlyNotRealCryptographicKey\n-----END RSA PRIVATE KEY-----\n"`,
				`"private_key": ""`)

			req := map[string]interface{}{
				"display_name":  "Empty PrivKey Service",
				"oauth2_flavor": "google",
				"client_secret": saJSON,
				"discovery":     map[string]interface{}{"enable_discovery": false},
				"scopes":        []map[string]interface{}{{"scope_value": "openid", "description": "OpenID"}},
			}

			// When: Admin creates the service
			resp := createService(req)
			defer func() { _ = resp.Body.Close() }()

			// Then: HTTP 400 mentioning private_key
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			var errorBody map[string]interface{}
			err := json.NewDecoder(resp.Body).Decode(&errorBody)
			Expect(err).NotTo(HaveOccurred())
			Expect(fmt.Sprintf("%v", errorBody)).To(ContainSubstring("private_key"))
		})
	})
})
