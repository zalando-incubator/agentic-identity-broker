// Package e2e_test tests permission-group selection and service requirements.
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

func expectPermissionGroupSelection(ctx context.Context, consentPage *pages.ConsentPage, name string, checked bool) {
	GinkgoHelper()
	Eventually(func() ([]pages.PermissionGroup, error) {
		return consentPage.PermissionGroups(ctx)
	}).Should(ContainElement(And(HaveField("Name", name), HaveField("Checked", checked))))
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

	// AS-08 from specs/047-redesign-consent-console/spec.md.
	It("should keep the required permission group selected and read-only", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		groups, err := consentPage.PermissionGroups(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(ContainElement(And(
			HaveField("Name", "Code Access"), HaveField("Required", true),
			HaveField("Checked", true), HaveField("ReadOnly", true),
		)))
	})

	// AS-08 from specs/047-redesign-consent-console/spec.md.
	It("should leave optional access unselected and editable", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		groups, err := consentPage.PermissionGroups(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(ContainElement(And(
			HaveField("Name", "Productivity Suite"), HaveField("Required", false),
			HaveField("Checked", false), HaveField("ReadOnly", false),
		)))
	})

	// US2-S2 from specs/019-permission-sets/spec.md.
	It("should show only each permission group's intersecting services", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.ExpandPermissionGroup(ctx, "Code Access")).To(Succeed())
		Expect(consentPage.GroupServices(ctx, "Code Access")).To(ConsistOf("GitHub"))
		Expect(consentPage.ExpandPermissionGroup(ctx, "Productivity Suite")).To(Succeed())
		Expect(consentPage.GroupServices(ctx, "Productivity Suite")).To(ConsistOf("Google", "Microsoft"))
	})

	// US2-S5 from specs/019-permission-sets/spec.md.
	It("should require connections only for selected optional access", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", false)
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Google")).To(BeFalse())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Microsoft")).To(BeFalse())

		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", true)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Google")).To(BeTrue())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Microsoft")).To(BeTrue())

		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", false)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", false)
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Google")).To(BeFalse())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Microsoft")).To(BeFalse())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "GitHub")).To(BeTrue())
	})

	Context("when the required service is already connected", func() {
		BeforeEach(func() {
			principal := fixtures.DefaultPrincipal().String()
			grant := fixtures.IndefiniteGrant(principal, testAgentID, psGitHubServiceID.String(), []string{"repo", "user"})
			grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{
				PermissionSetID: psMandatoryID, IncludedServiceIDs: []id.ServiceID{psGitHubServiceID},
			}}
			Expect(GetTestStorage().UserGrants().Create(ctx, grant)).To(Succeed())
			Expect(GetTestStorage().UserSessions().Create(ctx, fixtures.SessionForService(principal, psGitHubServiceID.String()))).To(Succeed())
		})

		// US2-S3 from specs/019-permission-sets/spec.md; AS-08 from specs/047-redesign-consent-console/spec.md.
		It("should allow a deliberate edit without reconnecting the service", func() {
			Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
			Expect(consentPage.HasServiceConnectPrompt(ctx, "GitHub")).To(BeFalse())
			Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeTrue())
			Expect(consentPage.HasServiceConnectPrompt(ctx, "GitHub")).To(BeFalse())
		})

		// AS-08 from specs/047-redesign-consent-console/spec.md.
		It("should show Save only for edits and discard them on Cancel", func() {
			Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
			Expect(consentPage.IsSaveBarVisible(ctx)).To(BeFalse())
			Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())
			Expect(consentPage.IsSaveBarVisible(ctx)).To(BeTrue())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeTrue())
			Expect(consentPage.CancelChanges(ctx)).To(Succeed())
			Expect(consentPage.IsSaveBarVisible(ctx)).To(BeFalse())
			Expect(consentPage.SelectedDuration(ctx)).To(Equal("Until revoked"))
			expectPermissionGroupSelection(ctx, consentPage, "Code Access", true)
			expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", false)
		})
	})

	// US2-S6 from specs/019-permission-sets/spec.md; AS-08 from specs/047-redesign-consent-console/spec.md.
	It("should block saving an edit until required services are connected", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.ChooseDuration(ctx, "30 days")).To(Succeed())
		Expect(consentPage.IsSaveBarVisible(ctx)).To(BeTrue())
		Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeFalse())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "GitHub")).To(BeTrue())
	})
})

// AS-08 from specs/047-redesign-consent-console/spec.md.
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

	It("should keep every required permission group selected and read-only", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		groups, err := consentPage.PermissionGroups(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(ConsistOf(
			And(HaveField("Name", "Code Access"), HaveField("Required", true), HaveField("Checked", true), HaveField("ReadOnly", true)),
			And(HaveField("Name", "Calendar Access"), HaveField("Required", true), HaveField("Checked", true), HaveField("ReadOnly", true)),
		))
	})

	It("should prevent deselecting required services in either group", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.PermissionServices(ctx, "Code Access")).To(Equal([]pages.PermissionService{
			{Name: "GitHub", Required: true, ReadOnly: true, Checked: true},
		}))
		Expect(consentPage.PermissionServices(ctx, "Calendar Access")).To(Equal([]pages.PermissionService{
			{Name: "Google", Required: true, ReadOnly: true, Checked: true},
		}))
	})
})

