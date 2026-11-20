//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const identityRevisionConstraints = `'chk_thirdparty_oauth2_services_client_identity', 'chk_thirdparty_oauth2_services_client_identity_revision', 'chk_user_sessions_upstream_client_revision'`

func newIdentityRevisionMigrationFramework(t *testing.T, version uint) *MigrationTestFramework {
	t.Helper()
	postgres := bootstrap.RequireSharedPostgres(t)
	dbName, connStr, cleanup := postgres.SetupDatabaseFromTemplate(t, fmt.Sprintf("client_identity_revision_%d", version), func(t *testing.T, dbName string) {
		postgres.ApplyMigrationsUpTo(t, dbName, version)
	})
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	require.NoError(t, err)
	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(context.Background()))
	return &MigrationTestFramework{db: db, connStr: connStr, dbName: dbName, migrationsDir: migrationsDir, cleanup: cleanup}
}

func seedIdentityRevisionRows(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('55000000-0000-0000-0000-000000000001', 'reports', 'Reports', 'reports-v1', decode('01020304', 'hex'), 'https://provider.example.test', '[]');
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context, initiated_at, callback_id, upstream_client_id)
		VALUES ('55000000-0000-0000-0000-000000000101', 'alice@example.test', '55000000-0000-0000-0000-000000000001',
			decode('11223344', 'hex'), decode('55667788', 'hex'), 'Bearer', ARRAY['read'],
			'{"service_id":"55000000-0000-0000-0000-000000000001"}', '2026-11-01 08:00:00+00', 'reports-callback', NULL),
			('55000000-0000-0000-0000-000000000103', 'known@example.test', '55000000-0000-0000-0000-000000000001',
			decode('aabbccdd', 'hex'), decode('eeff0011', 'hex'), 'Bearer', ARRAY['read'],
			'{"service_id":"55000000-0000-0000-0000-000000000001"}', '2026-11-01 08:00:00+00', 'known-callback', 'reports-v1');`))
}

func identityRevisionData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `WITH services AS (
		SELECT id, canonical_id, display_name, client_id, client_secret_encrypted,
			token_endpoint_auth_method, client_registration, credential_source, credential_source_transitioned,
			oauth2_flavor, issuer_uri, enable_discovery, metadata_url, resource_url, client_method, resource_explicit,
			discovery_last_attempt_at, discovery_last_success_at, discovery_failure_reason,
			token_endpoint, authorize_endpoint, scopes, authorization_params, created_at, updated_at, version
		FROM thirdparty_oauth2_services
	), sessions AS (
		SELECT id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
			access_token_expires_at, refresh_token_expires_at, token_type, scope, encryption_context,
			initiated_at, created_at, updated_at, callback_id, upstream_client_id
		FROM user_sessions
	) SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM services s),
		'sessions', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s)
	)::text`)
	require.NoError(t, err)
	return data
}

func identityRevisionFullData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	evidence, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(jsonb_build_object('id', id, 'identity', client_identity, 'revision', client_identity_revision) ORDER BY id) FROM thirdparty_oauth2_services),
		'sessions', (SELECT jsonb_agg(jsonb_build_object('id', id, 'revision', upstream_client_revision) ORDER BY id) FROM user_sessions)
	)::text`)
	require.NoError(t, err)
	return identityRevisionData(t, f) + evidence
}

func requireIdentityRevisionValidation(t *testing.T, f *MigrationTestFramework, validated bool) {
	t.Helper()
	state, err := f.QuerySQL(t, `SELECT count(*)::text || ':' || bool_and(convalidated)::text || ':' || bool_or(convalidated)::text
		FROM pg_constraint WHERE conname IN (`+identityRevisionConstraints+`)`)
	require.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("3:%t:%t", validated, validated), state)
}

func requireIdentityRevisionMigrationVersion(t *testing.T, f *MigrationTestFramework, expected uint, expectedDirty bool) {
	t.Helper()
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, expected, version)
	assert.Equal(t, expectedDirty, dirty)
}

