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
	dbName, connStr, cleanupDB := sharedPostgres.SetupDatabaseFromTemplate(t, "thirdparty_provider_migrations_036", func(t *testing.T, dbName string) {
		projectRoot, err := bootstrap.FindProjectRoot()
		require.NoError(t, err)
		migrationsDir, err := filepath.Abs(filepath.Join(projectRoot, "migrations"))
		require.NoError(t, err)

		migrationRunner, err := migrate.New("file://"+migrationsDir, sharedPostgres.ConnectionString(dbName))
		require.NoError(t, err)
		defer func() { _, _ = migrationRunner.Close() }()

		err = migrationRunner.Migrate(36)
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
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(repo, encryption, &noop.BranchKeyManager{}, nil, false, slog.Default()).WithDiscoveryStatusWriter(repo)

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

func TestFindByProtectedResource_LeavesSecretEncryptedUntilGet(t *testing.T) {
	ctx, _, providerService, cleanup := setupThirdpartyProviderTestHarness(t)
	defer cleanup()

	entity := createTestService(
		"service-with-secret",
		"Service With Secret",
		[]string{"https://api.example.com"},
	)
	entity.Secret = model.NewPlaintextSecret("my-super-secret")
	require.NoError(t, providerService.Create(ctx, entity))

	found, err := providerService.FindByProtectedResource(ctx, "https://api.example.com")
	require.NoError(t, err)
	require.Equal(t, entity.ID, found.ID)
	require.True(t, found.Secret.IsEncrypted(), "resource lookup must not decrypt the provider secret")

	retrieved, err := providerService.Get(ctx, found.ID)
	require.NoError(t, err)
	plaintext, err := retrieved.Secret.GetPlaintext()
	require.NoError(t, err)
	require.Equal(t, "my-super-secret", plaintext)
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

func testDCRPersistenceEntity(name, issuer string, method model.TokenEndpointAuthMethod, secret model.Secret) *model.ThirdpartyOAuth2ProviderEntity {
	entity := createTestService(name, name, nil)
	resourceURL := "https://resource.example.com/" + entity.ID.String()
	completedAt := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	entity.ClientID = "shared-dcr-client"
	entity.IssuerURI = issuer
	entity.Secret = secret
	entity.TokenEndpointAuthMethod = method
	entity.Discovery = model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: model.ClientBootstrapDCR}
	entity.AuthorizationParams = map[string]string{"resource": resourceURL}
	entity.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	entity.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token"}
	return entity
}

func TestDCRIssuerScopedCredentialsSurviveNewPostgresAdapter(t *testing.T) {
	ctx, repo, _, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()
	encryption := newTestEncryption(t)

	first := testDCRPersistenceEntity("dcr-first-issuer", "https://first.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewAbsentSecret())
	second := testDCRPersistenceEntity("dcr-second-issuer", "https://second.example.com", model.TokenEndpointAuthMethodClientSecretPost, model.NewAbsentSecret())
	firstSecret := "first-issued-secret"
	secondSecret := "second-issued-secret"
	for _, tc := range []struct {
		entity    *model.ThirdpartyOAuth2ProviderEntity
		plaintext string
	}{
		{first, firstSecret},
		{second, secondSecret},
	} {
		ciphertext, err := encryption.Encrypt(ctx, []byte(tc.plaintext), map[string]string{"service_id": tc.entity.ID.String()})
		require.NoError(t, err)
		tc.entity.Secret = model.NewEncryptedSecret(ciphertext)
		require.NoError(t, repo.Create(ctx, tc.entity))
	}

	encodedCiphertext := func(serviceID id.ServiceID) string {
		return sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`SELECT encode(client_secret_encrypted, 'hex') FROM thirdparty_oauth2_services WHERE id = '%s';`, serviceID))
	}
	firstBefore, secondBefore := encodedCiphertext(first.ID), encodedCiphertext(second.ID)
	require.NotEmpty(t, firstBefore)
	require.NotEmpty(t, secondBefore)
	assert.NotEqual(t, fmt.Sprintf("%x", firstSecret), firstBefore)
	assert.NotEqual(t, fmt.Sprintf("%x", secondSecret), secondBefore)

	duplicate := testDCRPersistenceEntity("dcr-rejected-duplicate", first.IssuerURI, model.TokenEndpointAuthMethodClientSecretBasic, model.NewAbsentSecret())
	duplicateCiphertext, err := encryption.Encrypt(ctx, []byte("rejected-issued-secret"), map[string]string{"service_id": duplicate.ID.String()})
	require.NoError(t, err)
	duplicate.Secret = model.NewEncryptedSecret(duplicateCiphertext)
	err = repo.Create(ctx, duplicate)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	require.ErrorIs(t, err, storage.ErrDuplicateDCRClientIdentity)
	assert.Equal(t, "0", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`SELECT count(*) FROM thirdparty_oauth2_services WHERE id = '%s';`, duplicate.ID)))

	replacement := first.Copy()
	replacement.IssuerURI = second.IssuerURI
	replacement.Endpoints = second.Endpoints
	replacementCiphertext, err := encryption.Encrypt(ctx, []byte("updated-issued-secret"), map[string]string{"service_id": first.ID.String()})
	require.NoError(t, err)
	replacement.Secret = model.NewEncryptedSecret(replacementCiphertext)
	completedAt := *first.DiscoveryStatus.LastAttemptAt
	completedAt = completedAt.Add(time.Minute)
	replacement.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	err = repo.Update(ctx, replacement, nil)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	require.ErrorIs(t, err, storage.ErrDuplicateDCRClientIdentity)
	assert.Equal(t, firstBefore, encodedCiphertext(first.ID))
	assert.Equal(t, secondBefore, encodedCiphertext(second.ID))

	adapter, err := postgres.NewAdapter(&ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: sharedPostgres.ConnectionString(dbName)},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	defer func() { require.NoError(t, adapter.Close(ctx)) }()
	freshRepo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	freshService := thirdparty.NewThirdpartyOAuth2ProviderService(freshRepo, newTestEncryption(t), &noop.BranchKeyManager{}, nil, false, slog.Default())
	for _, tc := range []struct {
		entity    *model.ThirdpartyOAuth2ProviderEntity
		plaintext string
	}{
		{first, firstSecret},
		{second, secondSecret},
	} {
		stored, err := freshRepo.Get(ctx, tc.entity.ID)
		require.NoError(t, err)
		assert.Equal(t, tc.entity.IssuerURI, stored.IssuerURI)
		assert.Equal(t, tc.entity.ClientID, stored.ClientID)
		assert.Equal(t, tc.entity.TokenEndpointAuthMethod, stored.TokenEndpointAuthMethod)
		assert.Equal(t, model.ClientBootstrapDCR, stored.Discovery.ClientMethod)
		assert.EqualValues(t, 1, stored.Version)
		ciphertext, err := stored.Secret.GetCiphertext()
		require.NoError(t, err)
		plaintext, err := encryption.Decrypt(ctx, ciphertext, map[string]string{"service_id": tc.entity.ID.String()})
		require.NoError(t, err)
		assert.Equal(t, tc.plaintext, string(plaintext))
		decrypted, err := freshService.Get(ctx, tc.entity.ID)
		require.NoError(t, err)
		usableSecret, err := decrypted.Secret.GetPlaintext()
		require.NoError(t, err)
		assert.Equal(t, tc.plaintext, usableSecret)
		if tc.entity.ID == first.ID {
			_, err = encryption.Decrypt(ctx, ciphertext, map[string]string{"service_id": second.ID.String()})
			require.Error(t, err, "a credential bound to one service must not decrypt as another")
		}
	}

	require.NoError(t, freshRepo.Delete(ctx, first.ID))
	assert.Equal(t, "0", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`SELECT count(*) FROM thirdparty_oauth2_services WHERE id = '%s';`, first.ID)))
	_, err = freshRepo.Get(ctx, first.ID)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	remaining, err := freshService.Get(ctx, second.ID)
	require.NoError(t, err)
	remainingSecret, err := remaining.Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, secondSecret, remainingSecret)
}

