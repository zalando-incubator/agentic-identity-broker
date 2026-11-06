package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
)

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization UI", func() {
	var (
		ctx       context.Context
		server    *bootstrap.TestServer
		cast      *fixtures.IncrementalCast
		principal string
	)

	BeforeEach(func() {
		ctx = context.Background()
		principal = fixtures.DefaultPrincipal().String()
		GetMockUpstream().WithSuccessfulTokenResponse().WithAccessToken("synthetic-github-access").WithTokenScope("repo user:email")
		config := fixtures.OAuth2ConfigWithTokenExchange(GetMockUpstream().URL())
		builder, err := bootstrap.NewTestServerBuilder(config, GetTestStorage(), bootstrap.NewServerFactory(config, GetLogger()), GetLogger())
		Expect(err).NotTo(HaveOccurred())
		server, err = builder.Build()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(server.Close)

		cast, err = fixtures.SeedIncrementalCast(ctx, GetTestStorage(), principal, GetMockUpstream().URL())
		Expect(err).NotTo(HaveOccurred())
		cast.Agent.ClientID = ptr.To(id.ClientID("data-analysis-assistant"))
		Expect(GetTestStorage().Agents().Update(ctx, cast.Agent)).To(Succeed())
	})

	githubScope := func() string { return server.BaseURL() + "/sessions/github" }

	authorizeToConsent := func(scope string) string {
		response, err := server.AuthenticatedGET("/oauth2/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {cast.Agent.ID.String()},
			"redirect_uri": {"https://client.example.com/cb"}, "state": {"incremental-ui"}, "scope": {scope},
		}.Encode(), principal)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := helpers.ExtractRedirectURL(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/agents/" + cast.Agent.ID.String()))
		return location.Query().Get("session_token")
	}

	exchange := func() int {
		now := time.Now()
		sign := func(claims map[string]any) string {
			token, err := helpers.SignTestJWT(claims, GetMockUpstream().GetPrivateKeyPEM())
			Expect(err).NotTo(HaveOccurred())
			return token
		}
		form := url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {sign(map[string]any{"sub": principal, "azp": cast.Agent.ID.String(), "iss": GetMockUpstream().URL(), "aud": "token-exchange-broker", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()})},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {sign(map[string]any{"sub": "test-gateway-client", "iss": GetMockUpstream().URL(), "aud": "token-exchange-broker", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()})},
			"resource":              {"https://api.github.com"},
		}
		response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		var body map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
		return response.StatusCode
	}

	saveEnabled := func(consentPage *pages.ConsentPage) func() bool {
		return func() bool {
			enabled, err := consentPage.IsSaveButtonEnabled(ctx)
			return err == nil && enabled
		}
	}

	Context("when the grant omits GitHub and Alice has no GitHub session", func() {
		BeforeEach(func() {
			cast.Grant.GrantedPermissionSets = cast.Grant.GrantedPermissionSets[:1]
			Expect(GetTestStorage().UserGrants().Create(ctx, cast.Grant)).To(Succeed())
		})

		// US1 S4 from specs/055-incremental-service-authorization/spec.md.
		It("lets the user choose covering sets, log in, and save without auto-selecting other optional sets, then the retried call succeeds", func() {
			Expect(exchange()).To(Equal(http.StatusForbidden))
			sessionToken := authorizeToConsent("openid " + githubScope())
			consentPage := pages.NewConsentPage(GetTestPage(), server.BaseURL())
			Expect(consentPage.NavigateToAgentWithSessionToken(ctx, cast.Agent.ID.String(), sessionToken)).To(Succeed())
			Expect(consentPage.WaitForRequestedService(ctx, "GitHub Production")).To(Succeed())
			endDate, err := consentPage.GetExpirationDate(ctx)
			Expect(err).NotTo(HaveOccurred())
			for _, name := range []string{"GitHub Read Access", "GitHub Issue Triage"} {
				Expect(consentPage.IsRequestedCandidate(ctx, name)).To(BeTrue())
				Expect(consentPage.IsPermissionSetSelected(ctx, name)).To(BeFalse())
			}
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeFalse())
			Expect(consentPage.SaveDisabledReason(ctx)).To(ContainSubstring("GitHub Production"))

			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Issue Triage")).To(Succeed())
			Expect(consentPage.SaveDisabledReason(ctx)).To(ContainSubstring("user:email"))
			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Read Access")).To(Succeed())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeFalse(), "a missing session keeps Save disabled")
			Expect(consentPage.TakeRequestedServiceScreenshot(ctx, "consent_requested_service_needs_login")).To(Succeed())

			Expect(consentPage.LoginRequestedService(ctx, "GitHub Production")).To(Succeed())
			Eventually(func() string {
				success, _ := consentPage.GetURLQueryParam("success")
				return success
			}, 10*time.Second, 100*time.Millisecond).Should(Equal("true"))
			Expect(consentPage.WaitForRequestedService(ctx, "GitHub Production")).To(Succeed())
			Expect(consentPage.IsPermissionSetSelected(ctx, "GitHub Read Access")).To(BeTrue())
			Expect(consentPage.IsPermissionSetSelected(ctx, "GitHub Issue Triage")).To(BeTrue())
			Expect(consentPage.GetExpirationDate(ctx)).To(Equal(endDate))
			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Issue Triage")).To(Succeed())
			Eventually(saveEnabled(consentPage), 5*time.Second, 100*time.Millisecond).Should(BeTrue())
			Expect(consentPage.RequestedServiceStatus(ctx, "GitHub Production")).To(Equal("Connected"))
			Expect(consentPage.TakeRequestedServiceScreenshot(ctx, "consent_requested_service_ready")).To(Succeed())

			previousAuthorizeURL := GetMockUpstream().GetLastAuthorizeURL()
			Expect(consentPage.ClickSave(ctx)).To(Succeed())
			Eventually(GetMockUpstream().GetLastAuthorizeURL, 10*time.Second, 100*time.Millisecond).ShouldNot(Equal(previousAuthorizeURL))
			upstream, err := url.Parse(GetMockUpstream().GetLastAuthorizeURL())
			Expect(err).NotTo(HaveOccurred())
			Expect(upstream.Query().Get("scope")).To(Equal("openid"))
			Expect(upstream.Query().Get("state")).To(Equal("incremental-ui"))

			grant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(principal), cast.Agent.ID)
			Expect(err).NotTo(HaveOccurred())
			var granted []id.PermissionSetID
			for _, entry := range grant.GrantedPermissionSets {
				granted = append(granted, entry.PermissionSetID)
			}
			Expect(granted).To(ConsistOf(cast.IdentityAccess.ID, cast.GitHubRead.ID))
			Expect(exchange()).To(Equal(http.StatusOK), "the first retried tool call succeeds")

			By("gating Save on every selected candidate's effective scopes under require_all_scopes")
			cast.Agent.ServiceRequirements[0].RequiredScopes = nil
			cast.Agent.ServiceRequirements[0].RequireAllScopes = true
			Expect(GetTestStorage().Agents().Update(ctx, cast.Agent)).To(Succeed())
			Expect(GetTestStorage().UserSessions().Create(ctx, fixtures.SessionForServiceWithScopes(principal, cast.GitHub.ID.String(), []string{"repo"}))).To(Succeed())
			sessionToken = authorizeToConsent("openid " + githubScope())
			Expect(consentPage.NavigateToAgentWithSessionToken(ctx, cast.Agent.ID.String(), sessionToken)).To(Succeed())
			Expect(consentPage.WaitForRequestedService(ctx, "GitHub Production")).To(Succeed())
			Expect(consentPage.IsPermissionSetSelected(ctx, "GitHub Read Access")).To(BeTrue())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeFalse(), "the selected failing candidate disables Save")
			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Read Access")).To(Succeed())
			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Issue Triage")).To(Succeed())
			Eventually(saveEnabled(consentPage), 5*time.Second, 100*time.Millisecond).Should(BeTrue())
			Expect(consentPage.TogglePermissionSet(ctx, "GitHub Read Access")).To(Succeed())
			Eventually(saveEnabled(consentPage), 5*time.Second, 100*time.Millisecond).Should(BeFalse())
		})
	})

	Context("when Alice has a usable GitHub session", func() {
		BeforeEach(func() {
			Expect(GetTestStorage().UserSessions().Create(ctx, fixtures.SessionForServiceWithScopes(principal, cast.GitHub.ID.String(), []string{"repo", "user:email"}))).To(Succeed())
			GetMockUpstream().WithAccessToken("synthetic-github-refreshed")
		})

		// US2 S2 from specs/055-incremental-service-authorization/spec.md.
		It("shows the active session, refreshes it, and terminates it back to the no-session state", func() {
			sessionsPage := pages.NewSessionsPage(GetTestPage(), server.BaseURL())
			Expect(sessionsPage.NavigateToServiceSession(ctx, "github", "GitHub Production")).To(Succeed())
			status, err := sessionsPage.ServiceSessionStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).To(ContainSubstring("Active"))
			Expect(status).To(ContainSubstring("user:email"))
			Expect(sessionsPage.DependentAgentNames(ctx)).To(ConsistOf("Data Analysis Assistant"))
			Expect(sessionsPage.TakeScreenshot(ctx, "service_session_active")).To(Succeed())

			Expect(sessionsPage.ClickRefreshButton(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForSuccessMessage(ctx, "Session token refreshed successfully.")).To(Succeed())
			Expect(GetMockUpstream().GetTokenRequests()).NotTo(BeEmpty())

			Expect(sessionsPage.TerminateAndConfirm(ctx, "GitHub Production")).To(Succeed())
			Eventually(func() string {
				status, _ := sessionsPage.ServiceSessionStatus(ctx)
				return status
			}, 10*time.Second, 100*time.Millisecond).Should(ContainSubstring("No active session"))
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			Expect(sessionsPage.TakeScreenshot(ctx, "service_session_needs_login")).To(Succeed())
			session, err := GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), cast.GitHub.ID)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeNil())
			Expect(sessionsPage.NavigateToServiceSession(ctx, cast.GitHub.ID.String(), "GitHub Production")).To(Succeed())
			Expect(sessionsPage.ServiceSessionStatus(ctx)).To(ContainSubstring("No active session"))
		})
	})
})