func requireIdentityRevisionTraffic(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var clientID string
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT client_id FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`).Scan(&clientID))
	assert.Equal(t, "reports-v1", clientID)
	var token []byte
	require.NoError(t, f.db.QueryRowContext(ctx, `SELECT encrypted_access_token FROM user_sessions WHERE principal = 'alice@example.test'`).Scan(&token))
	assert.Equal(t, []byte{0x11, 0x22, 0x33, 0x44}, token)
	_, err := f.db.ExecContext(ctx, `UPDATE thirdparty_oauth2_services SET display_name = 'Still serving' WHERE canonical_id = 'reports';
		UPDATE user_sessions SET scope = ARRAY['read', 'write'] WHERE principal = 'alice@example.test'`)
	require.NoError(t, err, "migration locks must leave both runtime tables readable and writable")
}

func requireIdentityRevisionChecks(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	for _, tc := range []struct {
		query string
		code  string
	}{
		{`UPDATE thirdparty_oauth2_services SET client_identity = '' WHERE canonical_id = 'reports'`, "23514"},
		{`UPDATE thirdparty_oauth2_services SET client_identity_revision = -1 WHERE canonical_id = 'reports'`, "23514"},
		{`UPDATE user_sessions SET upstream_client_revision = -1 WHERE principal = 'alice@example.test'`, "23514"},
		{`UPDATE thirdparty_oauth2_services SET client_identity_revision = NULL WHERE canonical_id = 'reports'`, "23502"},
		{`UPDATE user_sessions SET upstream_client_revision = NULL WHERE principal = 'alice@example.test'`, "23502"},
	} {
		err := f.ExecuteSQL(t, tc.query)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr, "validated and NOT VALID constraints must protect new writes: %s", tc.query)
		assert.Equal(t, tc.code, pgErr.Code)
	}
}

func TestMigration049IdentityRevisionLifecycle(t *testing.T) {
	f := newIdentityRevisionMigrationFramework(t, 48)
	defer f.Cleanup(t)
	seedIdentityRevisionRows(t, f)
	before, schema := identityRevisionData(t, f), credentialMigrationSchema(t, f)
	heapBefore, err := f.QuerySQL(t, `SELECT jsonb_object_agg(relname, relfilenode)::text FROM pg_class WHERE oid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass)`)
	require.NoError(t, err)

	require.NoError(t, f.Up(t, 49))
	requireIdentityRevisionMigrationVersion(t, f, 49, false)
	requireIdentityRevisionValidation(t, f, false)
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
	known, err := f.QuerySQL(t, `SELECT upstream_client_id = 'reports-v1' AND upstream_client_revision = 0 FROM user_sessions WHERE principal = 'known@example.test'`)
	require.NoError(t, err)
	assert.Equal(t, "true", known, "already recorded identities must remain intact")
	require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET client_identity = 'reports-v1' WHERE canonical_id = 'reports'`))
	expanded := identityRevisionFullData(t, f)

	for _, version := range []uint{50, 49, 50, 49} {
		if version == 50 {
			require.NoError(t, f.Up(t, version))
		} else {
			require.NoError(t, f.Down(t, version))
		}
		requireIdentityRevisionMigrationVersion(t, f, version, false)
		requireIdentityRevisionValidation(t, f, version == 50)
		requireIdentityRevisionChecks(t, f)
		assert.Equal(t, expanded, identityRevisionFullData(t, f), "validation, unvalidation and rejected writes must not mutate rows")
	}
	require.NoError(t, f.Down(t, 48))
	requireIdentityRevisionMigrationVersion(t, f, 48, false)
	assert.Equal(t, schema, credentialMigrationSchema(t, f))
	assert.Equal(t, before, identityRevisionData(t, f))
	require.NoError(t, f.Up(t, 50))
	requireIdentityRevisionMigrationVersion(t, f, 50, false)
	requireIdentityRevisionValidation(t, f, true)
	reapplied, err := f.QuerySQL(t, `SELECT client_identity IS NULL AND client_identity_revision = 0 FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`)
	require.NoError(t, err)
	assert.Equal(t, "true", reapplied, "reapplication must not backfill first-observation evidence")
	assert.Equal(t, before, identityRevisionData(t, f))
}

