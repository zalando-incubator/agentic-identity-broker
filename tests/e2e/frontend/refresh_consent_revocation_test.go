package e2e_test

import (
	"context"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consent-Bound Refresh Sessions / US1 / browser", func() {
	var (
		ctx            context.Context
		env            *bootstrap.RefreshEnvironment
		client         *fixtures.RefreshClient
		browserContext playwright.BrowserContext
		consentPage    *pages.ConsentPage
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("", "", "")
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
		client, err = env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())

		// The ordinary frontend harness defaults to an upstream issuer. Use the same
		// browser with a new authenticated context against this local issuer instead.
		browserContext, err = suiteCtx.Browser.NewContext(playwright.BrowserNewContextOptions{
			Viewport:   &playwright.Size{Width: 1280, Height: 720},
			Locale:     playwright.String("en-US"),
			TimezoneId: playwright.String("UTC"),
			ExtraHttpHeaders: map[string]string{
				"X-Remote-User": client.Principal.String(),
			},
		})
		Expect(err).NotTo(HaveOccurred())
		browserPage, err := browserContext.NewPage()
		Expect(err).NotTo(HaveOccurred())
		browserPage.SetDefaultTimeout(30 * 1000)
		consentPage = pages.NewConsentPage(browserPage, env.Enduser.BaseURL())
	})

	AfterEach(func() {
		if browserContext != nil {
			Expect(browserContext.Close()).To(Succeed())
		}
		if env != nil {
			env.Close()
		}
	})

	// US1-S2 from specs/049-fix-refresh-consent/spec.md: the maintained consent UI actually revokes a local refresh session.
	It("US1-S2 revokes through the browser and denies the previously usable refresh token", func() {
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK), "consent must permit renewal before browser revocation")
		Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())

		Expect(consentPage.NavigateToAgent(ctx, client.Agent.ID.String())).To(Succeed())
		Eventually(func() (bool, error) {
			return consentPage.IsRevokeButtonPresent(ctx)
		}).Should(BeTrue(), "the authenticated user must see the existing revoke action")
		Expect(consentPage.TakeScreenshot(ctx, "refresh_consent_before_revoke")).To(Succeed())
		Expect(consentPage.ClickRevokeButton(ctx)).To(Succeed())
		Expect(consentPage.WaitForRevokeDialog(ctx)).To(Succeed())
		Expect(consentPage.ConfirmRevoke(ctx)).To(Succeed())
		Expect(consentPage.WaitForRevokeDialogDismissed(ctx)).To(Succeed())
		Expect(consentPage.NavigateToAgent(ctx, client.Agent.ID.String())).To(Succeed())
		Eventually(func() (bool, error) {
			present, err := consentPage.IsRevokeButtonPresent(ctx)
			return !present, err
		}).Should(BeTrue(), "the grant must no longer expose the revoke action")
		Expect(consentPage.TakeScreenshot(ctx, "refresh_consent_after_revoke")).To(Succeed())

		denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(denied.Status).To(Equal(http.StatusBadRequest))
		Expect(denied.Error).To(Equal("invalid_grant"))
		Expect(denied.ErrorURI).To(BeEmpty())
		for _, field := range []string{"access_token", "refresh_token", "token_type"} {
			_, present := denied.Body[field]
			Expect(present).To(BeFalse(), field+" must be absent after browser revocation")
		}
	})
})
