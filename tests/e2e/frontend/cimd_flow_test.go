package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
)

var _ = Describe("CIMD Full Browser Authorization Flow", func() {
	var (
		cimdDocServer  *httptest.Server
		cimdAppServer  *bootstrap.TestServer
		callbackServer *httptest.Server
		consentPage    *pages.ConsentPage
		cimdClientURL  string
		cimdServerURL  string
		redirectURI    string
	)

	const fakeHost = "cimd-browser-flow.test.invalid"

	BeforeEach(func() {
		ctx := context.Background()

		// 1. Capture server: the broker redirects the browser here after grant approval.
		callbackServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		redirectURI = callbackServer.URL + "/callback"
		cimdClientURL = "https://" + fakeHost + "/client"

		// 2. Mock CIMD document server (TLS; the broker fetches from this).
		cimdDocServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			_, _ = fmt.Fprintf(w, `{"client_id":%q,"client_name":"CIMD Browser Agent","redirect_uris":[%q]}`,
				cimdClientURL, redirectURI)
		}))

		// 3. Register the CIMD agent in test storage.
		now := time.Now()
		cimdAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{cimdClientURL},
			DisplayName:    "CIMD Browser Flow Agent",
			Description:    "Agent for full browser CIMD authorization flow test",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(GetTestStorage().Agents().Create(ctx, cimdAgent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(ctx, GetTestStorage(), id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		// 4. Build a CIMD-enabled app server with URL alignment so the OAuth2 service's
		// consent redirect URL matches the actual httptest server port.
		// testCtx.Server (the default non-CIMD server) is intentionally left untouched.
		config := fixtures.OAuth2ConfigWithCIMD(GetMockUpstream().Server.URL)
		sf := bootstrap.NewServerFactory(config, GetLogger())
		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdDocServer, fakeHost, 5120)
		Expect(err).NotTo(HaveOccurred())
		cimdAppServer, err = bootstrap.NewCIMDEndUserTestServer(GetTestStorage(), sf, cimdFetcher, GetLogger())
		Expect(err).NotTo(HaveOccurred())
		cimdServerURL = cimdAppServer.BaseURL()

		consentPage = pages.NewConsentPage(GetTestPage(), cimdServerURL)
	})

	AfterEach(func() {
		if cimdAppServer != nil {
			cimdAppServer.Close()
			cimdAppServer = nil
		}
		if cimdDocServer != nil {
			cimdDocServer.Close()
			cimdDocServer = nil
		}
		if callbackServer != nil {
			callbackServer.Close()
			callbackServer = nil
		}
	})

	// Scenario US6.2 from specs/028-cimd-support/spec.md
	It("navigates from authorize to consent UI, approves grant, and receives authorization code", func() {
		ctx := context.Background()
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		authorizeURL := fmt.Sprintf(
			"%s/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=fl002&code_challenge=%s&code_challenge_method=S256",
			cimdServerURL,
			url.QueryEscape(cimdClientURL),
			url.QueryEscape(redirectURI),
			challenge,
		)

		// Step 1: Navigate browser to the authorize endpoint.
		// The broker resolves the CIMD client, fetches the document, and issues a
		// 302 redirect to /agents/{id}?session_token=... .
		_, err := GetTestPage().Goto(authorizeURL)
		Expect(err).NotTo(HaveOccurred())

		// Step 2: Wait for the browser to land on the consent page.
		err = consentPage.WaitForURL(ctx, "/agents/")
		Expect(err).NotTo(HaveOccurred(), "browser should be redirected to consent page")

		// Wait for React to finish rendering before asserting UI components.
		err = consentPage.WaitForPageLoad(ctx)
		Expect(err).NotTo(HaveOccurred(), "consent page should fully render")

		// Step 3: Assert CIMD consent UI components are rendered.
		visible, err := consentPage.IsCIMDSummaryVisible(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue(), "CIMD consent summary should be visible")

		hasBadge, err := consentPage.HasCIMDDomainBadge(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasBadge).To(BeTrue(), "verified domain badge should be visible")

		hasDomain, err := consentPage.HasCIMDDomainText(ctx, fakeHost)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasDomain).To(BeTrue(), "domain badge should show fakeHost")

		// localhost redirect_uri (callbackServer) triggers the warning.
		hasWarning, err := consentPage.HasCIMDLocalhostWarning(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasWarning).To(BeTrue(), "localhost redirect warning should be visible for 127.0.0.1 callback")

		// Step 4: Click "Approve & Delegate".
		err = consentPage.SubmitConsent(ctx)
		Expect(err).NotTo(HaveOccurred(), "approve & delegate should succeed")

		// Step 5: Wait for the browser to arrive at the callback URL.
		// Flow: frontend POST /api/consent/agent/{id}/grants?session_token=... →
		//       response {redirect_url: "/oauth2/authorize?..."} →
		//       window.location.href = redirect_url →
		//       broker 302 → callbackServer.URL/callback?code=...&state=...
		err = consentPage.WaitForURL(ctx, callbackServer.URL)
		Expect(err).NotTo(HaveOccurred(), "browser should reach callback server after grant approval")

		// Step 6: Assert the authorization code and state are present in the final URL.
		finalURL := GetTestPage().URL()
		Expect(finalURL).To(ContainSubstring("code="), "authorization code must be present")
		Expect(finalURL).To(ContainSubstring("state=fl002"), "original state must be preserved")
	})
})

