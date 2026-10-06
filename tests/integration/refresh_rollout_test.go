//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := bootstrap.TerminateSharedPostgres(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "terminating shared PostgreSQL:", err)
		code = 1
	}
	os.Exit(code)
}

func newRolloutStorage(t *testing.T) (*storageadapter.Adapter, *sqlx.DB) {
	t.Helper()
	shared := bootstrap.RequireSharedPostgres(t)
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	_, connStr, drop := shared.SetupDatabaseFromTemplate(t, "refresh_rollout", func(t *testing.T, name string) {
		runner, migrationErr := migrate.New("file://"+filepath.Join(root, "migrations"), shared.ConnectionString(name))
		require.NoError(t, migrationErr)
		defer func() { _, _ = runner.Close() }()
		migrationErr = runner.Up()
		require.True(t, migrationErr == nil || migrationErr == migrate.ErrNoChange, "migrating refresh rollout database: %v", migrationErr)
	})
	store, err := storageadapter.NewAdapter(&ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: connStr}})
	require.NoError(t, err)
	db, err := sqlx.Connect("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, store.Close(context.Background()))
		require.NoError(t, db.Close())
		drop()
	})
	return store, db
}

func createRolloutRoot(t *testing.T, store *storageadapter.Adapter, db *sqlx.DB, startedAt time.Time) *storage.RefreshSession {
	t.Helper()
	ctx := context.Background()
	permissionSetID := id.NewPermissionSetID()
	_, err := db.ExecContext(ctx, `INSERT INTO permission_sets (id, name, description, created_at, updated_at)
		VALUES ($1, $2, 'rollout maintenance', NOW(), NOW())`, permissionSetID, "rollout-"+permissionSetID.String())
	require.NoError(t, err)
	clientID := id.NewClientID("rollout-" + id.NewAgentID().String())
	agent := &storage.Agent{
		ID: id.NewAgentID(), ClientID: &clientID, DisplayName: "Rollout agent", Description: "Refresh maintenance",
		CreatedAt: startedAt, UpdatedAt: startedAt,
		PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: permissionSetID, RequirementType: storage.RequirementTypeOptional}},
	}
	require.NoError(t, store.Agents().Create(ctx, agent))
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: id.Principal("rollout@example.test"), CreatedAt: startedAt, UpdatedAt: startedAt}
	require.NoError(t, store.UserGrants().Create(ctx, grant))
	rootID := id.NewRefreshSessionID()
	sig := refreshContractSignature(rootID.String())
	tokenExpiry := startedAt.Add(time.Hour)
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grant.ID, OriginalTokenSignature: sig,
		AgentID: agent.ID, Principal: grant.Principal, ClientID: clientID, Scope: "offline_access read",
		StartedAt: startedAt, LastFreshAt: startedAt, InactivityExpiresAt: tokenExpiry,
		RetainUntil: tokenExpiry, BranchKeyID: "refresh_" + rootID.String() + "_branch_key", CurrentSignature: sig,
	}
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, agent.ID, func(scope context.Context, _ time.Time) error {
		if err := store.RefreshSessions().Create(scope, root); err != nil {
			return err
		}
		return store.RefreshTokens().Create(scope, &storage.RefreshToken{
			Signature: sig, SessionID: rootID, IssuedAt: startedAt, ExpiresAt: tokenExpiry,
		})
	}))
	return root
}

func rolloutCleanup(store *storageadapter.Adapter, policy storage.RefreshSessionPolicy) *oauth2server.SessionCleanup {
	return oauth2server.NewSessionCleanup(store.AuthorizationCodes(), store.PKCESessions(), oauth2server.RefreshSessionDependencies{
		Sessions: store.RefreshSessions(), Tokens: store.RefreshTokens(), Revocations: store.RefreshRevocations(),
		Coordinator: store.AuthorizationCoordinator(), Clock: store.AuthorizationClock(), Policy: policy,
	}, store.RefreshMaintenance(), store.RefreshMaintenanceControl(), slog.Default())
}

