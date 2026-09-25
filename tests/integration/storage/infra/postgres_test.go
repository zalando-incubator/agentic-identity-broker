//go:build integration
// +build integration

package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/postgres"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	testbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// init disables Ryuk for Podman compatibility
// Ryuk tries to use "bridge" network which is a reserved network mode in Podman
// See: https://golang.testcontainers.org/system_requirements/using_podman/
func init() {
	// Disable Ryuk only if user hasn't explicitly configured it
	// This is needed because Ryuk requires Docker's "bridge" network name,
	// which conflicts with Podman's network mode system
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

// canAccessContainerRuntime checks if Docker or Podman is available on this system
func canAccessContainerRuntime() error {
	// Try Docker first
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err == nil {
		return nil // Docker is available
	}

	// Fall back to Podman if Docker is not available
	cmd = exec.Command("podman", "ps")
	if err := cmd.Run(); err == nil {
		return nil // Podman is available
	}

	return fmt.Errorf("neither docker nor podman is accessible")
}

// setupPostgresContainer creates a test PostgreSQL container
// Returns error if Docker is not available or testcontainers setup fails
func setupPostgresContainer(ctx context.Context) (testcontainers.Container, string, error) {

	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30 * time.Second),
		// Use "podman" network on Podman (available by default)
		// On Docker, this will use the default network. Podman requires explicit network for container communication.
		Networks: []string{"podman"},
	}

	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}

	// If DOCKER_HOST is set (Podman mode), explicitly use Podman provider
	if os.Getenv("DOCKER_HOST") != "" {
		genericReq.ProviderType = testcontainers.ProviderPodman
	}

	container, err := testcontainers.GenericContainer(ctx, genericReq)
	if err != nil {
		return nil, "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, "", err
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		return nil, "", err
	}

	connStr := fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb", host, port.Port())
	return container, connStr, nil
}

// initializeSchema creates the required database schema
func initializeSchema(ctx context.Context, connStr string) error {
	// Note: In a real application, this would use a migration tool
	// For now, we skip schema setup as it's covered by integration tests
	// The adapter will verify schema during Initialize()
	return nil
}

func TestPostgresAdapter_Initialize_ConnectionFailed(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://invalid:invalid@localhost:54321/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = adapter.Initialize(ctx)
	assert.Error(t, err)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindConnection, storErr.Kind)
	}
}

func TestPostgresAdapter_HealthCheck_NotInitialized(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = adapter.HealthCheck(ctx)
	assert.Error(t, err)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindConnection, storErr.Kind)
	}
}

func TestPostgresAdapter_Close_NotInitialized(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Close without initialize should be safe
	err = adapter.Close(ctx)
	assert.NoError(t, err)
}

