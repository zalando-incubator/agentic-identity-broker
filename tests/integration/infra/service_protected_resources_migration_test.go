//go:build integration
// +build integration

package integration

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

func TestServiceProtectedResourcesMigrationCleanApply(t *testing.T) {
	postgres, dbName, cleanup := setupProtectedResourcesMigrationDatabase(t, "protected_resources_clean_apply")
	defer cleanup()

	seedService(t, postgres, dbName, "11111111-1111-1111-1111-111111111111", "{https://resource.example/a,https://resource.example/b?x=1#fragment}")
	require.NoError(t, migrateOneProtectedResourcesStep(postgres.ConnectionString(dbName), 1))

	assert.Equal(t, "1", postgres.QuerySQL(t, dbName, `
		SELECT version FROM thirdparty_oauth2_services
		WHERE id = '11111111-1111-1111-1111-111111111111';
	`))
	assert.Equal(t, "2", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM service_protected_resources
		WHERE service_id = '11111111-1111-1111-1111-111111111111';
	`))
	assert.Equal(t, "0", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'thirdparty_oauth2_services' AND column_name = 'protected_resources';
	`))
	assert.Equal(t, "1", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM pg_indexes
		WHERE tablename = 'service_protected_resources' AND indexname = 'idx_service_protected_resources_service_id';
	`))
}

func TestServiceProtectedResourcesMigrationRejectsNonCanonicalLegacyValue(t *testing.T) {
	postgres, dbName, cleanup := setupProtectedResourcesMigrationDatabase(t, "protected_resources_noncanonical")
	defer cleanup()

	seedService(t, postgres, dbName, "22222222-2222-2222-2222-222222222222", "{https://resource.example/path/}")
	err := migrateOneProtectedResourcesStep(postgres.ConnectionString(dbName), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-canonical protected resource")
	assert.Equal(t, "1", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'thirdparty_oauth2_services' AND column_name = 'protected_resources';
	`))
	assert.Equal(t, "0", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'service_protected_resources';
	`))
}

func TestServiceProtectedResourcesMigrationRejectsLegacyCollisions(t *testing.T) {
	postgres, dbName, cleanup := setupProtectedResourcesMigrationDatabase(t, "protected_resources_collision")
	defer cleanup()

	seedService(t, postgres, dbName, "33333333-3333-3333-3333-333333333333", "{https://resource.example/shared,https://resource.example/duplicate,https://resource.example/duplicate}")
	seedService(t, postgres, dbName, "44444444-4444-4444-4444-444444444444", "{https://resource.example/shared}")
	err := migrateOneProtectedResourcesStep(postgres.ConnectionString(dbName), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protected resource collision")
	assert.Equal(t, "1", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'thirdparty_oauth2_services' AND column_name = 'protected_resources';
	`))
	assert.Equal(t, "0", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = 'service_protected_resources';
	`))
}

func TestServiceProtectedResourcesMigrationRollbackAndReapplyPreservesResources(t *testing.T) {
	postgres, dbName, cleanup := setupProtectedResourcesMigrationDatabase(t, "protected_resources_rollback_reapply")
	defer cleanup()

	seedService(t, postgres, dbName, "55555555-5555-5555-5555-555555555555", "{https://resource.example/one,https://resource.example/two}")
	connStr := postgres.ConnectionString(dbName)
	require.NoError(t, migrateOneProtectedResourcesStep(connStr, 1))
	require.NoError(t, migrateOneProtectedResourcesStep(connStr, -1))

	assert.Equal(t, "{https://resource.example/one,https://resource.example/two}", postgres.QuerySQL(t, dbName, `
		SELECT protected_resources::text FROM thirdparty_oauth2_services
		WHERE id = '55555555-5555-5555-5555-555555555555';
	`))
	require.NoError(t, migrateOneProtectedResourcesStep(connStr, 1))
	assert.Equal(t, "2", postgres.QuerySQL(t, dbName, `
		SELECT COUNT(*) FROM service_protected_resources
		WHERE service_id = '55555555-5555-5555-5555-555555555555';
	`))
}

func setupProtectedResourcesMigrationDatabase(t *testing.T, templateKey string) (*bootstrap.SharedPostgres, string, func()) {
	t.Helper()

	postgres := bootstrap.RequireSharedPostgres(t)
	dbName, _, cleanup := postgres.SetupDatabaseFromTemplate(t, templateKey, func(t *testing.T, dbName string) {
		postgres.ApplyMigrationsUpTo(t, dbName, 26)
	})
	return postgres, dbName, cleanup
}

func seedService(t *testing.T, postgres *bootstrap.SharedPostgres, dbName, id, resources string) {
	t.Helper()
	postgres.ExecuteSQL(t, dbName, `
		INSERT INTO thirdparty_oauth2_services
			(id, display_name, client_id, client_secret_encrypted, issuer_uri, enable_discovery, scopes, protected_resources)
		VALUES
			('`+id+`', 'migration test service', 'migration-test-`+id+`', '\x01',
			 'https://issuer.example.com', FALSE, '[]', '`+resources+`');
	`)
}

func migrateOneProtectedResourcesStep(connStr string, steps int) error {
	projectRoot, err := bootstrap.FindProjectRoot()
	if err != nil {
		return err
	}

	filename := "028_normalize_service_protected_resources.up.sql"
	if steps < 0 {
		filename = "028_normalize_service_protected_resources.down.sql"
	}
	statement, err := os.ReadFile(filepath.Join(projectRoot, "migrations", filename))
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(string(statement))
	return err
}