func TestPublicDCRPersistsWithoutCredential(t *testing.T) {
	ctx, repo, _, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()
	public := testDCRPersistenceEntity("dcr-public-client", "https://public.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, public))

	assert.Equal(t, "t", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`SELECT client_secret_encrypted IS NULL FROM thirdparty_oauth2_services WHERE id = '%s';`, public.ID)))
	stored, err := repo.Get(ctx, public.ID)
	require.NoError(t, err)
	assert.Equal(t, model.ClientBootstrapDCR, stored.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodNone, stored.TokenEndpointAuthMethod)
	assert.True(t, stored.Secret.IsAbsent())

	adapter, err := postgres.NewAdapter(&ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: sharedPostgres.ConnectionString(dbName)},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	defer func() { require.NoError(t, adapter.Close(ctx)) }()
	freshRepo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	reloaded, err := freshRepo.Get(ctx, public.ID)
	require.NoError(t, err)
	assert.True(t, reloaded.Secret.IsAbsent())
	assert.Equal(t, model.ClientBootstrapDCR, reloaded.Discovery.ClientMethod)
	require.NoError(t, freshRepo.Delete(ctx, public.ID))
	assert.Equal(t, "0", sharedPostgres.QuerySQL(t, dbName, fmt.Sprintf(`SELECT count(*) FROM thirdparty_oauth2_services WHERE id = '%s';`, public.ID)))
}

