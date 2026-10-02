//go:build integration

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
	"github.com/lestrrat-go/jwx/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func consentRootTimestamp(root map[string]any, field string) time.Time {
	value, ok := root[field].(string)
	ExpectWithOffset(1, ok).To(BeTrue(), "refresh root must persist "+field)
	parsed, err := time.Parse(time.RFC3339Nano, value)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return parsed
}

func submitConsentGrant(env *bootstrap.RefreshEnvironment, client *fixtures.RefreshClient, permissionSets map[string][]string, validUntil time.Time) (int, map[string]any, error) {
	body, err := json.Marshal(map[string]any{
		"granted_permission_sets": permissionSets,
		"valid_until":             validUntil.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return 0, nil, err
	}
	response, err := env.Enduser.AuthenticatedPOST(
		"/api/consent/agents/"+client.Agent.ID.String()+"/grants",
		client.Principal.String(), "application/json", bytes.NewReader(body),
	)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = response.Body.Close() }()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return 0, nil, err
	}
	return response.StatusCode, result, nil
}

var _ = Describe("Consent-Bound Refresh Sessions / US1 / PostgreSQL", Label("integration"), func() {
	var (
		ctx     context.Context
		env     *bootstrap.RefreshEnvironment
		db      *sqlx.DB
		cleanup func()
		client  *fixtures.RefreshClient
	)

	BeforeEach(func() {
		ctx = context.Background()
		config, err := fixtures.RefreshConfig("", "", "")
		Expect(err).NotTo(HaveOccurred())
		store, database, closeDatabase, err := bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, config)
		Expect(err).NotTo(HaveOccurred())
		db, cleanup = database, closeDatabase
		env, err = bootstrap.NewRefreshEnvironment(config, store)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, env.Storage)).To(Succeed())
		client, err = env.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if env != nil {
			env.Stop()
		}
		if cleanup != nil {
			cleanup()
		}
	})

	// US1-S1 from specs/049-fix-refresh-consent/spec.md: a narrowed response does not narrow the immutable first-issued session.
	It("US1-S1 rotates for the bound client without changing the original evidence or scope ceiling", func() {
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read write offline_access")
		Expect(err).NotTo(HaveOccurred())
		initialRoot, err := helpers.ReadRefreshRoot(ctx, db, issued.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred(), "first issuance must persist the original authorization evidence")
		Expect(initialRoot["original_grant_id"]).To(Equal(client.Grant.ID.String()))
		Expect(initialRoot["original_token_signature"]).To(Equal(helpers.RefreshSignature(issued.Tokens.RefreshToken)))
		Expect(initialRoot["current_signature"]).To(Equal(helpers.RefreshSignature(issued.Tokens.RefreshToken)))
		Expect(initialRoot["agent_id"]).To(Equal(client.Agent.ID.String()))
		Expect(initialRoot["principal"]).To(Equal(client.Principal.String()))
		Expect(initialRoot["client_id"]).To(Equal(client.Agent.ID.String()))
		Expect(strings.Fields(initialRoot["scope"].(string))).To(ConsistOf("read", "write", "offline_access"))
		started := consentRootTimestamp(initialRoot, "started_at")
		Expect(consentRootTimestamp(initialRoot, "last_fresh_at")).To(Equal(started))
		initialInactivity := consentRootTimestamp(initialRoot, "inactivity_expires_at")
		var firstIssued time.Time
		Expect(db.GetContext(ctx, &firstIssued, `SELECT issued_at FROM refresh_tokens WHERE signature = $1`,
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		Expect(firstIssued).To(BeTemporally("~", started, time.Microsecond))

		narrowed, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		Expect(narrowed.Status).To(Equal(http.StatusOK))
		Expect(narrowed.Tokens.RefreshToken).NotTo(BeEmpty())
		Expect(narrowed.Tokens.RefreshToken != issued.Tokens.RefreshToken).To(BeTrue(), "narrowed rotation must replace the refresh token")
		narrowedClaims := verifyPublishedConsentJWT(env.Enduser, narrowed.Tokens.AccessToken)
		scope, err := jwt.Get[string](narrowedClaims, "scope")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(scope)).To(ConsistOf("read", "offline_access"))
		principal, ok := narrowedClaims.Subject()
		Expect(ok).To(BeTrue())
		Expect(principal).To(Equal(client.Principal.String()))
		agentID, err := jwt.Get[string](narrowedClaims, "agent_id")
		Expect(err).NotTo(HaveOccurred())
		Expect(agentID).To(Equal(client.Agent.ID.String()))

		rootAfterNarrowing, err := helpers.ReadRefreshRoot(ctx, db, narrowed.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(rootAfterNarrowing["id"]).To(Equal(initialRoot["id"]))
		Expect(rootAfterNarrowing["original_token_signature"]).To(Equal(initialRoot["original_token_signature"]))
		Expect(rootAfterNarrowing["original_grant_id"]).To(Equal(initialRoot["original_grant_id"]))
		Expect(rootAfterNarrowing["started_at"]).To(Equal(initialRoot["started_at"]))
		Expect(rootAfterNarrowing["scope"]).To(Equal(initialRoot["scope"]))
		Expect(rootAfterNarrowing["previous_signature"]).To(Equal(helpers.RefreshSignature(issued.Tokens.RefreshToken)))
		Expect(rootAfterNarrowing["current_signature"]).To(Equal(helpers.RefreshSignature(narrowed.Tokens.RefreshToken)))
		Expect(consentRootTimestamp(rootAfterNarrowing, "last_fresh_at")).To(BeTemporally(">", started))
		Expect(consentRootTimestamp(rootAfterNarrowing, "inactivity_expires_at")).To(BeTemporally(">", initialInactivity))

		restored, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, narrowed.Tokens.RefreshToken, "read write offline_access")
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.Status).To(Equal(http.StatusOK), "a narrower response must not lower the session ceiling")
		Expect(restored.Tokens.RefreshToken).NotTo(BeEmpty())
		restoredClaims := verifyPublishedConsentJWT(env.Enduser, restored.Tokens.AccessToken)
		fullScope, err := jwt.Get[string](restoredClaims, "scope")
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(fullScope)).To(ConsistOf("read", "write", "offline_access"))
		finalRoot, err := helpers.ReadRefreshRoot(ctx, db, restored.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		for _, field := range []string{"id", "original_grant_id", "original_token_signature", "started_at", "scope", "principal", "agent_id", "client_id"} {
			Expect(finalRoot[field]).To(Equal(initialRoot[field]), fmt.Sprintf("%s is immutable", field))
		}
		Expect(consentRootTimestamp(finalRoot, "last_fresh_at")).To(BeTemporally(">", consentRootTimestamp(rootAfterNarrowing, "last_fresh_at")))
	})

	// US1-S5 from specs/049-fix-refresh-consent/spec.md: a broken real grant relation is not a missing grant.
	It("US1-S5 preserves an unconsumed token during unavailable consent and recovers afterward", func() {
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		signature := helpers.RefreshSignature(issued.Tokens.RefreshToken)
		var unused bool
		Expect(db.GetContext(ctx, &unused, `SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, signature)).To(Succeed())
		Expect(unused).To(BeTrue(), "the presented token must be unused before the fault")

		_, err = db.ExecContext(ctx, `ALTER TABLE user_grants RENAME TO user_grants_unavailable_us1_s5`)
		Expect(err).NotTo(HaveOccurred())
		renamed := true
		defer func() {
			if renamed {
				_, restoreErr := db.ExecContext(context.Background(), `ALTER TABLE user_grants_unavailable_us1_s5 RENAME TO user_grants`)
				Expect(restoreErr).NotTo(HaveOccurred(), "restore the isolated grant relation")
			}
		}()

		unavailable, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(unavailable.Status).To(BeNumerically(">=", http.StatusInternalServerError))
		Expect(unavailable.Status).To(BeNumerically("<", 600))
		Expect(unavailable.Error).To(Equal("server_error"), "unknown consent state is not invalid_grant")
		for _, field := range []string{"access_token", "refresh_token", "token_type"} {
			_, present := unavailable.Body[field]
			Expect(present).To(BeFalse(), field+" must be absent on server error")
		}
		Expect(db.GetContext(ctx, &unused, `SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, signature)).To(Succeed())
		Expect(unused).To(BeTrue(), "the unavailable-grant error must not consume the legacy mirror")
		root, err := helpers.ReadRefreshRoot(ctx, db, issued.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(root["current_signature"]).To(Equal(signature))
		Expect(root["previous_signature"]).To(BeNil())
		Expect(root["revoked_at"]).To(BeNil())
		Expect(root["last_fresh_at"]).To(Equal(root["started_at"]))
		var anchoredUnused bool
		Expect(db.GetContext(ctx, &anchoredUnused, `SELECT used_at IS NULL FROM refresh_tokens WHERE signature = $1`, signature)).To(Succeed())
		Expect(anchoredUnused).To(BeTrue(), "the anchored token must also remain unused")

		_, err = db.ExecContext(ctx, `ALTER TABLE user_grants_unavailable_us1_s5 RENAME TO user_grants`)
		Expect(err).NotTo(HaveOccurred())
		renamed = false
		recovered, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(recovered.Status).To(Equal(http.StatusOK), "the identical previously presented token must work after recovery")
		Expect(recovered.Tokens.RefreshToken).NotTo(BeEmpty())
		Expect(recovered.Tokens.RefreshToken != issued.Tokens.RefreshToken).To(BeTrue(), "recovered rotation must replace the original token")
	})

	// US1-S8 from specs/049-fix-refresh-consent/spec.md: renewal after expiry cannot resurrect the old session.
	It("US1-S8 invalidates pre-expiry tokens on grant renewal but permits a new authorization", func() {
		initialGrant := client.Grant.Copy()
		firstDeadline := time.Now().UTC().Add(time.Hour)
		initialGrant.ValidUntil = &firstDeadline
		Expect(env.Storage.UserGrants().Update(ctx, initialGrant)).To(Succeed())
		Expect(env.Storage.UserSessions().Create(ctx, fixtures.SessionForService(client.Principal.String(), fixtures.PlaceholderServiceID.String()))).To(Succeed())
		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK))
		Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())

		sharedTime, err := helpers.RefreshSharedTime(ctx, db)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.ExecContext(ctx, `UPDATE user_grants SET valid_until = (clock_timestamp() AT TIME ZONE 'UTC') - INTERVAL '1 second' WHERE id = $1`, initialGrant.ID)
		Expect(err).NotTo(HaveOccurred())
		var expired bool
		Expect(db.GetContext(ctx, &expired, `SELECT valid_until AT TIME ZONE 'UTC' <= clock_timestamp() FROM user_grants WHERE id = $1`, initialGrant.ID)).To(Succeed())
		Expect(expired).To(BeTrue(), "the old grant must actually have passed its deadline")

		status, renewal, err := submitConsentGrant(env, client, map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
		}, sharedTime.Add(2*time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusCreated), "the user must successfully renew the expired grant")
		grantBody, ok := renewal["data"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(grantBody["id"]).To(Equal(initialGrant.ID.String()), "upsert retains the same grant ID, so ID mismatch cannot explain old-token denial")

		denied, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(denied, http.StatusBadRequest, "invalid_grant")
		original, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		expectConsentRefreshError(original, http.StatusBadRequest, "invalid_grant")

		root, err := helpers.ReadRefreshRoot(ctx, db, rotated.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(root["terminal_reason"]).To(Equal("expired_grant_renewal"))
		Expect(root["revoked_at"]).NotTo(BeNil())
		fresh, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred(), "fresh authorization must create a new root after renewal")
		Expect(fresh.Tokens.RefreshToken != rotated.Tokens.RefreshToken).To(BeTrue(), "fresh authorization must issue a different token")
		freshRoot, err := helpers.ReadRefreshRoot(ctx, db, fresh.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(freshRoot["id"]).NotTo(Equal(root["id"]), "new authorization must establish a different session")
		freshResult, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, fresh.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(freshResult.Status).To(Equal(http.StatusOK))
	})

	// US1-S9 from specs/049-fix-refresh-consent/spec.md: edits to an active grant keep its session's original clock and authority.
	It("US1-S9 retains session origin and clocks through active permission edits and an extension", func() {
		updatedAgent := client.Agent.Copy()
		updatedAgent.PermissionSets = append(updatedAgent.PermissionSets, storage.AgentPermissionSetEntry{
			PermissionSetID: fixtures.SecondaryPlaceholderPermissionSetID,
			RequirementType: storage.RequirementTypeOptional,
		})
		Expect(env.Storage.Agents().Update(ctx, updatedAgent)).To(Succeed())
		grant := client.Grant.Copy()
		originalDeadline := time.Now().UTC().Add(time.Hour)
		grant.ValidUntil = &originalDeadline
		Expect(env.Storage.UserGrants().Update(ctx, grant)).To(Succeed())
		for _, serviceID := range []string{fixtures.PlaceholderServiceID.String(), fixtures.SecondaryPlaceholderServiceID.String()} {
			Expect(env.Storage.UserSessions().Create(ctx, fixtures.SessionForService(client.Principal.String(), serviceID))).To(Succeed())
		}

		issued, err := helpers.AuthorizeRefreshSession(ctx, env.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
		status, changed, err := submitConsentGrant(env, client, map[string][]string{
			fixtures.PlaceholderPermissionSetID.String():          {fixtures.PlaceholderServiceID.String()},
			fixtures.SecondaryPlaceholderPermissionSetID.String(): {fixtures.SecondaryPlaceholderServiceID.String()},
		}, originalDeadline.Add(time.Hour))
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusCreated))
		grantBody, ok := changed["data"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(grantBody["id"]).To(Equal(grant.ID.String()))
		permissionSets, ok := grantBody["granted_permission_sets"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(permissionSets).To(HaveKey(fixtures.SecondaryPlaceholderPermissionSetID.String()))
		var extended bool
		Expect(db.GetContext(ctx, &extended, `SELECT valid_until > $1 FROM user_grants WHERE id = $2`, originalDeadline, grant.ID)).To(Succeed())
		Expect(extended).To(BeTrue())

		rotated, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.Status).To(Equal(http.StatusOK), "current, still-active consent must allow refresh")
		Expect(rotated.Tokens.RefreshToken).NotTo(BeEmpty())
		root, err := helpers.ReadRefreshRoot(ctx, db, rotated.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(root["original_grant_id"]).To(Equal(grant.ID.String()))
		Expect(root["original_token_signature"]).To(Equal(helpers.RefreshSignature(issued.Tokens.RefreshToken)))
		Expect(root["current_signature"]).To(Equal(helpers.RefreshSignature(rotated.Tokens.RefreshToken)))
		var firstIssued time.Time
		Expect(db.GetContext(ctx, &firstIssued, `SELECT issued_at FROM refresh_tokens WHERE signature = $1`,
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		started := consentRootTimestamp(root, "started_at")
		Expect(started).To(BeTemporally("~", firstIssued, time.Microsecond), "grant extension must not restart the session")
		Expect(consentRootTimestamp(root, "last_fresh_at")).To(BeTemporally(">", started))
		var initialTokenExpiry time.Time
		Expect(db.GetContext(ctx, &initialTokenExpiry, `SELECT expires_at FROM refresh_tokens WHERE signature = $1`,
			helpers.RefreshSignature(issued.Tokens.RefreshToken))).To(Succeed())
		Expect(consentRootTimestamp(root, "inactivity_expires_at")).To(BeTemporally(">", initialTokenExpiry), "a fresh rotation advances inactivity from the original activity interval")
		Expect(root["revoked_at"]).To(BeNil())
		Expect(root["expired_at"]).To(BeNil())

		again, err := helpers.RotateRefreshSession(ctx, env.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
		Expect(err).NotTo(HaveOccurred())
		Expect(again.Status).To(Equal(http.StatusOK), "the edited grant remains authorized for later refreshes")
		after, err := helpers.ReadRefreshRoot(ctx, db, again.Tokens.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(after["started_at"]).To(Equal(root["started_at"]))
		Expect(after["original_grant_id"]).To(Equal(root["original_grant_id"]))
		Expect(after["original_token_signature"]).To(Equal(root["original_token_signature"]))
		Expect(consentRootTimestamp(after, "last_fresh_at")).To(BeTemporally(">", consentRootTimestamp(root, "last_fresh_at")))
	})
})
