package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

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
		calendarID    id.ServiceID
		agent         *storage.Agent
		consentPage   *pages.ConsentPage
		sessionsPage  *pages.SessionsPage
		beforeSession *storage.UserSession
		beforeGrant   *storage.UserGrant
		requiredSetID id.PermissionSetID
		optionalSetID id.PermissionSetID
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

		calendarID, _, err = journey.Register(fixtures.ClientIdentityCalendarRequest(journey.Upstream.URL()))
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
		requiredSetID, optionalSetID = required.ID, optional.ID
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

	expectConnectedServices := func(serviceIDs ...string) {
		GinkgoHelper()
		response, err := journey.EndUser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String(), journey.Principal)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(body["data"]).To(HaveKeyWithValue("active_session_service_ids", ConsistOf(serviceIDs)))
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

	expectReportsSignIn := func() {
		GinkgoHelper()
		Expect(sessionsPage.WaitForServiceStatus(ctx, "Reports Production", "Sign-in required")).To(Succeed())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Login")).To(BeTrue())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Refresh")).To(BeFalse())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Terminate")).To(BeTrue())
		Expect(sessionsPage.WaitForServiceStatus(ctx, "Calendar Production", "Active")).To(Succeed())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Login")).To(BeFalse())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Refresh")).To(BeTrue())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Terminate")).To(BeTrue())
	}

	expectReconnectFailure := func(message string) {
		GinkgoHelper()
		Expect(sessionsPage.WaitForErrorMessage(ctx, message)).To(Succeed())
		Expect(sessionsPage.WaitForURL(ctx, `/sessions$`)).To(Succeed())
		expectReportsSignIn()
		requests := len(journey.Upstream.GetTokenRequests())
		authorization := journey.Upstream.GetLastAuthorizeURL()
		Consistently(func(g Gomega) {
			g.Expect(GetTestPage().URL()).To(Equal(journey.EndUser.BaseURL() + "/sessions"))
			g.Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(requests))
			g.Expect(journey.Upstream.GetLastAuthorizeURL()).To(Equal(authorization))
		}, 2*time.Second, 50*time.Millisecond).Should(Succeed())
		expectSessionUnchanged()
		expectGrantUnchanged()
		expectConnectedServices(calendarID.String())
		denied := exchange(http.StatusBadRequest)
		Expect(denied).To(HaveKeyWithValue("error", "invalid_grant"))
		Expect(denied).NotTo(HaveKey("access_token"))
		Expect(denied).NotTo(HaveKey("refresh_token"))
	}

	retryReports := func() {
		GinkgoHelper()
		Expect(sessionsPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Space")).To(Succeed())
		Expect(sessionsPage.WaitForURL(ctx, `/sessions\?.*success=true`)).To(Succeed())
		Expect(sessionsPage.WaitForServiceStatus(ctx, "Reports Production", "Active")).To(Succeed())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Login")).To(BeFalse())
		Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Refresh")).To(BeTrue())
		expectCurrentProviderIdentity()
		expectConnectedServices(serviceID.String(), calendarID.String())
		expectGrantUnchanged()
		Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
	}

	Context("after Reports client identity changes", func() {
		BeforeEach(publishChange)

		// AS-2.1 from specs/056-invalidate-client-sessions/spec.md
		It("automatically enters consent on the next agent authorization and offers Reports sign-in", func() {
			enterAuthorization()
			Expect(consentPage.IsServiceLoginVisible(ctx, "Reports Production")).To(BeTrue())
			Expect(consentPage.IsPermissionSetServiceIncluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Calendar Production")).To(BeFalse())
			normalContrast, hoverContrast, err := consentPage.GetServiceLoginContrast(ctx, "Reports Production")
			Expect(err).NotTo(HaveOccurred())
			Expect(normalContrast).To(BeNumerically(">=", 4.5), "Reports Login normal text contrast meets WCAG AA")
			Expect(hoverContrast).To(BeNumerically(">=", 4.5), "Reports Login hover text contrast meets WCAG AA")
			expectConnectedServices(calendarID.String())
			Expect(consentPage.TakeScreenshot(ctx, "client_identity_consent_sign_in_required")).To(Succeed())
			expectGrantUnchanged()
			expectSessionUnchanged()
		})

		// AS-2.2 from specs/056-invalidate-client-sessions/spec.md
		It("reconnects from consent with tab-local selections intact and completes the original authorization", func() {
			enterAuthorization()
			Expect(consentPage.TogglePermissionSet(ctx, "Additional Reports Access")).To(Succeed())
			Expect(consentPage.IsPermissionSetSelected(ctx, "Additional Reports Access")).To(BeTrue())
			Expect(consentPage.ExcludePermissionSetService(ctx, "Reports Access", "Calendar Production")).To(Succeed())
			Expect(consentPage.IsPermissionSetServiceExcluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
			Expect(consentPage.WaitForURL(ctx, "/agents/"+agent.ID.String()+`\?.*success=true`)).To(Succeed())
			Expect(consentPage.WaitForPageLoad(ctx)).To(Succeed())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Reports Production")).To(BeFalse())
			Expect(consentPage.IsPermissionSetSelected(ctx, "Additional Reports Access")).To(BeTrue())
			Expect(consentPage.IsPermissionSetServiceExcluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeTrue())
			expectConnectedServices(serviceID.String(), calendarID.String())
			Expect(consentPage.TakeScreenshot(ctx, "client_identity_consent_reconnected")).To(Succeed())
			expectCurrentProviderIdentity()
			Expect(consentPage.SaveConsent(ctx)).To(Succeed())
			Expect(consentPage.WaitForURL(ctx, regexp.QuoteMeta(agent.RedirectURIs[0])+`\?`)).To(Succeed())
			completed, err := url.Parse(GetTestPage().URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(completed.Query().Get("code")).NotTo(BeEmpty())
			Expect(completed.Query().Get("state")).To(Equal("reports-browser-state"))
			response, err := journey.EndUser.AuthenticatedGET("/api/consent/agents/"+agent.ID.String()+"/grants", journey.Principal)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			body, err := helpers.CredentialResponseObject(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(body["data"]).To(HaveKeyWithValue("granted_permission_sets", Equal(map[string]any{
				requiredSetID.String(): []any{serviceID.String()},
				optionalSetID.String(): []any{serviceID.String()},
			})))
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.3 from specs/056-invalidate-client-sessions/spec.md
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
			Expect(sessionsPage.WaitForServiceStatus(ctx, "Reports Production", "Active")).To(Succeed())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Login")).To(BeFalse())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Refresh")).To(BeTrue())
			Expect(sessionsPage.WaitForServiceStatus(ctx, "Calendar Production", "Active")).To(Succeed())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Login")).To(BeFalse())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Refresh")).To(BeTrue())
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_sessions_reconnected")).To(Succeed())
			expectCurrentProviderIdentity()
			expectGrantUnchanged()
			Expect(exchange(http.StatusOK)).To(HaveKey("access_token"))
		})

		// AS-2.4 from specs/056-invalidate-client-sessions/spec.md
		It("keeps failed reconnection unusable without grant expansion and permits a keyboard retry", func() {
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			expectReportsSignIn()
			journey.Upstream.WithErrorResponse("access_denied")
			Expect(sessionsPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
			expectReconnectFailure("Authorization failed - please try again")
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_reconnect_failed")).To(Succeed())
			journey.Upstream.WithSuccessfulTokenResponse()
			retryReports()
		})

		// AS-2.4 from specs/056-invalidate-client-sessions/spec.md
		It("retains the invalidated session after a provider decline and offers a service-specific retry", func() {
			journey.Upstream.WithAuthorizationError("access_denied")
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			expectReportsSignIn()
			calls := len(journey.Upstream.GetTokenRequests())
			Expect(sessionsPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
			expectReconnectFailure("You denied access to the service. No tokens were stored.")
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_reconnect_declined")).To(Succeed())
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(calls))
			lastAuthorization, err := url.Parse(journey.Upstream.GetLastAuthorizeURL())
			Expect(err).NotTo(HaveOccurred())
			Expect(lastAuthorization.Query().Get("client_id")).To(Equal(fixtures.ReportsClientV2))
			journey.Upstream.WithAuthorizationError("")
			retryReports()
		})

		Context("when reconnecting from agent consent", func() {
			BeforeEach(func() {
				enterAuthorization()
				Expect(consentPage.TogglePermissionSet(ctx, "Additional Reports Access")).To(Succeed())
				Expect(consentPage.IsPermissionSetSelected(ctx, "Additional Reports Access")).To(BeTrue())
			})

			// AS-2.4 from specs/056-invalidate-client-sessions/spec.md (User Story 2), consent provider-failure variant
			It("returns a failed consent reconnect to sessions without releasing old tokens and succeeds on keyboard retry", func() {
				journey.Upstream.WithErrorResponse("access_denied")
				calls := len(journey.Upstream.GetTokenRequests())
				Expect(consentPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
				expectReconnectFailure("Authorization failed - please try again")
				requests := journey.Upstream.GetTokenRequests()[calls:]
				Expect(requests).NotTo(BeEmpty())
				for _, request := range requests {
					form, err := url.ParseQuery(request.Body)
					Expect(err).NotTo(HaveOccurred())
					Expect(form.Get("grant_type")).To(Equal("authorization_code"))
					clientID, secret, err := helpers.CredentialTokenRequest(request)
					Expect(err).NotTo(HaveOccurred())
					Expect(clientID).To(Equal(fixtures.ReportsClientV2))
					Expect(secret).To(Equal(fixtures.ReportsSecretV2))
				}
				journey.Upstream.WithSuccessfulTokenResponse()
				retryReports()
			})

			// AS-2.4 from specs/056-invalidate-client-sessions/spec.md (User Story 2), consent provider-decline variant
			It("returns a declined consent reconnect to sessions without expanding grants and succeeds on keyboard retry", func() {
				journey.Upstream.WithAuthorizationError("access_denied")
				calls := len(journey.Upstream.GetTokenRequests())
				Expect(consentPage.LoginServiceWithKeyboard(ctx, "Reports Production", "Enter")).To(Succeed())
				expectReconnectFailure("You denied access to the service. No tokens were stored.")
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(calls))
				lastAuthorization, err := url.Parse(journey.Upstream.GetLastAuthorizeURL())
				Expect(err).NotTo(HaveOccurred())
				Expect(lastAuthorization.Query().Get("client_id")).To(Equal(fixtures.ReportsClientV2))
				journey.Upstream.WithAuthorizationError("")
				retryReports()
			})
		})

		// AS-2.5 from specs/056-invalidate-client-sessions/spec.md (User Story 2)
		It("shows current Reports sign-in on SPA consent and sessions loads without unsolicited navigation", func() {
			Expect(helpers.PublishCredentialPair(binding, fixtures.ReportsClientV1, fixtures.ReportsSecretV1)).To(Succeed())
			calendarBefore, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(journey.Principal), calendarID)
			Expect(err).NotTo(HaveOccurred())

			By("populating ordinary consent detail and sessions with connected state")
			Expect(consentPage.NavigateToOverview(ctx)).To(Succeed())
			_, err = GetTestPage().Evaluate(`() => { window.clientIdentitySPAMarker = crypto.randomUUID(); }`)
			Expect(err).NotTo(HaveOccurred())
			marker, err := GetTestPage().Evaluate(`() => window.clientIdentitySPAMarker`)
			Expect(err).NotTo(HaveOccurred())
			Expect(marker).NotTo(BeEmpty())
			Expect(consentPage.OpenAgentFromOverview(ctx, agent.DisplayName)).To(Succeed())
			initialDetail, err := url.Parse(GetTestPage().URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(initialDetail.RawQuery).To(BeEmpty(), "populate the ordinary consent detail cache before changing identity")
			Expect(consentPage.IsPermissionSetServiceIncluded(ctx, "Reports Access", "Calendar Production")).To(BeTrue())
			Expect(consentPage.IsSaveButtonEnabled(ctx)).To(BeTrue())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Reports Production")).To(BeFalse())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Calendar Production")).To(BeFalse())
			Expect(sessionsPage.ClickHeaderNavigation(ctx, "Third-Party Sessions")).To(Succeed())
			Expect(sessionsPage.WaitForServiceStatus(ctx, "Reports Production", "Active")).To(Succeed())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Login")).To(BeFalse())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Reports Production", "Refresh")).To(BeTrue())
			Expect(sessionsPage.WaitForServiceStatus(ctx, "Calendar Production", "Active")).To(Succeed())
			Expect(sessionsPage.IsServiceActionVisible(ctx, "Calendar Production", "Refresh")).To(BeTrue())
			Expect(consentPage.ClickHeaderNavigation(ctx, "Agent Delegations")).To(Succeed())
			beforeURL := GetTestPage().URL()
			requests := len(journey.Upstream.GetTokenRequests())
			authorization := journey.Upstream.GetLastAuthorizeURL()
			publishChange()
			Consistently(func(g Gomega) {
				g.Expect(GetTestPage().URL()).To(Equal(beforeURL))
				g.Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(requests))
				g.Expect(journey.Upstream.GetLastAuthorizeURL()).To(Equal(authorization))
			}, 2*time.Second, 50*time.Millisecond).Should(Succeed())

			By("returning through the overview card to fresh ordinary consent detail")
			Expect(consentPage.OpenAgentFromOverview(ctx, agent.DisplayName)).To(Succeed())
			current, err := url.Parse(GetTestPage().URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(current.Path).To(Equal("/agents/" + agent.ID.String()))
			Expect(current.RawQuery).To(BeEmpty(), "ordinary consent detail must not use session-token cache bypass")
			Eventually(func() (bool, error) {
				return consentPage.IsServiceLoginVisible(ctx, "Reports Production")
			}).Should(BeTrue())
			Expect(consentPage.IsServiceLoginVisible(ctx, "Calendar Production")).To(BeFalse())

			By("returning through the header to fresh sessions without resetting JavaScript context")
			Expect(sessionsPage.ClickHeaderNavigation(ctx, "Third-Party Sessions")).To(Succeed())
			expectReportsSignIn()
			Expect(GetTestPage().URL()).To(Equal(journey.EndUser.BaseURL() + "/sessions"))
			retainedMarker, err := GetTestPage().Evaluate(`() => window.clientIdentitySPAMarker`)
			Expect(err).NotTo(HaveOccurred())
			Expect(retainedMarker).To(Equal(marker), "SPA return must not reload the document and clear API caches")
			Consistently(GetTestPage().URL, 2*time.Second, 50*time.Millisecond).Should(Equal(journey.EndUser.BaseURL() + "/sessions"))
			Expect(sessionsPage.TakeScreenshot(ctx, "client_identity_sessions_sign_in_required")).To(Succeed())
			expectSessionUnchanged()
			expectGrantUnchanged()
			calendarAfter, err := journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(journey.Principal), calendarID)
			Expect(err).NotTo(HaveOccurred())
			Expect(calendarAfter).To(Equal(calendarBefore))
		})
	})
})
