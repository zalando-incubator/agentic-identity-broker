//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type restoredRefreshSnapshot struct {
	active, terminal *storage.RefreshSession
	serviceID        id.ServiceID
	signingID        id.SigningKeyID
	legacy           []string
	ciphertext       []byte
	preserved        map[string]string
}

type encryptedRestoreSessionRepo struct {
	ports.RefreshSessionRepository
	ciphertext []byte
}

func (r encryptedRestoreSessionRepo) Save(ctx context.Context, root *storage.RefreshSession) error {
	root.RetryCiphertext = r.ciphertext
	return r.RefreshSessionRepository.Save(ctx, root)
}

func restoreCommandBinary(t *testing.T) string {
	t.Helper()
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "agentic-identity-broker")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/agentic-identity-broker")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "build actual broker: %s", output)
	return binary
}

func restoreConnectionURL(t *testing.T, db *sqlx.DB) string {
	t.Helper()
	var database string
	require.NoError(t, db.Get(&database, `SELECT current_database()`))
	return bootstrap.RequireSharedPostgres(t).ConnectionString(database)
}

func restoreCommandConfig(t *testing.T, url string) string {
	t.Helper()
	// Both ports are already bound: a normal server startup cannot pass this test.
	listeners := make([]net.Listener, 0, 2)
	for range 2 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		listeners = append(listeners, listener)
	}
	t.Cleanup(func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	})
	path := filepath.Join(t.TempDir(), "restore.yaml")
	cfg := fmt.Sprintf(`server:
  enduser:
    bind: "127.0.0.1"
    port: %d
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    bind: "127.0.0.1"
    port: %d
    authentication:
      preauth:
        principal_header_name: X-Remote-User
storage:
  backend: postgres
  postgres:
    connection_url: %q
oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 1h
    refresh_token_ttl: 2h
    refresh_token_reuse_interval: 30s
third_party_oauth2:
  jwe_signing_key: %q
encryption:
  memory:
    raw_key: %q
`, listeners[0].Addr().(*net.TCPAddr).Port, listeners[1].Addr().(*net.TCPAddr).Port,
		url, base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")), testutil.TestKEKBase64)
	require.NoError(t, os.WriteFile(path, []byte(cfg), 0o600))
	return path
}

func runRestoreCommand(t *testing.T, binary, configPath, databaseID string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--config", configPath, "refresh-sessions", "invalidate-restored", "--database-id", databaseID)
	cmd.Env = append(os.Environ(), "IDENTITY_BROKER_CONFIG_PATH="+configPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "offline restore command must terminate; output: %s", output)
	return string(output), err
}

func readRestorePreserved(t *testing.T, db *sqlx.DB, active *storage.RefreshSession, serviceID id.ServiceID, signingID id.SigningKeyID) map[string]string {
	t.Helper()
	rows := map[string]string{
		"agent":       `SELECT to_jsonb(row)::text FROM (SELECT * FROM agents WHERE id = $1) row`,
		"grant":       `SELECT to_jsonb(row)::text FROM (SELECT * FROM user_grants WHERE id = $1) row`,
		"credential":  `SELECT to_jsonb(row)::text FROM (SELECT * FROM client_credentials WHERE agent_id = $1) row`,
		"signing_key": `SELECT to_jsonb(row)::text FROM (SELECT * FROM signing_keys WHERE id = $1) row`,
		"provider":    `SELECT to_jsonb(row)::text FROM (SELECT * FROM thirdparty_oauth2_services WHERE id = $1) row`,
		"upstream":    `SELECT to_jsonb(row)::text FROM (SELECT * FROM user_sessions WHERE service_id = $1) row`,
	}
	ids := map[string]any{"agent": active.AgentID, "grant": active.OriginalGrantID, "credential": active.AgentID,
		"signing_key": signingID, "provider": serviceID, "upstream": serviceID}
	for name, query := range rows {
		var state string
		require.NoError(t, db.Get(&state, query, ids[name]), "snapshot %s", name)
		rows[name] = state
	}
	return rows
}

