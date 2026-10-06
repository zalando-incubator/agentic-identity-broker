package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptrace"
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
	It("US3-S3 keeps the first-consumption retry deadline fixed and revokes at or after it", Serial, func() {
		j := startRetryJourney("12s", "", "", "read offline_access")
		before := time.Now()
		first := j.rotate(j.first.Tokens.RefreshToken, "read")
		after := time.Now()
		expectRetryTokens(first)
		waitUntilRetryTime(before.Add(2 * time.Second))
		retry := j.rotate(j.first.Tokens.RefreshToken, "read")
		expectRetryTokens(retry)
		Expect(retry.Tokens.RefreshToken == first.Tokens.RefreshToken).To(BeTrue())
		Expect(retry.Tokens.AccessToken == first.Tokens.AccessToken).To(BeTrue())
		Expect(time.Now().Before(before.Add(12*time.Second))).To(BeTrue(), "positive control must occur before the earliest possible deadline")

		waitUntilRetryTime(after.Add(12 * time.Second))
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

	// Zero-reuse concurrent presentation edge case from specs/049-fix-refresh-consent/spec.md (FR-015).
	It("revokes a zero-reuse session after overlapping bound-client presentations", func() {
		j := startRetryJourney("0s", "", "", "read offline_access")
		ctx := context.Background()
		unrelated, err := j.env.AddClient(ctx, j.client.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		other, err := helpers.AuthorizeRefreshSession(ctx, j.env.Enduser.BaseURL(), *unrelated, "read offline_access")
		Expect(err).NotTo(HaveOccurred())

		held := make(chan struct{})
		release := make(chan struct{})
		releaseGate := sync.OnceFunc(func() { close(release) })
		DeferCleanup(releaseGate)
		holderDone := make(chan error, 1)
		go func() {
			holderDone <- j.env.Storage.AuthorizationCoordinator().Run(ctx, j.client.Agent.ID, func(context.Context, time.Time) error {
				close(held)
				<-release
				return nil
			})
		}()
		Eventually(held).WithTimeout(10 * time.Second).Should(BeClosed())

		// Both complete their HTTP writes while the real per-agent transaction guard is held.
		// Neither presentation can consume the token before both are in flight.
		written := make(chan error, 2)
		traceCtx := httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			WroteRequest: func(info httptrace.WroteRequestInfo) { written <- info.Err },
		})
		type outcome struct {
			result *helpers.RefreshTokenResult
			err    error
		}
		results := make(chan outcome, 2)
		for range 2 {
			go func() {
				result, err := helpers.RotateRefreshSession(traceCtx, j.env.Enduser.BaseURL(), j.client, j.first.Tokens.RefreshToken, "read")
				results <- outcome{result, err}
			}()
			Eventually(written).WithTimeout(12 * time.Second).Should(Receive(Succeed()))
			Expect(results).NotTo(Receive(), "a refresh returned while its agent guard was held")
		}
		releaseGate()
		Expect(<-holderDone).To(Succeed())

		var winner *helpers.RefreshTokenResult
		denials := 0
		for range 2 {
			got := <-results
			Expect(got.err).NotTo(HaveOccurred())
			switch got.result.Status {
			case http.StatusOK:
				Expect(winner == nil).To(BeTrue(), "at most one rotation may succeed")
				winner = got.result
				expectRetryTokens(winner)
				Expect(winner.Tokens.RefreshToken != j.first.Tokens.RefreshToken).To(BeTrue())
			case http.StatusBadRequest:
				expectRetryInvalidGrant(got.result)
				Expect(got.result.Body).NotTo(HaveKey("token_type"))
				Expect(got.result.Body).NotTo(HaveKey("expires_in"))
				denials++
			default:
				Fail("concurrent refresh must rotate once or reject prohibited reuse")
			}
		}
		Expect(winner).NotTo(BeNil(), "the first presentation must rotate")
		Expect(denials).To(Equal(1), "the slower presentation must be prohibited reuse")

		// Check the successor first: replaying the predecessor here would mask a missing
		// revocation by revoking the family only during this post-race assertion.
		expectRetryInvalidGrant(j.rotate(winner.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
		unrelatedRotation, err := helpers.RotateRefreshSession(ctx, j.env.Enduser.BaseURL(), *unrelated, other.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(unrelatedRotation)
		Expect(unrelatedRotation.Tokens.RefreshToken != other.Tokens.RefreshToken).To(BeTrue())
		expectRetryInvalidGrant(j.rotate(winner.Tokens.RefreshToken, "read"))
		expectRetryInvalidGrant(j.rotate(j.first.Tokens.RefreshToken, "read"))
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
