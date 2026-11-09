package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

var _ = Describe("CIMD Private-Use Redirect URIs", func() {
	const (
		fakeHost  = "github-copilot-e2e.test.invalid"
		clientURL = "https://" + fakeHost + "/copilot/desktop/client-metadata.json"
	)

	var (
		logger               *slog.Logger
		storageFactory       *bootstrap.StorageFactory
		testStorage          *storageadapter.Adapter
		cimdServer           *httptest.Server
		adminServer          *bootstrap.TestServer
		server               *bootstrap.TestServer
		agent                *storage.Agent
		documentRedirectURIs []string
		agentRedirectURIs    []string
		fetches              atomic.Int32
	)
	principal := fixtures.DefaultPrincipal().String()

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		documentRedirectURIs = []string{fixtures.GitHubAppCallback, "http://127.0.0.1/", "http://127.0.0.1/callback"}
		agentRedirectURIs = []string{fixtures.GitHubAppCallback}
		fetches.Store(0)
	})

	JustBeforeEach(func() {
		cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fetches.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			_, _ = w.Write(fixtures.GitHubCopilotDesktopDocument(clientURL, documentRedirectURIs...))
		}))

		now := time.Now()
		agent = &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientURL},
			RedirectURIs:   agentRedirectURIs,
			DisplayName:    "GitHub Copilot",
			Description:    "GitHub Copilot Desktop",
			PermissionSets: fixtures.DefaultPermissionSets(),
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

		config := fixtures.OAuth2ConfigWithCIMD("")
		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdServer, fakeHost, 5120)
		Expect(err).ToNot(HaveOccurred())
		app, err := bootstrap.NewServerFactory(config, logger).BuildAppWithCIMDFetcher(testStorage, cimdFetcher)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).To(Succeed())
		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if adminServer != nil {
			adminServer.Close()
		}
		if cimdServer != nil {
			cimdServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("when the Agent allowlists github-app://oauth/callback", func() {
		// Scenario US7.1 from specs/028-cimd-support/spec.md
		It("accepts the published document and redirects to consent", func() {
			resp, err := server.AuthenticatedGET(authorizeURL(clientURL, fixtures.GitHubAppCallback), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusFound))
			location, err := url.Parse(resp.Header.Get("Location"))
			Expect(err).ToNot(HaveOccurred())
			Expect(location.Path).To(ContainSubstring("/agents/" + agent.ID.String()))
			Expect(location.Query().Get("session_token")).ToNot(BeEmpty())
		})

		// Scenario US7.3 from specs/028-cimd-support/spec.md
		It("rejects every look-alike callback without redirecting", func() {
			for _, callback := range []string{
				"GITHUB-APP://oauth/callback",
				"github-app://OAUTH/callback",
				"github-app://oauth/callback/extra",
				"github-app://oauth/callback?x=1",
				"github-app://evil/callback",
			} {
				resp, err := server.AuthenticatedGET(authorizeURL(clientURL, callback), principal)
				Expect(err).ToNot(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), callback)
				Expect(resp.Header.Get("Location")).To(BeEmpty(), callback)
				Expect(resp).To(matchers.HaveOAuth2Error("invalid_request"), callback)
				Expect(resp.Body.Close()).To(Succeed())
			}
		})

		// Scenario US7.5 from specs/028-cimd-support/spec.md
		It("completes consent, code issuance, and public-client token exchange", func() {
			code, verifier := completeAuthorizationCodeFlow(server, testStorage, principal, clientURL, fixtures.GitHubAppCallback, func(data map[string]any) {
				assertCIMDConsentContext(data, fixtures.GitHubAppCallback, fakeHost)
			})
			resp := exchangeAuthorizationCode(server, clientURL, code, fixtures.GitHubAppCallback, verifier)
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			var token map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&token)).To(Succeed())
			Expect(token["token_type"]).To(Equal("Bearer"))
			Expect(token["access_token"]).ToNot(BeEmpty())
		})

		// Scenario US7.6 from specs/028-cimd-support/spec.md
		It("rejects exchange of a native-app code for a loopback callback", func() {
			code, verifier := completeAuthorizationCodeFlow(server, testStorage, principal, clientURL, fixtures.GitHubAppCallback, nil)
			resp := exchangeAuthorizationCode(server, clientURL, code, "http://127.0.0.1/callback", verifier)
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_grant"))
		})

		// Scenario US7.7 from specs/028-cimd-support/spec.md
		It("rejects exchange with a mismatched PKCE verifier", func() {
			code, verifier := completeAuthorizationCodeFlow(server, testStorage, principal, clientURL, fixtures.GitHubAppCallback, nil)
			wrongVerifier := helpers.PKCEVerifier()
			Expect(wrongVerifier).ToNot(Equal(verifier))
			resp := exchangeAuthorizationCode(server, clientURL, code, fixtures.GitHubAppCallback, wrongVerifier)
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_grant"))
		})

		// Scenario US7.8 from specs/028-cimd-support/spec.md
		It("rechecks the Agent allowlist on a cached document after an Admin update", func() {
			first, err := server.AuthenticatedGET(authorizeURL(clientURL, fixtures.GitHubAppCallback), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = first.Body.Close() }()
			Expect(first.StatusCode).To(Equal(http.StatusFound))
			Expect(fetches.Load()).To(Equal(int32(1)))

			Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(principal))).To(Succeed())
			body, err := json.Marshal(map[string]any{
				"display_name":  agent.DisplayName,
				"description":   agent.Description,
				"client_uris":   []string{clientURL},
				"redirect_uris": []string{},
				"permission_sets": []map[string]string{{
					"permission_set_id": fixtures.PlaceholderPermissionSetID.String(),
					"requirement_type":  "mandatory",
				}},
			})
			Expect(err).ToNot(HaveOccurred())
			updated, err := adminServer.DirectRequest("PUT", "/api/agents/"+agent.ID.String(), "admin@example.com", map[string]string{"Content-Type": "application/json"}, bytes.NewReader(body))
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = updated.Body.Close() }()
			Expect(updated.StatusCode).To(Equal(http.StatusOK))

			second, err := server.AuthenticatedGET(authorizeURL(clientURL, fixtures.GitHubAppCallback), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = second.Body.Close() }()
			Expect(second.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(second.Header.Get("Location")).To(BeEmpty())
			Expect(second).To(matchers.HaveOAuth2Error("invalid_client"))
			Expect(fetches.Load()).To(Equal(int32(1)))
		})
	})

	Context("when the Agent does not allowlist the callback", func() {
		BeforeEach(func() { agentRedirectURIs = nil })

		// Scenario US7.2 from specs/028-cimd-support/spec.md
		It("rejects the entire document even for its loopback callback", func() {
			resp, err := server.AuthenticatedGET(authorizeURL(clientURL, "http://127.0.0.1/callback"), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
		})
	})

	Context("when only the Agent lists the callback", func() {
		BeforeEach(func() { documentRedirectURIs = []string{"http://127.0.0.1/callback"} })

		// Scenario US7.4 from specs/028-cimd-support/spec.md
		It("rejects a native callback absent from the document", func() {
			resp, err := server.AuthenticatedGET(authorizeURL(clientURL, fixtures.GitHubAppCallback), principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(resp.Header.Get("Location")).To(BeEmpty())
			Expect(resp).To(matchers.HaveOAuth2Error("invalid_request"))
		})
	})
})
