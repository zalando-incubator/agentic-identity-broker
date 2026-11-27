//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func serviceAccountMetadataMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "service_account_metadata_51", func(t *testing.T, name string) {
		pg.ApplyMigrationsUpTo(t, name, 51)
	})
	db, err := sql.Open("pgx", url)
	require.NoError(t, err)
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	require.NoError(t, err)
	f := &MigrationTestFramework{db: db, connStr: url, dbName: name, migrationsDir: dir, cleanup: cleanup}
	t.Cleanup(func() { f.Cleanup(t) })
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery,
		 token_endpoint, authorize_endpoint, scopes, authorization_params, labels, version, created_at, updated_at)
		VALUES ('52000000-0000-0000-0000-000000000001', 'company-platform', 'Company Platform', 'platform-client', decode('01020304','hex'),
		 'https://idp.example.test', false, 'https://idp.example.test/token', 'https://idp.example.test/authorize',
		 '[{"scope_value":"read","description":"Read"}]', '{}', '{"tier":"gold"}', 3, '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO service_accounts
		(id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary,
		 credential_expires_at, version, created_at, updated_at)
		VALUES ('52000000-0000-0000-0000-000000000002', '52000000-0000-0000-0000-000000000001', 'platform-triage-bot', 'jwt_bearer',
		 '["read"]', decode('0a0b0c','hex'),
		 '{"format":"signed_jwt","use":"access_token","fingerprint":"SHA256:fhbwk8g0dDlEiuXzadc7Ytx+R1pSlxRBBNgzBxN+3UQ"}',
		 '2030-01-01 00:00:00+00', 4, '2020-01-02 03:04:05+00', '2021-02-03 04:05:06+00');`))
	return f
}

func serviceAccountMetadataMigrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'accounts', (SELECT jsonb_agg(jsonb_build_array(id, service_id, canonical_id, kind, scopes, credential_encrypted,
		 credential_summary, credential_expires_at, version, created_at, updated_at) ORDER BY id) FROM service_accounts),
		'services', (SELECT jsonb_agg(jsonb_build_array(id, labels, annotations, version, updated_at) ORDER BY id)
		 FROM thirdparty_oauth2_services)
	)::text`)
	require.NoError(t, err)
	return data
}

func serviceAccountMetadataColumns(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	columns, err := f.QuerySQL(t, `SELECT COALESCE(string_agg(column_name, ',' ORDER BY column_name), '')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'service_accounts' AND column_name IN ('labels', 'annotations')`)
	require.NoError(t, err)
	return columns
}

func TestServiceAccountMetadataMigrationUpDownUpPreservesExistingData(t *testing.T) {
	f := serviceAccountMetadataMigrationFixture(t)
	before := serviceAccountMetadataMigrationData(t, f)
	require.NoError(t, f.Up(t, 52))
	assert.Equal(t, before, serviceAccountMetadataMigrationData(t, f))
	for _, column := range []string{"labels", "annotations"} {
		shape, err := f.QuerySQL(t, `SELECT data_type = 'jsonb' AND is_nullable = 'NO' AND column_default = '''{}''::jsonb'
			FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'service_accounts' AND column_name = '`+column+`'`)
		require.NoError(t, err)
		assert.Equal(t, "true", shape, "service_accounts.%s must be JSONB NOT NULL DEFAULT '{}'", column)
		empty, err := f.QuerySQL(t, `SELECT bool_and(`+column+` = '{}'::jsonb) FROM service_accounts`)
		require.NoError(t, err)
		assert.Equal(t, "true", empty, "pre-migration accounts read as empty %s", column)
	}
	require.NoError(t, f.Down(t, 51))
	assert.Empty(t, serviceAccountMetadataColumns(t, f))
	assert.Equal(t, before, serviceAccountMetadataMigrationData(t, f), "rollback loses only service-account metadata")
	require.NoError(t, f.Up(t, 52))
	assert.Equal(t, before, serviceAccountMetadataMigrationData(t, f))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(52), version)
	assert.False(t, dirty)
}

func TestServiceAccountMetadataMigrationRejectsNonObjectValues(t *testing.T) {
	f := serviceAccountMetadataMigrationFixture(t)
	require.NoError(t, f.Up(t, 52))
	for column, constraint := range map[string]string{"labels": "chk_service_accounts_labels_object", "annotations": "chk_service_accounts_annotations_object"} {
		for _, value := range []string{"null", "[]", `"text"`, "1"} {
			t.Run(column+"="+value, func(t *testing.T) {
				_, err := f.db.ExecContext(context.Background(), `UPDATE service_accounts SET `+column+` = $1::jsonb`, value)
				var pgErr *pgconn.PgError
				require.ErrorAs(t, err, &pgErr)
				assert.Equal(t, "23514", pgErr.Code)
				assert.Equal(t, constraint, pgErr.ConstraintName)
			})
		}
		t.Run(column+" accepts an object", func(t *testing.T) {
			_, err := f.db.ExecContext(context.Background(), `UPDATE service_accounts SET `+column+` = $1::jsonb`, `{"tier":"gold"}`)
			require.NoError(t, err)
		})
	}
}

func TestServiceAccountMetadataMigrationBusyTableFailsWithoutQueueing(t *testing.T) {
	t.Run("up", func(t *testing.T) {
		f := serviceAccountMetadataMigrationFixture(t)
		before := serviceAccountMetadataMigrationData(t, f)
		boundLockWait(f)
		release := holdReadLock(t, f, "service_accounts")
		started := time.Now()
		err := f.Up(t, 52)
		require.ErrorContains(t, err, "could not obtain lock on relation", "a busy table must fail NOWAIT instead of queueing an exclusive lock")
		assert.Less(t, time.Since(started), 5*time.Second)
		assert.Empty(t, serviceAccountMetadataColumns(t, f), "contention must roll back every added column")
		assert.Equal(t, before, serviceAccountMetadataMigrationData(t, f))
		release()
		require.NoError(t, f.Force(t, 51))
		require.NoError(t, f.Up(t, 52))
	})
	t.Run("down", func(t *testing.T) {
		f := serviceAccountMetadataMigrationFixture(t)
		require.NoError(t, f.Up(t, 52))
		boundLockWait(f)
		release := holdReadLock(t, f, "service_accounts")
		err := f.Down(t, 51)
		require.ErrorContains(t, err, "could not obtain lock on relation")
		assert.Equal(t, "annotations,labels", serviceAccountMetadataColumns(t, f), "contention must keep every column")
		release()
		require.NoError(t, f.Force(t, 52))
		require.NoError(t, f.Down(t, 51))
		assert.Empty(t, serviceAccountMetadataColumns(t, f))
	})
}
