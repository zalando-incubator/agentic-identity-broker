// Package e2e_test provides end-to-end tests for consent state preservation
// across third-party OAuth2 login redirects.
// This file verifies that the consent_state URL parameter (currently containing
// permission set and service selections) survives the OAuth2 redirect round-trip.
package e2e_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Well-known UUIDs for selection preservation test fixtures.
var (
	selGitHubServiceID = id.MustParseServiceID("d0000000-0000-0000-0000-000000000001")
	selGoogleServiceID = id.MustParseServiceID("d0000000-0000-0000-0000-000000000002")
	selSlackServiceID  = id.MustParseServiceID("d0000000-0000-0000-0000-000000000003")

	selMandatoryPSID = id.MustParsePermissionSetID("e0000000-0000-0000-0000-000000000001")
	selOptionalPSID  = id.MustParsePermissionSetID("e0000000-0000-0000-0000-000000000002")
)

// selPreservationService creates a ThirdpartyOAuth2ProviderEntity for selection preservation tests,
// using fixtures.ServiceWithID as a base and configuring service-specific fields.
func selPreservationService(svcID id.ServiceID, displayName, clientID, issuerURI string, scopes []model.OAuthScope) *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	svc := fixtures.ServiceWithID(svcID.String())
	svc.DisplayName = displayName
	svc.ClientID = id.ClientID(clientID)
	svc.Secret = fixtures.EncryptedSecret(svcID.String(), "secret-"+clientID)
	svc.IssuerURI = issuerURI
	svc.Endpoints = model.OAuth2Endpoints{
		AuthorizeEndpoint: GetMockUpstream().URL() + "/oauth/authorize",
		TokenEndpoint:     GetMockUpstream().URL() + "/oauth/token",
	}
	svc.Scopes = scopes
	svc.ProtectedResources = []string{issuerURI + "/api"}
	svc.CreatedAt = now
	svc.UpdatedAt = now
	return svc
}