// AS-08 from specs/047-redesign-consent-console/spec.md.
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

	It("should leave all optional permission groups unselected and editable", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		groups, err := consentPage.PermissionGroups(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(groups).To(ConsistOf(
			And(HaveField("Name", "Productivity Suite"), HaveField("Required", false), HaveField("Checked", false), HaveField("ReadOnly", false)),
			And(HaveField("Name", "Communication Tools"), HaveField("Required", false), HaveField("Checked", false), HaveField("ReadOnly", false)),
		))
	})

	It("should allow selecting and clearing optional groups independently", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", true)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		expectPermissionGroupSelection(ctx, consentPage, "Communication Tools", false)
		Expect(consentPage.SetPermissionGroupChecked(ctx, "Communication Tools", true)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		expectPermissionGroupSelection(ctx, consentPage, "Communication Tools", true)
		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", false)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", false)
		expectPermissionGroupSelection(ctx, consentPage, "Communication Tools", true)
	})
})

// US2-S4 from specs/019-permission-sets/spec.md.
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

	It("should lock required services without locking optional services in the same group", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.PermissionServices(ctx, "Development Tools")).To(ConsistOf(
			pages.PermissionService{Name: "GitHub", Required: true, ReadOnly: true, Checked: true},
			pages.PermissionService{Name: "Slack", Checked: true},
		))
	})

	It("should allow excluding an optional service without removing required access", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.SetPermissionServiceChecked(ctx, "Development Tools", "Slack", false)).To(Succeed())
		Expect(consentPage.PermissionServices(ctx, "Development Tools")).To(ConsistOf(
			pages.PermissionService{Name: "GitHub", Required: true, ReadOnly: true, Checked: true},
			pages.PermissionService{Name: "Slack"},
		))
		expectPermissionGroupSelection(ctx, consentPage, "Development Tools", true)
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Slack")).To(BeFalse())
		Expect(consentPage.HasServiceConnectPrompt(ctx, "GitHub")).To(BeTrue())
	})

	It("should apply service requirements when optional access is selected", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.SetPermissionGroupChecked(ctx, "Productivity Suite", true)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Productivity Suite", true)
		Expect(consentPage.PermissionServices(ctx, "Productivity Suite")).To(ConsistOf(
			pages.PermissionService{Name: "Google", Required: true, ReadOnly: true, Checked: true},
			pages.PermissionService{Name: "Microsoft", Checked: true},
		))
	})

})

// US2-S4 from specs/019-permission-sets/spec.md.
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

	It("should not mark an unselected optional group's service as required access", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		expectPermissionGroupSelection(ctx, consentPage, "Access to core productivity tools", false)
		Expect(consentPage.PermissionServices(ctx, "Access to core productivity tools")).To(Equal([]pages.PermissionService{
			{Name: "Google Workspace", ReadOnly: true},
		}))
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Google Workspace")).To(BeFalse())
	})

	It("should lock a mandatory ServiceScope even when the agent service requirement is optional", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		Expect(consentPage.SetPermissionGroupChecked(ctx, "Access to core productivity tools", true)).To(Succeed())
		Expect(consentPage.PermissionServices(ctx, "Access to core productivity tools")).To(Equal([]pages.PermissionService{
			{Name: "Google Workspace", Required: true, ReadOnly: true, Checked: true},
		}))
		Expect(consentPage.HasServiceConnectPrompt(ctx, "Google Workspace")).To(BeTrue())
	})

	Context("when ServiceScope requirement_type is optional", func() {
		var (
			optAgentID        string
			psOptionalScopeID = id.MustParsePermissionSetID("c0000000-0000-0000-0000-000000000006")
		)

		BeforeEach(func() {
			// Both the permission-set scope and the agent service requirement are optional.
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

		It("should allow excluding a service when both requirement sources are optional", func() {
			Expect(consentPage.NavigateToAgent(ctx, optAgentID)).To(Succeed())
			Expect(consentPage.SetPermissionGroupChecked(ctx, "Optional Scope Productivity", true)).To(Succeed())
			Expect(consentPage.PermissionServices(ctx, "Optional Scope Productivity")).To(Equal([]pages.PermissionService{
				{Name: "Google Workspace", Checked: true},
			}))
			Expect(consentPage.SetPermissionServiceChecked(ctx, "Optional Scope Productivity", "Google Workspace", false)).To(Succeed())
			Expect(consentPage.PermissionServices(ctx, "Optional Scope Productivity")).To(Equal([]pages.PermissionService{
				{Name: "Google Workspace"},
			}))
			Expect(consentPage.HasServiceConnectPrompt(ctx, "Google Workspace")).To(BeFalse())
		})
	})
})