func createRestoredRefreshSnapshot(t *testing.T, store *storageadapter.Adapter, db *sqlx.DB) restoredRefreshSnapshot {
	t.Helper()
	ctx := context.Background()
	now, err := store.AuthorizationClock().Now(ctx)
	require.NoError(t, err)
	active := createRolloutRoot(t, store, db, now.UTC().Add(-3*time.Second))
	terminal := createRolloutRoot(t, store, db, now.UTC().Add(-3*time.Second))
	cipher := testutil.NewTestEncryptionAdapter(t)
	ciphertext, err := cipher.Encrypt(ctx, []byte("encrypted retry result for restored family"),
		encryption.NewRefreshSessionBranchKeySubject(active.ID).EncryptionContext())
	require.NoError(t, err)
	previous := active.CurrentSignature
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, active.AgentID, func(scope context.Context, at time.Time) error {
		return rotateContractRoot(scope, encryptedRestoreSessionRepo{store.RefreshSessions(), ciphertext},
			store.RefreshTokens(), active.ID, previous, refreshContractSignature("restored-current-"+active.ID.String()), at)
	}))
	require.NoError(t, store.AuthorizationCoordinator().Run(ctx, terminal.AgentID, func(scope context.Context, at time.Time) error {
		return store.RefreshRevocations().RevokeByID(scope, terminal.ID, at, storage.RefreshReasonGrantDeleted)
	}))
	oldDescendant := refreshContractSignature("restored-old-descendant-" + active.ID.String())
	unanchored := refreshContractSignature("restored-unanchored-" + active.ID.String())
	terminalLegacy := refreshContractSignature("restored-terminal-legacy-" + terminal.ID.String())
	rootless := refreshContractSignature("restored-rootless-" + active.ID.String())
	rootlessAgent := id.NewAgentID()
	_, err = db.ExecContext(ctx, `INSERT INTO agents (id, client_id, display_name, description)
		VALUES ($1, $2, 'Rootless old writer', 'No refresh root')`, rootlessAgent, "rootless-"+rootlessAgent.String())
	require.NoError(t, err)
	legacy := []string{previous, refreshContractSignature("restored-current-" + active.ID.String()), oldDescendant, unanchored, terminalLegacy, rootless}
	for _, row := range []struct {
		signature, requestID string
		agent                id.AgentID
		client               id.ClientID
		principal            id.Principal
	}{
		{oldDescendant, active.ID.String(), active.AgentID, active.ClientID, active.Principal},
		{unanchored, id.NewRefreshSessionID().String(), active.AgentID, active.ClientID, active.Principal},
		{terminalLegacy, terminal.ID.String(), terminal.AgentID, terminal.ClientID, terminal.Principal},
		{rootless, id.NewRefreshSessionID().String(), rootlessAgent, id.NewClientID("rootless-" + rootlessAgent.String()), id.NewPrincipal("rootless@example.test")},
	} {
		_, err = db.ExecContext(ctx, `INSERT INTO refresh_token_sessions
			(signature, request_id, agent_id, client_id, principal, expires_at)
			VALUES ($1,$2,$3,$4,$5,$6)`, row.signature, row.requestID, row.agent, row.client, row.principal, now.Add(time.Hour))
		require.NoError(t, err)
	}
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: active.AgentID,
		SecretHash: "already-hashed-test-secret", CreatedAt: now}
	require.NoError(t, store.BrokerCredentials().Create(ctx, credential))
	serviceID := id.NewServiceID()
	signingService := oauth2server.NewSigningKeyService(store.SigningKeys(), store.SigningKeyBootstrapCoordinator(),
		cipher, &encryptionnoop.BranchKeyManager{}, slog.Default())
	signingKey, created, err := signingService.EnsureInitialKey(ctx, "ES256")
	require.NoError(t, err)
	require.True(t, created)
	provider := fixtures.ServiceWithID(serviceID.String())
	provider.DisplayName = "Unrelated provider"
	provider.ClientID = id.NewClientID("provider-client")
	require.NoError(t, store.Services().Create(ctx, provider))
	upstreamCipher, err := cipher.Encrypt(ctx, []byte("preserved upstream refresh"), map[string]string{"service_id": serviceID.String()})
	require.NoError(t, err)
	upstream := &storage.UserSession{
		ID: id.NewSessionID(), Principal: active.Principal, ServiceID: serviceID,
		EncryptedAccessToken: upstreamCipher, EncryptedRefreshToken: upstreamCipher,
		TokenType: "Bearer", Scope: []string{"read"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, store.UserSessions().Create(ctx, upstream))
	_, err = store.Services().Get(ctx, serviceID)
	require.NoError(t, err)
	_, err = store.UserSessions().Get(ctx, upstream.ID)
	require.NoError(t, err)
	return restoredRefreshSnapshot{active: active, terminal: terminal, serviceID: serviceID, signingID: signingKey.ID,
		legacy: legacy, ciphertext: ciphertext, preserved: readRestorePreserved(t, db, active, serviceID, signingKey.ID)}
}

