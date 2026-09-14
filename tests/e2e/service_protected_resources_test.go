package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("Protected Resource Subresources", func() {
	var (
		adminServer    *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		principal      string
	)

	fullyEscapeResource := func(resource string) string {
		const hex = "0123456789ABCDEF"
		var escaped strings.Builder
		escaped.Grow(len(resource) * 3)
		for i := 0; i < len(resource); i++ {
			c := resource[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~' {
				escaped.WriteByte(c)
				continue
			}
			escaped.WriteByte('%')
			escaped.WriteByte(hex[c>>4])
			escaped.WriteByte(hex[c&0x0f])
		}
		return escaped.String()
	}

	toInterfaces := func(values []string) []interface{} {
		result := make([]interface{}, len(values))
		for i, value := range values {
			result[i] = value
		}
		return result
	}

	decodeObject := func(response *http.Response) map[string]interface{} {
		defer func() { _ = response.Body.Close() }()
		Expect(response.Header.Get("Content-Type")).To(HavePrefix("application/json"))
		var body map[string]interface{}
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		return body
	}

	resourceSet := func(body map[string]interface{}) []interface{} {
		resources, ok := body["protected_resources"].([]interface{})
		Expect(ok).To(BeTrue(), "response must contain protected_resources as an array")
		return resources
	}

	seedService := func(kind string, resources []string) string {
		ctx := context.Background()
		if kind == "github" {
			service := fixtures.GitHubService()
			service.ProtectedResources = resources
			Expect(testStorage.Services().Create(ctx, service)).To(Succeed())
			return service.ID.String()
		}

		service := fixtures.GoogleService()
		service.ProtectedResources = resources
		Expect(testStorage.Services().Create(ctx, service)).To(Succeed())
		return service.ID.String()
	}

	getService := func(serviceID string) map[string]interface{} {
		response, err := adminServer.AuthenticatedGET("/api/services/"+serviceID, principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response).To(matchers.HaveStatusCode(http.StatusOK))
		return decodeObject(response)
	}

	listResources := func(serviceID string) *http.Response {
		response, err := adminServer.AuthenticatedGET("/api/services/"+serviceID+"/protected-resources", principal)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	mutate := func(method, path string, body interface{}) *http.Response {
		var reader *bytes.Reader
		headers := map[string]string{}
		if body != nil {
			encoded, err := json.Marshal(body)
			Expect(err).NotTo(HaveOccurred())
			reader = bytes.NewReader(encoded)
			headers["Content-Type"] = "application/json"
		} else {
			reader = bytes.NewReader(nil)
		}
		response, err := adminServer.DirectRequest(method, path, principal, headers, reader)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	expectMutation := func(response *http.Response, status int, resource string, resources []string) {
		Expect(response).To(matchers.HaveStatusCode(status))
		Expect(response.Header.Get("ETag")).To(MatchRegexp(`^"[^"]+"$`))
		body := decodeObject(response)
		Expect(body["resource"]).To(Equal(resource))
		Expect(resourceSet(body)).To(ConsistOf(toInterfaces(resources)...))
	}

	expectError := func(response *http.Response, status int) {
		Expect(response).To(matchers.HaveStatusCode(status))
		body := decodeObject(response)
		Expect(body["error"]).To(BeAssignableToTypeOf(""))
		Expect(body["error"]).NotTo(BeEmpty())
	}

	BeforeEach(func() {
		logger := bootstrap.TestLogger(0)
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

	Describe("US1: add a single protected resource", func() {
		// Scenario 1.1 from specs/035-protected-resource-subresources/spec.md
		It("should add a member-addressed resource and leave unrelated service fields unchanged", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			before := getService(serviceID)
			resource := "https://resources.example.test/v1/?view=all#fragment%25value"
			normalized := "https://resources.example.test/v1?view=all#fragment%25value"

			response := mutate(http.MethodPut, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource(resource), nil)
			expectMutation(response, http.StatusCreated, normalized, []string{"https://api.github.com", normalized})

			after := getService(serviceID)
			Expect(resourceSet(after)).To(ConsistOf("https://api.github.com", normalized))
			Expect(after["display_name"]).To(Equal(before["display_name"]))
			Expect(after["client_id"]).To(Equal(before["client_id"]))
			Expect(after["issuer_uri"]).To(Equal(before["issuer_uri"]))
			Expect(after["discovery"]).To(Equal(before["discovery"]))
			Expect(after["endpoints"]).To(Equal(before["endpoints"]))
			Expect(after["scopes"]).To(Equal(before["scopes"]))
			Expect(after).To(HaveKeyWithValue("client_secret", "REDACTED"))
		})

		// Scenario 1.1 from specs/035-protected-resource-subresources/spec.md (POST alternate add form)
		It("should add a collection resource through POST without requiring service credentials", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			resource := "https://resources.example.test/post/"

			response := mutate(http.MethodPost, "/api/services/"+serviceID+"/protected-resources", map[string]string{"resource_uri": resource})
			expectMutation(response, http.StatusCreated, "https://resources.example.test/post", []string{"https://api.github.com", "https://resources.example.test/post"})

			after := getService(serviceID)
			Expect(after).To(HaveKeyWithValue("client_secret", "REDACTED"))
			Expect(resourceSet(after)).To(ConsistOf("https://api.github.com", "https://resources.example.test/post"))
		})

		// Scenario 1.2 from specs/035-protected-resource-subresources/spec.md
		It("should be idempotent when a normalized URI is already owned by the service", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/already"})

			response := mutate(http.MethodPost, "/api/services/"+serviceID+"/protected-resources", map[string]string{"resource_uri": "https://resources.example.test/already/"})
			expectMutation(response, http.StatusOK, "https://resources.example.test/already", []string{"https://api.github.com", "https://resources.example.test/already"})
		})

		// Scenario 1.3 from specs/035-protected-resource-subresources/spec.md
		It("should reject a resource URI owned by another service without changing either set", func() {
			githubID := seedService("github", []string{"https://api.github.com"})
			googleID := seedService("google", []string{"https://www.googleapis.com", "https://shared.example.test/resource"})

			response := mutate(http.MethodPut, "/api/services/"+githubID+"/protected-resources/"+fullyEscapeResource("https://shared.example.test/resource"), nil)
			expectError(response, http.StatusConflict)
			Expect(resourceSet(getService(githubID))).To(ConsistOf("https://api.github.com"))
			Expect(resourceSet(getService(googleID))).To(ConsistOf("https://www.googleapis.com", "https://shared.example.test/resource"))
		})

		// Scenario 1.4 from specs/035-protected-resource-subresources/spec.md
		It("should reject malformed, relative, empty, and whitespace-only resource URIs without changing the set", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			for _, invalid := range []string{"not a URI", "/relative", "", " \t "} {
				response := mutate(http.MethodPost, "/api/services/"+serviceID+"/protected-resources", map[string]string{"resource_uri": invalid})
				expectError(response, http.StatusBadRequest)
				Expect(resourceSet(getService(serviceID))).To(ConsistOf("https://api.github.com"))
			}
		})

		// Scenario 1.5 from specs/035-protected-resource-subresources/spec.md
		It("should return not found when adding to a non-existent service", func() {
			response := mutate(http.MethodPost, "/api/services/00000000-0000-0000-0000-000000000099/protected-resources", map[string]string{"resource_uri": "https://resources.example.test/missing"})
			expectError(response, http.StatusNotFound)
		})
	})

	Describe("US2: remove a single protected resource", func() {
		// Scenario 2.1 from specs/035-protected-resource-subresources/spec.md
		It("should remove an owned resource and preserve unrelated service fields", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/remove"})
			before := getService(serviceID)

			response := mutate(http.MethodDelete, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/remove"), nil)
			expectMutation(response, http.StatusOK, "https://resources.example.test/remove", []string{"https://api.github.com"})

			after := getService(serviceID)
			Expect(resourceSet(after)).To(ConsistOf("https://api.github.com"))
			Expect(after["display_name"]).To(Equal(before["display_name"]))
			Expect(after["client_id"]).To(Equal(before["client_id"]))
			Expect(after["issuer_uri"]).To(Equal(before["issuer_uri"]))
			Expect(after["endpoints"]).To(Equal(before["endpoints"]))
		})

		// Scenario 2.2 from specs/035-protected-resource-subresources/spec.md
		It("should return not found when removing a resource the service does not own", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			response := mutate(http.MethodDelete, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/not-owned"), nil)
			expectError(response, http.StatusNotFound)
			Expect(resourceSet(getService(serviceID))).To(ConsistOf("https://api.github.com"))
		})

		Context("when the resource was resolvable via token exchange", func() {
			var (
				enduserServer *bootstrap.TestServer
				mockUpstream  *helpers.MockUpstreamOAuth2Server
				postExchange  func() *http.Response
				resource      string
				serviceID     string
			)

			BeforeEach(func() {
				resource = "https://resources.example.test/resolution"
				serviceID = seedService("github", []string{"https://api.github.com", resource})
				mockUpstream = helpers.NewMockUpstreamOAuth2Server()
				agent := fixtures.ValidAgent()
				ctx := context.Background()
				Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())
				Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, id.MustParseServiceID(serviceID))).To(Succeed())
				Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), serviceID, []string{"repo", "user"}))).To(Succeed())
				Expect(testStorage.UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal))).To(Succeed())

				app, err := bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL()), bootstrap.TestLogger(0)).BuildApp(testStorage)
				Expect(err).NotTo(HaveOccurred())
				enduserServer, err = bootstrap.NewEndUserTestServer(app, bootstrap.TestLogger(0))
				Expect(err).NotTo(HaveOccurred())

				tokens := generateTokenFixtures(mockUpstream, principal, agent)
				postExchange = func() *http.Response {
					exchange, err := enduserServer.PublicPOST(
						"/oauth2/token",
						"application/x-www-form-urlencoded",
						strings.NewReader(url.Values{
							"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
							"subject_token":         {tokens.SubjectToken},
							"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
							"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
							"client_assertion":      {tokens.ClientAssertion},
							"resource":              {resource},
						}.Encode()),
					)
					Expect(err).NotTo(HaveOccurred())
					return exchange
				}
			})

			AfterEach(func() {
				if enduserServer != nil {
					enduserServer.Close()
				}
				if mockUpstream != nil {
					mockUpstream.Close()
				}
			})

			// Scenario 2.3 from specs/035-protected-resource-subresources/spec.md
			It("should make the removed URI unresolvable to RFC 8693 token exchange", func() {
				beforeDeletion := postExchange()
				Expect(beforeDeletion).To(matchers.HaveStatusCode(http.StatusOK))
				Expect(beforeDeletion.Body.Close()).To(Succeed())

				response := mutate(http.MethodDelete, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource(resource), nil)
				expectMutation(response, http.StatusOK, resource, []string{"https://api.github.com"})

				listed := listResources(serviceID)
				Expect(listed).To(matchers.HaveStatusCode(http.StatusOK))
				Expect(resourceSet(decodeObject(listed))).To(ConsistOf("https://api.github.com"))

				exchange := postExchange()
				defer func() { _ = exchange.Body.Close() }()
				Expect(exchange).To(matchers.HaveStatusCode(http.StatusBadRequest))

				var errorResponse map[string]interface{}
				Expect(json.NewDecoder(exchange.Body).Decode(&errorResponse)).To(Succeed())
				Expect(errorResponse["error"]).To(Equal("invalid_target"))
			})
		})

		// Scenario 2.4 from specs/035-protected-resource-subresources/spec.md
		It("should allow removing the last resource and leave an empty set", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			response := mutate(http.MethodDelete, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://api.github.com"), nil)
			expectMutation(response, http.StatusOK, "https://api.github.com", []string{})

			body := decodeObject(listResources(serviceID))
			Expect(resourceSet(body)).To(BeEmpty())
		})
	})

	Describe("US3: rename a protected resource", func() {
		// Scenario 3.1 from specs/035-protected-resource-subresources/spec.md
		It("should rename an owned resource to a normalized unclaimed URI", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/source"})
			response := mutate(http.MethodPatch, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/source"), map[string]string{"to": "https://resources.example.test/target/"})
			expectMutation(response, http.StatusOK, "https://resources.example.test/target", []string{"https://api.github.com", "https://resources.example.test/target"})
		})

		// Scenario 3.2 from specs/035-protected-resource-subresources/spec.md
		It("should reject a rename to a URI owned elsewhere or already present", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/source", "https://resources.example.test/existing"})
			seedService("google", []string{"https://www.googleapis.com", "https://resources.example.test/other-service"})

			for _, target := range []string{"https://resources.example.test/existing", "https://resources.example.test/other-service"} {
				response := mutate(http.MethodPatch, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/source"), map[string]string{"to": target})
				expectError(response, http.StatusConflict)
				Expect(resourceSet(getService(serviceID))).To(ConsistOf("https://api.github.com", "https://resources.example.test/source", "https://resources.example.test/existing"))
			}
		})

		// Scenario 3.3 from specs/035-protected-resource-subresources/spec.md
		It("should return not found when renaming a resource the service does not own", func() {
			serviceID := seedService("github", []string{"https://api.github.com"})
			response := mutate(http.MethodPatch, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/missing-source"), map[string]string{"to": "https://resources.example.test/target"})
			expectError(response, http.StatusNotFound)
			Expect(resourceSet(getService(serviceID))).To(ConsistOf("https://api.github.com"))
		})

		// Scenario 3.4 from specs/035-protected-resource-subresources/spec.md
		It("should treat renaming a resource to itself as a no-op success", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/same"})
			response := mutate(http.MethodPatch, "/api/services/"+serviceID+"/protected-resources/"+fullyEscapeResource("https://resources.example.test/same"), map[string]string{"to": "https://resources.example.test/same"})
			expectMutation(response, http.StatusOK, "https://resources.example.test/same", []string{"https://api.github.com", "https://resources.example.test/same"})
		})
	})

	Describe("US4: retrieve protected resources", func() {
		// Scenario 4.1 from specs/035-protected-resource-subresources/spec.md
		It("should return every normalized protected resource for a service", func() {
			serviceID := seedService("github", []string{"https://api.github.com", "https://resources.example.test/v1", "https://resources.example.test/v2"})
			response := listResources(serviceID)
			Expect(response).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(response.Header.Get("ETag")).To(MatchRegexp(`^"[^"]+"$`))
			body := decodeObject(response)
			Expect(resourceSet(body)).To(ConsistOf("https://api.github.com", "https://resources.example.test/v1", "https://resources.example.test/v2"))
		})

		// Scenario 4.2 from specs/035-protected-resource-subresources/spec.md
		It("should return not found when listing resources for a non-existent service", func() {
			response := listResources("00000000-0000-0000-0000-000000000099")
			expectError(response, http.StatusNotFound)
		})
	})
})
