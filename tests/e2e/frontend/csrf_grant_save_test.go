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
		ctx                 context.Context
		consentPage         *pages.ConsentPage
		testAgentID         id.AgentID
		testPermissionSetID id.PermissionSetID
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
		testPermissionSetID = id.NewPermissionSetID()
		err = GetTestStorage().PermissionSets().Create(ctx, &storage.PermissionSet{
			ID:          testPermissionSetID,
			Name:        "Grant Save Permission Set",
			Description: "Required services for the grant-save flow",
			ServiceScopes: []storage.ServiceScope{
				{ServiceID: grantTestServiceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory},
				{ServiceID: grantTestService2ID, Scopes: []string{"email"}, RequirementType: storage.RequirementTypeMandatory},
			},
		})
		Expect(err).NotTo(HaveOccurred(), "Failed to create test permission set")

		agent := fixtures.AgentWithClientID("grant-flow-test-client")
		testAgentID = agent.ID
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
			PermissionSetID: testPermissionSetID,
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

	It("should successfully save a grant through the consent screen", func() {
		// specs/007-consent-frontend/spec.md — User Story 3, Scenario 7:
		// The console requires an explicit draft edit before saving (047 AS-08).
		err := consentPage.NavigateToAgent(ctx, testAgentID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to consent page")

		agentName, err := consentPage.GetAgentName(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to get agent name")
		Expect(agentName).NotTo(BeEmpty(), "Agent name should be displayed")

		Expect(consentPage.IsSaveBarVisible(ctx)).To(BeFalse())
		Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())
		Expect(consentPage.IsSaveBarVisible(ctx)).To(BeTrue(), "Save appears only after the duration edit")
		Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeTrue())
		err = consentPage.SaveChanges(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to save the edited grant")

		err = consentPage.WaitForGrantSuccess(ctx)
		Expect(err).NotTo(HaveOccurred(), "Grant save should succeed")
		grant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(fixtures.DefaultPrincipal().String()), testAgentID)
		Expect(err).NotTo(HaveOccurred())
		Expect(grant).NotTo(BeNil())
		Expect(grant.GrantedPermissionSets).To(ConsistOf(And(
			HaveField("PermissionSetID", testPermissionSetID),
			HaveField("IncludedServiceIDs", ConsistOf(grantTestServiceID, grantTestService2ID)),
		)))
		Expect(grant.ValidUntil).NotTo(BeNil(), "the edited duration must be persisted")
		Expect(consentPage.IsSaveBarVisible(ctx)).To(BeFalse())
	})

	It("should save grant when redirect_uri parameter is present", func() {
		// User Story 3, Scenario 7 from specs/007-consent-frontend/spec.md.
		// Without session_token, redirect_uri does not turn a direct grant save into an OAuth2 redirect.
		err := consentPage.NavigateToAgentWithRedirectURI(ctx, testAgentID.String(), "/oauth2/callback?code=abc")
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate with redirect_uri")

		agentName, err := consentPage.GetAgentName(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(agentName).NotTo(BeEmpty())

		Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())
		Expect(consentPage.IsSaveBarVisible(ctx)).To(BeTrue())
		err = consentPage.SaveChanges(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to save grant with redirect_uri")

		Expect(consentPage.WaitForGrantSuccess(ctx)).To(Succeed(),
			"Direct grant save should display success even with redirect_uri in the page URL")
		hasError, err := consentPage.HasError(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasError).To(BeFalse(), "No validation error should remain after saving the grant")
		Expect(consentPage.GetURLQueryParam("redirect_uri")).To(Equal("/oauth2/callback?code=abc"),
			"A direct console save must not follow an untrusted redirect_uri")
		grant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(fixtures.DefaultPrincipal().String()), testAgentID)
		Expect(err).NotTo(HaveOccurred())
		Expect(grant).NotTo(BeNil())
		Expect(grant.GrantedPermissionSets).To(ConsistOf(And(
			HaveField("PermissionSetID", testPermissionSetID),
			HaveField("IncludedServiceIDs", ConsistOf(grantTestServiceID, grantTestService2ID)),
		)))
		Expect(grant.ValidUntil).NotTo(BeNil())
	})
})