func TestRefreshRollout_StartupExpiryRollsBackOnLegacyMirrorFailure(t *testing.T) {
	store, db := newRolloutStorage(t)
	ctx := context.Background()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	startedAt := now.UTC().Truncate(time.Second).Add(-2 * time.Minute)
	root := createRolloutRoot(t, store, db, startedAt)
	other := createRolloutRoot(t, store, db, now.UTC().Truncate(time.Second))
	successor := refreshContractSignature("rollout-successor-" + root.ID.String())
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return rotateContractRoot(scope, store.RefreshSessions(), store.RefreshTokens(), root.ID, root.CurrentSignature, successor, at)
	}))
	before, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.NotEmpty(t, before.RetryCiphertext)

	// Old writers may have created descendants with no new-table anchor. Both
	// the anchored current row and this matching old-only row must be invalidated.
	oldDescendant := refreshContractSignature("old-only-descendant-" + root.ID.String())
	lateExpiry := before.RetainUntil.Add(10 * time.Minute)
	_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)`, oldDescendant, root.ID.String(), root.AgentID, root.ClientID, root.Principal, lateExpiry)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE FUNCTION reject_rollout_mirror_write() RETURNS TRIGGER LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'simulated mirror update failure'; END; $$;
		CREATE TRIGGER reject_rollout_mirror_write BEFORE UPDATE OF used_at ON refresh_token_sessions
		FOR EACH ROW WHEN (OLD.used_at IS NULL) EXECUTE FUNCTION reject_rollout_mirror_write();`)
	require.NoError(t, err)
	cleanup := rolloutCleanup(store, storage.RefreshSessionPolicy{ReuseInterval: 30 * time.Second, AbsoluteLifetime: time.Minute, InactivityLifetime: 2 * time.Hour})
	require.Error(t, cleanup.Reconcile(ctx), "a failed mirror update must block startup")
	require.Error(t, cleanup.HealthCheck(ctx))
	unchanged, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.Nil(t, unchanged.TerminalReason)
	require.Nil(t, unchanged.ExpiredAt)
	require.Equal(t, before.RetryCiphertext, unchanged.RetryCiphertext)
	require.Equal(t, before.RetainUntil, unchanged.RetainUntil)
	for _, signature := range []string{successor, oldDescendant} {
		var usedAt *time.Time
		require.NoError(t, db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.Nil(t, usedAt, "rolled-back expiry cannot partially consume an old-binary mirror")
	}
	var receiptCount int
	require.NoError(t, db.GetContext(ctx, &receiptCount, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1`, root.ID))
	require.Zero(t, receiptCount)

	_, err = db.ExecContext(ctx, `DROP TRIGGER reject_rollout_mirror_write ON refresh_token_sessions; DROP FUNCTION reject_rollout_mirror_write()`)
	require.NoError(t, err)
	require.NoError(t, cleanup.Reconcile(ctx))
	require.NoError(t, cleanup.HealthCheck(ctx))
	expired, err := store.RefreshSessions().FindByID(ctx, root.ID)
	require.NoError(t, err)
	require.NotNil(t, expired.TerminalReason, "startup must expire the entire native and legacy lineage")
	require.Equal(t, storage.RefreshReasonAbsoluteExpiry, *expired.TerminalReason)
	require.NotNil(t, expired.ExpiredAt)
	require.Empty(t, expired.RetryCiphertext)
	require.Equal(t, lateExpiry, expired.RetainUntil)
	for _, signature := range []string{successor, oldDescendant} {
		var usedAt *time.Time
		require.NoError(t, db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
		require.NotNil(t, usedAt, "matching legacy descendant cannot survive expiry")
		require.Equal(t, *expired.ExpiredAt, usedAt.UTC(), "terminal expiry must invalidate every matching old-binary lineage")
	}
	require.NoError(t, db.GetContext(ctx, &receiptCount, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'absolute_expiry'`, root.ID))
	require.Equal(t, 1, receiptCount)
	stillActive, err := store.RefreshSessions().FindByID(ctx, other.ID)
	require.NoError(t, err)
	require.Nil(t, stillActive.TerminalReason, "unrelated sessions remain usable")
	var otherMirrorUsed *time.Time
	require.NoError(t, db.GetContext(ctx, &otherMirrorUsed, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, other.CurrentSignature))
	require.Nil(t, otherMirrorUsed)
}
