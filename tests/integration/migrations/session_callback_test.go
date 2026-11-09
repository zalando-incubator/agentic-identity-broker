//go:build integration

package migrations

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigration038SessionCallbackGeneration(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	seedSessionExpiryMigration(t, f)
	require.NoError(t, f.Up(t, 38))
	var callbackID string
	require.NoError(t, f.db.QueryRowContext(context.Background(),
		`SELECT callback_id FROM user_sessions WHERE principal = 'expiry@example.com'`).Scan(&callbackID))
	require.Empty(t, callbackID, "existing sessions must remain reconnectable without a prior callback generation")
	requireSessionExpiryMigrationData(t, f)
	require.NoError(t, f.ExecuteSQL(t, `UPDATE user_sessions SET callback_id = 'connected' WHERE principal = 'expiry@example.com'`))
	require.NoError(t, f.Up(t, 37))
	requireSessionExpiryMigrationData(t, f)
	exists, err := f.ColumnExists(t, "user_sessions", "callback_id")
	require.NoError(t, err)
	require.False(t, exists)
	require.NoError(t, f.Up(t, 38))
	requireSessionExpiryMigrationData(t, f)
	require.NoError(t, f.db.QueryRowContext(context.Background(),
		`SELECT callback_id FROM user_sessions WHERE principal = 'expiry@example.com'`).Scan(&callbackID))
	require.Empty(t, callbackID)
}
