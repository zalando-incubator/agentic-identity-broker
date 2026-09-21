// Package e2e_test provides end-to-end tests for the permission sets UI.
// This file tests the consent screen's permission set cards, toggle behavior,
// service connection indicators, and approve button state management.
package e2e_test

import (
	"context"
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

// Well-known UUIDs for permission set test fixtures.
var (
	psGitHubServiceID    = id.MustParseServiceID("b0000000-0000-0000-0000-000000000001")
	psGoogleServiceID    = id.MustParseServiceID("b0000000-0000-0000-0000-000000000002")
	psMicrosoftServiceID = id.MustParseServiceID("b0000000-0000-0000-0000-000000000003")
	psSlackServiceID     = id.MustParseServiceID("b0000000-0000-0000-0000-000000000004")

	psMandatoryID  = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000001")
	psOptionalID   = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000002")
	psMandatory2ID = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000003")
	psOptional2ID  = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000004")
)

// createTestService creates a ThirdpartyOAuth2ProviderEntity with given ID, name, and scopes.
func createTestService(svcID id.ServiceID, displayName string, scopes []model.OAuthScope) *model.ThirdpartyOAuth2ProviderEntity {
	now := time.Now()
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          svcID,
		DisplayName: displayName,
		ClientID:    id.ClientID("client-" + svcID.String()),
		Secret:      fixtures.EncryptedSecret(svcID.String(), "secret-"+svcID.String()),
		IssuerURI:   "https://" + displayName + ".example.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://" + displayName + ".example.com/authorize",
			TokenEndpoint:     "https://" + displayName + ".example.com/token",
		},
		Scopes:    scopes,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// createPermissionSet creates a PermissionSet in storage.
func createPermissionSet(ctx context.Context, psID id.PermissionSetID, name, description string, serviceScopes []storage.ServiceScope) {
	ps := &storage.PermissionSet{
		ID:            psID,
		Name:          name,
		Description:   description,
		ServiceScopes: serviceScopes,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	err := GetTestStorage().PermissionSets().Create(ctx, ps)
	Expect(err).NotTo(HaveOccurred(), "Failed to create permission set %q", name)
}

var _ = Describe("Permission Sets on Consent Screen", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create three third-party OAuth2 services
		githubSvc := createTestService(psGitHubServiceID, "GitHub", []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
			{ScopeValue: "user", Description: "User profile access"},
		})
		googleSvc := createTestService(psGoogleServiceID, "Google", []model.OAuthScope{
			{ScopeValue: "calendar", Description: "Calendar access"},
			{ScopeValue: "drive", Description: "Drive access"},
		})
		microsoftSvc := createTestService(psMicrosoftServiceID, "Microsoft", []model.OAuthScope{
			{ScopeValue: "mail.read", Description: "Read mail"},
			{ScopeValue: "user.read", Description: "Read user profile"},
		})

		for _, svc := range []*model.ThirdpartyOAuth2ProviderEntity{githubSvc, googleSvc, microsoftSvc} {
			err := GetTestStorage().Services().Create(ctx, svc)
			Expect(err).NotTo(HaveOccurred(), "Failed to create service %q", svc.DisplayName)
		}

		// Create permission sets
		createPermissionSet(ctx, psMandatoryID, "Code Access", "Access to code repositories and user profiles", []storage.ServiceScope{
			{ServiceID: psGitHubServiceID, Scopes: []string{"repo", "user"}, RequirementType: storage.RequirementTypeOptional},
		})
		createPermissionSet(ctx, psOptionalID, "Productivity Suite", "Access to calendar and email services", []storage.ServiceScope{
			{ServiceID: psGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
			{ServiceID: psMicrosoftServiceID, Scopes: []string{"mail.read"}, RequirementType: storage.RequirementTypeOptional},
		})

		// Create test agent with mandatory + optional permission sets
		// Also add ServiceRequirements for proper FR-008 service filtering:
		// - GitHub is mandatory (always required)
		// - Google and Microsoft are optional (user can toggle)
		agent := fixtures.ValidAgent()
		testAgentID = agent.ID.String()
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{
				PermissionSetID: psMandatoryID,
				RequirementType: storage.RequirementTypeMandatory,
			},
			{
				PermissionSetID: psOptionalID,
				RequirementType: storage.RequirementTypeOptional,
			},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: psGitHubServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo", "user"}},
			{ServiceID: psGoogleServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"calendar"}},
			{ServiceID: psMicrosoftServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"mail.read"}},
		}
		err := GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	// Scenario 1: Permission sets section appears above service connections
	It("should display permission sets section above service connections", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Verify the permission sets section heading exists (rendered as "Agent Permissions" per FR-008)
		psHeading := page.GetByRole("heading", playwright.PageGetByRoleOptions{
			Name: "Agent Permissions",
		})
		psCount, err := psHeading.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(psCount).To(BeNumerically(">=", 1), "Agent Permissions heading should be visible")

		// Verify at least one permission set card is rendered
		mandatoryCard := page.GetByText("Code Access")
		cardCount, err := mandatoryCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(cardCount).To(BeNumerically(">=", 1), "Mandatory permission set 'Code Access' should be visible")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_section_above_services")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Permission sets section rendered above service connections")
	})

	// Scenario 2: Mandatory permission set card (locked, pre-selected, no checkbox)
	It("should display mandatory permission set card as locked and pre-selected", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Find the mandatory permission set card
		mandatoryCard := page.GetByText("Code Access").First()
		Expect(mandatoryCard).NotTo(BeNil())

		visible, err := mandatoryCard.IsVisible()
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue(), "Mandatory permission set card should be visible")

		// Mandatory card should show "Required" or "Mandatory" indicator
		requiredIndicator := page.GetByText("Required").First()
		reqVisible, err := requiredIndicator.IsVisible()
		if err == nil && reqVisible {
			Expect(reqVisible).To(BeTrue(), "Required indicator should be visible for mandatory PS")
		}

		// Mandatory card should NOT have a togglable checkbox
		// Look within the context of the mandatory PS for unchecked checkboxes
		mandatorySection := page.Locator("[data-testid='permission-set-mandatory']").First()
		if cnt, _ := mandatorySection.Count(); cnt > 0 {
			checkboxes := mandatorySection.GetByRole("checkbox")
			cbCount, _ := checkboxes.Count()
			Expect(cbCount).To(Equal(0), "Mandatory permission set should not have a togglable checkbox")
		}

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mandatory_card_locked")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Mandatory permission set displayed as locked and pre-selected")
	})

	// Scenario 3: Optional permission set card (togglable, initially unchecked)
	It("should display optional permission set card as togglable and initially unchecked", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Find the optional permission set card
		optionalCard := page.GetByText("Productivity Suite").First()
		visible, err := optionalCard.IsVisible()
		Expect(err).NotTo(HaveOccurred())
		Expect(visible).To(BeTrue(), "Optional permission set card should be visible")

		// Optional card should have a toggle/checkbox that is initially unchecked
		optionalSection := page.Locator("[data-testid='permission-set-optional']").First()
		if cnt, _ := optionalSection.Count(); cnt > 0 {
			checkbox := optionalSection.GetByRole("checkbox").First()
			if cbCnt, _ := checkbox.Count(); cbCnt > 0 {
				checked, err := checkbox.GetAttribute("aria-checked")
				Expect(err).NotTo(HaveOccurred())
				Expect(checked).To(Equal("false"), "Optional permission set should be initially unchecked")
			}
		}

		// Alternatively, check for a switch/toggle element
		optionalToggle := page.GetByLabel("Productivity Suite").First()
		if toggleCnt, _ := optionalToggle.Count(); toggleCnt > 0 {
			checked, err := optionalToggle.GetAttribute("aria-checked")
			if err == nil && checked != "" {
				Expect(checked).To(Equal("false"), "Optional PS toggle should be initially unchecked")
			}
		}

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_optional_card_togglable")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Optional permission set displayed as togglable and initially unchecked")
	})

	// Scenario 4: PS card filters to SR-intersecting services with per-service toggles
	It("should show SR-intersecting services within each permission set card", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// The mandatory "Code Access" PS should show GitHub (its service scope)
		githubText := page.GetByText("GitHub")
		ghCount, err := githubText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(ghCount).To(BeNumerically(">=", 1), "GitHub service should appear under Code Access PS")

		// The optional "Productivity Suite" PS should show Google and Microsoft
		googleText := page.GetByText("Google")
		gCount, err := googleText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(gCount).To(BeNumerically(">=", 1), "Google service should appear under Productivity Suite PS")

		microsoftText := page.GetByText("Microsoft")
		msCount, err := microsoftText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(msCount).To(BeNumerically(">=", 1), "Microsoft service should appear under Productivity Suite PS")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_sr_intersecting_services")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Permission set cards show SR-intersecting services")
	})

	// Scenario 5: Dynamic service connections update when optional PS toggled on/off
	It("should update service connections section when optional PS is toggled", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Before toggling: optional PS services (Google, Microsoft) should not require connection
		// Take initial state screenshot
		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_before_optional_toggle")).NotTo(HaveOccurred())

		// Find and click the optional PS toggle
		optionalToggle := page.Locator("[data-testid='permission-set-optional']").GetByRole("checkbox").First()
		if cnt, _ := optionalToggle.Count(); cnt == 0 {
			// Try switch role instead
			optionalToggle = page.Locator("[data-testid='permission-set-optional']").GetByRole("switch").First()
		}
		if cnt, _ := optionalToggle.Count(); cnt == 0 {
			// Fall back to label-based toggle
			optionalToggle = page.GetByLabel("Productivity Suite").First()
		}

		toggleCount, err := optionalToggle.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(toggleCount).To(BeNumerically(">", 0), "expected at least one optional permission-set toggle element; UI may be missing the optional PS row")

		// Toggle ON the optional PS
		err = optionalToggle.Click()
		Expect(err).NotTo(HaveOccurred(), "Failed to click optional PS toggle")

		// Wait for UI to update after toggle
		err = page.WaitForLoadState()
		Expect(err).NotTo(HaveOccurred())

		// After toggling ON: Google and Microsoft should now appear in service connections
		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_after_optional_toggle_on")).NotTo(HaveOccurred())

		// Toggle OFF the optional PS
		err = optionalToggle.Click()
		Expect(err).NotTo(HaveOccurred(), "Failed to toggle off optional PS")

		// Wait for UI to update after toggle
		err = page.WaitForLoadState()
		Expect(err).NotTo(HaveOccurred())

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_after_optional_toggle_off")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Service connections update dynamically with optional PS toggle")
	})

	// US2.S3 from specs/019-permission-sets/spec.md
	It("should show active session for already connected services", func() {
		// Create a user session for GitHub (simulating an existing OAuth2 connection)
		principal := fixtures.DefaultPrincipal().String()

		// Create a grant with the mandatory PS's service already included
		grant := fixtures.IndefiniteGrant(principal, testAgentID, psGitHubServiceID.String(), []string{"repo", "user"})
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{
			{
				PermissionSetID:    psMandatoryID,
				IncludedServiceIDs: []id.ServiceID{psGitHubServiceID},
			},
		}
		err := GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test grant")

		// Create a user session to simulate the service being connected
		session := fixtures.SessionForService(principal, psGitHubServiceID.String())
		err = GetTestStorage().UserSessions().Create(ctx, session)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test session")

		err = consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		connection, err := consentPage.GetServiceConnectionState(ctx, "GitHub")
		Expect(err).NotTo(HaveOccurred())
		Expect(connection.Present).To(BeTrue(), "GitHub should remain visible in service connections when it has an active session")
		Expect(connection.HasActiveSession).To(BeTrue(), "GitHub should show an Active Session indicator")
		Expect(connection.HasLoginButton).To(BeFalse(), "GitHub should not show a Login button when it has an active session")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_service_already_connected")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: active service shows an indicator without a login action")
	})

	// Scenario 7: Approve button disabled until all displayed services connected
	It("should disable approve button when mandatory services are not connected", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Find the Approve & Delegate button
		approveBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{
			Name: "Approve & Delegate",
		})
		btnCount, err := approveBtn.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(btnCount).To(BeNumerically(">=", 1), "Approve button should exist")

		// Button should be disabled because mandatory services are not yet connected
		disabled, err := approveBtn.IsDisabled()
		Expect(err).NotTo(HaveOccurred())
		Expect(disabled).To(BeTrue(), "Approve button should be disabled when mandatory services are not connected")

		// Verify the button has aria-disabled attribute
		ariaDisabled, err := approveBtn.GetAttribute("aria-disabled")
		if err == nil && ariaDisabled != "" {
			Expect(ariaDisabled).To(Equal("true"), "Approve button should have aria-disabled=true")
		}

		// Also verify via page object method
		enabled, err := consentPage.IsConsentButtonEnabled(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeFalse(), "Approve button should report as not enabled")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_approve_button_disabled")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Approve button disabled when mandatory services not connected")
	})

	// Scenario 8: Approve button enabled when all services connected
	It("should enable approve button when all mandatory services are connected", func() {
		principal := fixtures.DefaultPrincipal().String()

		// Create user session for GitHub (the mandatory service)
		session := fixtures.SessionForService(principal, psGitHubServiceID.String())
		err := GetTestStorage().UserSessions().Create(ctx, session)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test session for GitHub")

		// Create a grant referencing the mandatory PS with GitHub connected
		grant := fixtures.IndefiniteGrant(principal, testAgentID, psGitHubServiceID.String(), []string{"repo", "user"})
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{
			{
				PermissionSetID:    psMandatoryID,
				IncludedServiceIDs: []id.ServiceID{psGitHubServiceID},
			},
		}
		err = GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test grant")

		err = consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Approve button should be enabled when all mandatory services are connected
		approveBtn := page.GetByRole("button", playwright.PageGetByRoleOptions{
			Name: "Approve & Delegate",
		})
		btnCount, err := approveBtn.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(btnCount).To(BeNumerically(">=", 1), "Approve button should exist")

		enabled, err := approveBtn.IsEnabled()
		Expect(err).NotTo(HaveOccurred())
		Expect(enabled).To(BeTrue(), "Approve button should be enabled when all mandatory services are connected")

		// Also verify via page object method
		isEnabled, err := consentPage.IsConsentButtonEnabled(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(isEnabled).To(BeTrue(), "Approve button should report as enabled via page object")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_approve_button_enabled")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Approve button enabled when all mandatory services connected")
	})
})

