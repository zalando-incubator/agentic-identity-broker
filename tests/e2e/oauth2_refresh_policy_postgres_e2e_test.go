//go:build integration

package e2e_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func policyOldBinary() string {
	binary := os.Getenv("AIB_REFRESH_PREFEATURE_BINARY")
	Expect(binary).NotTo(BeEmpty(), "set AIB_REFRESH_PREFEATURE_BINARY to the captured pre-feature broker executable")
	info, err := os.Stat(binary)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Mode().IsRegular()).To(BeTrue())
	file, err := os.Open(binary)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	_, err = io.Copy(digest, file)
	Expect(err).NotTo(HaveOccurred())
	Expect(hex.EncodeToString(digest.Sum(nil))).To(Equal("a174ef70679f170cc8e93d285d101d47a2da8a3c6aa773aeab94bb93f2c9a70b"), "wrong pre-feature issuer fixture")
	return binary
}

func policyPersistedClient(ctx context.Context, store *storageadapter.Adapter) fixtures.RefreshClient {
	principal := policyPrincipal()
	agent := fixtures.RefreshAgent()
	Expect(store.Agents().Create(ctx, agent)).To(Succeed())
	grant := fixtures.RefreshGrant(principal, agent.ID)
	Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
	return fixtures.RefreshClient{Agent: agent, Grant: grant, Principal: principal}
}

func policyRoot(ctx context.Context, db *sqlx.DB, token string) map[string]any {
	root, err := helpers.ReadRefreshRoot(ctx, db, token)
	Expect(err).NotTo(HaveOccurred())
	return root
}

func policyRootTime(root map[string]any, field string) time.Time {
	encoded, ok := root[field].(string)
	Expect(ok).To(BeTrue(), "root must retain %s", field)
	parsed, err := time.Parse(time.RFC3339Nano, encoded)
	Expect(err).NotTo(HaveOccurred())
	return parsed
}

func policyTokenExpires(ctx context.Context, db *sqlx.DB, token string) time.Time {
	var expires time.Time
	Expect(db.GetContext(ctx, &expires, "SELECT expires_at FROM refresh_tokens WHERE signature = $1", helpers.RefreshSignature(token))).To(Succeed())
	return expires
}

func policyExpectLegacyBlocked(ctx context.Context, db *sqlx.DB, token string) {
	var used bool
	Expect(db.GetContext(ctx, &used,
		"SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1", helpers.RefreshSignature(token))).To(Succeed())
	Expect(used).To(BeTrue(), "terminal expiry must invalidate the legacy mirror before old binaries resume")
}

func policyAssertRootUnchanged(before, after map[string]any) {
	for _, field := range []string{
		"current_signature", "previous_signature", "last_fresh_at", "inactivity_expires_at",
		"absolute_expires_at", "revoked_at", "expired_at", "terminal_reason", "retry_count", "retry_ciphertext",
	} {
		Expect(after[field] == before[field]).To(BeTrue(), "unsupported writer must preserve %s", field)
	}
}

func policyStopPG(environment *bootstrap.RefreshEnvironment) {
	if environment == nil {
		return
	}
	environment.Stop()
	if environment.App != nil && environment.App.Shutdown != nil {
		Expect(environment.App.Shutdown(context.Background())).To(Succeed())
	}
}

