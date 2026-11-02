//go:build integration

package migrations

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration046PreservesLegacySessionReaders(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 45))
	seedSessionExpiryMigration(t, f)
	require.NoError(t, f.Up(t, 46))
	_, err := f.QuerySQL(t, `SELECT COALESCE(refresh_token_expires_at::text, 'unknown') FROM user_sessions WHERE principal = 'expiry@example.com'`)
	require.NoError(t, err, "preparation migration must preserve old session queries")
	rows, err := f.db.QueryContext(context.Background(), `SELECT * FROM user_sessions`)
	require.NoError(t, err)
	defer rows.Close()
	columns, err := rows.Columns()
	require.NoError(t, err)
	require.NotContains(t, columns, "refresh_token_rejected", "preparation must not break strict legacy SELECT-star readers")
}

func TestMigration046PreservesLegacySessionWriters(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 45))
	seedSessionExpiryMigration(t, f)
	for _, phase := range []uint{45, 46} {
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
			"token_type", "access_token_expires_at", "refresh_token_expires_at", "scope", "encryption_context", "initiated_at", "created_at", "updated_at", "callback_id", "upstream_client_id"}, columns)
	}
}

func TestPreparedCredentialReplacementRestoresLegacyRefresh(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 45))
	seedSessionExpiryMigration(t, f)
	require.NoError(t, f.Up(t, 46))
	for _, testCase := range []struct {
		name, write string
		cleared     bool
	}{
		{"authorization replacement", `SELECT upsert_user_session_state(id, principal, service_id,
			'\x0405', '\x0607', token_type, access_token_expires_at, FALSE, scope, encryption_context,
			initiated_at, created_at, NOW(), 'reconnected', upstream_client_id, '', '', TRUE)
			FROM user_session_state WHERE principal = 'expiry@example.com'`, true},
		{"rotated refresh", `SELECT update_user_session_state(id, '\x0809', '\x0a0b', access_token_expires_at,
			FALSE, NOW(), token_type, scope, encryption_context, callback_id, upstream_client_id)
			FROM user_session_state WHERE principal = 'expiry@example.com'`, true},
		{"retained refresh", `SELECT update_user_session_state(id, '\x0c0d', encrypted_refresh_token, access_token_expires_at,
			FALSE, NOW(), token_type, scope, encryption_context, callback_id, upstream_client_id)
			FROM user_session_state WHERE principal = 'expiry@example.com'`, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, f.ExecuteSQL(t, `UPDATE user_sessions SET refresh_token_expires_at = '2000-01-01'
				WHERE principal = 'expiry@example.com'`))
			require.NoError(t, f.ExecuteSQL(t, testCase.write))
			usable, err := f.QuerySQL(t, `SELECT encrypted_refresh_token IS NOT NULL
				AND (refresh_token_expires_at IS NULL OR refresh_token_expires_at > NOW())
				FROM user_sessions WHERE principal = 'expiry@example.com'`)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprint(testCase.cleared), usable, "legacy refresh eligibility must follow credential replacement")
			marker, err := f.QuerySQL(t, `SELECT COALESCE(refresh_token_expires_at = '2000-01-01'::timestamptz, FALSE)
				FROM user_sessions WHERE principal = 'expiry@example.com'`)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprint(!testCase.cleared), marker, "retaining the credential must preserve its legacy marker")
			if testCase.cleared {
				require.NoError(t, f.ExecuteSQL(t, `UPDATE user_sessions SET encrypted_access_token = '\x1011',
					access_token_expires_at = NOW() + INTERVAL '1 hour'
					WHERE principal = 'expiry@example.com' AND encrypted_refresh_token IS NOT NULL
					AND (refresh_token_expires_at IS NULL OR refresh_token_expires_at > NOW())`))
				refreshed, err := f.QuerySQL(t, `SELECT encrypted_access_token = '\x1011'
					AND access_token_expires_at > NOW() AND NOT refresh_token_rejected
					FROM user_session_state WHERE principal = 'expiry@example.com'`)
				require.NoError(t, err)
				require.Equal(t, "true", refreshed, "prepared readers must see a legacy refresh after credential recovery")
			}
		})
	}
}