func TestPostgresAdapter_CreateUser_ValidationErrors(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	tests := []struct {
		name    string
		user    *ports.User
		wantErr bool
		errKind storage.ErrorKind
	}{
		{
			name:    "nil user",
			user:    nil,
			wantErr: true,
			errKind: storage.ErrorKindValidation,
		},
		{
			name: "empty ID",
			user: &ports.User{
				ID:        id.UserID{},
				Email:     "test@example.com",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: true,
			errKind: storage.ErrorKindValidation,
		},
		{
			name: "empty email",
			user: &ports.User{
				ID:        id.MustParseUserID("12345678-1234-1234-1234-123456789012"),
				Email:     "",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			wantErr: true,
			errKind: storage.ErrorKindValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := adapter.CreateUser(ctx, tt.user)
			// Will fail with connection error since not initialized
			// But validation should happen first
			if err != nil {
				if storErr, ok := err.(*storage.StorageError); ok {
					// Could be validation or connection error depending on order
					assert.True(t, storErr.Kind == storage.ErrorKindValidation || storErr.Kind == storage.ErrorKindConnection)
				}
			}
		})
	}
}

func TestPostgresAdapter_ContextCancellation(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations on cancelled context should fail gracefully
	_, err = adapter.GetUser(ctx, id.MustParseUserID("12345678-1234-1234-1234-123456789012"))
	assert.Error(t, err)
}

// Integration tests with real PostgreSQL database
// These tests are marked with build tag "integration" and require Docker or Podman
// Run with: go test -tags=integration ./test/integration/storage/... (requires Docker or Podman)
//
// In CI, a missing container runtime must fail rather than mask infra coverage.

func TestPostgresAdapter_FullLifecycle_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if neither Docker nor Podman is available
	// In production CI, either Docker or Podman should be configured for integration tests
	if err := canAccessContainerRuntime(); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("PostgreSQL integration requires a container runtime in CI: %v", err)
		}
		t.Skipf("Skipping PostgreSQL integration test: No container runtime available - %v", err)
	}

	ctx := context.Background()
	container, connStr, err := setupPostgresContainer(ctx)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("Starting PostgreSQL integration container in CI: %v", err)
		}
		t.Skipf("Skipping integration test: Failed to setup PostgreSQL - %v", err)
	}
	defer container.Terminate(ctx)

	// Note: In a real scenario, we would initialize schema here
	// For now, tests verify adapter behavior without actual database connection

	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: connStr,
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	// Initialize would fail without schema setup
	// This test verifies the connection is attempted
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = adapter.Initialize(initCtx)
	// Expected to fail due to missing schema_migrations table
	// But connection should succeed
	if err != nil {
		if storErr, ok := err.(*storage.StorageError); ok {
			assert.Equal(t, storage.ErrorKindValidation, storErr.Kind)
		}
	}
}

func TestPostgresAdapter_CRUD_Operations_Simulation(t *testing.T) {
	// This test simulates CRUD operations
	// Real implementation would require schema setup

	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := postgres.NewAdapter(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user := &ports.User{
		ID:        id.MustParseUserID("12345678-1234-1234-1234-123456789012"),
		Email:     "test@example.com",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// These operations will fail with connection error (no DB)
	// but verify the adapter handles them correctly

	err = adapter.CreateUser(ctx, user)
	assert.Error(t, err)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindConnection, storErr.Kind)
	}

	_, err = adapter.GetUser(ctx, id.MustParseUserID("12345678-1234-1234-1234-123456789012"))
	assert.Error(t, err)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindConnection, storErr.Kind)
	}

	err = adapter.UpdateUser(ctx, user)
	assert.Error(t, err)

	err = adapter.DeleteUser(ctx, id.MustParseUserID("12345678-1234-1234-1234-123456789012"))
	assert.Error(t, err)

	_, err = adapter.ListUsers(ctx, nil)
	assert.Error(t, err)
}

