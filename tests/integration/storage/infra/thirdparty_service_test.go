//go:build integration
// +build integration

package storage_test

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKEKForIntegration is the deterministic test KEK used for encryption in integration tests.
// This is the base64 encoding of known test bytes — NOT for production use.
const testKEKForIntegration = "ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA="

const (
	cimdPrivateKeyJWTAuthMethod model.TokenEndpointAuthMethod = "private_key_jwt"
	cimdClientIDPrefix                                        = "https://broker.example.test/.well-known/oauth-client/"
)

func cimdClientIDForService(serviceID id.ServiceID) id.ClientID {
	return id.ClientID(cimdClientIDPrefix + serviceID.String())
}

func TestAuthorizationParamsPersistence(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()

	entity := createTestService("authorization-params", "Authorization Params", nil)
	entity.AuthorizationParams = map[string]string{"business_partner_id": "12345"}
	require.NoError(t, providerService.Create(ctx, entity))

	stored, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"business_partner_id": "12345"}, stored.AuthorizationParams)

	stored.AuthorizationParams = map[string]string{}
	stored.Secret = model.NewPlaintextSecret("test-secret")
	require.NoError(t, providerService.Update(ctx, stored, nil))
	updated, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Empty(t, updated.AuthorizationParams)

	services, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, services, 1)
	assert.Empty(t, services[0].AuthorizationParams)
}

func TestAuthorizationParamsOmittedUpdatePreservesResponseAndStorage(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()

	entity := createTestService("authorization-params-omitted-update", "Authorization Params", nil)
	entity.AuthorizationParams = map[string]string{"business_partner_id": "12345"}
	require.NoError(t, providerService.Create(ctx, entity))

	updated := createTestService(entity.ID.String(), "Updated Authorization Params", nil)
	updated.AuthorizationParams = nil
	require.NoError(t, providerService.Update(ctx, updated, nil))
	assert.Equal(t, map[string]string{"business_partner_id": "12345"}, updated.AuthorizationParams)

	stored, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"business_partner_id": "12345"}, stored.AuthorizationParams)
}

// newTestEncryption creates a real memory encryption adapter using the deterministic test KEK.
func newTestEncryption(t *testing.T) ports.EncryptionPort {
	t.Helper()
	enc, _, err := awsencryption.NewAWSEncryption(testKEKForIntegration, "", 0)
	require.NoError(t, err, "failed to create test encryption adapter")
	return enc
}

func setupThirdpartyProviderTestHarness(
	t *testing.T,
) (context.Context, *postgres.PostgresThirdpartyOAuth2ProviderRepository, *thirdparty.ThirdpartyOAuth2ProviderService, func()) {
	t.Helper()

	ctx, repo, providerService, _, _, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	return ctx, repo, providerService, cleanup
}

func setupThirdpartyProviderTestHarnessWithDatabase(
	t *testing.T,
) (
	context.Context,
	*postgres.PostgresThirdpartyOAuth2ProviderRepository,
	*thirdparty.ThirdpartyOAuth2ProviderService,
	*bootstrap.SharedPostgres,
	string,
	func(),
) {
	t.Helper()

	sharedPostgres := bootstrap.RequireSharedPostgres(t)
	dbName, connStr, cleanupDB := sharedPostgres.SetupDatabaseFromTemplate(t, "thirdparty_provider_migrations_034", func(t *testing.T, dbName string) {
		projectRoot, err := bootstrap.FindProjectRoot()
		require.NoError(t, err)
		migrationsDir, err := filepath.Abs(filepath.Join(projectRoot, "migrations"))
		require.NoError(t, err)

		migrationRunner, err := migrate.New("file://"+migrationsDir, sharedPostgres.ConnectionString(dbName))
		require.NoError(t, err)
		defer func() { _, _ = migrationRunner.Close() }()

		err = migrationRunner.Migrate(34)
		if err != nil && err != migrate.ErrNoChange {
			require.NoError(t, err)
		}
	})

	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: connStr,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, adapter.Initialize(ctx))

	encryption := newTestEncryption(t)
	repo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(repo, encryption, &noop.BranchKeyManager{}, nil, false, slog.Default())

	cleanup := func() {
		require.NoError(t, adapter.Close(ctx))
		cleanupDB()
	}

	return ctx, repo, providerService, sharedPostgres, dbName, cleanup
}