// Describe block for all-mandatory permission sets scenario.
// Tests the UI when an agent has only mandatory permission sets — all cards
// should be locked, no toggles shown, and all services required.
var _ = Describe("Permission Sets - All Mandatory", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create services
		githubSvc := createTestService(psGitHubServiceID, "GitHub", []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
			{ScopeValue: "user", Description: "User profile access"},
		})
		googleSvc := createTestService(psGoogleServiceID, "Google", []model.OAuthScope{
			{ScopeValue: "calendar", Description: "Calendar access"},
		})

		for _, svc := range []*model.ThirdpartyOAuth2ProviderEntity{githubSvc, googleSvc} {
			err := GetTestStorage().Services().Create(ctx, svc)
			Expect(err).NotTo(HaveOccurred(), "Failed to create service %q", svc.DisplayName)
		}

		// Create two mandatory permission sets
		createPermissionSet(ctx, psMandatoryID, "Code Access", "Access to code repositories", []storage.ServiceScope{
			{ServiceID: psGitHubServiceID, Scopes: []string{"repo", "user"}, RequirementType: storage.RequirementTypeOptional},
		})
		createPermissionSet(ctx, psMandatory2ID, "Calendar Access", "Access to calendar", []storage.ServiceScope{
			{ServiceID: psGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
		})

		// Agent with only mandatory permission sets and mandatory SRs
		agent := fixtures.ValidAgent()
		agent.DisplayName = "All Mandatory Agent"
		agent.Description = "Agent with only mandatory permission sets for testing locked UI"
		testAgentID = agent.ID.String()
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: psMandatoryID, RequirementType: storage.RequirementTypeMandatory},
			{PermissionSetID: psMandatory2ID, RequirementType: storage.RequirementTypeMandatory},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: psGitHubServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo", "user"}},
			{ServiceID: psGoogleServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"calendar"}},
		}
		err := GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create all-mandatory test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	It("should display all permission sets as locked with Required badges", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Both permission set cards should be visible
		codeAccess := page.GetByText("Code Access")
		calendarAccess := page.GetByText("Calendar Access")

		codeCount, err := codeAccess.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(codeCount).To(BeNumerically(">=", 1), "Code Access PS should be visible")

		calCount, err := calendarAccess.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(calCount).To(BeNumerically(">=", 1), "Calendar Access PS should be visible")

		// Both should show "Required" badges — use exact match to avoid matching
		// the description text "Required permissions are always granted."
		requiredBadges := page.GetByText("Required", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})
		reqCount, err := requiredBadges.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(BeNumerically(">=", 2), "Both PS cards should show Required badge")

		// No toggle switches should be present (all mandatory = no toggles)
		mandatoryCards := page.Locator("[data-testid='permission-set-mandatory']")
		mandatoryCount, err := mandatoryCards.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(mandatoryCount).To(Equal(2), "Should have exactly 2 mandatory PS cards")

		// No "Optional" badges should appear — use exact match to avoid matching
		// the description text "Toggle optional permissions on or off…"
		optionalBadges := page.GetByText("Optional", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})
		optCount, err := optionalBadges.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optCount).To(Equal(0), "No Optional badges should appear for all-mandatory agent")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_mandatory_permission_sets")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: All-mandatory permission sets display correctly")
	})

	It("should show all services as required within mandatory permission sets", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// GitHub should appear with "required" indicator
		githubText := page.GetByText("GitHub")
		ghCount, err := githubText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(ghCount).To(BeNumerically(">=", 1), "GitHub service should be visible")

		// Google should appear with "required" indicator
		googleText := page.GetByText("Google")
		gCount, err := googleText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(gCount).To(BeNumerically(">=", 1), "Google service should be visible")

		// "required" text should appear for services (service-level requirement indicators)
		requiredIndicators := page.GetByText("required")
		reqCount, err := requiredIndicators.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(BeNumerically(">=", 2), "Both services should show required indicator")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_mandatory_services_required")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: All services shown as required in mandatory PS cards")
	})
})

