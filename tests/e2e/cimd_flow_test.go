package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
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

var _ = Describe("CIMD Full Authorization Flow", func() {
	var (
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		testStorage    *storageadapter.Adapter
		cimdServer     *httptest.Server
		server         *bootstrap.TestServer
		agent          *storage.Agent
		clientURL      string
		redirectURI    string
	)

	const fakeHost = "cimd-e2e-flow.test.invalid"

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		clientURL = "https://" + fakeHost + "/client"
		redirectURI = "https://" + fakeHost + "/callback"

		cimdServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cimdDocument(clientURL, []string{redirectURI}))
		}))

		now := time.Now()
		agent = &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{clientURL},
			DisplayName:    "Flow Test Agent",
			Description:    "E2E test agent for full CIMD authorization flow",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

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
		if server != nil {
			server.Close()
		}
		if cimdServer != nil {
			cimdServer.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario FL-001 from specs/028-cimd-support/spec.md
	Describe("when a CIMD agent completes the full authorization flow", func() {
		It("issues an authorization code after authorize → consent → grant → re-authorize", func() {
			principal := fixtures.DefaultPrincipal().String()
			verifier := helpers.PKCEVerifier()
			challenge := helpers.GenerateCodeChallenge(verifier)
			authorizeURL := fmt.Sprintf(
				"/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=xyz123&code_challenge=%s&code_challenge_method=S256",
				clientURL, redirectURI, challenge,
			)

			// Step 1: First authorize — no grant exists; expect redirect to consent page.
			resp, err := server.AuthenticatedGET(authorizeURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			Expect(resp.StatusCode).To(Equal(http.StatusFound))

			loc := resp.Header.Get("Location")
			Expect(loc).To(ContainSubstring("/agents/"))
			Expect(loc).To(ContainSubstring(agent.ID.String()))

			consentLoc, err := url.Parse(loc)
			Expect(err).ToNot(HaveOccurred())
			sessionToken := consentLoc.Query().Get("session_token")
			Expect(sessionToken).ToNot(BeEmpty(), "authorize redirect must embed a session_token")

			// Step 2: Load consent context using the token produced by the authorize endpoint.
			// This proves the token format from the authorize handler is decodable by the consent handler.
			consentResp, err := server.AuthenticatedGET(
				fmt.Sprintf("/api/consent/agents/%s?session_token=%s", agent.ID, sessionToken),
				principal,
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = consentResp.Body.Close() }()
			Expect(consentResp.StatusCode).To(Equal(http.StatusOK))

			var consentBody map[string]any
			Expect(json.NewDecoder(consentResp.Body).Decode(&consentBody)).To(Succeed())
			data := consentBody["data"].(map[string]any)
			cimdMeta, ok := data["cimd_metadata"].(map[string]any)
			Expect(ok).To(BeTrue(), "cimd_metadata should be present")
			Expect(cimdMeta["verified_domain"]).To(Equal(fakeHost))

			grantBody, _ := json.Marshal(map[string]any{
				"granted_permission_sets": map[string][]string{
					fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
				},
			})
			grantResp, err := server.AuthenticatedPOST(
				fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", agent.ID, sessionToken),
				principal,
				"application/json",
				bytes.NewReader(grantBody),
			)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = grantResp.Body.Close() }()
			Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

			var grantRespBody map[string]any
			Expect(json.NewDecoder(grantResp.Body).Decode(&grantRespBody)).To(Succeed())
			redirectURL, ok := grantRespBody["redirect_url"].(string)
			Expect(ok).To(BeTrue(), "grant response must include redirect_url")
			Expect(redirectURL).To(ContainSubstring("/oauth2/authorize"))

			// Step 4: Re-authorize using the redirect_url embedded in the JWE by the authorize handler.
			// Grant now exists, so the broker issues an authorization code instead of redirecting to consent.
			codeResp, err := server.AuthenticatedGET(redirectURL, principal)
			Expect(err).ToNot(HaveOccurred())
			defer func() { _ = codeResp.Body.Close() }()
			Expect(codeResp.StatusCode).To(Equal(http.StatusFound))

			codeLoc := codeResp.Header.Get("Location")
			Expect(codeLoc).To(HavePrefix(redirectURI))
			Expect(codeLoc).To(ContainSubstring("code="))
			Expect(codeLoc).To(ContainSubstring("state=xyz123"))
		})
	})
})
