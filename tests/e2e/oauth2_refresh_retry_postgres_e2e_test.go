//go:build integration

package e2e_test

import (
	"context"
	cryptorand "crypto/rand"
	"io"
	"net/http"
	"strings"
	"sync"
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

type delayedRefreshEntropy struct {
	reader io.Reader
	delay  sync.Once
}

func (r *delayedRefreshEntropy) Read(value []byte) (int, error) {
	n, err := r.reader.Read(value)
	if len(value) == 32 {
		r.delay.Do(func() { <-time.NewTimer(2250 * time.Millisecond).C })
	}
	return n, err
}

func expectRetryServerError(result *helpers.RefreshTokenResult) {
	ExpectWithOffset(1, result.Status).To(Equal(http.StatusInternalServerError))
	ExpectWithOffset(1, result.Error).To(Equal("server_error"))
	_, hasAccess := result.Body["access_token"]
	_, hasRefresh := result.Body["refresh_token"]
	ExpectWithOffset(1, hasAccess).To(BeFalse(), "server errors must not contain an access token")
	ExpectWithOffset(1, hasRefresh).To(BeFalse(), "server errors must not contain a refresh token")
}

func expectIndeterminateRetryError(result *helpers.RefreshTokenResult) {
	expectRetryServerError(result)
	_, claimsRollback := result.Body["rollback"]
	_, claimsRolledBack := result.Body["rolled_back"]
	ExpectWithOffset(1, claimsRollback || claimsRolledBack).To(BeFalse(), "a lost acknowledgement cannot claim rollback")
	if description, ok := result.Body["error_description"].(string); ok {
		ExpectWithOffset(1, strings.ToLower(description)).NotTo(ContainSubstring("rolled back"))
	}
}

func readRetryState(ctx context.Context, db *sqlx.DB, token string) map[string]any {
	root, err := helpers.ReadRefreshRoot(ctx, db, token)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return root
}

func countRetryChildren(ctx context.Context, db *sqlx.DB, sessionID any) int {
	var count int
	ExpectWithOffset(1, db.GetContext(ctx, &count,
		"SELECT count(*) FROM refresh_tokens WHERE session_id = $1", sessionID)).To(Succeed())
	return count
}

var _ = Describe("Consent-Bound Refresh Sessions / US3 PostgreSQL", func() {
	var (
		ctx     context.Context
		cfg     *ports.Config
		env     *bootstrap.RefreshEnvironment
		db      *sqlx.DB
		cleanup func()
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		cfg, err = fixtures.RefreshConfig("60s", "", "")
		Expect(err).NotTo(HaveOccurred())
		var storage *storageadapter.Adapter
		storage, db, cleanup, err = bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, cfg)
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(cfg, storage)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
	})

	AfterEach(func() {
		if env != nil {
			env.Close()
		}
		if cleanup != nil {
			cleanup()
		}
	})

	// US3-S6 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S6 recovers the identical pair on a separate replica and after restart", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(rotated)

		replicaStorage, err := storageadapter.NewAdapter(&cfg.Storage)
		Expect(err).NotTo(HaveOccurred())
		replica, err := bootstrap.NewRefreshEnvironment(cfg, replicaStorage)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			if replica != nil {
				replica.Close()
			}
		}()
		replicaResult, err := helpers.RotateRefreshSession(ctx, replica.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(replicaResult)
		Expect(replicaResult.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "replica must return original access token")
		Expect(replicaResult.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "replica must return original successor")
		Expect(replicaResult.Tokens.ExpiresIn).To(BeNumerically("<=", rotated.Tokens.ExpiresIn))

		root := readRetryState(ctx, db, issued.Tokens.RefreshToken)
		Expect(root["retry_count"]).To(BeNumerically("==", 1))
		Expect(root["current_signature"]).To(Equal(helpers.RefreshSignature(rotated.Tokens.RefreshToken)))
		env.Close()
		env = nil
		replica.Close()
		replica = nil

		restartedStorage, err := storageadapter.NewAdapter(&cfg.Storage)
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(cfg, restartedStorage)
		Expect(err).NotTo(HaveOccurred())
		restartedResult, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(restartedResult)
		Expect(restartedResult.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "restart must return original access token")
		Expect(restartedResult.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "restart must return original successor")
		Expect(readRetryState(ctx, db, issued.Tokens.RefreshToken)["retry_count"]).To(BeNumerically("==", 2))
		usable, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(usable)
		Expect(usable.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeFalse())
	})

	// US3-S8 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S8 expires the signed cached access result before grace without consuming its successor", Serial, func() {
		// Refresh entropy is obtained after JWT minting and before consumption.
		// Delay that real operation while keeping 5s reuse < 6s token_ttl.
		env.Close()
		env = nil
		short, err := fixtures.RefreshConfig("5s", "", "45s")
		Expect(err).NotTo(HaveOccurred())
		short.OAuth2AuthServer.Local.TokenTTL = 6 * time.Second
		short.Storage = cfg.Storage
		shortStorage, err := storageadapter.NewAdapter(&short.Storage)
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(short, shortStorage)
		Expect(err).NotTo(HaveOccurred())
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		originalEntropy := cryptorand.Reader
		cryptorand.Reader = &delayedRefreshEntropy{reader: originalEntropy}
		DeferCleanup(func() { cryptorand.Reader = originalEntropy })

		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(rotated)
		cryptorand.Reader = originalEntropy
		claims := verifyPublishedConsentJWT(env.Enduser, rotated.Tokens.AccessToken)
		expiresAt, hasExpiry := claims.Expiration()
		Expect(hasExpiry).To(BeTrue())
		Expect(time.Now().Before(expiresAt)).To(BeTrue(), "JWT must initially be live even after the real insert delay")
		eligible, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(eligible)
		Expect(eligible.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "eligible retry returns the original signed access token")
		Expect(eligible.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "eligible retry returns the original successor")
		Expect(time.Now().Before(expiresAt)).To(BeTrue(), "positive control must finish while cached access remains live")
		before, beforeErr := helpers.ReadRefreshRoot(ctx, db, issued.Tokens.RefreshToken)
		Expect(beforeErr).NotTo(HaveOccurred()) // Already passed the acceptance-linked identical recovery assertion.
		reuseText, ok := before["reuse_until"].(string)
		Expect(ok).To(BeTrue())
		reuseUntil, err := time.Parse(time.RFC3339Nano, reuseText)
		Expect(err).NotTo(HaveOccurred())
		Expect(expiresAt.Before(reuseUntil)).To(BeTrue(), "signed access expiry must precede this predecessor's persisted reuse deadline")
		waitUntilRetryTime(expiresAt)
		expired, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryInvalidGrant(expired)
		observedAt, clockErr := helpers.RefreshSharedTime(ctx, db)
		Expect(clockErr).NotTo(HaveOccurred())
		after, afterErr := helpers.ReadRefreshRoot(ctx, db, issued.Tokens.RefreshToken)

		current, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(current) // Consumer-visible proof expiry alone preserved the successor.
		Expect(current.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeFalse())

		Expect(afterErr).NotTo(HaveOccurred())
		Expect(observedAt.Before(reuseUntil)).To(BeTrue(), "expired access must be checked before the reuse window closes")
		for _, field := range []string{"current_signature", "last_fresh_at", "inactivity_expires_at", "reuse_until", "retry_expires_at", "retry_count"} {
			Expect(after[field]).To(Equal(before[field]), "expired access must not advance "+field)
		}
		Expect(after["revoked_at"]).To(BeNil())
	})

	// US3-S11 from specs/049-fix-refresh-consent/spec.md.
	It("US3-S11 resolves lost PostgreSQL COMMIT acknowledgement from durable state without branches or count refunds", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		directURL := cfg.Storage.Postgres.ConnectionURL
		env.Close()
		env = nil

		fault, err := helpers.NewRefreshCommitFault(directURL)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			if env != nil {
				env.Close()
				env = nil
			}
			Expect(fault.Close()).To(Succeed())
		}()
		cfg.Storage.Postgres.ConnectionURL = fault.ConnectionURL()
		proxyStorage, err := storageadapter.NewAdapter(&cfg.Storage)
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(cfg, proxyStorage)
		Expect(err).NotTo(HaveOccurred())

		By("suppressing the committed fresh-rotation acknowledgement")
		rotationCommit, err := fault.ArmCommitLoss()
		Expect(err).NotTo(HaveOccurred())
		lostRotation, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectIndeterminateRetryError(lostRotation)
		var committed bool
		Eventually(rotationCommit, 3*time.Second).Should(Receive(&committed))
		Expect(committed).To(BeTrue(), "PostgreSQL must have committed before acknowledgement was suppressed")
		// The legacy mirror is independently observable even before migration 036.
		var consumed bool
		Expect(db.GetContext(ctx, &consumed,
			"SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1",
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		Expect(consumed).To(BeTrue(), "PostgreSQL durably consumed the predecessor")
		var legacyCount int
		Expect(db.GetContext(ctx, &legacyCount, `SELECT count(*) FROM refresh_token_sessions
			WHERE request_id = (SELECT request_id FROM refresh_token_sessions WHERE signature = $1)`,
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		Expect(legacyCount).To(Equal(2), "exactly one durable replacement must exist")
		var committedSignature string
		Expect(db.GetContext(ctx, &committedSignature, `SELECT signature FROM refresh_token_sessions
			WHERE request_id = (SELECT request_id FROM refresh_token_sessions WHERE signature = $1)
			AND signature <> $1`, helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		fault.SetUnavailable(true)
		unresolved, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectIndeterminateRetryError(unresolved)
		Expect(db.GetContext(ctx, &legacyCount, `SELECT count(*) FROM refresh_token_sessions
			WHERE request_id = (SELECT request_id FROM refresh_token_sessions WHERE signature = $1)`,
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		Expect(legacyCount).To(Equal(2), "unavailable resolution cannot create a successor")
		fault.SetUnavailable(false)

		recovered, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(recovered)
		Expect(helpers.RefreshSignature(recovered.Tokens.RefreshToken)).To(Equal(committedSignature), "recovery must return the already durable successor")
		root := readRetryState(ctx, db, issued.Tokens.RefreshToken)
		Expect(root["previous_signature"]).To(Equal(helpers.RefreshSignature(issued.Tokens.RefreshToken)))
		Expect(root["current_signature"]).NotTo(Equal(root["previous_signature"]))
		Expect(root["retry_count"]).To(BeNumerically("==", 1))
		Expect(countRetryChildren(ctx, db, root["id"])).To(Equal(2))
		initialDeadline := root["reuse_until"]
		initialActivity := root["inactivity_expires_at"]

		Expect(helpers.RefreshSignature(recovered.Tokens.RefreshToken)).To(Equal(root["current_signature"]))

		By("suppressing a committed stored-result count acknowledgement")
		retryCommit, err := fault.ArmCommitLoss()
		Expect(err).NotTo(HaveOccurred())
		lostRetry, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectIndeterminateRetryError(lostRetry)
		Eventually(retryCommit, 3*time.Second).Should(Receive(&committed))
		Expect(committed).To(BeTrue(), "retry-count increment must be committed despite lost acknowledgement")
		root = readRetryState(ctx, db, issued.Tokens.RefreshToken)
		Expect(root["retry_count"]).To(BeNumerically("==", 2))
		Expect(root["reuse_until"]).To(Equal(initialDeadline))
		Expect(root["inactivity_expires_at"]).To(Equal(initialActivity))
		Expect(countRetryChildren(ctx, db, root["id"])).To(Equal(2))

		lastAllowed, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(lastAllowed)
		Expect(lastAllowed.Tokens.AccessToken == recovered.Tokens.AccessToken).To(BeTrue())
		Expect(lastAllowed.Tokens.RefreshToken == recovered.Tokens.RefreshToken).To(BeTrue())
		Expect(readRetryState(ctx, db, issued.Tokens.RefreshToken)["retry_count"]).To(BeNumerically("==", 3))
		fourth, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryInvalidGrant(fourth)
		current, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, recovered.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryInvalidGrant(current)
		Expect(readRetryState(ctx, db, issued.Tokens.RefreshToken)["terminal_reason"]).To(Equal("prohibited_reuse"))

		// A deferrable PostgreSQL constraint fails at COMMIT with an acknowledged
		// ErrorResponse: unlike an interrupted acknowledgement, this proves rollback.
		rollbackSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		beforeRollback := readRetryState(ctx, db, rollbackSession.Tokens.RefreshToken)
		_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_refresh_retry_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test-owned refresh rotation rollback' USING ERRCODE = '23514'; END $$`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `CREATE CONSTRAINT TRIGGER reject_refresh_retry_commit
		AFTER INSERT ON refresh_tokens DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
		EXECUTE FUNCTION reject_refresh_retry_commit()`)
		Expect(err).NotTo(HaveOccurred())
		confirmedRollback, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rollbackSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryServerError(confirmedRollback)
		afterRollback := readRetryState(ctx, db, rollbackSession.Tokens.RefreshToken)
		Expect(afterRollback["current_signature"]).To(Equal(beforeRollback["current_signature"]))
		Expect(afterRollback["previous_signature"]).To(BeNil())
		Expect(afterRollback["retry_count"]).To(BeNumerically("==", 0))
		Expect(countRetryChildren(ctx, db, beforeRollback["id"])).To(Equal(1))
		_, err = db.ExecContext(ctx, `DROP TRIGGER reject_refresh_retry_commit ON refresh_tokens`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `DROP FUNCTION reject_refresh_retry_commit()`)
		Expect(err).NotTo(HaveOccurred())
		postRollback, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rollbackSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(postRollback)
		Expect(helpers.RefreshSignature(postRollback.Tokens.RefreshToken) == helpers.RefreshSignature(rollbackSession.Tokens.RefreshToken)).To(BeFalse())

		By("confirming a rejected retry-count COMMIT preserves the stored pair and count")
		beforeRetryRollback := readRetryState(ctx, db, rollbackSession.Tokens.RefreshToken)
		_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_refresh_count_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test-owned retry count rollback' USING ERRCODE = '23514'; END $$`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `CREATE CONSTRAINT TRIGGER reject_refresh_count_commit
		AFTER UPDATE ON refresh_sessions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
		WHEN (NEW.retry_count > OLD.retry_count) EXECUTE FUNCTION reject_refresh_count_commit()`)
		Expect(err).NotTo(HaveOccurred())
		rolledBackRetry, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rollbackSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryServerError(rolledBackRetry)
		afterRetryRollback := readRetryState(ctx, db, rollbackSession.Tokens.RefreshToken)
		for _, field := range []string{"current_signature", "previous_signature", "retry_count", "last_fresh_at", "reuse_until", "retry_expires_at", "retry_ciphertext"} {
			Expect(afterRetryRollback[field]).To(Equal(beforeRetryRollback[field]), "confirmed rollback changed "+field)
		}
		_, err = db.ExecContext(ctx, `DROP TRIGGER reject_refresh_count_commit ON refresh_sessions`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `DROP FUNCTION reject_refresh_count_commit()`)
		Expect(err).NotTo(HaveOccurred())
		afterRetryFailure, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rollbackSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(afterRetryFailure)
		Expect(afterRetryFailure.Tokens.AccessToken == postRollback.Tokens.AccessToken).To(BeTrue())
		Expect(afterRetryFailure.Tokens.RefreshToken == postRollback.Tokens.RefreshToken).To(BeTrue())
		Expect(readRetryState(ctx, db, rollbackSession.Tokens.RefreshToken)["retry_count"]).To(BeNumerically("==", 1))
		Expect(countRetryChildren(ctx, db, beforeRollback["id"])).To(Equal(2))

		// Restart on the same database with strict zero reuse; a committed lost
		// rotation cannot be retrieved under this operator policy.
		env.Close()
		env = nil
		zero, err := fixtures.RefreshConfig("0s", "", "")
		Expect(err).NotTo(HaveOccurred())
		zero.Storage = cfg.Storage
		zeroStorage, err := storageadapter.NewAdapter(&zero.Storage)
		Expect(err).NotTo(HaveOccurred())
		env, err = bootstrap.NewRefreshEnvironment(zero, zeroStorage)
		Expect(err).NotTo(HaveOccurred())
		zeroSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		zeroCommit, err := fault.ArmCommitLoss()
		Expect(err).NotTo(HaveOccurred())
		zeroLost, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, zeroSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectIndeterminateRetryError(zeroLost)
		Eventually(zeroCommit, 3*time.Second).Should(Receive(&committed))
		Expect(committed).To(BeTrue())
		zeroRoot := readRetryState(ctx, db, zeroSession.Tokens.RefreshToken)
		Expect(zeroRoot["previous_signature"]).To(Equal(helpers.RefreshSignature(zeroSession.Tokens.RefreshToken)))
		Expect(zeroRoot["retry_count"]).To(BeNumerically("==", 0))
		Expect(countRetryChildren(ctx, db, zeroRoot["id"])).To(Equal(2))
		fault.SetUnavailable(true)
		zeroUnresolved, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, zeroSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectIndeterminateRetryError(zeroUnresolved)
		Expect(readRetryState(ctx, db, zeroSession.Tokens.RefreshToken)["retry_count"]).To(BeNumerically("==", 0))
		Expect(countRetryChildren(ctx, db, zeroRoot["id"])).To(Equal(2))
		fault.SetUnavailable(false)
		zeroRetry, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, zeroSession.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryInvalidGrant(zeroRetry)
		Expect(readRetryState(ctx, db, zeroSession.Tokens.RefreshToken)["terminal_reason"]).To(Equal("prohibited_reuse"))
		fresh, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		By("confirming zero-reuse rotation rollback leaves the original token usable")
		zeroBeforeRollback := readRetryState(ctx, db, fresh.Tokens.RefreshToken)
		_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_zero_refresh_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test-owned zero rotation rollback' USING ERRCODE = '23514'; END $$`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `CREATE CONSTRAINT TRIGGER reject_zero_refresh_commit
		AFTER INSERT ON refresh_tokens DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
		EXECUTE FUNCTION reject_zero_refresh_commit()`)
		Expect(err).NotTo(HaveOccurred())
		zeroRolledBack, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, fresh.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryServerError(zeroRolledBack)
		zeroAfterRollback := readRetryState(ctx, db, fresh.Tokens.RefreshToken)
		Expect(zeroAfterRollback["current_signature"]).To(Equal(zeroBeforeRollback["current_signature"]))
		Expect(zeroAfterRollback["previous_signature"]).To(BeNil())
		Expect(zeroAfterRollback["retry_count"]).To(BeNumerically("==", 0))
		Expect(countRetryChildren(ctx, db, zeroBeforeRollback["id"])).To(Equal(1))
		_, err = db.ExecContext(ctx, `DROP TRIGGER reject_zero_refresh_commit ON refresh_tokens`)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `DROP FUNCTION reject_zero_refresh_commit()`)
		Expect(err).NotTo(HaveOccurred())
		freshResult, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, fresh.Tokens.RefreshToken, "read")
		Expect(err).NotTo(HaveOccurred())
		expectRetryTokens(freshResult)
	})
})
