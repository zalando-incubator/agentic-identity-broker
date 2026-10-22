package e2e_test

import (
	"context"
	"net/url"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
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
})

var _ = Describe("Third-Party Sessions Rejected Refresh Token", func() {
	var (
		ctx          context.Context
		sessionsPage *pages.SessionsPage
		serviceID    id.ServiceID
	)

	BeforeEach(func() {
		ctx = context.Background()
		principal := fixtures.DefaultPrincipal().String()
		GetMockUpstream().WithErrorResponse("invalid_grant")

		serviceID = id.NewServiceID()
		Expect(GetTestStorage().Services().Create(ctx, refreshTestService(serviceID))).To(Succeed())

		session := fixtures.SessionForService(principal, serviceID.String())
		expired := time.Now().Add(-time.Minute)
		session.AccessTokenExpiresAt = &expired
		session.RefreshTokenExpiresAt = nil
		Expect(GetTestStorage().UserSessions().Create(ctx, session)).To(Succeed())

		sessionsPage = pages.NewSessionsPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario 1.6 from specs/008-thirdparty-oauth2-sessions/spec.md
	It("shows the session as expired with Login after the provider rejects its refresh token", func() {
		Expect(sessionsPage.NavigateToSessions(ctx)).To(Succeed())
		Expect(sessionsPage.ClickRefreshButton(ctx)).To(Succeed())

		Expect(sessionsPage.WaitForLoginButton(ctx)).To(Succeed(), "an expired session must offer Login")
		Expect(sessionsPage.IsReauthenticationRequiredVisible(ctx)).To(BeTrue())
		Expect(sessionsPage.GetRefreshButtonCount(ctx)).To(Equal(0), "an expired session must not offer Refresh")
		Expect(sessionsPage.TakeScreenshot(ctx, "sessions_rejected_refresh_login")).To(Succeed())

		page := sessionsPage.GetPlaywrightPage()
		authorizeURL := make(chan string, 1)
		Expect(page.Route("**/api/third-party/*/oauth2/authorize*", func(route playwright.Route) {
			authorizeURL <- route.Request().URL()
			Expect(route.Abort()).To(Succeed())
		})).To(Succeed())
		Expect(sessionsPage.ClickLoginButton(ctx)).To(Succeed())

		var requested string
		Eventually(authorizeURL).Should(Receive(&requested))
		target, err := url.Parse(requested)
		Expect(err).NotTo(HaveOccurred())
		Expect(target.Path).To(Equal("/api/third-party/" + serviceID.String() + "/oauth2/authorize"))
		Expect(target.Query().Get("redirect_uri")).To(HaveSuffix("/sessions"))
	})
})
