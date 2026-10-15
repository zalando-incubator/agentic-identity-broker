package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func refreshTestService(serviceID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	svc := fixtures.ServiceWithID(serviceID.String())
	svc.DisplayName = "Refreshable Service"
	svc.ClientID = id.ClientID("refreshable-client")
	svc.Secret = fixtures.EncryptedSecret(serviceID.String(), "refreshable-secret")
	svc.IssuerURI = GetMockUpstream().URL()
	svc.Discovery.EnableDiscovery = false
	svc.ProtectedResources = nil
	svc.Discovery.MetadataURL = nil
	svc.Endpoints = model.OAuth2Endpoints{
		AuthorizeEndpoint: GetMockUpstream().URL() + "/oauth/authorize",
		TokenEndpoint:     GetMockUpstream().URL() + "/oauth/token",
	}
	svc.Scopes = []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}}
	return svc
}

var _ = Describe("Third-Party Sessions Refresh Button", func() {
	var (
		ctx          context.Context
		sessionsPage *pages.SessionsPage
	)

	BeforeEach(func() {
		ctx = context.Background()
		principal := fixtures.DefaultPrincipal().String()

		GetMockUpstream().WithSuccessfulTokenResponse().
			WithAccessToken("refreshed-access-token").
			WithRefreshToken("rotated-refresh-token").
			WithExpiresIn(3600)

		for range 2 {
			serviceID := id.NewServiceID()
			err := GetTestStorage().Services().Create(ctx, refreshTestService(serviceID))
			Expect(err).NotTo(HaveOccurred(), "Failed to create refreshable third-party service")

			session := fixtures.SessionForService(principal, serviceID.String())
			err = GetTestStorage().UserSessions().Create(ctx, session)
			Expect(err).NotTo(HaveOccurred(), "Failed to create refreshable session")
		}

		sessionsPage = pages.NewSessionsPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario 1.2 from specs/008-thirdparty-oauth2-sessions/spec.md (extended by approved force-refresh plan)
	It("shows Refresh for multiple refreshable sessions and refreshes the first", func() {
		err := sessionsPage.NavigateToSessions(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to third-party sessions page")

		count, err := sessionsPage.GetRefreshButtonCount(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to count refresh buttons")
		Expect(count).To(Equal(2), "Both refreshable sessions should offer Refresh")

		visible, err := sessionsPage.IsRefreshButtonVisible(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check refresh button visibility")
		Expect(visible).To(BeTrue(), "Refresh button should be visible when a refresh token exists")

		err = sessionsPage.ClickRefreshButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click refresh button")

		err = sessionsPage.WaitForSuccessMessage(ctx, "Session token refreshed successfully.")
		Expect(err).NotTo(HaveOccurred(), "Expected refresh success message after clicking Refresh")

		Eventually(func() string {
			request := GetMockUpstream().GetLastRequest()
			if request == nil {
				return ""
			}
			_ = request.ParseForm()
			return request.FormValue("grant_type")
		}).WithPolling(100*time.Millisecond).Should(Equal("refresh_token"), "Refresh should call the upstream refresh_token grant")
	})

	Context("when refresh is rejected in the built SPA", func() {
		var sessionServer *bootstrap.TestServer

		BeforeEach(func() {
			config := fixtures.OAuth2ConfigWithUpstream(GetMockUpstream().URL())
			factory := bootstrap.NewServerFactory(config, GetLogger())
			builder, err := bootstrap.NewTestServerBuilder(config, GetTestStorage(), factory, GetLogger())
			Expect(err).NotTo(HaveOccurred(), "Failed to create callback-aligned session server builder")
			sessionServer, err = builder.Build()
			Expect(err).NotTo(HaveOccurred(), "Failed to build session recovery server")
			sessionsPage = pages.NewSessionsPage(GetTestPage(), sessionServer.BaseURL())
			GetMockUpstream().WithErrorResponse("invalid_grant")
		})

		AfterEach(func() {
			if sessionServer != nil {
				sessionServer.Close()
				sessionServer = nil
			}
		})

		// User Story 1 Scenario 6 from specs/008-thirdparty-oauth2-sessions/spec.md.
		// User Story 2 Scenario 3 from specs/029-proactive-token-refresh/spec.md.
		It("shows rejection immediately and recovers the retained session through Login", func() {
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Eventually(func() (int, error) {
				return sessionsPage.GetRefreshButtonCount(ctx)
			}).WithTimeout(10 * time.Second).WithPolling(100 * time.Millisecond).Should(Equal(2))
			Expect(sessionsPage.GetActiveSessionCount(ctx)).To(Equal(2))
			Expect(sessionsPage.GetSessionLifetimeClaimCount(ctx)).To(BeZero())

			page := GetTestPage()
			documentOrigin, err := page.Evaluate("() => performance.timeOrigin")
			Expect(err).NotTo(HaveOccurred())
			Expect(sessionsPage.ClickRefreshButton(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForErrorMessage(ctx, "refresh token rejected by provider; reauthentication required")).To(Succeed())
			Expect(sessionsPage.WaitForReauthenticationRequired(ctx)).To(Succeed())
			currentDocumentOrigin, err := page.Evaluate("() => performance.timeOrigin")
			Expect(err).NotTo(HaveOccurred())
			Expect(currentDocumentOrigin).To(Equal(documentOrigin), "Rejection should update the current document without a reload")
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(Equal(1))
			Expect(sessionsPage.GetTerminateButtonCount(ctx)).To(Equal(2))
			Expect(sessionsPage.GetRefreshButtonCount(ctx)).To(Equal(1))
			Expect(sessionsPage.GetActiveSessionCount(ctx)).To(Equal(1))
			Expect(sessionsPage.GetSessionLifetimeClaimCount(ctx)).To(BeZero(), "Neither retained card should claim a refresh-token lifetime")

			var rejectedList, recoveredList struct {
				Data struct {
					Sessions []map[string]any `json:"sessions"`
				} `json:"data"`
			}
			rejectedResponse, err := sessionServer.AuthenticatedGET("/api/third-party/sessions", fixtures.DefaultPrincipal().String())
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = rejectedResponse.Body.Close() }()
			Expect(rejectedResponse.StatusCode).To(Equal(http.StatusOK))
			Expect(json.NewDecoder(rejectedResponse.Body).Decode(&rejectedList)).To(Succeed())
			Expect(rejectedList.Data.Sessions).To(HaveLen(2), "Rejection should retain both sessions")
			var rejectedSession map[string]any
			for _, session := range rejectedList.Data.Sessions {
				if session["requires_reauthentication"] == true {
					rejectedSession = session
				}
			}
			Expect(rejectedSession).NotTo(BeNil())
			Expect(rejectedSession).To(HaveKeyWithValue("access_token_expired", false))
			Expect(rejectedSession).To(HaveKeyWithValue("has_refresh_token", true))
			rejectedServiceID, ok := rejectedSession["service_id"].(string)
			Expect(ok).To(BeTrue())
			requests := GetMockUpstream().GetTokenRequests()
			Expect(requests).To(HaveLen(1), "Rejected refresh should call the provider only once")
			refreshForm, err := url.ParseQuery(requests[0].Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(refreshForm.Get("grant_type")).To(Equal("refresh_token"))

			GetMockUpstream().WithSuccessfulTokenResponse()
			Expect(sessionsPage.ClickLoginButton(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForSuccessMessage(ctx, "Successfully connected to service.")).To(Succeed())
			returnURL, err := url.Parse(page.URL())
			Expect(err).NotTo(HaveOccurred())
			Expect(returnURL.Scheme + "://" + returnURL.Host).To(Equal(sessionServer.BaseURL()))
			Expect(returnURL.Path).To(Equal("/sessions"))
			Eventually(func() (int, error) {
				return sessionsPage.GetActiveSessionCount(ctx)
			}).WithTimeout(10*time.Second).WithPolling(100*time.Millisecond).Should(Equal(2), "The recovered and healthy cards should both be Active")
			Expect(sessionsPage.GetRefreshButtonCount(ctx)).To(Equal(2))
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(BeZero())
			Expect(sessionsPage.GetTerminateButtonCount(ctx)).To(Equal(2))
			Expect(sessionsPage.GetSessionLifetimeClaimCount(ctx)).To(BeZero())

			Expect(GetMockUpstream().GetAuthorizeCalled()).To(BeTrue())
			requests = GetMockUpstream().GetTokenRequests()
			Expect(requests).To(HaveLen(2))
			authorizationForm, err := url.ParseQuery(requests[1].Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(authorizationForm.Get("grant_type")).To(Equal("authorization_code"))
			Expect(authorizationForm.Get("redirect_uri")).To(Equal(sessionServer.BaseURL() + "/api/third-party/" + rejectedServiceID + "/oauth2/callback"))
			recoveredResponse, err := sessionServer.AuthenticatedGET("/api/third-party/sessions", fixtures.DefaultPrincipal().String())
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = recoveredResponse.Body.Close() }()
			Expect(recoveredResponse.StatusCode).To(Equal(http.StatusOK))
			Expect(json.NewDecoder(recoveredResponse.Body).Decode(&recoveredList)).To(Succeed())
			Expect(recoveredList.Data.Sessions).To(HaveLen(2))
			var recoveredSession map[string]any
			for _, session := range recoveredList.Data.Sessions {
				Expect(session).To(HaveKeyWithValue("requires_reauthentication", false))
				if session["service_id"] == rejectedServiceID {
					recoveredSession = session
				}
			}
			Expect(recoveredSession).NotTo(BeNil())
			Expect(recoveredSession).To(HaveKeyWithValue("id", rejectedSession["id"]), "The callback should replace the retained session in place")

			AddReportEntry("session recovery browser outcomes", "Rejection updated without reload: Login=1, Terminate=2, Refresh=1. Real authorization_code callback returned to /sessions: Active=2, Refresh=2, requires_reauthentication=false. Neither card made lifetime claims.", ReportEntryVisibilityAlways)
		})

		// User Story 1 Scenario 6 and User Story 3 Scenario 1/2/5 from specs/008-thirdparty-oauth2-sessions/spec.md.
		It("retains a rejected session on cancellation and deletes only that session on confirmation", func() {
			readSessions := func() []storage.UserSessionSummary {
				response, err := sessionServer.AuthenticatedGET("/api/third-party/sessions", fixtures.DefaultPrincipal().String())
				Expect(err).NotTo(HaveOccurred())
				defer func() { _ = response.Body.Close() }()
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				var body struct {
					Data struct {
						Sessions []storage.UserSessionSummary `json:"sessions"`
					} `json:"data"`
				}
				Expect(json.NewDecoder(response.Body).Decode(&body)).To(Succeed())
				return body.Data.Sessions
			}
			Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
			Expect(sessionsPage.ClickRefreshButton(ctx)).To(Succeed())
			Expect(sessionsPage.WaitForReauthenticationRequired(ctx)).To(Succeed())
			before := readSessions()
			Expect(before).To(HaveLen(2))
			var rejected, healthy storage.UserSessionSummary
			for _, session := range before {
				if session.RequiresReauthentication {
					rejected = session
				} else {
					healthy = session
				}
			}
			Expect(rejected.ID.IsZero()).To(BeFalse())
			Expect(healthy.ID.IsZero()).To(BeFalse())
			Expect(sessionsPage.ClickRejectedTerminateButton(ctx)).To(Succeed())
			Expect(sessionsPage.CancelSessionTermination(ctx)).To(Succeed())
			Expect(readSessions()).To(ConsistOf(before))
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(Equal(1))
			Expect(sessionsPage.ClickRejectedTerminateButton(ctx)).To(Succeed())
			Expect(sessionsPage.ConfirmSessionTermination(ctx, rejected.ServiceDisplayName)).To(Succeed())
			Expect(sessionsPage.WaitForSuccessMessage(ctx, "Session terminated successfully.")).To(Succeed())
			Expect(readSessions()).To(ConsistOf(healthy))
			Expect(sessionsPage.GetLoginButtonCount(ctx)).To(BeZero())
			Expect(sessionsPage.GetTerminateButtonCount(ctx)).To(Equal(1))
			AddReportEntry("rejected session termination outcomes", "Cancel retained both rows; confirmation deleted only the rejected row through the real dialog and API.", ReportEntryVisibilityAlways)
		})
	})
})
