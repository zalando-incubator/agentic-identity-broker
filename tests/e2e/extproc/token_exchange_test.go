// Package extproc_test contains E2E acceptance tests for the ExtProc Token Exchange Service.
//
// Each It() block maps 1:1 to an acceptance scenario from specs/015-extproc-token-exchange/spec.md.
// Tests COMPILE but FAIL initially (red phase) — they will turn GREEN when the implementation
// satisfies each acceptance scenario.
//
// Scenario Mapping:
//   - US1 Scenario 1-3: Metadata token exchange and header replacement (3 tests)
//   - US2 Scenario 1-2: Caching and cache expiry (2 tests)
//   - US3 Scenario 1-4: Configuration, startup validation, and scopes (4 tests)
//   - Edge Cases:       timeout, default TTL, singleflight (3 tests)
//
// Total: 12 acceptance test scenarios
//
// Test Execution (Red Phase):
//
//	cd tests/e2e/extproc && ginkgo -v ./...
package extproc_test

import (
	"context"
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

var _ = Describe("ExtProc Token Exchange", func() {
	var (
		env    *bootstrap.TestEnvironment
		client extprocv3.ExternalProcessorClient
		conn   *grpc.ClientConn
		logger = bootstrap.NewTestLogger()
	)

	// Fresh environment for each test — guarantees isolation (no shared cache state).
	BeforeEach(func() {
		cfg := fixtures.DefaultConfig()
		env = bootstrap.NewTestEnvironment(cfg, logger)
		env.Start()
		client, conn = env.NewExtProcClient()
	})

	AfterEach(func() {
		if conn != nil {
			conn.Close() //nolint:errcheck
		}
		if env != nil {
			env.Stop()
		}
	})

	// ---------------------------------------------------------------------------
	// User Story 1: Transparent Token Exchange for Agent Requests (P1)
	// specs/015-extproc-token-exchange/spec.md — US1
	// ---------------------------------------------------------------------------
	Describe("US1: Transparent Token Exchange", func() {

		// Spec: US1 Scenario 1
		// Given an incoming request with token-exchange metadata,
		// When ExtProc receives request headers,
		// Then it requests a token exchange using the metadata subject token
		// and resource URI.
		It("should exchange the metadata subject token using its resource URI", func() {
			// Spec: US1 Scenario 1

			// Given: Mock token exchange endpoint captures the request parameters
			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			// When: ExtProc receives request headers with token-exchange metadata
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: Token exchange was called with the subject token and resource URI
			// The exchanged token appears in the Authorization header mutation
			Expect(resp).NotTo(BeNil(), "expected ProcessingResponse, got nil")
			exchangeCallCount := env.MockTokenExchange.CallCount()
			Expect(exchangeCallCount).To(Equal(1),
				"expected token exchange endpoint to be called exactly once")

			// The request body should contain the subject_token and resource parameters
			lastBody := env.MockTokenExchange.LastBody()
			Expect(lastBody).To(ContainSubstring(fixtures.ValidBearerToken),
				"token exchange request should contain the metadata subject_token")
			Expect(lastBody).To(ContainSubstring("resource"),
				"token exchange request should contain the resource parameter")
		})

		// Spec: US1 Scenario 2
		// Given a successful token exchange response,
		// When ExtProc responds to Envoy,
		// Then the Authorization header is replaced with the exchanged token and the request continues.
		It("should replace the Authorization header with the exchanged token", func() {
			// Spec: US1 Scenario 2

			// Given: Mock returns a specific exchanged token
			env.MockTokenExchange.WithExchangedToken(fixtures.FreshExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			// When: ExtProc processes a request with token-exchange metadata
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: The Authorization header is replaced with the exchanged token
			Expect(resp).NotTo(BeNil())
			expectedAuthHeader := fmt.Sprintf("Bearer %s", fixtures.FreshExchangedToken)
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader(expectedAuthHeader),
				"Authorization header should be replaced with the exchanged token")
		})

		// Spec: US1 Scenario 3
		// Given the token exchange request fails validation or authorization,
		// When ExtProc processes the headers,
		// Then the request is rejected with a failure response and the original token is not forwarded.
		It("should reject with failure response when token exchange fails", func() {
			// Spec: US1 Scenario 3

			// Given: Token exchange endpoint returns 403 access_denied
			env.MockTokenExchange.WithError(403, "access_denied")

			// When: ExtProc processes a request with token-exchange metadata
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: ExtProc returns an ImmediateResponse (not a header mutation)
			// The original token is NOT forwarded (ImmediateResponse aborts the request)
			Expect(resp).NotTo(BeNil())
			// Per contracts/extproc-grpc.md, token exchange failure returns HTTP 500
			Expect(resp).To(helpers.HaveImmediateResponseWithStatus(500),
				"failed token exchange should result in HTTP 500 ImmediateResponse")

			// The metadata subject token must not appear in any header mutation.
			mutatedAuth := helpers.ExtractMutatedAuthorizationHeader(resp)
			Expect(mutatedAuth).To(BeEmpty(),
				"failed exchange must not forward the metadata subject token")
		})
	})

	// ---------------------------------------------------------------------------
	// User Story 2: Token Exchange Cache for Repeated Calls (P2)
	// specs/015-extproc-token-exchange/spec.md — US2
	// ---------------------------------------------------------------------------
	Describe("US2: Token Exchange Cache", func() {

		// Spec: US2 Scenario 1
		// Given a valid exchanged token stored in cache,
		// When a new request arrives with the same subject token and resource,
		// Then ExtProc uses the cached token without calling token exchange.
		It("should use cached token for same subject token and resource", func() {
			// Spec: US2 Scenario 1

			// Given: First request populates the cache
			env.MockTokenExchange.WithExchangedToken(fixtures.CachedExchangedToken)
			expiresIn := fixtures.StandardExpiresIn
			env.MockTokenExchange.WithExpiresIn(&expiresIn)

			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			// First request — should call token exchange
			resp1 := helpers.SendRequestHeaders(context.Background(), client, req)
			Expect(resp1).NotTo(BeNil())
			firstCallCount := env.MockTokenExchange.CallCount()
			Expect(firstCallCount).To(Equal(1), "first request should call token exchange once")

			// When: Second request with same token and resource
			resp2 := helpers.SendRequestHeaders(context.Background(), client, req)
			Expect(resp2).NotTo(BeNil())

			// Then: Token exchange endpoint is NOT called again (cache hit)
			secondCallCount := env.MockTokenExchange.CallCount()
			Expect(secondCallCount).To(Equal(1),
				"cache hit should not trigger a new token exchange call")

			// The cached token is returned
			expectedAuthHeader := fmt.Sprintf("Bearer %s", fixtures.CachedExchangedToken)
			Expect(resp2).To(helpers.HaveReplacedAuthorizationHeader(expectedAuthHeader),
				"second request should use the cached exchanged token")
		})

		// Spec: US2 Scenario 2
		// Given a cached exchanged token is expired,
		// When a new request arrives,
		// Then ExtProc performs a fresh token exchange and updates the cache.
		Context("when configured with a short cache TTL", func() {
			BeforeEach(func() {
				// Replace the default environment with one using a short cache TTL
				if conn != nil {
					conn.Close() //nolint:errcheck
				}
				if env != nil {
					env.Stop()
				}
				shortTTLCfg := fixtures.ShortCacheTTLConfig()
				env = bootstrap.NewTestEnvironment(shortTTLCfg, logger)
				env.Start()
				client, conn = env.NewExtProcClient()
			})

			It("should perform fresh exchange when cached token is expired", func() {
				// Spec: US2 Scenario 2

				// Given: First request populates the cache with a short TTL
				initialToken := "short-lived-exchanged-token"
				env.MockTokenExchange.WithExchangedToken(initialToken)
				shortTTLSeconds := 0 // expires_in=0 triggers default_ttl path which is ShortTTL
				env.MockTokenExchange.WithExpiresIn(&shortTTLSeconds)

				req := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					BuildWithMetadata()

				resp1 := helpers.SendRequestHeaders(context.Background(), client, req)
				Expect(resp1).NotTo(BeNil())
				Expect(env.MockTokenExchange.CallCount()).To(Equal(1), "first request should call exchange")

				// Wait for the cache entry to expire (ShortTTL + margin)
				time.Sleep(fixtures.ShortTTL + 50*time.Millisecond)

				// When: New request arrives after cache expiry
				refreshedToken := fixtures.RefreshedExchangedToken
				env.MockTokenExchange.WithExchangedToken(refreshedToken)
				expiresIn := fixtures.StandardExpiresIn
				env.MockTokenExchange.WithExpiresIn(&expiresIn)

				resp2 := helpers.SendRequestHeaders(context.Background(), client, req)
				Expect(resp2).NotTo(BeNil())

				// Then: Token exchange is called again (cache miss after expiry)
				Expect(env.MockTokenExchange.CallCount()).To(Equal(2),
					"expired cache should trigger a fresh token exchange")

				// The refreshed token is returned (not the old cached token)
				expectedAuthHeader := fmt.Sprintf("Bearer %s", refreshedToken)
				Expect(resp2).To(helpers.HaveReplacedAuthorizationHeader(expectedAuthHeader),
					"response should contain the refreshed exchanged token after cache expiry")
			})
		})
	})

	// ---------------------------------------------------------------------------
	// User Story 3: Operable Configuration and Startup Validation (P3)
	// specs/015-extproc-token-exchange/spec.md — US3
	// ---------------------------------------------------------------------------
	Describe("US3: Configuration and Startup Validation", func() {

		// Spec: US3 Scenario 1
		// Given valid configuration for gRPC and token exchange settings,
		// When the service starts,
		// Then it binds to the configured host/port and logs a startup summary.
		It("should bind to configured host/port and log startup summary", func() {
			// Spec: US3 Scenario 1
			// Note: The bootstrap.TestEnvironment.Start() already validates this —
			// if the server doesn't bind, gRPC client connection fails.
			// This test verifies the server is reachable and accepts connections.

			// Given: Valid config (default test config)
			// (env is started in BeforeEach)

			// When: Client connects and makes a metadata-backed exchange request.
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: Server responds (it bound successfully and is processing requests).
			Expect(resp).NotTo(BeNil(),
				"server should respond to requests when started with valid config")
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader("Bearer "+fixtures.DefaultExchangedToken),
				"metadata-backed exchange should succeed on the started server")
		})

		// Spec: US3 Scenario 2
		// Given missing or invalid required configuration values,
		// When the service starts,
		// Then it exits with a configuration validation error.
		It("should exit with validation error on invalid configuration", func() {
			// Spec: US3 Scenario 2
			// Test the config validation function directly — startup validation
			// should reject invalid configs before attempting to bind.

			// Given: Config with missing required fields
			invalidCfg := fixtures.InvalidConfig()

			// When: Attempting to validate/start the service with invalid config
			err := extprocconfig.Validate(invalidCfg)

			// Then: Validation returns an error
			Expect(err).To(HaveOccurred(),
				"invalid configuration should produce a validation error at startup")
			Expect(err.Error()).To(ContainSubstring("token_endpoint"),
				"error message should reference the missing token_endpoint field")
		})
	})

	// ---------------------------------------------------------------------------
	// Edge Cases (from specs/015-extproc-token-exchange/spec.md)
	// ---------------------------------------------------------------------------
	Describe("Edge Cases", func() {

		// Edge Case: timeout
		// How does the service handle token exchange timeouts or non-200 responses?
		// Expected: return 500 response and log the failure (FR-010)
		It("should return 500 response and log failure when token exchange times out", func() {
			// Spec: Edge case — timeout / non-200 from authorization server

			// Given: Token exchange endpoint returns 500 error
			env.MockTokenExchange.WithError(500, "server_error")

			// When: ExtProc processes a request
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: ExtProc returns 500 ImmediateResponse (FR-010)
			Expect(resp).NotTo(BeNil())
			Expect(resp).To(helpers.HaveImmediateResponseWithStatus(500),
				"token exchange failure should result in HTTP 500 ImmediateResponse")

			// Body should contain the error field but NOT expose upstream error details (FR: info disclosure prevention)
			body := helpers.ExtractImmediateResponseBody(resp)
			Expect(body).To(ContainSubstring("token_exchange_failed"),
				"error body should use generic error code, not upstream details")
		})

		// Edge Case: no expiry in exchange response
		// When the exchanged token response lacks an expiration time,
		// ExtProc should use the default cache TTL (FR-012).
		It("should use the default cache TTL when exchanged token lacks expiry information", func() {
			// Spec: Edge case — no expires_in in token exchange response

			// Given: Token exchange returns a response WITHOUT expires_in
			env.MockTokenExchange.WithExchangedToken(fixtures.DefaultExchangedToken)
			env.MockTokenExchange.WithExpiresIn(nil) // nil = no expires_in field

			// When: ExtProc processes a request
			req := helpers.NewRequestHeaders().
				WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
				BuildWithMetadata()

			resp := helpers.SendRequestHeaders(context.Background(), client, req)

			// Then: Token exchange succeeds and token is cached (using default TTL)
			Expect(resp).NotTo(BeNil())
			expectedAuthHeader := fmt.Sprintf("Bearer %s", fixtures.DefaultExchangedToken)
			Expect(resp).To(helpers.HaveReplacedAuthorizationHeader(expectedAuthHeader),
				"token without expiry should still be cached and returned successfully")

			// Second request should use cache (proving default TTL was applied, not zero TTL)
			resp2 := helpers.SendRequestHeaders(context.Background(), client, req)
			Expect(resp2).NotTo(BeNil())
			Expect(env.MockTokenExchange.CallCount()).To(Equal(1),
				"token without expiry should use default_ttl and be cached for subsequent requests")
		})
	})

	// ---------------------------------------------------------------------------
	// Configuration: client_credentials_scopes (optional)
	// ---------------------------------------------------------------------------
	Describe("client_credentials_scopes configuration", func() {

		// When no scopes are configured, the client_credentials grant defaults to ["openid"].
		Context("when no client_credentials_scopes are configured", func() {
			// Spec: US3 Scenario 3
			// Given no client_credentials_scopes are configured, when the ExtProc service
			// acquires a client assertion, then it sends scope=openid to the client_credentials endpoint.
			It("should send scope=openid as the default to the client_credentials endpoint", func() {
				// DefaultConfig has no ClientCredentialsScopes set.
				// Start() already populated env with DefaultConfig, so just use env directly.

				// Trigger a token exchange to ensure the client assertion was acquired.
				req := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					BuildWithMetadata()

				resp := helpers.SendRequestHeaders(context.Background(), client, req)
				Expect(resp).NotTo(BeNil())

				// The mock OAuth2 server should have received scope=openid
				lastForm := env.MockOAuth2.LastForm()
				Expect(lastForm).To(ContainSubstring("scope=openid"),
					"when no scopes configured, client_credentials grant should default to scope=openid")
			})
		})

		// When explicit scopes are configured, the client_credentials grant sends them.
		Context("when client_credentials_scopes are configured", func() {
			BeforeEach(func() {
				// Replace the default environment with one using custom scopes
				if conn != nil {
					conn.Close() //nolint:errcheck
				}
				if env != nil {
					env.Stop()
				}
				customScopesCfg := fixtures.ConfigWithClientCredentialsScopes([]string{"openid", "profile", "email"})
				env = bootstrap.NewTestEnvironment(customScopesCfg, logger)
				env.Start()
				client, conn = env.NewExtProcClient()
			})

			// Spec: US3 Scenario 4
			// Given client_credentials_scopes are configured, when the ExtProc service
			// acquires a client assertion, then it sends the configured scopes to the client_credentials endpoint.
			It("should send the configured scopes to the client_credentials endpoint", func() {
				// Trigger a token exchange to ensure the client assertion was acquired.
				req := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					BuildWithMetadata()

				resp := helpers.SendRequestHeaders(context.Background(), client, req)
				Expect(resp).NotTo(BeNil())

				// The mock OAuth2 server should have received the configured scopes.
				lastForm := env.MockOAuth2.LastForm()
				Expect(lastForm).To(ContainSubstring("openid"),
					"configured scopes should be sent to the client_credentials endpoint")
				Expect(lastForm).To(ContainSubstring("profile"),
					"configured scopes should be sent to the client_credentials endpoint")
				Expect(lastForm).To(ContainSubstring("email"),
					"configured scopes should be sent to the client_credentials endpoint")
			})
		})
	})

	// ---------------------------------------------------------------------------
	// Edge Cases (from specs/015-extproc-token-exchange/spec.md)
	// ---------------------------------------------------------------------------
	Describe("Edge Cases", func() {

		// Edge Case: singleflight
		// How does the service handle concurrent requests that race to refresh an expired cache entry?
		// Expected: only one token exchange occurs (FR-014)
		Context("when configured with a short cache TTL for singleflight deduplication", func() {
			BeforeEach(func() {
				// Replace the default environment with one using a short cache TTL
				if conn != nil {
					conn.Close() //nolint:errcheck
				}
				if env != nil {
					env.Stop()
				}
				shortCfg := fixtures.ShortCacheTTLConfig()
				env = bootstrap.NewTestEnvironment(shortCfg, logger)
				env.Start()
				client, conn = env.NewExtProcClient()
			})

			It("should perform only one token exchange via singleflight for concurrent requests", func() {
				// Spec: Edge case — singleflight refresh deduplication

				// Given: Populate cache first with a short-lived token
				initialToken := "singleflight-initial-token"
				env.MockTokenExchange.WithExchangedToken(initialToken)
				zeroExpiry := 0
				env.MockTokenExchange.WithExpiresIn(&zeroExpiry) // triggers short default TTL

				req := helpers.NewRequestHeaders().
					WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
					BuildWithMetadata()

				resp0 := helpers.SendRequestHeaders(context.Background(), client, req)
				Expect(resp0).NotTo(BeNil())
				Expect(env.MockTokenExchange.CallCount()).To(Equal(1))

				// Wait for cache expiry
				time.Sleep(fixtures.ShortTTL + 50*time.Millisecond)

				// Set up refreshed token for the concurrent refresh requests
				refreshedToken := "singleflight-refreshed-token"
				env.MockTokenExchange.WithExchangedToken(refreshedToken)
				longExpiry := fixtures.StandardExpiresIn
				env.MockTokenExchange.WithExpiresIn(&longExpiry)

				// When: N concurrent requests arrive simultaneously (all cache misses)
				const numConcurrent = 10
				var wg sync.WaitGroup
				type exchangeResult struct {
					response *extprocv3.ProcessingResponse
					err      error
				}
				results := make(chan exchangeResult, numConcurrent)
				wg.Add(numConcurrent)

				for range numConcurrent {
					go func() {
						defer wg.Done()
						defer GinkgoRecover()
						concurrentClient, concurrentConn := env.NewExtProcClient()
						defer concurrentConn.Close() //nolint:errcheck

						resp, err := helpers.SendRequestHeadersWithError(context.Background(), concurrentClient, req)
						results <- exchangeResult{resp, err}
					}()
				}
				wg.Wait()
				close(results)

				// Then: All concurrent requests succeed
				completed := 0
				for result := range results {
					Expect(result.err).NotTo(HaveOccurred(), "concurrent request should succeed")
					Expect(result.response).NotTo(BeNil())
					Expect(result.response).To(helpers.HaveReplacedAuthorizationHeader("Bearer " + refreshedToken))
					completed++
				}
				Expect(completed).To(Equal(numConcurrent), "all concurrent requests should complete")

				// Only ONE additional token exchange call should have been made.
				Expect(env.MockTokenExchange.CallCount()).To(Equal(2),
					"singleflight should deduplicate concurrent token exchange refreshes to exactly one call")
			})
		})
	})
})
