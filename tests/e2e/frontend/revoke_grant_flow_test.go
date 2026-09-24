// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the revoke agent grant UI flow (feature 022).
package e2e_test

// Revoke Grant Flow UI E2E Tests
//
// Each It() block maps to exactly ONE acceptance scenario from spec.md:
// - US1-S1: "Revoke All Access" button visible on detail page when grant exists
// - US1-S2: Confirmation dialog shows agent name and OAuth2 services note
// - Edge:   "Revoke All Access" button absent when user has no active grant
// - US2-S1: Revoke action visible on each overview page agent card
// - US2-S2: Overview dialog content matches spec

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// RevokeGrantFlow tests verify the revoke agent consent UI flows described in spec.md.
var _ = Describe("Revoke Grant Flow", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	// Setup per-test resources in BeforeEach
	BeforeEach(func() {
		ctx = context.Background()

		// Step 1: Create a third-party OAuth2 service with scopes
		service := &model.ThirdpartyOAuth2ProviderEntity{
			ID:          id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
			DisplayName: "GitHub",
			ClientID:    id.ClientID("github-client-id"),
			Secret:      fixtures.EncryptedSecret("550e8400-e29b-41d4-a716-446655440000", "github-client-secret"),
			IssuerURI:   "https://github.com",
			Endpoints: model.OAuth2Endpoints{
				AuthorizeEndpoint: "https://github.com/login/oauth/authorize",
				TokenEndpoint:     "https://github.com/login/oauth/access_token",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "repo", Description: "Repository access"},
				{ScopeValue: "user", Description: "User profile access"},
			},
		}
		err := GetTestStorage().Services().Create(ctx, service)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test service")

		// Step 2: Create a test agent with a mandatory service requirement
		agent := fixtures.ValidAgent()
		testAgentID = agent.ID.String()
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{
				ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
				RequirementType: storage.RequirementTypeMandatory,
				RequiredScopes:  []string{"repo", "user"},
			},
		}
		err = GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		// Step 3: Create an active grant for the default principal (user has consented)
		principal := fixtures.DefaultPrincipal().String()
		grant := fixtures.IndefiniteGrant(principal, testAgentID, "550e8400-e29b-41d4-a716-446655440000", []string{"repo", "user"})
		err = GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test grant")

		// Initialize the consent page object
		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario US1-S1 from specs/022-revoke-agent-consent/spec.md
	It("Revoke All Access button visible on detail page when grant exists", func() {
		// Given: User has an active grant and navigates to the agent detail page
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		// Then: "Revoke All Access" button is visible alongside the approval controls
		present, err := consentPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke button presence")
		Expect(present).To(BeTrue(), "Revoke All Access button should be visible when grant exists")

		err = consentPage.TakeScreenshot(ctx, "revoke_button_visible_detail_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke All Access button visible on detail page")
	})

	// Scenario US1-S2 from specs/022-revoke-agent-consent/spec.md
	It("confirmation dialog shows agent name and OAuth2 services note", func() {
		// Given: User navigates to the agent detail page and clicks Revoke All Access
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		err = consentPage.ClickRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke All Access button")

		// Then: Confirmation dialog appears
		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear")

		// And: Dialog contains the agent name
		agentNamePresent, err := consentPage.RevokeDialogContainsText(ctx, "Test Agent Valid")
		Expect(err).NotTo(HaveOccurred())
		Expect(agentNamePresent).To(BeTrue(), "Dialog should mention the agent name")

		// And: Dialog informs user that connected services remain active (FR-010)
		// Actual dialog text: "Any connected services (e.g., GitHub, Google) remain active"
		servicesNotePresent, err := consentPage.RevokeDialogContainsText(ctx, "remain active")
		Expect(err).NotTo(HaveOccurred())
		Expect(servicesNotePresent).To(BeTrue(),
			"Dialog must inform user that connected services are NOT terminated (FR-010)")

		err = consentPage.TakeRevokeDialogScreenshot(ctx, "revoke_dialog_content")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke dialog shows agent name and service note")
	})

	// Scenario Edge from specs/022-revoke-agent-consent/spec.md
	It("Revoke All Access button absent when user has no active grant", func() {
		// Given: A second agent with NO grant for this user
		agentWithoutGrant := fixtures.AnotherAgent()
		err := GetTestStorage().Agents().Create(ctx, agentWithoutGrant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create agent without grant")

		// When: User navigates to the detail page of an agent they have NOT consented to
		noGrantPage := pages.NewConsentPage(GetTestPage(), GetFrontendURL())
		err = noGrantPage.Navigate(ctx, "/agents/"+agentWithoutGrant.ID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent without grant")

		// Then: "Revoke All Access" button must NOT be visible (FR-009)
		present, err := noGrantPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke button presence")
		Expect(present).To(BeFalse(), "Revoke All Access button should NOT appear when user has no active grant")

		err = noGrantPage.TakeScreenshot(ctx, "revoke_button_absent_no_grant")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke button absent when no active grant")
	})

	// Scenario US2-S1 from specs/022-revoke-agent-consent/spec.md
	It("Revoke action visible on each overview page agent card", func() {
		// Given: User has an active grant and views the consent overview page
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		// Then: Each agent card with an active grant shows a "Revoke" action
		present, err := consentPage.IsOverviewRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeTrue(),
			"Each agent card with an active grant should show a Revoke action")

		err = consentPage.TakeScreenshot(ctx, "revoke_action_overview_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke action visible on overview page agent card")
	})

	// Scenario US2-S2 from specs/022-revoke-agent-consent/spec.md
	It("overview dialog content matches spec", func() {
		// Given: User views the consent overview page and clicks Revoke on an agent card
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		// When: User clicks the Revoke action on an agent card
		err = consentPage.ClickOverviewRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke on agent card")

		// Then: A confirmation dialog appears (same RevokeGrantDialog as detail page per spec)
		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear from overview")

		// And: Dialog names the agent (FR-010)
		agentNamePresent, err := consentPage.RevokeDialogContainsText(ctx, "Test Agent Valid")
		Expect(err).NotTo(HaveOccurred())
		Expect(agentNamePresent).To(BeTrue(),
			"Overview dialog must mention the agent name")

		err = consentPage.TakeRevokeDialogScreenshot(ctx, "revoke_dialog_from_overview")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Overview revoke dialog content matches spec")
	})

	// Scenario US1-S5 from specs/022-revoke-agent-consent/spec.md
	It("cancel dialog from detail page — grant remains active, detail page stays open", func() {
		// Given: User navigates to the agent detail page and opens the revoke dialog
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		err = consentPage.ClickRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke All Access button")

		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear")

		// When: User cancels the confirmation dialog
		err = consentPage.CancelRevoke(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to cancel revoke dialog")

		// Then: Dialog is dismissed (wait for close animation to complete)
		err = consentPage.WaitForRevokeDialogDismissed(ctx)
		Expect(err).NotTo(HaveOccurred(), "Dialog should be dismissed after cancel")

		// And: Revoke button is still present (detail page still active, grant unchanged)
		present, err := consentPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke button after cancel")
		Expect(present).To(BeTrue(), "Detail page should still show Revoke button — grant was not deleted")

		err = consentPage.TakeScreenshot(ctx, "cancel_revoke_detail_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Cancel from detail page leaves grant intact")
	})

	// Scenario US2-S4 from specs/022-revoke-agent-consent/spec.md
	It("cancel dialog from overview page — agent card remains unchanged", func() {
		// Given: User views the consent overview page and opens the revoke dialog on a card
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		err = consentPage.ClickOverviewRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke on agent card")

		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear")

		// When: User cancels the confirmation
		err = consentPage.CancelRevoke(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to cancel revoke dialog from overview")

		// Then: Dialog is dismissed (wait for close animation to complete)
		err = consentPage.WaitForRevokeDialogDismissed(ctx)
		Expect(err).NotTo(HaveOccurred(), "Dialog should be dismissed after cancel")

		// And: Agent card is still present (grant was not revoked)
		cardStillPresent, err := consentPage.IsOverviewRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(cardStillPresent).To(BeTrue(),
			"Agent card with Revoke action must still be visible after cancellation (grant unchanged)")

		err = consentPage.TakeScreenshot(ctx, "cancel_revoke_overview_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Cancel from overview page leaves agent card unchanged")
	})
})
