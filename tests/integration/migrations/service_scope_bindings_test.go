//go:build integration

package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Migration 046 from specs/055-incremental-service-authorization/data-model.md (DEC-004).
func TestMigration046ServiceScopeBindings(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 45))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, display_name, description)
		VALUES ('10000000-0000-0000-0000-000000000046', 'bindings agent', 'migration test');
		INSERT INTO authorization_codes
			(id, code_hash, agent_id, client_id, principal, redirect_uri, code_challenge, scope, expires_at, email, display_name)
		VALUES ('30000000-0000-0000-0000-000000000046', 'legacy-code', '10000000-0000-0000-0000-000000000046', 'client',
			'user@example.com', 'https://client.example.com/cb', 'challenge', 'openid offline_access', '2030-01-01', NULL, '');
		INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, scope, expires_at, used_at, email, display_name)
		VALUES ('legacy-refresh', 'request', '10000000-0000-0000-0000-000000000046', 'client', 'user@example.com',
			'openid offline_access', '2030-01-01', NULL, NULL, '');
	`))
	codeSnapshot := `SELECT jsonb_agg(to_jsonb(c) - 'service_scope_bindings' ORDER BY code_hash)::text FROM authorization_codes c`
	refreshSnapshot := `SELECT jsonb_agg(to_jsonb(r) - 'service_scope_bindings' ORDER BY signature)::text FROM refresh_token_sessions r`
	codesBefore, err := f.QuerySQL(t, codeSnapshot)
	require.NoError(t, err)
	refreshBefore, err := f.QuerySQL(t, refreshSnapshot)
	require.NoError(t, err)

	requireColumns := func(present bool) {
		for _, table := range []string{"authorization_codes", "refresh_token_sessions"} {
			exists, err := f.ColumnExists(t, table, "service_scope_bindings")
			require.NoError(t, err)
			require.Equal(t, present, exists, table)
			if present {
				column, err := f.QuerySQL(t, `SELECT data_type || ':' || is_nullable FROM information_schema.columns WHERE table_name = '`+table+`' AND column_name = 'service_scope_bindings'`)
				require.NoError(t, err)
				assert.Equal(t, "jsonb:YES", column, table)
			}
		}
	}
	requireUnchanged := func() {
		after, err := f.QuerySQL(t, codeSnapshot)
		require.NoError(t, err)
		assert.Equal(t, codesBefore, after)
		after, err = f.QuerySQL(t, refreshSnapshot)
		require.NoError(t, err)
		assert.Equal(t, refreshBefore, after)
	}

	require.NoError(t, f.Up(t, 46))
	requireColumns(true)
	nullCount, err := f.QuerySQL(t, `SELECT (SELECT count(*) FROM authorization_codes WHERE service_scope_bindings IS NULL) + (SELECT count(*) FROM refresh_token_sessions WHERE service_scope_bindings IS NULL)`)
	require.NoError(t, err)
	assert.Equal(t, "2", nullCount, "pre-feature rows must not be backfilled")
	requireUnchanged()
	require.NoError(t, f.ExecuteSQL(t, `UPDATE refresh_token_sessions SET service_scope_bindings = '{"https://broker.example.com/sessions/github":"880e8400-e29b-41d4-a716-446655440010"}' WHERE signature = 'legacy-refresh'`))

	require.NoError(t, f.Down(t, 45))
	requireColumns(false)
	requireUnchanged()

	require.NoError(t, f.Up(t, 46))
	requireColumns(true)
	bindings, err := f.QuerySQL(t, `SELECT count(*) FROM refresh_token_sessions WHERE service_scope_bindings IS NOT NULL`)
	require.NoError(t, err)
	assert.Equal(t, "0", bindings, "rollback discards bindings by design")
	requireUnchanged()
}
