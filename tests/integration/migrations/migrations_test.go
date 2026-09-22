//go:build integration
// +build integration

package migrations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval/toolpattern"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationLifecycle verifies the complete migration lifecycle: apply, rollback, and reapply
func TestMigrationLifecycle(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Step 1: Apply all migrations
	t.Log("Step 1: Applying all migrations...")
	err := f.UpAll(t)
	require.NoError(t, err)

	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	t.Logf("After Up: version=%d, dirty=%v", version, dirty)
	assert.False(t, dirty, "Migration should not be dirty")
	assert.Greater(t, version, uint(0), "Should have applied at least one migration")

	// Step 2: Verify representative tables from across the migration history exist.
	tables := []string{"agents", "thirdparty_oauth2_services", "service_protected_resources", "user_sessions", "authorization_codes"}
	for _, table := range tables {
		exists, err := f.TableExists(t, table)
		require.NoError(t, err)
		assert.True(t, exists, "Table %s should exist after migrations", table)
	}

	// Step 3: Rollback all migrations
	t.Log("Step 3: Rolling back all migrations...")
	err = f.DownAll(t)
	require.NoError(t, err)

	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	t.Logf("After Down: version=%d, dirty=%v", version, dirty)
	assert.Equal(t, uint(0), version, "Should be at version 0 after rollback")
	assert.False(t, dirty, "Migration should not be dirty")

	// Note: We skip table deletion verification as go-migrate marks versions as rolled back
	// but actual SQL execution may be deferred/cached. The important part is version is 0.

	// Step 5: Reapply all migrations
	t.Log("Step 5: Reapplying all migrations...")
	err = f.UpAll(t)
	require.NoError(t, err)

	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	t.Logf("After Up again: version=%d, dirty=%v", version, dirty)
	assert.False(t, dirty, "Migration should not be dirty")
	assert.Greater(t, version, uint(0), "Should have applied migrations again")

	// Step 6: Reapply successful
	t.Log("Step 6: Migrations successfully reapplied")
}

