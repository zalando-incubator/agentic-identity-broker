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

const identityRevisionConstraints = `'chk_thirdparty_oauth2_services_client_identity', 'chk_thirdparty_oauth2_services_client_identity_revision', 'chk_user_sessions_upstream_client_revision'`

func seedIdentityRevisionRows(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('55000000-0000-0000-0000-000000000001', 'reports', 'Reports', 'reports-v1', decode('01020304', 'hex'), 'https://provider.example.test', '[]');
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context, initiated_at, callback_id)
		VALUES ('55000000-0000-0000-0000-000000000101', 'alice@example.test', '55000000-0000-0000-0000-000000000001',
			decode('11223344', 'hex'), decode('55667788', 'hex'), 'Bearer', ARRAY['read'],
			'{"service_id":"55000000-0000-0000-0000-000000000001"}', '2026-11-01 08:00:00+00', 'reports-callback');`))
}

func identityRevisionData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(to_jsonb(s) - 'client_identity' - 'client_identity_revision' ORDER BY id) FROM thirdparty_oauth2_services s),
		'sessions', (SELECT jsonb_agg(to_jsonb(s) - 'upstream_client_revision' ORDER BY id) FROM user_sessions s)
	)::text`)
	require.NoError(t, err)
	return data
}

func TestMigration047IdentityRevisionLifecycle(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 46))
	seedIdentityRevisionRows(t, f)
	before, schema := identityRevisionData(t, f), credentialMigrationSchema(t, f)
	heapBefore, err := f.QuerySQL(t, `SELECT jsonb_object_agg(relname, relfilenode)::text FROM pg_class WHERE oid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass)`)
	require.NoError(t, err)

	require.NoError(t, f.Up(t, 47))
	assert.Equal(t, before, identityRevisionData(t, f), "expansion must preserve ciphertext and existing row metadata")
	heapAfter, err := f.QuerySQL(t, `SELECT jsonb_object_agg(relname, relfilenode)::text FROM pg_class WHERE oid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass)`)
	require.NoError(t, err)
	assert.Equal(t, heapBefore, heapAfter, "metadata-only defaults must not rewrite tables")
	defaults, err := f.QuerySQL(t, `SELECT client_identity IS NULL AND client_identity_revision = 0 FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`)
	require.NoError(t, err)
	assert.Equal(t, "true", defaults)
	legacy, err := f.QuerySQL(t, `SELECT upstream_client_id IS NULL AND upstream_client_revision = 0 FROM user_sessions WHERE principal = 'alice@example.test'`)
	require.NoError(t, err)
	assert.Equal(t, "true", legacy, "migration must not infer the identity of legacy sessions")

	for _, version := range []uint{47, 48, 47} {
		if version == 48 {
			require.NoError(t, f.Up(t, version))
		} else {
			require.NoError(t, f.Down(t, version))
		}
		validated, err := f.QuerySQL(t, `SELECT count(*)::text || ':' || bool_and(convalidated)::text FROM pg_constraint WHERE conname IN (`+identityRevisionConstraints+`)`)
		require.NoError(t, err)
		if version == 48 {
			assert.Equal(t, "3:true", validated)
		} else {
			assert.Equal(t, "3:false", validated)
		}
		for _, query := range []string{
			`UPDATE thirdparty_oauth2_services SET client_identity = '' WHERE canonical_id = 'reports'`,
			`UPDATE thirdparty_oauth2_services SET client_identity_revision = -1 WHERE canonical_id = 'reports'`,
			`UPDATE user_sessions SET upstream_client_revision = -1 WHERE principal = 'alice@example.test'`,
		} {
			require.Error(t, f.ExecuteSQL(t, query), "both validated and NOT VALID constraints protect new writes")
		}
	}
	require.NoError(t, f.Down(t, 46))
	assert.Equal(t, schema, credentialMigrationSchema(t, f))
	assert.Equal(t, before, identityRevisionData(t, f))
	require.NoError(t, f.Up(t, 48))
	assert.Equal(t, before, identityRevisionData(t, f))
}

