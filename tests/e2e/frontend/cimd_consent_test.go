// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file covers the CIMD identity, loopback warning, and technical details in the decision view.
package e2e_test

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newCIMDSessionToken builds a JWE authorization session token for use in CIMD consent UI tests.
func newCIMDSessionToken(agentID id.AgentID, redirectURI string, meta *ports.SessionCIMDMetadata) string {
	originalURL := "https://cimd-example.com/authorize?client_id=https://cimd-example.com/client_metadata.json&redirect_uri=" + redirectURI + "&scope=read"
	claims, err := sessiontoken.NewAuthorizationSessionClaims(agentID, id.Principal("user@example.com"), originalURL, meta)
	Expect(err).NotTo(HaveOccurred(), "Failed to create authorization session claims")
	token, err := GetTestServer().App().SessionTokenService.Create(claims)
	Expect(err).NotTo(HaveOccurred(), "Failed to create authorization session token")
	return token
}

// CIMD consent tests keep identity and warning behavior when authorization-session
// metadata is rendered by the decision view (URL-based client_id flow).
var _ = Describe("CIMD Consent UI", func() {
	var (
		ctx          context.Context
		consentPage  *pages.ConsentPage
		cimdAgent    *storage.Agent
		sessionToken string
	)

	BeforeEach(func() {
		ctx = context.Background()

		now := time.Now()
		cimdAgent = &storage.Agent{
			ID:             id.NewAgentID(),
			DisplayName:    "CIMD Test Agent",
			Description:    "Test agent for CIMD consent UI scenarios",
			ClientURIs:     []string{"https://cimd-example.com/client_metadata.json"},
			RedirectURIs:   []string{"https://cimd-example.com/callback"},
			CreatedAt:      now,
			UpdatedAt:      now,
			PermissionSets: fixtures.DefaultPermissionSets(),
		}
		err := GetTestStorage().Agents().Create(ctx, cimdAgent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create CIMD test agent")
		Expect(fixtures.SeedDefaultConsentData(ctx, GetTestStorage(), id.Principal(fixtures.DefaultPrincipal().String()))).To(Succeed())

		sessionToken = newCIMDSessionToken(
			cimdAgent.ID,
			"https://cimd-example.com/callback",
			&ports.SessionCIMDMetadata{
				ClientID:     "https://cimd-example.com/client_metadata.json",
				ClientName:   "CIMD Test Client",
				RedirectURIs: []string{"https://cimd-example.com/callback"},
			},
		)

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	// CS-001 from specs/028-cimd-support/spec.md: plain-language access request.
	It("should identify the CIMD agent and explain its requested access", func() {
		// The focused decision card identifies who is asking and what the agent can do.
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		visible, err := consentPage.IsCIMDSummaryVisible(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check CIMD summary visibility")
		Expect(visible).To(BeTrue(), "The plain-language access request should be visible")

		hasName, err := consentPage.HasCIMDClientName(ctx, "CIMD Test Agent")
		Expect(err).NotTo(HaveOccurred(), "Failed to check agent display name")
		Expect(hasName).To(BeTrue(), "The agent display name should appear in the decision heading")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs001_consent_summary")
		Expect(err).NotTo(HaveOccurred())
	})

	// CS-002 from specs/028-cimd-support/spec.md: verified-domain origin label.
	It("should show the verified domain for the CIMD client_id URL", func() {
		// The origin label comes only from validated authorization-session metadata.
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		visible, err := consentPage.HasCIMDDomainBadge(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check domain badge visibility")
		Expect(visible).To(BeTrue(), "The verified-domain Agent Origin Label should be visible")

		hasDomain, err := consentPage.HasCIMDDomainText(ctx, "cimd-example.com")
		Expect(err).NotTo(HaveOccurred(), "Failed to check domain text")
		Expect(hasDomain).To(BeTrue(), "Seeded domain 'cimd-example.com' should appear in the verified domain badge")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs002_domain_badge")
		Expect(err).NotTo(HaveOccurred())
	})

	// CS-003 from specs/028-cimd-support/spec.md: prominent localhost redirect warning.
	Context("when the authorization session redirect_uri points to localhost", func() {
		var localhostSessionToken string

		BeforeEach(func() {
			localhostSessionToken = newCIMDSessionToken(
				cimdAgent.ID,
				"http://localhost:8080/callback",
				&ports.SessionCIMDMetadata{
					ClientID:     "https://cimd-example.com/client_metadata.json",
					ClientName:   "CIMD Test Client",
					RedirectURIs: []string{"http://localhost:8080/callback"},
				},
			)
		})

		It("should show a localhost redirect warning alert", func() {
			// The localhost risk callout keeps the redirect destination prominent.
			err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), localhostSessionToken)
			Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page with localhost session")

			has, err := consentPage.HasCIMDLocalhostWarning(ctx)
			Expect(err).NotTo(HaveOccurred(), "Failed to check localhost warning visibility")
			Expect(has).To(BeTrue(), "The loopback warning should be visible for localhost redirect_uri")

			warningText, err := consentPage.GetCIMDLocalhostWarningText(ctx)
			Expect(err).NotTo(HaveOccurred(), "Failed to get localhost warning text")
			Expect(warningText).To(ContainSubstring("own computer"), "Warning must explain the local redirect before granting access")

			err = consentPage.TakeScreenshot(ctx, "cimd_cs003_localhost_warning")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// CS-004 from specs/028-cimd-support/spec.md: technical authorization details stay available on demand.
	It("should show client ID, redirect URI, and requested scopes in Technical details", func() {
		// The dialog keeps raw authorization data one deliberate click from the decision.
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		err = consentPage.OpenTechnicalDetails(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to open Technical details")

		open, err := consentPage.IsTechnicalDetailsOpen(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check technical details dialog")
		Expect(open).To(BeTrue(), "Technical details dialog should display the existing authorization fields")

		hasRedirectURI, err := consentPage.HasCIMDRedirectURIInDetails(ctx, "https://cimd-example.com/callback")
		Expect(err).NotTo(HaveOccurred(), "Failed to check redirect URI in Technical details")
		Expect(hasRedirectURI).To(BeTrue(), "The requested redirect URI must appear in Technical details")

		hasScope, err := consentPage.HasCIMDScopeInDetails(ctx, "read")
		Expect(err).NotTo(HaveOccurred(), "Failed to check requested scope in Technical details")
		Expect(hasScope).To(BeTrue(), "Requested scope 'read' must appear in Technical details")

		hasClientID, err := consentPage.HasCIMDClientIDInDetails(ctx, "https://cimd-example.com/client_metadata.json")
		Expect(err).NotTo(HaveOccurred(), "Failed to check client ID in Technical details")
		Expect(hasClientID).To(BeTrue(), "The existing CIMD client ID must appear in Technical details")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs004_technical_details")
		Expect(err).NotTo(HaveOccurred())
	})
})