var _ = Describe("CIMD Loopback Warning Browser Flow", func() {
	var (
		cimdDocServer  *httptest.Server
		cimdAppServer  *bootstrap.TestServer
		callbackServer *httptest.Server
		consentPage    *pages.ConsentPage
		cimdClientURL  string
		cimdServerURL  string
		runtimeURI     string
	)

	const fakeHost = "cimd-loopback-warning.test.invalid"
	const registeredLoopbackURI = "http://127.0.0.1:3000/callback"

	BeforeEach(func() {
		ctx := context.Background()

		callbackServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		runtimeURI = callbackServer.URL + "/callback"
		cimdClientURL = "https://" + fakeHost + "/client"

		cimdDocServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			_, _ = fmt.Fprintf(w, `{"client_id":%q,"client_name":"CIMD Loopback Warning Agent","redirect_uris":[%q]}`,
				cimdClientURL, registeredLoopbackURI)
		}))

		now := time.Now()
		cimdAgent := &storage.Agent{
			ID:             id.NewAgentID(),
			ClientURIs:     []string{cimdClientURL},
			DisplayName:    "CIMD Loopback Warning Agent",
			Description:    "Agent for SC-005 loopback warning browser flow test",
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		Expect(GetTestStorage().Agents().Create(ctx, cimdAgent)).To(Succeed())
		Expect(fixtures.SeedDefaultConsentData(ctx, GetTestStorage(), id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		config := fixtures.OAuth2ConfigWithCIMD(GetMockUpstream().Server.URL)
		sf := bootstrap.NewServerFactory(config, GetLogger())
		cimdFetcher, err := bootstrap.NewCIMDTestFetcher(cimdDocServer, fakeHost, 5120)
		Expect(err).NotTo(HaveOccurred())
		cimdAppServer, err = bootstrap.NewCIMDEndUserTestServer(GetTestStorage(), sf, cimdFetcher, GetLogger())
		Expect(err).NotTo(HaveOccurred())
		cimdServerURL = cimdAppServer.BaseURL()

		consentPage = pages.NewConsentPage(GetTestPage(), cimdServerURL)
	})

	AfterEach(func() {
		if cimdAppServer != nil {
			cimdAppServer.Close()
			cimdAppServer = nil
		}
		if cimdDocServer != nil {
			cimdDocServer.Close()
			cimdDocServer = nil
		}
		if callbackServer != nil {
			callbackServer.Close()
			callbackServer = nil
		}
	})

	// SC-005 from specs/028b-portless-registration/spec.md
	It("shows the localhost warning when authorize uses a different runtime loopback port", func() {
		ctx := context.Background()
		verifier := helpers.PKCEVerifier()
		challenge := helpers.GenerateCodeChallenge(verifier)

		authorizeURL := fmt.Sprintf(
			"%s/oauth2/authorize?client_id=%s&redirect_uri=%s&response_type=code&state=sc005&code_challenge=%s&code_challenge_method=S256",
			cimdServerURL,
			url.QueryEscape(cimdClientURL),
			url.QueryEscape(runtimeURI),
			challenge,
		)

		_, err := GetTestPage().Goto(authorizeURL)
		Expect(err).NotTo(HaveOccurred())

		err = consentPage.WaitForURL(ctx, "/agents/")
		Expect(err).NotTo(HaveOccurred(), "browser should reach the consent page when explicit-port registration matches a different runtime loopback port")

		err = consentPage.WaitForPageLoad(ctx)
		Expect(err).NotTo(HaveOccurred(), "consent page should fully render")

		hasWarning, err := consentPage.HasCIMDLocalhostWarning(ctx)
		Expect(err).NotTo(HaveOccurred(), "failed to check localhost warning visibility")
		Expect(hasWarning).To(BeTrue(), "localhost warning should be visible when registered and runtime loopback ports differ")

		err = consentPage.TakeScreenshot(ctx, "cimd_loopback_warning_explicit_port")
		Expect(err).NotTo(HaveOccurred(), "failed to save SC-005 browser-flow screenshot")
	})
})