var _ = Describe("Consent-Bound Refresh Sessions / US6 PostgreSQL", func() {
	var (
		ctx     context.Context
		config  *ports.Config
		store   *storageadapter.Adapter
		db      *sqlx.DB
		cleanup func()
		env     *bootstrap.RefreshEnvironment
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		config, err = fixtures.RefreshConfig("1s", "0s", "10m")
		Expect(err).NotTo(HaveOccurred())
		store, db, cleanup, err = bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, config)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, store)).To(Succeed())
	})

	AfterEach(func() {
		if env != nil {
			policyStopPG(env)
		}
		if cleanup != nil {
			cleanup()
		}
	})

	withStorage := func(reuse, absolute, inactivity string) *ports.Config {
		policy, err := fixtures.RefreshConfig(reuse, absolute, inactivity)
		Expect(err).NotTo(HaveOccurred())
		policy.Storage = config.Storage
		return policy
	}

	rebuildHistory := func(age func(*sqlx.DB) error) {
		policyStopPG(env)
		oldCleanup := cleanup
		historicalStore, historicalDB, closeHistorical, err := bootstrap.NewHistoricalRefreshFixture(refreshSuiteTestingT, config, db, age)
		Expect(err).NotTo(HaveOccurred())
		store, db = historicalStore, historicalDB
		cleanup = func() { closeHistorical(); oldCleanup() }
	}

	// US6-S4 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S4 requires new authorization for every pre-feature unanchored family while complete new-issuer evidence survives restart", func() {
		client := policyPersistedClient(ctx, store)
		old := launchPolicyBroker(policyOldBinary(), policyConfigPath(policyConfigYAML("30s", "0s", "10m", config.Storage.Postgres.ConnectionURL)), nil)
		Expect(helpers.ProvisionSigningKey(old.AdminURL)).To(Succeed())
		legacy, err := helpers.AuthorizeRefreshSession(ctx, old.EnduserURL, client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		legacyRotated, err := helpers.RotateRefreshSession(ctx, old.EnduserURL, client, legacy.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(legacyRotated.Status).To(Equal(http.StatusOK), "pre-feature writer must have issued a real descendant")
		untouched, err := helpers.AuthorizeRefreshSession(ctx, old.EnduserURL, client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		Expect(untouched.Tokens.RefreshToken != legacyRotated.Tokens.RefreshToken).To(BeTrue())
		old.Close()

		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		for _, unsupported := range []string{legacyRotated.Tokens.RefreshToken, untouched.Tokens.RefreshToken, legacy.Tokens.RefreshToken} {
			denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), client, unsupported, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(denied)
		}
		fresh, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		root := policyRoot(ctx, db, fresh.Tokens.RefreshToken)
		Expect(root["original_grant_id"]).To(Equal(client.Grant.ID.String()))
		Expect(root["original_token_signature"]).To(Equal(helpers.RefreshSignature(fresh.Tokens.RefreshToken)))
		Expect(root["agent_id"]).To(Equal(client.Agent.ID.String()))
		Expect(root["principal"]).To(Equal(client.Principal.String()))
		Expect(root["client_id"]).To(Equal(client.Agent.ID.String()))
		Expect(root["previous_signature"]).To(BeNil())
		Expect(root["revoked_at"]).To(BeNil())
		started := policyRootTime(root, "started_at")
		Expect(policyRootTime(root, "last_fresh_at")).To(BeTemporally("~", started, time.Millisecond))
		Expect(policyRootTime(root, "inactivity_expires_at")).To(BeTemporally(">", started))
		var firstIssued time.Time
		Expect(db.GetContext(ctx, &firstIssued, "SELECT issued_at FROM refresh_tokens WHERE signature = $1", helpers.RefreshSignature(fresh.Tokens.RefreshToken))).To(Succeed())
		Expect(firstIssued).To(BeTemporally("~", started, time.Millisecond))
		policyStopPG(env)
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		continued, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), client, fresh.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(continued.Status).To(Equal(http.StatusOK))
		updated := policyRoot(ctx, db, continued.Tokens.RefreshToken)
		Expect(updated["original_grant_id"]).To(Equal(root["original_grant_id"]))
		Expect(updated["original_token_signature"]).To(Equal(root["original_token_signature"]))
		Expect(updated["started_at"]).To(Equal(root["started_at"]))
		Expect(updated["previous_signature"]).To(Equal(helpers.RefreshSignature(fresh.Tokens.RefreshToken)))
		Expect(updated["current_signature"]).To(Equal(helpers.RefreshSignature(continued.Tokens.RefreshToken)))
		Expect(policyRootTime(updated, "last_fresh_at")).To(BeTemporally(">", started))
		var predecessorUsed bool
		Expect(db.GetContext(ctx, &predecessorUsed, "SELECT used_at IS NOT NULL FROM refresh_tokens WHERE signature = $1", helpers.RefreshSignature(fresh.Tokens.RefreshToken))).To(Succeed())
		Expect(predecessorUsed).To(BeTrue())
	})

	// US6-S6 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S6 persists shortened lifetimes and next-rotation increases; reconciled terminal expiry defeats schema-preserving binary rollback", func() {
		var err error
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		absoluteClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		absoluteSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *absoluteClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		survivorClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		survivor, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *survivorClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rebuildHistory(func(historical *sqlx.DB) error {
			if err := helpers.AgeInitialRefreshSession(ctx, historical, absoluteSession.Tokens.RefreshToken, 3*time.Minute); err != nil {
				return err
			}
			return helpers.AgeInitialRefreshSession(ctx, historical, survivor.Tokens.RefreshToken, 20*time.Second)
		})

		shortAbsolute := withStorage("1s", "2m", "10m")
		env, err = bootstrap.NewRefreshEnvironment(shortAbsolute, store)
		Expect(err).NotTo(HaveOccurred())
		absoluteDenied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *absoluteClient, absoluteSession.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(absoluteDenied)
		absoluteRoot := policyRoot(ctx, db, absoluteSession.Tokens.RefreshToken)
		Expect(absoluteRoot["terminal_reason"]).To(Equal("absolute_expiry"))
		policyExpectLegacyBlocked(ctx, db, absoluteSession.Tokens.RefreshToken)
		Expect(absoluteRoot["expired_at"]).NotTo(BeNil())
		Expect(policyRootTime(absoluteRoot, "absolute_expires_at")).To(BeTemporally("~", policyRootTime(absoluteRoot, "started_at").Add(2*time.Minute), time.Second))
		survivingRoot := policyRoot(ctx, db, survivor.Tokens.RefreshToken)
		Expect(policyRootTime(survivingRoot, "absolute_expires_at")).To(BeTemporally("~", policyRootTime(survivingRoot, "started_at").Add(2*time.Minute), time.Second))
		stillValid, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *survivorClient, survivor.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(stillValid.Status).To(Equal(http.StatusOK))

		policyStopPG(env)
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		idleClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		idleSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *idleClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		idleSurvivorClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		idleSurvivor, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *idleSurvivorClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rebuildHistory(func(historical *sqlx.DB) error {
			if err := helpers.AgeInitialRefreshSession(ctx, historical, idleSession.Tokens.RefreshToken, 3*time.Minute); err != nil {
				return err
			}
			return helpers.AgeInitialRefreshSession(ctx, historical, idleSurvivor.Tokens.RefreshToken, 20*time.Second)
		})

		shortInactivity := withStorage("1s", "0s", "2m")
		env, err = bootstrap.NewRefreshEnvironment(shortInactivity, store)
		Expect(err).NotTo(HaveOccurred())
		idleDenied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *idleClient, idleSession.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(idleDenied)
		idleRoot := policyRoot(ctx, db, idleSession.Tokens.RefreshToken)
		Expect(idleRoot["terminal_reason"]).To(Equal("inactivity_expiry"))
		Expect(idleRoot["expired_at"]).NotTo(BeNil())
		policyExpectLegacyBlocked(ctx, db, idleSession.Tokens.RefreshToken)
		Expect(policyRootTime(idleRoot, "inactivity_expires_at")).To(BeTemporally("~", policyRootTime(idleRoot, "started_at").Add(2*time.Minute), time.Second))
		idleActiveRoot := policyRoot(ctx, db, idleSurvivor.Tokens.RefreshToken)
		Expect(policyRootTime(idleActiveRoot, "inactivity_expires_at")).To(BeTemporally("~", policyRootTime(idleActiveRoot, "started_at").Add(2*time.Minute), time.Second))
		idleStillValid, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *idleSurvivorClient, idleSurvivor.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(idleStillValid.Status).To(Equal(http.StatusOK))

		increaseClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		increaseSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *increaseClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		before := policyRoot(ctx, db, increaseSession.Tokens.RefreshToken)
		oldTokenExpiry := policyTokenExpires(ctx, db, increaseSession.Tokens.RefreshToken)
		policyStopPG(env)

		longer := withStorage("1s", "0s", "10m")
		env, err = bootstrap.NewRefreshEnvironment(longer, store)
		Expect(err).NotTo(HaveOccurred())
		for _, ended := range []struct {
			client fixtures.RefreshClient
			token  string
		}{{*absoluteClient, absoluteSession.Tokens.RefreshToken}, {*idleClient, idleSession.Tokens.RefreshToken}} {
			denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), ended.client, ended.token, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(denied)
		}
		stillBound := policyRoot(ctx, db, increaseSession.Tokens.RefreshToken)
		Expect(stillBound["inactivity_expires_at"]).To(Equal(before["inactivity_expires_at"]))
		Expect(policyTokenExpires(ctx, db, increaseSession.Tokens.RefreshToken)).To(BeTemporally("~", oldTokenExpiry, time.Millisecond))
		increased, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *increaseClient, increaseSession.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(increased.Status).To(Equal(http.StatusOK))
		after := policyRoot(ctx, db, increased.Tokens.RefreshToken)
		Expect(after["started_at"]).To(Equal(before["started_at"]))
		Expect(policyRootTime(after, "inactivity_expires_at")).To(BeTemporally(">", oldTokenExpiry.Add(7*time.Minute)))
		Expect(policyTokenExpires(ctx, db, increased.Tokens.RefreshToken)).To(BeTemporally(">", oldTokenExpiry.Add(7*time.Minute)))
		unrelatedClient, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		unrelatedSession, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *unrelatedClient, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		policyStopPG(env) // Quiesce the outgoing issuer before binary-only rollback; keep migration 036.
		env = nil

		var schema sql.NullString
		Expect(db.GetContext(ctx, &schema, "SELECT to_regclass('public.refresh_sessions')::text")).To(Succeed())
		Expect(schema.Valid).To(BeTrue())
		old := launchPolicyBroker(policyOldBinary(), policyConfigPath(policyConfigYAML("30s", "0s", "10m", config.Storage.Postgres.ConnectionURL)), nil)
		Expect(helpers.ProvisionSigningKey(old.AdminURL)).To(Succeed())
		for _, ended := range []struct {
			client fixtures.RefreshClient
			token  string
		}{{*absoluteClient, absoluteSession.Tokens.RefreshToken}, {*idleClient, idleSession.Tokens.RefreshToken}} {
			denied, err := helpers.RotateRefreshSession(ctx, old.EnduserURL, ended.client, ended.token, "")
			Expect(err).NotTo(HaveOccurred())
			policyAssertInvalidGrant(denied)
		}
		unrelated, err := helpers.RotateRefreshSession(ctx, old.EnduserURL, *unrelatedClient, unrelatedSession.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(unrelated.Status).To(Equal(http.StatusOK), "rollback must preserve unrelated eligible sessions")
		Expect(db.GetContext(ctx, &schema, "SELECT to_regclass('public.refresh_sessions')::text")).To(Succeed())
		Expect(schema.Valid).To(BeTrue())
		old.Close()
	})

	// US6-S7 from specs/049-fix-refresh-consent/spec.md.
	It("US6-S7 fences old-first descendants and makes a new-first rotation defeat the old conditional writer", func() {
		var err error
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		client, err := env.AddClient(ctx, policyPrincipal(), false)
		Expect(err).NotTo(HaveOccurred())
		first, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rootBefore, rootErr := helpers.ReadRefreshRoot(ctx, db, first.Tokens.RefreshToken)
		old := launchPolicyBroker(policyOldBinary(), policyConfigPath(policyConfigYAML("30s", "0s", "10m", config.Storage.Postgres.ConnectionURL)), nil)
		Expect(helpers.ProvisionSigningKey(old.AdminURL)).To(Succeed())
		oldFirst, err := helpers.RotateRefreshSession(ctx, old.EnduserURL, *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(oldFirst.Status).To(Equal(http.StatusOK), "old binary must consume the anchored mirror through its own legacy writer")
		unsupported, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, oldFirst.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(unsupported)
		Expect(rootErr).NotTo(HaveOccurred(), "anchored new-issuer root must survive old-only mirror consumption")
		policyAssertRootUnchanged(rootBefore, policyRoot(ctx, db, first.Tokens.RefreshToken))
		original, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		policyAssertInvalidGrant(original)
		policyAssertRootUnchanged(rootBefore, policyRoot(ctx, db, first.Tokens.RefreshToken))
		var descendants int
		Expect(db.GetContext(ctx, &descendants, "SELECT COUNT(*) FROM refresh_tokens WHERE session_id = $1", rootBefore["id"])).To(Succeed())
		Expect(descendants).To(Equal(1), "old-only successor must not be adopted into anchored lineage")

		second, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		newFirst, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, second.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(newFirst.Status).To(Equal(http.StatusOK))
		oldTx, err := helpers.LockLegacyRefresh(ctx, db, second.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = oldTx.Rollback() }()
		result, err := oldTx.ExecContext(ctx,
			"UPDATE refresh_token_sessions SET used_at = clock_timestamp() WHERE signature = $1 AND used_at IS NULL",
			helpers.RefreshSignature(second.Tokens.RefreshToken))
		Expect(err).NotTo(HaveOccurred())
		updated, err := result.RowsAffected()
		Expect(err).NotTo(HaveOccurred())
		Expect(updated).To(BeZero(), "new-first commit must defeat the old writer's conditional MarkUsed")
		Expect(oldTx.Commit()).To(Succeed())
		continued, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, newFirst.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(continued.Status).To(Equal(http.StatusOK), "a failed legacy writer cannot fork or revoke the winner")
		newRoot := policyRoot(ctx, db, continued.Tokens.RefreshToken)
		Expect(newRoot["previous_signature"]).To(Equal(helpers.RefreshSignature(newFirst.Tokens.RefreshToken)))
		Expect(db.GetContext(ctx, &descendants, "SELECT COUNT(*) FROM refresh_tokens WHERE session_id = $1", newRoot["id"])).To(Succeed())
		Expect(descendants).To(Equal(3), "the new-first lineage has exactly initial, first, and second tokens")
		old.Close()
	})
})
