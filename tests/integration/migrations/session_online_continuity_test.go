//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	e2ebootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func TestOnlineSessionValidityAcrossReplicas(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 37))
	cli := newOnlineMigrationCLI(t, f)
	require.NoError(t, cli.Run(38))
	requireSessionValidityPhase(t, f, 38)
	upstream := helpers.NewMockUpstreamOAuth2Server()
	defer upstream.Close()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	stores := make([]*storageadapter.Adapter, 2)
	servers := make([]*e2ebootstrap.TestServer, 2)
	for i := range stores {
		config := fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
		config.ThirdPartyOAuth2.SessionAPICompatibility = i == 0
		config.Storage.Backend = "postgres"
		connection, err := url.Parse(f.connStr)
		require.NoError(t, err)
		query := connection.Query()
		query.Set("application_name", fmt.Sprintf("session-online-%d", i))
		connection.RawQuery = query.Encode()
		config.Storage.Postgres.ConnectionURL = connection.String()
		config.TokenRefresh.LookaheadDuration = 5 * time.Minute
		config.TokenRefresh.BackgroundWorkers = 1
		config.TokenRefresh.Sweep.DefaultPageSize = 100
		stores[i], err = storageadapter.NewAdapter(&config.Storage)
		require.NoError(t, err)
		defer func(store *storageadapter.Adapter) { require.NoError(t, store.Close(context.Background())) }(stores[i])
		builder, err := e2ebootstrap.NewTestServerBuilder(config, stores[i], e2ebootstrap.NewServerFactory(config, logger), logger)
		require.NoError(t, err)
		servers[i], err = builder.Build()
		require.NoError(t, err)
		defer servers[i].Close()
	}
	ctx := context.Background()
	service := fixtures.GitHubService()
	service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
	service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
	require.NoError(t, stores[0].Services().Create(ctx, service))
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, stores[0], service.ID))
	agent := fixtures.ValidAgent()
	require.NoError(t, stores[0].Agents().Create(ctx, agent))
	healthy, rejected := "online-healthy@example.com", "online-rejected@example.com"
	for _, principal := range []string{healthy, rejected} {
		require.NoError(t, stores[0].UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), service.ID.String(), []string{"repo", "user"})))
		require.NoError(t, stores[0].UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal)))
	}
	before, err := stores[0].UserSessions().FindByPrincipalAndService(ctx, id.Principal(rejected), service.ID)
	require.NoError(t, err)
	healthyForm := onlineExchangeForm(t, upstream, healthy, agent)
	rejectedForm := onlineExchangeForm(t, upstream, rejected, agent)
	probes := onlineSessionProbes(servers, healthy, service.ID, healthyForm, "github-token-xyz", false)
	probes = append(probes, onlineSessionProbes(servers, rejected, service.ID, rejectedForm, "github-token-xyz", false)...)
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	healthyBefore, err := stores[0].UserSessions().FindByPrincipalAndService(ctx, id.Principal(healthy), service.ID)
	require.NoError(t, err)
	upstream.WithSuccessfulTokenResponse()
	response, err := servers[0].AuthenticatedPOST("/api/third-party/"+service.ID.String()+"/session/refresh", healthy, "application/json", nil)
	result := readOnlineHTTPResult(response, err)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status)
	require.Len(t, upstream.GetTokenRequests(), 1, "preparation must support a real successful provider refresh")
	for _, store := range stores {
		after, err := store.UserSessions().FindByPrincipalAndService(ctx, id.Principal(healthy), service.ID)
		require.NoError(t, err)
		require.False(t, after.RefreshTokenRejected)
		require.Equal(t, healthyBefore.ID, after.ID)
		require.True(t, healthyBefore.InitiatedAt.Equal(after.InitiatedAt))
		require.True(t, healthyBefore.CreatedAt.Equal(after.CreatedAt))
		require.NotEqual(t, healthyBefore.EncryptedAccessToken, after.EncryptedAccessToken)
		require.NotNil(t, after.AccessTokenExpiresAt)
		require.True(t, after.AccessTokenExpiresAt.After(time.Now()))
	}
	probes = onlineSessionProbes(servers, healthy, service.ID, healthyForm, "mock-access-token", false)
	probes = append(probes, onlineSessionProbes(servers, rejected, service.ID, rejectedForm, "github-token-xyz", false)...)
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	beforeRepeat := sessionValiditySnapshot(t, f, "")
	require.NoError(t, cli.Run(38), "repeating the preparation target must be a no-op")
	requireSessionValidityPhase(t, f, 38)
	require.JSONEq(t, beforeRepeat, sessionValiditySnapshot(t, f, ""))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	require.Equal(t, uint(38), version)
	require.False(t, dirty)
	require.Len(t, upstream.GetTokenRequests(), 1)
	upstream.Reset()

	// Only this disposable database has the gate. It holds the migration's short
	// metadata lock long enough to deterministically put real HTTP requests in flight.
	require.NoError(t, f.ExecuteSQL(t, `
		CREATE FUNCTION session_migration_test_gate() RETURNS event_trigger LANGUAGE plpgsql AS $$
		BEGIN
			PERFORM pg_advisory_lock(390040);
			PERFORM pg_advisory_unlock(390040);
		END $$;
		CREATE EVENT TRIGGER session_migration_test_gate ON ddl_command_end
			WHEN TAG IN ('ALTER TABLE') EXECUTE FUNCTION session_migration_test_gate();
	`))
	for _, relation := range []string{"user_sessions", "user_session_state"} {
		requireBusySessionPhase(t, f, 39, relation, probes, cli.Run)
	}
	applySessionPhaseWithTraffic(t, f, 39, probes, cli.Run)
	requireSessionValidityPhase(t, f, 39)
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}

	upstream.WithErrorResponse("invalid_grant")
	release := upstream.WithTokenResponseGate()
	defer release()
	results := make(chan onlineHTTPResult, len(servers))
	for _, server := range servers {
		go func(server *e2ebootstrap.TestServer) {
			response, err := server.AuthenticatedPOST("/api/third-party/"+service.ID.String()+"/session/refresh", rejected, "application/json", nil)
			results <- readOnlineHTTPResult(response, err)
		}(server)
	}
	require.Eventually(t, func() bool { return len(upstream.GetTokenRequests()) == 1 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND application_name LIKE 'session-online-%'
			AND wait_event_type = 'Lock' AND query LIKE '%FOR UPDATE%')`).Scan(&waiting)
		return err == nil && waiting
	}, time.Second, 10*time.Millisecond, "the other replica must contend on the original session row")
	release()
	for range servers {
		result := <-results
		require.NoError(t, result.err)
		require.Equal(t, http.StatusConflict, result.status)
		require.Equal(t, "refresh_rejected", result.body["error"])
		require.Equal(t, "refresh token rejected by provider; reauthentication required", result.body["message"])
	}
	require.Len(t, upstream.GetTokenRequests(), 1, "row locking must serialize independent applications, not only in-process refresh flights")
	for _, store := range stores {
		after, err := store.UserSessions().FindByPrincipalAndService(ctx, id.Principal(rejected), service.ID)
		require.NoError(t, err)
		require.True(t, after.RefreshTokenRejected)
		require.Equal(t, before.ID, after.ID)
		require.Equal(t, before.EncryptedAccessToken, after.EncryptedAccessToken)
		require.Equal(t, before.EncryptedRefreshToken, after.EncryptedRefreshToken)
		require.True(t, before.AccessTokenExpiresAt.Equal(*after.AccessTokenExpiresAt))
	}
	probes = onlineSessionProbes(servers, healthy, service.ID, healthyForm, "mock-access-token", false)
	probes = append(probes, onlineSessionProbes(servers, rejected, service.ID, rejectedForm, "", true)...)
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	for _, server := range servers {
		response, err := server.AuthenticatedPOST("/api/third-party/"+service.ID.String()+"/session/refresh", rejected, "application/json", nil)
		result := readOnlineHTTPResult(response, err)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusConflict, result.status)
		require.Equal(t, "refresh_rejected", result.body["error"])
	}
	require.Len(t, upstream.GetTokenRequests(), 1, "no replica may retry provider-rejected authority after activation")
	beforeStaleActivation := sessionValiditySnapshot(t, f, "")
	require.NoError(t, cli.Run(38), "a stale preparation target must never downgrade an already-activated database")
	requireSessionValidityPhase(t, f, 39)
	require.JSONEq(t, beforeStaleActivation, sessionValiditySnapshot(t, f, ""))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	require.Equal(t, uint(39), version)
	require.False(t, dirty)
	requireOnlineSessionTraffic(t, probes)
	for _, relation := range []string{"user_sessions", "user_session_state"} {
		requireBusySessionPhase(t, f, 40, relation, probes, cli.Run)
	}
	applySessionPhaseWithTraffic(t, f, 40, probes, cli.Run)
	requireSessionValidityPhase(t, f, 40)
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	for _, server := range servers {
		response, err := server.AuthenticatedPOST("/api/third-party/"+service.ID.String()+"/session/refresh", rejected, "application/json", nil)
		result := readOnlineHTTPResult(response, err)
		require.NoError(t, result.err)
		require.Equal(t, http.StatusConflict, result.status)
		require.Equal(t, "refresh_rejected", result.body["error"])
	}
	require.Len(t, upstream.GetTokenRequests(), 1, "no replica may retry provider-rejected authority after cleanup")
	beforeStaleStage := sessionValiditySnapshot(t, f, "")
	require.NoError(t, cli.Run(38), "a stale preparation target must never downgrade an already-contracted database")
	requireSessionValidityPhase(t, f, 40)
	require.JSONEq(t, beforeStaleStage, sessionValiditySnapshot(t, f, ""))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	require.Equal(t, uint(40), version)
	require.False(t, dirty)
	requireOnlineSessionTraffic(t, probes)
	require.Len(t, upstream.GetTokenRequests(), 1, "stale migration targets must not reactivate rejected authority")
	require.NoError(t, f.ExecuteSQL(t, `DROP EVENT TRIGGER session_migration_test_gate; DROP FUNCTION session_migration_test_gate()`))
	t.Log("real migration image and rendered Helm upgrade runner applied 038/039/040 and treated target038 at039/040 as a no-op; two preparation replicas served authenticated list/detail and RFC8693 exchanges through activation, contraction, and a successful provider refresh; NOWAIT table/view contention preserved traffic and dirty metadata; rejection retained ciphertext/access expiry, returned 409 on both replicas, denied later issuance, and made exactly one provider rejection request")
}

type onlineHTTPResult struct {
	status int
	body   map[string]any
	err    error
}

func readOnlineHTTPResult(response *http.Response, err error) onlineHTTPResult {
	if err != nil {
		return onlineHTTPResult{err: err}
	}
	defer func() { _ = response.Body.Close() }()
	result := onlineHTTPResult{status: response.StatusCode}
	result.err = json.NewDecoder(response.Body).Decode(&result.body)
	return result
}

func onlineExchangeForm(t *testing.T, upstream *helpers.MockUpstreamOAuth2Server, principal string, agent *storage.Agent) url.Values {
	t.Helper()
	tokens, err := helpers.BuildTokenExchangeJWTs(upstream.GetPrivateKeyPEM(), upstream.URL(),
		"token-exchange-broker", principal, agent.ID.String(), "test-gateway-client", time.Now())
	require.NoError(t, err)
	return url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:token-exchange"}, "subject_token": {tokens.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {tokens.ClientAssertion}, "resource": {"https://api.github.com"},
	}
}

func onlineSessionProbes(servers []*e2ebootstrap.TestServer, principal string, serviceID id.ServiceID, form url.Values, accessToken string, rejected bool) []func() error {
	probes := make([]func() error, 0, len(servers)*3)
	for index, server := range servers {
		for _, path := range []string{"/api/third-party/sessions", "/api/third-party/" + serviceID.String() + "/session"} {
			probes = append(probes, func() error {
				response, err := server.AuthenticatedGET(path, principal)
				return checkOnlineSession(readOnlineHTTPResult(response, err), path, rejected, index == 0)
			})
		}
		probes = append(probes, func() error {
			response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
			return checkOnlineExchange(readOnlineHTTPResult(response, err), server.BaseURL()+"/sessions", accessToken, rejected)
		})
	}
	return probes
}

func checkOnlineSession(result onlineHTTPResult, path string, rejected, compatibility bool) error {
	if result.err != nil {
		return result.err
	}
	if result.status != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d", path, result.status)
	}
	data, ok := result.body["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s omitted data", path)
	}
	session, ok := data["session"].(map[string]any)
	if path == "/api/third-party/sessions" {
		items, listed := data["sessions"].([]any)
		if !listed || len(items) != 1 {
			return fmt.Errorf("%s did not retain exactly one session", path)
		}
		session, ok = items[0].(map[string]any)
	}
	if !ok || session["requires_reauthentication"] != rejected || session["access_token_expired"] != false || session["has_refresh_token"] != true {
		return fmt.Errorf("%s returned incorrect session validity: %v", path, session)
	}
	legacy, aliased := session["is_expired"]
	if aliased != compatibility || (aliased && legacy != rejected) {
		return fmt.Errorf("%s returned incorrect opt-in compatibility state", path)
	}
	if _, fabricated := session["refresh_token_expires_at"]; fabricated {
		return fmt.Errorf("%s returned a fabricated refresh expiry", path)
	}
	return nil
}

func checkOnlineExchange(result onlineHTTPResult, recoveryURI, accessToken string, rejected bool) error {
	if result.err != nil {
		return result.err
	}
	if rejected {
		_, issued := result.body["access_token"]
		if result.status != http.StatusBadRequest || result.body["error"] != "invalid_grant" || result.body["error_uri"] != recoveryURI || issued {
			return fmt.Errorf("rejected exchange returned HTTP %d instead of token-free invalid_grant recovery", result.status)
		}
	} else if result.status != http.StatusOK || result.body["access_token"] != accessToken {
		return fmt.Errorf("healthy exchange returned HTTP %d without the expected decrypted access token", result.status)
	}
	return nil
}

func startOnlineSessionTraffic(probes []func() error) <-chan error {
	results := make(chan error, len(probes))
	for _, probe := range probes {
		go func() { results <- probe() }()
	}
	return results
}

func requireOnlineSessionTraffic(t *testing.T, probes []func() error) {
	t.Helper()
	results := startOnlineSessionTraffic(probes)
	for range probes {
		require.NoError(t, <-results)
	}
}

func applySessionPhaseWithTraffic(t *testing.T, f *MigrationTestFramework, phase uint, probes []func() error, migrate func(uint) error) {
	t.Helper()
	ctx := context.Background()
	gate, err := f.db.Conn(ctx)
	require.NoError(t, err)
	defer func() { _ = gate.Close() }()
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_lock(390040)`)
	require.NoError(t, err)
	defer func() { _, _ = gate.ExecContext(ctx, `SELECT pg_advisory_unlock(390040)`) }()
	migrated := make(chan error, 1)
	go func() { migrated <- migrate(phase) }()
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_locks
			WHERE locktype = 'advisory' AND NOT granted AND objid = 390040
			AND database = (SELECT oid FROM pg_database WHERE datname = current_database()))`).Scan(&waiting)
		return err == nil && waiting
	}, 10*time.Second, 10*time.Millisecond, "migration must reach ALTER TABLE while holding its metadata locks")
	results := startOnlineSessionTraffic(probes)
	require.Eventually(t, func() bool {
		var waiting bool
		err := f.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND application_name LIKE 'session-online-%' AND wait_event_type = 'Lock')`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 10*time.Millisecond, "real application requests must overlap the metadata lock")
	_, err = gate.ExecContext(ctx, `SELECT pg_advisory_unlock(390040)`)
	require.NoError(t, err)
	require.NoError(t, <-migrated)
	for range probes {
		require.NoError(t, <-results, "requests may briefly wait, but must not fail during the online cutover")
	}
}

