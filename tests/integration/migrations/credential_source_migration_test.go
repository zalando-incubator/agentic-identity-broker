//go:build integration

package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Snapshot complete persisted rows, excluding only columns introduced by 036.
// This observes ciphertext, timestamps, versions, modes, indexes' child data and associations.
func credentialMigrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT COALESCE(jsonb_agg(to_jsonb(s) - 'credential_source' - 'credential_source_transitioned' ORDER BY id), '[]'::jsonb) FROM thirdparty_oauth2_services s),
		'sessions', (SELECT COALESCE(jsonb_agg(to_jsonb(u) - 'upstream_client_id' ORDER BY id), '[]'::jsonb) FROM user_sessions u),
		'resources', (SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY resource_uri), '[]'::jsonb) FROM service_protected_resources r)
	)::text`)
	require.NoError(t, err)
	return data
}

func credentialMigrationFullData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY id), '[]'::jsonb) FROM thirdparty_oauth2_services s),
		'sessions', (SELECT COALESCE(jsonb_agg(to_jsonb(u) ORDER BY id), '[]'::jsonb) FROM user_sessions u),
		'resources', (SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY resource_uri), '[]'::jsonb) FROM service_protected_resources r)
	)::text`)
	require.NoError(t, err)
	return data
}

func credentialMigrationSchema(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	schema, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'columns', (SELECT jsonb_agg(jsonb_build_array(table_name, column_name, data_type, is_nullable, column_default, character_maximum_length) ORDER BY table_name, ordinal_position)
			FROM information_schema.columns WHERE table_schema = 'public' AND table_name IN ('thirdparty_oauth2_services', 'user_sessions', 'service_protected_resources')),
		'constraints', (SELECT jsonb_agg(jsonb_build_array(conrelid::regclass::text, conname, pg_get_constraintdef(oid)) ORDER BY conrelid::regclass::text, conname)
			FROM pg_constraint WHERE conrelid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass, 'service_protected_resources'::regclass)),
		'indexes', (SELECT jsonb_agg(jsonb_build_array(tablename, indexname, indexdef) ORDER BY tablename, indexname)
			FROM pg_indexes WHERE schemaname = 'public' AND tablename IN ('thirdparty_oauth2_services', 'user_sessions', 'service_protected_resources')),
		'triggers', (SELECT jsonb_agg(pg_get_triggerdef(oid) ORDER BY tgname)
			FROM pg_trigger WHERE NOT tgisinternal AND tgrelid IN ('thirdparty_oauth2_services'::regclass, 'user_sessions'::regclass))
	)::text`)
	require.NoError(t, err)
	return schema
}

func seedCredentialMigrationLegacyRows(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	// Non-secret opaque bytes exercise migration preservation, not cryptographic validity.
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, oauth2_flavor, issuer_uri, enable_discovery, scopes, version, created_at, updated_at)
		VALUES
		('51000000-0000-0000-0000-000000000001', 'legacy.Standard', 'Shared secret', 'legacy-client', decode('01020304','hex'), NULL, 'standard', 'https://provider.example.test', false, '[]', 7, '2020-01-02 03:04:05', '2021-02-03 04:05:06'),
		('51000000-0000-0000-0000-000000000002', 'legacy-public', 'Public', 'public-client', NULL, 'none', 'standard', 'https://provider.example.test', false, '[]', 9, '2020-01-02 03:04:05', '2021-02-03 04:05:06'),
		('51000000-0000-0000-0000-000000000003', 'legacy-cimd', 'CIMD', 'https://broker.example.test/.well-known/oauth-client/51000000-0000-0000-0000-000000000003', NULL, 'private_key_jwt', 'standard', 'https://provider.example.test', false, '[]', 11, '2020-01-02 03:04:05', '2021-02-03 04:05:06'),
		('51000000-0000-0000-0000-000000000004', 'legacy-google', 'Google', 'google-client', decode('05060708','hex'), NULL, 'google', 'https://provider.example.test', false, '[]', 13, '2020-01-02 03:04:05', '2021-02-03 04:05:06'),
		('51000000-0000-0000-0000-000000000005', 'legacy-github', 'GitHub', 'github-client', decode('090a0b0c','hex'), NULL, 'github', 'https://provider.example.test', false, '[]', 15, '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO service_protected_resources (service_id, resource_uri, created_at)
		VALUES ('51000000-0000-0000-0000-000000000001', 'https://api.example.test/legacy', '2020-01-02 03:04:05');
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context, initiated_at, created_at, updated_at)
		VALUES ('51000000-0000-0000-0000-000000000101', 'legacy@example.test', '51000000-0000-0000-0000-000000000001',
			decode('11223344','hex'), decode('55667788','hex'), 'Bearer', ARRAY['read'],
			'{"service_id":"51000000-0000-0000-0000-000000000001"}', '2020-01-02 03:04:05+00', '2020-01-02 03:04:05+00', '2021-02-03 04:05:06+00');`))
}

