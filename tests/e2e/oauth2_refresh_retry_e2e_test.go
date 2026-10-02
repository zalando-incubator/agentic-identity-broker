package e2e_test

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type retryJourney struct {
	env         *bootstrap.RefreshEnvironment
	client      fixtures.RefreshClient
	first       *helpers.RefreshAuthorization
	issuedAfter time.Time
}

func startRetryJourney(reuse, absolute, inactivity, scope string) retryJourney {
	ctx := context.Background()
	cfg, err := fixtures.RefreshConfig(reuse, absolute, inactivity)
	Expect(err).NotTo(HaveOccurred())
	env, err := bootstrap.NewRefreshEnvironment(cfg, nil)
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(env.Close)
	Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
	client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
	Expect(err).NotTo(HaveOccurred())
	first, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, scope)
	issuedAfter := time.Now()
	Expect(err).NotTo(HaveOccurred())
	return retryJourney{env: env, client: *client, first: first, issuedAfter: issuedAfter}
}

func (j retryJourney) rotate(token, scope string) *helpers.RefreshTokenResult {
	result, err := helpers.RotateRefreshSession(context.Background(), j.env.Enduser.BaseURL(), j.client, token, scope)
	Expect(err).NotTo(HaveOccurred())
	return result
}

func expectRetryTokens(result *helpers.RefreshTokenResult) {
	ExpectWithOffset(1, result.Status).To(Equal(http.StatusOK))
	ExpectWithOffset(1, result.Tokens.AccessToken).NotTo(BeEmpty())
	ExpectWithOffset(1, result.Tokens.RefreshToken).NotTo(BeEmpty())
}

func expectRetryInvalidGrant(result *helpers.RefreshTokenResult) {
	ExpectWithOffset(1, result.Status).To(Equal(http.StatusBadRequest))
	ExpectWithOffset(1, result.Error).To(Equal("invalid_grant"))
	ExpectWithOffset(1, result.ErrorURI).To(BeEmpty())
	_, hasAccess := result.Body["access_token"]
	_, hasRefresh := result.Body["refresh_token"]
	ExpectWithOffset(1, hasAccess).To(BeFalse(), "invalid_grant must not contain an access token")
	ExpectWithOffset(1, hasRefresh).To(BeFalse(), "invalid_grant must not contain a refresh token")
}

func waitUntilRetryTime(deadline time.Time) {
	EventuallyWithOffset(1, func() bool { return !time.Now().Before(deadline) },
		20*time.Second, 5*time.Millisecond).Should(BeTrue())
}