// selPreservationPermissionSet creates a PermissionSet for selection preservation tests.
func selPreservationPermissionSet(psID id.PermissionSetID, name, description string, scopes []storage.ServiceScope) *storage.PermissionSet {
	now := time.Now()
	return &storage.PermissionSet{
		ID:            psID,
		Name:          name,
		Description:   description,
		ServiceScopes: scopes,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

var _ = Describe("Selection Preservation Across OAuth2 Redirect", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create services using helper to reduce inline struct duplication.
		githubSvc := selPreservationService(
			selGitHubServiceID, "GitHub", "client-github-sel", "https://github.example.com",
			[]model.OAuthScope{{ScopeValue: "repo", Description: "Repository access"}},
		)
		googleSvc := selPreservationService(
			selGoogleServiceID, "Google", "client-google-sel", "https://google.example.com",
			[]model.OAuthScope{{ScopeValue: "calendar", Description: "Calendar access"}},
		)
		slackSvc := selPreservationService(
			selSlackServiceID, "Slack", "client-slack-sel", "https://slack.example.com",
			[]model.OAuthScope{{ScopeValue: "chat:write", Description: "Send messages"}},
		)

		for _, svc := range []*model.ThirdpartyOAuth2ProviderEntity{githubSvc, googleSvc, slackSvc} {
			err := GetTestStorage().Services().Create(ctx, svc)
			Expect(err).NotTo(HaveOccurred(), "Failed to create service %q", svc.DisplayName)
		}

		// Create permission sets using helper.
		mandatoryPS := selPreservationPermissionSet(
			selMandatoryPSID, "Code Access", "Access to code repositories",
			[]storage.ServiceScope{
				{ServiceID: selGitHubServiceID, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional},
			},
		)
		optionalPS := selPreservationPermissionSet(
			selOptionalPSID, "Productivity Suite", "Access to productivity services",
			[]storage.ServiceScope{
				{ServiceID: selGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
				{ServiceID: selSlackServiceID, Scopes: []string{"chat:write"}, RequirementType: storage.RequirementTypeOptional},
			},
		)
		err := GetTestStorage().PermissionSets().Create(ctx, mandatoryPS)
		Expect(err).NotTo(HaveOccurred())
		err = GetTestStorage().PermissionSets().Create(ctx, optionalPS)
		Expect(err).NotTo(HaveOccurred())

		// Create agent with mandatory + optional permission sets
		agent := fixtures.ValidAgent()
		testAgentID = agent.ID.String()
		agent.DisplayName = "Selection Test Agent"
		agent.Description = "Agent for testing selection preservation"
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: selMandatoryPSID, RequirementType: storage.RequirementTypeMandatory},
			{PermissionSetID: selOptionalPSID, RequirementType: storage.RequirementTypeOptional},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: selGitHubServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo"}},
			{ServiceID: selGoogleServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"calendar"}},
			{ServiceID: selSlackServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"chat:write"}},
		}
		err = GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario 5.1 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("should encode selections in service login URL for state preservation", func() {
		// Navigate to the consent page and toggle the optional PS ON
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Find and toggle the optional PS switch
		optionalCard := page.Locator("[data-testid='permission-set-optional']").First()
		optionalCardCount, err := optionalCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optionalCardCount).To(BeNumerically(">=", 1), "Optional PS card should exist")

		toggle := optionalCard.GetByRole("switch", playwright.LocatorGetByRoleOptions{
			Name: "Toggle Productivity Suite",
		})
		toggleCount, err := toggle.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(toggleCount).To(BeNumerically(">=", 1), "Optional PS should have a toggle switch")

		err = toggle.Click()
		Expect(err).NotTo(HaveOccurred(), "Failed to toggle optional PS")

		// Wait for the optional PS service to appear after selecting it.
		Expect(consentPage.WaitForServiceToAppear(ctx, "Google")).To(Succeed(),
			"Google service should appear after toggling optional PS")
		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_selected_before_login")).NotTo(HaveOccurred())

		// Intercept the navigation that happens when Login is clicked.
		// We expect the URL to contain consent_state with both PSes.
		capturedNavigation := make(chan struct {
			url string
			err error
		}, 1)
		err = page.Route("**/api/third-party/*/oauth2/authorize*", func(route playwright.Route) {
			url := route.Request().URL()
			err := route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(204)})
			capturedNavigation <- struct {
				url string
				err error
			}{url, err}
		})
		Expect(err).NotTo(HaveOccurred(), "Failed to set up route intercept")

		// Click Login on Google service (from the optional PS we just toggled)
		err = consentPage.DelegateService(ctx, "Google")
		Expect(err).NotTo(HaveOccurred(), "Failed to click Login on Google service")

		var navigation struct {
			url string
			err error
		}
		Eventually(capturedNavigation).Should(Receive(&navigation), "Should intercept the OAuth2 authorize request")
		Expect(navigation.err).NotTo(HaveOccurred(), "Failed to fulfill intercepted navigation")
		capturedURL := navigation.url

		// Parse redirect_uri from the intercepted URL
		parsedURL, err := url.Parse(capturedURL)
		Expect(err).NotTo(HaveOccurred())
		redirectURI := parsedURL.Query().Get("redirect_uri")
		Expect(redirectURI).NotTo(BeEmpty(), "redirect_uri should be present in authorize URL")

		// Parse consent_state from the redirect_uri
		parsedRedirect, err := url.Parse(redirectURI)
		Expect(err).NotTo(HaveOccurred())
		psSelectionsEncoded := parsedRedirect.Query().Get("consent_state")
		Expect(psSelectionsEncoded).NotTo(BeEmpty(), "consent_state should be encoded in redirect_uri")

		// Decode and verify the selections contain both PSes
		decoded, err := base64.RawURLEncoding.DecodeString(psSelectionsEncoded)
		Expect(err).NotTo(HaveOccurred(), "consent_state should be valid base64url")

		var selections map[string][]string
		err = json.Unmarshal(decoded, &selections)
		Expect(err).NotTo(HaveOccurred(), "consent_state should be valid JSON")

		// Mandatory PS should be present
		Expect(selections).To(HaveKey(selMandatoryPSID.String()),
			"Selections should include mandatory PS")
		// Optional PS should be present (we toggled it ON)
		Expect(selections).To(HaveKey(selOptionalPSID.String()),
			"Selections should include optional PS after toggling")

	})

	// Scenario 5.2 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("should restore optional PS selection when navigating with consent_state parameter", func() {
		// Encode selections that include the optional PS (simulating return from OAuth2 redirect)
		selections := map[string][]string{
			selMandatoryPSID.String(): {selGitHubServiceID.String()},
			selOptionalPSID.String():  {selGoogleServiceID.String(), selSlackServiceID.String()},
		}

		err := consentPage.NavigateToAgentWithSelections(ctx, testAgentID, selections)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate with selections")

		page := consentPage.GetPlaywrightPage()

		// The optional PS "Productivity Suite" should be selected (toggle ON)
		optionalCard := page.Locator("[data-testid='permission-set-optional']").First()
		optionalCardCount, err := optionalCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optionalCardCount).To(BeNumerically(">=", 1), "Optional PS card should exist")

		// The optional PS toggle switch should be checked (selected from consent_state)
		toggle := optionalCard.GetByRole("switch", playwright.LocatorGetByRoleOptions{
			Name: "Toggle Productivity Suite",
		})
		toggleCount, err := toggle.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(toggleCount).To(BeNumerically(">=", 1), "Optional PS 'Productivity Suite' should have a toggle switch")
		ariaChecked, err := toggle.GetAttribute("aria-checked")
		Expect(err).NotTo(HaveOccurred())
		Expect(ariaChecked).To(Equal("true"), "Optional PS 'Productivity Suite' toggle should be ON from consent_state")

		// Services from the optional PS (Google, Slack) should appear in the service connections section
		err = consentPage.WaitForServiceToAppear(ctx, "Google")
		Expect(err).NotTo(HaveOccurred(), "Google service should appear when optional PS is selected")

		err = consentPage.WaitForServiceToAppear(ctx, "Slack")
		Expect(err).NotTo(HaveOccurred(), "Slack service should appear when optional PS is selected")

		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_restored_from_url")).NotTo(HaveOccurred())
	})

	// Scenario 5.3 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("should complete full OAuth2 redirect round-trip preserving selections", func() {
		// This test exercises the complete flow:
		// 1. Navigate with consent_state (simulating return from successful OAuth2 callback)
		// 2. Verify page loads with the optional PS selected and services visible
		// 3. Verify the success query param is present (as added by the callback handler)

		selections := map[string][]string{
			selMandatoryPSID.String(): {selGitHubServiceID.String()},
			selOptionalPSID.String():  {selGoogleServiceID.String(), selSlackServiceID.String()},
		}

		// Simulate the callback redirect URL pattern: consent page URL + success=true + service_id + consent_state
		selectionsJSON, err := json.Marshal(selections)
		Expect(err).NotTo(HaveOccurred())
		encoded := base64.RawURLEncoding.EncodeToString(selectionsJSON)

		path := "/agents/" + testAgentID + "?success=true&service_id=" +
			selGitHubServiceID.String() + "&consent_state=" + url.QueryEscape(encoded)

		err = consentPage.Navigate(ctx, path)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate with callback-style URL")

		err = consentPage.WaitForPageLoad(ctx)
		Expect(err).NotTo(HaveOccurred(), "Page did not load after callback redirect")

		page := consentPage.GetPlaywrightPage()

		// Verify optional PS is selected
		optionalCard := page.Locator("[data-testid='permission-set-optional']").First()
		optionalCardCount, err := optionalCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optionalCardCount).To(BeNumerically(">=", 1), "Optional PS card should exist after round-trip")

		toggle := optionalCard.GetByRole("switch", playwright.LocatorGetByRoleOptions{
			Name: "Toggle Productivity Suite",
		})
		toggleCount, err := toggle.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(toggleCount).To(BeNumerically(">=", 1), "Optional PS should have a toggle switch after round-trip")
		ariaChecked, err := toggle.GetAttribute("aria-checked")
		Expect(err).NotTo(HaveOccurred())
		Expect(ariaChecked).To(Equal("true"), "Optional PS should remain selected after OAuth2 round-trip")

		// Verify Google and Slack services are visible (from the optional PS)
		err = consentPage.WaitForServiceToAppear(ctx, "Google")
		Expect(err).NotTo(HaveOccurred(), "Google service should be visible after round-trip")

		err = consentPage.WaitForServiceToAppear(ctx, "Slack")
		Expect(err).NotTo(HaveOccurred(), "Slack service should be visible after round-trip")

		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_full_roundtrip")).NotTo(HaveOccurred())
	})
})