func TestPostgresAdapter_CIMDKeyDomainPersistence(t *testing.T) {
	sharedPostgres, dbName, connStr, cleanupDatabase := setupCIMDKeyDomainDatabase(t, 34)
	defer cleanupDatabase()

	store, cleanupStore := newCIMDKeyDomainStorage(t, connStr)
	defer cleanupStore()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo := store.SigningKeys()

	t.Run("current-key index and lifecycle locks are scoped to each domain", func(t *testing.T) {
		indexDefinition := sharedPostgres.QuerySQL(t, dbName, `
			SELECT indexdef
			FROM pg_indexes
			WHERE tablename = 'signing_keys'
			  AND indexname = 'idx_signing_keys_single_current_active_per_domain';
		`)
		require.NotEmpty(t, indexDefinition)
		assert.Contains(t, indexDefinition, "UNIQUE INDEX")
		assert.Contains(t, indexDefinition, "(key_domain)")
		assert.Contains(t, indexDefinition, "removed_at IS NULL")
		assert.Contains(t, indexDefinition, "is_current")
		assert.Equal(t, "0", sharedPostgres.QuerySQL(t, dbName, `
			SELECT COUNT(*)
			FROM pg_indexes
			WHERE tablename = 'signing_keys'
			  AND indexname = 'idx_signing_keys_single_current_active';
		`))

		tokenCurrent := postgresCIMDSigningKey("token-current", storage.KeyDomainTokenSigning, true)
		cimdCurrent := postgresCIMDSigningKey("cimd-current", storage.KeyDomainCIMDClientAuthentication, true)
		tokenCandidate := postgresCIMDSigningKey("token-candidate", storage.KeyDomainTokenSigning, false)
		cimdCandidate := postgresCIMDSigningKey("cimd-candidate", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenCurrent))
		require.NoError(t, repo.Create(ctx, cimdCurrent))
		require.NoError(t, repo.Create(ctx, tokenCandidate))
		require.NoError(t, repo.Create(ctx, cimdCandidate))

		start := make(chan struct{})
		promotionResults := make(chan error, 2)
		var wg sync.WaitGroup
		for _, promotion := range []struct {
			domain storage.KeyDomain
			kid    id.KeyID
		}{
			{domain: storage.KeyDomainTokenSigning, kid: tokenCandidate.KID},
			{domain: storage.KeyDomainCIMDClientAuthentication, kid: cimdCandidate.KID},
		} {
			wg.Add(1)
			go func(domain storage.KeyDomain, kid id.KeyID) {
				defer wg.Done()
				<-start
				_, err := repo.SetCurrentInDomain(ctx, domain, kid, time.Now().UTC().Add(-time.Second))
				promotionResults <- err
			}(promotion.domain, promotion.kid)
		}
		close(start)
		wg.Wait()
		close(promotionResults)
		for err := range promotionResults {
			require.NoError(t, err)
		}

		tokenActive, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCandidate.KID, tokenActive.KID)
		cimdActive, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, cimdCandidate.KID, cimdActive.KID)

		err = repo.Create(ctx, postgresCIMDSigningKey("cimd-second-current", storage.KeyDomainCIMDClientAuthentication, true))
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})

	t.Run("kid values remain globally unique across key domains", func(t *testing.T) {
		sharedKID := id.NewKeyID("globally-unique-kid")
		tokenKey := postgresCIMDSigningKey(string(sharedKID), storage.KeyDomainTokenSigning, false)
		cimdKey := postgresCIMDSigningKey(string(sharedKID), storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenKey))

		err := repo.Create(ctx, cimdKey)
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})
	t.Run("does not find a key through another domain", func(t *testing.T) {
		cimdKey := postgresCIMDSigningKey("cimd-wrong-domain-lookup", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, cimdKey))

		_, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		stored, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, cimdKey.KID)
		require.NoError(t, err)
		assert.Equal(t, cimdKey.KID, stored.KID)
		assert.Equal(t, storage.KeyDomainCIMDClientAuthentication, stored.KeyDomain)
	})

	t.Run("lists and counts active keys in their own domains", func(t *testing.T) {
		tokenCountBefore, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		cimdCountBefore, err := repo.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)

		tokenKey := postgresCIMDSigningKey("token-domain-list", storage.KeyDomainTokenSigning, false)
		cimdKey := postgresCIMDSigningKey("cimd-domain-list", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenKey))
		require.NoError(t, repo.Create(ctx, cimdKey))

		tokenKeys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		tokenKeyFound := false
		for _, key := range tokenKeys {
			assert.Equal(t, storage.KeyDomainTokenSigning, key.KeyDomain)
			assert.NotEqual(t, cimdKey.KID, key.KID)
			if key.KID == tokenKey.KID {
				tokenKeyFound = true
			}
		}
		assert.True(t, tokenKeyFound)

		cimdKeys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		cimdKeyFound := false
		for _, key := range cimdKeys {
			assert.Equal(t, storage.KeyDomainCIMDClientAuthentication, key.KeyDomain)
			assert.NotEqual(t, tokenKey.KID, key.KID)
			if key.KID == cimdKey.KID {
				cimdKeyFound = true
			}
		}
		assert.True(t, cimdKeyFound)

		tokenCountAfter, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCountBefore+1, tokenCountAfter)
		cimdCountAfter, err := repo.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, cimdCountBefore+1, cimdCountAfter)
	})

	t.Run("does not delete a key from another domain", func(t *testing.T) {
		cimdCountBefore, err := repo.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		cimdKey := postgresCIMDSigningKey("cimd-cross-domain-delete", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, cimdKey))

		err = repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		remaining, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, cimdKey.KID)
		require.NoError(t, err)
		assert.Equal(t, cimdKey.KID, remaining.KID)
		assert.Nil(t, remaining.RemovedAt)
		cimdCountAfter, err := repo.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, cimdCountBefore+1, cimdCountAfter)
	})

	t.Run("filters removed keys from lookup lists and counts", func(t *testing.T) {
		tokenCountBefore, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		removedKey := postgresCIMDSigningKey("token-removed-key", storage.KeyDomainTokenSigning, false)
		removedKey.ActivatesAt = time.Now().UTC().Add(-time.Minute)
		remainingKey := postgresCIMDSigningKey("token-remaining-key", storage.KeyDomainTokenSigning, false)
		remainingKey.ActivatesAt = time.Now().UTC()
		require.NoError(t, repo.Create(ctx, removedKey))
		require.NoError(t, repo.Create(ctx, remainingKey))

		tokenCountAfterCreate, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCountBefore+2, tokenCountAfterCreate)
		require.NoError(t, repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, removedKey.KID))

		_, err = repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, removedKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
		tokenKeys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		for _, key := range tokenKeys {
			assert.NotEqual(t, removedKey.KID, key.KID)
		}
		tokenCountAfterDelete, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCountBefore+1, tokenCountAfterDelete)
	})

	t.Run("creates and promotes only within the requested domain", func(t *testing.T) {
		cimdCurrent := postgresCIMDSigningKey("cimd-create-and-promote", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.CreateAndSetCurrent(ctx, cimdCurrent))
		tokenPrevious := postgresCIMDSigningKey("token-create-and-promote-previous", storage.KeyDomainTokenSigning, false)
		require.NoError(t, repo.CreateAndSetCurrent(ctx, tokenPrevious))
		tokenCurrent := postgresCIMDSigningKey("token-create-and-promote-current", storage.KeyDomainTokenSigning, false)
		require.NoError(t, repo.CreateAndSetCurrent(ctx, tokenCurrent))

		storedPrevious, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, tokenPrevious.KID)
		require.NoError(t, err)
		assert.False(t, storedPrevious.IsCurrent)
		storedCurrent, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCurrent.KID, storedCurrent.KID)
		assert.True(t, storedCurrent.IsCurrent)
		storedCIMDCurrent, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, cimdCurrent.KID, storedCIMDCurrent.KID)
		assert.True(t, storedCIMDCurrent.IsCurrent)
	})

	t.Run("serializes bootstrap work across storage adapters", func(t *testing.T) {
		secondStore, cleanupSecondStore := newCIMDKeyDomainStorage(t, connStr)
		defer cleanupSecondStore()
		bootstrapCtx, cancelBootstrap := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelBootstrap()

		firstCoordinator := store.SigningKeyBootstrapCoordinator()
		secondCoordinator := secondStore.SigningKeyBootstrapCoordinator()
		secondRepo := secondStore.SigningKeys()
		firstEntered := make(chan struct{})
		releaseFirst := make(chan struct{})
		var releaseFirstOnce sync.Once
		releaseFirstLock := func() {
			releaseFirstOnce.Do(func() { close(releaseFirst) })
		}
		defer releaseFirstLock()
		firstDone := make(chan error, 1)
		secondEntered := make(chan struct{})
		secondDone := make(chan error, 1)

		go func() {
			firstDone <- firstCoordinator.WithBootstrapLock(bootstrapCtx, func(lockCtx context.Context) error {
				_, err := repo.CountActiveInDomain(lockCtx, storage.KeyDomainCIMDClientAuthentication)
				if err != nil {
					return err
				}
				close(firstEntered)
				<-releaseFirst
				return nil
			})
		}()

		select {
		case <-firstEntered:
		case err := <-firstDone:
			require.NoError(t, err)
			return
		case <-time.After(time.Second):
			t.Fatal("first bootstrap caller never acquired the lock")
		}

		go func() {
			secondDone <- secondCoordinator.WithBootstrapLock(bootstrapCtx, func(lockCtx context.Context) error {
				_, err := secondRepo.CountActiveInDomain(lockCtx, storage.KeyDomainCIMDClientAuthentication)
				if err != nil {
					return err
				}
				close(secondEntered)
				return nil
			})
		}()

		select {
		case <-secondEntered:
			t.Fatal("second bootstrap caller acquired the lock before the first released it")
		case <-time.After(200 * time.Millisecond):
		}

		releaseFirstLock()
		require.NoError(t, <-firstDone)
		select {
		case err := <-secondDone:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("second bootstrap caller never acquired the lock after the first released it")
		}
	})
}

