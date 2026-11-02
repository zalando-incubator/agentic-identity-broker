//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func externalIDMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "external_id_45", func(t *testing.T, name string) { pg.ApplyMigrationsUpTo(t, name, 45) })
	db, err := sql.Open("pgx", url)
	require.NoError(t, err)
	dir, err := filepath.Abs(filepath.Join("..", "..", "..", "migrations"))
	require.NoError(t, err)
	f := &MigrationTestFramework{db: db, connStr: url, dbName: name, migrationsDir: dir, cleanup: cleanup}
	t.Cleanup(func() { f.Cleanup(t) })
	return f
}

func insertExternalIDAgent(t *testing.T, f *MigrationTestFramework, value any) string {
	t.Helper()
	agentID := uuid.NewString()
	_, err := f.db.ExecContext(context.Background(), `INSERT INTO agents (id,external_id,display_name,description) VALUES ($1,$2,'Legacy','Definition')`, agentID, value)
	require.NoError(t, err)
	return agentID
}

func externalIDRows(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	result, err := f.QuerySQL(t, `SELECT jsonb_agg(jsonb_build_object('id',id,'external_id',external_id,'permission_sets',permission_sets,'service_requirements',service_requirements,'created_at',created_at,'updated_at',updated_at) ORDER BY id)::text FROM agents`)
	require.NoError(t, err)
	return result
}

func TestMigration046RejectsLegacyValuesAtomically(t *testing.T) {
	for _, mode := range []string{"duplicates", "empty", "both", "bounded"} {
		t.Run(mode, func(t *testing.T) {
			f := externalIDMigrationFixture(t)
			var ids []string
			if mode != "empty" {
				ids = append(ids, insertExternalIDAgent(t, f, "registry"), insertExternalIDAgent(t, f, "registry"))
			}
			if mode != "duplicates" {
				ids = append(ids, insertExternalIDAgent(t, f, ""))
			}
			if mode == "bounded" {
				for i := range 101 {
					insertExternalIDAgent(t, f, fmt.Sprintf("duplicate-%03d", i))
					insertExternalIDAgent(t, f, fmt.Sprintf("duplicate-%03d", i))
					insertExternalIDAgent(t, f, "")
				}
			}
			before := externalIDRows(t, f)
			err := f.Up(t, 46)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "agent external_id uniqueness migration blocked")
			assert.Equal(t, before, externalIDRows(t, f))
			index, indexErr := f.IndexExists(t, "uq_agents_external_id")
			require.NoError(t, indexErr)
			assert.False(t, index)
			index, indexErr = f.IndexExists(t, "idx_agents_external_id")
			require.NoError(t, indexErr)
			assert.True(t, index)
			if mode != "bounded" {
				for _, agentID := range ids {
					assert.Contains(t, err.Error(), agentID)
				}
			} else {
				assert.Contains(t, err.Error(), "remaining")
			}
			version, dirty, versionErr := f.Version(t)
			require.NoError(t, versionErr)
			assert.Equal(t, uint(46), version)
			assert.True(t, dirty)
			_, repairErr := f.db.ExecContext(context.Background(), `UPDATE agents SET external_id = CASE WHEN external_id = '' THEN NULL ELSE id::text END`)
			require.NoError(t, repairErr)
			repaired := externalIDRows(t, f)
			require.NoError(t, f.Force(t, 45))
			require.NoError(t, f.Up(t, 46))
			assert.Equal(t, repaired, externalIDRows(t, f))
		})
	}
}

func TestMigration046RollbackReapplyAndConcurrentEnforcement(t *testing.T) {
	f := externalIDMigrationFixture(t)
	owner := insertExternalIDAgent(t, f, "Registry")
	insertExternalIDAgent(t, f, nil)
	insertExternalIDAgent(t, f, nil)
	insertExternalIDAgent(t, f, "registry")
	insertExternalIDAgent(t, f, " Registry")
	_, err := f.db.ExecContext(context.Background(), `INSERT INTO agent_client_uris(agent_id,client_uri) VALUES($1,'https://agent.example/client.json')`, owner)
	require.NoError(t, err)
	before := externalIDRows(t, f)
	require.NoError(t, f.Up(t, 46))
	assert.Equal(t, before, externalIDRows(t, f))
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() {
			_, err := f.db.ExecContext(context.Background(), `INSERT INTO agents(id,external_id,display_name,description) VALUES($1,'racing','Race','Definition')`, uuid.NewString())
			results <- err
		})
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "23505", pgErr.Code)
		assert.Equal(t, "uq_agents_external_id", pgErr.ConstraintName)
	}
	assert.Equal(t, 1, successes)
	_, err = f.db.ExecContext(context.Background(), `UPDATE agents SET external_id='' WHERE id=$1`, owner)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23514", pgErr.Code)
	after := externalIDRows(t, f)
	require.NoError(t, f.Down(t, 45))
	assert.Equal(t, after, externalIDRows(t, f))
	require.NoError(t, f.Up(t, 46))
	assert.Equal(t, after, externalIDRows(t, f))
	uri, err := f.QuerySQL(t, `SELECT client_uri FROM agent_client_uris WHERE client_uri='https://agent.example/client.json'`)
	require.NoError(t, err)
	assert.Equal(t, "https://agent.example/client.json", uri)
}

func TestMigration046LockFailureDoesNotApplyDDL(t *testing.T) {
	f := externalIDMigrationFixture(t)
	insertExternalIDAgent(t, f, "registry")
	tx, err := f.db.Begin()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`LOCK TABLE agents IN ROW EXCLUSIVE MODE`)
	require.NoError(t, err)
	require.Error(t, f.Up(t, 46))
	require.NoError(t, tx.Rollback())
	index, err := f.IndexExists(t, "uq_agents_external_id")
	require.NoError(t, err)
	assert.False(t, index)
	require.NoError(t, f.Force(t, 45))
	require.NoError(t, f.Up(t, 46))
}