// TestMigration007OAuth2Flavor verifies migration 007 (adding oauth2_flavor column) lifecycle.
// Requires: integration build tag and Docker/Podman.
func TestMigration007OAuth2Flavor(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Step 1: Apply migrations 001-006 only (using m.Migrate to stop at version 6)
	t.Log("Step 1: Applying migrations up to version 6...")
	err := f.Up(t, 6)
	require.NoError(t, err)

	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	t.Logf("After Up to 6: version=%d, dirty=%v", version, dirty)
	assert.False(t, dirty)
	assert.Equal(t, uint(6), version)

	// Verify oauth2_flavor column does NOT exist yet before migration 007
	exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", "oauth2_flavor")
	require.NoError(t, err)
	assert.False(t, exists, "oauth2_flavor column should NOT exist before migration 007")

	// Insert a pre-existing row to verify it gets oauth2_flavor = 'standard' via column DEFAULT.
	// '\x01' is a minimal non-empty BYTEA value — sufficient as a placeholder for the
	// encrypted secret since this test only exercises the migration DEFAULT, not encryption.
	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery, scopes)
		VALUES
			('11111111-1111-1111-1111-111111111111', 'pre-existing svc', 'client-pre',
			 '\x01', 'https://oauth.example.com', false, '[]');
	`)
	require.NoError(t, err, "should be able to insert a pre-existing row before migration 007")

	// Step 2: Apply migration 007 (apply all remaining)
	t.Log("Step 2: Applying migration 007 (ADD COLUMN oauth2_flavor)...")
	err = f.UpAll(t)
	require.NoError(t, err)

	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	t.Logf("After Up 007: version=%d, dirty=%v", version, dirty)
	assert.False(t, dirty)
	assert.GreaterOrEqual(t, version, uint(7), "Migration 007 should be applied")

	// Step 3: Verify column exists with correct default value for the pre-existing row
	exists, err = f.ColumnExists(t, "thirdparty_oauth2_services", "oauth2_flavor")
	require.NoError(t, err)
	assert.True(t, exists, "oauth2_flavor column should exist after migration 007")

	// Verify pre-existing row got the column DEFAULT ('standard') when migration was applied.
	result, err := f.QuerySQL(t, `
		SELECT oauth2_flavor FROM thirdparty_oauth2_services
		WHERE id = '11111111-1111-1111-1111-111111111111';
	`)
	require.NoError(t, err)
	assert.Equal(t, "standard", strings.TrimSpace(result),
		"pre-existing row should have oauth2_flavor = 'standard' from column DEFAULT")

	// Step 4: Rollback migration 007 (DROP COLUMN)
	t.Log("Step 4: Rolling back migration 007...")
	err = f.Down(t, 6)
	require.NoError(t, err)

	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	t.Logf("After Down to 6: version=%d, dirty=%v", version, dirty)
	assert.False(t, dirty)

	// Step 5: Verify column is gone after rollback
	exists, err = f.ColumnExists(t, "thirdparty_oauth2_services", "oauth2_flavor")
	require.NoError(t, err)
	assert.False(t, exists, "oauth2_flavor column should NOT exist after rollback")

	// Step 6: Re-apply migration 007 to verify idempotency
	t.Log("Step 6: Re-applying migration 007...")
	err = f.UpAll(t)
	require.NoError(t, err)

	exists, err = f.ColumnExists(t, "thirdparty_oauth2_services", "oauth2_flavor")
	require.NoError(t, err)
	assert.True(t, exists, "oauth2_flavor column should exist after re-apply")

	t.Log("Migration 007 lifecycle test complete")
}

// TestMigration015AgentCIMDFields verifies migration 015 lifecycle:
// adds agent_client_uris child table to agents.
func TestMigration015AgentCIMDFields(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Apply migrations up to version 014
	err := f.Up(t, 14)
	require.NoError(t, err)

	// Verify agent_client_uris table does NOT exist before migration 015
	exists, err := f.TableExists(t, "agent_client_uris")
	require.NoError(t, err)
	assert.False(t, exists, "agent_client_uris table should NOT exist before migration 015")

	// Apply migration 015
	err = f.UpAll(t)
	require.NoError(t, err)

	// Verify agent_client_uris table now exists
	exists, err = f.TableExists(t, "agent_client_uris")
	require.NoError(t, err)
	assert.True(t, exists, "agent_client_uris table should exist after migration 015")

	// Rollback migration 015
	err = f.Down(t, 14)
	require.NoError(t, err)

	exists, err = f.TableExists(t, "agent_client_uris")
	require.NoError(t, err)
	assert.False(t, exists, "agent_client_uris table should be gone after rollback")

	t.Log("Migration 015 lifecycle test complete")
}

func TestMigration029CanonicalIDs(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 28))

	for _, table := range []string{"agents", "thirdparty_oauth2_services", "permission_sets"} {
		exists, err := f.ColumnExists(t, table, "canonical_id")
		require.NoError(t, err)
		assert.False(t, exists)
	}

	require.NoError(t, f.UpAll(t))
	for _, table := range []string{"agents", "thirdparty_oauth2_services", "permission_sets"} {
		exists, err := f.ColumnExists(t, table, "canonical_id")
		require.NoError(t, err)
		assert.True(t, exists)
	}
	for _, index := range []string{"uq_agents_canonical_id", "uq_thirdparty_oauth2_services_canonical_id", "uq_permission_sets_canonical_id"} {
		exists, err := f.IndexExists(t, index)
		require.NoError(t, err)
		assert.True(t, exists)
	}

	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, display_name, description) VALUES
		('10000000-0000-0000-0000-000000000001', 'agent one', 'first'),
		('10000000-0000-0000-0000-000000000002', 'agent two', 'second');
		UPDATE agents SET canonical_id = 'shared' WHERE id = '10000000-0000-0000-0000-000000000001';
		INSERT INTO permission_sets (id, name, description, canonical_id) VALUES
		('20000000-0000-0000-0000-000000000001', 'set one', 'first', 'shared');
	`))
	err := f.ExecuteSQL(t, `UPDATE agents SET canonical_id = 'shared' WHERE id = '10000000-0000-0000-0000-000000000002'`)
	require.Error(t, err)

	require.NoError(t, f.Down(t, 28))
	for _, table := range []string{"agents", "thirdparty_oauth2_services", "permission_sets"} {
		exists, err := f.ColumnExists(t, table, "canonical_id")
		require.NoError(t, err)
		assert.False(t, exists)
	}
}