func TestMigration049ColumnDefinitions(t *testing.T) {
	f := newIdentityRevisionMigrationFramework(t, 49)
	defer f.Cleanup(t)
	columns, err := f.QuerySQL(t, `SELECT jsonb_agg(jsonb_build_array(table_name, column_name, data_type, is_nullable, column_default) ORDER BY table_name, column_name)::text
		FROM information_schema.columns WHERE table_schema = 'public'
		AND ((table_name = 'thirdparty_oauth2_services' AND column_name IN ('client_identity', 'client_identity_revision'))
			OR (table_name = 'user_sessions' AND column_name = 'upstream_client_revision'))`)
	require.NoError(t, err)
	assert.JSONEq(t, `[["thirdparty_oauth2_services","client_identity","text","YES",null],
		["thirdparty_oauth2_services","client_identity_revision","bigint","NO","0"],
		["user_sessions","upstream_client_revision","bigint","NO","0"]]`, columns)
}

func TestMigration049ExplicitColumnLegacyWritersPreserveEvidence(t *testing.T) {
	for _, version := range []uint{49, 50} {
		t.Run(fmt.Sprintf("schema_%d", version), func(t *testing.T) {
			f := newIdentityRevisionMigrationFramework(t, version)
			defer f.Cleanup(t)
			seedIdentityRevisionRows(t, f)
			require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services
				SET client_identity = 'reports-v1', client_identity_revision = 3 WHERE canonical_id = 'reports';
				UPDATE user_sessions SET upstream_client_id = 'reports-v1', upstream_client_revision = 3
				WHERE principal = 'alice@example.test'`))
			legacyRead := identityRevisionData(t, f)
			assert.Contains(t, legacyRead, "reports-v1", "the pre-expansion explicit-column service projection must still read")
			assert.Contains(t, legacyRead, "alice@example.test", "the pre-expansion explicit-column session projection must still read")
			require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services
				SET display_name = 'Legacy metadata update', client_secret_encrypted = decode('cafebabe', 'hex'), version = version + 1
				WHERE canonical_id = 'reports';
				UPDATE user_sessions SET encrypted_access_token = decode('99aabb', 'hex'),
					encrypted_refresh_token = decode('ccddee', 'hex'), updated_at = NOW()
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
			var secret, token, refreshToken []byte
			require.NoError(t, f.db.QueryRowContext(context.Background(), `SELECT client_id, display_name, client_secret_encrypted
				FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`).Scan(&clientID, &displayName, &secret))
			assert.Equal(t, "reports-v1", clientID)
			assert.Equal(t, "Legacy metadata update", displayName)
			assert.Equal(t, []byte{0xca, 0xfe, 0xba, 0xbe}, secret)
			require.NoError(t, f.db.QueryRowContext(context.Background(), `SELECT encrypted_access_token, encrypted_refresh_token
				FROM user_sessions WHERE principal = 'alice@example.test'`).Scan(&token, &refreshToken))
			assert.Equal(t, []byte{0x99, 0xaa, 0xbb}, token)
			assert.Equal(t, []byte{0xcc, 0xdd, 0xee}, refreshToken)
			for _, query := range []string{
				`SELECT client_identity = 'reports-v1' AND client_identity_revision = 3 FROM thirdparty_oauth2_services WHERE canonical_id = 'reports'`,
				`SELECT upstream_client_id = 'reports-v1' AND upstream_client_revision = 3 FROM user_sessions WHERE principal = 'alice@example.test'`,
				`SELECT upstream_client_id IS NULL AND upstream_client_revision = 0 FROM user_sessions WHERE principal = 'bob@example.test'`,
				`SELECT client_identity IS NULL AND client_identity_revision = 0 FROM thirdparty_oauth2_services WHERE display_name = 'Legacy registration'`,
				`SELECT upstream_client_id = 'reports-v1' AND upstream_client_revision = 0 AND encrypted_access_token = decode('aabbccdd', 'hex')
					AND encrypted_refresh_token = decode('eeff0011', 'hex') FROM user_sessions WHERE principal = 'known@example.test'`,
			} {
				preserved, err := f.QuerySQL(t, query)
				require.NoError(t, err)
				assert.Equal(t, "true", preserved, query)
			}
			requireIdentityRevisionValidation(t, f, version == 50)
			requireIdentityRevisionMigrationVersion(t, f, version, false)
		})
	}
}

func TestMigration049DownRetainsInvalidationEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"changed_identity", `UPDATE thirdparty_oauth2_services SET client_identity = 'reports-v2', client_identity_revision = 1 WHERE canonical_id = 'reports'`},
		{"restored_identity", `UPDATE thirdparty_oauth2_services SET client_identity = client_id, client_identity_revision = 2 WHERE canonical_id = 'reports'`},
		{"unreferenced_service", `INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes, client_identity_revision)
			VALUES ('55000000-0000-0000-0000-000000000002', 'Unreferenced', 'other-client', decode('010203', 'hex'), 'https://provider.example.test', '[]', 9223372036854775807)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newIdentityRevisionMigrationFramework(t, 50)
			defer f.Cleanup(t)
			seedIdentityRevisionRows(t, f)
			require.NoError(t, f.ExecuteSQL(t, tc.query))
			before := identityRevisionFullData(t, f)
			require.NoError(t, f.Down(t, 49))
			assert.Equal(t, before, identityRevisionFullData(t, f), "DOWN 050 must retain positive revision evidence")
			schema := credentialMigrationSchema(t, f)
			require.ErrorContains(t, f.Down(t, 48), "client identity revisions", "DOWN must refuse to discard evidence that prevents old-session revival")
			assert.Equal(t, before, identityRevisionFullData(t, f))
			assert.Equal(t, schema, credentialMigrationSchema(t, f))
			requireIdentityRevisionValidation(t, f, false)
			requireIdentityRevisionMigrationVersion(t, f, 48, true)
			require.ErrorContains(t, f.Up(t, 50), "Dirty database version", "guard failure must not be silently retried")
		})
	}
}