func requireBusySessionPhase(t *testing.T, f *MigrationTestFramework, phase uint, relation string, probes []func() error, migrate func(uint) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	query := "SELECT id FROM " + relation + " LIMIT 1"
	if relation == "user_session_state" {
		query += " FOR UPDATE"
	}
	_, err = tx.ExecContext(ctx, query)
	require.NoError(t, err)
	before := sessionValiditySnapshot(t, f, "")
	result := make(chan error, 1)
	started := time.Now()
	go func() { result <- migrate(phase) }()
	select {
	case err = <-result:
	case <-time.After(5 * time.Second):
		_ = tx.Rollback()
		<-result
		t.Fatal("NOWAIT migration queued behind a long-lived table transaction")
	}
	elapsed := time.Since(started)
	require.ErrorContains(t, err, "could not obtain lock on relation")
	require.ErrorContains(t, err, relation, "the contended metadata lock must be the one under test")
	requireSessionValidityPhase(t, f, phase-1)
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""), "lock contention must leave all rows and schema unchanged")
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	require.Equal(t, phase, version)
	require.True(t, dirty)
	requireOnlineSessionTraffic(t, probes)
	require.ErrorContains(t, migrate(phase), "Dirty migration metadata", "the real wrapper must refuse dirty metadata rather than retrying DDL or forcing a version")
	requireSessionValidityPhase(t, f, phase-1)
	require.JSONEq(t, before, sessionValiditySnapshot(t, f, ""))
	version, dirty, err = f.Version(t)
	require.NoError(t, err)
	require.Equal(t, phase, version)
	require.True(t, dirty, "dirty abort must never repair metadata automatically")
	// Both the actual phase and dirty metadata have been inspected; running
	// replicas still serve traffic before this deliberate metadata-only repair.
	require.NoError(t, f.Force(t, phase-1))
	require.NoError(t, tx.Rollback())
	t.Logf("migration %03d refused a held %s transaction in %s without schema/data change or application request errors", phase, relation, elapsed)
}
