//go:build integration
// +build integration

package migrations_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	"github.com/stretchr/testify/require"
)

const (
	migrationAgentID              = "51000000-0000-0000-0000-000000000001"
	migrationOtherAgentID         = "51000000-0000-0000-0000-000000000002"
	migrationGrantID              = "52000000-0000-0000-0000-000000000001"
	migrationOtherGrantID         = "52000000-0000-0000-0000-000000000002"
	migrationNeighborGrantID      = "52000000-0000-0000-0000-000000000003"
	migrationRootID               = "53000000-0000-0000-0000-000000000001"
	migrationExpiredID            = "53000000-0000-0000-0000-000000000002"
	migrationLiveID               = "53000000-0000-0000-0000-000000000003"
	migrationOtherRootID          = "53000000-0000-0000-0000-000000000004"
	migrationLegacyRequestID      = "53000000-0000-0000-0000-000000000091"
	migrationOtherLegacyRequestID = "53000000-0000-0000-0000-000000000092"
)

// Clone a database already at 035 so each 036 lifecycle/fault test starts with
// the same pre-upgrade schema without replaying all migrations per subtest.
func newRefreshMigrationFramework(t *testing.T) *MigrationTestFramework {
	t.Helper()
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	require.NoError(t, err)

	pg := bootstrap.RequireSharedPostgres(t)
	_, connStr, cleanup := pg.SetupDatabaseFromTemplate(t, "refresh_sessions_at_035", func(t *testing.T, templateName string) {
		m, err := migrate.New("file://"+migrationsDir, pg.ConnectionString(templateName))
		require.NoError(t, err)
		defer func() {
			sourceErr, databaseErr := m.Close()
			require.NoError(t, sourceErr)
			require.NoError(t, databaseErr)
		}()
		require.NoError(t, m.Migrate(35))
	})
	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	f := &MigrationTestFramework{db: db, connStr: connStr, migrationsDir: migrationsDir, cleanup: cleanup}
	t.Cleanup(func() { f.Cleanup(t) })
	require.NoError(t, db.PingContext(context.Background()))
	return f
}

func migrationSignature(label string) string {
	sum := sha256.Sum256([]byte(label))
	return hex.EncodeToString(sum[:])
}

func migrationExec(t *testing.T, f *MigrationTestFramework, statement string, args ...any) {
	t.Helper()
	_, err := f.db.ExecContext(context.Background(), statement, args...)
	require.NoError(t, err)
}

func migrationBool(t *testing.T, f *MigrationTestFramework, query string, args ...any) bool {
	t.Helper()
	var result bool
	require.NoError(t, f.db.QueryRowContext(context.Background(), query, args...).Scan(&result))
	return result
}

func migrationRows(t *testing.T, f *MigrationTestFramework, query string, args ...any) int64 {
	t.Helper()
	var result int64
	require.NoError(t, f.db.QueryRowContext(context.Background(), query, args...).Scan(&result))
	return result
}

func seedRefreshMigrationOwners(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	migrationExec(t, f, `
		INSERT INTO agents (id, client_id, display_name, description) VALUES
		($1, 'migration-owner', 'owner agent', 'migration test'),
		($2, 'migration-other', 'other agent', 'migration test')`, migrationAgentID, migrationOtherAgentID)
	migrationExec(t, f, `
		INSERT INTO user_grants (id, principal, agent_id, granted_permission_sets) VALUES
		($1, 'owner@example.test', $2, '[]'),
		($3, 'other@example.test', $4, '[]')`,
		migrationGrantID, migrationAgentID, migrationOtherGrantID, migrationOtherAgentID)
	migrationExec(t, f, `
		INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('54000000-0000-0000-0000-000000000001', 'unrelated service', 'thirdparty-client',
		decode('deadbeef', 'hex'), 'https://issuer.example.test', '[]')`)
	migrationExec(t, f, `
		INSERT INTO user_sessions (id, principal, service_id, encrypted_access_token)
		VALUES ('55000000-0000-0000-0000-000000000001', 'owner@example.test',
		'54000000-0000-0000-0000-000000000001', decode('deadbeef', 'hex'))`)
}

