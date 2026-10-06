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
	svc.ProtectedResources = nil
	svc.Discovery.MetadataURL = nil
	svc.Endpoints = model.OAuth2Endpoints{
		AuthorizeEndpoint: GetMockUpstream().URL() + "/oauth/authorize",
		TokenEndpoint:     GetMockUpstream().URL() + "/oauth/token",
	}
	svc.Scopes = []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}}
	return svc
}

var _ = Describe("Connections Refresh Button", func() {
	var (
		ctx             context.Context
		connectionsPage *pages.ConnectionsPage
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
			// A valid refresh token with an expired access token warrants Refresh;
			// a healthy connection exposes only Disconnect in the redesigned card.
			expiredAccess := time.Now().Add(-time.Minute)
			session.AccessTokenExpiresAt = &expiredAccess
			err = GetTestStorage().UserSessions().Create(ctx, session)
			Expect(err).NotTo(HaveOccurred(), "Failed to create refreshable session")
		}

		connectionsPage = pages.NewConnectionsPage(GetTestPage(), GetFrontendURL())
	})

	// AS-11 from specs/047-redesign-consent-console/spec.md; Scenario 1.2 from specs/008-thirdparty-oauth2-sessions/spec.md.
	It("shows Refresh for multiple refreshable connections and refreshes the first", func() {
		err := connectionsPage.NavigateToConnections(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to Connections")

		count, err := connectionsPage.GetRefreshButtonCount(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to count refresh buttons")
		Expect(count).To(Equal(2), "Both refreshable connections should offer Refresh")

		visible, err := connectionsPage.IsRefreshButtonVisible(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to check refresh button visibility")
		Expect(visible).To(BeTrue(), "Refresh button should be visible when a refresh token exists")

		err = connectionsPage.ClickRefreshButton(ctx)
		Expect(err).NotTo(HaveOccurred(), "Failed to click refresh button")

		err = connectionsPage.WaitForSuccessMessage(ctx, "Connection refreshed.")
		Expect(err).NotTo(HaveOccurred(), "Expected connection refresh toast after clicking Refresh")

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
