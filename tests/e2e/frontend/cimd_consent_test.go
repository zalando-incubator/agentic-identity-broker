// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the CIMD (Client ID Metadata Document) consent UI components.
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

// CIMDConsentUI tests verify the CIMD consent screen components rendered when an
// authorization session carries CIMD metadata (URL-based client_id flow).
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

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	// CS-001: CIMDConsentSummary component — "The application … wants to access …"
	It("should display CIMD consent summary with client name and access target", func() {
		// CS-001 from specs/028-cimd-support/spec.md — CIMDConsentSummary component
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		visible, err := consentPage.IsCIMDSummaryVisible(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check CIMD summary visibility")
		Expect(visible).To(BeTrue(), "CIMDConsentSummary 'wants to access' text should be visible")

		hasName, err := consentPage.HasCIMDClientName(ctx, "CIMD Test Agent")
		Expect(err).NotTo(HaveOccurred(), "Failed to check agent display name")
		Expect(hasName).To(BeTrue(), "Agent display name 'CIMD Test Agent' should appear in the consent summary")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs001_consent_summary")
		Expect(err).NotTo(HaveOccurred())
	})

	// CS-002: CIMDDomainBadge component — "Verified domain: …"
	It("should display the verified domain badge for the CIMD client_id URL", func() {
		// CS-002 from specs/028-cimd-support/spec.md — CIMDDomainBadge component
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		visible, err := consentPage.HasCIMDDomainBadge(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check domain badge visibility")
		Expect(visible).To(BeTrue(), "CIMDDomainBadge 'Verified domain:' text should be visible")

		hasDomain, err := consentPage.HasCIMDDomainText(ctx, "cimd-example.com")
		Expect(err).NotTo(HaveOccurred(), "Failed to check domain text")
		Expect(hasDomain).To(BeTrue(), "Seeded domain 'cimd-example.com' should appear in the verified domain badge")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs002_domain_badge")
		Expect(err).NotTo(HaveOccurred())
	})

	// CS-003: CIMDLocalhostWarning component — role="alert" for localhost redirect URIs
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
			// CS-003 from specs/028-cimd-support/spec.md — CIMDLocalhostWarning component
			err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), localhostSessionToken)
			Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page with localhost session")

			has, err := consentPage.HasCIMDLocalhostWarning(ctx)
			Expect(err).NotTo(HaveOccurred(), "Failed to check localhost warning visibility")
			Expect(has).To(BeTrue(), "CIMDLocalhostWarning role=alert element should be visible for localhost redirect_uri")

			warningText, err := consentPage.GetCIMDLocalhostWarningText(ctx)
			Expect(err).NotTo(HaveOccurred(), "Failed to get localhost warning text")
			Expect(warningText).To(ContainSubstring("local machine"), "Warning alert should mention local machine")

			err = consentPage.TakeScreenshot(ctx, "cimd_cs003_localhost_warning")
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// CS-004: CIMDAdvancedDetails component — expandable details panel
	It("should expand CIMD advanced details when the user clicks the disclosure button", func() {
		// CS-004 from specs/028-cimd-support/spec.md — CIMDAdvancedDetails component
		err := consentPage.NavigateToAgentWithSessionToken(ctx, cimdAgent.ID.String(), sessionToken)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to CIMD consent page")

		err = consentPage.ClickCIMDAdvancedDetails(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Advanced Details button")

		expanded, err := consentPage.IsCIMDAdvancedDetailsExpanded(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check advanced details panel state")
		Expect(expanded).To(BeTrue(), "Advanced details panel should show Client ID after clicking the button")

		hasRedirectURI, err := consentPage.HasCIMDRedirectURIInDetails(ctx, "https://cimd-example.com/callback")
		Expect(err).NotTo(HaveOccurred(), "Failed to check redirect URI in advanced details")
		Expect(hasRedirectURI).To(BeTrue(), "Seeded redirect_uri 'https://cimd-example.com/callback' should appear in the expanded advanced details panel")

		hasScope, err := consentPage.HasCIMDScopeInDetails(ctx, "read")
		Expect(err).NotTo(HaveOccurred(), "Failed to check requested scope in advanced details")
		Expect(hasScope).To(BeTrue(), "Requested scope 'read' should appear in the expanded advanced details panel")

		hasClientID, err := consentPage.HasCIMDClientIDInDetails(ctx, "https://cimd-example.com/client_metadata.json")
		Expect(err).NotTo(HaveOccurred(), "Failed to check client ID in advanced details")
		Expect(hasClientID).To(BeTrue(), "Seeded client_id URL 'https://cimd-example.com/client_metadata.json' should appear in the expanded advanced details panel")

		err = consentPage.TakeScreenshot(ctx, "cimd_cs004_advanced_details_expanded")
		Expect(err).NotTo(HaveOccurred())
	})
})
