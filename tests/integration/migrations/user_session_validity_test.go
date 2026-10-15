//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration038UserSessionRefreshTokenValidity(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery, scopes)
		VALUES ('38000000-0000-0000-0000-000000000001', 'validity migration service',
			'validity-client', '\x01', 'https://oauth.example.com', false, '[]');
		INSERT INTO user_sessions
			(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type,
			 access_token_expires_at, refresh_token_expires_at, scope, encryption_context,
			 initiated_at, created_at, updated_at)
		SELECT ('38000000-0000-0000-0000-00000000000' || n)::uuid, 'validity-' || n || '@example.com',
			'38000000-0000-0000-0000-000000000001'::uuid, '\x0001ff'::bytea, '\x0200fe'::bytea, 'Bearer',
			CASE n WHEN 2 THEN '2020-01-01'::timestamptz WHEN 3 THEN '2030-01-01'::timestamptz END,
			CASE n WHEN 2 THEN '2020-02-01'::timestamptz WHEN 3 THEN '2030-02-01'::timestamptz END,
			ARRAY['read', 'write'], '{"service_id":"38000000-0000-0000-0000-000000000001"}'::jsonb,
			'2020-01-02'::timestamptz, '2020-01-03'::timestamptz, '2020-01-04'::timestamptz
		FROM generate_series(2, 4) n;
	`))
	before := sessionValiditySnapshot(t, f, "")
	require.NoError(t, f.Up(t, 38))
	requireSessionValidityPhase(t, f, 38)
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""))
	defaults, err := f.QuerySQL(t, `SELECT bool_and(NOT refresh_token_rejected) FROM user_session_state`)
	require.NoError(t, err)
	require.Equal(t, "true", defaults, "past, future, and NULL legacy expiry must not imply provider rejection")
	require.ErrorContains(t, f.ExecuteSQL(t, `SELECT update_user_session_state(
		'38000000-0000-0000-0000-000000000002', '\x01', '\x02', NULL, TRUE, NOW())`),
		"session rejection persistence requires migration 039")
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""), "preparation must not pretend rejection was persisted")

	require.NoError(t, f.Down(t, 37))
	removed, err := f.QuerySQL(t, `SELECT to_regclass('public.user_session_state') IS NULL
		AND to_regprocedure('public.update_user_session_state(uuid,bytea,bytea,timestamptz,boolean,timestamptz)') IS NULL
		AND to_regprocedure('public.upsert_user_session_state(uuid,text,uuid,bytea,bytea,text,timestamptz,boolean,text[],jsonb,timestamptz,timestamptz,timestamptz)') IS NULL`)
	require.NoError(t, err)
	require.Equal(t, "true", removed)
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""), "rollback must preserve every row and its original legacy dates")
	requireSessionExpiryIndex(t, f, true)
	require.NoError(t, f.Up(t, 38))
	requireSessionValidityPhase(t, f, 38)
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""), "reapplication must preserve all token and lifecycle metadata")
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	require.Equal(t, uint(38), version)
	require.False(t, dirty)
}

func sessionValiditySnapshot(t *testing.T, f *MigrationTestFramework, omit string) string {
	t.Helper()
	value, err := f.QuerySQL(t, `SELECT jsonb_agg(to_jsonb(s)`+omit+` ORDER BY id) FROM user_sessions s`)
	require.NoError(t, err)
	return value
}

func requireSessionValidityPhase(t *testing.T, f *MigrationTestFramework, phase uint) {
	t.Helper()
	legacy, err := f.ColumnExists(t, "user_sessions", "refresh_token_expires_at")
	require.NoError(t, err)
	require.Equal(t, phase < 40, legacy)
	rejected, err := f.ColumnExists(t, "user_sessions", "refresh_token_rejected")
	require.NoError(t, err)
	require.Equal(t, phase >= 39, rejected)
	requireSessionExpiryIndex(t, f, true)
	rows, err := f.db.QueryContext(context.Background(), `SELECT * FROM user_session_state LIMIT 0`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	require.NoError(t, err)
	require.Equal(t, []string{"id", "principal", "service_id", "encrypted_access_token", "encrypted_refresh_token",
		"token_type", "access_token_expires_at", "refresh_token_rejected", "scope", "encryption_context", "initiated_at", "created_at", "updated_at"}, columns)
	invoker, err := f.QuerySQL(t, `SELECT 'security_invoker=true' = ANY(reloptions)
		FROM pg_class WHERE oid = 'public.user_session_state'::regclass`)
	require.NoError(t, err)
	require.Equal(t, "true", invoker)
	functions, err := f.QuerySQL(t, `SELECT count(*) = 2 AND bool_and(NOT prosecdef)
		FROM pg_proc WHERE oid IN (
			to_regprocedure('public.update_user_session_state(uuid,bytea,bytea,timestamptz,boolean,timestamptz)'),
			to_regprocedure('public.upsert_user_session_state(uuid,text,uuid,bytea,bytea,text,timestamptz,boolean,text[],jsonb,timestamptz,timestamptz,timestamptz)'))`)
	require.NoError(t, err)
	require.Equal(t, "true", functions, "stable write API must not escalate privileges")
}