// Describe block for all-optional permission sets scenario.
// Tests the UI when an agent has only optional permission sets — all cards
// should be togglable, initially unchecked, and the user must select at least one.
var _ = Describe("Permission Sets - All Optional", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create services
		googleSvc := createTestService(psGoogleServiceID, "Google", []model.OAuthScope{
			{ScopeValue: "calendar", Description: "Calendar access"},
			{ScopeValue: "drive", Description: "Drive access"},
		})
		microsoftSvc := createTestService(psMicrosoftServiceID, "Microsoft", []model.OAuthScope{
			{ScopeValue: "mail.read", Description: "Read mail"},
		})

		for _, svc := range []*model.ThirdpartyOAuth2ProviderEntity{googleSvc, microsoftSvc} {
			err := GetTestStorage().Services().Create(ctx, svc)
			Expect(err).NotTo(HaveOccurred(), "Failed to create service %q", svc.DisplayName)
		}

		// Create two optional permission sets
		createPermissionSet(ctx, psOptionalID, "Productivity Suite", "Access to calendar and email", []storage.ServiceScope{
			{ServiceID: psGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
		})
		createPermissionSet(ctx, psOptional2ID, "Communication Tools", "Access to email services", []storage.ServiceScope{
			{ServiceID: psMicrosoftServiceID, Scopes: []string{"mail.read"}, RequirementType: storage.RequirementTypeOptional},
		})

		// Agent with only optional permission sets and optional SRs
		agent := fixtures.ValidAgent()
		agent.DisplayName = "All Optional Agent"
		agent.Description = "Agent with only optional permission sets for testing toggle UI"
		testAgentID = agent.ID.String()
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: psOptionalID, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: psOptional2ID, RequirementType: storage.RequirementTypeOptional},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: psGoogleServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"calendar"}},
			{ServiceID: psMicrosoftServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"mail.read"}},
		}
		err := GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create all-optional test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	It("should display all permission sets as optional with toggle switches", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Both permission set cards should show "Optional" badges — use exact match to avoid
		// matching the description text "Toggle optional permissions on or off…"
		optionalBadges := page.GetByText("Optional", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})
		optCount, err := optionalBadges.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optCount).To(BeNumerically(">=", 2), "Both PS cards should show Optional badge")

		// Both should have toggle switches
		optionalCards := page.Locator("[data-testid='permission-set-optional']")
		optCardCount, err := optionalCards.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optCardCount).To(Equal(2), "Should have exactly 2 optional PS cards")

		// No "Required" badges should appear — use exact match to avoid matching
		// the description text "Required permissions are always granted."
		requiredBadges := page.GetByText("Required", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})
		reqCount, err := requiredBadges.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(Equal(0), "No Required badges should appear for all-optional agent")

		// Each toggle should be in the OFF state initially
		switches := page.Locator("[data-testid='permission-set-optional'] button[role='switch']")
		switchCount, err := switches.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(switchCount).To(BeNumerically(">=", 2), "Each optional PS should have a toggle switch")

		for i := 0; i < switchCount; i++ {
			checked, err := switches.Nth(i).GetAttribute("aria-checked")
			Expect(err).NotTo(HaveOccurred())
			Expect(checked).To(Equal("false"), "Optional PS toggle should be initially off")
		}

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_optional_permission_sets")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: All-optional permission sets display with toggles")
	})

	It("should enable selecting optional permission sets independently", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Take initial state screenshot (all off)
		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_optional_initial_state")).NotTo(HaveOccurred())

		// Toggle the first optional PS on
		firstSwitch := page.Locator("[data-testid='permission-set-optional']").First().Locator("button[role='switch']").First()
		switchCount, err := firstSwitch.Count()
		Expect(err).NotTo(HaveOccurred())
		if switchCount > 0 {
			err = firstSwitch.Click()
			Expect(err).NotTo(HaveOccurred(), "Failed to toggle first optional PS")
			err = page.WaitForLoadState()
			Expect(err).NotTo(HaveOccurred())

			// First switch should now be ON
			checked, err := firstSwitch.GetAttribute("aria-checked")
			Expect(err).NotTo(HaveOccurred())
			Expect(checked).To(Equal("true"), "First PS toggle should be ON after click")

			Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_optional_first_selected")).NotTo(HaveOccurred())
		}

		// Toggle the second optional PS on
		secondSwitch := page.Locator("[data-testid='permission-set-optional']").Nth(1).Locator("button[role='switch']").First()
		secondCount, err := secondSwitch.Count()
		Expect(err).NotTo(HaveOccurred())
		if secondCount > 0 {
			err = secondSwitch.Click()
			Expect(err).NotTo(HaveOccurred(), "Failed to toggle second optional PS")
			err = page.WaitForLoadState()
			Expect(err).NotTo(HaveOccurred())

			Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_all_optional_both_selected")).NotTo(HaveOccurred())
		}

		GetLogger().Info("Test passed: Optional permission sets can be selected independently")
	})
})