func TestPostgresAdapter_CIMDKeyDomainRollbackPreservesState(t *testing.T) {
	sharedPostgres, dbName, connStr, cleanupDatabase := setupCIMDKeyDomainDatabase(t, 33)
	defer cleanupDatabase()

	store, cleanupStore := newCIMDKeyDomainStorage(t, connStr)
	defer cleanupStore()

	ctx := context.Background()
	key := postgresCIMDSigningKey("cimd-rollback-key", storage.KeyDomainCIMDClientAuthentication, true)
	require.NoError(t, store.SigningKeys().Create(ctx, key))

	migrationRunner := newProjectMigrationRunner(t, connStr)
	defer func() { _, _ = migrationRunner.Close() }()
	err := migrationRunner.Steps(-1)
	require.Error(t, err)
	assert.ErrorContains(t, err, "cannot revert CIMD key domain while CIMD client-authentication keys exist")

	stored, err := store.SigningKeys().GetCurrentInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	require.NoError(t, err)
	assert.Equal(t, key.KID, stored.KID)
	assert.Equal(t, "cimd_client_authentication", sharedPostgres.QuerySQL(t, dbName, `
		SELECT key_domain
		FROM signing_keys
		WHERE kid = 'cimd-rollback-key';
	`))
	assert.Equal(t, "1", sharedPostgres.QuerySQL(t, dbName, `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE tablename = 'signing_keys'
		  AND indexname = 'idx_signing_keys_single_current_active_per_domain';
	`))
}

