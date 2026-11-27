//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

const serviceAccountMigrationServiceID = "50000000-0000-0000-0000-000000000001"

func serviceAccountMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "service_accounts_49", func(t *testing.T, name string) {
		pg.ApplyMigrationsUpTo(t, name, 49)
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
		VALUES ('`+serviceAccountMigrationServiceID+`', 'company-platform', 'Company Platform', 'platform-client', decode('01020304','hex'),
		 'https://idp.example.test', false, 'https://idp.example.test/token', 'https://idp.example.test/authorize',
		 '[{"scope_value":"read","description":"Read"}]', '{}', 3, '2020-01-02 03:04:05', '2021-02-03 04:05:06');
		INSERT INTO user_sessions
		(id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context,
		 initiated_at, created_at, updated_at, callback_id)
		VALUES ('50000000-0000-0000-0000-000000000002', 'alice@example.test', '`+serviceAccountMigrationServiceID+`',
		 decode('11223344','hex'), decode('55667788','hex'), 'Bearer', ARRAY['read'],
		 '{"service_id":"`+serviceAccountMigrationServiceID+`"}', '2020-01-02 03:04:05+00',
		 '2020-01-02 03:04:05+00', '2021-02-03 04:05:06+00', 'original-callback');`))
	return f
}

func serviceAccountMigrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(jsonb_build_array(id, canonical_id, display_name, client_id, client_secret_encrypted,
		 issuer_uri, token_endpoint, authorize_endpoint, scopes, authorization_params, version, created_at, updated_at) ORDER BY id)
		 FROM thirdparty_oauth2_services),
		'sessions', (SELECT jsonb_agg(jsonb_build_array(id, principal, service_id, encrypted_access_token,
		 encrypted_refresh_token, token_type, scope, encryption_context, created_at, updated_at, callback_id) ORDER BY id)
		 FROM user_sessions)
	)::text`)
	require.NoError(t, err)
	return data
}

func insertServiceAccountRow(f *MigrationTestFramework, accountID, canonicalID, kind, scopes string, credential []byte, summary string, version int64) error {
	_, err := f.db.ExecContext(context.Background(), `INSERT INTO service_accounts
		(id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary, credential_expires_at, version)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7::jsonb, NULL, $8)`,
		accountID, serviceAccountMigrationServiceID, canonicalID, kind, scopes, credential, summary, version)
	return err
}

func TestServiceAccountsMigrationUpDownUpPreservesExistingData(t *testing.T) {
	f := serviceAccountMigrationFixture(t)
	before := serviceAccountMigrationData(t, f)
	for cycle := range 2 {
		require.NoError(t, f.Up(t, 50), "cycle %d", cycle)
		exists, err := f.TableExists(t, "service_accounts")
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, before, serviceAccountMigrationData(t, f))
		require.NoError(t, insertServiceAccountRow(f, "50000000-0000-0000-0000-0000000000a1", "platform-triage-bot", "jwt_bearer", `["read"]`, []byte{1, 2, 3}, `{"format":"signed_jwt"}`, 1))

		require.NoError(t, f.Down(t, 49), "cycle %d", cycle)
		exists, err = f.TableExists(t, "service_accounts")
		require.NoError(t, err)
		assert.False(t, exists, "rollback drops the table and its stored credentials")
		assert.Equal(t, before, serviceAccountMigrationData(t, f), "rollback removes only service_accounts rows")
	}
	require.NoError(t, f.Up(t, 50))
	count, err := f.CountRows(t, "service_accounts")
	require.NoError(t, err)
	assert.Zero(t, count)
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(50), version)
	assert.False(t, dirty)
}

func TestServiceAccountsMigrationColumnsHoldNoPlaintext(t *testing.T) {
	f := serviceAccountMigrationFixture(t)
	require.NoError(t, f.Up(t, 50))
	columns, err := f.QuerySQL(t, `SELECT string_agg(column_name || ':' || data_type || ':' || is_nullable, ',' ORDER BY ordinal_position)
		FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'service_accounts'`)
	require.NoError(t, err)
	assert.Equal(t, "id:uuid:NO,service_id:uuid:NO,canonical_id:character varying:NO,kind:character varying:NO,"+
		"scopes:jsonb:NO,credential_encrypted:bytea:NO,credential_summary:jsonb:NO,"+
		"credential_expires_at:timestamp with time zone:YES,version:bigint:NO,"+
		"created_at:timestamp with time zone:NO,updated_at:timestamp with time zone:NO", columns)
	defaulted, err := f.QuerySQL(t, `SELECT column_default FROM information_schema.columns WHERE table_name = 'service_accounts' AND column_name = 'version'`)
	require.NoError(t, err)
	assert.Equal(t, "1", defaulted)
}

