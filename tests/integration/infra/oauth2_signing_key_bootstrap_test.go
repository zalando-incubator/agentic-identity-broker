//go:build integration
// +build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/app"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	e2ebootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	testbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func TestConcurrentLocalModeBootstrapUsesSingleSigningKeyAcrossReplicas(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if !containerRuntimeAvailable() {
		t.Skip("Skipping test: No container runtime available")
	}

	connStr, cleanupDatabase := setupMigratedPostgresDatabase(t)
	defer cleanupDatabase()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storage1, cleanupStorage1 := newPostgresStorageAdapter(t, connStr)
	defer cleanupStorage1()
	storage2, cleanupStorage2 := newPostgresStorageAdapter(t, connStr)
	defer cleanupStorage2()

	start := make(chan struct{})
	results := make(chan buildResult, 2)
	var wg sync.WaitGroup

	for _, storage := range []*storageadapter.Adapter{storage1, storage2} {
		wg.Add(1)
		go func(storage *storageadapter.Adapter) {
			defer wg.Done()
			<-start
			cfg := newLocalModePostgresConfig(connStr)
			builtApp, err := e2ebootstrap.NewServerFactory(cfg, logger).BuildApp(storage)
			results <- buildResult{app: builtApp, err: err}
		}(storage)
	}

	close(start)
	wg.Wait()
	close(results)

	apps := make([]*app.App, 0, 2)
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.app)
		apps = append(apps, result.app)
	}
	require.Len(t, apps, 2)
	defer shutdownApp(t, apps[0])
	defer shutdownApp(t, apps[1])

	ctx := context.Background()
	count, err := storage1.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	current, err := storage1.SigningKeys().GetCurrentInDomain(ctx, domstorage.KeyDomainTokenSigning)
	require.NoError(t, err)

	agent := createLocalIntegrationAgent(t, storage1)

	adminServer, err := e2ebootstrap.NewAdminTestServer(apps[0], logger)
	require.NoError(t, err)
	defer adminServer.Close()

	enduserServer1, err := e2ebootstrap.NewEndUserTestServer(apps[0], logger)
	require.NoError(t, err)
	defer enduserServer1.Close()

	enduserServer2, err := e2ebootstrap.NewEndUserTestServer(apps[1], logger)
	require.NoError(t, err)
	defer enduserServer2.Close()

	clientSecret := createClientCredentials(t, adminServer, agent.ID.String())

	tokenFromReplica1 := issueClientCredentialsToken(t, enduserServer1, agent.ID.String(), clientSecret)
	tokenFromReplica2 := issueClientCredentialsToken(t, enduserServer2, agent.ID.String(), clientSecret)

	assert.Equal(t, string(current.KID), tokenKID(t, tokenFromReplica1))
	assert.Equal(t, string(current.KID), tokenKID(t, tokenFromReplica2))

	jwks1 := fetchJWKS(t, enduserServer1)
	jwks2 := fetchJWKS(t, enduserServer2)

	assert.Equal(t, 1, jwks1.Len())
	assert.Equal(t, 1, jwks2.Len())
	_, ok := jwks1.LookupKeyID(string(current.KID))
	assert.True(t, ok)
	_, ok = jwks2.LookupKeyID(string(current.KID))
	assert.True(t, ok)
}

func TestLocalModeBootstrapIsolatedToTokenSigningDomain(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if !containerRuntimeAvailable() {
		t.Skip("Skipping test: No container runtime available")
	}

	connStr, cleanupDatabase := setupMigratedPostgresDatabase(t)
	defer cleanupDatabase()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store, cleanupStorage := newPostgresStorageAdapter(t, connStr)
	defer cleanupStorage()

	brokerApp, err := e2ebootstrap.NewServerFactory(newLocalModePostgresConfig(connStr), logger).BuildApp(store)
	require.NoError(t, err)
	defer shutdownApp(t, brokerApp)

	ctx := context.Background()
	tokenSigningCount, err := store.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, 1, tokenSigningCount, "local bootstrap must create one token-signing key")

	cimdCount, err := store.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainCIMDClientAuthentication)
	require.NoError(t, err)
	assert.Zero(t, cimdCount, "local token-signing bootstrap must not create a CIMD client-authentication key")

	tokenSigningKey, err := store.SigningKeys().GetCurrentInDomain(ctx, domstorage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, domstorage.KeyDomainTokenSigning, tokenSigningKey.KeyDomain)

	_, err = store.SigningKeys().GetCurrentInDomain(ctx, domstorage.KeyDomainCIMDClientAuthentication)
	require.Error(t, err, "CIMD has no current key until its distinct lifecycle provisions one")
}