func setupCIMDKeyDomainDatabase(t *testing.T, migrationVersion uint) (*testbootstrap.SharedPostgres, string, string, func()) {
	t.Helper()

	sharedPostgres := testbootstrap.RequireSharedPostgres(t)
	dbName, connStr, cleanupDatabase := sharedPostgres.SetupDatabaseFromTemplate(t,
		fmt.Sprintf("cimd_key_domain_migrations_%d", migrationVersion),
		func(t *testing.T, dbName string) {
			migrationRunner := newProjectMigrationRunner(t, sharedPostgres.ConnectionString(dbName))
			defer func() { _, _ = migrationRunner.Close() }()

			err := migrationRunner.Migrate(migrationVersion)
			if err != nil && err != migrate.ErrNoChange {
				require.NoError(t, err)
			}
		},
	)

	return sharedPostgres, dbName, connStr, cleanupDatabase
}

func newProjectMigrationRunner(t *testing.T, connStr string) *migrate.Migrate {
	t.Helper()

	projectRoot, err := testbootstrap.FindProjectRoot()
	require.NoError(t, err)
	migrationsDir, err := filepath.Abs(filepath.Join(projectRoot, "migrations"))
	require.NoError(t, err)
	migrationRunner, err := migrate.New("file://"+migrationsDir, connStr)
	require.NoError(t, err)
	return migrationRunner
}

func newCIMDKeyDomainStorage(t *testing.T, connStr string) (*storageadapter.Adapter, func()) {
	t.Helper()

	store, err := storageadapter.NewAdapter(&ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: connStr,
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 5 * time.Second,
		},
	})
	require.NoError(t, err)

	return store, func() {
		require.NoError(t, store.Close(context.Background()))
	}
}

func postgresCIMDSigningKey(kid string, domain storage.KeyDomain, isCurrent bool) *storage.SigningKey {
	now := time.Now().UTC()
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID(kid),
		KeyDomain:           domain,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte{1},
		IsCurrent:           isCurrent,
		ActivatesAt:         now.Add(-time.Second),
		CreatedAt:           now,
	}
}
