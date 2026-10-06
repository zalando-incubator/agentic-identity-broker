package e2e_test

import (
	"context"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consent-Bound Refresh Sessions / US4", func() {
	const absoluteLifetime = 12 * time.Second
	var (
		ctx         context.Context
		environment *bootstrap.RefreshEnvironment
		client      *fixtures.RefreshClient
		initial     *helpers.RefreshAuthorization
		issuedUpper time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("0s", "12s", "10s")
		Expect(err).NotTo(HaveOccurred())
		environment, err = bootstrap.NewRefreshEnvironment(config, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, environment.Storage)).To(Succeed())
		client, err = environment.AddClient(ctx, id.NewPrincipal("absolute-session@example.com"), true)
		Expect(err).NotTo(HaveOccurred())
		initial, err = helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		issuedUpper = time.Now()
	})

	AfterEach(func() {
		if environment != nil {
			environment.Close()
		}
	})

	// US4-S1 from specs/049-fix-refresh-consent/spec.md: a fresh rotation does not move the first issuance deadline.
	It("retains the original absolute deadline after a fresh rotation", func() {
		Eventually(func() bool {
			return !time.Now().Before(issuedUpper.Add(3 * time.Second))
		}, 4*time.Second, 20*time.Millisecond).Should(BeTrue())
		first, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, initial.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Status).To(Equal(http.StatusOK))
		Expect(first.Tokens.RefreshToken).NotTo(BeEmpty())
		Expect(helpers.RefreshSignature(first.Tokens.RefreshToken)).NotTo(Equal(helpers.RefreshSignature(initial.Tokens.RefreshToken)))

		// The predecessor was still valid at rotation, and this successor remains
		// inside its five-second inactivity lifetime at the original deadline.
		Eventually(func() bool {
			return !time.Now().Before(issuedUpper.Add(absoluteLifetime + 100*time.Millisecond))
		}, absoluteLifetime, 20*time.Millisecond).Should(BeTrue())
		denied, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(denied.Status).To(Equal(http.StatusBadRequest))
		Expect(denied.Error).To(Equal("invalid_grant"))
		_, hasAccess := denied.Body["access_token"]
		_, hasRefresh := denied.Body["refresh_token"]
		Expect(hasAccess).To(BeFalse())
		Expect(hasRefresh).To(BeFalse())
	})

	// US4-S2 from specs/049-fix-refresh-consent/spec.md: even continuous fresh activity cannot cross the original deadline.
	It("denies continuous refresh at the original deadline but allows fresh authorization", Serial, func() {
		current := initial.Tokens.RefreshToken
		for _, offset := range []time.Duration{2 * time.Second, 4 * time.Second} {
			Eventually(func() bool {
				return !time.Now().Before(issuedUpper.Add(offset))
			}, 3*time.Second, 20*time.Millisecond).Should(BeTrue())
			rotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, current, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Status).To(Equal(http.StatusOK))
			Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())
			Expect(helpers.RefreshSignature(rotated.Tokens.RefreshToken)).NotTo(Equal(helpers.RefreshSignature(current)))
			current = rotated.Tokens.RefreshToken
		}
		Eventually(func() bool {
			return !time.Now().Before(issuedUpper.Add(absoluteLifetime + 100*time.Millisecond))
		}, absoluteLifetime, 20*time.Millisecond).Should(BeTrue())
		denied, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, current, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(denied.Status).To(Equal(http.StatusBadRequest))
		Expect(denied.Error).To(Equal("invalid_grant"))
		_, hasAccess := denied.Body["access_token"]
		_, hasRefresh := denied.Body["refresh_token"]
		Expect(hasAccess).To(BeFalse())
		Expect(hasRefresh).To(BeFalse())

		fresh, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		reauthorized, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, fresh.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(reauthorized.Status).To(Equal(http.StatusOK))
		Expect(reauthorized.Tokens.RefreshToken).NotTo(BeEmpty())
	})
})