var _ = Describe("Consent-Bound Refresh Sessions / US3", func() {
	// US3-S1 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S1 recovers the identical lost refresh result under the default retry policy", func() {
		j := startRetryJourney("", "", "", "read offline_access")
		lost := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(lost)
		Expect(lost.Tokens.RefreshToken != j.first.Tokens.RefreshToken).To(BeTrue())

		recovered := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(recovered)
		Expect(recovered.Tokens.AccessToken == lost.Tokens.AccessToken).To(BeTrue())
		Expect(recovered.Tokens.RefreshToken == lost.Tokens.RefreshToken).To(BeTrue())
		Expect(recovered.Tokens.Scope).To(Equal(lost.Tokens.Scope))
		Expect(recovered.Tokens.ExpiresIn).To(BeNumerically(">", 0))
		Expect(recovered.Tokens.ExpiresIn).To(BeNumerically("<=", lost.Tokens.ExpiresIn))

		usable := j.rotate(recovered.Tokens.RefreshToken, "read")
		expectRetryTokens(usable)
		Expect(usable.Tokens.RefreshToken != recovered.Tokens.RefreshToken).To(BeTrue())
	})

	// US3-S2 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S2 converges concurrent uses of one refresh token on one successor", func() {
		j := startRetryJourney("", "", "", "read offline_access")
		start := make(chan struct{})
		var wg sync.WaitGroup
		type outcome struct {
			result *helpers.RefreshTokenResult
			err    error
		}
		results := make(chan outcome, 2)
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				result, err := helpers.RotateRefreshSession(context.Background(), j.env.Enduser.BaseURL(), j.client, j.first.Tokens.RefreshToken, "read")
				results <- outcome{result, err}
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		var first *helpers.RefreshTokenResult
		for outcome := range results {
			Expect(outcome.err).NotTo(HaveOccurred())
			expectRetryTokens(outcome.result)
			if first == nil {
				first = outcome.result
				continue
			}
			Expect(outcome.result.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
			Expect(outcome.result.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		}
		Expect(first).NotTo(BeNil())
		expectRetryTokens(j.rotate(first.Tokens.RefreshToken, "read"))
	})

	// US3-S3 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S3 keeps the first-consumption retry deadline fixed and revokes at or after it", func() {
		j := startRetryJourney("4s", "", "", "read offline_access")
		before := time.Now()
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		after := time.Now()
		expectRetryTokens(first)
		waitUntilRetryTime(before.Add(2 * time.Second))
		retry := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(retry)
		Expect(retry.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		Expect(retry.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
		Expect(time.Now().Before(before.Add(4*time.Second))).To(BeTrue(), "positive control must occur before the earliest possible deadline")

		waitUntilRetryTime(after.Add(4 * time.Second))
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(first.Tokens.RefreshToken, "read"))
	})

	// US3-S4 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S4 revokes live descendants when an expired older predecessor is replayed", func() {
		j := startRetryJourney("5s", "", "8s", "read offline_access")
		waitUntilRetryTime(j.issuedAfter.Add(4 * time.Second))
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(first)
		current := j.rotate(first.Tokens.RefreshToken, "read")
		expectRetryTokens(current)
		Expect(current.Tokens.RefreshToken != first.Tokens.RefreshToken).To(BeTrue())

		waitUntilRetryTime(j.issuedAfter.Add(8 * time.Second))
		Expect(time.Now().Before(j.issuedAfter.Add(9*time.Second))).To(BeTrue(), "the expired predecessor's original reuse interval remains open")
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(current.Tokens.RefreshToken, "read"))
	})

	// US3-S5 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S5 enforces strict single use when the configured retry interval is zero", func() {
		j := startRetryJourney("0s", "", "", "read offline_access")
		successor := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(successor)
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(successor.Tokens.RefreshToken, "read"))
	})

	// US3-S7 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S7 rejects retries and the current token after session expiry despite open grace", func() {
		j := startRetryJourney("3s", "5s", "12s", "read offline_access")
		waitUntilRetryTime(j.issuedAfter.Add(3 * time.Second))
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(first)
		withinGrace := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(withinGrace)
		Expect(withinGrace.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		waitUntilRetryTime(j.issuedAfter.Add(5 * time.Second))
		Expect(time.Now().Before(j.issuedAfter.Add(6*time.Second))).To(BeTrue(), "grace must remain open past the session deadline")
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(first.Tokens.RefreshToken, "read"))
	})

	// US3-S9 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S9 permits equivalent normalized scope but rejects a different retry scope as reuse", func() {
		j := startRetryJourney("", "", "", "read offline_access")
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(first)
		matching := j.rotate(j.first.Tokens.RefreshToken, " read  read ")
		expectRetryTokens(matching)
		Expect(matching.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
		Expect(matching.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())

		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "offline_access"))
		expectRetryInvalidGrant(j.rotate(first.Tokens.RefreshToken, "read"))
	})

	// US3-S10 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S10 returns a stored result three times, then revokes on a fourth presentation", func() {
		j := startRetryJourney("", "", "", "read offline_access")
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(first)
		for range 3 {
			retry := j.rotate(j.first.Tokens.RefreshToken, "read")
			expectRetryTokens(retry)
			Expect(retry.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
			Expect(retry.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		}
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(first.Tokens.RefreshToken, "read"))
	})
})
