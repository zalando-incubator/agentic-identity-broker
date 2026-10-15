//go:build integration

package migrations_test

import (
	"context"
	"net/url"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestSessionUpsertRemainsAvailableDuringExpansionRollback(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 39))
	requireSessionValidityPhase(t, f, 39)
	seedSessionExpiryMigration(t, f)
	connection, err := url.Parse(f.connStr)
	require.NoError(t, err)
	query := connection.Query()
	query.Set("application_name", "upsert-schema-rollback")
	connection.RawQuery = query.Encode()
	var config ports.StorageConfig
	config.Backend = "postgres"
	config.Postgres.ConnectionURL = connection.String()
	config.Timeouts.Read = 5 * time.Second
	config.Timeouts.Write = 5 * time.Second
	store, err := storageadapter.NewAdapter(&config)
	require.NoError(t, err)
	ctx := context.Background()
	defer func() { require.NoError(t, store.Close(ctx)) }()
	principal := id.Principal("expiry@example.com")
	serviceID := id.MustParseServiceID("36000000-0000-0000-0000-000000000001")
	before, err := store.UserSessions().FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, before)
	require.NoError(t, f.ExecuteSQL(t, `
		CREATE FUNCTION upsert_rollback_test_gate() RETURNS event_trigger LANGUAGE plpgsql AS $$
		BEGIN
			PERFORM pg_advisory_lock(390041);
			PERFORM pg_advisory_unlock(390041);
		END $$;
		CREATE EVENT TRIGGER upsert_rollback_test_gate ON ddl_command_end
			WHEN TAG IN ('ALTER TABLE') EXECUTE FUNCTION upsert_rollback_test_gate();
	`))
	gate, err := f.db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = gate.Close() }()
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_lock(390041)`)
	require.NoError(t, err)
	defer func() { _, _ = gate.ExecContext(ctx, `SELECT pg_advisory_unlock(390041)`) }()
	rolledBack := make(chan error, 1)
	go func() { rolledBack <- f.Down(t, 38) }()
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objid = 390041
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database()))`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "rollback must hold metadata locks after dropping the flag")
	expiry := time.Now().Add(time.Hour)
	reauthorized := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: []byte("reauthorized-access"), EncryptedRefreshToken: []byte("reauthorized-refresh"),
		TokenType: "Bearer", AccessTokenExpiresAt: &expiry, Scope: []string{"updated"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	upserted := make(chan error, 1)
	go func() { upserted <- store.UserSessions().Create(ctx, reauthorized) }()
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND application_name = 'upsert-schema-rollback'
			AND wait_event_type = 'Lock')`).Scan(&waiting)
		return err == nil && waiting
	}, time.Second, 10*time.Millisecond, "a real repository upsert must overlap the rollback")
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_unlock(390041)`)
	require.NoError(t, err)
	require.NoError(t, <-rolledBack)
	require.NoError(t, <-upserted, "upsert must select the restored function only after its schema fence is acquired")
	requireSessionValidityPhase(t, f, 38)
	after, err := store.UserSessions().FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.Equal(t, before.ID, after.ID)
	require.True(t, before.CreatedAt.Equal(after.CreatedAt))
	require.True(t, before.InitiatedAt.Equal(after.InitiatedAt))
	require.Equal(t, reauthorized.EncryptedAccessToken, after.EncryptedAccessToken)
	require.Equal(t, reauthorized.EncryptedRefreshToken, after.EncryptedRefreshToken)
	require.True(t, expiry.Equal(*after.AccessTokenExpiresAt))
	require.Equal(t, reauthorized.Scope, after.Scope)
	require.False(t, after.RefreshTokenRejected)
	require.NoError(t, f.ExecuteSQL(t, `DROP EVENT TRIGGER upsert_rollback_test_gate; DROP FUNCTION upsert_rollback_test_gate()`))
	t.Log("real preparation upsert waited at the schema boundary and completed after DOWN 039 without callback persistence failure")
}