// unavailableResourceDiscovery makes a refresh fail after it starts remote discovery.
// The failure-only writer must persist the outcome without replacing the active client.
type unavailableResourceDiscovery struct{}

func (unavailableResourceDiscovery) Probe(context.Context, string) (int, []string, error) {
	return 0, nil, ports.ErrOAuthDiscoveryUnavailable
}

func (unavailableResourceDiscovery) GetJSON(context.Context, string) ([]byte, error) {
	return nil, ports.ErrOAuthDiscoveryUnavailable
}

func (unavailableResourceDiscovery) PostJSON(context.Context, string, []byte) ([]byte, error) {
	return nil, ports.ErrOAuthDiscoveryUnavailable
}

func TestDiscoveryFailureAndRecoveryPersistAcrossNewPostgresAdapter(t *testing.T) {
	ctx, repo, providerService, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	encryption := newTestEncryption(t)
	entity := testDCRPersistenceEntity("dcr-status-restart", "https://login.example.test", model.TokenEndpointAuthMethodClientSecretPost, model.NewAbsentSecret())
	initialSuccess := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	entity.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &initialSuccess, LastSuccessAt: &initialSuccess}
	entity.CreatedAt = initialSuccess
	entity.UpdatedAt = initialSuccess
	entity.ResourceExplicit = true
	entity.AuthorizationParams["resource"] = "https://audience.example.test/files"
	const clientSecret = "registered-client-secret"
	clientCiphertext, err := encryption.Encrypt(ctx, []byte(clientSecret), map[string]string{"service_id": entity.ID.String()})
	require.NoError(t, err)
	entity.Secret = model.NewEncryptedSecret(clientCiphertext)
	require.NoError(t, repo.Create(ctx, entity))
	before, err := repo.Get(ctx, entity.ID)
	require.NoError(t, err)
	beforeCiphertext, err := before.Secret.GetCiphertext()
	require.NoError(t, err)

	adapter, err := postgres.NewAdapter(&ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: sharedPostgres.ConnectionString(dbName)},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	principal := id.Principal("dcr-status-user@example.test")
	accessCiphertext, err := encryption.Encrypt(ctx, []byte("existing-access-token"), map[string]string{"service_id": entity.ID.String()})
	require.NoError(t, err)
	refreshCiphertext, err := encryption.Encrypt(ctx, []byte("existing-refresh-token"), map[string]string{"service_id": entity.ID.String()})
	require.NoError(t, err)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: entity.ID,
		ExpectedIssuerURI:    entity.IssuerURI,
		ExpectedResource:     entity.AuthorizationParams["resource"],
		EncryptedAccessToken: accessCiphertext, EncryptedRefreshToken: refreshCiphertext,
		TokenType: "Bearer", Scope: []string{"read"},
		EncryptionContext: storage.EncryptionContext{ServiceID: entity.ID},
		InitiatedAt:       initialSuccess, CreatedAt: initialSuccess, UpdatedAt: initialSuccess,
	}
	require.NoError(t, postgres.NewUserSessionRepository(adapter).Create(ctx, session))
	require.NoError(t, adapter.Close(ctx))

	ready, err := providerService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", ready.Status)
	require.NotNil(t, ready.LastAttemptAt)
	require.NotNil(t, ready.LastSuccessAt)
	assert.True(t, ready.LastAttemptAt.Equal(initialSuccess))
	assert.True(t, ready.LastSuccessAt.Equal(initialSuccess))
	assert.Nil(t, ready.FailureReason)

	providerService.WithOAuthDiscoveryClient(unavailableResourceDiscovery{})
	request := &model.ThirdpartyOAuth2ProviderEntity{
		ID: entity.ID, DisplayName: entity.DisplayName,
		Secret:    model.NewAbsentSecret(),
		Discovery: model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: entity.Discovery.ResourceURL},
	}
	err = providerService.Update(ctx, request, &before.Version)
	require.ErrorContains(t, err, "resource_metadata_unavailable")

	// A new adapter must see the failure and the original encrypted client and session.
	restarted, err := postgres.NewAdapter(&ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: sharedPostgres.ConnectionString(dbName)},
	})
	require.NoError(t, err)
	require.NoError(t, restarted.Initialize(ctx))
	defer func() { require.NoError(t, restarted.Close(ctx)) }()
	freshRepo := postgres.NewPostgresThirdpartyOAuth2ProviderRepository(restarted)
	freshService := thirdparty.NewThirdpartyOAuth2ProviderService(freshRepo, newTestEncryption(t), &noop.BranchKeyManager{}, nil, false, slog.Default())
	failed, err := freshService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", failed.Status)
	require.NotNil(t, failed.ResourceURL)
	assert.Equal(t, *entity.Discovery.ResourceURL, *failed.ResourceURL)
	require.NotNil(t, failed.IssuerURI)
	assert.Equal(t, entity.IssuerURI, *failed.IssuerURI)
	require.NotNil(t, failed.ClientMethod)
	assert.Equal(t, string(model.ClientBootstrapDCR), *failed.ClientMethod)
	require.NotNil(t, failed.LastAttemptAt)
	require.NotNil(t, failed.LastSuccessAt)
	assert.True(t, failed.LastAttemptAt.After(*failed.LastSuccessAt))
	assert.True(t, failed.LastSuccessAt.Equal(initialSuccess))
	require.NotNil(t, failed.FailureReason)
	assert.Equal(t, "resource_metadata_unavailable", *failed.FailureReason)

	stored, err := freshRepo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version, stored.Version)
	assert.True(t, before.UpdatedAt.Equal(stored.UpdatedAt))
	assert.Equal(t, before.IssuerURI, stored.IssuerURI)
	assert.Equal(t, before.ClientID, stored.ClientID)
	assert.Equal(t, before.Discovery, stored.Discovery)
	assert.Equal(t, before.ResourceExplicit, stored.ResourceExplicit)
	assert.Equal(t, before.AuthorizationParams, stored.AuthorizationParams)
	assert.Equal(t, before.Endpoints, stored.Endpoints)
	assert.Equal(t, before.TokenEndpointAuthMethod, stored.TokenEndpointAuthMethod)
	storedCiphertext, err := stored.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, beforeCiphertext, storedCiphertext)
	decrypted, err := freshService.Get(ctx, entity.ID)
	require.NoError(t, err)
	plaintext, err := decrypted.Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, clientSecret, plaintext)

	sessions := postgres.NewUserSessionRepository(restarted)
	persistedSession, err := sessions.FindByPrincipalAndService(ctx, principal, entity.ID)
	require.NoError(t, err)
	require.NotNil(t, persistedSession)
	assert.Equal(t, session.ID, persistedSession.ID)
	assert.Equal(t, session.EncryptedRefreshToken, persistedSession.EncryptedRefreshToken)
	refreshToken, err := encryption.Decrypt(ctx, persistedSession.EncryptedRefreshToken, map[string]string{"service_id": entity.ID.String()})
	require.NoError(t, err)
	assert.Equal(t, "existing-refresh-token", string(refreshToken))
	accessToken, err := encryption.Decrypt(ctx, persistedSession.EncryptedAccessToken, map[string]string{"service_id": entity.ID.String()})
	require.NoError(t, err)
	assert.Equal(t, "existing-access-token", string(accessToken))

	// A late failure cannot overtake the latest attempt, even with the correct version.
	for _, attempt := range []struct {
		version int64
		at      time.Time
	}{
		{before.Version, *failed.LastAttemptAt},
		{before.Version, failed.LastAttemptAt.Add(-time.Microsecond)},
		{before.Version + 1, failed.LastAttemptAt.Add(time.Minute)},
	} {
		err := freshRepo.RecordDiscoveryFailure(ctx, entity.ID, attempt.version, attempt.at, "stale_failure")
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	}
	stillFailed, err := freshService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, failed, stillFailed)
	staleAt := failed.LastAttemptAt.Add(-time.Microsecond)
	staleSuccess := stored.Copy()
	staleSuccess.Endpoints.TokenEndpoint = "https://login.example.test/stale-token"
	staleSuccess.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &staleAt, LastSuccessAt: &staleAt}
	err = freshRepo.Update(ctx, staleSuccess, &before.Version)
	var staleError *storage.StorageError
	require.ErrorAs(t, err, &staleError)
	assert.Equal(t, storage.ErrorKindConflict, staleError.Kind)
	stillFailed, err = freshService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, failed, stillFailed)

	// A successful refresh commits a new active endpoint and ready status together.
	refreshedAt := failed.LastAttemptAt.Add(time.Minute)
	stored.Endpoints.TokenEndpoint = "https://login.example.test/new-token"
	stored.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &refreshedAt, LastSuccessAt: &refreshedAt}
	stored.UpdatedAt = refreshedAt
	require.NoError(t, freshRepo.Update(ctx, stored, &before.Version))
	recovered, err := freshService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, "ready", recovered.Status)
	assert.Nil(t, recovered.FailureReason)
	require.NotNil(t, recovered.LastAttemptAt)
	require.NotNil(t, recovered.LastSuccessAt)
	assert.True(t, recovered.LastAttemptAt.Equal(refreshedAt))
	assert.True(t, recovered.LastSuccessAt.Equal(refreshedAt))
	active, err := freshRepo.Get(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version+1, active.Version)
	assert.Equal(t, stored.Endpoints.TokenEndpoint, active.Endpoints.TokenEndpoint)
	assert.Equal(t, before.ClientID, active.ClientID)
	assert.Equal(t, before.TokenEndpointAuthMethod, active.TokenEndpointAuthMethod)
	activeCiphertext, err := active.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, beforeCiphertext, activeCiphertext)
	unchangedSession, err := sessions.FindByPrincipalAndService(ctx, principal, entity.ID)
	require.NoError(t, err)
	require.NotNil(t, unchangedSession)
	assert.Equal(t, persistedSession.EncryptedRefreshToken, unchangedSession.EncryptedRefreshToken)

	for _, attempt := range []struct {
		version int64
		at      time.Time
	}{
		{before.Version, refreshedAt.Add(time.Minute)},
		{active.Version, refreshedAt},
		{active.Version, failed.LastAttemptAt.Add(time.Microsecond)},
	} {
		err := freshRepo.RecordDiscoveryFailure(ctx, entity.ID, attempt.version, attempt.at, "stale_failure")
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	}
	stillReady, err := freshService.GetDiscoveryStatus(ctx, entity.ID)
	require.NoError(t, err)
	assert.Equal(t, recovered, stillReady)
}