func seedRefreshLegacyRows(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	migrationExec(t, f, `
		INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at) VALUES
		($1, $2, $3, 'migration-owner', 'owner@example.test', 'openid', TIMESTAMPTZ '2026-11-01 00:00:00+00'),
		($4, $5, $6, 'migration-other', 'other@example.test', 'openid', TIMESTAMPTZ '2026-11-01 00:00:00+00')`,
		migrationSignature("old owner"), migrationLegacyRequestID, migrationAgentID,
		migrationSignature("old other"), migrationOtherLegacyRequestID, migrationOtherAgentID)
}

func insertMigrationRefreshRoot(t *testing.T, f *MigrationTestFramework, rootID, signature, agentID, grantID, principal, clientID string) {
	t.Helper()
	tx, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO refresh_sessions
		(id, original_grant_id, original_token_signature, agent_id, principal, client_id,
		 scope, display_name, started_at, last_fresh_at, inactivity_expires_at, retain_until,
		 branch_key_id, current_signature, retry_count)
		VALUES ($1, $2, $3, $4, $5, $6, 'openid profile', '',
		        TIMESTAMPTZ '2026-09-01 00:00:00+00', TIMESTAMPTZ '2026-09-01 00:00:00+00',
		        TIMESTAMPTZ '2026-11-01 00:00:00+00', TIMESTAMPTZ '2026-11-01 00:00:00+00',
		        'refresh_' || $1::uuid::text || '_branch_key', $3, 0)`,
		rootID, grantID, signature, agentID, principal, clientID)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO refresh_tokens (signature, session_id, issued_at, expires_at)
		VALUES ($1, $2, TIMESTAMPTZ '2026-09-01 00:00:00+00', TIMESTAMPTZ '2026-11-01 00:00:00+00')`,
		signature, rootID)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, created_at, expires_at, session_id)
		VALUES ($1, $2::uuid::text, $3, $4, $5, 'openid profile',
		TIMESTAMPTZ '2026-09-01 00:00:00+00', TIMESTAMPTZ '2026-11-01 00:00:00+00', $2::uuid)`,
		signature, rootID, agentID, clientID, principal)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
}

func TestMigration036LegacyCompatibility(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	seedRefreshLegacyRows(t, f)
	require.NoError(t, f.Up(t, 36), "migration 036 must add the consent-bound refresh schema")

	for _, table := range []string{"refresh_sessions", "refresh_tokens", "refresh_revocation_receipts"} {
		exists, err := f.TableExists(t, table)
		require.NoError(t, err)
		require.True(t, exists, "%s must exist after upgrade", table)
	}
	for _, column := range []string{"session_id", "predecessor_signature"} {
		exists, err := f.ColumnExists(t, "refresh_token_sessions", column)
		require.NoError(t, err)
		require.True(t, exists, "the old writer must retain its original insert shape")
	}

	// The old writer supplies only the original seven columns both before and after 036.
	migrationExec(t, f, `
		INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at)
		VALUES ($1, $2, $3, 'migration-owner', 'owner@example.test', 'openid',
		        TIMESTAMPTZ '2026-11-01 00:00:00+00')`,
		migrationSignature("post-up old writer"), migrationLegacyRequestID, migrationAgentID)
	require.Equal(t, int64(2), migrationRows(t, f, `
		SELECT count(*) FROM refresh_token_sessions
		WHERE signature IN ($1, $2) AND session_id IS NULL AND predecessor_signature IS NULL`,
		migrationSignature("old owner"), migrationSignature("post-up old writer")))
	result, err := f.db.ExecContext(context.Background(), `
		UPDATE refresh_token_sessions SET used_at = TIMESTAMPTZ '2026-09-02 00:00:00+00'
		WHERE signature = $1 AND used_at IS NULL`, migrationSignature("post-up old writer"))
	require.NoError(t, err)
	changed, err := result.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), changed, "the old writer's conditional MarkUsed remains usable")
	require.True(t, migrationBool(t, f, `
		SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("old owner")))
	requireUnrelatedMigrationData(t, f)
}

