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

func TestMigration038SessionTrafficAcrossReplicas(t *testing.T) {
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
	principal := "online-healthy@example.com"
	require.NoError(t, stores[0].UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), service.ID.String(), []string{"repo", "user"})))
	require.NoError(t, stores[0].UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal)))
	before, err := stores[0].UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), service.ID)
	require.NoError(t, err)
	form := onlineExchangeForm(t, upstream, principal, agent)
	probes := onlineSessionProbes(servers, principal, service.ID, form, "github-token-xyz")
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	upstream.WithSuccessfulTokenResponse()
	response, err := servers[0].AuthenticatedPOST("/api/third-party/"+service.ID.String()+"/session/refresh", principal, "application/json", nil)
	result := readOnlineHTTPResult(response, err)
	require.NoError(t, result.err)
	require.Equal(t, http.StatusOK, result.status)
	require.Len(t, upstream.GetTokenRequests(), 1, "preparation must support a real successful provider refresh")
	for _, store := range stores {
		after, err := store.UserSessions().FindByPrincipalAndService(ctx, id.Principal(principal), service.ID)
		require.NoError(t, err)
		require.False(t, after.RefreshTokenRejected)
		require.Equal(t, before.ID, after.ID)
		require.True(t, before.InitiatedAt.Equal(after.InitiatedAt))
		require.True(t, before.CreatedAt.Equal(after.CreatedAt))
		require.NotEqual(t, before.EncryptedAccessToken, after.EncryptedAccessToken)
		require.True(t, after.AccessTokenExpiresAt.After(time.Now()))
	}
	probes = onlineSessionProbes(servers, principal, service.ID, form, "mock-access-token")
	for range 3 {
		requireOnlineSessionTraffic(t, probes)
	}
	beforeRepeat := sessionValiditySnapshot(t, f, "")
	require.NoError(t, cli.Run(38), "repeating the deployed preparation target must be a no-op")
	requireSessionValidityPhase(t, f, 38)
	require.JSONEq(t, beforeRepeat, sessionValiditySnapshot(t, f, ""))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	require.Equal(t, uint(38), version)
	require.False(t, dirty)
	requireOnlineSessionTraffic(t, probes)
	require.Len(t, upstream.GetTokenRequests(), 1)
	t.Log("real migration image and rendered Helm upgrade runner applied 038 and repeated it without changes; two Builder replicas served compatibility and modern session responses plus RFC8693 token acquisition before and after a successful provider refresh")
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

func onlineSessionProbes(servers []*e2ebootstrap.TestServer, principal string, serviceID id.ServiceID, form url.Values, accessToken string) []func() error {
	probes := make([]func() error, 0, len(servers)*3)
	for index, server := range servers {
		for _, path := range []string{"/api/third-party/sessions", "/api/third-party/" + serviceID.String() + "/session"} {
			probes = append(probes, func() error {
				response, err := server.AuthenticatedGET(path, principal)
				return checkOnlineSession(readOnlineHTTPResult(response, err), path, index == 0)
			})
		}
		probes = append(probes, func() error {
			response, err := server.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
			return checkOnlineExchange(readOnlineHTTPResult(response, err), accessToken)
		})
	}
	return probes
}

func checkOnlineSession(result onlineHTTPResult, path string, compatibility bool) error {
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
	if !ok || session["requires_reauthentication"] != false || session["access_token_expired"] != false || session["has_refresh_token"] != true {
		return fmt.Errorf("%s returned incorrect session validity: %v", path, session)
	}
	legacy, aliased := session["is_expired"]
	if aliased != compatibility || (aliased && legacy != false) {
		return fmt.Errorf("%s returned incorrect opt-in compatibility state", path)
	}
	if _, fabricated := session["refresh_token_expires_at"]; fabricated {
		return fmt.Errorf("%s returned a fabricated refresh expiry", path)
	}
	return nil
}

func checkOnlineExchange(result onlineHTTPResult, accessToken string) error {
	if result.err != nil {
		return result.err
	}
	if result.status != http.StatusOK || result.body["access_token"] != accessToken {
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
