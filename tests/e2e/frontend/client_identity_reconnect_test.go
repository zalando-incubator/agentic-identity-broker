package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client identity browser recovery", Label("client-identity-change"), func() {
	var (
		journey       *helpers.OAuth2CredentialJourney
		binding       map[string]string
		serviceID     id.ServiceID
		agent         *storage.Agent
		consentPage   *pages.ConsentPage
		sessionsPage  *pages.SessionsPage
		beforeSession *storage.UserSession
		beforeGrant   *storage.UserGrant
	)
	ctx := context.Background()

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		binding, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
		Expect(err).NotTo(HaveOccurred())
		document := fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]map[string]string{"reports-prod": binding}, 8000, 14000)
		document["oauth2_authorization_server"].(map[string]any)["mode"] = "hybrid"
		Expect(journey.StartBrowserDocument(document)).To(Succeed())
		serviceID, _, err = journey.Register(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem"))
		Expect(err).NotTo(HaveOccurred())

		calendarID, _, err := journey.Register(fixtures.ClientIdentityCalendarRequest(journey.Upstream.URL()))
		Expect(err).NotTo(HaveOccurred())
		calendarResponse, err := journey.Complete(calendarID)
		Expect(err).NotTo(HaveOccurred())
		Expect(calendarResponse.StatusCode).To(Equal(http.StatusFound))
		calendarLocation, err := url.Parse(calendarResponse.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(calendarLocation.Query().Get("success")).To(Equal("true"))
		Expect(calendarResponse.Body.Close()).To(Succeed())

		required := fixtures.ClientIdentityPermissionSet(serviceID, "Reports Access")
		required.ServiceScopes = append(required.ServiceScopes, storage.ServiceScope{
			ServiceID: calendarID, Scopes: []string{"profile"}, RequirementType: storage.RequirementTypeOptional,
		})
		optional := fixtures.ClientIdentityPermissionSet(serviceID, "Additional Reports Access")
		Expect(journey.Storage.PermissionSets().Create(ctx, required)).To(Succeed())
		Expect(journey.Storage.PermissionSets().Create(ctx, optional)).To(Succeed())
		agent = fixtures.ClientIdentityAgent(serviceID, required.ID, optional.ID)
		agent.RedirectURIs = []string{journey.EndUser.BaseURL() + "/browser-agent-callback"}
		agent.ServiceRequirements = append(agent.ServiceRequirements, storage.ServiceRequirement{
			ServiceID: calendarID, RequiredScopes: []string{"profile"}, RequirementType: storage.RequirementTypeOptional,
		})
		Expect(journey.Storage.Agents().Create(ctx, agent)).To(Succeed())
		delegation := fixtures.ActiveGrant(journey.Principal, agent.ID.String(), serviceID.String(), []string{"profile"})
		delegation.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: required.ID, IncludedServiceIDs: []id.ServiceID{serviceID, calendarID}}}
		Expect(journey.Storage.UserGrants().Create(ctx, delegation)).To(Succeed())

		response, err := journey.Complete(serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.Body.Close()).To(Succeed())
		Expect(location.Query().Get("success")).To(Equal("true"))
		beforeSession, err = journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(journey.Principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		beforeGrant, err = journey.Storage.UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(journey.Principal), agent.ID)
		Expect(err).NotTo(HaveOccurred())

		consentPage = pages.NewConsentPage(GetTestPage(), journey.EndUser.BaseURL())
		sessionsPage = pages.NewSessionsPage(GetTestPage(), journey.EndUser.BaseURL())
		Expect(GetTestPage().Route(journey.EndUser.BaseURL()+"/browser-agent-callback**", func(route playwright.Route) {
			Expect(route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(http.StatusOK), Body: "Agent authorization complete"})).To(Succeed())
		})).To(Succeed())
	})

	publishChange := func() {
		GinkgoHelper()
		Expect(helpers.PublishCredentialPair(binding, fixtures.ReportsClientV2, fixtures.ReportsSecretV2)).To(Succeed())
	}

	exchange := func(status int) map[string]any {
		GinkgoHelper()
		form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, journey.Principal)
		Expect(err).NotTo(HaveOccurred())
		response, err := journey.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(status), "%v", body)
		return body
	}

	enterAuthorization := func() {
		GinkgoHelper()
		query := url.Values{
			"client_id": {agent.ID.String()}, "response_type": {"code"},
			"redirect_uri": {agent.RedirectURIs[0]}, "scope": {"openid profile"}, "state": {"reports-browser-state"},
			"code_challenge": {helpers.GenerateCodeChallenge(helpers.PKCEVerifier())}, "code_challenge_method": {"S256"},
		}
		Expect(consentPage.Navigate(ctx, "/oauth2/authorize?"+query.Encode())).To(Succeed())
		current, err := url.Parse(GetTestPage().URL())
		Expect(err).NotTo(HaveOccurred())
		Expect(current.Path).To(Equal("/agents/" + agent.ID.String()))
		Expect(current.Query().Get("session_token")).NotTo(BeEmpty())
		Expect(consentPage.WaitForServiceToAppear(ctx, "Reports Production")).To(Succeed())
	}

	expectGrantUnchanged := func() {
		GinkgoHelper()
		current, err := journey.Storage.UserGrants().FindByPrincipalAndAgent(ctx, id.Principal(journey.Principal), agent.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current).To(Equal(beforeGrant))
	}

	expectSessionUnchanged := func() {
		GinkgoHelper()
		current, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(journey.Principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(current).To(Equal(beforeSession))
	}

	expectCurrentProviderIdentity := func() {
		GinkgoHelper()
		requests := journey.Upstream.GetTokenRequests()
		Expect(requests).NotTo(BeEmpty())
		clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
		Expect(err).NotTo(HaveOccurred())
		Expect(clientID).To(Equal(fixtures.ReportsClientV2))
		Expect(secret).To(Equal(fixtures.ReportsSecretV2))
	}

	Context("after Reports client identity changes", func() {
		BeforeEach(publishChange)

		// AS-2.1 from specs/055-invalidate-client-sessions/spec.md
		It("automatically enters consent on the next agent authorization and offers Reports sign-in", func() {
			enterAuthorization()
			Expect(consentPage.IsServiceLoginVisible(ctx, "Reports Production")).To(BeTrue())
			Expect(consentPage.WaitForServiceToAppear(ctx, "Calendar Production")).To(Succeed())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Calendar Production")).To(BeFalse())
			Expect(consentPage.TakeScreenshot(ctx, "client_identity_consent_sign_in_required")).To(Succeed())
			expectGrantUnchanged()
			expectSessionUnchanged()
		})

		// AS-2.2 from specs/055-invalidate-client-sessions/spec.md
		It("reconnects from consent with tab-local selections intact and completes the original authorization", func() {
			enterAuthorization()
			Expect(consentPage.TogglePermissionSet(ctx, "Additional Reports Access")).To(Succeed())
			Expect(consentPage.IsPermissionSetSelected(ctx, "Additional Reports Access")).To(BeTrue())
			Expect(consentPage.ExcludePermissionSetService(ctx, "Reports Access", "Calendar Production")).To(Succeed())
			Expect(consentPage.IsPermissionSetServiceExcluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
			Expect(consentPage.WaitForURL(ctx, "/agents/"+agent.ID.String()+`\?.*success=true`)).To(Succeed())
			Expect(consentPage.WaitForServiceToAppear(ctx, "Reports Production")).To(Succeed())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Reports Production")).To(BeFalse())
			Expect(consentPage.IsPermissionSetSelected(ctx, "Additional Reports Access")).To(BeTrue())
			Expect(consentPage.IsPermissionSetServiceExcluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.TakeScreenshot(ctx, "client_identity_consent_reconnected")).To(Succeed())
			expectCurrentProviderIdentity()
			Expect(consentPage.SubmitConsent(ctx)).To(Succeed())
			Expect(consentPage.WaitForURL(ctx, regexp.QuoteMeta(agent.RedirectURIs[0])+`\?`)).To(Succeed())
			completed, err := url.Parse(GetTestPage().URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(completed.Query().Get("code")).NotTo(BeEmpty())
			Expect(completed.Query().Get("state")).To(Equal("reports-browser-state"))
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.3 from specs/055-invalidate-client-sessions/spec.md
		It("follows the token recovery URI through the provider back to sessions and restores exchange", func() {
			body := exchange(http.StatusBadRequest)
			Expect(body).To(HaveKeyWithValue("error", "invalid_grant"))
			Expect(body).NotTo(HaveKey("access_token"))
			recovery, ok := body["error_uri"].(string)
			Expect(ok).To(BeTrue())
			target, err := url.Parse(recovery)
			Expect(err).NotTo(HaveOccurred())
			Expect(target.Scheme + "://" + target.Host).To(Equal(journey.EndUser.BaseURL()))
			Expect(target.Path).To(Equal("/api/third-party/" + serviceID.String() + "/oauth2/authorize"))
			Expect(target.Query().Get("redirect_uri")).To(Equal(journey.EndUser.BaseURL() + "/sessions"))
			Expect(sessionsPage.Navigate(ctx, target.RequestURI())).To(Succeed())
			Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*success=true`)).To(Succeed())
			Expect(sessionsPage.WaitForTerminateButton(ctx)).To(Succeed())
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(Equal(0))
			Expect(sessionsPage.GetRefreshButtonCount(ctx)).To(Equal(2))
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_sessions_reconnected")).To(Succeed())
			expectCurrentProviderIdentity()
			expectGrantUnchanged()
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.4 from specs/055-invalidate-client-sessions/spec.md
		It("keeps failed reconnection unusable without grant expansion and permits a keyboard retry", func() {
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			journey.Upstream.WithErrorResponse("access_denied")
			Expect(sessionsPage.LoginWithKeyboard(ctx, "Enter")).To(Succeed())
			Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*error=callback_failed`)).To(Succeed())
			Expect(sessionsPage.WaitForErrorMessage(ctx, "Authorization failed - please try again")).To(Succeed())
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_reconnect_failed")).To(Succeed())
			expectSessionUnchanged()
			expectGrantUnchanged()
			Expect(exchange(http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid_grant"))
			journey.Upstream.WithSuccessfulTokenResponse()
			Expect(sessionsPage.LoginWithKeyboard(ctx, "Space")).To(Succeed())
			Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*success=true`)).To(Succeed())
			Expect(sessionsPage.WaitForTerminateButton(ctx)).To(Succeed())
			expectCurrentProviderIdentity()
			expectGrantUnchanged()
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.4 from specs/055-invalidate-client-sessions/spec.md
		It("retains the invalidated session after a provider decline and offers a service-specific retry", func() {
			Expect(GetTestPage().Route(journey.Upstream.URL()+"/oauth/authorize**", func(route playwright.Route) {
				authorization, err := url.Parse(route.Request().URL())
				Expect(err).NotTo(HaveOccurred())
				Expect(authorization.Query().Get("client_id")).To(Equal(fixtures.ReportsClientV2))
				callback, err := url.Parse(authorization.Query().Get("redirect_uri"))
				Expect(err).NotTo(HaveOccurred())
				callback.RawQuery = url.Values{"state": {authorization.Query().Get("state")}, "error": {"access_denied"}}.Encode()
				Expect(route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(http.StatusFound), Headers: map[string]string{"Location": callback.String()}})).To(Succeed())
			})).To(Succeed())
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			calls := len(journey.Upstream.GetTokenRequests())
			Expect(sessionsPage.LoginWithKeyboard(ctx, "Enter")).To(Succeed())
			Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*error=access_denied`)).To(Succeed())
			Expect(sessionsPage.WaitForErrorMessage(ctx, "Authorization failed - please try again")).To(Succeed())
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_reconnect_declined")).To(Succeed())
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(calls))
			expectSessionUnchanged()
			expectGrantUnchanged()
			Expect(exchange(http.StatusBadRequest)).To(HaveKeyWithValue("error", "invalid_grant"))
			Expect(GetTestPage().Unroute(journey.Upstream.URL() + "/oauth/authorize**")).To(Succeed())
			Expect(sessionsPage.LoginWithKeyboard(ctx, "Space")).To(Succeed())
			Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*success=true`)).To(Succeed())
			expectCurrentProviderIdentity()
			expectGrantUnchanged()
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.5 from specs/055-invalidate-client-sessions/spec.md
		It("shows Reports sign-in after the next sessions load without unsolicited consent navigation", func() {
			Expect(helpers.PublishCredentialPair(binding, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)).To(Succeed())
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForTerminateButton(ctx)).To(Succeed())
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(Equal(0))
			beforeURL := GetTestPage().URL()
			publishChange()
			Expect(GetTestPage().URL()).To(Equal(beforeURL))
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed())
			Expect(sessionsPage.IsTextVisible(ctx, "Sign-in required")).To(BeTrue())
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(Equal(1))
			Expect(sessionsPage.GetRefreshButtonCount(ctx)).To(Equal(1), "Calendar remains refreshable")
			Expect(sessionsPage.GetTerminateButtonCount(ctx)).To(Equal(2))
			current, err := url.Parse(GetTestPage().URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(current.Path).To(Equal("/sessions"))
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_sessions_sign_in_required")).To(Succeed())
			expectSessionUnchanged()
			expectGrantUnchanged()
		})
	})
})
