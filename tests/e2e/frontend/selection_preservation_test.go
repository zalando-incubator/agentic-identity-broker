// Package e2e_test provides end-to-end tests for compact consent-selection
// references across third-party OAuth2 login redirects.
package e2e_test

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	consentStateIDPattern                    = "^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
	selectionPreservationLargeSelectionCount = 250
)

var (
	selGitHubServiceID = id.MustParseServiceID("d0000000-0000-0000-0000-000000000001")
	selGoogleServiceID = id.MustParseServiceID("d0000000-0000-0000-0000-000000000002")
	selSlackServiceID  = id.MustParseServiceID("d0000000-0000-0000-0000-000000000003")

	selMandatoryPSID = id.MustParsePermissionSetID("e0000000-0000-0000-0000-000000000001")
	selOptionalPSID  = id.MustParsePermissionSetID("e0000000-0000-0000-0000-000000000002")
)

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

func selectOptionalPermissionSet(ctx context.Context, consentPage *pages.ConsentPage) {
	Expect(consentPage.TogglePermissionSet(ctx, "Productivity Suite")).To(Succeed())
	Expect(consentPage.WaitForServiceToAppear(ctx, "Google", 5000)).To(Succeed())
	Expect(consentPage.WaitForServiceToAppear(ctx, "Slack", 5000)).To(Succeed())
}

func expectOptionalSelectionRestored(ctx context.Context, consentPage *pages.ConsentPage, serviceNames ...string) {
	selected, err := consentPage.IsPermissionSetSelected(ctx, "Productivity Suite")
	Expect(err).NotTo(HaveOccurred())
	Expect(selected).To(BeTrue())
	for _, serviceName := range serviceNames {
		Expect(consentPage.WaitForServiceToAppear(ctx, serviceName, 5000)).To(Succeed())
	}
}
func addLargeActiveSelectionMap(ctx context.Context, agent *storage.Agent) {
	permissionSets := make([]storage.AgentPermissionSetEntry, selectionPreservationLargeSelectionCount)
	for index := range permissionSets {
		permissionSetID := id.NewPermissionSetID()
		permissionSet := selPreservationPermissionSet(
			permissionSetID,
			fmt.Sprintf("Large Selection %d", index),
			"Ensures the current-tab selection map exceeds the legacy URL transport limit",
			[]storage.ServiceScope{
				{ServiceID: selGoogleServiceID, Scopes: []string{"calendar"}, RequirementType: storage.RequirementTypeOptional},
			},
		)
		Expect(GetTestStorage().PermissionSets().Create(ctx, permissionSet)).To(Succeed())
		permissionSets[index] = storage.AgentPermissionSetEntry{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeMandatory,
		}
	}
	agent.PermissionSets = append(agent.PermissionSets, permissionSets...)
	Expect(GetTestStorage().Agents().Update(ctx, agent)).To(Succeed())
}

