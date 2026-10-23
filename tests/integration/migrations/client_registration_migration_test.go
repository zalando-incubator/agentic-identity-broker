//go:build integration

package migrations

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedClientRegistrationLegacyRows(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, canonical_id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, credential_source, oauth2_flavor, issuer_uri, scopes)
		VALUES
		('62000000-0000-0000-0000-000000000001', NULL, 'Static confidential', 'operator-confidential', decode('0102', 'hex'), NULL, 'stored', 'standard', 'https://provider.example.test', '[]'),
		('62000000-0000-0000-0000-000000000002', NULL, 'Static public', 'operator-public', NULL, 'none', 'stored', 'standard', 'https://provider.example.test', '[]'),
		('62000000-0000-0000-0000-000000000003', NULL, 'CIMD confidential', 'https://broker.example.test/.well-known/oauth-client/62000000-0000-0000-0000-000000000003', NULL, 'private_key_jwt', 'stored', 'standard', 'https://provider.example.test', '[]'),
		('62000000-0000-0000-0000-000000000004', 'filesystem-service', 'Filesystem', NULL, NULL, NULL, 'filesystem', 'standard', 'https://provider.example.test', '[]');
		INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, credential_source, oauth2_flavor, issuer_uri, scopes,
		 enable_discovery, resource_url, client_method, authorization_params, discovery_last_attempt_at, discovery_last_success_at)
		VALUES
		('62000000-0000-0000-0000-000000000005', 'DCR public', 'dcr-public', NULL, 'none', 'stored', 'standard', 'https://provider.example.test', '[]',
		 true, 'https://api.example.test/public', 'dcr', '{"resource":"https://api.example.test/public"}', now(), now()),
		('62000000-0000-0000-0000-000000000006', 'DCR confidential', 'dcr-basic', decode('0304', 'hex'), 'client_secret_basic', 'stored', 'standard', 'https://provider.example.test', '[]',
		 true, 'https://api.example.test/basic', 'dcr', '{"resource":"https://api.example.test/basic"}', now(), now());
		INSERT INTO user_sessions (id, principal, service_id, encrypted_access_token, encrypted_refresh_token, token_type, scope, encryption_context, initiated_at, callback_id)
		VALUES ('62000000-0000-0000-0000-000000000101', 'legacy@example.test', '62000000-0000-0000-0000-000000000002',
		 decode('1020', 'hex'), decode('3040', 'hex'), 'Bearer', ARRAY['read'],
		 '{"service_id":"62000000-0000-0000-0000-000000000002"}', now(), 'legacy-callback');`))
}

func legacyClientRegistrationData(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	data, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'services', (SELECT jsonb_agg(jsonb_build_object(
			'id', id, 'client_id', client_id, 'secret', encode(client_secret_encrypted, 'hex'),
			'method', token_endpoint_auth_method, 'source', credential_source,
			'version', version, 'updated_at', updated_at) ORDER BY id) FROM thirdparty_oauth2_services),
		'session', (SELECT jsonb_build_object(
			'principal', principal, 'service_id', service_id,
			'access', encode(encrypted_access_token, 'hex'), 'refresh', encode(encrypted_refresh_token, 'hex'))
			FROM user_sessions WHERE principal = 'legacy@example.test'))::text`)
	require.NoError(t, err)
	return data
}

func TestMigration043ClassifiesLegacyServicesWithoutChangingIdentitiesOrSessions(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 41))
	seedClientRegistrationLegacyRows(t, f)
	before := legacyClientRegistrationData(t, f)

	require.NoError(t, f.Up(t, 43))
	assert.Equal(t, before, legacyClientRegistrationData(t, f))
	for index, want := range []string{"static", "static", "cimd", "static", "static", "static"} {
		rowID := fmt.Sprintf("62000000-0000-0000-0000-%012d", index+1)
		var actual string
		err := f.db.QueryRowContext(context.Background(), `SELECT client_registration FROM thirdparty_oauth2_services WHERE id = $1`, rowID).Scan(&actual)
		require.NoError(t, err)
		assert.Equal(t, want, actual, rowID)
	}
	validated, err := f.QuerySQL(t, `SELECT convalidated FROM pg_constraint
		WHERE conrelid = 'thirdparty_oauth2_services'::regclass AND conname = 'chk_thirdparty_oauth2_services_client_registration'`)
	require.NoError(t, err)
	assert.Equal(t, "true", validated)

	require.NoError(t, f.Down(t, 42))
	validated, err = f.QuerySQL(t, `SELECT convalidated FROM pg_constraint
		WHERE conrelid = 'thirdparty_oauth2_services'::regclass AND conname = 'chk_thirdparty_oauth2_services_client_registration'`)
	require.NoError(t, err)
	assert.Equal(t, "false", validated)
	require.NoError(t, f.Up(t, 43))
	assert.Equal(t, before, legacyClientRegistrationData(t, f))
}

