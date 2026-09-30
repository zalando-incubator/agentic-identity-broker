//go:build integration
// +build integration

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

var agentServiceRequirementsMigrations = []bootstrap.SQLMigration{
	{File: "001_create_agents.up.sql", Version: 1},
	{File: "002_create_thirdparty_services.up.sql", Version: 2},
	{File: "003_create_user_grants.up.sql", Version: 3},
	{File: "004_create_user_sessions.up.sql", Version: 4},
	{File: "005_add_agent_service_requirements.up.sql", Version: 5},
}

// TestAgentServiceRequirementsMigration tests migration 005.
func TestAgentServiceRequirementsMigration(t *testing.T) {
	postgres, dbName, cleanup := setupAgentServiceRequirementsDatabase(t, "agent_service_requirements_base_004", 4)
	defer cleanup()

	t.Run("migration 005 applies cleanly", func(t *testing.T) {
		postgres.ApplyMigration(t, dbName, "005_add_agent_service_requirements.up.sql")

		output := postgres.QuerySQL(t, dbName, `
			SELECT column_name || '|' || data_type || '|' || is_nullable
			FROM information_schema.columns
			WHERE table_name = 'agents'
			  AND column_name = 'service_requirements';
		`)

		assert.Contains(t, output, "service_requirements", "Column should exist")
		assert.Contains(t, output, "jsonb", "Column should be JSONB type")
		assert.Contains(t, output, "YES", "Column should be nullable")
	})

	t.Run("GIN index is created", func(t *testing.T) {
		output := postgres.QuerySQL(t, dbName, `
			SELECT indexname || '|' || indexdef
			FROM pg_indexes
			WHERE tablename = 'agents'
			  AND indexname = 'idx_agents_service_requirements';
		`)

		assert.Contains(t, output, "idx_agents_service_requirements", "Index should exist")
		assert.Contains(t, output, "USING gin", "Index should be GIN type")
	})

	t.Run("existing agents have NULL service_requirements", func(t *testing.T) {
		agentID := postgres.QuerySQL(t, dbName, `
			INSERT INTO agents (id, client_id, display_name, description, created_at, updated_at)
			VALUES (uuid_generate_v4(), 'test-client', 'Test Agent', 'Test Description', NOW(), NOW())
			RETURNING id;
		`)
		require.NotEmpty(t, agentID)

		output := postgres.QuerySQL(t, dbName, `
			SELECT service_requirements IS NULL
			FROM agents
			WHERE client_id = 'test-client';
		`)

		assert.Equal(t, "t", output, "service_requirements should be NULL for new agents")
	})
}

// TestAgentServiceRequirementsMigrationRollback tests migration 005 rollback.
func TestAgentServiceRequirementsMigrationRollback(t *testing.T) {
	postgres, dbName, cleanup := setupAgentServiceRequirementsDatabase(t, "agent_service_requirements_up_005", 5)
	defer cleanup()

	t.Run("migration 005 rollback removes column and index", func(t *testing.T) {
		postgres.ApplyMigration(t, dbName, "005_add_agent_service_requirements.down.sql")

		columnCount := postgres.QuerySQL(t, dbName, `
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_name = 'agents'
			  AND column_name = 'service_requirements';
		`)
		assert.Equal(t, "0", columnCount, "Column should not exist after rollback")

		indexCount := postgres.QuerySQL(t, dbName, `
			SELECT COUNT(*)
			FROM pg_indexes
			WHERE tablename = 'agents'
			  AND indexname = 'idx_agents_service_requirements';
		`)
		assert.Equal(t, "0", indexCount, "Index should not exist after rollback")
	})

	t.Run("can reapply migration after rollback", func(t *testing.T) {
		postgres.ApplyMigration(t, dbName, "005_add_agent_service_requirements.up.sql")

		columnCount := postgres.QuerySQL(t, dbName, `
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_name = 'agents'
			  AND column_name = 'service_requirements';
		`)
		assert.Equal(t, "1", columnCount, "Column should exist after reapplying migration")
	})
}

func setupAgentServiceRequirementsDatabase(t *testing.T, templateKey string, upTo int) (*bootstrap.SharedPostgres, string, func()) {
	t.Helper()

	postgres := bootstrap.RequireSharedPostgres(t)
	dbName, _, cleanup := postgres.SetupDatabaseFromTemplate(t, templateKey, func(dbName string) {
		postgres.ApplyMigrationsUpTo(t, dbName, agentServiceRequirementsMigrations, upTo)
	})

	return postgres, dbName, cleanup
}