func TestMigration036RootTokenLineageAndConstraints(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	require.NoError(t, f.Up(t, 36))
	first := migrationSignature("owned first")
	next := migrationSignature("owned next")
	insertMigrationRefreshRoot(t, f, migrationRootID, first, migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")

	require.True(t, migrationBool(t, f, `
		SELECT r.original_grant_id = $1::uuid AND r.agent_id = $2::uuid
		   AND r.principal = 'owner@example.test' AND r.client_id = 'migration-owner'
		   AND r.scope = 'openid profile' AND r.started_at = first.issued_at
		   AND r.last_fresh_at = first.issued_at AND r.original_token_signature = first.signature
		   AND r.current_signature = first.signature AND first.session_id = r.id
		   AND first.expires_at > first.issued_at AND r.retain_until = first.expires_at
		   AND first.used_at IS NULL AND legacy.session_id = r.id
		   AND legacy.request_id = r.id::text AND legacy.predecessor_signature IS NULL
		FROM refresh_sessions r
		JOIN user_grants grant_origin ON grant_origin.id = r.original_grant_id
		   AND grant_origin.agent_id = r.agent_id AND grant_origin.principal = r.principal
		JOIN refresh_tokens first ON first.signature = r.original_token_signature
		JOIN refresh_token_sessions legacy ON legacy.signature = first.signature
		WHERE r.id = $3`, migrationGrantID, migrationAgentID, migrationRootID))

	for _, foreignKey := range []struct{ child, parent string }{
		{"refresh_sessions", "agents"},
		{"refresh_tokens", "refresh_sessions"},
		{"refresh_token_sessions", "refresh_sessions"},
	} {
		require.True(t, migrationBool(t, f, `
			SELECT EXISTS (SELECT 1 FROM pg_constraint
			WHERE conrelid = $1::regclass AND confrelid = $2::regclass
			AND contype = 'f' AND confdeltype = 'c')`, foreignKey.child, foreignKey.parent),
			"%s must cascade only with its owning %s", foreignKey.child, foreignKey.parent)
	}

	// Rotate without discarding the first signature or the original authorization.
	tx, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(context.Background(), `
		UPDATE refresh_tokens SET used_at = TIMESTAMPTZ '2026-09-01 00:00:20+00'
		WHERE signature = $1 AND used_at IS NULL`, first)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		UPDATE refresh_token_sessions SET used_at = TIMESTAMPTZ '2026-09-01 00:00:20+00'
		WHERE signature = $1 AND used_at IS NULL`, first)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO refresh_tokens (signature, session_id, issued_at, expires_at)
		VALUES ($1, $2, TIMESTAMPTZ '2026-09-01 00:00:20+00', TIMESTAMPTZ '2026-11-01 00:00:20+00')`, next, migrationRootID)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at,
		 session_id, predecessor_signature)
		VALUES ($1, $2::uuid::text, $3, 'migration-owner', 'owner@example.test', 'openid profile',
		        TIMESTAMPTZ '2026-11-01 00:00:20+00', $2::uuid, $4)`,
		next, migrationRootID, migrationAgentID, first)
	require.NoError(t, err)
	_, err = tx.ExecContext(context.Background(), `
		UPDATE refresh_sessions SET current_signature = $2, previous_signature = $3,
		 previous_consumed_at = TIMESTAMPTZ '2026-09-01 00:00:20+00',
		 reuse_until = TIMESTAMPTZ '2026-09-01 00:00:50+00',
		 original_requested_scope = '', original_request_context_fingerprint = $4,
		 retry_access_expires_at = TIMESTAMPTZ '2026-09-01 00:05:00+00',
		 retry_expires_at = TIMESTAMPTZ '2026-09-01 00:00:50+00',
		 last_fresh_at = TIMESTAMPTZ '2026-09-01 00:00:20+00',
		 inactivity_expires_at = TIMESTAMPTZ '2026-11-01 00:00:20+00',
		 retain_until = TIMESTAMPTZ '2026-11-01 00:00:20+00'
		 WHERE id = $1`, migrationRootID, next, first, migrationSignature("redacted request context"))
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	require.True(t, migrationBool(t, f, `
		SELECT r.original_token_signature = first.signature AND first.issued_at = r.started_at
		   AND first.used_at = r.previous_consumed_at AND r.previous_signature = first.signature
		   AND r.current_signature = next.signature AND next.used_at IS NULL
		   AND next.session_id = r.id AND next.issued_at = r.last_fresh_at
		   AND mirror.predecessor_signature = first.signature AND mirror.session_id = r.id
		   AND r.original_grant_id = $2::uuid AND r.scope = 'openid profile'
		   AND r.retain_until = next.expires_at AND r.retry_count = 0
		FROM refresh_sessions r
		JOIN refresh_tokens first ON first.signature = r.original_token_signature
		JOIN refresh_tokens next ON next.signature = r.current_signature
		JOIN refresh_token_sessions mirror ON mirror.signature = next.signature
		WHERE r.id = $1`, migrationRootID, migrationGrantID))

	for _, count := range []int{3, 0} {
		migrationExec(t, f, `UPDATE refresh_sessions SET retry_count = $2 WHERE id = $1`, migrationRootID, count)
		require.True(t, migrationBool(t, f, `SELECT retry_count = $2 FROM refresh_sessions WHERE id = $1`, migrationRootID, count))
	}
	for _, count := range []int{-1, 4} {
		_, err := f.db.ExecContext(context.Background(), `UPDATE refresh_sessions SET retry_count = $2 WHERE id = $1`, migrationRootID, count)
		require.Error(t, err, "retry_count must be restricted to 0..3 by PostgreSQL")
	}
	require.True(t, migrationBool(t, f, `SELECT retry_count = 0 FROM refresh_sessions WHERE id = $1`, migrationRootID))

	_, err = f.db.ExecContext(context.Background(), `
		INSERT INTO refresh_tokens (signature, session_id, issued_at, expires_at)
		VALUES ($1, $2, TIMESTAMPTZ '2026-09-01 00:00:40+00', TIMESTAMPTZ '2026-11-01 00:00:40+00')`,
		migrationSignature("second unused child"), migrationRootID)
	require.Error(t, err, "a second unused token would branch a refresh root")
	_, err = f.db.ExecContext(context.Background(), `
		INSERT INTO refresh_tokens (signature, session_id, issued_at, expires_at)
		VALUES ($1, $2, TIMESTAMPTZ '2026-09-01 00:00:40+00', TIMESTAMPTZ '2026-11-01 00:00:40+00')`,
		migrationSignature("orphan child"), "53000000-0000-0000-0000-000000000099")
	require.Error(t, err, "a token cannot claim a nonexistent root")
	_, err = f.db.ExecContext(context.Background(), `
		INSERT INTO refresh_token_sessions
		(signature, request_id, agent_id, client_id, principal, scope, expires_at, session_id, predecessor_signature)
		VALUES ($1, $2::uuid::text, $3, 'migration-owner', 'owner@example.test', 'openid',
		 TIMESTAMPTZ '2026-11-01 00:00:00+00', $2::uuid, 'not-a-sha256-digest')`,
		migrationSignature("bad predecessor"), migrationRootID, migrationAgentID)
	require.Error(t, err, "anchored predecessor signatures must be lowercase SHA-256 digests")
	require.Equal(t, int64(2), migrationRows(t, f, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, migrationRootID))
	for _, column := range []string{"revoked_at", "expired_at"} {
		_, err = f.db.ExecContext(context.Background(),
			"UPDATE refresh_sessions SET "+column+" = TIMESTAMPTZ '2026-09-01 00:00:40+00', terminal_reason = NULL WHERE id = $1", migrationRootID)
		require.Error(t, err, "terminal state must always retain its reason")
	}
	require.True(t, migrationBool(t, f, `SELECT revoked_at IS NULL AND expired_at IS NULL AND terminal_reason IS NULL FROM refresh_sessions WHERE id = $1`, migrationRootID))
}

func TestMigration036DownAndReapplyDoesNotRestoreAuthority(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	seedRefreshLegacyRows(t, f)
	require.NoError(t, f.Up(t, 36))
	for _, root := range []struct {
		id, label, agent, grant, principal, client string
	}{
		{migrationRootID, "revoked first", migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner"},
		{migrationExpiredID, "expired first", migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner"},
		{migrationLiveID, "active first", migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner"},
		{migrationOtherRootID, "other first", migrationOtherAgentID, migrationOtherGrantID, "other@example.test", "migration-other"},
	} {
		insertMigrationRefreshRoot(t, f, root.id, migrationSignature(root.label), root.agent, root.grant, root.principal, root.client)
	}
	migrationExec(t, f, `
		UPDATE refresh_sessions SET revoked_at = TIMESTAMPTZ '2026-09-02 00:00:00+00',
		terminal_reason = 'grant_deleted', retry_ciphertext = NULL WHERE id = $1`, migrationRootID)
	migrationExec(t, f, `
		UPDATE refresh_sessions SET expired_at = TIMESTAMPTZ '2026-09-02 00:00:00+00',
		terminal_reason = 'inactivity_expiry', retry_ciphertext = NULL WHERE id = $1`, migrationExpiredID)
	// Leave both terminal mirrors unused to prove down migration itself fences them.
	require.NoError(t, f.Down(t, 35))
	for _, label := range []string{"revoked first", "expired first", "active first", "other first"} {
		signature := migrationSignature(label)
		require.True(t, migrationBool(t, f, `
			SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`, signature),
			"rollback must never leave a formerly anchored lineage renewable: %s", label)
		result, err := f.db.ExecContext(context.Background(), `
			UPDATE refresh_token_sessions SET used_at = NOW() WHERE signature = $1 AND used_at IS NULL`, signature)
		require.NoError(t, err)
		changed, err := result.RowsAffected()
		require.NoError(t, err)
		require.Zero(t, changed, "the old writer cannot consume a revoked/terminal lineage")
	}
	for _, column := range []string{"session_id", "predecessor_signature"} {
		exists, err := f.ColumnExists(t, "refresh_token_sessions", column)
		require.NoError(t, err)
		require.False(t, exists)
	}
	for _, table := range []string{"refresh_sessions", "refresh_tokens"} {
		exists, err := f.TableExists(t, table)
		require.NoError(t, err)
		require.False(t, exists)
	}
	require.Equal(t, int64(1), migrationRows(t, f, `
		SELECT count(*) FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("old other")),
		"rollback keeps an unrelated old-writer record without assuming its authority is still safe")
	requireUnrelatedMigrationData(t, f)

	require.NoError(t, f.Up(t, 36))
	require.Equal(t, int64(0), migrationRows(t, f, `
		SELECT count(*) FROM refresh_sessions WHERE id IN ($1, $2, $3, $4)`,
		migrationRootID, migrationExpiredID, migrationLiveID, migrationOtherRootID),
		"reapplication cannot invent a first token or an original grant for legacy rows")
	for _, label := range []string{"revoked first", "expired first", "active first", "other first"} {
		require.True(t, migrationBool(t, f, `
			SELECT used_at IS NOT NULL AND session_id IS NULL
			FROM refresh_token_sessions WHERE signature = $1`, migrationSignature(label)))
	}
	requireUnrelatedMigrationData(t, f)
}