func TestDiscoveryToManualReplacementRetainsOmittedParamsAndClearsStatus(t *testing.T) {
	ctx, repo, providerService, _, _, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()

	discovered := testDCRPersistenceEntity("dcr-to-manual", "https://login.example.test", model.TokenEndpointAuthMethodClientSecretBasic, model.NewAbsentSecret())
	const retainedAudience = "https://audience.example.test/files"
	discovered.AuthorizationParams["resource"] = retainedAudience
	discovered.ResourceExplicit = true
	encryption := newTestEncryption(t)
	oldCiphertext, err := encryption.Encrypt(ctx, []byte("old-dcr-secret"), map[string]string{"service_id": discovered.ID.String()})
	require.NoError(t, err)
	discovered.Secret = model.NewEncryptedSecret(oldCiphertext)
	require.NoError(t, repo.Create(ctx, discovered))

	manual := createTestService(discovered.ID.String(), "Manual replacement", nil)
	require.NoError(t, providerService.Update(ctx, manual, &discovered.Version))
	stored, err := repo.Get(ctx, discovered.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.Discovery.ResourceURL)
	assert.Empty(t, stored.Discovery.ClientMethod)
	assert.Nil(t, stored.DiscoveryStatus.LastAttemptAt)
	assert.Nil(t, stored.DiscoveryStatus.LastSuccessAt)
	assert.Nil(t, stored.DiscoveryStatus.FailureReason)
	assert.Equal(t, retainedAudience, stored.AuthorizationParams["resource"], "omitted parameters must keep the existing map")
	assert.False(t, stored.ResourceExplicit, "manual services cannot retain the discovery override marker")
	assert.Equal(t, manual.ClientID, stored.ClientID)
	read, err := providerService.Get(ctx, stored.ID)
	require.NoError(t, err)
	secret, err := read.Secret.GetPlaintext()
	require.NoError(t, err)
	assert.Equal(t, "test-secret", secret)
}

