//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serviceIconMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "service_icons_48", func(t *testing.T, name string) {
		pg.ApplyMigrationsUpTo(t, name, 48)
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
		VALUES ('49000000-0000-0000-0000-000000000001', 'atlas-files', 'Atlas Files', 'atlas-client', decode('01020304','hex'),
		 'https://atlas.example.test', false, 'https://atlas.example.test/token', 'https://atlas.example.test/authorize',
		 '[{"scope_value":"read","description":"Read files"}]', '{"audience":"atlas"}', 7, '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO service_protected_resources (service_id, resource_uri, created_at)
		VALUES ('49000000-0000-0000-0000-000000000001', 'https://api.atlas.example.test/files', '2020-01-02 03:04:05');
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context,
		 initiated_at, created_at, updated_at, callback_id)
		VALUES ('49000000-0000-0000-0000-000000000002', 'alice@example.test', '49000000-0000-0000-0000-000000000001',
		 decode('11223344','hex'), decode('55667788','hex'), 'Bearer', ARRAY['read'],
		 '{"service_id":"49000000-0000-0000-0000-000000000001"}', '2020-01-02 03:04:05+00',
		 '2020-01-02 03:04:05+00', '2021-02-03 04:05:06+00', 'original-callback');`))
	return f
}

func serviceIconMigrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(jsonb_build_array(id, canonical_id, display_name, client_id,
		 client_secret_encrypted, token_endpoint_auth_method, client_registration, credential_source,
		 credential_source_transitioned, oauth2_flavor, issuer_uri, enable_discovery, metadata_url, resource_url,
		 client_method, resource_explicit, discovery_last_attempt_at, discovery_last_success_at, discovery_failure_reason,
		 token_endpoint, authorize_endpoint, scopes, authorization_params, created_at, updated_at, version) ORDER BY id)
		 FROM thirdparty_oauth2_services),
		'resources', (SELECT jsonb_agg(jsonb_build_array(service_id, resource_uri, created_at) ORDER BY resource_uri)
		 FROM service_protected_resources),
		'sessions', (SELECT jsonb_agg(jsonb_build_array(id, principal, service_id, encrypted_access_token,
		 encrypted_refresh_token, token_type, scope, encryption_context, initiated_at, created_at, updated_at, callback_id) ORDER BY id)
		 FROM user_sessions)
	)::text`)
	require.NoError(t, err)
	return data
}