func TestMigration036GrantRevocationRetainsOriginAndOtherPrincipal(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	seedRefreshLegacyRows(t, f)
	require.NoError(t, f.Up(t, 36))
	migrationExec(t, f, `
		INSERT INTO user_grants (id, principal, agent_id, granted_permission_sets)
		VALUES ($1, 'neighbor@example.test', $2, '[]')`, migrationNeighborGrantID, migrationAgentID)
	insertMigrationRefreshRoot(t, f, migrationRootID, migrationSignature("owned first"), migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	insertMigrationRefreshRoot(t, f, migrationLiveID, migrationSignature("neighbor first"), migrationAgentID, migrationNeighborGrantID, "neighbor@example.test", "migration-owner")

	ctx := context.Background()
	tx, err := f.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO refresh_revocation_receipts
		(session_id, agent_id, principal, client_id, reason, "at", redacted_context)
		SELECT id, agent_id, principal, client_id, 'grant_deleted',
		TIMESTAMPTZ '2026-09-02 00:00:00+00', '{}'::jsonb
		FROM refresh_sessions WHERE id = $1`, migrationRootID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		UPDATE refresh_sessions SET revoked_at = TIMESTAMPTZ '2026-09-02 00:00:00+00',
		terminal_reason = 'grant_deleted', retry_ciphertext = NULL
		WHERE agent_id = $1 AND principal = 'owner@example.test'`, migrationAgentID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `
		UPDATE refresh_token_sessions SET used_at = TIMESTAMPTZ '2026-09-02 00:00:00+00'
		WHERE agent_id = $1 AND principal = 'owner@example.test' AND used_at IS NULL`, migrationAgentID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `DELETE FROM user_grants WHERE id = $1`, migrationGrantID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM user_grants WHERE id = $1`, migrationGrantID))
	require.True(t, migrationBool(t, f, `
		SELECT original_grant_id = $2::uuid AND revoked_at IS NOT NULL
		AND terminal_reason = 'grant_deleted' FROM refresh_sessions WHERE id = $1`, migrationRootID, migrationGrantID),
		"grant removal must retain immutable original-grant evidence and terminal root history")
	require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM refresh_tokens WHERE session_id = $1`, migrationRootID))
	require.True(t, migrationBool(t, f, `
		SELECT used_at IS NOT NULL FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("owned first")))
	require.True(t, migrationBool(t, f, `
		SELECT reason = 'grant_deleted' AND agent_id = $2::uuid
		FROM refresh_revocation_receipts WHERE session_id = $1`, migrationRootID, migrationAgentID))
	require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM user_grants WHERE id = $1`, migrationNeighborGrantID))
	require.True(t, migrationBool(t, f, `
		SELECT original_grant_id = $2::uuid AND revoked_at IS NULL AND expired_at IS NULL
		FROM refresh_sessions WHERE id = $1`, migrationLiveID, migrationNeighborGrantID))
	require.True(t, migrationBool(t, f, `
		SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("neighbor first")))
	requireUnrelatedMigrationDataAfterAgentDelete(t, f)
}

func TestMigration036ReceiptPrecedesAgentCascade(t *testing.T) {
	f := newRefreshMigrationFramework(t)
	seedRefreshMigrationOwners(t, f)
	seedRefreshLegacyRows(t, f)
	require.NoError(t, f.Up(t, 36))
	insertMigrationRefreshRoot(t, f, migrationRootID, migrationSignature("owned first"), migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	insertMigrationRefreshRoot(t, f, migrationLiveID, migrationSignature("owned second"), migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
	insertMigrationRefreshRoot(t, f, migrationOtherRootID, migrationSignature("other first"), migrationOtherAgentID, migrationOtherGrantID, "other@example.test", "migration-other")
	require.NoError(t, deleteMigrationAgentWithReceipt(f))

	// Agent and all its children can disappear only after the non-credential receipt
	// is durable independently of both the root and the deleted agent.
	require.True(t, migrationBool(t, f, `
		SELECT agent_id = $2::uuid AND principal = 'owner@example.test'
		AND client_id = 'migration-owner' AND reason = 'agent_deleted'
		AND "at" = TIMESTAMPTZ '2026-09-02 00:00:00+00'
		AND redacted_context = '{"request_id":"migration-agent-delete"}'::jsonb
		FROM refresh_revocation_receipts WHERE session_id = $1`, migrationRootID, migrationAgentID))
	require.Equal(t, int64(2), migrationRows(t, f, `
		SELECT count(*) FROM refresh_revocation_receipts
		WHERE session_id IN ($1, $2) AND reason = 'agent_deleted' AND agent_id = $3`,
		migrationRootID, migrationLiveID, migrationAgentID),
		"every removed root needs its own durable audit receipt")
	require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM agents WHERE id = $1`, migrationAgentID))
	require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM refresh_sessions WHERE id IN ($1, $2)`, migrationRootID, migrationLiveID))
	require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM refresh_tokens WHERE session_id IN ($1, $2)`, migrationRootID, migrationLiveID))
	require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM refresh_token_sessions WHERE agent_id = $1`, migrationAgentID))
	require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM refresh_sessions WHERE id = $1`, migrationOtherRootID))
	require.True(t, migrationBool(t, f, `SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("other first")))
	requireUnrelatedMigrationDataAfterAgentDelete(t, f)
	_, err := f.db.ExecContext(context.Background(), `
		INSERT INTO refresh_revocation_receipts
		(session_id, agent_id, principal, client_id, reason, "at", redacted_context)
		VALUES ($1, $2, 'owner@example.test', 'migration-owner', 'agent_deleted',
		TIMESTAMPTZ '2026-09-02 00:00:00+00', '{}'::jsonb)`, migrationRootID, migrationAgentID)
	require.Error(t, err, "a revocation reason for one root must have one immutable receipt")
}

func TestMigration036AgentDeletionFailuresRollback(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fault func(t *testing.T, f *MigrationTestFramework)
	}{
		{"receipt write", func(t *testing.T, f *MigrationTestFramework) {
			migrationExec(t, f, `ALTER TABLE refresh_revocation_receipts
				ADD CONSTRAINT reject_receipt_write CHECK (reason <> 'agent_deleted')`)
		}},
		{"root revocation", func(t *testing.T, f *MigrationTestFramework) {
			migrationExec(t, f, `ALTER TABLE refresh_sessions
				ADD CONSTRAINT reject_root_revocation CHECK (terminal_reason IS DISTINCT FROM 'agent_deleted')`)
		}},
		{"legacy invalidation", func(t *testing.T, f *MigrationTestFramework) {
			migrationExec(t, f, fmt.Sprintf(`ALTER TABLE refresh_token_sessions
				ADD CONSTRAINT reject_legacy_invalidation CHECK (signature <> '%s' OR used_at IS NULL)`,
				migrationSignature("owned first")))
		}},
		{"agent cascade", func(t *testing.T, f *MigrationTestFramework) {
			migrationExec(t, f, `CREATE TABLE deletion_guard (
				agent_id UUID PRIMARY KEY REFERENCES agents(id) ON DELETE RESTRICT)`)
			migrationExec(t, f, `INSERT INTO deletion_guard (agent_id) VALUES ($1)`, migrationAgentID)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRefreshMigrationFramework(t)
			seedRefreshMigrationOwners(t, f)
			seedRefreshLegacyRows(t, f)
			require.NoError(t, f.Up(t, 36))
			insertMigrationRefreshRoot(t, f, migrationRootID, migrationSignature("owned first"), migrationAgentID, migrationGrantID, "owner@example.test", "migration-owner")
			tc.fault(t, f)
			require.Error(t, deleteMigrationAgentWithReceipt(f), "failure at %s must abort the entire lifecycle transaction", tc.name)
			require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM agents WHERE id = $1`, migrationAgentID))
			require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM user_grants WHERE id = $1`, migrationGrantID))
			require.True(t, migrationBool(t, f, `
				SELECT revoked_at IS NULL AND expired_at IS NULL AND terminal_reason IS NULL
				AND retry_count = 0 AND current_signature = $2
				FROM refresh_sessions WHERE id = $1`, migrationRootID, migrationSignature("owned first")))
			require.True(t, migrationBool(t, f, `SELECT used_at IS NULL FROM refresh_tokens WHERE signature = $1`, migrationSignature("owned first")))
			require.True(t, migrationBool(t, f, `SELECT used_at IS NULL FROM refresh_token_sessions WHERE signature = $1`, migrationSignature("owned first")))
			require.Equal(t, int64(0), migrationRows(t, f, `SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1`, migrationRootID),
				"a rolled-back receipt cannot claim a revocation succeeded")
			requireUnrelatedMigrationData(t, f)
		})
	}
}

// This is the SQL lifecycle that a coordinated repository must execute. The
// later repository contract tests verify that the broker invokes this order.
func deleteMigrationAgentWithReceipt(f *MigrationTestFramework) error {
	ctx := context.Background()
	tx, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO refresh_revocation_receipts
		(session_id, agent_id, principal, client_id, reason, "at", redacted_context)
		SELECT id, agent_id, principal, client_id, 'agent_deleted',
		TIMESTAMPTZ '2026-09-02 00:00:00+00', '{"request_id":"migration-agent-delete"}'::jsonb
		FROM refresh_sessions WHERE agent_id = $1`, migrationAgentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE refresh_sessions SET revoked_at = TIMESTAMPTZ '2026-09-02 00:00:00+00',
		terminal_reason = 'agent_deleted', retry_ciphertext = NULL WHERE agent_id = $1`, migrationAgentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE refresh_token_sessions SET used_at = TIMESTAMPTZ '2026-09-02 00:00:00+00'
		WHERE agent_id = $1 AND used_at IS NULL`, migrationAgentID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, migrationAgentID); err != nil {
		return err
	}
	return tx.Commit()
}

func requireUnrelatedMigrationData(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.Equal(t, int64(2), migrationRows(t, f, `SELECT count(*) FROM agents WHERE id IN ($1, $2)`, migrationAgentID, migrationOtherAgentID))
	require.Equal(t, int64(2), migrationRows(t, f, `SELECT count(*) FROM user_grants WHERE id IN ($1, $2)`, migrationGrantID, migrationOtherGrantID))
	requireUnrelatedMigrationDataAfterAgentDelete(t, f)
}

func requireUnrelatedMigrationDataAfterAgentDelete(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM agents WHERE id = $1`, migrationOtherAgentID))
	require.Equal(t, int64(1), migrationRows(t, f, `SELECT count(*) FROM user_grants WHERE id = $1`, migrationOtherGrantID))
	require.Equal(t, int64(1), migrationRows(t, f, `
		SELECT count(*) FROM thirdparty_oauth2_services WHERE id = '54000000-0000-0000-0000-000000000001'`))
	require.Equal(t, int64(1), migrationRows(t, f, `
		SELECT count(*) FROM user_sessions
		WHERE id = '55000000-0000-0000-0000-000000000001'
		AND principal = 'owner@example.test'
		AND service_id = '54000000-0000-0000-0000-000000000001'`))
}
