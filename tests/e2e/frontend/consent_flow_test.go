// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the OAuth2 consent flow.
package e2e_test

import (
	"context"

	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ConsentFlow tests verify the OAuth2 consent flow where users grant scopes to agents.
var _ = Describe("Consent Flow", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	// Setup per-test resources in BeforeEach
	BeforeEach(func() {
		ctx = context.Background()

		// Step 1: Create a third-party OAuth2 service with scopes
		// This provides the scopes that can be delegated by the user
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
		createPermissionSet(ctx, fixtures.PlaceholderPermissionSetID, "GitHub Access", "Repository access for the consent-flow fixture", []storage.ServiceScope{{
			ServiceID:       service.ID,
			Scopes:          []string{"repo", "user"},
			RequirementType: storage.RequirementTypeMandatory,
		}})

		// Step 2: Create a test agent with service requirements
		// The agent's service requirements define what services can be delegated to it
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

		// Step 3: Create a grant linking the agent to the current user
		// This makes the agent appear in the user's consent page with delegated services/scopes
		principal := fixtures.DefaultPrincipal().String()
		grant := fixtures.IndefiniteGrant(principal, testAgentID, "550e8400-e29b-41d4-a716-446655440000", []string{"repo", "user"})
		err = GetTestStorage().UserGrants().Create(ctx, grant)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test grant")

		// Initialize the page object
		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	// Cleanup after each test
	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	Context("when agent has only optional service requirements", func() {
		BeforeEach(func() {
			// Create two optional services using fixtures; customize only the IDs to avoid
			// conflicts with the service created in the outer BeforeEach (…440000).
			githubService := fixtures.ServiceWithID("550e8400-e29b-41d4-a716-446655440001")
			githubService.ProtectedResources = []string{"https://api.github.example.com/consent-flow-optional"}
			err := GetTestStorage().Services().Create(ctx, githubService)
			Expect(err).NotTo(HaveOccurred(), "Failed to create optional GitHub service")

			gitlabService := fixtures.ServiceWithID("550e8400-e29b-41d4-a716-446655440002")
			gitlabService.ProtectedResources = []string{"https://api.gitlab.example.com/consent-flow-optional"}
			err = GetTestStorage().Services().Create(ctx, gitlabService)
			Expect(err).NotTo(HaveOccurred(), "Failed to create optional GitLab service")
			optionalPermissionSetID := id.NewPermissionSetID()
			createPermissionSet(ctx, optionalPermissionSetID, "Optional Services", "Optional service access for the consent-flow fixture", []storage.ServiceScope{
				{ServiceID: githubService.ID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
				{ServiceID: gitlabService.ID, Scopes: []string{"write"}, RequirementType: storage.RequirementTypeOptional},
			})
			principal := fixtures.DefaultPrincipal().String()
			Expect(GetTestStorage().UserSessions().Create(ctx, fixtures.SessionForService(principal, githubService.ID.String()))).To(Succeed())
			Expect(GetTestStorage().UserSessions().Create(ctx, fixtures.SessionForService(principal, gitlabService.ID.String()))).To(Succeed())

			// Create agent with both services as OPTIONAL requirements.
			// Use AnotherAgent() to avoid client_id conflict with the outer BeforeEach
			// which creates a ValidAgent() with ClientID "test-client-valid".
			agent := fixtures.AnotherAgent()
			testAgentID = agent.ID.String()
			agent.PermissionSets = []storage.AgentPermissionSetEntry{{
				PermissionSetID: optionalPermissionSetID,
				RequirementType: storage.RequirementTypeOptional,
			}}
			agent.ServiceRequirements = []storage.ServiceRequirement{
				{
					ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440001"),
					RequirementType: storage.RequirementTypeOptional,
					RequiredScopes:  []string{"read"},
				},
				{
					ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440002"),
					RequirementType: storage.RequirementTypeOptional,
					RequiredScopes:  []string{"write"},
				},
			}
			err = GetTestStorage().Agents().Create(ctx, agent)
			Expect(err).NotTo(HaveOccurred(), "Failed to create optional-only agent")
		})

		It("approves consent after selecting optional services", func() {
			err := consentPage.NavigateToAgent(ctx, testAgentID)
			Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

			err = consentPage.TogglePermissionSet(ctx, "Optional Services")
			Expect(err).NotTo(HaveOccurred(), "Failed to select optional services")

			err = consentPage.SubmitConsent(ctx)
			Expect(err).NotTo(HaveOccurred(), "SubmitConsent should succeed after selecting optional services")
			Expect(consentPage.WaitForNoValidationError(ctx, 2000)).To(Succeed(), "Expected no validation error after approving optional services")
		})
	})

	// Minimal test: Verify Playwright works and frontend renders
	It("should load consent page and display basic UI elements", func() {
		// When: Navigate to consent page for the test agent
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		// Then: Agent name heading should be visible
		agentName, err := consentPage.GetAgentName(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get agent name")
		Expect(agentName).NotTo(BeEmpty(), "Agent name should not be empty")

		// And: Available scopes should be present
		scopes, err := consentPage.GetAvailableScopes(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get available scopes")
		Expect(scopes).NotTo(BeEmpty(), "Should have at least one scope available")

		// And: Take screenshot for verification
		err = consentPage.TakeScreenshot(ctx, "consent_page_loaded")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Consent page renders correctly with UI elements visible")
	})

	// Scenario 10 from specs/007-consent-frontend/spec.md
	It("allows a user to set and read a future end date", func() {
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		err = consentPage.EnableSpecificEndDate(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to enable a specific end date")

		endDate := time.Now().AddDate(0, 1, 0)
		err = consentPage.SetExpirationDate(ctx, endDate)
		Expect(err).NotTo(HaveOccurred(), "Failed to set end date")

		actualEndDate, err := consentPage.GetExpirationDate(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get end date")
		Expect(actualEndDate).To(Equal(endDate.Format("2006-01-02")))
	})

	// Test: Verify scopes are displayed correctly
	// Note: In the current UI (Phase 8), scopes are displayed as read-only badges based on service requirements.
	// Service delegation happens at the service level (Login/Delegate buttons), not at individual scope level.
	It("should display service scopes as read-only badges based on requirements", func() {
		// Given: User navigates to consent page
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		// When: Get available scopes displayed on the page
		scopes, err := consentPage.GetAvailableScopes(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get scopes")
		Expect(scopes).NotTo(BeEmpty(), "Must have at least one scope")

		// Then: Verify expected scopes from service definition are present
		Expect(scopes).To(ContainElement("repo"), "Service should display 'repo' scope")
		Expect(scopes).To(ContainElement("user"), "Service should display 'user' scope")

		// And: Take screenshot for verification
		err = consentPage.TakeScreenshot(ctx, "service_scopes_displayed")
		Expect(err).NotTo(HaveOccurred(), "Failed to take screenshot")

		GetLogger().Info("Test passed: Service scopes are displayed correctly as read-only badges",
			"scopes_count", len(scopes),
			"scopes", scopes,
		)
	})

})