var _ = Describe("Selection Preservation Across OAuth2 Redirect", func() {
	var (
		ctx         context.Context
		consentPage *pages.ConsentPage
		testAgentID string
		testAgent   *storage.Agent
	)

	BeforeEach(func() {
		ctx = context.Background()

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
			Expect(GetTestStorage().Services().Create(ctx, svc)).To(Succeed())
		}

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
		Expect(GetTestStorage().PermissionSets().Create(ctx, mandatoryPS)).To(Succeed())
		Expect(GetTestStorage().PermissionSets().Create(ctx, optionalPS)).To(Succeed())

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
		testAgent = agent
		Expect(GetTestStorage().Agents().Create(ctx, testAgent)).To(Succeed())

		consentPage = pages.NewConsentPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if consentPage != nil {
			_ = consentPage.Close()
		}
	})

	// Feature 008 User Story 5 Amendment, Scenario 1: Compact reference creation
	It("keeps a large selection map compact through the provider callback", func() {
		addLargeActiveSelectionMap(ctx, testAgent)

		config := fixtures.OAuth2ConfigWithUpstream(GetMockUpstream().URL())
		serverFactory := bootstrap.NewServerFactory(config, GetLogger())
		serverBuilder, err := bootstrap.NewTestServerBuilder(config, GetTestStorage(), serverFactory, GetLogger())
		Expect(err).NotTo(HaveOccurred())
		alignedServer, err := serverBuilder.Build()
		Expect(err).NotTo(HaveOccurred())
		defer alignedServer.Close()

		consentPage = pages.NewConsentPage(GetTestPage(), alignedServer.BaseURL())
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		selectOptionalPermissionSet(ctx, consentPage)
		GetMockUpstream().WithSuccessfulTokenResponse()

		page := consentPage.GetPlaywrightPage()
		previousAuthorizeURL := GetMockUpstream().GetLastAuthorizeURL()
		Expect(consentPage.DelegateService(ctx, "Google")).To(Succeed())
		var upstreamState string
		Eventually(func() string {
			lastAuthorizeURL := GetMockUpstream().GetLastAuthorizeURL()
			if lastAuthorizeURL == previousAuthorizeURL {
				return ""
			}
			authorizeURL, err := url.Parse(lastAuthorizeURL)
			if err != nil {
				return ""
			}
			upstreamState = authorizeURL.Query().Get("state")
			return upstreamState
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())
		Expect(len(upstreamState)).To(BeNumerically("<", 6000))
		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 100*time.Millisecond).Should(ContainSubstring("success=true"))
		Eventually(func() string {
			stateID, _ := consentPage.GetURLQueryParam("consent_state_id")
			return stateID
		}, 10*time.Second, 100*time.Millisecond).Should(BeEmpty())
		legacyState, err := consentPage.GetURLQueryParam("consent_state")
		Expect(err).NotTo(HaveOccurred())
		Expect(legacyState).To(BeEmpty())
		expectOptionalSelectionRestored(ctx, consentPage, "Slack")
	})

	// Feature 008 User Story 5 Amendment, Scenario 2: Same-tab restoration
	It("restores selected permission sets from the captured current-tab reference", func() {
		Expect(consentPage.NavigateToAgent(ctx, testAgentID)).To(Succeed())
		selectOptionalPermissionSet(ctx, consentPage)

		page := consentPage.GetPlaywrightPage()
		var capturedLoginURL, postData, loginMethod string
		Expect(page.Route("**/api/third-party/*/oauth2/authorize*", func(route playwright.Route) {
			request := route.Request()
			capturedLoginURL = request.URL()
			loginMethod = request.Method()
			var err error
			postData, err = request.PostData()
			Expect(err).NotTo(HaveOccurred())
			Expect(loginMethod).To(Equal("POST"))
			Expect(postData).NotTo(BeEmpty())
			form, err := url.ParseQuery(postData)
			Expect(err).NotTo(HaveOccurred())
			returnURL, err := url.Parse(form.Get("redirect_uri"))
			Expect(err).NotTo(HaveOccurred())
			callbackQuery := returnURL.Query()
			callbackQuery.Set("success", "true")
			callbackQuery.Set("service_id", selGoogleServiceID.String())
			callbackQuery.Set("consent_state_id", form.Get("consent_state_id"))
			returnURL.RawQuery = callbackQuery.Encode()
			status := 302
			Expect(route.Fulfill(playwright.RouteFulfillOptions{
				Status:  &status,
				Headers: map[string]string{"Location": returnURL.String()},
			})).To(Succeed())
		})).To(Succeed())

		Expect(consentPage.DelegateService(ctx, "Google")).To(Succeed())
		Eventually(func() string {
			return capturedLoginURL
		}, 5*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())
		Eventually(func() string {
			return page.URL()
		}, 5*time.Second, 100*time.Millisecond).Should(ContainSubstring("success=true"))
		Eventually(func() string {
			stateID, _ := consentPage.GetURLQueryParam("consent_state_id")
			return stateID
		}, 5*time.Second, 100*time.Millisecond).Should(BeEmpty())

		loginURL, err := url.Parse(capturedLoginURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(loginURL.Query().Get("consent_state_id")).To(BeEmpty())
		Expect(loginMethod).To(Equal("POST"))

		form, err := url.ParseQuery(postData)
		Expect(err).NotTo(HaveOccurred())
		stateID := form.Get("consent_state_id")
		Expect(stateID).To(MatchRegexp(consentStateIDPattern))
		returnURL, err := url.Parse(form.Get("redirect_uri"))
		Expect(err).NotTo(HaveOccurred())
		Expect(returnURL.Query().Get("consent_state_id")).To(BeEmpty())
		expectOptionalSelectionRestored(ctx, consentPage, "Google", "Slack")
		Expect(consentPage.TogglePermissionSet(ctx, "Productivity Suite")).To(Succeed())
		Eventually(func() (bool, error) {
			return consentPage.IsPermissionSetSelected(ctx, "Productivity Suite")
		}, 5*time.Second, 100*time.Millisecond).Should(BeFalse())
		_, err = page.Reload()
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() (bool, error) {
			return consentPage.IsPermissionSetSelected(ctx, "Productivity Suite")
		}, 5*time.Second, 100*time.Millisecond).Should(BeFalse())
	})

	// Feature 008 User Story 5 Amendment, Scenario 3: Complete provider callback
	It("preserves authorization session callback parameters and restores selections", func() {
		config := fixtures.OAuth2ConfigWithUpstream(GetMockUpstream().URL())
		serverFactory := bootstrap.NewServerFactory(config, GetLogger())
		serverBuilder, err := bootstrap.NewTestServerBuilder(config, GetTestStorage(), serverFactory, GetLogger())
		Expect(err).NotTo(HaveOccurred())
		alignedServer, err := serverBuilder.Build()
		Expect(err).NotTo(HaveOccurred())
		defer alignedServer.Close()

		agentID, err := id.ParseAgentID(testAgentID)
		Expect(err).NotTo(HaveOccurred())
		claims, err := sessiontoken.NewAuthorizationSessionClaims(
			agentID,
			id.Principal(fixtures.DefaultPrincipal().String()),
			"/oauth2/authorize?client_id=selection-test&redirect_uri=https%3A%2F%2Fclient.example.com%2Fcallback&response_type=code&state="+strings.Repeat("client-state-", 350),
			nil,
		)
		Expect(err).NotTo(HaveOccurred())
		sessionToken, err := alignedServer.App().SessionTokenService.Create(claims)
		Expect(err).NotTo(HaveOccurred())

		consentPage = pages.NewConsentPage(GetTestPage(), alignedServer.BaseURL())
		Expect(consentPage.NavigateToAgentWithSessionToken(ctx, testAgentID, sessionToken)).To(Succeed())
		selectOptionalPermissionSet(ctx, consentPage)
		GetMockUpstream().WithSuccessfulTokenResponse()

		page := consentPage.GetPlaywrightPage()
		previousAuthorizeURL := GetMockUpstream().GetLastAuthorizeURL()
		Expect(consentPage.DelegateService(ctx, "Google")).To(Succeed())
		var upstreamState string
		Eventually(func() string {
			lastAuthorizeURL := GetMockUpstream().GetLastAuthorizeURL()
			if lastAuthorizeURL == previousAuthorizeURL {
				return ""
			}
			authorizeURL, parseErr := url.Parse(lastAuthorizeURL)
			if parseErr != nil {
				return ""
			}
			upstreamState = authorizeURL.Query().Get("state")
			return upstreamState
		}, 10*time.Second, 100*time.Millisecond).ShouldNot(BeEmpty())
		Expect(len(upstreamState)).To(BeNumerically("<", 6000))
		stateClaims, err := alignedServer.App().OAuth2SessionService.ValidateStateToken(
			upstreamState, id.Principal(fixtures.DefaultPrincipal().String()), selGoogleServiceID,
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(stateClaims.RedirectURI).To(Equal(alignedServer.BaseURL() + "/agents/" + testAgentID))
		Expect(stateClaims.ConsentStateID).To(MatchRegexp(consentStateIDPattern))
		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 100*time.Millisecond).Should(ContainSubstring("success=true"))
		Eventually(func() string {
			returnedSessionToken, _ := consentPage.GetURLQueryParam("session_token")
			return returnedSessionToken
		}, 10*time.Second, 100*time.Millisecond).Should(Equal(sessionToken))
		success, err := consentPage.GetURLQueryParam("success")
		Expect(err).NotTo(HaveOccurred())
		Expect(success).To(Equal("true"))
		serviceID, err := consentPage.GetURLQueryParam("service_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(serviceID).To(Equal(selGoogleServiceID.String()))
		selectionStateID, err := consentPage.GetURLQueryParam("consent_state_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(selectionStateID).To(BeEmpty())
		legacyState, err := consentPage.GetURLQueryParam("consent_state")
		Expect(err).NotTo(HaveOccurred())
		Expect(legacyState).To(BeEmpty())
		expectOptionalSelectionRestored(ctx, consentPage, "Slack")
	})
})
