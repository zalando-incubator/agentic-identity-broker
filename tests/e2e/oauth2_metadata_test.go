package e2e_test

import (
	"encoding/json"
	"log/slog"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// Test constants for metadata validation
const (
	metadataEndpoint = "/.well-known/oauth-authorization-server"
)

// RFC 8414 required fields for OAuth2 authorization server metadata
var requiredMetadataFields = []string{
	"issuer",
	"authorization_endpoint",
	"token_endpoint",
	"response_types_supported",
	"grant_types_supported",
}

// Unsupported response types that should not be advertised
var unsupportedResponseTypes = []string{"implicit", "hybrid"}

var _ = Describe("OAuth2 Authorization Server Metadata Discovery", func() {
	var (
		server         *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
	)

	BeforeEach(func() {
		// Create logger for test
		logger = bootstrap.TestLogger(slog.LevelInfo)

		// Create storage factory
		storageFactory = bootstrap.NewStorageFactory(logger)

		// Create fresh storage for this test
		storageAdapter, err := storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// Create server factory with OAuth2 config
		// Note: The config's PublicURL will be overridden by the test server's actual URL
		// We use a placeholder here, but it will be updated by NewTestServerBuilder
		config := fixtures.DefaultOAuth2Config()
		serverFactory := bootstrap.NewServerFactory(config, logger)

		// Use the builder pattern to ensure the app is built with the correct test server URL
		// This is critical because OAuth2Service is initialized during app.Builder.Build()
		// so it must know the correct PublicURL before that happens
		builder, err := bootstrap.NewTestServerBuilder(config, storageAdapter, serverFactory, logger)
		Expect(err).ToNot(HaveOccurred())

		server, err = builder.Build()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up server
		if server != nil {
			server.Close()
		}
	})

	// Context for testing basic metadata response format and RFC 8414 compliance
	Context("metadata endpoint responses", func() {
		var metadata map[string]interface{}

		BeforeEach(func() {
			// Fetch and decode metadata once for all tests in this context
			// This eliminates 15+ duplicate request/decode patterns across tests
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Verify basic HTTP contract
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

			// Decode JSON once, reuse in all tests
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata).NotTo(BeEmpty())
		})

		// Scenario 3.1: Returns valid RFC 8414 metadata document
		It("should return RFC 8414 compliant JSON metadata document", func() {
			// Verify response body is valid JSON and contains issuer
			// RFC 8414 requires issuer field as the minimum requirement
			Expect(metadata).To(HaveKey("issuer"))
		})

		// Scenario 3.2: Issuer field set to broker's public base URL
		It("should include issuer field set to broker's public base URL", func() {
			issuer, ok := metadata["issuer"].(string)
			Expect(ok).To(BeTrue(), "issuer should be a string")
			Expect(issuer).To(Equal(server.BaseURL()))
		})

		// Scenario 3.3: authorization_endpoint points to broker's /oauth2/authorize
		It("should include authorization_endpoint pointing to broker's /oauth2/authorize", func() {
			Expect(metadata).To(HaveKey("authorization_endpoint"))

			authzEndpoint, ok := metadata["authorization_endpoint"].(string)
			Expect(ok).To(BeTrue(), "authorization_endpoint should be a string")
			Expect(authzEndpoint).To(Equal(server.BaseURL() + "/oauth2/authorize"))
		})

		// Scenario 3.4: token_endpoint points to broker's /oauth2/token
		It("should include token_endpoint pointing to broker's /oauth2/token", func() {
			Expect(metadata).To(HaveKey("token_endpoint"))

			tokenEndpoint, ok := metadata["token_endpoint"].(string)
			Expect(ok).To(BeTrue(), "token_endpoint should be a string")
			Expect(tokenEndpoint).To(Equal(server.BaseURL() + "/oauth2/token"))
		})

		// Scenario 3.5: response_types_supported includes "code"
		It("should include response_types_supported with 'code' response type", func() {
			Expect(metadata).To(HaveKey("response_types_supported"))

			responseTypes, ok := metadata["response_types_supported"].([]interface{})
			Expect(ok).To(BeTrue(), "response_types_supported should be an array")
			Expect(responseTypes).To(ContainElement("code"))
		})

		// Scenario 3.6: grant_types_supported includes "authorization_code" and "refresh_token"
		It("should include grant_types_supported with 'authorization_code' and 'refresh_token'", func() {
			Expect(metadata).To(HaveKey("grant_types_supported"))

			grantTypes, ok := metadata["grant_types_supported"].([]interface{})
			Expect(ok).To(BeTrue(), "grant_types_supported should be an array")
			Expect(grantTypes).To(ContainElement("authorization_code"))
			Expect(grantTypes).To(ContainElement("refresh_token"))
		})
	})

	// Context for RFC 8414 compliance and HTTP header validation
	Context("metadata endpoint compliance", func() {
		It("should have correct Content-Type header", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Content-Type must be application/json
			contentType := resp.Header.Get("Content-Type")
			Expect(contentType).To(ContainSubstring("application/json"))
		})

		It("should be publicly accessible (no authentication required)", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Should succeed without authentication
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("should return proper HTTP status codes", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Status should be 200 OK
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
		})

		It("should include all required RFC 8414 fields", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var metadata map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())

			// Verify all required RFC 8414 fields are present
			for _, field := range requiredMetadataFields {
				Expect(metadata).To(HaveKey(field), "missing required field: "+field)
			}
		})

		It("should handle metadata requests correctly with charset in Accept header", func() {
			req, err := http.NewRequest("GET", server.BaseURL()+metadataEndpoint, nil)
			Expect(err).ToNot(HaveOccurred())

			req.Header.Set("Accept", "application/json; charset=utf-8")

			client := &http.Client{}
			resp, err := client.Do(req)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Should still return 200 OK with charset in Accept header
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	// Context for endpoint URL construction and consistency validation
	Context("endpoint URLs in metadata", func() {
		var metadata map[string]interface{}

		BeforeEach(func() {
			// Shared fetch and decode for all URL validation tests
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should construct authorization_endpoint correctly from public URL", func() {
			authzEndpoint, _ := metadata["authorization_endpoint"].(string)
			Expect(authzEndpoint).To(HavePrefix(server.BaseURL()))
			Expect(authzEndpoint).To(HaveSuffix("/oauth2/authorize"))
		})

		It("should construct token_endpoint correctly from public URL", func() {
			tokenEndpoint, _ := metadata["token_endpoint"].(string)
			Expect(tokenEndpoint).To(HavePrefix(server.BaseURL()))
			Expect(tokenEndpoint).To(HaveSuffix("/oauth2/token"))
		})

		It("should have consistent issuer and endpoint base URLs", func() {
			issuer, _ := metadata["issuer"].(string)
			authzEndpoint, _ := metadata["authorization_endpoint"].(string)
			tokenEndpoint, _ := metadata["token_endpoint"].(string)

			// All endpoints should start with the issuer URL
			Expect(authzEndpoint).To(HavePrefix(issuer))
			Expect(tokenEndpoint).To(HavePrefix(issuer))
		})
	})

	// Context for supported response and grant types validation
	Context("supported response and grant types", func() {
		var metadata map[string]interface{}

		BeforeEach(func() {
			// Shared fetch and decode for all type validation tests
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should only list supported response types", func() {
			responseTypes, ok := metadata["response_types_supported"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(len(responseTypes)).To(BeNumerically(">", 0))

			// All entries should be strings
			for _, rt := range responseTypes {
				_, ok := rt.(string)
				Expect(ok).To(BeTrue(), "response type should be a string")
			}
		})

		It("should only list supported grant types", func() {
			grantTypes, ok := metadata["grant_types_supported"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(len(grantTypes)).To(BeNumerically(">", 0))

			// All entries should be strings
			for _, gt := range grantTypes {
				_, ok := gt.(string)
				Expect(ok).To(BeTrue(), "grant type should be a string")
			}
		})

		It("should not include unsupported response types", func() {
			responseTypes, ok := metadata["response_types_supported"].([]interface{})
			Expect(ok).To(BeTrue())

			// Verify unsupported types are not listed
			for _, unsupType := range unsupportedResponseTypes {
				Expect(responseTypes).NotTo(ContainElement(unsupType))
			}
		})
	})

	// Context for metadata JSON structure and field type validation
	Context("metadata JSON structure", func() {
		It("should produce valid JSON without errors", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var metadata map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata).NotTo(BeEmpty())
		})

		It("should have string values for endpoint URLs", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var metadata map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())

			// Verify type correctness for URL fields
			fields := []string{"issuer", "authorization_endpoint", "token_endpoint"}
			for _, field := range fields {
				value, ok := metadata[field].(string)
				Expect(ok).To(BeTrue(), "field "+field+" should be a string, got "+value)
				Expect(value).NotTo(BeEmpty(), "field "+field+" should not be empty")
			}
		})

		It("should have array values for supported types", func() {
			resp, err := server.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			var metadata map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())

			// Verify array types for supported types fields
			arrayFields := []string{"response_types_supported", "grant_types_supported"}
			for _, field := range arrayFields {
				_, ok := metadata[field].([]interface{})
				Expect(ok).To(BeTrue(), "field "+field+" should be an array")
			}
		})
	})

	// Context for testing with custom public URL configuration
	// Note: Uses separate server setup due to custom configuration requirements
	Context("with different public URL configurations", func() {
		It("should reflect configured public URL in metadata", func() {
			// Given: Server configured with specific public URL
			testConfig := fixtures.DefaultOAuth2Config()

			// Create fresh storage for this test
			testStorageAdapter, err := storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			testFactory := bootstrap.NewServerFactory(testConfig, logger)

			// Use the builder to ensure proper URL alignment
			builder, err := bootstrap.NewTestServerBuilder(testConfig, testStorageAdapter, testFactory, logger)
			Expect(err).ToNot(HaveOccurred())

			testServer, err := builder.Build()
			Expect(err).ToNot(HaveOccurred())
			defer testServer.Close()

			// When: Request metadata
			resp, err := testServer.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Verify metadata uses the actual test server URL
			var metadata map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&metadata)
			Expect(err).ToNot(HaveOccurred())

			// The issuer and endpoints should match the actual test server URL
			expectedBaseURL := testServer.BaseURL()
			issuer, _ := metadata["issuer"].(string)
			Expect(issuer).To(Equal(expectedBaseURL))

			authzEndpoint, _ := metadata["authorization_endpoint"].(string)
			Expect(authzEndpoint).To(Equal(expectedBaseURL + "/oauth2/authorize"))

			tokenEndpoint, _ := metadata["token_endpoint"].(string)
			Expect(tokenEndpoint).To(Equal(expectedBaseURL + "/oauth2/token"))
		})
	})

	// Scenario 4.1 and 4.5 from specs/20260328-120000-oauth2-server/spec.md
	Context("local mode offline access advertisement", func() {
		var (
			localServer         *bootstrap.TestServer
			localStorageFactory *bootstrap.StorageFactory
			localStorage        *storageadapter.Adapter
			metadata            map[string]interface{}
		)

		BeforeEach(func() {
			testConfig := fixtures.LocalConfig()
			localStorageFactory = bootstrap.NewStorageFactory(logger)
			var err error
			localStorage, err = localStorageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			testFactory := bootstrap.NewServerFactory(testConfig, logger)
			builder, err := bootstrap.NewTestServerBuilder(testConfig, localStorage, testFactory, logger)
			Expect(err).ToNot(HaveOccurred())

			localServer, err = builder.Build()
			Expect(err).ToNot(HaveOccurred())

			resp, err := localServer.PublicGET(metadataEndpoint)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(json.NewDecoder(resp.Body).Decode(&metadata)).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if localServer != nil {
				localServer.Close()
			}
			if localStorage != nil {
				_ = localStorageFactory.CloseStorage(localStorage)
			}
		})

		It("advertises the refresh_token grant type", func() {
			grantTypes, ok := metadata["grant_types_supported"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(grantTypes).To(ContainElement("refresh_token"))
		})

		It("advertises the offline_access scope", func() {
			scopes, ok := metadata["scopes_supported"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(scopes).To(ContainElement("offline_access"))
		})
	})
})