func TestServiceAccountsMigrationEnforcesConstraints(t *testing.T) {
	f := serviceAccountMigrationFixture(t)
	require.NoError(t, f.Up(t, 50))
	require.NoError(t, insertServiceAccountRow(f, "50000000-0000-0000-0000-0000000000b1", "platform-triage-bot", "jwt_bearer", `["read"]`, []byte{1}, `{}`, 1))

	tests := []struct {
		name       string
		canonical  string
		kind       string
		scopes     string
		credential []byte
		summary    string
		version    int64
		code       string
		constraint string
	}{
		{"duplicate canonical_id within the service", "platform-triage-bot", "jwt_bearer", `["read"]`, []byte{1}, `{}`, 1, "23505", "uq_service_accounts_service_canonical_id"},
		{"canonical_id with a space", "platform triage", "jwt_bearer", `["read"]`, []byte{1}, `{}`, 1, "23514", "chk_service_accounts_canonical_id"},
		{"canonical_id with a non-ASCII letter", "plätform", "jwt_bearer", `["read"]`, []byte{1}, `{}`, 1, "23514", "chk_service_accounts_canonical_id"},
		{"empty kind", "a", "", `["read"]`, []byte{1}, `{}`, 1, "23514", "chk_service_accounts_kind"},
		{"empty scopes", "b", "jwt_bearer", `[]`, []byte{1}, `{}`, 1, "23514", "chk_service_accounts_scopes"},
		{"scopes not an array", "c", "jwt_bearer", `{"read":true}`, []byte{1}, `{}`, 1, "23514", "chk_service_accounts_scopes"},
		{"empty ciphertext", "d", "jwt_bearer", `["read"]`, []byte{}, `{}`, 1, "23514", "chk_service_accounts_credential_encrypted"},
		{"summary not an object", "e", "jwt_bearer", `["read"]`, []byte{1}, `[]`, 1, "23514", "chk_service_accounts_credential_summary"},
		{"version zero", "f", "jwt_bearer", `["read"]`, []byte{1}, `{}`, 0, "23514", "chk_service_accounts_version"},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			accountID := "50000000-0000-0000-0000-0000000000c" + string(rune('0'+i))
			err := insertServiceAccountRow(f, accountID, tc.canonical, tc.kind, tc.scopes, tc.credential, tc.summary, tc.version)
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, tc.code, pgErr.Code)
			assert.Equal(t, tc.constraint, pgErr.ConstraintName)
		})
	}

	t.Run("an account cannot exist without its service", func(t *testing.T) {
		_, err := f.db.ExecContext(context.Background(), `INSERT INTO service_accounts
			(id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary)
			VALUES ('50000000-0000-0000-0000-0000000000d1', '50000000-0000-0000-0000-0000000000ff', 'x', 'jwt_bearer', '["read"]', '\x01', '{}')`)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "23503", pgErr.Code)
	})

	t.Run("a service that holds an account cannot be deleted", func(t *testing.T) {
		_, err := f.db.ExecContext(context.Background(), `DELETE FROM thirdparty_oauth2_services WHERE id = $1`, serviceAccountMigrationServiceID)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "23503", pgErr.Code)
		count, err := f.CountRows(t, "service_accounts")
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)
	})

	t.Run("the same canonical_id is free on another service", func(t *testing.T) {
		require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
			(id, canonical_id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery,
			 token_endpoint, authorize_endpoint, scopes, authorization_params)
			VALUES ('50000000-0000-0000-0000-0000000000e1', 'other-platform', 'Other', 'other-client', decode('01','hex'),
			 'https://other.example.test', false, 'https://other.example.test/token', 'https://other.example.test/authorize', '[]', '{}')`))
		_, err := f.db.ExecContext(context.Background(), `INSERT INTO service_accounts
			(id, service_id, canonical_id, kind, scopes, credential_encrypted, credential_summary)
			VALUES ('50000000-0000-0000-0000-0000000000e2', '50000000-0000-0000-0000-0000000000e1', 'platform-triage-bot', 'jwt_bearer', '["read"]', '\x01', '{}')`)
		require.NoError(t, err)
	})
}
