//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration042ClientAssertionReplays(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 41))

	require.NoError(t, f.UpAll(t))
	exists, err := f.TableExists(t, "client_assertion_replays")
	require.NoError(t, err)
	assert.True(t, exists)
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO client_assertion_replays (client_id, jti, expires_at)
		VALUES ('client-a', 'shared-jti', '2030-01-01');
		INSERT INTO client_assertion_replays (client_id, jti, expires_at)
		VALUES ('client-b', 'shared-jti', '2030-01-01');
	`))
	require.Error(t, f.ExecuteSQL(t, `
		INSERT INTO client_assertion_replays (client_id, jti, expires_at)
		VALUES ('client-a', 'shared-jti', '2030-01-01');
	`), "the primary key must reject a repeated client and jti")

	require.NoError(t, f.Down(t, 41))
	exists, err = f.TableExists(t, "client_assertion_replays")
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, f.UpAll(t))
	exists, err = f.TableExists(t, "client_assertion_replays")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMigration043RefreshTokenClientAuthMethod(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 42))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, display_name, description)
		VALUES ('10000000-0000-0000-0000-000000000039', 'migration agent', 'test');
		INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, scope, expires_at)
		VALUES ('legacy-session', 'legacy-request', '10000000-0000-0000-0000-000000000039',
			'client', 'user@example.com', 'offline_access', '2030-01-01');
	`))
	before, err := f.QuerySQL(t, `SELECT to_jsonb(r)::text FROM refresh_token_sessions r WHERE signature = 'legacy-session'`)
	require.NoError(t, err)

	require.NoError(t, f.UpAll(t))
	exists, err := f.ColumnExists(t, "refresh_token_sessions", "client_auth_method")
	require.NoError(t, err)
	assert.True(t, exists)
	method, err := f.QuerySQL(t, `SELECT client_auth_method FROM refresh_token_sessions WHERE signature = 'legacy-session'`)
	require.NoError(t, err)
	assert.Empty(t, method, "legacy refresh sessions must remain NULL")
	after, err := f.QuerySQL(t, `SELECT (to_jsonb(r) - 'client_auth_method')::text FROM refresh_token_sessions r WHERE signature = 'legacy-session'`)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the migration must not change existing columns")

	require.Error(t, f.ExecuteSQL(t, `
		INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, scope, expires_at, client_auth_method)
		VALUES ('invalid-session', 'invalid-request', '10000000-0000-0000-0000-000000000039',
			'client', 'user@example.com', 'read', '2030-01-01', 'client_secret_basic');
	`), "the constraint must reject unsupported methods on insert")
	require.Error(t, f.ExecuteSQL(t, `
		UPDATE refresh_token_sessions
		SET client_auth_method = 'client_secret_basic'
		WHERE signature = 'legacy-session';
	`), "the constraint must reject unsupported methods on update")
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, scope, expires_at, client_auth_method)
		VALUES ('valid-session', 'valid-request', '10000000-0000-0000-0000-000000000039',
			'client', 'user@example.com', 'read', '2030-01-01', 'private_key_jwt');
	`))
	validated, err := f.QuerySQL(t, `
		SELECT convalidated::text FROM pg_constraint
		WHERE conname = 'chk_refresh_token_sessions_client_auth_method'
	`)
	require.NoError(t, err)
	assert.Equal(t, "false", validated)

	require.NoError(t, f.Down(t, 42))
	exists, err = f.ColumnExists(t, "refresh_token_sessions", "client_auth_method")
	require.NoError(t, err)
	assert.False(t, exists)
	afterRollback, err := f.QuerySQL(t, `SELECT to_jsonb(r)::text FROM refresh_token_sessions r WHERE signature = 'legacy-session'`)
	require.NoError(t, err)
	assert.Equal(t, before, afterRollback, "rollback must preserve the existing session")
	require.NoError(t, f.UpAll(t))
	method, err = f.QuerySQL(t, `SELECT client_auth_method FROM refresh_token_sessions WHERE signature = 'legacy-session'`)
	require.NoError(t, err)
	assert.Empty(t, method)
}