func TestServiceIconsMigrationUpDownUpPreservesExistingData(t *testing.T) {
	f := serviceIconMigrationFixture(t)
	before := serviceIconMigrationData(t, f)
	require.NoError(t, f.Up(t, 49))
	assert.Equal(t, before, serviceIconMigrationData(t, f))
	columnType, err := f.GetColumnType(t, "thirdparty_oauth2_services", "icon_svg")
	require.NoError(t, err)
	assert.Equal(t, "text", columnType)
	shape, err := f.QuerySQL(t, `SELECT is_nullable = 'YES' AND column_default IS NULL
		FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'thirdparty_oauth2_services' AND column_name = 'icon_svg'`)
	require.NoError(t, err)
	assert.Equal(t, "true", shape)
	absent, err := f.QuerySQL(t, `SELECT icon_svg IS NULL FROM thirdparty_oauth2_services`)
	require.NoError(t, err)
	assert.Equal(t, "true", absent, "old rows must remain absent without a backfill")

	const source = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n<svg xmlns=\"http://www.w3.org/2000/svg\"><text>Ä &amp; 東京 🗺</text></svg>\n"
	_, err = f.db.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services SET icon_svg=$1`, source)
	require.NoError(t, err)
	stored, err := f.QuerySQL(t, `SELECT icon_svg FROM thirdparty_oauth2_services`)
	require.NoError(t, err)
	assert.Equal(t, []byte(source), []byte(stored))
	assert.Equal(t, before, serviceIconMigrationData(t, f))
	require.NoError(t, f.Down(t, 48))
	exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", "icon_svg")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, before, serviceIconMigrationData(t, f), "rollback loses only icon metadata")
	require.NoError(t, f.Up(t, 49))
	assert.Equal(t, before, serviceIconMigrationData(t, f))
	absent, err = f.QuerySQL(t, `SELECT icon_svg IS NULL FROM thirdparty_oauth2_services`)
	require.NoError(t, err)
	assert.Equal(t, "true", absent)
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(49), version)
	assert.False(t, dirty)
}

func TestServiceIconsMigrationEnforcesDecodedByteShapeOnly(t *testing.T) {
	f := serviceIconMigrationFixture(t)
	require.NoError(t, f.Up(t, 49))
	for _, tc := range []struct {
		name  string
		value any
		valid bool
	}{
		{name: "null", value: nil, valid: true},
		{name: "one byte", value: "x", valid: true},
		{name: "ASCII boundary", value: strings.Repeat("x", 65536), valid: true},
		{name: "UTF8 boundary", value: strings.Repeat("界", 21845) + "x", valid: true},
		{name: "not XML is application concern", value: "not an SVG document", valid: true},
		{name: "empty", value: ""},
		{name: "ASCII over limit", value: strings.Repeat("x", 65537)},
		{name: "UTF8 over limit", value: strings.Repeat("界", 21845) + "xx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := f.QuerySQL(t, `SELECT COALESCE(icon_svg, '<absent>') FROM thirdparty_oauth2_services`)
			require.NoError(t, err)
			_, err = f.db.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services SET icon_svg=$1`, tc.value)
			if tc.valid {
				require.NoError(t, err)
				var value sql.NullString
				require.NoError(t, f.db.QueryRowContext(context.Background(), `SELECT icon_svg FROM thirdparty_oauth2_services`).Scan(&value))
				if tc.value == nil {
					assert.False(t, value.Valid)
				} else {
					assert.True(t, value.Valid)
					assert.Equal(t, []byte(tc.value.(string)), []byte(value.String))
				}
				return
			}
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, "23514", pgErr.Code)
			assert.Equal(t, "chk_thirdparty_oauth2_services_icon_svg", pgErr.ConstraintName)
			after, err := f.QuerySQL(t, `SELECT COALESCE(icon_svg, '<absent>') FROM thirdparty_oauth2_services`)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestServiceIconsMigrationDDLFailureIsAtomic(t *testing.T) {
	f := serviceIconMigrationFixture(t)
	before := serviceIconMigrationData(t, f)
	require.NoError(t, f.ExecuteSQL(t, `ALTER TABLE thirdparty_oauth2_services ADD CONSTRAINT chk_thirdparty_oauth2_services_icon_svg CHECK (true)`))
	require.Error(t, f.Up(t, 49), "a conflicting constraint must roll back the added column")
	exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", "icon_svg")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, before, serviceIconMigrationData(t, f))
	require.NoError(t, f.Force(t, 48))
	require.NoError(t, f.ExecuteSQL(t, `ALTER TABLE thirdparty_oauth2_services DROP CONSTRAINT chk_thirdparty_oauth2_services_icon_svg`))
	require.NoError(t, f.Up(t, 49))
	require.NoError(t, f.ExecuteSQL(t, `CREATE VIEW icon_migration_dependency AS SELECT icon_svg FROM thirdparty_oauth2_services`))
	require.Error(t, f.Down(t, 48), "a dependent view must roll back constraint removal with the column drop")
	exists, err = f.ColumnExists(t, "thirdparty_oauth2_services", "icon_svg")
	require.NoError(t, err)
	assert.True(t, exists)
	constraint, err := f.QuerySQL(t, `SELECT EXISTS (SELECT 1 FROM pg_constraint
		WHERE conrelid = 'thirdparty_oauth2_services'::regclass AND conname = 'chk_thirdparty_oauth2_services_icon_svg')`)
	require.NoError(t, err)
	assert.Equal(t, "true", constraint)
	assert.Equal(t, before, serviceIconMigrationData(t, f))
	require.NoError(t, f.Force(t, 49))
	require.NoError(t, f.ExecuteSQL(t, `DROP VIEW icon_migration_dependency`))
	require.NoError(t, f.Down(t, 48))
	require.NoError(t, f.Up(t, 49))
	assert.Equal(t, before, serviceIconMigrationData(t, f))
}
