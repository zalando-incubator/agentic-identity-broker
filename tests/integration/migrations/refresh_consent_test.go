//go:build integration

package migrations

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration036RefreshTokenGrantIdentity(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 35))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, display_name, description)
		VALUES ('10000000-0000-0000-0000-000000000036', 'refresh agent', 'migration test');
		INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, scope, expires_at, used_at, email, display_name)
		VALUES
			('legacy-unused', 'request-unused', '10000000-0000-0000-0000-000000000036', 'client', 'user@example.com', 'offline_access', '2030-01-01', NULL, NULL, ''),
			('legacy-used', 'request-used', '10000000-0000-0000-0000-000000000036', 'client', 'user@example.com', 'read', '2030-01-01', '2026-01-01', 'user@example.com', 'User');
	`))
	snapshotSQL := `SELECT jsonb_agg(to_jsonb(r) - 'grant_id' ORDER BY signature)::text FROM refresh_token_sessions r`
	before, err := f.QuerySQL(t, snapshotSQL)
	require.NoError(t, err)
	require.NoError(t, f.UpAll(t))
	exists, err := f.ColumnExists(t, "refresh_token_sessions", "grant_id")
	require.NoError(t, err)
	require.True(t, exists, "refresh sessions need a nullable grant identity")
	column, err := f.QuerySQL(t, `SELECT data_type || ':' || is_nullable FROM information_schema.columns WHERE table_name = 'refresh_token_sessions' AND column_name = 'grant_id'`)
	require.NoError(t, err)
	assert.Equal(t, "uuid:YES", column)
	nullCount, err := f.QuerySQL(t, `SELECT count(*) FROM refresh_token_sessions WHERE grant_id IS NULL`)
	require.NoError(t, err)
	assert.Equal(t, "2", nullCount, "legacy rows must not be backfilled")
	require.NoError(t, f.ExecuteSQL(t, `UPDATE refresh_token_sessions SET grant_id = '20000000-0000-0000-0000-000000000036' WHERE signature = 'legacy-unused'`))
	grant, err := f.QuerySQL(t, `SELECT grant_id FROM refresh_token_sessions WHERE signature = 'legacy-unused'`)
	require.NoError(t, err)
	assert.Equal(t, "20000000-0000-0000-0000-000000000036", grant, "historical grant IDs need no live grant foreign key")
	after, err := f.QuerySQL(t, snapshotSQL)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.NoError(t, f.Down(t, 35))
	exists, err = f.ColumnExists(t, "refresh_token_sessions", "grant_id")
	require.NoError(t, err)
	assert.False(t, exists)
	after, err = f.QuerySQL(t, snapshotSQL)
	require.NoError(t, err)
	assert.Equal(t, before, after, "rollback must preserve every pre-existing refresh field")
	require.NoError(t, f.Up(t, 36))
	nullCount, err = f.QuerySQL(t, `SELECT count(*) FROM refresh_token_sessions WHERE grant_id IS NULL`)
	require.NoError(t, err)
	assert.Equal(t, "2", nullCount)
	after, err = f.QuerySQL(t, snapshotSQL)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestMigration036RestoresConnectionLockTimeout(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	ctx := t.Context()
	conn, err := f.db.Conn(ctx)
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	_, err = conn.ExecContext(ctx, `CREATE TEMP TABLE refresh_token_sessions (signature text)`)
	require.NoError(t, err)
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	require.NoError(t, err)
	var baseline string
	require.NoError(t, conn.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&baseline))

	for _, direction := range []string{"up", "down"} {
		t.Run(direction, func(t *testing.T) {
			migration, err := os.ReadFile(filepath.Join(f.migrationsDir, "036_add_refresh_token_grant_id."+direction+".sql"))
			require.NoError(t, err)
			require.NoError(t, driver.Run(bytes.NewReader(migration)))
			var timeout string
			require.NoError(t, conn.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&timeout))
			assert.Equal(t, baseline, timeout, "later migrations must not inherit migration 036's lock timeout")
		})
	}
}
