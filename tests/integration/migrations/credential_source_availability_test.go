//go:build integration

package migrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration040BusyTablesDoNotQueueExclusiveLocks(t *testing.T) {
	for _, table := range []string{"thirdparty_oauth2_services", "user_sessions"} {
		t.Run(table, func(t *testing.T) {
			f := NewMigrationTestFramework(t)
			defer f.Cleanup(t)
			require.NoError(t, f.Up(t, 39))
			seedCredentialMigrationLegacyRows(t, f)
			before := credentialMigrationSchema(t, f)

			traffic, err := f.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = traffic.Rollback() }()
			_, err = traffic.ExecContext(context.Background(), "SELECT id FROM "+table)
			require.NoError(t, err)

			f.connStr += "&options=-c%20statement_timeout%3D1000"
			err = f.Up(t, 40)
			require.ErrorContains(t, err, "could not obtain lock on relation", "busy tables must fail NOWAIT, not wait until live requests time out")
			assert.Equal(t, before, credentialMigrationSchema(t, f), "contention must roll back the entire expansion")

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = f.db.ExecContext(ctx, `UPDATE user_sessions SET scope = ARRAY['read', 'write'] WHERE principal = 'legacy@example.test'`)
			require.NoError(t, err, "traffic must remain writable while the blocking request is still active")
			require.NoError(t, f.Force(t, 39))
			require.NoError(t, traffic.Rollback())
			require.NoError(t, f.Up(t, 40))
		})
	}
}

func TestMigration041ValidationAllowsLiveReadsAndWrites(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 39))
	seedCredentialMigrationLegacyRows(t, f)
	heapFiles, err := f.QuerySQL(t, `SELECT jsonb_object_agg(relname, relfilenode)::text FROM pg_class
		WHERE oid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass)`)
	require.NoError(t, err)
	require.NoError(t, f.Up(t, 40))
	expandedHeapFiles, err := f.QuerySQL(t, `SELECT jsonb_object_agg(relname, relfilenode)::text FROM pg_class
		WHERE oid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass)`)
	require.NoError(t, err)
	assert.Equal(t, heapFiles, expandedHeapFiles, "expansion must not rewrite either live table")
	before := credentialMigrationFullData(t, f)

	traffic, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = traffic.Rollback() }()
	_, err = traffic.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services SET display_name = display_name;
		UPDATE user_sessions SET scope = scope`)
	require.NoError(t, err)
	f.connStr += "&options=-c%20statement_timeout%3D1000"
	require.NoError(t, f.Up(t, 41), "validation must coexist with active runtime writers")
	require.NoError(t, traffic.Rollback())

	validated, err := f.QuerySQL(t, `SELECT bool_and(convalidated) FROM pg_constraint
		WHERE conname IN ('chk_thirdparty_oauth2_services_credential_source', 'chk_thirdparty_oauth2_services_client_auth', 'chk_user_sessions_upstream_client_id')`)
	require.NoError(t, err)
	assert.Equal(t, "true", validated)
	require.Equal(t, before, credentialMigrationFullData(t, f))

	require.NoError(t, f.Down(t, 40))
	validated, err = f.QuerySQL(t, `SELECT bool_or(convalidated) FROM pg_constraint
		WHERE conname IN ('chk_thirdparty_oauth2_services_credential_source', 'chk_thirdparty_oauth2_services_client_auth', 'chk_user_sessions_upstream_client_id')`)
	require.NoError(t, err)
	assert.Equal(t, "false", validated)

	validationSQL, err := os.ReadFile(filepath.Join(f.migrationsDir, "041_validate_oauth2_credential_constraints.up.sql"))
	require.NoError(t, err)
	validation, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = validation.Rollback() }()
	_, err = validation.ExecContext(context.Background(), string(validationSQL))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var clientID string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT client_id FROM thirdparty_oauth2_services WHERE canonical_id = 'legacy.Standard'`).Scan(&clientID))
	assert.Equal(t, "legacy-client", clientID)
	_, err = f.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET display_name = 'Live update' WHERE canonical_id = 'legacy.Standard';
		UPDATE user_sessions SET scope = ARRAY['read', 'write'] WHERE principal = 'legacy@example.test'`)
	require.NoError(t, err, "validation locks must not block runtime reads or writes")
	require.Error(t, f.ExecuteSQL(t, `UPDATE user_sessions SET upstream_client_id = ''`), "unvalidated constraints must still protect new writes")
	require.NoError(t, validation.Rollback())
	require.NoError(t, f.Up(t, 41))
}