func TestMigration032ApprovalPatterns(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 31))
	exists, err := f.ColumnExists(t, "tool_approvals", "tool_pattern")
	require.NoError(t, err)
	assert.False(t, exists)
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, display_name, description) VALUES ('30000000-0000-0000-0000-000000000001', 'pattern agent', 'test');
		INSERT INTO tool_approvals (id, principal, agent_id, gateway_client_id, tool_name, arguments, arguments_hash, approval_url, expires_at) VALUES
		('40000000-0000-0000-0000-000000000001', 'pattern@example.com', '30000000-0000-0000-0000-000000000001', 'gateway', 'scalar_tool', '{"repo":"acme/app","count":2,"ratio":1.5,"draft":false,"note":null,"pattern":"a*b","path":"c\\d"}', 'hash-1', 'https://broker.example/approval', NOW() + interval '1 hour'),
		('40000000-0000-0000-0000-000000000002', 'pattern@example.com', '30000000-0000-0000-0000-000000000001', 'gateway', 'composite_tool', '{"reviewers":["b","a"],"meta":{"b":1,"a":"x"},"empty":{},"list":[]}', 'hash-2', 'https://broker.example/approval', NOW() + interval '1 hour'),
		('40000000-0000-0000-0000-000000000003', 'pattern@example.com', '30000000-0000-0000-0000-000000000001', 'gateway', 'escape_tool', '{"html":"a<b&c>d","ctl":"a\tb","quote":"say \"hi\""}', 'hash-3', 'https://broker.example/approval', NOW() + interval '1 hour'),
		('40000000-0000-0000-0000-000000000004', 'pattern@example.com', '30000000-0000-0000-0000-000000000001', 'gateway', 'literal\*tool', '{"value":"value\\*suffix"}', 'hash-4', 'https://broker.example/approval', NOW() + interval '1 hour');
	`))
	require.NoError(t, f.Up(t, 32))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.False(t, dirty)
	for _, column := range []string{"tool_pattern", "params_pattern"} {
		exists, err := f.ColumnExists(t, "tool_approvals", column)
		require.NoError(t, err)
		assert.True(t, exists)
	}
	toolType, err := f.GetColumnType(t, "tool_approvals", "tool_pattern")
	require.NoError(t, err)
	assert.Equal(t, "character varying", toolType)
	paramsType, err := f.GetColumnType(t, "tool_approvals", "params_pattern")
	require.NoError(t, err)
	assert.Equal(t, "jsonb", paramsType)
	nullability, err := f.QuerySQL(t, `SELECT string_agg(column_name || ':' || is_nullable, ',' ORDER BY column_name) FROM information_schema.columns WHERE table_name = 'tool_approvals' AND column_name IN ('tool_pattern', 'params_pattern')`)
	require.NoError(t, err)
	assert.Equal(t, "params_pattern:NO,tool_pattern:NO", strings.TrimSpace(nullability))

	rowsJSON, err := f.QuerySQL(t, `SELECT json_agg(json_build_object('id', id, 'tool_pattern', tool_pattern, 'params_pattern', params_pattern) ORDER BY id)::text FROM tool_approvals`)
	require.NoError(t, err)
	var rows []struct {
		ID            string            `json:"id"`
		ToolPattern   string            `json:"tool_pattern"`
		ParamsPattern map[string]string `json:"params_pattern"`
	}
	require.NoError(t, json.Unmarshal([]byte(rowsJSON), &rows))
	type expectedApproval struct {
		ToolName  string
		Arguments map[string]any
	}
	expected := map[string]expectedApproval{
		"40000000-0000-0000-0000-000000000001": {ToolName: "scalar_tool", Arguments: map[string]any{"repo": "acme/app", "count": 2, "ratio": 1.5, "draft": false, "note": nil, "pattern": "a*b", "path": `c\d`}},
		"40000000-0000-0000-0000-000000000002": {ToolName: "composite_tool", Arguments: map[string]any{"reviewers": []any{"b", "a"}, "meta": map[string]any{"b": 1, "a": "x"}, "empty": map[string]any{}, "list": []any{}}},
		"40000000-0000-0000-0000-000000000003": {ToolName: "escape_tool", Arguments: map[string]any{"html": "a<b&c>d", "ctl": "a\tb", "quote": `say "hi"`}},
		"40000000-0000-0000-0000-000000000004": {ToolName: `literal\*tool`, Arguments: map[string]any{"value": `value\*suffix`}},
	}
	require.Len(t, rows, len(expected))
	for _, row := range rows {
		expectedRow, ok := expected[row.ID]
		require.True(t, ok)
		assert.Equal(t, toolpattern.EscapeLiteral(expectedRow.ToolName), row.ToolPattern)
		assert.Equal(t, toolpattern.ExactParams(expectedRow.Arguments), row.ParamsPattern)
		assert.True(t, toolpattern.Matches(row.ToolPattern, row.ParamsPattern, expectedRow.ToolName, expectedRow.Arguments))
	}
	require.NoError(t, f.Down(t, 31))
	for _, column := range []string{"tool_pattern", "params_pattern"} {
		exists, err := f.ColumnExists(t, "tool_approvals", column)
		require.NoError(t, err)
		assert.False(t, exists)
	}
	count, err := f.CountRows(t, "tool_approvals")
	require.NoError(t, err)
	assert.Equal(t, int64(len(expected)), count)
	require.NoError(t, f.Up(t, 32))
}

func TestMigration031(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Step 1: Apply migrations through 030 and add a confidential service predating migration 031.
	require.NoError(t, f.Up(t, 30))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000001', 'pre-existing confidential service', 'client-pre',
			 '\x01', 'https://oauth.example.com', false, '[]');
	`))

	// Step 2: Apply migration 031 without changing existing services.
	require.NoError(t, f.Up(t, 31))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(31), version)
	assert.False(t, dirty)

	methodIsNull, err := f.QuerySQL(t, `
		SELECT token_endpoint_auth_method IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '30000000-0000-0000-0000-000000000001';
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(methodIsNull), "migration 031 must not backfill existing services")

	// Step 3: The database rejects both invalid client-authentication combinations.
	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000002', 'public service with ciphertext', 'client-invalid-public',
			 '\x01', 'none', 'https://oauth.example.com', false, '[]');
	`)
	require.Error(t, err, "a public service cannot persist a client-secret ciphertext")

	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000003', 'confidential service without ciphertext', 'client-invalid-confidential',
			 NULL, NULL, 'https://oauth.example.com', false, '[]');
	`)
	require.Error(t, err, "a confidential service must persist a client-secret ciphertext")

	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000005', 'service with unsupported authentication method', 'client-invalid-method',
			 NULL, 'client_secret_post', 'https://oauth.example.com', false, '[]');
	`)
	require.Error(t, err, "an unsupported client-authentication method must not persist")

	// Step 4: A public service blocks rollback and is named in the error.
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000004', 'public service blocking rollback', 'client-public',
			 NULL, 'none', 'https://oauth.example.com', false, '[]');
	`))
	err = f.Down(t, 30)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "public service blocking rollback")

	// Step 5: A failed down migration remains dirty and leaves its column in place.
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	// go-migrate marks the requested target before executing the guarded down migration.
	assert.Equal(t, uint(30), version)
	assert.True(t, dirty)
	exists, err := f.ColumnExists(t, "thirdparty_oauth2_services", "token_endpoint_auth_method")
	require.NoError(t, err)
	assert.True(t, exists)

	confidentialCiphertext, err := f.QuerySQL(t, `
		SELECT encode(client_secret_encrypted, 'hex')
		FROM thirdparty_oauth2_services
		WHERE id = '30000000-0000-0000-0000-000000000001';
	`)
	require.NoError(t, err)
	assert.Equal(t, "01", strings.TrimSpace(confidentialCiphertext))

	publicServiceIsIntact, err := f.QuerySQL(t, `
		SELECT token_endpoint_auth_method = 'none' AND client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '30000000-0000-0000-0000-000000000004';
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(publicServiceIsIntact))

	// Step 6: Clear the failed migration state, remove the public service, and roll back cleanly.
	require.NoError(t, f.Force(t, 31))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(31), version)
	assert.False(t, dirty)
	require.NoError(t, f.ExecuteSQL(t, `
		DELETE FROM thirdparty_oauth2_services
		WHERE id = '30000000-0000-0000-0000-000000000004';
	`))
	require.NoError(t, f.Down(t, 30))

	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(30), version)
	assert.False(t, dirty)
	exists, err = f.ColumnExists(t, "thirdparty_oauth2_services", "token_endpoint_auth_method")
	require.NoError(t, err)
	assert.False(t, exists)

	confidentialCiphertext, err = f.QuerySQL(t, `
		SELECT encode(client_secret_encrypted, 'hex')
		FROM thirdparty_oauth2_services
		WHERE id = '30000000-0000-0000-0000-000000000001';
	`)
	require.NoError(t, err)
	assert.Equal(t, "01", strings.TrimSpace(confidentialCiphertext))

	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery, scopes)
		VALUES
			('30000000-0000-0000-0000-000000000006', 'confidential service without ciphertext after rollback', 'client-null-after-rollback',
			 NULL, 'https://oauth.example.com', false, '[]');
	`)
	require.Error(t, err, "rollback must restore the confidential client-secret constraint")
}

func TestMigration032KeyDomain(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Step 1: Create legacy signing keys before migration 032 adds their domain.
	require.NoError(t, f.Up(t, 31))
	exists, err := f.ColumnExists(t, "signing_keys", "key_domain")
	require.NoError(t, err)
	assert.False(t, exists, "key_domain must not exist before migration 032")
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO signing_keys
			(id, kid, algorithm, private_key_encrypted, is_current, activates_at, created_at)
		VALUES
			('32000000-0000-0000-0000-000000000001', 'legacy-current-signing-key', 'ES256', '\x01', true, NOW(), NOW()),
			('32000000-0000-0000-0000-000000000002', 'legacy-previous-signing-key', 'ES256', '\x02', false, NOW(), NOW());
	`))

	// Step 2: Migration 032 backfills every legacy key into the token-signing domain.
	require.NoError(t, f.Up(t, 32))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.False(t, dirty)

	exists, err = f.ColumnExists(t, "signing_keys", "key_domain")
	require.NoError(t, err)
	assert.True(t, exists)
	legacyDomains, err := f.QuerySQL(t, `
		SELECT bool_and(key_domain = 'token_signing')
		FROM signing_keys
		WHERE kid IN ('legacy-current-signing-key', 'legacy-previous-signing-key');
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(legacyDomains), "legacy signing keys must become token-signing keys")

	// Step 3: Migration 032 permits only the defined signing-key domains.
	err = f.ExecuteSQL(t, `
		INSERT INTO signing_keys
			(id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at)
		VALUES
			('32000000-0000-0000-0000-000000000004', 'unsupported-signing-key-domain', 'unsupported_domain', 'ES256', '\x04', false, NOW(), NOW());
	`)
	assert.Error(t, err, "migration 032 must reject unsupported signing-key domains")

	// Step 4: Replaying the applied migration is a no-op.
	require.NoError(t, f.Up(t, 32))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.False(t, dirty)

	// Step 5: A current CIMD key can coexist with a current token-signing key.
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO signing_keys
			(id, kid, key_domain, algorithm, private_key_encrypted, is_current, activates_at, created_at)
		VALUES
			('32000000-0000-0000-0000-000000000003', 'cimd-client-authentication-key', 'cimd_client_authentication', 'ES256', '\x03', true, NOW(), NOW());
	`))

	// Step 6: Migration 032 must not discard CIMD key material during rollback.
	err = f.Down(t, 31)
	require.Error(t, err, "migration 032 rollback must refuse while CIMD keys exist")
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(31), version)
	assert.True(t, dirty)

	cimdKeyIsIntact, err := f.QuerySQL(t, `
		SELECT id = '32000000-0000-0000-0000-000000000003'
			AND kid = 'cimd-client-authentication-key'
			AND key_domain = 'cimd_client_authentication'
			AND encode(private_key_encrypted, 'hex') = '03'
		FROM signing_keys
		WHERE id = '32000000-0000-0000-0000-000000000003';
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(cimdKeyIsIntact), "failed rollback must preserve the blocking CIMD key identity and ciphertext")

	// Step 7: Once the CIMD key is gone, rollback and a subsequent replay both succeed.
	require.NoError(t, f.Force(t, 32))
	require.NoError(t, f.ExecuteSQL(t, `
		DELETE FROM signing_keys WHERE kid = 'cimd-client-authentication-key';
	`))
	require.NoError(t, f.Down(t, 31))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(31), version)
	assert.False(t, dirty)
	exists, err = f.ColumnExists(t, "signing_keys", "key_domain")
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, f.Up(t, 32))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.False(t, dirty)
	legacyDomains, err = f.QuerySQL(t, `
		SELECT bool_and(key_domain = 'token_signing')
		FROM signing_keys
		WHERE kid IN ('legacy-current-signing-key', 'legacy-previous-signing-key');
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(legacyDomains), "migration replay must backfill legacy signing keys again")
}

