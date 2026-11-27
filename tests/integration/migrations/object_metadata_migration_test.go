//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var objectMetadataTables = []struct {
	table, labels, annotations string
}{
	{"agents", "chk_agents_labels_object", "chk_agents_annotations_object"},
	{"thirdparty_oauth2_services", "chk_thirdparty_oauth2_services_labels_object", "chk_thirdparty_oauth2_services_annotations_object"},
	{"permission_sets", "chk_permission_sets_labels_object", "chk_permission_sets_annotations_object"},
}

func objectMetadataMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "object_metadata_50", func(t *testing.T, name string) {
		pg.ApplyMigrationsUpTo(t, name, 50)
	})
	db, err := sql.Open("pgx", url)
	require.NoError(t, err)
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	require.NoError(t, err)
	f := &MigrationTestFramework{db: db, connStr: url, dbName: name, migrationsDir: dir, cleanup: cleanup}
	t.Cleanup(func() { f.Cleanup(t) })
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery,
		 token_endpoint, authorize_endpoint, scopes, authorization_params, version, created_at, updated_at)
		VALUES ('50000000-0000-0000-0000-000000000001', 'atlas-files', 'Atlas Files', 'atlas-client', decode('01020304','hex'),
		 'https://atlas.example.test', false, 'https://atlas.example.test/token', 'https://atlas.example.test/authorize',
		 '[{"scope_value":"read","description":"Read files"}]', '{"audience":"atlas"}', 7, '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO permission_sets (id, canonical_id, name, description, created_at, updated_at)
		VALUES ('50000000-0000-0000-0000-000000000002', 'files-read', 'Files read', 'Read files', '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO permission_set_service_scopes (permission_set_id, service_id, scopes, requirement_type)
		VALUES ('50000000-0000-0000-0000-000000000002', '50000000-0000-0000-0000-000000000001', '["read"]', 'mandatory');
		INSERT INTO agents (id, canonical_id, external_id, display_name, description, permission_sets, created_at, updated_at)
		VALUES ('50000000-0000-0000-0000-000000000003', 'research', 'registry/research', 'Research', 'Definition',
		 '[{"permission_set_id":"50000000-0000-0000-0000-000000000002","requirement_type":"mandatory"}]',
		 '2020-01-02 03:04:05', '2021-02-03 04:05:06');`))
	return f
}

func objectMetadataMigrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'agents', (SELECT jsonb_agg(jsonb_build_array(id, canonical_id, client_id, external_id, display_name, description,
		 governance_url, user_documentation_url, agent_interface_url, service_requirements, permission_sets,
		 redirect_uris, allowed_scopes, created_at, updated_at) ORDER BY id) FROM agents),
		'services', (SELECT jsonb_agg(jsonb_build_array(id, canonical_id, display_name, icon_svg, client_id,
		 client_secret_encrypted, issuer_uri, enable_discovery, token_endpoint, authorize_endpoint, scopes,
		 authorization_params, created_at, updated_at, version) ORDER BY id) FROM thirdparty_oauth2_services),
		'permission_sets', (SELECT jsonb_agg(jsonb_build_array(id, canonical_id, name, description, created_at, updated_at) ORDER BY id)
		 FROM permission_sets),
		'scopes', (SELECT jsonb_agg(jsonb_build_array(permission_set_id, service_id, scopes, requirement_type) ORDER BY permission_set_id)
		 FROM permission_set_service_scopes)
	)::text`)
	require.NoError(t, err)
	return data
}

func TestObjectMetadataMigrationUpDownUpPreservesExistingData(t *testing.T) {
	f := objectMetadataMigrationFixture(t)
	before := objectMetadataMigrationData(t, f)
	require.NoError(t, f.Up(t, 51))
	assert.Equal(t, before, objectMetadataMigrationData(t, f))
	for _, tc := range objectMetadataTables {
		for _, column := range []string{"labels", "annotations"} {
			columnType, err := f.GetColumnType(t, tc.table, column)
			require.NoError(t, err)
			assert.Equal(t, "jsonb", columnType, "%s.%s", tc.table, column)
			shape, err := f.QuerySQL(t, `SELECT is_nullable = 'NO' AND column_default = '''{}''::jsonb'
				FROM information_schema.columns WHERE table_schema = 'public' AND table_name = '`+tc.table+`' AND column_name = '`+column+`'`)
			require.NoError(t, err)
			assert.Equal(t, "true", shape, "%s.%s must be NOT NULL DEFAULT '{}'", tc.table, column)
			empty, err := f.QuerySQL(t, `SELECT bool_and(`+column+` = '{}'::jsonb) FROM `+tc.table)
			require.NoError(t, err)
			assert.Equal(t, "true", empty, "pre-migration rows of %s read as empty %s", tc.table, column)
		}
	}
	require.NoError(t, f.Down(t, 50))
	for _, tc := range objectMetadataTables {
		for _, column := range []string{"labels", "annotations"} {
			exists, err := f.ColumnExists(t, tc.table, column)
			require.NoError(t, err)
			assert.False(t, exists, "%s.%s", tc.table, column)
		}
	}
	assert.Equal(t, before, objectMetadataMigrationData(t, f), "rollback loses only metadata")
	require.NoError(t, f.Up(t, 51))
	assert.Equal(t, before, objectMetadataMigrationData(t, f))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(51), version)
	assert.False(t, dirty)
}