func TestMigration049And050ExclusiveMigrationsNeverQueueBehindTraffic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		from   uint
		target uint
	}{
		{"049_up", 48, 49},
		{"049_down", 49, 48},
		{"050_down", 50, 49},
	} {
		for _, table := range []string{"thirdparty_oauth2_services", "user_sessions"} {
			t.Run(tc.name+"/"+table, func(t *testing.T) {
				f := newIdentityRevisionMigrationFramework(t, tc.from)
				defer f.Cleanup(t)
				seedIdentityRevisionRows(t, f)
				var evidence string
				if tc.from >= 49 {
					require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET client_identity = client_id WHERE canonical_id = 'reports'`))
					evidence = identityRevisionFullData(t, f)
				}
				before, schema := identityRevisionData(t, f), credentialMigrationSchema(t, f)
				traffic, err := f.db.BeginTx(context.Background(), nil)
				require.NoError(t, err)
				defer func() { _ = traffic.Rollback() }()
				_, err = traffic.ExecContext(context.Background(), "SELECT id FROM "+table)
				require.NoError(t, err)
				f.connStr += "&options=-c%20statement_timeout%3D1000"
				if tc.target > tc.from {
					err = f.Up(t, tc.target)
				} else {
					err = f.Down(t, tc.target)
				}
				require.ErrorContains(t, err, "could not obtain lock on relation", "contention must fail NOWAIT rather than queue until timeout")
				assert.Equal(t, schema, credentialMigrationSchema(t, f), "failure must roll back both tables atomically")
				assert.Equal(t, before, identityRevisionData(t, f))
				if tc.from >= 49 {
					assert.Equal(t, evidence, identityRevisionFullData(t, f))
				}
				requireIdentityRevisionMigrationVersion(t, f, tc.target, true)
				requireIdentityRevisionTraffic(t, f)
			})
		}
	}
}

func TestMigration050ValidationCoexistsWithTraffic(t *testing.T) {
	f := newIdentityRevisionMigrationFramework(t, 49)
	defer f.Cleanup(t)
	seedIdentityRevisionRows(t, f)
	before := identityRevisionFullData(t, f)
	traffic, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = traffic.Rollback() }()
	_, err = traffic.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services SET display_name = display_name; UPDATE user_sessions SET scope = scope`)
	require.NoError(t, err)
	f.connStr += "&options=-c%20statement_timeout%3D1000"
	require.NoError(t, f.Up(t, 50), "validation locks must permit ordinary writers")
	require.NoError(t, traffic.Rollback())
	requireIdentityRevisionValidation(t, f, true)
	assert.Equal(t, before, identityRevisionFullData(t, f))
	require.NoError(t, f.Down(t, 49))
	requireIdentityRevisionValidation(t, f, false)
	assert.Equal(t, before, identityRevisionFullData(t, f))

	validationSQL, err := os.ReadFile(filepath.Join(f.migrationsDir, "050_validate_client_identity_revision.up.sql"))
	require.NoError(t, err)
	validation, err := f.db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = validation.Rollback() }()
	_, err = validation.ExecContext(context.Background(), string(validationSQL))
	require.NoError(t, err)
	requireIdentityRevisionTraffic(t, f)
	require.NoError(t, validation.Rollback())
	requireIdentityRevisionValidation(t, f, false)
	before = identityRevisionFullData(t, f)
	require.NoError(t, f.Up(t, 50))
	requireIdentityRevisionValidation(t, f, true)
	requireIdentityRevisionMigrationVersion(t, f, 50, false)
	assert.Equal(t, before, identityRevisionFullData(t, f))
}

func TestMigration050ValidationNeverQueuesBehindMaintenance(t *testing.T) {
	for _, table := range []string{"thirdparty_oauth2_services", "user_sessions"} {
		t.Run(table, func(t *testing.T) {
			f := newIdentityRevisionMigrationFramework(t, 49)
			defer f.Cleanup(t)
			seedIdentityRevisionRows(t, f)
			before, schema := identityRevisionFullData(t, f), credentialMigrationSchema(t, f)
			maintenance, err := f.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = maintenance.Rollback() }()
			_, err = maintenance.ExecContext(context.Background(), "LOCK TABLE "+table+" IN SHARE UPDATE EXCLUSIVE MODE")
			require.NoError(t, err)
			f.connStr += "&options=-c%20statement_timeout%3D1000"
			require.ErrorContains(t, f.Up(t, 50), "could not obtain lock on relation", "either maintenance lock must abort validation NOWAIT")
			assert.Equal(t, before, identityRevisionFullData(t, f))
			assert.Equal(t, schema, credentialMigrationSchema(t, f))
			requireIdentityRevisionValidation(t, f, false)
			requireIdentityRevisionMigrationVersion(t, f, 50, true)
			requireIdentityRevisionTraffic(t, f)
		})
	}
}

func TestMigration050ValidationFailureRollsBackEveryConstraint(t *testing.T) {
	for _, tc := range []struct {
		name       string
		table      string
		constraint string
		assignment string
		check      string
	}{
		{"empty_identity", "thirdparty_oauth2_services", "chk_thirdparty_oauth2_services_client_identity", "client_identity = ''", "client_identity IS NULL OR client_identity <> ''"},
		{"negative_service_revision", "thirdparty_oauth2_services", "chk_thirdparty_oauth2_services_client_identity_revision", "client_identity_revision = -1", "client_identity_revision >= 0"},
		{"negative_session_revision", "user_sessions", "chk_user_sessions_upstream_client_revision", "upstream_client_revision = -1", "upstream_client_revision >= 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newIdentityRevisionMigrationFramework(t, 49)
			defer f.Cleanup(t)
			seedIdentityRevisionRows(t, f)
			// Model a bad historical row; NOT VALID must still reject it when 050 scans.
			require.NoError(t, f.ExecuteSQL(t, fmt.Sprintf(`ALTER TABLE %s DROP CONSTRAINT %s;
				UPDATE %s SET %s;
				ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s) NOT VALID`,
				tc.table, tc.constraint, tc.table, tc.assignment, tc.table, tc.constraint, tc.check)))
			before, schema := identityRevisionFullData(t, f), credentialMigrationSchema(t, f)
			require.ErrorContains(t, f.Up(t, 50), tc.constraint)
			assert.Equal(t, before, identityRevisionFullData(t, f), "failed validation must not repair or delete legacy rows")
			assert.Equal(t, schema, credentialMigrationSchema(t, f), "later constraint failure must roll back earlier validations")
			requireIdentityRevisionValidation(t, f, false)
			requireIdentityRevisionMigrationVersion(t, f, 50, true)
			require.ErrorContains(t, f.Up(t, 50), "Dirty database version", "failed validation requires operator inspection, not an automatic retry")
		})
	}
}