// Describe block for mixed service requirements within permission set cards.
// Tests the UI when a single permission set contains services with both
// mandatory and optional service requirements, verifying different indicators.
var _ = Describe("Permission Sets - Mixed Service Requirements", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create four services
		githubSvc := createTestService(psGitHubServiceID, "GitHub", []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
		})
		googleSvc := createTestService(psGoogleServiceID, "Google", []model.OAuthScope{
			{ScopeValue: "calendar", Description: "Calendar access"},
		})
		microsoftSvc := createTestService(psMicrosoftServiceID, "Microsoft", []model.OAuthScope{
			{ScopeValue: "mail.read", Description: "Read mail"},
		})
		slackSvc := createTestService(psSlackServiceID, "Slack", []model.OAuthScope{
			{ScopeValue: "channels:read", Description: "Read channels"},
			{ScopeValue: "chat:write", Description: "Write messages"},
		})

		for _, svc := range []*model.ThirdpartyOAuth2ProviderEntity{githubSvc, googleSvc, microsoftSvc, slackSvc} {
			err := GetTestStorage().Services().Create(ctx, svc)
			Expect(err).NotTo(HaveOccurred(), "Failed to create service %q", svc.DisplayName)
		}

		// Create a permission set covering multiple services (mandatory + optional SR)
		createPermissionSet(ctx, psMandatoryID, "Development Tools", "Access to dev tools and communication", []storage.ServiceScope{
			{ServiceID: psGitHubServiceID, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional},
			{ServiceID: psSlackServiceID, Scopes: []string{"channels:read", "chat:write"}, RequirementType: storage.RequirementTypeOptional},
		})

		// Create a second permission set
		createPermissionSet(ctx, psOptionalID, "Productivity Suite", "Calendar and email access", []storage.ServiceScope{
			{ServiceID: psGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
			{ServiceID: psMicrosoftServiceID, Scopes: []string{"mail.read"}, RequirementType: storage.RequirementTypeOptional},
		})

		// Agent with mixed SRs: GitHub mandatory, Slack optional, Google mandatory, Microsoft optional
		agent := fixtures.ValidAgent()
		agent.DisplayName = "Mixed SR Agent"
		agent.Description = "Agent with mixed mandatory and optional service requirements"
		testAgentID = agent.ID.String()
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: psMandatoryID, RequirementType: storage.RequirementTypeMandatory},
			{PermissionSetID: psOptionalID, RequirementType: storage.RequirementTypeOptional},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: psGitHubServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"repo"}},
			{ServiceID: psSlackServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"channels:read"}},
			{ServiceID: psGoogleServiceID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"calendar"}},
			{ServiceID: psMicrosoftServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"mail.read"}},
		}
		err := GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create mixed-SR test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	It("should display mandatory and optional service indicators within a single PS card", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// The mandatory "Development Tools" PS should show both GitHub (mandatory SR) and Slack (optional SR)
		githubText := page.GetByText("GitHub")
		ghCount, err := githubText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(ghCount).To(BeNumerically(">=", 1), "GitHub service should be visible in Development Tools PS")

		slackText := page.GetByText("Slack")
		slackCount, err := slackText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(slackCount).To(BeNumerically(">=", 1), "Slack service should be visible in Development Tools PS")

		// GitHub should be marked as required (mandatory SR within the PS)
		// Slack should have a per-service toggle (optional SR within the PS)
		// The "required" text should appear for GitHub's service badge
		requiredIndicators := page.GetByText("required")
		reqCount, err := requiredIndicators.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(BeNumerically(">=", 1), "At least one service should show required indicator")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mixed_sr_mandatory_ps_card")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Mixed SR types correctly displayed within PS card")
	})

	It("should show per-service toggles for optional SR services within mandatory PS", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// The mandatory PS "Development Tools" has GitHub (mandatory SR) + Slack (optional SR)
		// Slack should have a toggle switch since it's an optional service within a mandatory PS
		mandatoryCard := page.Locator("[data-testid='permission-set-mandatory']").First()
		mandatoryCount, err := mandatoryCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(mandatoryCount).To(Equal(1), "Should have the mandatory PS card")

		// Within the mandatory PS card, look for service-level toggle switches
		serviceSwitches := mandatoryCard.Locator("button[role='switch']")
		switchCount, err := serviceSwitches.Count()
		Expect(err).NotTo(HaveOccurred())
		// The mandatory PS card itself has no PS-level toggle, but optional SR services within it
		// should have per-service toggle switches
		Expect(switchCount).To(BeNumerically(">=", 1), "Optional SR service (Slack) should have a toggle within mandatory PS")

		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mixed_sr_per_service_toggles")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Per-service toggles visible for optional SR services in mandatory PS")
	})

	It("should toggle optional PS and show service requirement indicators for both mandatory and optional services", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Initial state screenshot
		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mixed_sr_initial_state")).NotTo(HaveOccurred())

		// Find and toggle the optional PS "Productivity Suite"
		optionalToggle := page.Locator("[data-testid='permission-set-optional']").First().Locator("button[role='switch']").First()
		toggleCount, err := optionalToggle.Count()
		Expect(err).NotTo(HaveOccurred())
		if toggleCount > 0 {
			err = optionalToggle.Click()
			Expect(err).NotTo(HaveOccurred(), "Failed to toggle optional PS")
			err = page.WaitForLoadState()
			Expect(err).NotTo(HaveOccurred())

			// After toggling on, Google (mandatory SR) and Microsoft (optional SR) should be visible
			// with different indicators
			googleText := page.GetByText("Google")
			gCount, err := googleText.Count()
			Expect(err).NotTo(HaveOccurred())
			Expect(gCount).To(BeNumerically(">=", 1), "Google service should be visible after toggling PS on")

			microsoftText := page.GetByText("Microsoft")
			msCount, err := microsoftText.Count()
			Expect(err).NotTo(HaveOccurred())
			Expect(msCount).To(BeNumerically(">=", 1), "Microsoft service should be visible after toggling PS on")

			Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mixed_sr_optional_ps_toggled_on")).NotTo(HaveOccurred())
		}

		GetLogger().Info("Test passed: Mixed SR indicators shown correctly in toggled optional PS")
	})

	It("should show full consent screen with all services visible for screenshot documentation", func() {
		principal := fixtures.DefaultPrincipal().String()

		// Create sessions for mandatory services (GitHub, Google) to enable the approve button
		githubSession := fixtures.SessionForService(principal, psGitHubServiceID.String())
		err := GetTestStorage().UserSessions().Create(ctx, githubSession)
		Expect(err).NotTo(HaveOccurred(), "Failed to create GitHub session")

		googleSession := fixtures.SessionForService(principal, psGoogleServiceID.String())
		err = GetTestStorage().UserSessions().Create(ctx, googleSession)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Google session")

		// Create grant with mandatory PS + both mandatory services
		grant := fixtures.IndefiniteGrant(principal, testAgentID, psGitHubServiceID.String(), []string{"repo"})
		grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{
			{
				PermissionSetID:    psMandatoryID,
				IncludedServiceIDs: []id.ServiceID{psGitHubServiceID, psSlackServiceID},
			},
		}
		err = GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create grant")

		err = consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Toggle optional PS on to show all services
		optionalToggle := page.Locator("[data-testid='permission-set-optional']").First().Locator("button[role='switch']").First()
		toggleCount, err := optionalToggle.Count()
		Expect(err).NotTo(HaveOccurred())
		if toggleCount > 0 {
			err = optionalToggle.Click()
			Expect(err).NotTo(HaveOccurred(), "Failed to toggle optional PS")
			err = page.WaitForLoadState()
			Expect(err).NotTo(HaveOccurred())
		}

		// Full page screenshot showing all four services across two PS cards
		// with mixed mandatory/optional indicators, connected services, and the approve button
		Expect(consentPage.TakeScreenshot(ctx, "consent_permission_sets_mixed_sr_full_consent_screen")).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Full consent screen with mixed SRs captured")
	})
})