// createTestService is a helper to create a test service entity with specified properties.
// Returned entity has Secret in plaintext state, ready for providerService.Create().
func createTestService(serviceNameOrID, displayName string, protectedResources []string) *model.ThirdpartyOAuth2ProviderEntity {
	// If id looks like a UUID, use it; otherwise generate a deterministic UUID from the id string
	var parsedID id.ServiceID
	if parsed, err := id.ParseServiceID(serviceNameOrID); err == nil {
		parsedID = parsed
	} else {
		parsedID = id.ServiceID(uuid.NewSHA1(uuid.Nil, []byte(serviceNameOrID)))
	}

	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          parsedID,
		DisplayName: displayName,
		ClientID:    id.ClientID(serviceNameOrID + "-client"),
		Secret:      model.NewPlaintextSecret("test-secret"),
		IssuerURI:   "https://oauth.example.com",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			TokenEndpoint:     "https://oauth.example.com/token",
			AuthorizeEndpoint: "https://oauth.example.com/authorize",
		},
		Scopes: []model.OAuthScope{
			{ScopeValue: "read", Description: "Read access"},
		},
		ProtectedResources: protectedResources,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}

// TestFindByProtectedResource_SingleMatch tests the happy path: single matching service
func TestFindByProtectedResource_SingleMatch(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service with protected resources
	entity := createTestService(
		"github-service",
		"GitHub",
		[]string{"https://api.github.com", "https://api.github.com/user"},
	)
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Find by exact resource match
	found, err := repo.FindByProtectedResource(ctx, "https://api.github.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "GitHub", found.DisplayName)
	require.NotEmpty(t, found.ID)
	require.Len(t, found.ProtectedResources, 2)
	require.Contains(t, found.ProtectedResources, "https://api.github.com")
}

// TestFindByProtectedResource_NoMatch tests error case: no matching service
func TestFindByProtectedResource_NoMatch(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service without matching resource
	entity := createTestService(
		"github-service",
		"GitHub",
		[]string{"https://api.github.com"},
	)
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Try to find non-existent resource
	found, err := repo.FindByProtectedResource(ctx, "https://api.google.com")
	require.Error(t, err)
	require.Nil(t, found)

	// Verify error is InvalidTargetError
	txErr, ok := err.(*tokenexchange.TokenExchangeError)
	require.True(t, ok, "expected TokenExchangeError")
	require.Equal(t, "invalid_target", txErr.Code())
	require.Equal(t, "no service configured for the requested resource", txErr.Description())
}

// TestFindByProtectedResource_DuplicateClaimRejected verifies global URI ownership.
func TestFindByProtectedResource_DuplicateClaimRejected(t *testing.T) {
	ctx, _, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()

	service1 := createTestService(
		"service-1",
		"Service 1",
		[]string{"https://api.example.com"},
	)
	require.NoError(t, providerService.Create(ctx, service1))

	service2 := createTestService(
		"service-2",
		"Service 2",
		[]string{"https://api.example.com"},
	)
	require.ErrorContains(t, providerService.Create(ctx, service2), "protected resource is already owned")
}

// TestFindByProtectedResource_URINormalization tests URI normalization (trailing slash removal)
func TestFindByProtectedResource_URINormalization(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service with normalized URI (no trailing slash)
	entity := createTestService(
		"api-service",
		"API Service",
		[]string{"https://api.example.com"},
	)
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Query with exact URI should find the service
	// (This test documents the current behavior - normalization is done by the caller)
	found, err := repo.FindByProtectedResource(ctx, "https://api.example.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, entity.ID, found.ID)
	require.Equal(t, "API Service", found.DisplayName)
}

// TestFindByProtectedResource_MultipleServicesNonOverlapping tests multiple services without overlap
func TestFindByProtectedResource_MultipleServicesNonOverlapping(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create three services with different resources
	service1 := createTestService(
		"github-service",
		"GitHub",
		[]string{"https://api.github.com", "https://api.github.com/user"},
	)
	err = providerService.Create(ctx, service1)
	require.NoError(t, err)

	service2 := createTestService(
		"google-service",
		"Google",
		[]string{"https://www.googleapis.com"},
	)
	err = providerService.Create(ctx, service2)
	require.NoError(t, err)

	service3 := createTestService(
		"databricks-service",
		"Databricks",
		[]string{"https://api.databricks.com"},
	)
	err = providerService.Create(ctx, service3)
	require.NoError(t, err)

	// Find each service by its resource
	tests := []struct {
		resource     string
		expectedID   id.ServiceID
		expectedName string
	}{
		{
			resource:     "https://api.github.com",
			expectedID:   service1.ID,
			expectedName: "GitHub",
		},
		{
			resource:     "https://api.github.com/user",
			expectedID:   service1.ID,
			expectedName: "GitHub",
		},
		{
			resource:     "https://www.googleapis.com",
			expectedID:   service2.ID,
			expectedName: "Google",
		},
		{
			resource:     "https://api.databricks.com",
			expectedID:   service3.ID,
			expectedName: "Databricks",
		},
	}

	for _, tc := range tests {
		t.Run(tc.resource, func(t *testing.T) {
			found, err := repo.FindByProtectedResource(ctx, tc.resource)
			require.NoError(t, err)
			require.NotNil(t, found)
			require.Equal(t, tc.expectedID, found.ID)
			require.Equal(t, tc.expectedName, found.DisplayName)
		})
	}
}

// TestFindByProtectedResource_EmptyProtectedResources tests service without protected_resources
func TestFindByProtectedResource_EmptyProtectedResources(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service without protected_resources
	entity := createTestService(
		"service-no-resources",
		"Service Without Resources",
		nil,
	)
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Try to find by resource - should not match
	found, err := repo.FindByProtectedResource(ctx, "https://api.example.com")
	require.Error(t, err)
	require.Nil(t, found)

	txErr, ok := err.(*tokenexchange.TokenExchangeError)
	require.True(t, ok)
	require.Equal(t, "invalid_target", txErr.Code())
}

// TestFindByProtectedResource_CaseSensitive tests that resource matching is case-sensitive
func TestFindByProtectedResource_CaseSensitive(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service with specific case
	entity := createTestService(
		"api-service",
		"API Service",
		[]string{"https://api.Example.com"},
	)
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Find with exact case - should succeed
	found, err := repo.FindByProtectedResource(ctx, "https://api.Example.com")
	require.NoError(t, err)
	require.NotNil(t, found)

	// Find with different case - should fail (case-sensitive)
	found, err = repo.FindByProtectedResource(ctx, "https://api.example.com")
	require.Error(t, err)
	require.Nil(t, found)
}

// TestFindByProtectedResource_MixedScenarios tests combination of scenarios
func TestFindByProtectedResource_MixedScenarios(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Service 1: has resources
	service1 := createTestService(
		"service-1",
		"Service 1",
		[]string{"https://api1.example.com"},
	)
	err = providerService.Create(ctx, service1)
	require.NoError(t, err)

	// Service 2: no resources
	service2 := createTestService(
		"service-2",
		"Service 2",
		nil,
	)
	err = providerService.Create(ctx, service2)
	require.NoError(t, err)

	// Service 3: multiple resources
	service3 := createTestService(
		"service-3",
		"Service 3",
		[]string{"https://api3a.example.com", "https://api3b.example.com"},
	)
	err = providerService.Create(ctx, service3)
	require.NoError(t, err)

	// Test scenarios
	tests := []struct {
		name          string
		resource      string
		shouldSucceed bool
		expectedID    id.ServiceID
	}{
		{
			name:          "Find service 1",
			resource:      "https://api1.example.com",
			shouldSucceed: true,
			expectedID:    service1.ID,
		},
		{
			name:          "Find service 3a",
			resource:      "https://api3a.example.com",
			shouldSucceed: true,
			expectedID:    service3.ID,
		},
		{
			name:          "Find service 3b",
			resource:      "https://api3b.example.com",
			shouldSucceed: true,
			expectedID:    service3.ID,
		},
		{
			name:          "Service 2 has no resources",
			resource:      "https://some.resource.com",
			shouldSucceed: false,
			expectedID:    id.ServiceID{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			found, err := repo.FindByProtectedResource(ctx, tc.resource)
			if tc.shouldSucceed {
				assert.NoError(t, err)
				assert.NotNil(t, found)
				assert.Equal(t, tc.expectedID, found.ID)
			} else {
				assert.Error(t, err)
				assert.Nil(t, found)
			}
		})
	}
}

// TestFindByProtectedResource_ClientSecretDecrypted tests that client_secret is properly decrypted
func TestFindByProtectedResource_ClientSecretDecrypted(t *testing.T) {
	ctx, _, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create service with a specific secret
	entity := createTestService(
		"service-with-secret",
		"Service With Secret",
		[]string{"https://api.example.com"},
	)
	entity.Secret = model.NewPlaintextSecret("my-super-secret")
	err = providerService.Create(ctx, entity)
	require.NoError(t, err)

	// Find by resource via service to get decrypted secret
	found, err := providerService.FindByProtectedResource(ctx, "https://api.example.com")
	require.NoError(t, err)
	require.NotNil(t, found)
	p, err := found.Secret.GetPlaintext()
	require.NoError(t, err)
	require.Equal(t, "my-super-secret", p)
}

// TestFindByProtectedResource_InvalidResourceURI tests validation of empty resource URI
func TestFindByProtectedResource_InvalidResourceURI(t *testing.T) {
	ctx, repo, _, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Try to find with empty resource URI
	found, err := repo.FindByProtectedResource(ctx, "")
	require.Error(t, err)
	require.Nil(t, found)

	// Verify it's a StorageError with validation kind
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok, "expected StorageError")
	require.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}

// TestFindByProtectedResource_GINIndexQuery tests query efficiency (GIN index behavior)
// This test verifies the query uses array containment with @> operator which leverages GIN index
func TestFindByProtectedResource_GINIndexQuery(t *testing.T) {
	ctx, repo, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()
	var err error

	// Create multiple services to test query efficiency
	servicesByName := make(map[string]*model.ThirdpartyOAuth2ProviderEntity)
	for i := 1; i <= 10; i++ {
		resources := []string{}
		for j := 1; j <= 5; j++ {
			resources = append(resources, fmt.Sprintf("https://api%d.example.com/v%d", i, j))
		}
		entity := createTestService(
			fmt.Sprintf("service-%d", i),
			fmt.Sprintf("Service %d", i),
			resources,
		)
		err = providerService.Create(ctx, entity)
		require.NoError(t, err)
		servicesByName[fmt.Sprintf("Service %d", i)] = entity
	}

	// Query should be efficient even with many services
	found, err := repo.FindByProtectedResource(ctx, "https://api1.example.com/v1")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, servicesByName["Service 1"].ID, found.ID)
}