func assertRestoreSnapshot(t *testing.T, store *storageadapter.Adapter, db *sqlx.DB, snapshot restoredRefreshSnapshot, succeeded bool) {
	t.Helper()
	ctx := context.Background()
	active, err := store.RefreshSessions().FindByID(ctx, snapshot.active.ID)
	require.NoError(t, err)
	terminal, err := store.RefreshSessions().FindByID(ctx, snapshot.terminal.ID)
	require.NoError(t, err)
	if succeeded {
		require.Equal(t, storage.RefreshReasonRestoreInvalidation, *active.TerminalReason)
		require.NotNil(t, active.RevokedAt)
		require.Empty(t, active.RetryCiphertext)
		require.Equal(t, storage.RefreshReasonGrantDeleted, *terminal.TerminalReason)
		for _, signature := range snapshot.legacy {
			var usedAt *time.Time
			require.NoError(t, db.GetContext(ctx, &usedAt, `SELECT used_at FROM refresh_token_sessions WHERE signature = $1`, signature))
			require.NotNil(t, usedAt, "unused legacy authority: %s", signature)
		}
		var remaining bool
		require.NoError(t, db.GetContext(ctx, &remaining, `SELECT EXISTS (
			SELECT 1 FROM refresh_sessions WHERE terminal_reason IS NULL OR retry_ciphertext IS NOT NULL
			UNION ALL SELECT 1 FROM refresh_token_sessions WHERE used_at IS NULL)`))
		require.False(t, remaining, "zero exit requires no local refresh authority")
	} else {
		require.Nil(t, active.TerminalReason, "failed agent scope must not claim revocation")
		require.Equal(t, snapshot.ciphertext, active.RetryCiphertext)
	}
	var originalReceipt, restoreReceipt int
	require.NoError(t, db.GetContext(ctx, &originalReceipt,
		`SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'grant_deleted'`, terminal.ID))
	require.Equal(t, 1, originalReceipt, "existing independent receipt must survive")
	require.NoError(t, db.GetContext(ctx, &restoreReceipt,
		`SELECT count(*) FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'restore_invalidation'`, terminal.ID))
	require.Zero(t, restoreReceipt, "terminal reason and receipt must never be overwritten")
	require.Equal(t, snapshot.preserved, readRestorePreserved(t, db, snapshot.active, snapshot.serviceID, snapshot.signingID),
		"restore must not modify grants, agent, credentials, keys, providers, or upstream sessions")
}