func TestObjectMetadataMigrationRejectsNonObjectValues(t *testing.T) {
	f := objectMetadataMigrationFixture(t)
	require.NoError(t, f.Up(t, 51))
	for _, tc := range objectMetadataTables {
		for column, constraint := range map[string]string{"labels": tc.labels, "annotations": tc.annotations} {
			for _, value := range []string{"null", "[]", `"text"`, "1"} {
				t.Run(tc.table+"."+column+"="+value, func(t *testing.T) {
					_, err := f.db.ExecContext(context.Background(), `UPDATE `+tc.table+` SET `+column+` = $1::jsonb`, value)
					var pgErr *pgconn.PgError
					require.ErrorAs(t, err, &pgErr)
					assert.Equal(t, "23514", pgErr.Code)
					assert.Equal(t, constraint, pgErr.ConstraintName)
				})
			}
			t.Run(tc.table+"."+column+" accepts an object", func(t *testing.T) {
				_, err := f.db.ExecContext(context.Background(), `UPDATE `+tc.table+` SET `+column+` = $1::jsonb`, `{"tier":"gold"}`)
				require.NoError(t, err)
			})
		}
	}
}

func objectMetadataColumns(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	columns, err := f.QuerySQL(t, `SELECT COALESCE(string_agg(table_name || '.' || column_name, ',' ORDER BY table_name, column_name), '')
		FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name IN ('labels', 'annotations')
		  AND table_name IN ('agents', 'thirdparty_oauth2_services', 'permission_sets')`)
	require.NoError(t, err)
	return columns
}

// holdReadLock keeps an open transaction with an ACCESS SHARE lock on table,
// as a live request would, until the returned function releases it.
func holdReadLock(t *testing.T, f *MigrationTestFramework, table string) func() {
	t.Helper()
	traffic, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	_, err = traffic.ExecContext(context.Background(), "SELECT id FROM "+table)
	require.NoError(t, err)
	released := false
	release := func() {
		if !released {
			released = true
			require.NoError(t, traffic.Rollback())
		}
	}
	t.Cleanup(release)
	return release
}

// boundLockWait makes a migration that queues for a lock fail after three
// seconds instead of waiting for the blocking transaction to end.
func boundLockWait(f *MigrationTestFramework) {
	separator := "?"
	if strings.Contains(f.connStr, "?") {
		separator = "&"
	}
	f.connStr += separator + "options=-c%20lock_timeout%3D3000"
}

func expectLiveReads(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, tc := range objectMetadataTables {
		_, err := f.db.ExecContext(ctx, "SELECT id FROM "+tc.table)
		require.NoError(t, err, "%s must stay readable while the migration is refused", tc.table)
	}
}

func TestObjectMetadataMigrationBusyTablesFailWithoutQueueing(t *testing.T) {
	for _, tc := range objectMetadataTables {
		t.Run("up/"+tc.table, func(t *testing.T) {
			f := objectMetadataMigrationFixture(t)
			before := objectMetadataMigrationData(t, f)
			boundLockWait(f)
			release := holdReadLock(t, f, tc.table)
			started := time.Now()
			err := f.Up(t, 51)
			require.ErrorContains(t, err, "could not obtain lock on relation", "a busy table must fail NOWAIT instead of queueing an exclusive lock")
			assert.Less(t, time.Since(started), 5*time.Second)
			assert.Empty(t, objectMetadataColumns(t, f), "contention must roll back every added column")
			assert.Equal(t, before, objectMetadataMigrationData(t, f))
			expectLiveReads(t, f)
			release()
			require.NoError(t, f.Force(t, 50))
			require.NoError(t, f.Up(t, 51))
		})
		t.Run("down/"+tc.table, func(t *testing.T) {
			f := objectMetadataMigrationFixture(t)
			require.NoError(t, f.Up(t, 51))
			columns := objectMetadataColumns(t, f)
			boundLockWait(f)
			release := holdReadLock(t, f, tc.table)
			err := f.Down(t, 50)
			require.ErrorContains(t, err, "could not obtain lock on relation")
			assert.Equal(t, columns, objectMetadataColumns(t, f), "contention must keep every column")
			expectLiveReads(t, f)
			release()
			require.NoError(t, f.Force(t, 51))
			require.NoError(t, f.Down(t, 50))
			assert.Empty(t, objectMetadataColumns(t, f))
		})
	}
}
