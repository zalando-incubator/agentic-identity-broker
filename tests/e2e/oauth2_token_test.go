package e2e_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("OAuth2 Token Endpoint E2E (User Story 2)", func() {
	var (
		serverFactory  *bootstrap.ServerFactory
		storageFactory *bootstrap.StorageFactory
		logger         *slog.Logger
		testStorage    *storage.Adapter
		testServer     *bootstrap.TestServer
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		config         *ports.Config
		testAgent      *domainstorage.Agent // registered agent whose UUID is used as client_id
	)

	// Set up test infrastructure before each test
	BeforeEach(func() {
		// Initialize logger for test
		logger = slog.New(slog.DiscardHandler)

		// Create fresh mock upstream server
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()

		// Create configuration pointing to mock upstream
		config = fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())

		// Create storage factory and fresh storage instance
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		// Register a test agent in storage so the token handler can resolve
		// the broker-internal agent UUID to the upstream client_id.
		testAgent = fixtures.ValidAgent()
		Expect(testStorage.Agents().Create(context.Background(), testAgent)).NotTo(HaveOccurred())

		// Create server factory
		serverFactory = bootstrap.NewServerFactory(config, logger)

		// Build fresh application instance
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())

		// Create test server wrapping production app
		testServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).NotTo(HaveOccurred())
	})

	// Clean up after each test
	AfterEach(func() {
		if testServer != nil {
			testServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("Scenario 1: Successful authorization code exchange", func() {
		It("should exchange authorization code for tokens using grant_type=authorization_code", func() {
			// Given: Mock upstream configured to return successful token response
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("access-token-abc123").
				WithRefreshToken("refresh-token-xyz789").
				WithExpiresIn(3600)

			// When: POST to token endpoint with authorization_code grant
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type":    []string{"authorization_code"},
				"code":          []string{"auth-code-123"},
				"redirect_uri":  []string{"http://localhost:3000/callback"},
				"client_id":     []string{testAgent.ID.String()},
				"client_secret": []string{"secret-xyz"},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be successful (200 OK)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Then: Response should contain tokens
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var tokenResp map[string]interface{}
			err = json.Unmarshal([]byte(body), &tokenResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(tokenResp["access_token"]).To(Equal("access-token-abc123"))
			Expect(tokenResp["refresh_token"]).To(Equal("refresh-token-xyz789"))
			Expect(tokenResp["token_type"]).To(Equal("Bearer"))
			Expect(tokenResp["expires_in"]).To(Equal(float64(3600)))

			// Then: Verify upstream received the request
			Expect(mockUpstream.GetTokenCalled()).To(BeTrue())
		})
	})

	Context("Scenario 2: All parameters preserved", func() {
		It("should forward all parameters unchanged to upstream token endpoint", func() {
			// Given: Mock upstream configured for success
			mockUpstream.WithSuccessfulTokenResponse()

			// When: POST to token endpoint with various parameters
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type":    []string{"authorization_code"},
				"code":          []string{"special-auth-code-xyz"},
				"redirect_uri":  []string{"https://example.com/callback?with=params"},
				"client_id":     []string{testAgent.ID.String()},
				"client_secret": []string{"my-secret-key"},
				"scope":         []string{"openid profile email"},
				"code_verifier": []string{"pkce-verifier-value"},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be successful
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Then: Verify upstream received all parameters unchanged
			lastRequest := mockUpstream.GetLastRequest()
			Expect(lastRequest).NotTo(BeNil())
			Expect(lastRequest.FormValue("grant_type")).To(Equal("authorization_code"))
			Expect(lastRequest.FormValue("code")).To(Equal("special-auth-code-xyz"))
			Expect(lastRequest.FormValue("redirect_uri")).To(Equal("https://example.com/callback?with=params"))
			// The broker replaces the broker-internal agent UUID with the agent's upstream client_id.
			Expect(lastRequest.FormValue("client_id")).To(Equal(string(*testAgent.ClientID)))
			Expect(lastRequest.FormValue("client_secret")).To(Equal("my-secret-key"))
			Expect(lastRequest.FormValue("scope")).To(Equal("openid profile email"))
			Expect(lastRequest.FormValue("code_verifier")).To(Equal("pkce-verifier-value"))
		})
	})

	Context("Scenario 3: Response proxied unmodified", func() {
		It("should forward response body and status code exactly as received from upstream", func() {
			// Given: Mock upstream configured with specific token response
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("proxy-access-token").
				WithRefreshToken("proxy-refresh-token").
				WithExpiresIn(7200)

			// When: POST to token endpoint
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response status code should match upstream exactly
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Then: Response body should match upstream exactly
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var tokenResp map[string]interface{}
			err = json.Unmarshal([]byte(body), &tokenResp)
			Expect(err).NotTo(HaveOccurred())

			// All fields should match exactly
			Expect(tokenResp["access_token"]).To(Equal("proxy-access-token"))
			Expect(tokenResp["refresh_token"]).To(Equal("proxy-refresh-token"))
			Expect(tokenResp["expires_in"]).To(Equal(float64(7200)))
			Expect(tokenResp["token_type"]).To(Equal("Bearer"))
		})
	})

	Context("Scenario 4: Error responses from upstream", func() {
		It("should handle invalid_grant error from upstream correctly", func() {
			// Given: Mock upstream configured to return invalid_grant error
			mockUpstream.WithErrorResponseAndDescription(
				"invalid_grant",
				"The authorization code has expired or is invalid",
			)

			// When: POST to token endpoint with invalid code
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"invalid-code"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be 400 Bad Request (or similar error status)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Then: Response body should contain OAuth2 error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(errResp["error"]).To(Equal("invalid_grant"))
			Expect(errResp["error_description"]).To(ContainSubstring("authorization code"))
		})

		It("should reject non-UUID client_id with 401 invalid_client (broker-level validation)", func() {
			// The broker always treats client_id as the agent UUID. A non-UUID value is
			// rejected by the broker before the request reaches the upstream OAuth2 server.

			// When: POST to token endpoint with a non-UUID client_id
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
				"client_id":  []string{"not-a-valid-agent-uuid"},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Broker rejects with 401 before forwarding to upstream
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

			// Then: Response should contain invalid_client error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(errResp["error"]).To(Equal("invalid_client"))
		})

		It("should reject missing client_id with 400 invalid_request (broker-level validation)", func() {
			// The broker requires client_id (agent UUID) on every non-token-exchange request.

			// When: POST to token endpoint without client_id
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Broker rejects with 400 — missing required parameter per RFC 6749 §5.2
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Then: Response should contain invalid_request error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(errResp["error"]).To(Equal("invalid_request"))
		})

		It("should handle server_error from upstream correctly", func() {
			// Given: Mock upstream configured to return server error
			mockUpstream.WithErrorResponseAndDescription(
				"server_error",
				"The authorization server encountered an unexpected condition",
			)

			// When: POST to token endpoint
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be error status (400 or 500 depending on implementation)
			Expect(resp.StatusCode).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusInternalServerError)))

			// Then: Response should contain error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(errResp["error"]).To(Equal("server_error"))
		})
	})

	Context("Scenario 5: Refresh token grant", func() {
		It("should support grant_type=refresh_token for token refresh", func() {
			// Given: Mock upstream configured for successful token response
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("new-access-token").
				WithRefreshToken("new-refresh-token").
				WithExpiresIn(3600)

			// When: POST to token endpoint with refresh_token grant
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type":    []string{"refresh_token"},
				"refresh_token": []string{"old-refresh-token"},
				"client_id":     []string{testAgent.ID.String()},
				"client_secret": []string{"client-secret"},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be successful (200 OK)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Then: Response should contain new tokens
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var tokenResp map[string]interface{}
			err = json.Unmarshal([]byte(body), &tokenResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(tokenResp["access_token"]).To(Equal("new-access-token"))
			Expect(tokenResp["token_type"]).To(Equal("Bearer"))

			// Then: Verify upstream received refresh_token grant
			lastRequest := mockUpstream.GetLastRequest()
			Expect(lastRequest).NotTo(BeNil())
			Expect(lastRequest.FormValue("grant_type")).To(Equal("refresh_token"))
			Expect(lastRequest.FormValue("refresh_token")).To(Equal("old-refresh-token"))
		})

		It("should return error when refresh token is invalid or expired", func() {
			// Given: Mock upstream configured to return invalid_grant for expired refresh token
			mockUpstream.WithErrorResponseAndDescription(
				"invalid_grant",
				"The refresh token has expired",
			)

			// When: POST to token endpoint with expired refresh token
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type":    []string{"refresh_token"},
				"refresh_token": []string{"expired-refresh-token"},
				"client_id":     []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be error status
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Then: Response should contain error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())

			Expect(errResp["error"]).To(Equal("invalid_grant"))
		})
	})

	Context("Scenario 6: Invalid grant_type", func() {
		It("should return error for unsupported grant_type", func() {
			// Given: Mock upstream configured to return error for unsupported grant type
			mockUpstream.WithErrorResponseAndDescription(
				"unsupported_grant_type",
				"The authorization server does not support the grant_type: implicit",
			)

			// When: POST to token endpoint with unsupported grant_type
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"implicit"},
				"code":       []string{"code-123"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be error status
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Then: Response should contain OAuth2 error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			// If response is JSON, it should contain error field
			var errResp map[string]interface{}
			if err := json.Unmarshal([]byte(body), &errResp); err == nil {
				// JSON response should have error field
				Expect(errResp).To(HaveKey("error"))
			}
		})

		It("should return error when grant_type is missing", func() {
			// Given: Mock upstream configured to return error for missing grant_type
			mockUpstream.WithErrorResponseAndDescription(
				"invalid_request",
				"The grant_type parameter is missing",
			)

			// When: POST to token endpoint without grant_type
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"code":      []string{"code-123"},
				"client_id": []string{testAgent.ID.String()},
				// grant_type is missing
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be error status
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))

			// Then: Response should contain OAuth2 error
			body, err := helpers.ReadResponseBody(resp)
			Expect(err).NotTo(HaveOccurred())

			var errResp map[string]string
			err = json.Unmarshal([]byte(body), &errResp)
			Expect(err).NotTo(HaveOccurred())
			Expect(errResp["error"]).To(Equal("invalid_request"))
		})
	})

	Context("Additional Coverage: Content-Type validation", func() {
		It("should require application/x-www-form-urlencoded Content-Type", func() {
			// Given: Mock upstream configured for token endpoint
			mockUpstream.WithSuccessfulTokenResponse()

			// When: POST with invalid Content-Type (JSON instead of form)
			jsonBody := `{"grant_type": "authorization_code", "code": "code-123"}`
			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/json", // Invalid
				},
				strings.NewReader(jsonBody),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be error (400 Bad Request)
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("should accept form-urlencoded requests", func() {
			// Given: Mock upstream configured for token endpoint
			mockUpstream.WithSuccessfulTokenResponse().
				WithAccessToken("valid-token")

			// When: POST with correct Content-Type
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should be successful
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Context("Additional Coverage: Response headers preservation", func() {
		It("should preserve security headers from upstream response", func() {
			// Given: Mock upstream configured with security headers
			// (Note: httptest server will add Content-Type header)
			mockUpstream.WithSuccessfulTokenResponse()

			// When: POST to token endpoint
			// client_id must be a valid agent UUID (broker-internal identifier)
			formData := url.Values{
				"grant_type": []string{"authorization_code"},
				"code":       []string{"code-123"},
				"client_id":  []string{testAgent.ID.String()},
			}

			resp, err := testServer.DirectRequest(
				"POST",
				"/oauth2/token",
				"", // no principal required for token endpoint
				map[string]string{
					"Content-Type": "application/x-www-form-urlencoded",
				},
				strings.NewReader(formData.Encode()),
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			// Then: Response should have Content-Type header
			Expect(resp.Header.Get("Content-Type")).NotTo(BeEmpty())

			// Then: Response should be JSON (per OAuth2 spec)
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))
		})
	})
})
