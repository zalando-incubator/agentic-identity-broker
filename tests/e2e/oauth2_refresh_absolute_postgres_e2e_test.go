//go:build integration

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consent-Bound Refresh Sessions / US4 / PostgreSQL", func() {
	var (
		ctx         context.Context
		config      *ports.Config
		environment *bootstrap.RefreshEnvironment
		replica     *bootstrap.RefreshEnvironment
		stopReplica func()
		client      *fixtures.RefreshClient
		database    *sqlx.DB
		cleanup     func()
	)

	BeforeEach(func() {
		ctx = context.Background()
		stopReplica = nil
	})
	JustBeforeEach(func() {
		var store *storageadapter.Adapter
		var err error
		store, database, cleanup, err = bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, config)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, store)).To(Succeed())
		environment, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		client, err = environment.AddClient(ctx, id.NewPrincipal("absolute-postgres@example.com"), true)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		if stopReplica != nil {
			stopReplica()
		}
		if environment != nil {
			environment.Stop()
		}
		if cleanup != nil {
			cleanup()
		}
	})

	Context("with the omitted absolute lifetime", func() {
		BeforeEach(func() {
			var err error
			config, err = fixtures.RefreshConfig("", "", "")
			Expect(err).NotTo(HaveOccurred())
		})

		// US4-S3 from specs/049-fix-refresh-consent/spec.md: activity after day 30 cannot create an absolute deadline.
		It("keeps an evidenced 35-day original start and no absolute deadline after fresh activity", func() {
			initial, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
			Expect(err).NotTo(HaveOccurred())
			first, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, initial.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(first.Status).To(Equal(http.StatusOK))
			Expect(first.Tokens.RefreshToken).NotTo(BeEmpty())
			second, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(second.Status).To(Equal(http.StatusOK))
			Expect(second.Tokens.RefreshToken).NotTo(BeEmpty())
			environment.Stop()
			oldCleanup := cleanup
			var history agedAbsoluteHistory
			store, historical, closeHistorical, err := bootstrap.NewHistoricalRefreshFixture(refreshSuiteTestingT, config, database, func(fixture *sqlx.DB) error {
				var err error
				history, err = ageVerifiedAbsoluteHistory(ctx, fixture, [3]string{
					initial.Tokens.RefreshToken, first.Tokens.RefreshToken, second.Tokens.RefreshToken,
				})
				return err
			})
			Expect(err).NotTo(HaveOccurred())
			cleanup = func() { closeHistorical(); oldCleanup() }
			database = historical
			environment, err = bootstrap.NewRefreshEnvironment(config, store)
			Expect(err).NotTo(HaveOccurred())
			sharedBefore, err := helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			Expect(sharedBefore.Sub(history.FirstIssuedAt)).To(BeNumerically(">", 35*24*time.Hour))
			Expect(sharedBefore.Before(history.CurrentExpiresAt)).To(BeTrue())
			Expect(sharedBefore.Before(history.LastFreshAt.Add(30 * 24 * time.Hour))).To(BeTrue())

			rotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, second.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Status).To(Equal(http.StatusOK))
			Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())
			Expect(helpers.RefreshSignature(rotated.Tokens.RefreshToken)).NotTo(Equal(helpers.RefreshSignature(second.Tokens.RefreshToken)))
			sharedAfter, err := helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			var latest struct {
				CreatedAt time.Time `db:"created_at"`
				ExpiresAt time.Time `db:"expires_at"`
				Unused    bool      `db:"unused"`
			}
			Expect(database.GetContext(ctx, &latest, `SELECT created_at, expires_at,
				used_at IS NULL AS unused FROM refresh_token_sessions WHERE signature = $1`,
				helpers.RefreshSignature(rotated.Tokens.RefreshToken))).To(Succeed())
			Expect(latest.Unused).To(BeTrue())
			Expect(latest.CreatedAt.After(history.LastFreshAt)).To(BeTrue())
			Expect(latest.ExpiresAt.After(history.CurrentExpiresAt)).To(BeTrue())
			Expect(latest.ExpiresAt.Sub(latest.CreatedAt)).To(BeNumerically(">", 29*24*time.Hour))

			if history.HasRoot {
				root, err := loadAbsoluteRoot(ctx, database, rotated.Tokens.RefreshToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(root.OriginalTokenSignature).To(Equal(history.OriginalTokenSignature))
				Expect(root.StartedAt.Equal(history.FirstIssuedAt)).To(BeTrue())
				Expect(root.AbsoluteExpiresAt).To(BeNil())
				Expect(root.LastFreshAt.After(history.LastFreshAt)).To(BeTrue())
				Expect(root.LastFreshAt.Before(sharedBefore)).To(BeFalse())
				Expect(root.LastFreshAt.After(sharedAfter)).To(BeFalse())
				Expect(root.InactivityExpiresAt.Equal(root.LastFreshAt.Add(30 * 24 * time.Hour))).To(BeTrue())
			}
		})
	})

	Context("with a finite absolute lifetime on durable storage", func() {
		const lifetime = 12 * time.Second
		BeforeEach(func() {
			var err error
			config, err = fixtures.RefreshConfig("0s", "12s", "20s")
			Expect(err).NotTo(HaveOccurred())
		})

		// US4-S4 from specs/049-fix-refresh-consent/spec.md: restart and another broker retain the first-issued deadline using shared database time.
		It("preserves one absolute deadline through restart and a second broker", func() {
			sharedBefore, err := helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			initial, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
			Expect(err).NotTo(HaveOccurred())
			sharedAfter, err := helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			var hasRoot bool
			Expect(database.GetContext(ctx, &hasRoot, "SELECT to_regclass('public.refresh_sessions') IS NOT NULL")).To(Succeed())
			deadline := sharedAfter.Add(lifetime) // Safe issuance upper bound on a pre-feature schema.
			var original absoluteRootObservation
			if hasRoot {
				original, err = loadAbsoluteRoot(ctx, database, initial.Tokens.RefreshToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(original.OriginalTokenSignature).To(Equal(helpers.RefreshSignature(initial.Tokens.RefreshToken)))
				Expect(original.StartedAt.Before(sharedBefore)).To(BeFalse())
				Expect(original.StartedAt.After(sharedAfter)).To(BeFalse())
				Expect(original.AbsoluteExpiresAt).NotTo(BeNil())
				deadline = *original.AbsoluteExpiresAt
				Expect(deadline.Equal(original.StartedAt.Add(lifetime))).To(BeTrue())
			}

			environment.Stop()
			Expect(environment.Start()).To(Succeed())
			afterRestart, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, initial.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(afterRestart.Status).To(Equal(http.StatusOK))
			Expect(afterRestart.Tokens.RefreshToken).NotTo(BeEmpty())
			if hasRoot {
				root, err := loadAbsoluteRoot(ctx, database, afterRestart.Tokens.RefreshToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(root.OriginalTokenSignature).To(Equal(original.OriginalTokenSignature))
				Expect(root.StartedAt.Equal(original.StartedAt)).To(BeTrue())
				Expect(root.AbsoluteExpiresAt).NotTo(BeNil())
				Expect(root.AbsoluteExpiresAt.Equal(deadline)).To(BeTrue())
			}

			var behindClock time.Time
			replica, behindClock, stopReplica, err = bootstrap.NewClockSkewedRefreshEnvironment(refreshSuiteTestingT, config, -time.Minute)
			Expect(err).NotTo(HaveOccurred())
			shared, err := helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			Expect(behindClock.Before(shared.Add(-45 * time.Second))).To(BeTrue())
			onReplica, err := helpers.RotateRefreshSession(ctx, replica.Enduser.BaseURL(), *client, afterRestart.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(onReplica.Status).To(Equal(http.StatusOK))
			Expect(onReplica.Tokens.RefreshToken).NotTo(BeEmpty())
			stopReplica()
			stopReplica = nil
			ahead, aheadClock, stopAhead, err := bootstrap.NewClockSkewedRefreshEnvironment(refreshSuiteTestingT, config, time.Minute)
			stopReplica = stopAhead
			Expect(err).NotTo(HaveOccurred())
			shared, err = helpers.RefreshSharedTime(ctx, database)
			Expect(err).NotTo(HaveOccurred())
			Expect(aheadClock.After(shared.Add(45 * time.Second))).To(BeTrue())
			Eventually(func() (bool, error) {
				now, err := helpers.RefreshSharedTime(ctx, database)
				return !now.Before(deadline.Add(-3 * time.Second)), err
			}, lifetime, 20*time.Millisecond).Should(BeTrue())
			onAhead, err := helpers.RotateRefreshSession(ctx, ahead.Enduser.BaseURL(), *client, onReplica.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(onAhead.Status).To(Equal(http.StatusOK))
			var current struct {
				ExpiresAt time.Time `db:"expires_at"`
				Unused    bool      `db:"unused"`
			}
			Expect(database.GetContext(ctx, &current, `SELECT expires_at, used_at IS NULL AS unused
				FROM refresh_token_sessions WHERE signature = $1`,
				helpers.RefreshSignature(onAhead.Tokens.RefreshToken))).To(Succeed())
			Expect(current.Unused).To(BeTrue())
			Expect(current.ExpiresAt.After(deadline.Add(time.Second))).To(BeTrue())
			if hasRoot {
				root, err := loadAbsoluteRoot(ctx, database, onAhead.Tokens.RefreshToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(root.OriginalTokenSignature).To(Equal(original.OriginalTokenSignature))
				Expect(root.StartedAt.Equal(original.StartedAt)).To(BeTrue())
				Expect(root.AbsoluteExpiresAt).NotTo(BeNil())
				Expect(root.AbsoluteExpiresAt.Equal(deadline)).To(BeTrue())
				Expect(root.InactivityExpiresAt.After(deadline.Add(time.Second))).To(BeTrue())
			}

			Eventually(func() (bool, error) {
				now, err := helpers.RefreshSharedTime(ctx, database)
				return !now.Before(deadline.Add(100 * time.Millisecond)), err
			}, 4*time.Second, 20*time.Millisecond).Should(BeTrue())
			denied, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, onAhead.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(denied.Status).To(Equal(http.StatusBadRequest))
			Expect(denied.Error).To(Equal("invalid_grant"))
			_, hasAccess := denied.Body["access_token"]
			_, hasRefresh := denied.Body["refresh_token"]
			Expect(hasAccess).To(BeFalse())
			Expect(hasRefresh).To(BeFalse())
			deniedOnSkewed, err := helpers.RotateRefreshSession(ctx, ahead.Enduser.BaseURL(), *client, onAhead.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(deniedOnSkewed.Status).To(Equal(http.StatusBadRequest))
			Expect(deniedOnSkewed.Error).To(Equal("invalid_grant"))
			_, hasSkewedAccess := deniedOnSkewed.Body["access_token"]
			_, hasSkewedRefresh := deniedOnSkewed.Body["refresh_token"]
			Expect(hasSkewedAccess || hasSkewedRefresh).To(BeFalse())
			if hasRoot {
				root, err := loadAbsoluteRoot(ctx, database, onAhead.Tokens.RefreshToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(root.OriginalTokenSignature).To(Equal(original.OriginalTokenSignature))
				Expect(root.StartedAt.Equal(original.StartedAt)).To(BeTrue())
				Expect(root.AbsoluteExpiresAt).NotTo(BeNil())
				Expect(root.AbsoluteExpiresAt.Equal(deadline)).To(BeTrue())
			}
		})
	})
})

type absoluteRootObservation struct {
	OriginalTokenSignature string
	StartedAt              time.Time
	LastFreshAt            time.Time
	AbsoluteExpiresAt      *time.Time
	InactivityExpiresAt    time.Time
}

func loadAbsoluteRoot(ctx context.Context, database *sqlx.DB, token string) (absoluteRootObservation, error) {
	root, err := helpers.ReadRefreshRoot(ctx, database, token)
	if err != nil {
		return absoluteRootObservation{}, err
	}
	parseTimestamp := func(key string) (time.Time, error) {
		value, ok := root[key].(string)
		if !ok {
			return time.Time{}, fmt.Errorf("refresh root %s missing", key)
		}
		return time.Parse(time.RFC3339Nano, value)
	}
	started, err := parseTimestamp("started_at")
	if err != nil {
		return absoluteRootObservation{}, err
	}
	last, err := parseTimestamp("last_fresh_at")
	if err != nil {
		return absoluteRootObservation{}, err
	}
	inactivity, err := parseTimestamp("inactivity_expires_at")
	if err != nil {
		return absoluteRootObservation{}, err
	}
	var absolute *time.Time
	if root["absolute_expires_at"] != nil {
		value, err := parseTimestamp("absolute_expires_at")
		if err != nil {
			return absoluteRootObservation{}, err
		}
		absolute = &value
	}
	signature, ok := root["original_token_signature"].(string)
	if !ok {
		return absoluteRootObservation{}, fmt.Errorf("refresh root missing original signature")
	}
	return absoluteRootObservation{
		OriginalTokenSignature: signature,
		StartedAt:              started, LastFreshAt: last,
		AbsoluteExpiresAt: absolute, InactivityExpiresAt: inactivity,
	}, nil
}