func TestRefreshRestoreCommand(t *testing.T) {
	binary := restoreCommandBinary(t)
	t.Run("memory backend cannot acknowledge persistent restore", func(t *testing.T) {
		store, db := newRolloutStorage(t)
		config := restoreCommandConfig(t, restoreConnectionURL(t, db))
		content, err := os.ReadFile(config)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(config, []byte(strings.Replace(string(content), "backend: postgres", "backend: memory", 1)), 0o600))
		databaseID, err := store.RefreshMaintenanceControl().DatabaseID(t.Context())
		require.NoError(t, err)
		output, err := runRestoreCommand(t, binary, config, databaseID)
		require.Error(t, err)
		require.Contains(t, output, "requires PostgreSQL storage")
	})
	t.Run("wrong database target cannot acknowledge restore", func(t *testing.T) {
		expected, _ := newRolloutStorage(t)
		wrong, db := newRolloutStorage(t)
		// Cloned fixture databases inherit one template UUID; make this an independent target.
		_, err := db.ExecContext(t.Context(), "UPDATE refresh_maintenance SET database_id = gen_random_uuid()")
		require.NoError(t, err)
		databaseID, err := expected.RefreshMaintenanceControl().DatabaseID(t.Context())
		require.NoError(t, err)
		now, err := wrong.AuthorizationClock().Now(t.Context())
		require.NoError(t, err)
		root := createRolloutRoot(t, wrong, db, now.Add(-time.Second))
		config := restoreCommandConfig(t, restoreConnectionURL(t, db))
		output, err := runRestoreCommand(t, binary, config, databaseID)
		require.Error(t, err)
		require.Contains(t, output, "does not match --database-id")
		unchanged, err := wrong.RefreshSessions().FindByID(t.Context(), root.ID)
		require.NoError(t, err)
		require.Nil(t, unchanged.TerminalReason, "wrong-target failure must precede any invalidation")
	})

	t.Run("offline invalidation and idempotent rerun", func(t *testing.T) {
		store, db := newRolloutStorage(t)
		snapshot := createRestoredRefreshSnapshot(t, store, db)
		config := restoreCommandConfig(t, restoreConnectionURL(t, db))
		databaseID, err := store.RefreshMaintenanceControl().DatabaseID(t.Context())
		require.NoError(t, err)
		output, err := runRestoreCommand(t, binary, config, databaseID)
		require.NoError(t, err, "acknowledged restore must succeed offline: %s", output)
		require.NotContains(t, output, "encrypted retry result")
		assertRestoreSnapshot(t, store, db, snapshot, true)
		var receiptsBefore int
		require.NoError(t, db.Get(&receiptsBefore, `SELECT count(*) FROM refresh_revocation_receipts`))
		output, err = runRestoreCommand(t, binary, config, databaseID)
		require.NoError(t, err, "idempotent rerun must succeed: %s", output)
		assertRestoreSnapshot(t, store, db, snapshot, true)
		var receiptsAfter int
		require.NoError(t, db.Get(&receiptsAfter, `SELECT count(*) FROM refresh_revocation_receipts`))
		require.Equal(t, receiptsBefore, receiptsAfter)
	})

	for _, fault := range []struct {
		name, install, remove string
	}{
		{"receipt", `ALTER TABLE refresh_revocation_receipts ADD CONSTRAINT reject_restore_receipt CHECK (reason <> 'restore_invalidation')`,
			`ALTER TABLE refresh_revocation_receipts DROP CONSTRAINT reject_restore_receipt`},
		{"legacy mirror", `CREATE FUNCTION reject_restore_mirror() RETURNS TRIGGER LANGUAGE plpgsql AS $$
			BEGIN RAISE EXCEPTION 'reject restored mirror'; END; $$;
			CREATE TRIGGER reject_restore_mirror BEFORE UPDATE OF used_at ON refresh_token_sessions
			FOR EACH ROW WHEN (OLD.used_at IS NULL) EXECUTE FUNCTION reject_restore_mirror()`,
			`DROP TRIGGER reject_restore_mirror ON refresh_token_sessions; DROP FUNCTION reject_restore_mirror()`},
	} {
		t.Run(fault.name+" write fails closed then reruns", func(t *testing.T) {
			store, db := newRolloutStorage(t)
			snapshot := createRestoredRefreshSnapshot(t, store, db)
			config := restoreCommandConfig(t, restoreConnectionURL(t, db))
			databaseID, err := store.RefreshMaintenanceControl().DatabaseID(t.Context())
			require.NoError(t, err)
			_, err = db.Exec(fault.install)
			require.NoError(t, err)
			output, err := runRestoreCommand(t, binary, config, databaseID)
			require.Error(t, err, "failed agent transaction must block admission: %s", output)
			assertRestoreSnapshot(t, store, db, snapshot, false)
			_, err = db.Exec(fault.remove)
			require.NoError(t, err)
			output, err = runRestoreCommand(t, binary, config, databaseID)
			require.NoError(t, err, "acknowledged rerun must complete after fault removal: %s", output)
			assertRestoreSnapshot(t, store, db, snapshot, true)
		})
	}

	t.Run("lost COMMIT acknowledgement fails despite durable revocation", func(t *testing.T) {
		store, db := newRolloutStorage(t)
		snapshot := createRestoredRefreshSnapshot(t, store, db)
		fault, err := helpers.NewRefreshCommitFault(restoreConnectionURL(t, db))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, fault.Close()) })
		config := restoreCommandConfig(t, fault.ConnectionURL())
		databaseID, err := store.RefreshMaintenanceControl().DatabaseID(t.Context())
		require.NoError(t, err)
		commit, err := fault.ArmCommitLoss()
		require.NoError(t, err)
		output, err := runRestoreCommand(t, binary, config, databaseID)
		require.Error(t, err, "indeterminate COMMIT must not admit traffic: %s", output)
		select {
		case acknowledged := <-commit:
			require.True(t, acknowledged, "the test must lose an acknowledged durable COMMIT, not a pre-commit error")
		case <-time.After(2 * time.Second):
			t.Fatal("restore operation did not reach the protocol COMMIT fault")
		}
		// The direct connection observes durable state independently of the fault relay.
		var committedInvalidation bool
		require.NoError(t, db.Get(&committedInvalidation, `SELECT EXISTS (
			SELECT 1 FROM refresh_revocation_receipts WHERE reason = 'restore_invalidation'
			UNION ALL SELECT 1 FROM refresh_token_sessions WHERE used_at IS NOT NULL AND signature IN ($1, $2)
		)`, refreshContractSignature("restored-rootless-"+snapshot.active.ID.String()),
			refreshContractSignature("restored-terminal-legacy-"+snapshot.terminal.ID.String())))
		require.True(t, committedInvalidation, "the direct connection must observe the committed owner invalidation")
		config = restoreCommandConfig(t, restoreConnectionURL(t, db))
		output, err = runRestoreCommand(t, binary, config, databaseID)
		require.NoError(t, err, "acknowledged rerun must finish unfinished agents: %s", output)
		assertRestoreSnapshot(t, store, db, snapshot, true)
	})
}