func TestMigration049And050TimeoutsRollBackPartialDDL(t *testing.T) {
	for _, tc := range []struct {
		name   string
		from   uint
		target uint
	}{
		{"049_up", 48, 49},
		{"049_down", 49, 48},
		{"050_up", 49, 50},
		{"050_down", 50, 49},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newIdentityRevisionMigrationFramework(t, tc.from)
			defer f.Cleanup(t)
			seedIdentityRevisionRows(t, f)
			before, schema := identityRevisionData(t, f), credentialMigrationSchema(t, f)
			var evidence string
			if tc.from >= 49 {
				require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET client_identity = client_id WHERE canonical_id = 'reports'`))
				evidence = identityRevisionFullData(t, f)
			}
			blocker, err := f.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer func() { _ = blocker.Rollback() }()
			_, err = blocker.ExecContext(context.Background(), `SELECT pg_advisory_xact_lock(49050)`)
			require.NoError(t, err)
			// Block only after the second table's ALTER, so cancellation must undo partial DDL on both tables.
			require.NoError(t, f.ExecuteSQL(t, `CREATE FUNCTION block_identity_revision_ddl() RETURNS event_trigger LANGUAGE plpgsql AS $function$
				BEGIN
					IF EXISTS (SELECT 1 FROM pg_event_trigger_ddl_commands() WHERE objid = 'user_sessions'::regclass) THEN
						IF current_setting('statement_timeout') <> '1s' THEN
							RAISE EXCEPTION 'Expected a one-second migration statement timeout';
						END IF;
						PERFORM pg_advisory_xact_lock(49050);
					END IF;
				END
				$function$;
				CREATE EVENT TRIGGER block_identity_revision_ddl ON ddl_command_end
					WHEN TAG IN ('ALTER TABLE') EXECUTE FUNCTION block_identity_revision_ddl();`))
			if tc.name == "050_up" {
				f.connStr += "&options=-c%20statement_timeout%3D1000"
			} else {
				f.connStr += "&options=-c%20statement_timeout%3D5000"
			}
			if tc.target > tc.from {
				err = f.Up(t, tc.target)
			} else {
				err = f.Down(t, tc.target)
			}
			require.ErrorContains(t, err, "canceling statement due to statement timeout")
			assert.Equal(t, schema, credentialMigrationSchema(t, f), "timed-out DDL must leave column and constraint definitions unchanged")
			assert.Equal(t, before, identityRevisionData(t, f), "ciphertext and legacy metadata must survive cancellation")
			if tc.from >= 49 {
				assert.Equal(t, evidence, identityRevisionFullData(t, f))
				requireIdentityRevisionValidation(t, f, tc.from == 50)
			}
			requireIdentityRevisionMigrationVersion(t, f, tc.target, true)
			require.ErrorContains(t, f.Up(t, tc.target), "Dirty database version")
			requireIdentityRevisionTraffic(t, f)
		})
	}
}
