//go:build integration

package migrations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration038PreservesLegacySessionReaders(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	seedSessionExpiryMigration(t, f)
	require.NoError(t, f.Up(t, 38))
	_, err := f.QuerySQL(t, `SELECT COALESCE(refresh_token_expires_at::text, 'unknown') FROM user_sessions WHERE principal = 'expiry@example.com'`)
	require.NoError(t, err, "preparation migration must preserve old session queries")
	rows, err := f.db.QueryContext(context.Background(), `SELECT * FROM user_sessions`)
	require.NoError(t, err)
	defer rows.Close()
	columns, err := rows.Columns()
	require.NoError(t, err)
	require.NotContains(t, columns, "refresh_token_rejected", "preparation must not break strict legacy SELECT-star readers")
}

func TestMigration038PreservesLegacySessionWriters(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	seedSessionExpiryMigration(t, f)
	for _, phase := range []uint{37, 38} {
		require.NoError(t, f.Up(t, phase))
		require.NoError(t, f.ExecuteSQL(t, `
			INSERT INTO user_sessions
				(id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
				 token_type, access_token_expires_at, refresh_token_expires_at, scope,
				 encryption_context, initiated_at, created_at, updated_at)
			VALUES ('38000000-0000-0000-0000-000000000010', 'expiry@example.com',
				'36000000-0000-0000-0000-000000000001', '\x0405', '\x0607', 'Bearer',
				'2031-01-02', '2032-01-02', ARRAY['legacy'], '{}', NOW(), NOW(), NOW())
			ON CONFLICT (principal, service_id) DO UPDATE SET
				encrypted_access_token = EXCLUDED.encrypted_access_token,
				encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
				access_token_expires_at = EXCLUDED.access_token_expires_at,
				refresh_token_expires_at = EXCLUDED.refresh_token_expires_at,
				scope = EXCLUDED.scope,
				updated_at = NOW();
		`))
		preserved, err := f.QuerySQL(t, `SELECT id = '36000000-0000-0000-0000-000000000002'::uuid
			AND encrypted_access_token = '\x0405' AND encrypted_refresh_token = '\x0607'
			AND refresh_token_expires_at = '2032-01-02'::timestamptz
			FROM user_sessions WHERE principal = 'expiry@example.com'`)
		require.NoError(t, err)
		require.Equal(t, "true", preserved)
		rows, err := f.db.QueryContext(context.Background(), `SELECT * FROM user_sessions LIMIT 0`)
		require.NoError(t, err)
		columns, err := rows.Columns()
		require.NoError(t, err)
		require.NoError(t, rows.Close())
		require.Equal(t, []string{"id", "principal", "service_id", "encrypted_access_token", "encrypted_refresh_token",
			"token_type", "access_token_expires_at", "refresh_token_expires_at", "scope", "encryption_context", "initiated_at", "created_at", "updated_at"}, columns)
	}
}