func TestPostgresCIMDBootstrapRequiresPersistedCIMDService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("does not provision a CIMD key for an existing static service", func(t *testing.T) {
		connStr, cleanupDatabase := setupMigratedPostgresDatabase(t)
		defer cleanupDatabase()

		store, cleanupStorage := newPostgresStorageAdapter(t, connStr)
		defer cleanupStorage()
		createStaticIntegrationService(t, store)

		brokerApp, err := e2ebootstrap.NewServerFactory(newLocalModePostgresConfig(connStr), logger).BuildApp(store)
		require.NoError(t, err)
		defer shutdownApp(t, brokerApp)

		ctx := context.Background()
		tokenSigningCount, err := store.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, 1, tokenSigningCount)

		cimdCount, err := store.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Zero(t, cimdCount, "only persisted CIMD services may trigger CIMD bootstrap")
	})

	t.Run("provisions an immediately usable CIMD key for a persisted CIMD service", func(t *testing.T) {
		connStr, cleanupDatabase := setupMigratedPostgresDatabase(t)
		defer cleanupDatabase()

		store, cleanupStorage := newPostgresStorageAdapter(t, connStr)
		defer cleanupStorage()
		createCIMDIntegrationService(t, store)

		brokerApp, err := e2ebootstrap.NewServerFactory(newLocalModePostgresConfig(connStr), logger).BuildApp(store)
		require.NoError(t, err)
		defer shutdownApp(t, brokerApp)

		ctx := context.Background()
		cimdCount, err := store.SigningKeys().CountActiveInDomain(ctx, domstorage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, 1, cimdCount)

		key, err := store.SigningKeys().GetCurrentInDomain(ctx, domstorage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, domstorage.KeyDomainCIMDClientAuthentication, key.KeyDomain)
		assert.Equal(t, "ES256", key.Algorithm)
		assert.True(t, key.IsCurrent)
		assert.False(t, key.ActivatesAt.After(time.Now().UTC()), "the first CIMD key must not wait through the rotation grace period")
	})
}

type buildResult struct {
	app *app.App
	err error
}

func containerRuntimeAvailable() bool {
	if err := exec.Command("docker", "ps").Run(); err == nil {
		return true
	}
	return exec.Command("podman", "ps").Run() == nil
}

func setupMigratedPostgresDatabase(t *testing.T) (string, func()) {
	t.Helper()

	postgres := testbootstrap.RequireSharedPostgres(t)
	_, connStr, cleanupDatabase := postgres.SetupDatabaseFromTemplate(t, "oauth2_signing_key_bootstrap_full_migrations", func(t *testing.T, dbName string) {
		projectRoot, err := testbootstrap.FindProjectRoot()
		require.NoError(t, err)
		migrationsDir, err := filepath.Abs(filepath.Join(projectRoot, "migrations"))
		require.NoError(t, err)

		migrationConnStr := postgres.ConnectionString(dbName)
		migrationRunner, err := migrate.New("file://"+migrationsDir, migrationConnStr)
		require.NoError(t, err)
		defer func() { _, _ = migrationRunner.Close() }()

		err = migrationRunner.Up()
		if err != nil && err != migrate.ErrNoChange {
			require.NoError(t, err)
		}
	})

	return connStr, cleanupDatabase
}

func newPostgresStorageAdapter(t *testing.T, connStr string) (*storageadapter.Adapter, func()) {
	t.Helper()

	adapter, err := storageadapter.NewAdapter(&ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: connStr,
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	})
	require.NoError(t, err)

	return adapter, func() {
		require.NoError(t, adapter.Close(context.Background()))
	}
}

func newLocalModePostgresConfig(connStr string) *ports.Config {
	jweKey := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	return &ports.Config{
		Log: ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port:      8000,
				Bind:      "127.0.0.1",
				PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port:      14000,
				Bind:      "127.0.0.1",
				PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
				},
			},
			Shutdown: ports.ShutdownConfig{Timeout: 5 * time.Second},
		},
		Storage: ports.StorageConfig{
			Backend: "postgres",
			Postgres: ports.PostgresConfig{
				ConnectionURL: connStr,
			},
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 10 * time.Second,
			},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{JWESigningKey: jweKey},
		Encryption:       ports.EncryptionConfig{Memory: &ports.MemoryConfig{RawKey: testutil.TestKEKBase64}},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode: "local",
			Local: ports.LocalModeConfig{
				TokenTTL: time.Hour,
				SigningKeys: ports.LocalSigningKeysConfig{
					BootstrapTimeout: 5 * time.Second,
				},
			},
		},
	}
}