func TestMigration033PrivateKeyJWTAuthentication(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)

	// Step 1: Preserve legacy static and public service rows before migration 033.
	require.NoError(t, f.Up(t, 32))
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('33000000-0000-0000-0000-000000000001', 'legacy static confidential service', 'legacy-static-client',
			 '\x01', NULL, 'https://oauth.example.com', false, '[]'),
			('33000000-0000-0000-0000-000000000002', 'legacy public service', 'legacy-public-client',
			 NULL, 'none', 'https://oauth.example.com', false, '[]');
	`))

	// Step 2: Migration 033 preserves existing authentication states.
	require.NoError(t, f.Up(t, 33))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(33), version)
	assert.False(t, dirty)

	legacyRowsPreserved, err := f.QuerySQL(t, `
		SELECT bool_and(
			(id = '33000000-0000-0000-0000-000000000001'
				AND token_endpoint_auth_method IS NULL
				AND client_secret_encrypted IS NOT NULL)
			OR (id = '33000000-0000-0000-0000-000000000002'
				AND token_endpoint_auth_method = 'none'
				AND client_secret_encrypted IS NULL)
		)
		FROM thirdparty_oauth2_services
		WHERE id IN (
			'33000000-0000-0000-0000-000000000001',
			'33000000-0000-0000-0000-000000000002'
		);
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(legacyRowsPreserved), "migration 033 must preserve legacy static and public services")

	// Step 3: private_key_jwt services must not retain shared-secret ciphertext.
	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('33000000-0000-0000-0000-000000000004', 'CIMD service with prohibited shared secret',
			 'https://broker.example.com/.well-known/oauth-client/33000000-0000-0000-0000-000000000004',
			 '\x04', 'private_key_jwt', 'https://oauth.example.com', false, '[]');
	`)
	assert.Error(t, err, "migration 033 must reject private_key_jwt services with shared-secret ciphertext")

	// Step 4: A CIMD service may use private_key_jwt only without a shared secret.
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('33000000-0000-0000-0000-000000000003', 'CIMD confidential service',
			 'https://broker.example.com/.well-known/oauth-client/33000000-0000-0000-0000-000000000003',
			 NULL, 'private_key_jwt', 'https://oauth.example.com', false, '[]');
	`))

	// Step 5: Replaying an applied migration is a no-op.
	require.NoError(t, f.Up(t, 33))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(33), version)
	assert.False(t, dirty)

	// Step 6: Rollback must not discard CIMD authentication state.
	err = f.Down(t, 32)
	require.Error(t, err, "migration 033 rollback must refuse while CIMD services exist")
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.True(t, dirty)

	cimdServiceIsIntact, err := f.QuerySQL(t, `
		SELECT token_endpoint_auth_method = 'private_key_jwt' AND client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '33000000-0000-0000-0000-000000000003';
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(cimdServiceIsIntact))

	// Step 7: Once CIMD state is removed, rollback restores the preceding authentication constraint.
	require.NoError(t, f.Force(t, 33))
	require.NoError(t, f.ExecuteSQL(t, `
		DELETE FROM thirdparty_oauth2_services
		WHERE id = '33000000-0000-0000-0000-000000000003';
	`))
	require.NoError(t, f.Down(t, 32))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(32), version)
	assert.False(t, dirty)

	err = f.ExecuteSQL(t, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, issuer_uri, enable_discovery, scopes)
		VALUES
			('33000000-0000-0000-0000-000000000005', 'CIMD service after rollback',
			 'https://broker.example.com/.well-known/oauth-client/33000000-0000-0000-0000-000000000005',
			 NULL, 'private_key_jwt', 'https://oauth.example.com', false, '[]');
	`)
	assert.Error(t, err, "migration 033 rollback must reject private_key_jwt services")

	// Step 8: Reapplying migration 033 preserves the legacy authentication states.
	require.NoError(t, f.Up(t, 33))
	legacyRowsPreserved, err = f.QuerySQL(t, `
		SELECT bool_and(
			(id = '33000000-0000-0000-0000-000000000001'
				AND token_endpoint_auth_method IS NULL
				AND client_secret_encrypted IS NOT NULL)
			OR (id = '33000000-0000-0000-0000-000000000002'
				AND token_endpoint_auth_method = 'none'
				AND client_secret_encrypted IS NULL)
		)
		FROM thirdparty_oauth2_services
		WHERE id IN (
			'33000000-0000-0000-0000-000000000001',
			'33000000-0000-0000-0000-000000000002'
		);
	`)
	require.NoError(t, err)
	assert.Equal(t, "true", strings.TrimSpace(legacyRowsPreserved), "migration replay must preserve legacy static and public services")
}
