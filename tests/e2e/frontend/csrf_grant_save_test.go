// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file tests the complete consent grant save flow including cross-origin protection,
// verifying that the full agent → broker → consent → save cycle works correctly.
package e2e_test

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

// Consent Grant Save Flow tests verify that users can navigate to the consent screen
// and successfully save grants. Cross-origin protection via http.CrossOriginProtection
// is transparent to same-origin browser requests. This covers the full
// agent → broker → consent → save cycle.
var _ = Describe("Consent Grant Save Flow", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
	)

	// Well-known IDs for this test's fixtures
	var (
		grantTestServiceID  = id.MustParseServiceID("d0000000-0000-0000-0000-000000000001")
		grantTestService2ID = id.MustParseServiceID("d0000000-0000-0000-0000-000000000002")
	)

	BeforeEach(func() {
		ctx = context.Background()

		svc := &model.ThirdpartyOAuth2ProviderEntity{
			ID:          grantTestServiceID,
			DisplayName: "Grant Test Service",
			ClientID:    id.ClientID("grant-test-client"),
			Secret:      fixtures.EncryptedSecret(grantTestServiceID.String(), "grant-test-secret"),
			IssuerURI:   "https://grant-test.example.com",
			Endpoints: model.OAuth2Endpoints{
				AuthorizeEndpoint: "https://grant-test.example.com/authorize",
				TokenEndpoint:     "https://grant-test.example.com/token",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "read", Description: "Read access"},
				{ScopeValue: "write", Description: "Write access"},
			},
		}
		err := GetTestStorage().Services().Create(ctx, svc)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test service")

		svc2 := &model.ThirdpartyOAuth2ProviderEntity{
			ID:          grantTestService2ID,
			DisplayName: "Grant Test Service 2",
			ClientID:    id.ClientID("grant-test-client-2"),
			Secret:      fixtures.EncryptedSecret(grantTestService2ID.String(), "grant-test-secret-2"),
			IssuerURI:   "https://grant-test2.example.com",
			Endpoints: model.OAuth2Endpoints{
				AuthorizeEndpoint: "https://grant-test2.example.com/authorize",
				TokenEndpoint:     "https://grant-test2.example.com/token",
			},
			Scopes: []model.OAuthScope{
				{ScopeValue: "email", Description: "Email access"},
			},
		}
		err = GetTestStorage().Services().Create(ctx, svc2)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test service 2")
		permissionSetID := id.NewPermissionSetID()
		err = GetTestStorage().PermissionSets().Create(ctx, &storage.PermissionSet{
			ID:          permissionSetID,
			Name:        "Grant Save Permission Set",
			Description: "Required services for the grant-save flow",
			ServiceScopes: []storage.ServiceScope{
				{ServiceID: grantTestServiceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory},
				{ServiceID: grantTestService2ID, Scopes: []string{"email"}, RequirementType: storage.RequirementTypeMandatory},
			},
		})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test permission set")

		agent := fixtures.AgentWithClientID("grant-flow-test-client")
		testAgentID = agent.ID.String()
		agent.ServiceRequirements = []storage.ServiceRequirement{
			{
				ServiceID:       grantTestServiceID,
				RequirementType: storage.RequirementTypeOptional,
				RequiredScopes:  []string{"read"},
			},
			{
				ServiceID:       grantTestService2ID,
				RequirementType: storage.RequirementTypeOptional,
				RequiredScopes:  []string{"email"},
			},
		}
		agent.PermissionSets = []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeMandatory,
		}}
		err = GetTestStorage().Agents().Create(ctx, agent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")
		for _, serviceID := range []id.ServiceID{grantTestServiceID, grantTestService2ID} {
			err = GetTestStorage().UserSessions().Create(ctx, &storage.UserSession{
				ID:                   id.NewSessionID(),
				Principal:            id.Principal(fixtures.DefaultPrincipal().String()),
				ServiceID:            serviceID,
				EncryptedAccessToken: []byte("opaque-test-token"),
				TokenType:            "Bearer",
				EncryptionContext:    storage.EncryptionContext{ServiceID: serviceID},
			})
			Expect(err).NotTo(HaveOccurred(), "Failed to create test session")
		}

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	It("should successfully save a grant through the consent screen", func() {
		// specs/007-consent-frontend/spec.md — User Story 3, Scenario 7:
		// "Given a user clicks Approve & Delegate, When the request is processed successfully,
		// Then the system creates or updates the grant and displays a success message."
		err := consentPage.NavigateToAgent(ctx, testAgentID)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		agentName, err := consentPage.GetAgentName(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get agent name")
		Expect(agentName).NotTo(BeEmpty(), "Agent name should be displayed")

		err = consentPage.SubmitConsent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click Approve & Delegate button")

		err = consentPage.WaitForGrantSuccess(ctx, 5000)
		Expect(err).NotTo(HaveOccurred(), "Grant save should succeed")
	})

	It("should save grant when redirect_uri parameter is present", func() {
		// specs/011-agent-permission-requirements/spec.md — User Story 6, Scenario 1:
		// "Given a user is on the consent screen with a redirect_uri query parameter,
		// When the user clicks the Approve button, Then the backend issues an HTTP redirect
		// to the URL specified in redirect_uri."
		err := consentPage.NavigateToAgentWithRedirectURI(ctx, testAgentID, "/oauth2/callback?code=abc")
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate with redirect_uri")

		agentName, err := consentPage.GetAgentName(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(agentName).NotTo(BeEmpty())

		err = consentPage.SubmitConsent(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to submit consent with redirect_uri")

		Expect(consentPage.WaitForNoValidationError(ctx, 3000)).To(Succeed(),
			"No error should appear after grant save with redirect_uri")
	})
})