func createStaticIntegrationService(t *testing.T, store *storageadapter.Adapter) {
	t.Helper()

	now := time.Now().UTC()
	require.NoError(t, store.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Static integration service",
		ClientID:    id.ClientID("static-integration-client"),
		Secret:      model.NewEncryptedSecret([]byte("static-integration-ciphertext")),
		IssuerURI:   "https://static.integration.example.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://static.integration.example.com/authorize",
			TokenEndpoint:     "https://static.integration.example.com/token",
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}},
		CreatedAt: now,
		UpdatedAt: now,
	}))
}

func createCIMDIntegrationService(t *testing.T, store *storageadapter.Adapter) {
	t.Helper()

	now := time.Now().UTC()
	serviceID := id.NewServiceID()
	require.NoError(t, store.Services().Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      serviceID,
		DisplayName:             "CIMD integration service",
		ClientID:                id.ClientID("https://broker.integration.example.com/.well-known/oauth-client/" + serviceID.String()),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodPrivateKeyJWT,
		IssuerURI:               "https://cimd.integration.example.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://cimd.integration.example.com/authorize",
			TokenEndpoint:     "https://cimd.integration.example.com/token",
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}},
		CreatedAt: now,
		UpdatedAt: now,
	}))
}

func newLocalIntegrationAgent() *domstorage.Agent {
	now := time.Now().UTC()
	return &domstorage.Agent{
		ID:           id.NewAgentID(),
		DisplayName:  "HA test agent",
		Description:  "Agent used to verify cross-replica signing-key bootstrap",
		RedirectURIs: []string{"https://client.example.com/callback"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func createLocalIntegrationAgent(t *testing.T, store *storageadapter.Adapter) *domstorage.Agent {
	t.Helper()

	ctx := context.Background()
	serviceID := id.NewServiceID()
	permissionSetID := id.NewPermissionSetID()
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID,
		DisplayName: "HA test service",
		ClientID:    id.ClientID("ha-test-client"),
		Secret:      model.NewEncryptedSecret([]byte("test-ciphertext")),
		IssuerURI:   "https://service.example.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://service.example.com/authorize",
			TokenEndpoint:     "https://service.example.com/token",
		},
		Scopes:    []model.OAuthScope{{ScopeValue: "read", Description: "Read access"}},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, store.Services().Create(ctx, service))
	require.NoError(t, store.PermissionSets().Create(ctx, &domstorage.PermissionSet{
		ID:          permissionSetID,
		Name:        "HA test permission set",
		Description: "Permission set for signing-key bootstrap",
		ServiceScopes: []domstorage.ServiceScope{{
			ServiceID:       serviceID,
			Scopes:          []string{"read"},
			RequirementType: domstorage.RequirementTypeOptional,
		}},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}))

	agent := newLocalIntegrationAgent()
	agent.PermissionSets = []domstorage.AgentPermissionSetEntry{{
		PermissionSetID: permissionSetID,
		RequirementType: domstorage.RequirementTypeOptional,
	}}
	require.NoError(t, store.Agents().Create(ctx, agent))
	return agent
}

func createClientCredentials(t *testing.T, adminServer *e2ebootstrap.TestServer, agentID string) string {
	t.Helper()

	resp, err := http.Post(
		adminServer.BaseURL()+"/api/agents/"+agentID+"/client-credentials",
		"application/json",
		nil,
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	clientSecret, ok := body["client_secret"].(string)
	require.True(t, ok)
	require.NotEmpty(t, clientSecret)
	return clientSecret
}

func issueClientCredentialsToken(t *testing.T, server *e2ebootstrap.TestServer, clientID, clientSecret string) string {
	t.Helper()

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	resp, err := server.PublicPOST(
		"/oauth2/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	accessToken, ok := body["access_token"].(string)
	require.True(t, ok)
	require.NotEmpty(t, accessToken)
	return accessToken
}

func tokenKID(t *testing.T, accessToken string) string {
	t.Helper()

	msg, err := jws.Parse([]byte(accessToken))
	require.NoError(t, err)
	require.Len(t, msg.Signatures(), 1)
	kid, ok := msg.Signatures()[0].ProtectedHeaders().KeyID()
	require.True(t, ok)
	require.NotEmpty(t, kid)
	return kid
}

func fetchJWKS(t *testing.T, server *e2ebootstrap.TestServer) jwk.Set {
	t.Helper()

	resp, err := http.Get(server.BaseURL() + "/oauth2/jwks.json")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	set, err := jwk.ParseReader(resp.Body)
	require.NoError(t, err)
	return set
}

func shutdownApp(t *testing.T, brokerApp *app.App) {
	t.Helper()
	if brokerApp == nil || brokerApp.Shutdown == nil {
		return
	}
	require.NoError(t, brokerApp.Shutdown(context.Background()))
}
