package e2e_test

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
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
		serviceID := id.NewServiceID()
		principal := fixtures.DefaultPrincipal().String()

		GetMockUpstream().WithSuccessfulTokenResponse().
			WithAccessToken("refreshed-access-token").
			WithRefreshToken("rotated-refresh-token").
			WithExpiresIn(3600)

		err := GetTestStorage().Services().Create(ctx, refreshTestService(serviceID))
		Expect(err).NotTo(HaveOccurred(), "Failed to create refreshable third-party service")

		session := fixtures.SessionForService(principal, serviceID.String())
		err = GetTestStorage().UserSessions().Create(ctx, session)
		Expect(err).NotTo(HaveOccurred(), "Failed to create refreshable session")

		sessionsPage = pages.NewSessionsPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario 1.2 from specs/008-thirdparty-oauth2-sessions/spec.md (extended by approved force-refresh plan)
	It("shows Refresh for a refreshable session and refreshes successfully", func() {
		err := sessionsPage.NavigateToSessions(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to third-party sessions page")

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