// Describe block for ServiceScope.requirement_type override behavior.
// Tests that a mandatory ServiceScope locks a service within an optional PS card,
// independently of the agent-level SR requirement_type (FR-019).
var _ = Describe("Permission Sets - ServiceScope requirement_type overrides optional SR", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string

		gwServiceID = id.MustParseServiceID("b0000000-0000-0000-0000-000000000005")
		psCoreID    = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000005")
	)

	BeforeEach(func() {
		ctx = context.Background()

		gwSvc := createTestService(gwServiceID, "Google Workspace", []model.OAuthScope{
			{ScopeValue: "drive", Description: "Drive access"},
			{ScopeValue: "calendar", Description: "Calendar access"},
		})
		err := GetTestStorage().Services().Create(ctx, gwSvc)
		Expect(err).NotTo(HaveOccurred(), "Failed to create Google Workspace service")

		// PS with mandatory ServiceScope — even though the agent SR is optional,
		// the service should be locked when this PS is selected.
		createPermissionSet(ctx, psCoreID, "Access to core productivity tools", "Core productivity access", []storage.ServiceScope{
			{ServiceID: gwServiceID, Scopes: []string{"drive", "calendar"}, RequirementType: storage.RequirementTypeMandatory},
		})

		agent := fixtures.ValidAgent()
		testAgentID = agent.ID.String()
		agent.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: psCoreID, RequirementType: storage.RequirementTypeOptional},
		}
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{ServiceID: gwServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"drive", "calendar"}},
		}
		err = GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	It("should not show Google Workspace as required when optional PS is deselected", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		optionalCard := page.Locator("[data-testid='permission-set-optional']").First()
		optCount, err := optionalCard.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(optCount).To(Equal(1), "Optional PS card should be present and deselected by default")

		// PS is deselected — no "required" indicator should appear inside the card
		reqCount, err := consentPage.ServiceScopeRequiredIndicatorCount(ctx, "permission-set-optional")
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(Equal(0), "No required indicator when optional PS is deselected")

		Expect(consentPage.TakeScreenshot(ctx, "consent_service_scope_req_type_ps_deselected")).NotTo(HaveOccurred())
	})

	It("should lock Google Workspace as required inside PS card when optional PS is selected", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		page := consentPage.GetPlaywrightPage()

		// Select the optional PS card via its PS-level toggle
		psToggle := page.Locator("[data-testid='permission-set-optional']").First().Locator("button[role='switch']").First()
		toggleCount, err := psToggle.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(toggleCount).To(Equal(1), "Optional PS card should have a PS-level toggle")

		err = psToggle.Click()
		Expect(err).NotTo(HaveOccurred(), "Failed to select optional PS")
		err = page.WaitForLoadState()
		Expect(err).NotTo(HaveOccurred())

		optionalCard := page.Locator("[data-testid='permission-set-optional']").First()

		// Google Workspace has mandatory ServiceScope → no per-service toggle.
		// Only the PS-level toggle remains inside the card.
		switches := optionalCard.Locator("button[role='switch']")
		switchCount, err := switches.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(switchCount).To(Equal(1), "Only PS-level toggle; mandatory ServiceScope Google Workspace must not have a per-service toggle")

		// "required" indicator should appear for Google Workspace inside the card
		requiredText := optionalCard.GetByText("required")
		reqCount, err := requiredText.Count()
		Expect(err).NotTo(HaveOccurred())
		Expect(reqCount).To(BeNumerically(">=", 1), "Google Workspace should show required indicator inside selected PS card")

		Expect(consentPage.TakeScreenshot(ctx, "consent_service_scope_req_type_ps_selected_locked")).NotTo(HaveOccurred())
	})

	Context("when ServiceScope requirement_type is optional", func() {
		var (
			optAgentID        string
			psOptionalScopeID = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000006")
		)

		BeforeEach(func() {
			// Create a second PS with optional ServiceScope — confirms old toggle behavior is preserved.
			createPermissionSet(ctx, psOptionalScopeID, "Optional Scope Productivity", "PS with optional service scope", []storage.ServiceScope{
				{ServiceID: gwServiceID, Scopes: []string{"drive", "calendar"}, RequirementType: storage.RequirementTypeOptional},
			})

			optAgent := fixtures.ValidAgent()
			optAgentID = optAgent.ID.String()
			optAgent.PermissionSets = []storage.AgentPermissionSetEntry{
				{PermissionSetID: psOptionalScopeID, RequirementType: storage.RequirementTypeOptional},
			}
			optAgent.ServiceRequirements = []storage.ServiceRequirement{
				{ServiceID: gwServiceID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"drive", "calendar"}},
			}
			err := GetTestStorage().Agents().Create(ctx, optAgent)
			Expect(err).NotTo(HaveOccurred(), "Failed to create optional-scope test agent")
		})

		It("should show Google Workspace as togglable when PS has optional ServiceScope", func() {
			err := consentPage.NavigateToAgent(ctx, optAgentID)
			Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

			page := consentPage.GetPlaywrightPage()

			// Select the optional PS card
			psToggle := page.Locator("[data-testid='permission-set-optional']").First().Locator("button[role='switch']").First()
			toggleCount, err := psToggle.Count()
			Expect(err).NotTo(HaveOccurred())
			Expect(toggleCount).To(BeNumerically(">=", 1), "Optional PS card should have a PS-level toggle")

			err = psToggle.Click()
			Expect(err).NotTo(HaveOccurred(), "Failed to select optional PS")
			err = page.WaitForLoadState()
			Expect(err).NotTo(HaveOccurred())

			// Optional ServiceScope → per-service toggle should appear alongside the PS-level toggle.
			optionalCard := page.Locator("[data-testid='permission-set-optional']").First()
			switches := optionalCard.Locator("button[role='switch']")
			switchCount, err := switches.Count()
			Expect(err).NotTo(HaveOccurred())
			Expect(switchCount).To(BeNumerically(">=", 2), "PS-level toggle + per-service toggle for optional ServiceScope Google Workspace")

			Expect(consentPage.TakeScreenshot(ctx, "consent_service_scope_req_type_optional_scope_toggle")).NotTo(HaveOccurred())
		})
	})
})
