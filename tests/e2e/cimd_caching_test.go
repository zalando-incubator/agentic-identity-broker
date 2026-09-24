package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("CIMD Response Caching", func() {
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		testStorage    *storageadapter.Adapter
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario US3.1 from specs/028-cimd-support/spec.md
	// Given a CIMD document was fetched on the first request,
	// When a second authorization request arrives within the TTL,
	// Then the CIMD server receives only one HTTP request (cache hit).
	Describe("when two authorization requests arrive for the same client_id within TTL", func() {
		It("fetches the CIMD document only once (cache hit on second request)", func() {
			const fakeHost = "cimd-e2e-cache.test.invalid"
			var fetchCount int64

			var clientURL string
			var redirectURI string

			cimdServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&fetchCount, 1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "max-age=300")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
			}))
			defer cimdServer.Close()

			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Caching Test Agent",
				Description:    "E2E test agent for CIMD caching scenario",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
				clientURL, redirectURI,
			)

			resp1, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp1.Body.Close()
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)), "first request should trigger exactly one CIMD fetch")

			resp2, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp2.Body.Close()
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)), "second request within TTL must not trigger a second CIMD fetch")
		})
	})

	// Scenario US3.2 from specs/028-cimd-support/spec.md
	// Given a CIMD document fetch returns a non-200 status,
	// When a second authorization request arrives immediately,
	// Then the CIMD server is called again (failed fetch is not cached).
	Describe("when the CIMD server returns a non-200 error response", func() {
		It("does not cache failed fetches and retries on the next request", func() {
			const fakeHost = "cimd-e2e-cache-error.test.invalid"
			var fetchCount int64

			var clientURL string

			cimdServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&fetchCount, 1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer cimdServer.Close()

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "No-Error-Cache Agent",
				Description:    "E2E test agent for CIMD no-error-cache scenario",
				ClientURIs:     []string{clientURL},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
				clientURL, clientURL,
			)

			resp1, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)))

			resp2, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(2)), "failed fetch must not be cached — second request must retry")
		})
	})

	// Scenario US3.3 from specs/028-cimd-support/spec.md
	// Given a CIMD document is served with max-age exceeding the operator maxTTL,
	// When a second request arrives within maxTTL,
	// Then it is served from cache (TTL is clamped to operator maxTTL, not rejected).
	Describe("when the CIMD document's Cache-Control max-age exceeds operator maxTTL", func() {
		It("clamps the TTL to operator maxTTL and still serves from cache", func() {
			const fakeHost = "cimd-e2e-cache-clamp.test.invalid"
			var fetchCount int64

			var clientURL string
			var redirectURI string

			cimdServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&fetchCount, 1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "max-age=86400") // 24h — exceeds operator maxTTL of 1h
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
			}))
			defer cimdServer.Close()

			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "TTL Clamp Test Agent",
				Description:    "E2E test agent for CIMD TTL clamping scenario",
				ClientURIs:     []string{clientURL},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
				clientURL, redirectURI,
			)

			resp1, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp1.Body.Close()
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)))

			// Immediately after — must be served from cache (clamped maxTTL applies)
			resp2, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp2.Body.Close()
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)),
				"document should be served from cache with TTL clamped to operator maxTTL (1h)")
		})
	})

	// Scenario US3.4 from specs/028-cimd-support/spec.md
	// Given a CIMD document is cached with a short TTL,
	// When a new authorization request arrives after TTL expiry,
	// Then the CIMD server is called again (cache miss after expiry).
	Describe("when the cached CIMD entry has expired", func() {
		It("re-fetches the document after TTL expiry", func() {
			const fakeHost = "cimd-e2e-cache-expiry.test.invalid"
			var fetchCount int64

			var clientURL string
			var redirectURI string

			cimdServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt64(&fetchCount, 1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "max-age=1") // 1s — expires quickly
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
			}))
			defer cimdServer.Close()

			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "TTL Expiry Agent",
				Description:    "E2E test agent for CIMD TTL expiry scenario",
				ClientURIs:     []string{clientURL},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			// Use a minTTL of 1s so the short max-age is honoured
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			config.OAuth2AuthServer.CIMD.Cache.MinTTL = 1 * time.Second
			config.OAuth2AuthServer.CIMD.Cache.MaxTTL = 1 * time.Hour
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
				clientURL, redirectURI,
			)

			resp1, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
			Expect(err).ToNot(HaveOccurred())
			_ = resp1.Body.Close()
			Expect(atomic.LoadInt64(&fetchCount)).To(Equal(int64(1)))

			Eventually(func() int64 {
				resp, err := server.AuthenticatedGET(authorizeURL, fixtures.DefaultPrincipal().String())
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.Body.Close()).To(Succeed())
				return atomic.LoadInt64(&fetchCount)
			}, 5*time.Second, 200*time.Millisecond).Should(Equal(int64(2)), "expired cache entry must trigger a re-fetch")
		})
	})
})