func TestMigration036CredentialSourceLegacyBackfillAndCompatibleReapply(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 35))
	seedCredentialMigrationLegacyRows(t, f)
	before := credentialMigrationData(t, f)
	oldSchema := credentialMigrationSchema(t, f)

	// A missing 036 is an executable migration failure, never a compilation dependency.
	require.NoError(t, f.Up(t, 36))
	assert.Equal(t, before, credentialMigrationData(t, f), "backfill must not update credentials, timestamps, versions, modes or sessions")
	defaults, err := f.QuerySQL(t, `SELECT bool_and(credential_source = 'stored' AND NOT credential_source_transitioned) FROM thirdparty_oauth2_services`)
	require.NoError(t, err)
	assert.Equal(t, "true", defaults)
	legacyIdentity, err := f.QuerySQL(t, `SELECT upstream_client_id IS NULL FROM user_sessions WHERE principal = 'legacy@example.test'`)
	require.NoError(t, err)
	assert.Equal(t, "true", legacyIdentity, "current stored client ID cannot establish a legacy session identity")

	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('51000000-0000-0000-0000-000000000006', 'Default source', 'default-client', decode('01','hex'), 'https://provider.example.test', '[]');
		UPDATE user_sessions SET upstream_client_id = 'explicit-established-client' WHERE principal = 'legacy@example.test';`))
	newDefaults, err := f.QuerySQL(t, `SELECT credential_source = 'stored' AND NOT credential_source_transitioned FROM thirdparty_oauth2_services WHERE id = '51000000-0000-0000-0000-000000000006'`)
	require.NoError(t, err)
	assert.Equal(t, "true", newDefaults)
	compatibleData := credentialMigrationData(t, f)
	require.NoError(t, f.Down(t, 35))
	assert.Equal(t, oldSchema, credentialMigrationSchema(t, f), "DOWN restores 035 constraints, indexes, nullability and triggers")
	assert.Equal(t, compatibleData, credentialMigrationData(t, f))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(35), version)
	assert.False(t, dirty)
	require.NoError(t, f.Up(t, 36))
	assert.Equal(t, compatibleData, credentialMigrationData(t, f))
	identity, err := f.QuerySQL(t, `SELECT upstream_client_id IS NULL FROM user_sessions WHERE principal = 'legacy@example.test'`)
	require.NoError(t, err)
	assert.Equal(t, "true", identity, "reapply must not infer the association lost during compatible DOWN")
	defaults, err = f.QuerySQL(t, `SELECT bool_and(credential_source = 'stored' AND NOT credential_source_transitioned) FROM thirdparty_oauth2_services`)
	require.NoError(t, err)
	assert.Equal(t, "true", defaults)
}

func TestMigration036CredentialSourceConstraints(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 36))
	for _, tc := range []struct {
		name      string
		source    any
		canonical any
		client    any
		secret    any
		method    any
		flavor    any
		valid     bool
	}{
		{"stored shared secret", "stored", nil, "client", []byte{1}, nil, "standard", true},
		{"stored public", "stored", nil, "client", nil, "none", "standard", true},
		{"stored CIMD", "stored", nil, "https://broker.example.test/client", nil, "private_key_jwt", "standard", true},
		{"stored Google", "stored", nil, "client", []byte{1}, nil, "google", true},
		{"filesystem standard", "filesystem", "Platform.v1", nil, nil, nil, "standard", true},
		{"filesystem github", "filesystem", "platform-github", nil, nil, nil, "github", true},
		{"filesystem zero flavor", "filesystem", "platform-zero", nil, nil, nil, "", true},
		{"filesystem maximum canonical", "filesystem", strings.Repeat("a", 128), nil, nil, nil, "standard", true},
		{"unknown source", "other", "platform", nil, nil, nil, "standard", false},
		{"NULL source", nil, "platform", nil, nil, nil, "standard", false},
		{"stored NULL client", "stored", nil, nil, []byte{1}, nil, "standard", false},
		{"stored empty client", "stored", nil, "", []byte{1}, nil, "standard", false},
		{"stored NULL secret NULL method", "stored", nil, "client", nil, nil, "standard", false},
		{"stored empty ciphertext", "stored", nil, "client", []byte{}, nil, "standard", false},
		{"stored unknown method with NULL secret", "stored", nil, "client", nil, "unsupported", "standard", false},
		{"stored public with ciphertext", "stored", nil, "client", []byte{1}, "none", "standard", false},
		{"stored CIMD with ciphertext", "stored", nil, "client", []byte{1}, "private_key_jwt", "standard", false},
		{"filesystem NULL canonical", "filesystem", nil, nil, nil, nil, "standard", false},
		{"filesystem empty canonical", "filesystem", "", nil, nil, nil, "standard", false},
		{"filesystem whitespace canonical", "filesystem", "has space", nil, nil, nil, "standard", false},
		{"filesystem non ASCII canonical", "filesystem", "plätform", nil, nil, nil, "standard", false},
		{"filesystem overlong canonical", "filesystem", strings.Repeat("a", 129), nil, nil, nil, "standard", false},
		{"filesystem standard UUID canonical", "filesystem", "51000000-0000-0000-0000-000000000099", nil, nil, nil, "standard", false},
		{"filesystem raw UUID canonical", "filesystem", "51000000000000000000000000000099", nil, nil, nil, "standard", false},
		{"filesystem wrapped UUID canonical", "filesystem", "a51000000-0000-0000-0000-000000000099z", nil, nil, nil, "standard", false},
		{"filesystem inline client", "filesystem", "platform", "client", nil, nil, "standard", false},
		{"filesystem empty inline client", "filesystem", "platform", "", nil, nil, "standard", false},
		{"filesystem ciphertext", "filesystem", "platform", nil, []byte{1}, nil, "standard", false},
		{"filesystem empty ciphertext", "filesystem", "platform", nil, []byte{}, nil, "standard", false},
		{"filesystem public", "filesystem", "platform", nil, nil, "none", "standard", false},
		{"filesystem CIMD", "filesystem", "platform", nil, nil, "private_key_jwt", "standard", false},
		{"filesystem empty method", "filesystem", "platform", nil, nil, "", "standard", false},
		{"filesystem Google", "filesystem", "platform", nil, nil, nil, "google", false},
		{"filesystem unknown flavor", "filesystem", "platform", nil, nil, nil, "unsupported", false},
		{"filesystem NULL flavor", "filesystem", "platform", nil, nil, nil, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serviceID := uuid.NewString()
			_, err := f.db.ExecContext(context.Background(), `INSERT INTO thirdparty_oauth2_services
				(id, display_name, credential_source, canonical_id, client_id, client_secret_encrypted, token_endpoint_auth_method, oauth2_flavor, issuer_uri, scopes)
				VALUES ($1, 'Constraint test', $2, $3, $4, $5, $6, $7, 'https://provider.example.test', '[]')`,
				serviceID, tc.source, tc.canonical, tc.client, tc.secret, tc.method, tc.flavor)
			if tc.valid {
				require.NoError(t, err)
				marker, err := f.QuerySQL(t, fmt.Sprintf(`SELECT credential_source_transitioned FROM thirdparty_oauth2_services WHERE id = '%s'`, serviceID))
				require.NoError(t, err)
				assert.Equal(t, "false", marker, "direct creation does not manufacture transition evidence")
			} else {
				require.Error(t, err)
				var sqlError *pgconn.PgError
				require.True(t, errors.As(err, &sqlError))
				assert.Contains(t, []string{"23514", "23502"}, sqlError.Code, "CHECK/NOT NULL must reject invalid states, including UNKNOWN combinations")
				rows, err := f.CountRows(t, "thirdparty_oauth2_services", fmt.Sprintf("id = '%s'", serviceID))
				require.NoError(t, err)
				assert.Zero(t, rows)
			}
		})
	}
	for _, identity := range []*string{nil, new("established-client"), new(strings.Repeat("a", 300)), new("")} {
		name := "NULL legacy identity"
		if identity != nil {
			name = fmt.Sprintf("identity length %d", len(*identity))
		}
		t.Run(name, func(t *testing.T) {
			serviceID := uuid.NewString()
			_, err := f.db.ExecContext(context.Background(), `INSERT INTO thirdparty_oauth2_services (id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
				VALUES ($1, 'Identity test', 'current-service-client', decode('01','hex'), 'https://provider.example.test', '[]')`, serviceID)
			require.NoError(t, err)
			_, err = f.db.ExecContext(context.Background(), `INSERT INTO user_sessions (principal, service_id, encrypted_access_token, upstream_client_id)
				VALUES ('identity@example.test', $1, decode('01','hex'), $2)`, serviceID, identity)
			if identity != nil && *identity == "" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMigration036CredentialSourceGuardedDownPreservesSchemaAndDirtyMetadata(t *testing.T) {
	for _, state := range []string{"filesystem false marker", "filesystem true marker", "stored after round trip"} {
		t.Run(state, func(t *testing.T) {
			f := NewMigrationTestFramework(t)
			defer f.Cleanup(t)
			require.NoError(t, f.Up(t, 35))
			seedCredentialMigrationLegacyRows(t, f)
			require.NoError(t, f.Up(t, 36))
			require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET credential_source = 'filesystem', client_id = NULL, client_secret_encrypted = NULL
				WHERE id = '51000000-0000-0000-0000-000000000001'`))
			if state != "filesystem false marker" {
				require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET credential_source_transitioned = true
					WHERE id = '51000000-0000-0000-0000-000000000001'`))
			}
			if state == "stored after round trip" {
				require.NoError(t, f.ExecuteSQL(t, `UPDATE thirdparty_oauth2_services SET credential_source = 'stored', client_id = 'explicit-reverse-client', client_secret_encrypted = decode('abcdef','hex')
					WHERE id = '51000000-0000-0000-0000-000000000001'; DELETE FROM user_sessions;`))
			}
			data, schema := credentialMigrationFullData(t, f), credentialMigrationSchema(t, f)
			require.Error(t, f.Down(t, 35), "filesystem and durable transition evidence must never be coerced/deleted to downgrade")
			version, dirty, err := f.Version(t)
			require.NoError(t, err)
			assert.Equal(t, uint(35), version, "dirty target metadata is not the actual schema version")
			assert.True(t, dirty)
			require.Equal(t, schema, credentialMigrationSchema(t, f), "guard and all DDL must roll back together")
			require.Equal(t, data, credentialMigrationFullData(t, f))
			// Recover only after comparing the complete live 036 schema/data above.
			require.NoError(t, f.Force(t, 36))
			version, dirty, err = f.Version(t)
			require.NoError(t, err)
			assert.Equal(t, uint(36), version)
			assert.False(t, dirty)
			require.Error(t, f.Down(t, 35), "metadata recovery cannot bypass the incompatible-data guard")
			assert.Equal(t, schema, credentialMigrationSchema(t, f))
			assert.Equal(t, data, credentialMigrationFullData(t, f))
		})
	}
}

func TestMigration036CredentialSourceInvalidLegacyRowRollsBackAllDDL(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 35))
	seedCredentialMigrationLegacyRows(t, f)
	// 035's CHECK permits UNKNOWN when both the auth method and ciphertext are NULL.
	// 036 must refuse that corrupt row, not repair it or partially change the schema.
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services (id, display_name, client_id, client_secret_encrypted, issuer_uri, scopes)
		VALUES ('51000000-0000-0000-0000-000000000099', 'Invalid legacy row', 'legacy-client', NULL, 'https://provider.example.test', '[]')`))
	data, schema := credentialMigrationFullData(t, f), credentialMigrationSchema(t, f)
	require.Error(t, f.Up(t, 36))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(36), version)
	assert.True(t, dirty)
	require.Equal(t, schema, credentialMigrationSchema(t, f))
	require.Equal(t, data, credentialMigrationFullData(t, f))
	for _, column := range []string{"credential_source", "credential_source_transitioned"} {
		exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", column)
		require.NoError(t, err)
		assert.False(t, exists)
	}
	exists, err := f.ColumnExists(t, "user_sessions", "upstream_client_id")
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, f.Force(t, 35))
	// A deliberate operator repair is separate from migration behavior.
	require.NoError(t, f.ExecuteSQL(t, `DELETE FROM thirdparty_oauth2_services WHERE id = '51000000-0000-0000-0000-000000000099'`))
	require.NoError(t, f.Up(t, 36))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(36), version)
	assert.False(t, dirty)
}
