// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the revoke agent grant UI flow (feature 022).
package e2e_test

// Revoke Grant Flow UI E2E Tests
//
// Each It() block maps to exactly ONE acceptance scenario from spec.md:
// - US1-S1: "Revoke all access" available in the detail header overflow when a grant exists
// - US1-S2: Confirmation dialog names the agent and keeps OAuth2 services connected
// - Edge:   Overflow revoke action absent when the user has no active grant
// - US2-S1: Revoke action visible on each granted agent's overview table row
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
		testAgentID id.AgentID
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

		// IndefiniteGrant references the shared placeholder set and service. Seed its
		// backing rows so both overview and agent detail resolve the active grant.
		Expect(fixtures.SeedPlaceholderGrantData(ctx, GetTestStorage(), service.ID)).To(Succeed())

		// Step 2: Create a test agent with a mandatory service requirement
		agent := fixtures.ValidAgent()
		testAgentID = agent.ID
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
		grant := fixtures.IndefiniteGrant(principal, testAgentID.String(), "550e8400-e29b-41d4-a716-446655440000", []string{"repo", "user"})
		err = GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test grant")

		// Initialize the consent page object
		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario US1-S1 from specs/022-revoke-agent-consent/spec.md
	It("Revoke all access is available in the detail header overflow when a grant exists", func() {
		// Given: User has an active grant and navigates to the agent detail page
		err := consentPage.NavigateToAgent(ctx, testAgentID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		// Then: the overflow offers Revoke all access, not a resting destructive button.
		present, err := consentPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke button presence")
		Expect(present).To(BeTrue(), "Revoke all access should be in the header overflow when a grant exists")

		Expect(consentPage.OpenOverflowMenu(ctx)).To(Succeed(), "Failed to open the overflow for its screenshot")
		err = consentPage.TakeScreenshot(ctx, "revoke_button_visible_detail_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke all access available in detail header overflow")
	})

	// Scenario US1-S2 from specs/022-revoke-agent-consent/spec.md
	It("confirmation dialog shows agent name and OAuth2 services note", func() {
		// Given: User navigates to the agent detail page and clicks Revoke All Access
		err := consentPage.NavigateToAgent(ctx, testAgentID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		err = consentPage.ClickRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to choose Revoke all access from the header overflow")

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
	It("Revoke all access is absent when the user has no active grant", func() {
		// Given: A second agent with NO grant for this user
		agentWithoutGrant := fixtures.AnotherAgent()
		err := GetTestStorage().Agents().Create(ctx, agentWithoutGrant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create agent without grant")

		// When: User navigates to the detail page of an agent they have NOT consented to
		noGrantPage := pages.NewConsentPage(GetTestPage(), GetFrontendURL())
		err = noGrantPage.Navigate(ctx, "/agents/"+agentWithoutGrant.ID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent without grant")

		// Then: no detail header overflow revoke action exists for this user.
		present, err := noGrantPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke action presence")
		Expect(present).To(BeFalse(), "The header overflow must be absent without an active grant")

		err = noGrantPage.TakeScreenshot(ctx, "revoke_button_absent_no_grant")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke button absent when no active grant")
	})

	// Scenario US2-S1 from specs/022-revoke-agent-consent/spec.md
	It("Revoke action visible on each overview table row", func() {
		// Given: User has an active grant and views the consent overview page
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		// Then: a delegation table row with an active grant offers Revoke.
		present, err := consentPage.IsOverviewRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(present).To(BeTrue(),
			"Delegation row with an active grant should show a Revoke action")

		err = consentPage.TakeScreenshot(ctx, "revoke_action_overview_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Revoke action visible on overview table row")
	})

	// Scenario US2-S2 from specs/022-revoke-agent-consent/spec.md
	It("overview dialog content matches spec", func() {
		// Given: User views the consent overview and clicks Revoke on a table row.
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		// When: User clicks the Revoke action on a delegation row.
		err = consentPage.ClickOverviewRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke on a delegation row")

		// Then: A confirmation dialog appears (the same RevokeAgentDialog as on detail).
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
		err := consentPage.NavigateToAgent(ctx, testAgentID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to agent detail page")

		err = consentPage.ClickRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to choose Revoke all access from the header overflow")

		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear")

		// When: User cancels the confirmation dialog
		err = consentPage.CancelRevoke(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to cancel revoke dialog")

		// Then: Dialog is dismissed (wait for close animation to complete)
		err = consentPage.WaitForRevokeDialogDismissed(ctx)
		Expect(err).NotTo(HaveOccurred(), "Dialog should be dismissed after cancel")

		// And: The overflow action remains (detail page still active, grant unchanged).
		present, err := consentPage.IsRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check revoke action after cancel")
		Expect(present).To(BeTrue(), "The overflow should still offer Revoke after cancellation")
		storedGrant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(fixtures.DefaultPrincipal().String()), testAgentID)
		Expect(err).NotTo(HaveOccurred())
		Expect(storedGrant).NotTo(BeNil(), "Cancelling must not delete this user's grant")

		err = consentPage.TakeScreenshot(ctx, "cancel_revoke_detail_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Cancel from detail page leaves grant intact")
	})

	// Scenario US2-S4 from specs/022-revoke-agent-consent/spec.md
	It("cancel dialog from overview page — delegation row remains unchanged", func() {
		// Given: User views the overview and opens the revoke dialog on a table row.
		err := consentPage.NavigateToOverview(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent overview page")

		err = consentPage.ClickOverviewRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Revoke on a delegation row")

		err = consentPage.WaitForRevokeDialog(ctx)
		Expect(err).NotTo(HaveOccurred(), "Revoke confirmation dialog did not appear")

		// When: User cancels the confirmation
		err = consentPage.CancelRevoke(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to cancel revoke dialog from overview")

		// Then: Dialog is dismissed (wait for close animation to complete)
		err = consentPage.WaitForRevokeDialogDismissed(ctx)
		Expect(err).NotTo(HaveOccurred(), "Dialog should be dismissed after cancel")

		// And: The delegation row remains (grant was not revoked).
		rowStillPresent, err := consentPage.IsOverviewRevokeButtonPresent(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(rowStillPresent).To(BeTrue(),
			"Delegation row with Revoke action must remain after cancellation (grant unchanged)")
		storedGrant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(fixtures.DefaultPrincipal().String()), testAgentID)
		Expect(err).NotTo(HaveOccurred())
		Expect(storedGrant).NotTo(BeNil(), "Cancelling must not delete this user's grant")

		err = consentPage.TakeScreenshot(ctx, "cancel_revoke_overview_page")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Cancel from overview leaves delegation row unchanged")
	})
})
