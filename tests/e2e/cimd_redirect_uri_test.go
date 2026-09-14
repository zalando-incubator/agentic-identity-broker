package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// authorizeURL builds an /oauth2/authorize query for the given client_id and redirect_uri.
func authorizeURL(clientID, redirectURI string) string {
	return fmt.Sprintf(
		"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz&code_challenge=redirect-uri-test-challenge&code_challenge_method=S256",
		url.QueryEscape(clientID), url.QueryEscape(redirectURI),
	)
}

func assertCIMDConsentContext(data map[string]any, redirectURI string, verifiedDomain string) {
	cimdMeta, ok := data["cimd_metadata"].(map[string]any)
	Expect(ok).To(BeTrue(), "cimd_metadata should be present")
	Expect(cimdMeta["redirect_uri"]).To(Equal(redirectURI))
	Expect(cimdMeta["verified_domain"]).To(Equal(verifiedDomain))
}

var _ = Describe("CIMD Redirect URI Matching — Portless Registration", func() {
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
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

	// US1: Native app registers WITHOUT a port — any ephemeral port at runtime matches.
	Describe("US1: portless loopback registered URI", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		const fakeHost = "cimd-e2e-us1.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + "/client"
			// CIMD document advertises a portless loopback redirect URI.
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{"http://localhost/callback"}))
			}))

			agent := fixtures.LocalAgent()
			agent.ClientURIs = []string{clientURL}
			agent.DisplayName = "US1 Portless Agent"
			agent.Description = "CIMD agent with portless loopback redirect URI for 028b testing"
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			sf := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := sf.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		// Scenario US1.1 from specs/028b-portless-registration/spec.md
		It("should accept ephemeral port 52341 when portless URI is registered", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://localhost:52341/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://localhost:52341/callback", fakeHost)
				},
			)
		})

		// Scenario US1.2 from specs/028b-portless-registration/spec.md
		It("should accept ephemeral port 8080 when portless URI is registered", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://localhost:8080/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://localhost:8080/callback", fakeHost)
				},
			)
		})

		// Scenario US1.3 from specs/028b-portless-registration/spec.md
		It("should accept portless request when portless URI is registered", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://localhost/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://localhost/callback", fakeHost)
				},
			)
		})

		// Scenario US1.4 from specs/028b-portless-registration/spec.md
		It("should reject request with path mismatch even when host is loopback", func() {
			resp, err := server.AuthenticatedGET(
				authorizeURL(clientURL, "http://localhost:52341/other"),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
		})
	})

	// US2: Native app registers WITH an explicit port — any other port at runtime still matches.
	Describe("US2: explicit-port loopback registered URI", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		const fakeHost = "cimd-e2e-us2.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + "/client"
			// CIMD document advertises an explicit-port loopback redirect URI.
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{"http://localhost:3000/callback"}))
			}))

			agent := fixtures.LocalAgent()
			agent.ClientURIs = []string{clientURL}
			agent.DisplayName = "US2 Explicit Port Agent"
			agent.Description = "CIMD agent with explicit-port loopback redirect URI for 028b testing"
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			sf := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := sf.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		// Scenario US2.1 from specs/028b-portless-registration/spec.md
		It("should accept a different ephemeral port when explicit port :3000 is registered", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://localhost:9999/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://localhost:9999/callback", fakeHost)
				},
			)
		})

		// Scenario US2.2 from specs/028b-portless-registration/spec.md
		It("should accept portless request when explicit port :3000 is registered", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://localhost/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://localhost/callback", fakeHost)
				},
			)
		})
	})

	// US2 Scenario 3 uses 127.0.0.1 — separate Describe for its own CIMD document.
	Describe("US2: 127.0.0.1 explicit-port registration", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		const fakeHost = "cimd-e2e-us2b.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + "/client"
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{"http://127.0.0.1:8080/callback"}))
			}))

			agent := fixtures.LocalAgent()
			agent.ClientURIs = []string{clientURL}
			agent.DisplayName = "US2b 127.0.0.1 Agent"
			agent.Description = "CIMD agent with 127.0.0.1 explicit-port redirect URI for 028b testing"
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			sf := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := sf.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		// Scenario US2.3 from specs/028b-portless-registration/spec.md
		It("should accept a different ephemeral port for 127.0.0.1 loopback registration", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"http://127.0.0.1:51234/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "http://127.0.0.1:51234/callback", fakeHost)
				},
			)
		})
	})

	// US3: Non-loopback URIs require exact four-component match — port exception does NOT apply.
	Describe("US3: non-loopback registered URI", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		const fakeHost = "cimd-e2e-us3.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + "/client"
			// CIMD document advertises a non-loopback HTTPS redirect URI (portless).
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{"https://" + fakeHost + "/callback"}))
			}))

			agent := fixtures.LocalAgent()
			agent.ClientURIs = []string{clientURL}
			agent.DisplayName = "US3 Non-Loopback Agent"
			agent.Description = "CIMD agent with non-loopback HTTPS redirect URI for 028b testing"
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			sf := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := sf.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		// Scenario US3.1 from specs/028b-portless-registration/spec.md
		It("should reject non-loopback request with a port when portless non-loopback URI is registered", func() {
			resp, err := server.AuthenticatedGET(
				authorizeURL(clientURL, "https://"+fakeHost+":9999/callback"),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
		})

		// Additional coverage: exact match for portless non-loopback registration.
		It("should accept exact match for non-loopback URI", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"https://"+fakeHost+"/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "https://"+fakeHost+"/callback", fakeHost)
				},
			)
		})
	})

	// US3 Scenario 2 uses an explicit-port non-loopback URI — separate Describe.
	Describe("US3: explicit-port non-loopback registered URI", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		const fakeHost = "cimd-e2e-us3b.test.invalid"

		BeforeEach(func() {
			clientURL = "https://" + fakeHost + ":443/client"
			// CIMD document advertises an explicit-port non-loopback redirect URI.
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{"https://" + fakeHost + ":443/callback"}))
			}))

			agent := fixtures.LocalAgent()
			agent.ClientURIs = []string{clientURL}
			agent.DisplayName = "US3b Explicit-Port Non-Loopback Agent"
			agent.Description = "CIMD agent with explicit-port non-loopback redirect URI for 028b testing"
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			sf := bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := sf.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if cimdServer != nil {
				cimdServer.Close()
			}
			if server != nil {
				server.Close()
			}
		})

		// Scenario US3.2 from specs/028b-portless-registration/spec.md
		It("should reject non-loopback request with a different port when explicit port :443 is registered", func() {
			resp, err := server.AuthenticatedGET(
				authorizeURL(clientURL, "https://"+fakeHost+":9000/callback"),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
		})

		// Scenario US3.3 from specs/028b-portless-registration/spec.md
		It("should accept non-loopback request when explicit port :443 matches exactly", func() {
			completeAuthorizationCodeFlow(
				server,
				testStorage,
				fixtures.DefaultPrincipal().String(),
				clientURL,
				"https://"+fakeHost+":443/callback",
				func(data map[string]any) {
					assertCIMDConsentContext(data, "https://"+fakeHost+":443/callback", fakeHost)
				},
			)
		})
	})
})
