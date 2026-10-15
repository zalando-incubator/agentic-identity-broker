//go:build integration

package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestMigration038SessionStateWriteContract(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	seedSessionExpiryMigration(t, f)
	identity, err := f.QuerySQL(t, `SELECT jsonb_build_array(id, principal, service_id, initiated_at, created_at)
		FROM user_sessions WHERE principal = 'expiry@example.com'`)
	require.NoError(t, err)
	require.NoError(t, f.Up(t, 38))
	requireSessionValidityPhase(t, f, 38)
	ctx := context.Background()
	_, err = f.db.ExecContext(ctx, `SELECT upsert_user_session_state(
		'38000000-0000-0000-0000-000000000001', $1,
		'36000000-0000-0000-0000-000000000001', $2, $3, 'UpdatedType', '2031-01-02', FALSE,
		ARRAY['updated'], '{"service_id":"36000000-0000-0000-0000-000000000001"}',
		'2021-01-01', '2021-01-02', '2001-01-01')`, "expiry@example.com", []byte{4, 5}, []byte{6, 7})
	require.NoError(t, err, "function parameter names must not collide with column or conflict-target names")
	after, err := f.QuerySQL(t, `SELECT jsonb_build_array(id, principal, service_id, initiated_at, created_at)
		FROM user_session_state WHERE principal = 'expiry@example.com'`)
	require.NoError(t, err)
	require.JSONEq(t, identity, after, "upsert must retain the original ID and immutable lifecycle timestamps")
	updated, err := f.QuerySQL(t, `SELECT encrypted_access_token = '\x0405' AND encrypted_refresh_token = '\x0607'
		AND token_type = 'UpdatedType' AND access_token_expires_at = '2031-01-02'::timestamptz
		AND scope = ARRAY['updated'] AND encryption_context = '{"service_id":"36000000-0000-0000-0000-000000000001"}'::jsonb
		AND updated_at > '2001-01-01'::timestamptz AND NOT refresh_token_rejected
		FROM user_session_state WHERE principal = 'expiry@example.com'`)
	require.NoError(t, err)
	require.Equal(t, "true", updated)
	beforeRejection := sessionValiditySnapshot(t, f, "")
	require.ErrorContains(t, f.ExecuteSQL(t, `SELECT upsert_user_session_state(
		id, principal, service_id, '\xdead', '\xbeef', 'DiscardedType',
		NULL, TRUE, ARRAY['discarded'], '{}', NOW(), NOW(), NOW())
		FROM user_session_state WHERE principal = 'expiry@example.com'`), "session rejection persistence requires migration 039")
	require.JSONEq(t, beforeRejection, sessionValiditySnapshot(t, f, ""), "refused upsert must not partially replace credentials or metadata")
	_, err = f.db.ExecContext(ctx, `SELECT upsert_user_session_state(
		'38000000-0000-0000-0000-000000000002', $1,
		'36000000-0000-0000-0000-000000000001', '\x08', NULL, 'Bearer', NULL, FALSE,
		ARRAY[]::text[], '{}', '2022-01-01', '2022-01-02', '2022-01-03')`, "principal'; DELETE FROM user_sessions; --")
	require.NoError(t, err, "caller input must remain data")
	other, err := f.QuerySQL(t, `SELECT to_jsonb(s) FROM user_session_state s WHERE id = '38000000-0000-0000-0000-000000000002'`)
	require.NoError(t, err)
	_, err = f.db.ExecContext(ctx, `SELECT update_user_session_state(
		'36000000-0000-0000-0000-000000000002', '\x090a', '\x0b0c', '2032-01-02', FALSE, NOW())`)
	require.NoError(t, err)
	state, err := f.QuerySQL(t, `SELECT encrypted_access_token = '\x090a' AND encrypted_refresh_token = '\x0b0c'
		AND access_token_expires_at = '2032-01-02'::timestamptz AND token_type = 'UpdatedType'
		AND scope = ARRAY['updated'] AND NOT refresh_token_rejected
		FROM user_session_state WHERE id = '36000000-0000-0000-0000-000000000002'`)
	require.NoError(t, err)
	require.Equal(t, "true", state)
	beforeRejection = sessionValiditySnapshot(t, f, "")
	require.ErrorContains(t, f.ExecuteSQL(t, `SELECT update_user_session_state(
		id, '\xdead', '\xbeef', NULL, TRUE, NOW())
		FROM user_session_state WHERE principal = 'expiry@example.com'`), "session rejection persistence requires migration 039")
	require.JSONEq(t, beforeRejection, sessionValiditySnapshot(t, f, ""), "refused locked update must not partially replace credentials or metadata")
	otherAfter, err := f.QuerySQL(t, `SELECT to_jsonb(s) FROM user_session_state s WHERE id = '38000000-0000-0000-0000-000000000002'`)
	require.NoError(t, err)
	require.JSONEq(t, other, otherAfter, "locked update must change only its target row")
	require.NoError(t, f.ExecuteSQL(t, `SELECT update_user_session_state(
		'38000000-0000-0000-0000-000000000003', '\x01', NULL, NULL, FALSE, NOW())`))
	missing, err := f.QuerySQL(t, `SELECT count(*) FROM user_sessions WHERE id = '38000000-0000-0000-0000-000000000003'`)
	require.NoError(t, err)
	require.Equal(t, "0", missing, "locked update must never recreate a terminated session")
	tx, err := f.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var lockedID string
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT id FROM user_session_state WHERE principal = 'expiry@example.com' FOR UPDATE`).Scan(&lockedID))
	_, err = f.db.ExecContext(ctx, `SELECT id FROM user_sessions WHERE id = $1 FOR UPDATE NOWAIT`, lockedID)
	require.ErrorContains(t, err, "could not obtain lock on row", "view reads must lock the original table row")
	require.NoError(t, tx.Rollback())
}

func TestSessionStateSQLBoundaryDoesNotEscalatePrivileges(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	seedSessionExpiryMigration(t, f)
	require.NoError(t, f.Up(t, 38))
	role := "session_state_" + strings.ReplaceAll(id.NewSessionID().String(), "-", "")
	require.NoError(t, f.ExecuteSQL(t, "CREATE ROLE "+role))
	defer func() {
		require.NoError(t, f.ExecuteSQL(t, "DROP POLICY IF EXISTS session_state_test_tenant ON user_sessions; DROP OWNED BY "+role+"; DROP ROLE "+role))
	}()
	require.NoError(t, f.ExecuteSQL(t, "GRANT USAGE ON SCHEMA public TO "+role+"; GRANT SELECT ON user_session_state TO "+role))
	ctx := context.Background()
	for _, query := range []string{
		`SELECT id FROM user_session_state`,
		`SELECT update_user_session_state('36000000-0000-0000-0000-000000000002', '\x01', NULL, NULL, FALSE, NOW())`,
		`SELECT upsert_user_session_state('39000000-0000-0000-0000-000000000004', 'other@example.com',
			'36000000-0000-0000-0000-000000000001', '\x01', NULL, 'Bearer', NULL, FALSE,
			ARRAY[]::text[], '{}', NOW(), NOW(), NOW())`,
	} {
		tx, err := f.db.BeginTx(ctx, nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+role)
		require.NoError(t, err)
		_, err = tx.ExecContext(ctx, query)
		require.ErrorContains(t, err, "permission denied for table user_sessions")
		require.NoError(t, tx.Rollback())
	}
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO user_sessions (principal, service_id, encrypted_access_token)
		VALUES ('another-tenant@example.com', '36000000-0000-0000-0000-000000000001', '\x02');
		ALTER TABLE user_sessions ENABLE ROW LEVEL SECURITY;
		GRANT SELECT ON user_sessions TO `+role+`;
		CREATE POLICY session_state_test_tenant ON user_sessions FOR SELECT TO `+role+`
			USING (principal = 'expiry@example.com');
	`))
	tx, err := f.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "SET LOCAL ROLE "+role)
	require.NoError(t, err)
	var visible int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT count(*) FROM user_session_state`).Scan(&visible))
	require.Equal(t, 1, visible, "security-invoker view must preserve the caller's underlying tenant policy")
	require.NoError(t, tx.Rollback())
}
