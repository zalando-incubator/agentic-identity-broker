package e2e_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

// SR-002 Mode Boundary Enforcement E2E Tests
//
// Maps to FR-006, FR-007, and SR-002 from specs/030-hybrid-oauth-modes/spec.md.
// SR-002 requires that mode strategy boundaries are enforced at the HTTP layer —
// a builder wiring regression (wrong strategy injected for a mode) must produce a
// detectable HTTP error response, not silent misrouting.
//
// FR-005 CIMD UUID Rejection E2E Test
//
// Maps to scenario 4 (US2) from specs/030-hybrid-oauth-modes/spec.md.

var _ = Describe("SR-002: Mode Boundary Enforcement", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)
	})

	// FR-006: proxy mode rejects LocalClient agents (spec scenario US2.7)
	Describe("proxy mode", func() {
		var (
			mockUpstream *helpers.MockUpstreamOAuth2Server
			testStorage  *storageadapter.Adapter
			server       *bootstrap.TestServer
		)

		BeforeEach(func() {
			mockUpstream = helpers.NewMockUpstreamOAuth2Server()
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if mockUpstream != nil {
				mockUpstream.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// Scenario US2.7 from specs/030-hybrid-oauth-modes/spec.md
		It("rejects LocalClient agent with unauthorized_client (FR-006, SR-002)", func() {
			now := time.Now()
			localAgent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Local Agent in Proxy Mode",
				Description:    "agent with no ClientID — classified as LocalClient",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), localAgent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", localAgent.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("unauthorized_client"))
		})

		// Token endpoint must also enforce mode boundary (SR-002 — covers wiring regression
		// where wrong OAuth2Service is injected into OAuth2TokenHandler).
		It("rejects LocalClient agent at token endpoint with unauthorized_client (SR-002)", func() {
			now := time.Now()
			localAgent := &storage.Agent{
				ID:             id.NewAgentID(),
				DisplayName:    "Local Agent at Token Endpoint",
				Description:    "agent with no ClientID — classified as LocalClient",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), localAgent)).To(Succeed())

			formData := url.Values{
				"grant_type": {"authorization_code"},
				"client_id":  {localAgent.ID.String()},
				"code":       {"fake-code"},
			}
			resp, err := server.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(formData.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("unauthorized_client"))
		})
	})

	// FR-007: local mode rejects ProxyClient agents (spec scenario US2.8)
	Describe("local mode", func() {
		var (
			testStorage *storageadapter.Adapter
			server      *bootstrap.TestServer
		)

		BeforeEach(func() {
			var err error
			testStorage, err = storageFactory.NewTestStorage()
			Expect(err).ToNot(HaveOccurred())

			config := fixtures.LocalConfig()
			serverFactory := bootstrap.NewServerFactory(config, logger)
			app, err := serverFactory.BuildApp(testStorage)
			Expect(err).ToNot(HaveOccurred())

			server, err = bootstrap.NewEndUserTestServer(app, logger)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			if server != nil {
				server.Close()
			}
			if testStorage != nil {
				_ = storageFactory.CloseStorage(testStorage)
			}
		})

		// Scenario US2.8 from specs/030-hybrid-oauth-modes/spec.md
		It("rejects ProxyClient agent with unauthorized_client (FR-007, SR-002)", func() {
			now := time.Now()
			proxyAgent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientID:       ptr.To(id.ClientID("upstream-client-xyz")),
				DisplayName:    "Proxy Agent in Local Mode",
				Description:    "agent with ClientID — classified as ProxyClient",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), proxyAgent)).To(Succeed())

			resp, err := server.AuthenticatedGET(
				fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&state=xyz", proxyAgent.ID.String()),
				fixtures.DefaultPrincipal().String(),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("unauthorized_client"))
		})

		// Token endpoint mode boundary enforcement (SR-002).
		It("rejects ProxyClient agent at token endpoint with unauthorized_client (SR-002)", func() {
			now := time.Now()
			proxyAgent := &storage.Agent{
				ID:             id.NewAgentID(),
				ClientID:       ptr.To(id.ClientID("upstream-client-token")),
				DisplayName:    "Proxy Agent at Token Endpoint",
				Description:    "agent with ClientID — classified as ProxyClient",
				RedirectURIs:   []string{"https://example.com/cb"},
				CreatedAt:      now,
				UpdatedAt:      now,
				PermissionSets: fixtures.DefaultPermissionSets(),
			}
			Expect(testStorage.Agents().Create(context.Background(), proxyAgent)).To(Succeed())

			formData := url.Values{
				"grant_type": {"authorization_code"},
				"client_id":  {proxyAgent.ID.String()},
				"code":       {"fake-code"},
			}
			resp, err := server.PublicPOST(
				"/oauth2/token",
				"application/x-www-form-urlencoded",
				strings.NewReader(formData.Encode()),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(resp).To(matchers.HaveOAuth2Error("unauthorized_client"))
		})
	})
})

// FR-005 CIMD UUID Rejection E2E Test
//
// Maps to scenario 4 (US2) from specs/030-hybrid-oauth-modes/spec.md.
// CIMD agents must be addressed via their URL-format client_id, never by their internal UUID.
// UUID-format resolution that finds a CIMD agent must be rejected before any CIMD fetch occurs.
var _ = Describe("FR-005: CIMD Agent UUID Rejection", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		cimdAgent      *storage.Agent
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		now := time.Now()
		cimdAgent = &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{"https://cimd-fr005.test.invalid/client"},
			DisplayName:    "CIMD Agent",
			Description:    "agent with client_uris — classified as CIMDClient",
			RedirectURIs:   []string{"https://cimd-fr005.test.invalid/callback"},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), cimdAgent)).To(Succeed())

		config := fixtures.OAuth2ConfigWithCIMD("")
		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 4 (US2) from specs/030-hybrid-oauth-modes/spec.md
	It("rejects UUID-format client_id that resolves to a CIMD agent (FR-005)", func() {
		resp, err := server.AuthenticatedGET(
			fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://cimd-fr005.test.invalid/callback&response_type=code&state=xyz",
				cimdAgent.ID.String()),
			fixtures.DefaultPrincipal().String(),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()

		Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
		Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
	})
})
