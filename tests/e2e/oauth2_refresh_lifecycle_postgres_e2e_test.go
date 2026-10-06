//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type lifecycleRotationOutcome struct {
	result *helpers.RefreshTokenResult
	err    error
}

type lifecycleDeletionOutcome struct {
	status int
	err    error
}

func lifecycleWaitingOnDatabase(ctx context.Context, database *sqlx.DB) (int, error) {
	var blocked int
	err := database.GetContext(ctx, &blocked, `
		SELECT count(*) FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid()
		AND wait_event_type = 'Lock'`)
	return blocked, err
}

var _ = Describe("Consent-Bound Refresh Sessions / US2 PostgreSQL", func() {
	var (
		ctx      context.Context
		env      *bootstrap.RefreshEnvironment
		database *sqlx.DB
		cleanup  func()
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("2m", "0s", "720h")
		Expect(err).NotTo(HaveOccurred())
		store, db, release, err := bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, config)
		Expect(err).NotTo(HaveOccurred())
		database, cleanup = db, release
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
	})

	AfterEach(func() {
		if env != nil {
			env.Stop()
		}
		if cleanup != nil {
			cleanup()
		}
	})

	// US2-S6 from specs/049-fix-refresh-consent/spec.md
	It("invalidates an overlapping replacement on both instances once grant deletion succeeds", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		authorization := lifecycleAuthorize(ctx, env, client)
		unrelated, err := env.AddClient(ctx, client.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		unrelatedAuthorization := lifecycleAuthorize(ctx, env, unrelated)
		replicaStore, err := storageadapter.NewAdapter(&env.Config.Storage)
		Expect(err).NotTo(HaveOccurred())
		replica, err := bootstrap.NewRefreshEnvironment(env.Config, replicaStore)
		Expect(err).NotTo(HaveOccurred())
		defer replica.Close()

		// Block the mutation, not a read-only preparation guard that may release first.
		_, err = database.ExecContext(ctx, `CREATE FUNCTION wait_lifecycle_rotation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
			BEGIN PERFORM pg_advisory_xact_lock(493006); RETURN NEW; END; $$;
			CREATE TRIGGER wait_lifecycle_rotation BEFORE UPDATE OF used_at ON refresh_tokens
			FOR EACH ROW WHEN (OLD.used_at IS NULL) EXECUTE FUNCTION wait_lifecycle_rotation()`)
		Expect(err).NotTo(HaveOccurred())
		locked, err := database.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = locked.Rollback() }()
		_, err = locked.ExecContext(ctx, "SELECT pg_advisory_xact_lock(493006)")
		Expect(err).NotTo(HaveOccurred())
		rotated := make(chan lifecycleRotationOutcome, 1)
		go func() {
			result, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			rotated <- lifecycleRotationOutcome{result, err}
		}()
		Eventually(func(g Gomega) {
			var blocked bool
			err := database.GetContext(ctx, &blocked, `SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity WHERE datname = current_database()
				AND wait_event = 'advisory' AND query LIKE '%UPDATE refresh_tokens SET used_at%')`)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(blocked).To(BeTrue())
		}, 6*time.Second, 20*time.Millisecond).Should(Succeed())

		deleted := make(chan lifecycleDeletionOutcome, 1)
		go func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodDelete,
				env.Enduser.BaseURL()+"/api/consent/agents/"+client.Agent.ID.String()+"/grants", nil)
			if err != nil {
				deleted <- lifecycleDeletionOutcome{err: err}
				return
			}
			request.Header.Set("X-Remote-User", client.Principal.String())
			response, err := helpers.HTTPClient().Do(request)
			if err != nil {
				deleted <- lifecycleDeletionOutcome{err: err}
				return
			}
			defer func() { _ = response.Body.Close() }()
			deleted <- lifecycleDeletionOutcome{status: response.StatusCode}
		}()
		// A correct guarded lifecycle request is another lock waiter. A prefeature
		// handler may instead return immediately; neither outcome requires a sleep.
		var early lifecycleDeletionOutcome
		returned := false
		Eventually(func() bool {
			select {
			case early = <-deleted:
				returned = true
				return true
			default:
			}
			blocked, err := lifecycleWaitingOnDatabase(ctx, database)
			return err == nil && blocked >= 2
		}, 6*time.Second, 20*time.Millisecond).Should(BeTrue())
		Expect(locked.Rollback()).To(Succeed())

		rotation := <-rotated
		Expect(rotation.err).NotTo(HaveOccurred())
		lifecycleExpectTokens(rotation.result)
		if !returned {
			early = <-deleted
		}
		Expect(early.err).NotTo(HaveOccurred())
		Expect(early.status).To(Equal(http.StatusNoContent))
		_, err = database.ExecContext(ctx, "DROP TRIGGER wait_lifecycle_rotation ON refresh_tokens; DROP FUNCTION wait_lifecycle_rotation()")
		Expect(err).NotTo(HaveOccurred())
		// Check the current replacement first: strict predecessor reuse must not
		// incidentally revoke an otherwise live family and mask a lifecycle failure.
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, replica, client, rotation.result.Tokens.RefreshToken))
		lifecycleExpectInvalidGrant(lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken))
		lifecycleExpectTokens(lifecycleRotate(ctx, replica, unrelated, unrelatedAuthorization.Tokens.RefreshToken))

		root, err := helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(root["terminal_reason"]).To(Equal("grant_deleted"))
		Expect(root["retry_ciphertext"]).To(BeNil())
		var receipts int
		Expect(database.GetContext(ctx, &receipts, `
			SELECT count(*) FROM refresh_revocation_receipts
			WHERE session_id = $1::uuid AND reason = 'grant_deleted'`, root["id"])).To(Succeed())
		Expect(receipts).To(Equal(1))
		for _, token := range []string{authorization.Tokens.RefreshToken, rotation.result.Tokens.RefreshToken} {
			var invalidated bool
			Expect(database.GetContext(ctx, &invalidated, `
				SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`,
				helpers.RefreshSignature(token))).To(Succeed())
			Expect(invalidated).To(BeTrue())
		}
	})

	// US2-S7 from specs/049-fix-refresh-consent/spec.md
	It("reports a lifecycle failure and rolls back the grant, root, and mirror on a real database write fault", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		other, err := env.AddClient(ctx, client.Principal, true)
		Expect(err).NotTo(HaveOccurred())
		authorization := lifecycleAuthorize(ctx, env, client)
		unrelated := lifecycleAuthorize(ctx, env, other)

		// Only the target current token's revocation write fails. This is a real
		// PostgreSQL write failure, not a swapped repository or mocked service.
		_, err = database.ExecContext(ctx, `
			CREATE FUNCTION reject_lifecycle_legacy_write() RETURNS trigger
			LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected legacy revocation write failure'; END $$`)
		Expect(err).NotTo(HaveOccurred())
		_, err = database.ExecContext(ctx, fmt.Sprintf(`
			CREATE TRIGGER reject_lifecycle_legacy_write BEFORE UPDATE OF used_at
			ON refresh_token_sessions FOR EACH ROW
			WHEN (OLD.signature = '%s' AND OLD.used_at IS NULL AND NEW.used_at IS NOT NULL)
			EXECUTE FUNCTION reject_lifecycle_legacy_write()`, helpers.RefreshSignature(authorization.Tokens.RefreshToken)))
		Expect(err).NotTo(HaveOccurred())

		status := lifecycleDelete(ctx, env.Enduser, "/api/consent/agents/"+client.Agent.ID.String()+"/grants", client.Principal)
		Expect(status).To(Equal(http.StatusInternalServerError), "revocation write failure must not report success")
		response, err := env.Enduser.AuthenticatedGET("/api/consent/agents/"+client.Agent.ID.String()+"/grants", client.Principal.String())
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = response.Body.Close() }()
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		var grant map[string]any
		Expect(json.NewDecoder(response.Body).Decode(&grant)).To(Succeed())
		Expect(grant["data"]).NotTo(BeNil())

		root, err := helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(root["revoked_at"]).To(BeNil())
		Expect(root["terminal_reason"]).To(BeNil())
		var stillCurrent bool
		Expect(database.GetContext(ctx, &stillCurrent, `
			SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`,
			helpers.RefreshSignature(authorization.Tokens.RefreshToken))).To(Succeed())
		Expect(stillCurrent).To(BeTrue())
		var receipts int
		Expect(database.GetContext(ctx, &receipts, `
			SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1::uuid`, root["id"])).To(Succeed())
		Expect(receipts).To(BeZero())
		lifecycleExpectTokens(lifecycleRotate(ctx, env, other, unrelated.Tokens.RefreshToken))

		_, err = database.ExecContext(ctx, `DROP TRIGGER reject_lifecycle_legacy_write ON refresh_token_sessions`)
		Expect(err).NotTo(HaveOccurred())
		lifecycleExpectTokens(lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken))
	})

	// US2-S8 from specs/049-fix-refresh-consent/spec.md
	It("retains clocks and committed retry counts while only replacement credentials recover the same pair", func() {
		client, err := env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		authorization := lifecycleAuthorize(ctx, env, client)
		rotated := lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken)
		lifecycleExpectTokens(rotated)
		before, beforeErr := helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		oldCredential := *client
		Expect(env.ReplaceCredentials(ctx, client)).To(Succeed())
		// The prefeature schema lacks refresh_sessions. Do not fail setup on it:
		// the first changed-behavior assertion is recovery with the new secret.
		var afterReplacement map[string]any
		var afterReplacementErr error
		if beforeErr == nil {
			afterReplacement, afterReplacementErr = helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		}
		oldDenied := lifecycleRotate(ctx, env, &oldCredential, authorization.Tokens.RefreshToken)
		Expect(oldDenied.Status).To(Equal(http.StatusUnauthorized))
		lifecycleExpectNoTokens(oldDenied)

		recovered := lifecycleRotate(ctx, env, client, authorization.Tokens.RefreshToken)
		lifecycleExpectTokens(recovered)
		Expect(helpers.RefreshSignature(recovered.Tokens.AccessToken)).To(Equal(helpers.RefreshSignature(rotated.Tokens.AccessToken)))
		Expect(helpers.RefreshSignature(recovered.Tokens.RefreshToken)).To(Equal(helpers.RefreshSignature(rotated.Tokens.RefreshToken)))

		Expect(beforeErr).NotTo(HaveOccurred(), "newly issued session must have persisted root state")
		Expect(afterReplacementErr).NotTo(HaveOccurred())
		for _, field := range []string{"started_at", "last_fresh_at", "inactivity_expires_at", "absolute_expires_at", "previous_consumed_at", "reuse_until", "retry_expires_at", "current_signature", "retry_count"} {
			if before[field] == nil {
				Expect(afterReplacement[field]).To(BeNil(), "credential generation changed %s", field)
			} else {
				Expect(afterReplacement[field]).To(Equal(before[field]), "credential generation changed %s", field)
			}
		}
		afterRecovery, err := helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		for _, field := range []string{"started_at", "last_fresh_at", "inactivity_expires_at", "absolute_expires_at", "previous_consumed_at", "reuse_until", "retry_expires_at", "current_signature"} {
			if before[field] == nil {
				Expect(afterRecovery[field]).To(BeNil(), "eligible retry changed %s", field)
			} else {
				Expect(afterRecovery[field]).To(Equal(before[field]), "eligible retry changed %s", field)
			}
		}
		Expect(afterRecovery["retry_count"]).To(BeNumerically("==", before["retry_count"].(float64)+1))
		fresh := lifecycleRotate(ctx, env, client, rotated.Tokens.RefreshToken)
		lifecycleExpectTokens(fresh)
		current, err := helpers.ReadRefreshRoot(ctx, database, authorization.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(current["started_at"]).To(Equal(before["started_at"]))
		Expect(current["last_fresh_at"]).NotTo(Equal(before["last_fresh_at"]))
		oldDenied = lifecycleRotate(ctx, env, &oldCredential, fresh.Tokens.RefreshToken)
		Expect(oldDenied.Status).To(Equal(http.StatusUnauthorized))
		lifecycleExpectNoTokens(oldDenied)
	})
})