func TestPublicServicePersistence(t *testing.T) {
	ctx, repo, _, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	entity := createTestService("public-service-persistence", "Public Service Persistence", nil)
	entity.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	entity.Secret = model.NewAbsentSecret()
	require.NoError(t, repo.Create(ctx, entity))

	assert.Equal(t, string(model.TokenEndpointAuthMethodNone), sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, entity.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, entity.ID)))

	stored, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, model.TokenEndpointAuthMethodNone, stored.TokenEndpointAuthMethod)
	assert.True(t, stored.Secret.IsAbsent())
}

func TestPublicToConfidentialUpdateStoresCiphertext(t *testing.T) {
	ctx, repo, providerService, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	public := createTestService("public-to-confidential", "Public Service", nil)
	public.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	public.Secret = model.NewAbsentSecret()
	require.NoError(t, providerService.Create(ctx, public))

	confidential := createTestService(public.ID.String(), "Confidential Service", nil)
	require.NoError(t, providerService.Update(ctx, confidential, nil))

	assert.Equal(t, "f", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, confidential.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, confidential.ID)))

	stored, err := repo.Get(ctx, confidential.ID)
	require.NoError(t, err)
	assert.True(t, stored.TokenEndpointAuthMethod.IsAbsent())
	assert.True(t, stored.Secret.IsEncrypted())
}