func TestMigration047ExplicitColumnLegacyWritersPreserveEvidence(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 48))
	seedIdentityRevisionRows(t, f)
	require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services
		SET client_identity = 'reports-v1', client_identity_revision = 3 WHERE canonical_id = 'reports';
		UPDATE user_sessions SET upstream_client_id = 'reports-v1', upstream_client_revision = 3
		WHERE principal = 'alice@example.test';
		UPDATE thirdparty_oauth2_services SET display_name = 'Legacy metadata update', version = version + 1
		WHERE canonical_id = 'reports';
		UPDATE user_sessions SET encrypted_access_token = decode('99aabb', 'hex'), updated_at = NOW()
		WHERE principal = 'alice@example.test';
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, token_type, scope, encryption_context, initiated_at, callback_id)
		VALUES ('55000000-0000-0000-0000-000000000102', 'bob@example.test', '55000000-0000-0000-0000-000000000001',
			decode('010203', 'hex'), 'Bearer', ARRAY['read'],
			'{"service_id":"55000000-0000-0000-0000-000000000001"}', NOW(), 'legacy-writer-callback');
		INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('55000000-0000-0000-0000-000000000002', 'Legacy registration', 'legacy-client', decode('010203', 'hex'),
			'https://provider.example.test', '[]');`))
	var clientID, displayName string
	require.NoError(t, f.db.QueryRowContext(context.Background(), `SELECT client_id, display_name FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`).Scan(&clientID, &displayName))
	assert.Equal(t, "reports-v1", clientID)
	assert.Equal(t, "Legacy metadata update", displayName)
	var token []byte
	require.NoError(t, f.db.QueryRowContext(context.Background(), `SELECT encrypted_access_token FROM user_sessions WHERE principal = 'alice@example.test'`).Scan(&token))
	assert.Equal(t, []byte{0x99, 0xaa, 0xbb}, token)
	for _, query := range []string{
		`SELECT client_identity = 'reports-v1' AND client_identity_revision = 3 FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`,
		`SELECT upstream_client_id = 'reports-v1' AND upstream_client_revision = 3 FROM user_sessions WHERE principal = 'alice@example.test'`,
		`SELECT upstream_client_id IS NULL AND upstream_client_revision = 0 FROM user_sessions WHERE principal = 'bob@example.test'`,
		`SELECT client_identity IS NULL AND client_identity_revision = 0 FROM thirdparty_oauth2_services WHERE display_name = 'Legacy registration'`,
	} {
		preserved, err := f.QuerySQL(t, query)
		require.NoError(t, err)
		assert.Equal(t, "true", preserved)
	}
}

func TestMigration047DownRetainsInvalidationEvidence(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 48))
	seedIdentityRevisionRows(t, f)
	require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET client_identity = 'reports-v2', client_identity_revision = 1 WHERE canonical_id = 'reports'`))
	require.NoError(t, f.Down(t, 47))
	before, schema := credentialMigrationFullData(t, f), credentialMigrationSchema(t, f)
	require.ErrorContains(t, f.Down(t, 46), "client identity revisions", "DOWN must refuse to discard evidence that prevents old-session revival")
	assert.Equal(t, before, credentialMigrationFullData(t, f))
	assert.Equal(t, schema, credentialMigrationSchema(t, f))
}

func TestMigration047ExpansionNeverQueuesBehindTraffic(t *testing.T) {
	for _, table := range []string{"thirdparty_oauth2_services", "user_sessions"} {
		t.Run(table, func(t *testing.T) {
			f := NewMigrationTestFramework(t)
			defer f.Cleanup(t)
			require.NoError(t, f.Up(t, 46))
			seedIdentityRevisionRows(t, f)
			before, schema := identityRevisionData(t, f), credentialMigrationSchema(t, f)
			traffic, err := f.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = traffic.Rollback() }()
			_, err = traffic.ExecContext(context.Background(), "SELECT id FROM "+table)
			require.NoError(t, err)
			f.connStr += "&options=-c%20statement_timeout%3D1000"
			require.ErrorContains(t, f.Up(t, 47), "could not obtain lock on relation")
			assert.Equal(t, schema, credentialMigrationSchema(t, f), "failure must roll back both tables atomically")
			assert.Equal(t, before, identityRevisionData(t, f))
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, err = f.db.ExecContext(ctx, `UPDATE user_sessions SET scope = ARRAY['read', 'write'] WHERE principal = 'alice@example.test'`)
			require.NoError(t, err, "failed expansion must leave ordinary traffic writable")
		})
	}
}

func TestMigration048ValidationCoexistsWithTrafficAndFailsOnMaintenanceContention(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 47))
	seedIdentityRevisionRows(t, f)
	traffic, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = traffic.Rollback() }()
	_, err = traffic.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services SET display_name = display_name; UPDATE user_sessions SET scope = scope`)
	require.NoError(t, err)
	f.connStr += "&options=-c%20statement_timeout%3D1000"
	require.NoError(t, f.Up(t, 48), "validation locks must permit ordinary writers")
	require.NoError(t, traffic.Rollback())
	require.NoError(t, f.Down(t, 47))

	validationSQL, err := os.ReadFile(filepath.Join(f.migrationsDir, "048_validate_client_identity_revision.up.sql"))
	require.NoError(t, err)
	validation, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = validation.Rollback() }()
	_, err = validation.ExecContext(context.Background(), string(validationSQL))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = f.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET display_name = 'Still serving'; UPDATE user_sessions SET scope = ARRAY['read', 'write']`)
	require.NoError(t, err, "validation must leave both runtime tables writable")
	before, schema := credentialMigrationFullData(t, f), credentialMigrationSchema(t, f)
	require.ErrorContains(t, f.Up(t, 48), "could not obtain lock on relation", "concurrent maintenance must fail NOWAIT")
	assert.Equal(t, before, credentialMigrationFullData(t, f))
	assert.Equal(t, schema, credentialMigrationSchema(t, f))
}
