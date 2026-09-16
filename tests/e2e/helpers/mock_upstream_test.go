// Package helpers provides test utilities for E2E testing.
package helpers_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("MockUpstreamOAuth2Server", func() {
	var mockServer *helpers.MockUpstreamOAuth2Server

	BeforeEach(func() {
		mockServer = helpers.NewMockUpstreamOAuth2Server()
		DeferCleanup(func() {
			mockServer.Close()
		})
	})

	Describe("Server Initialization", func() {
		It("should create a running server", func() {
			Expect(mockServer).NotTo(BeNil())
			Expect(mockServer.Server).NotTo(BeNil())
			Expect(mockServer.URL()).NotTo(BeEmpty())
			Expect(mockServer.URL()).To(ContainSubstring("http://127.0.0.1:"))
		})

		It("should have default token configuration", func() {
			Expect(mockServer).NotTo(BeNil())
		})
	})

	Describe("Authorization Endpoint", func() {
		It("should capture authorization request", func() {
			authzURL := fmt.Sprintf("%s/oauth/authorize?client_id=test-client&redirect_uri=http://localhost:9999/cb&response_type=code&state=xyz", mockServer.URL())

			// Use a custom client that doesn't follow redirects
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			resp, err := client.Get(authzURL)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			lastReq := mockServer.GetLastRequest()
			Expect(lastReq).NotTo(BeNil())
			Expect(mockServer.GetAuthorizeCalled()).To(BeTrue())
		})

		It("should provide valid authorization code in redirect", func() {
			authzURL := fmt.Sprintf("%s/oauth/authorize?client_id=test-client&redirect_uri=http://localhost:9999/cb&response_type=code&state=xyz", mockServer.URL())

			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			resp, err := client.Get(authzURL)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			location := resp.Header.Get("Location")
			Expect(location).To(ContainSubstring("code="))
			Expect(location).To(ContainSubstring("state=xyz"))
		})
	})

	Describe("Token Endpoint", func() {
		It("should return successful token response by default", func() {
			mockServer.WithSuccessfulTokenResponse()

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"auth-code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("access_token"))
			Expect(bodyStr).To(ContainSubstring("token_type"))
			Expect(bodyStr).To(ContainSubstring("Bearer"))
		})

		It("should capture token request", func() {
			mockServer.WithSuccessfulTokenResponse()

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			_, _ = http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"test-code"},
			})

			Expect(mockServer.GetTokenCalled()).To(BeTrue())
		})

		It("should retain token requests in order", func() {
			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			for _, code := range []string{"first-code", "second-code"} {
				response, err := http.PostForm(tokenURL, url.Values{"code": {code}})
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Body.Close()).To(Succeed())
			}

			requests := mockServer.GetTokenRequests()
			Expect(requests).To(HaveLen(2))
			for i, code := range []string{"first-code", "second-code"} {
				form, err := url.ParseQuery(requests[i].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(form.Get("code")).To(Equal(code))
			}
		})

		It("should return error response when configured", func() {
			mockServer.WithErrorResponse("invalid_grant")

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"expired-code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("error"))
			Expect(bodyStr).To(ContainSubstring("invalid_grant"))
		})

		It("should include custom tokens in response", func() {
			mockServer.
				WithSuccessfulTokenResponse().
				WithAccessToken("custom-access-token").
				WithRefreshToken("custom-refresh-token")

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("custom-access-token"))
			Expect(bodyStr).To(ContainSubstring("custom-refresh-token"))
		})

		It("should block token responses until the request context is canceled when configured", func() {
			mockServer.WithTokenHangUntilCanceled()

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			client := &http.Client{Timeout: 50 * time.Millisecond}

			start := time.Now()
			_, err := client.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"code"},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Client.Timeout exceeded"))
			Expect(time.Since(start)).To(BeNumerically("<", 500*time.Millisecond))
			Expect(mockServer.GetTokenCalled()).To(BeTrue())
		})
		It("binds each strict PKCE verifier to its authorization code", func() {
			mockServer.WithStrictPublicClientMode()
			client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			authorize := func(state, verifier string) string {
				authorizationURL := mockServer.URL() + "/oauth/authorize?" + url.Values{
					"client_id":             {"test-client"},
					"redirect_uri":          {"http://localhost:9999/cb"},
					"response_type":         {"code"},
					"state":                 {state},
					"code_challenge":        {helpers.GenerateCodeChallenge(verifier)},
					"code_challenge_method": {"S256"},
				}.Encode()
				response, err := client.Get(authorizationURL)
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = response.Body.Close() }()
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				callbackURL, err := url.Parse(response.Header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
				return callbackURL.Query().Get("code")
			}

			firstVerifier := "first-flow-verifier"
			secondVerifier := "second-flow-verifier"
			firstCode := authorize("first-flow", firstVerifier)
			secondCode := authorize("second-flow", secondVerifier)
			Expect(firstCode).NotTo(Equal(secondCode))

			crossFlowResponse, err := http.PostForm(mockServer.URL()+"/oauth/token", url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {firstCode},
				"code_verifier": {secondVerifier},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = crossFlowResponse.Body.Close() }()
			Expect(crossFlowResponse.StatusCode).To(Equal(http.StatusBadRequest))

			validResponse, err := http.PostForm(mockServer.URL()+"/oauth/token", url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {firstCode},
				"code_verifier": {firstVerifier},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = validResponse.Body.Close() }()
			Expect(validResponse.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Metadata Endpoint", func() {
		It("should return metadata configuration", func() {
			metadataURL := fmt.Sprintf("%s/.well-known/openid-configuration", mockServer.URL())

			resp, err := http.Get(metadataURL)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/json"))

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("issuer"))
			Expect(bodyStr).To(ContainSubstring("authorization_endpoint"))
			Expect(bodyStr).To(ContainSubstring("token_endpoint"))
		})

		It("should capture metadata request", func() {
			metadataURL := fmt.Sprintf("%s/.well-known/openid-configuration", mockServer.URL())
			_, _ = http.Get(metadataURL)

			Expect(mockServer.GetMetadataCalled()).To(BeTrue())
		})

		It("should include valid URLs in metadata", func() {
			metadataURL := fmt.Sprintf("%s/.well-known/openid-configuration", mockServer.URL())
			resp, err := http.Get(metadataURL)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring(mockServer.URL()))
			Expect(bodyStr).To(ContainSubstring("/oauth/authorize"))
			Expect(bodyStr).To(ContainSubstring("/oauth/token"))
		})
	})

	Describe("Configuration Options", func() {
		It("should support custom error descriptions", func() {
			mockServer.WithErrorResponseAndDescription("server_error", "Custom error message")

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("Custom error message"))
		})

		It("should support custom expiration time", func() {
			mockServer.
				WithSuccessfulTokenResponse().
				WithExpiresIn(7200)

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			body, _ := io.ReadAll(resp.Body)
			bodyStr := string(body)

			Expect(bodyStr).To(ContainSubstring("7200"))
		})
	})

	Describe("Request Capture", func() {
		It("should capture request method", func() {
			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			_, _ = http.PostForm(tokenURL, url.Values{"code": {"test"}})

			lastReq := mockServer.GetLastRequest()
			Expect(lastReq.Method).To(Equal("POST"))
		})

		It("should capture request URL", func() {
			authzURL := fmt.Sprintf("%s/oauth/authorize?client_id=client-1&redirect_uri=https://app.example.com/cb&response_type=code&state=xyz", mockServer.URL())
			_, _ = http.Get(authzURL)

			lastReq := mockServer.GetLastRequest()
			Expect(lastReq.URL.Path).To(Equal("/oauth/authorize"))
		})

		It("should provide thread-safe access to captured requests", func() {
			// Make multiple concurrent requests
			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())

			for i := 0; i < 5; i++ {
				go func() { _, _ = http.PostForm(tokenURL, url.Values{"code": {"test"}}) }()
			}

			// Should be able to access without panic
			_ = mockServer.GetLastRequest()
			_ = mockServer.GetTokenCalled()
			_ = mockServer.GetAuthorizeCalled()
		})
	})

	Describe("Reset Functionality", func() {
		It("should reset request state", func() {
			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			_, _ = http.PostForm(tokenURL, url.Values{"code": {"test"}})

			Expect(mockServer.GetTokenCalled()).To(BeTrue())

			mockServer.Reset()

			Expect(mockServer.GetTokenCalled()).To(BeFalse())
			Expect(mockServer.GetLastRequest()).To(BeNil())
		})
	})

	Describe("URL Handling", func() {
		It("should provide correct server URL", func() {
			serverURL := mockServer.URL()

			Expect(serverURL).To(ContainSubstring("http://"))
			// URL should be format: http://127.0.0.1:PORT
			// It's OK to have port but no path component
			Expect(serverURL).NotTo(BeEmpty())
		})

		It("should close server gracefully", func() {
			url := mockServer.URL()
			Expect(url).NotTo(BeEmpty())

			mockServer.Close()

			// After close, new requests should fail
			_, err := http.Get(fmt.Sprintf("%s/oauth/token", url))
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Response Configuration Chaining", func() {
		It("should support method chaining for configuration", func() {
			server := helpers.NewMockUpstreamOAuth2Server()
			defer server.Close()

			configured := server.
				WithSuccessfulTokenResponse().
				WithAccessToken("token123").
				WithExpiresIn(1800)

			Expect(configured).To(Equal(server))
		})

		It("should apply configuration in correct order", func() {
			mockServer.
				WithSuccessfulTokenResponse().
				WithErrorResponse("error1").
				WithSuccessfulTokenResponse()

			tokenURL := fmt.Sprintf("%s/oauth/token", mockServer.URL())
			resp, err := http.PostForm(tokenURL, url.Values{
				"grant_type": {"authorization_code"},
				"code":       {"code"},
			})
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})

// TestMockUpstreamOAuth2ServerBasic provides a traditional test function for basic functionality
func TestMockUpstreamOAuth2ServerBasic(t *testing.T) {
	server := helpers.NewMockUpstreamOAuth2Server()
	defer server.Close()

	if server == nil {
		t.Fatal("server should not be nil")
	}

	if server.URL() == "" {
		t.Fatal("server URL should not be empty")
	}

	// Test that we can reach the metadata endpoint
	resp, err := http.Get(fmt.Sprintf("%s/.well-known/openid-configuration", server.URL()))
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