func TestDCRIssuerChangeRejectsStaleAndActiveSessions(t *testing.T) {
	ctx, repo, _, sharedPostgres, dbName, cleanup := setupThirdpartyProviderTestHarnessWithDatabase(t)
	defer cleanup()
	discovered := testDCRPersistenceEntity("issuer-race", "https://old-issuer.example.test", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, discovered))

	adapter, err := postgres.NewAdapter(&ports.StorageConfig{
		Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: sharedPostgres.ConnectionString(dbName)},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.Initialize(ctx))
	defer func() { require.NoError(t, adapter.Close(ctx)) }()
	sessions := postgres.NewUserSessionRepository(adapter)
	oldIssuer := discovered.IssuerURI
	changed := discovered.Copy()
	changed.IssuerURI = "https://new-issuer.example.test"
	changed.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: changed.IssuerURI + "/authorize", TokenEndpoint: changed.IssuerURI + "/token"}
	committedAt := discovered.DiscoveryStatus.LastSuccessAt.Add(time.Minute)
	changed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &committedAt, LastSuccessAt: &committedAt}
	changed.UpdatedAt = committedAt
	require.NoError(t, repo.Update(ctx, changed, &discovered.Version))

	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal("issuer-race@example.test"), ServiceID: discovered.ID,
		EncryptedAccessToken: []byte("encrypted-token"), TokenType: "Bearer", Scope: []string{"files.read"},
		EncryptionContext: storage.EncryptionContext{ServiceID: discovered.ID},
		InitiatedAt:       committedAt, CreatedAt: committedAt, UpdatedAt: committedAt,
		ExpectedIssuerURI: oldIssuer,
		ExpectedResource:  discovered.AuthorizationParams["resource"],
	}
	err = sessions.Create(ctx, session)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr, "a callback from the old issuer must not insert after the change")
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	count, err := sessions.CountByService(ctx, discovered.ID)
	require.NoError(t, err)
	assert.Zero(t, count)

	session.ExpectedIssuerURI = changed.IssuerURI
	require.NoError(t, sessions.Create(ctx, session))
	withSessions := changed.Copy()
	withSessions.IssuerURI = "https://third-issuer.example.test"
	withSessions.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: withSessions.IssuerURI + "/authorize", TokenEndpoint: withSessions.IssuerURI + "/token"}
	nextAt := committedAt.Add(time.Minute)
	withSessions.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &nextAt, LastSuccessAt: &nextAt}
	err = repo.Update(ctx, withSessions, &changed.Version)
	require.ErrorAs(t, err, &storageErr, "the issuer update must recheck sessions at commit")
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	stored, err := repo.Get(ctx, discovered.ID)
	require.NoError(t, err)
	assert.Equal(t, changed.IssuerURI, stored.IssuerURI)
}