func TestMigration042RejectsIncompatibleRegistrationRows(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 43))

	for _, tc := range []struct {
		name, registration                           string
		method, client, source, clientMethod, secret any
		valid                                        bool
	}{
		{name: "CIMD public", registration: "cimd", method: "none", client: "hosted", source: "stored", valid: true},
		{name: "CIMD confidential", registration: "cimd", method: "private_key_jwt", client: "hosted", source: "stored", valid: true},
		{name: "static confidential", registration: "static", client: "operator", source: "stored", secret: []byte{1}, valid: true},
		{name: "static public", registration: "static", method: "none", client: "operator", source: "stored", valid: true},
		{name: "static private key", registration: "static", method: "private_key_jwt", client: "operator", source: "stored"},
		{name: "CIMD shared secret", registration: "cimd", client: "hosted", source: "stored", secret: []byte{1}},
		{name: "CIMD filesystem", registration: "cimd", source: "filesystem"},
		{name: "CIMD DCR", registration: "cimd", method: "none", client: "hosted", source: "stored", clientMethod: "dcr"},
		{name: "static hosted discovery", registration: "static", method: "private_key_jwt", client: "operator", source: "stored", clientMethod: "cimd"},
		{name: "CIMD wrong identifier", registration: "cimd", method: "none", client: "operator", source: "stored"},
		{name: "unknown registration", registration: "unknown", method: "none", client: "operator", source: "stored"},
		{name: "empty registration", method: "none", client: "operator", source: "stored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serviceID := uuid.NewString()
			clientID := tc.client
			if clientID == "hosted" {
				clientID = "https://broker.example.test/.well-known/oauth-client/" + serviceID
			}
			var resourceURL any
			var authorizationParams = `{}`
			if tc.clientMethod != nil {
				resourceURL = "https://api.example.test/discovered/" + serviceID
				authorizationParams = fmt.Sprintf(`{"resource":%q}`, resourceURL)
			}
			var canonicalID any
			if tc.source == "filesystem" {
				canonicalID = "filesystem-test"
			}
			_, err := f.db.ExecContext(context.Background(), `INSERT INTO thirdparty_oauth2_services
				(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, credential_source,
				 client_registration, client_method, canonical_id, oauth2_flavor, issuer_uri, scopes,
				 enable_discovery, resource_url, authorization_params, discovery_last_attempt_at, discovery_last_success_at)
				VALUES ($1, 'Registration constraint', $2, $3, $4, $5, $6, $7, $8, 'standard', 'https://provider.example.test', '[]',
				 $9::text IS NOT NULL, $9, $10::jsonb, CASE WHEN $9::text IS NOT NULL THEN now() END, CASE WHEN $9::text IS NOT NULL THEN now() END)`,
				serviceID, clientID, tc.secret, tc.method, tc.source, tc.registration, tc.clientMethod, canonicalID,
				resourceURL, authorizationParams)
			if tc.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var sqlError *pgconn.PgError
			require.True(t, errors.As(err, &sqlError))
			assert.Equal(t, "23514", sqlError.Code)
			assert.Equal(t, "chk_thirdparty_oauth2_services_client_registration", sqlError.ConstraintName)
		})
	}

	serviceID := uuid.NewString()
	_, err := f.db.ExecContext(context.Background(), `INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, credential_source,
		 client_registration, oauth2_flavor, issuer_uri, scopes)
		VALUES ($1, 'Null registration', 'operator', NULL, 'none', 'stored', NULL, 'standard', 'https://provider.example.test', '[]')`, serviceID)
	require.Error(t, err)
	var sqlError *pgconn.PgError
	require.True(t, errors.As(err, &sqlError))
	assert.Equal(t, "23502", sqlError.Code)
}

func TestMigration043RejectsMalformedLegacyCIMDIdentity(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 41))
	require.NoError(t, f.ExecuteSQL(t, `INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, scopes)
		VALUES ('62000000-0000-0000-0000-000000000099', 'Malformed CIMD', 'https://broker.example.test/other-client',
		 NULL, 'private_key_jwt', 'https://provider.example.test', '[]')`))
	require.NoError(t, f.Up(t, 42))
	require.Error(t, f.Up(t, 43))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(43), version)
	assert.True(t, dirty)
	registration, err := f.QuerySQL(t, `SELECT client_registration FROM thirdparty_oauth2_services
		WHERE id = '62000000-0000-0000-0000-000000000099'`)
	require.NoError(t, err)
	assert.Equal(t, "static", registration, "failed backfill must not partially classify a malformed legacy row")
}

func TestMigration042GuardedRollbackPreservesCIMDPublicIdentity(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 43))
	serviceID := uuid.NewString()
	clientID := "https://broker.example.test/.well-known/oauth-client/" + serviceID
	_, err := f.db.ExecContext(context.Background(), `INSERT INTO thirdparty_oauth2_services
		(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method,
		 credential_source, client_registration, issuer_uri, scopes)
		VALUES ($1, 'CIMD public', $2, NULL, 'none', 'stored', 'cimd', 'https://provider.example.test', '[]')`,
		serviceID, clientID)
	require.NoError(t, err)
	require.NoError(t, f.Down(t, 42))
	require.Error(t, f.Down(t, 41), "rollback must not reinterpret a hosted URL as an operator public ID")
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(41), version)
	assert.True(t, dirty)
	var actual string
	err = f.db.QueryRowContext(context.Background(), `SELECT client_id FROM thirdparty_oauth2_services WHERE id = $1`, serviceID).Scan(&actual)
	require.NoError(t, err)
	assert.Equal(t, clientID, actual)
	require.NoError(t, f.Force(t, 42))
	require.Error(t, f.Down(t, 41), "force clears the dirty bit, not the rollback guard")
	require.NoError(t, f.Force(t, 42))
	_, err = f.db.ExecContext(context.Background(), `UPDATE thirdparty_oauth2_services
		SET client_registration = 'static', client_id = 'operator-public' WHERE id = $1`, serviceID)
	require.NoError(t, err)
	require.NoError(t, f.Down(t, 41))
	exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", "client_registration")
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, f.Up(t, 43))
	err = f.db.QueryRowContext(context.Background(), `SELECT client_registration FROM thirdparty_oauth2_services WHERE id = $1`, serviceID).Scan(&actual)
	require.NoError(t, err)
	assert.Equal(t, "static", actual)
}
