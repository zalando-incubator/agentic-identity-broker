// Package e2e_test provides end-to-end tests for consent state preservation
// across third-party OAuth2 login redirects.
// The consent_state envelope preserves selections and duration across provider callbacks.
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

type preservedConsentState struct {
	Selections map[string][]string `json:"selections"`
	Duration   string              `json:"duration"`
	CustomDate string              `json:"customDate"`
}

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

		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", true)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())

		// Wait for the optional PS service to appear after selecting it.
		Expect(consentPage.WaitForServiceToAppear(ctx, "Google")).To(Succeed(),
			"Google service should appear after toggling optional PS")
		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_selected_before_login")).NotTo(HaveOccurred())

		// Intercept Connect and inspect the callback's canonical draft envelope.
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

		err = consentPage.DelegateService(ctx, "Google")
		Expect(err).NotTo(HaveOccurred(), "Failed to connect Google")

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

		// Decode the envelope and verify the exact groups and service inclusions.
		decoded, err := base64.RawURLEncoding.DecodeString(psSelectionsEncoded)
		Expect(err).NotTo(HaveOccurred(), "consent_state should be valid base64url")

		var state preservedConsentState
		Expect(json.Unmarshal(decoded, &state)).To(Succeed(), "consent_state should be a valid draft envelope")
		Expect(state.Selections).To(HaveLen(2))
		Expect(state.Selections).To(HaveKey(selMandatoryPSID.String()))
		Expect(state.Selections).To(HaveKey(selOptionalPSID.String()))
		Expect(state.Selections[selMandatoryPSID.String()]).To(ConsistOf(selGitHubServiceID.String()))
		Expect(state.Selections[selOptionalPSID.String()]).To(ConsistOf(selGoogleServiceID.String(), selSlackServiceID.String()))
		Expect(state.Duration).To(Equal("30-days"))
		Expect(state.CustomDate).To(BeEmpty())

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

		expectPermissionGroupSelection(ctx, consentPage, "Code Access", true)
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		Expect(consentPage.PermissionServices(ctx, "Code Access")).To(Equal([]pages.PermissionService{
			{Name: "GitHub", Required: true, ReadOnly: true, Checked: true},
		}))
		Expect(consentPage.PermissionServices(ctx, "Productivity Suite")).To(ConsistOf(
			pages.PermissionService{Name: "Google", Checked: true},
			pages.PermissionService{Name: "Slack", Checked: true},
		))

		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_restored_from_url")).NotTo(HaveOccurred())
	})

	// Scenario 5.3 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("should restore selections and duration from a provider callback URL", func() {

		selections := map[string][]string{
			selMandatoryPSID.String(): {selGitHubServiceID.String()},
			selOptionalPSID.String():  {selGoogleServiceID.String(), selSlackServiceID.String()},
		}

		// Simulate the callback redirect URL pattern: consent page URL + success=true + service_id + consent_state
		selectionsJSON, err := json.Marshal(preservedConsentState{
			Selections: selections, Duration: "custom", CustomDate: "2099-11-06",
		})
		Expect(err).NotTo(HaveOccurred())
		encoded := base64.RawURLEncoding.EncodeToString(selectionsJSON)

		path := "/agents/" + testAgentID + "?success=true&service_id=" +
			selGitHubServiceID.String() + "&consent_state=" + url.QueryEscape(encoded)

		err = consentPage.Navigate(ctx, path)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate with callback-style URL")

		err = consentPage.WaitForPageLoad(ctx)
		Expect(err).NotTo(HaveOccurred(), "Page did not load after callback redirect")

		expectPermissionGroupSelection(ctx, consentPage, "Code Access", true)
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		Expect(consentPage.PermissionServices(ctx, "Productivity Suite")).To(ConsistOf(
			pages.PermissionService{Name: "Google", Checked: true},
			pages.PermissionService{Name: "Slack", Checked: true},
		))
		Expect(consentPage.SelectedDuration(ctx)).To(Equal("Custom date"))
		Expect(consentPage.CustomDateValue(ctx)).To(Equal("2099-11-06"))
		Expect(consentPage.GetURLQueryParam("success")).To(Equal("true"))
		Expect(consentPage.GetURLQueryParam("service_id")).To(Equal(selGitHubServiceID.String()))

		Expect(consentPage.TakeScreenshot(ctx, "selection_preservation_full_roundtrip")).NotTo(HaveOccurred())
	})
})