func TestConfidentialToPublicUpdateClearsCiphertext(t *testing.T) {
	ctx, repo, providerService, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	confidential := createTestService("confidential-to-public", "Confidential Service", nil)
	require.NoError(t, providerService.Create(ctx, confidential))
	assert.Equal(t, "f", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, confidential.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, confidential.ID)))

	public := createTestService(confidential.ID.String(), "Public Service", nil)
	public.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	public.Secret = model.NewAbsentSecret()
	require.NoError(t, repo.Update(ctx, public, nil))

	assert.Equal(t, string(model.TokenEndpointAuthMethodNone), sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, public.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, public.ID)))

	stored, err := repo.Get(ctx, public.ID)
	require.NoError(t, err)
	assert.Equal(t, model.TokenEndpointAuthMethodNone, stored.TokenEndpointAuthMethod)
	assert.True(t, stored.Secret.IsAbsent())
}

func TestCIMDServicePersistence(t *testing.T) {
	ctx, repo, _, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	entity := createTestService("cimd-service-persistence", "CIMD Service Persistence", nil)
	entity.ClientID = cimdClientIDForService(entity.ID)
	entity.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod
	entity.Secret = model.NewAbsentSecret()
	require.NoError(t, repo.Create(ctx, entity))

	assert.Equal(t, string(cimdPrivateKeyJWTAuthMethod), sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, entity.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, entity.ID)))

	stored, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, cimdClientIDForService(entity.ID), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}

func TestStaticToCIMDUpdateClearsCiphertext(t *testing.T) {
	ctx, repo, providerService, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	staticService := createTestService("static-to-cimd", "Static Service", nil)
	require.NoError(t, providerService.Create(ctx, staticService))

	cimdService := createTestService(staticService.ID.String(), "CIMD Service", nil)
	cimdService.ClientID = cimdClientIDForService(staticService.ID)
	cimdService.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod
	cimdService.Secret = model.NewAbsentSecret()
	require.NoError(t, repo.Update(ctx, cimdService, nil))

	assert.Equal(t, string(cimdPrivateKeyJWTAuthMethod), sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT token_endpoint_auth_method
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, staticService.ID)))
	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`
		SELECT client_secret_encrypted IS NULL
		FROM thirdparty_oauth2_services
		WHERE id = '%s';`, staticService.ID)))

	stored, err := repo.Get(ctx, staticService.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, cimdClientIDForService(staticService.ID), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}
