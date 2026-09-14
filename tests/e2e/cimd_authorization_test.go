package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// cimdDocument builds a CIMD JSON document with the given clientID and redirectURIs.
func cimdDocument(clientID string, redirectURIs []string) []byte {
	doc := map[string]any{
		"client_id":     clientID,
		"client_name":   "Test CIMD Agent",
		"redirect_uris": redirectURIs,
	}
	b, _ := json.Marshal(doc)
	return b
}

type countingCIMDFetcher struct {
	calls atomic.Int32
}

func (f *countingCIMDFetcher) Fetch(_ context.Context, rawURL string) (*ports.CIMDFetchResult, error) {
	f.calls.Add(1)
	return nil, fmt.Errorf("unexpected CIMD fetch for %s", rawURL)
}

func (f *countingCIMDFetcher) CallCount() int {
	return int(f.calls.Load())
}

var _ = Describe("CIMD Authorization", func() {
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

	// Scenario US1.5 from specs/028-cimd-support/spec.md
	Describe("when CIMD is disabled", func() {
		It("rejects URL-format client_id with invalid_client without attempting a fetch", func() {
			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
			// CIMD disabled by default (Enabled: false)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			appInstance, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://agent.example.com/client&redirect_uri=https://agent.example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Scenario US1.1 from specs/028-cimd-support/spec.md
	Describe("when CIMD is enabled and the document is valid", func() {
		var (
			cimdServer  *httptest.Server
			agent       *storage.Agent
			clientURL   string
			redirectURI string
			server      *bootstrap.TestServer
		)

		BeforeEach(func() {
			// Use a fake public hostname to satisfy validateClientURI (port 443 or absent required).
			// The custom HTTP client redirects connections to this hostname to the test server.
			const fakeHost = "cimd-e2e.test.invalid"

			// Start TLS mock CIMD server — URL known only after server starts.
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "max-age=300")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
			}))

			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			// Register agent with pre-registered CIMD client URI (FR-026).
			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "CIMD Test Agent",
				Description:    "E2E test agent for CIMD authorization",
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

			// Wire CIMD-enabled config with custom test client that redirects fakeHost → test server.
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)

			cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
			Expect(err).ToNot(HaveOccurred())

			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
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

		It("resolves the agent, redirects to the consent page, and embeds an opaque session_token", func() {
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
					clientURL, redirectURI,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusFound))

			loc, parseErr := url.Parse(resp.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			// Consent redirect must land on the correct agent's page.
			Expect(loc.Path).To(ContainSubstring("/agents/" + agent.ID.String()))
			// SR-013: full authorization context must be sealed in an opaque JWE session_token.
			Expect(loc.Query().Get("session_token")).ToNot(BeEmpty(),
				"consent redirect must carry an opaque session_token per SR-013")
		})
	})

	// Scenario US1.2 from specs/028-cimd-support/spec.md
	Describe("when CIMD document client_id does not match the request URL", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-mismatch.test.invalid"

			// CIMD server returns a document with a DIFFERENT client_id.
			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(cimdDocument("https://other.example.com/different", []string{clientURL + "/callback"}))
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Mismatch Agent",
				Description:    "E2E test agent for CIMD mismatch scenario",
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

		It("rejects the request with invalid_client", func() {
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Scenario US1.3 from specs/028-cimd-support/spec.md
	Describe("when the CIMD endpoint returns a non-200 status", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-404.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Missing CIMD Agent",
				Description:    "E2E test agent for CIMD 404 scenario",
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

		It("rejects the authorization request with invalid_client", func() {
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Scenario US1.4 from specs/028-cimd-support/spec.md
	Describe("when redirect_uri is not listed in the CIMD document", func() {
		var (
			cimdServer *httptest.Server
			server     *bootstrap.TestServer
			clientURL  string
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-redirect.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Document only allows /registered-callback, not /other-callback.
				_, _ = w.Write(cimdDocument(clientURL, []string{clientURL + "/registered-callback"}))
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Redirect Check Agent",
				Description:    "E2E test agent for CIMD redirect URI validation",
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

		It("rejects the request with invalid_request", func() {
			// Use a redirect URI that is NOT registered in the CIMD document.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/other-callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_request"))
		})
	})

	// FR-027 from specs/028-cimd-support/spec.md
	Describe("when the client_id URL is not pre-registered on any Agent", func() {
		It("rejects the authorization request with invalid_client before any CIMD fetch", func() {
			config := fixtures.OAuth2ConfigWithCIMD(mockUpstream.Server.URL)
			serverFactory = bootstrap.NewServerFactory(config, logger)
			cimdFetcher := &countingCIMDFetcher{}
			appInstance, err := serverFactory.BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
			Expect(err).ToNot(HaveOccurred())
			server, err := bootstrap.NewEndUserTestServer(appInstance, logger)
			Expect(err).ToNot(HaveOccurred())
			defer server.Close()

			resp, err := server.AuthenticatedGET(
				"/oauth2/authorize?client_id=https://unregistered.example.com/client&redirect_uri=https://unregistered.example.com/cb&response_type=code&state=xyz",
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
			// FR-027: lookup must happen before any network call — no fetch should be attempted.
			Expect(cimdFetcher.CallCount()).To(Equal(0))
		})
	})

	// FR-022 from specs/028-cimd-support/spec.md
	Describe("when the CIMD document specifies a secret-bearing token_endpoint_auth_method", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-auth-method.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				doc := map[string]any{
					"client_id":                  clientURL,
					"client_name":                "Auth Method Agent",
					"redirect_uris":              []string{clientURL + "/callback"},
					"token_endpoint_auth_method": "client_secret_post",
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Auth Method Agent",
				Description:    "E2E test agent for non-public auth method rejection",
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

		It("rejects the authorization request with invalid_client", func() {
			// FR-022: CIMD clients are always public clients — any auth method other than
			// "none" is rejected, even if the document is otherwise structurally valid.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// Edge case from specs/028-cimd-support/spec.md: redirect_uris is mandatory (FR-004)
	Describe("when the CIMD document has an empty redirect_uris array", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-empty-redirects.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "Empty Redirects Agent",
					"redirect_uris": []string{},
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Empty Redirects Agent",
				Description:    "E2E test agent for empty redirect_uris scenario",
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

		It("rejects the authorization request with invalid_client", func() {
			// FR-004: redirect_uris is required; an empty array is treated the same as omission.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// RFC §4.1 (from specs/028-cimd-support/spec.md document validation rules)
	Describe("when the CIMD document contains a client_secret field", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-client-secret.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "Secret-Bearing Agent",
					"redirect_uris": []string{clientURL + "/callback"},
					"client_secret": "this-field-must-not-be-present",
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Secret-Bearing Agent",
				Description:    "E2E test agent for client_secret rejection",
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

		It("rejects the authorization request with invalid_client", func() {
			// RFC §4.1: client_secret MUST NOT appear in a CIMD document — presence of this
			// field is treated as a malformed document regardless of the secret's value.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// FR-004a from specs/028-cimd-support/spec.md
	Describe("when the CIMD document contains a cross-origin redirect_uri", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-cross-origin.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// redirect_uri origin differs from the client_id URL origin.
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "Cross-Origin Agent",
					"redirect_uris": []string{"https://other-domain.example.com/callback"},
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Cross-Origin Agent",
				Description:    "E2E test agent for cross-origin redirect_uri rejection",
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

		It("rejects the authorization request with invalid_client", func() {
			// FR-004a: every redirect_uri in the document must be same-origin with the
			// client_id URL; cross-origin redirect URIs make the entire document invalid.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=https://other-domain.example.com/callback&response_type=code&state=xyz",
					clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	// FR-004a localhost exception from specs/028-cimd-support/spec.md
	Describe("when the CIMD document redirect_uri is a localhost address", func() {
		var (
			cimdServer *httptest.Server
			agent      *storage.Agent
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-localhost-redir.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// localhost redirect URI is always permitted regardless of the client_id origin.
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "Localhost Redirect Agent",
					"redirect_uris": []string{"http://localhost:3000/callback"},
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Localhost Redirect Agent",
				Description:    "E2E test agent for localhost redirect URI exception",
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

		It("accepts the document and redirects to the consent page", func() {
			// FR-004a exception: localhost and 127.0.0.1 redirect URIs are unconditionally
			// permitted regardless of the client_id origin — supporting locally-running agents.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=http://localhost:3000/callback&response_type=code&state=xyz",
					clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusFound))

			loc, parseErr := url.Parse(resp.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			Expect(loc.Path).To(ContainSubstring("/agents/" + agent.ID.String()))
		})
	})

	// FR-023a from specs/028-cimd-support/spec.md
	Describe("when the CIMD document client_name differs from Agent.DisplayName", func() {
		var (
			cimdServer  *httptest.Server
			agent       *storage.Agent
			clientURL   string
			redirectURI string
			server      *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-brand-mismatch.test.invalid"

			clientURL = "https://" + fakeHost + "/client"
			redirectURI = "https://" + fakeHost + "/callback"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// client_name intentionally differs from the Agent's DisplayName.
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "Rebranded Agent Name",
					"redirect_uris": []string{redirectURI},
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			now := time.Now()
			agent = &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Official Agent Name",
				Description:    "E2E test agent for brand pin mismatch scenario",
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

		It("continues the authorization flow and redirects to the consent page", func() {
			// FR-023a: brand pin mismatch emits a structured audit log event but does NOT
			// block the flow — the authorization continues to the consent page.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz",
					clientURL, redirectURI,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusFound))

			loc, parseErr := url.Parse(resp.Header.Get("Location"))
			Expect(parseErr).ToNot(HaveOccurred())
			Expect(loc.Path).To(ContainSubstring("/agents/" + agent.ID.String()))
		})
	})

	// FR-023b from specs/028-cimd-support/spec.md
	Describe("when the CIMD document client_name matches a built-in blocked term", func() {
		var (
			cimdServer *httptest.Server
			clientURL  string
			server     *bootstrap.TestServer
		)

		BeforeEach(func() {
			const fakeHost = "cimd-e2e-blocklist.test.invalid"

			cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// "admin" is in the built-in reserved name list (FR-023c default set).
				doc := map[string]any{
					"client_id":     clientURL,
					"client_name":   "admin",
					"redirect_uris": []string{clientURL + "/callback"},
				}
				b, _ := json.Marshal(doc)
				_, _ = w.Write(b)
			}))

			clientURL = "https://" + fakeHost + "/client"

			now := time.Now()
			agent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientURIs:     []string{clientURL},
				DisplayName:    "Admin-Spoofing Agent",
				Description:    "E2E test agent for built-in client_name blocklist",
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

		It("rejects the authorization request with invalid_client", func() {
			// FR-023b/c: "admin" and other reserved terms are in the non-empty default
			// blocklist and are always enforced regardless of operator configuration.
			resp, err := server.AuthenticatedGET(
				fmt.Sprintf(
					"/oauth2/authorize?client_id=%s&redirect_uri=%s/callback&response_type=code&state=xyz",
					clientURL, clientURL,
				),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})
})
