//go:build integration

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func externalIDMigrationFixture(t *testing.T) *MigrationTestFramework {
	t.Helper()
	pg := bootstrap.RequireSharedPostgres(t)
	name, url, cleanup := pg.SetupDatabaseFromTemplate(t, "external_id_46", func(t *testing.T, name string) { pg.ApplyMigrationsUpTo(t, name, 46) })
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

func seedExternalIDRelationships(t *testing.T, f *MigrationTestFramework, agentID string) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO user_grants(id, principal, agent_id, granted_permission_sets) VALUES(gen_random_uuid(),'migration@example.com',$1,'[]')`,
		`INSERT INTO client_credentials(agent_id,secret_hash) VALUES($1,'synthetic-hash')`,
		`INSERT INTO authorization_codes(agent_id,code_hash,principal,redirect_uri,code_challenge,client_id,expires_at) VALUES($1::uuid, $1::text,'migration@example.com','https://example.com/cb','synthetic-challenge','synthetic-client',NOW()+interval '1 hour')`,
		`INSERT INTO refresh_token_sessions(signature,request_id,agent_id,client_id,principal,expires_at) VALUES($1::text,$1::text,$1::uuid,'synthetic-client','migration@example.com',NOW()+interval '1 hour')`,
		`INSERT INTO tool_approvals(id,principal,agent_id,gateway_client_id,tool_name,tool_pattern,arguments_hash,approval_url,expires_at) VALUES(gen_random_uuid(),'migration@example.com',$1,'synthetic-gateway','read','read','synthetic-hash','https://example.com/approval',NOW()+interval '1 hour')`,
		`INSERT INTO agent_client_uris(agent_id,client_uri) VALUES($1,'https://agent.example/client.json')`,
	} {
		_, err := f.db.ExecContext(context.Background(), query, agentID)
		require.NoError(t, err)
	}
	_, err := f.db.ExecContext(context.Background(), `UPDATE agents SET permission_sets=$2::jsonb, service_requirements=$3::jsonb WHERE id=$1`, agentID,
		fmt.Sprintf(`[{"permission_set_id":"%s","requirement_type":"mandatory"}]`, uuid.NewString()),
		fmt.Sprintf(`[{"service_id":"%s","requirement_type":"optional","required_scopes":["read"]}]`, uuid.NewString()))
	require.NoError(t, err)
}

func externalIDRelationships(t *testing.T, f *MigrationTestFramework) string {
	t.Helper()
	result, err := f.QuerySQL(t, `SELECT jsonb_build_object(
		'grants',(SELECT jsonb_agg(to_jsonb(g) ORDER BY g.id) FROM (SELECT id,principal,agent_id,valid_until,granted_permission_sets,created_at,updated_at FROM user_grants) AS g),
		'credentials',(SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM (SELECT id,agent_id,secret_hash,created_at,rotated_at FROM client_credentials) AS c),
		'codes',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM (SELECT id,agent_id,code_hash,client_id,principal,expires_at,created_at FROM authorization_codes) AS a),
		'refresh',(SELECT jsonb_agg(to_jsonb(r) ORDER BY r.signature) FROM (SELECT signature,request_id,agent_id,client_id,principal,grant_id,expires_at,created_at FROM refresh_token_sessions) AS r),
		'approvals',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM (SELECT id,agent_id,principal,tool_name,tool_pattern,status,arguments,created_at,expires_at FROM tool_approvals) AS a),
		'uris',(SELECT jsonb_agg(to_jsonb(u) ORDER BY u.agent_id,u.client_uri) FROM (SELECT agent_id,client_uri FROM agent_client_uris) AS u))::text`)
	require.NoError(t, err)
	return result
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
					ids = append(ids, insertExternalIDAgent(t, f, fmt.Sprintf("duplicate-%03d", i)),
						insertExternalIDAgent(t, f, fmt.Sprintf("duplicate-%03d", i)),
						insertExternalIDAgent(t, f, ""))
				}
			}
			seedExternalIDRelationships(t, f, ids[0])
			relationships := externalIDRelationships(t, f)
			before := externalIDRows(t, f)
			err := f.Up(t, 47)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "agent external_id uniqueness migration blocked")
			assert.Equal(t, before, externalIDRows(t, f))
			assert.Equal(t, relationships, externalIDRelationships(t, f))
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
				assert.Contains(t, err.Error(), "remaining duplicate groups: 2")
				assert.Contains(t, err.Error(), "remaining empty-value agents: 2")
				rows, inventoryErr := f.db.QueryContext(context.Background(), `SELECT external_id, array_agg(id ORDER BY created_at, id) AS agent_ids
					FROM agents WHERE external_id IS NOT NULL AND external_id <> '' GROUP BY external_id HAVING COUNT(*) > 1 ORDER BY external_id`)
				require.NoError(t, inventoryErr)
				groups := 0
				var inventoriedIDs []string
				for rows.Next() {
					var value string
					var agentIDs []string
					require.NoError(t, rows.Scan(&value, pq.Array(&agentIDs)))
					assert.NotEmpty(t, value)
					assert.Len(t, agentIDs, 2)
					inventoriedIDs = append(inventoriedIDs, agentIDs...)
					groups++
				}
				require.NoError(t, rows.Err())
				require.NoError(t, rows.Close())
				assert.Equal(t, 102, groups)
				rows, inventoryErr = f.db.QueryContext(context.Background(), `SELECT id, created_at FROM agents WHERE external_id = '' ORDER BY created_at, id`)
				require.NoError(t, inventoryErr)
				empties := 0
				for rows.Next() {
					var agentID string
					var createdAt time.Time
					require.NoError(t, rows.Scan(&agentID, &createdAt))
					inventoriedIDs = append(inventoriedIDs, agentID)
					empties++
				}
				require.NoError(t, rows.Err())
				require.NoError(t, rows.Close())
				assert.Equal(t, 102, empties)
				assert.ElementsMatch(t, ids, inventoriedIDs)
			}
			version, dirty, versionErr := f.Version(t)
			require.NoError(t, versionErr)
			assert.Equal(t, uint(47), version)
			assert.True(t, dirty)
			_, repairErr := f.db.ExecContext(context.Background(), `UPDATE agents SET external_id = CASE WHEN external_id = '' THEN NULL ELSE id::text END`)
			require.NoError(t, repairErr)
			repaired := externalIDRows(t, f)
			require.NoError(t, f.Force(t, 46))
			require.NoError(t, f.Up(t, 47))
			assert.Equal(t, repaired, externalIDRows(t, f))
			assert.Equal(t, relationships, externalIDRelationships(t, f))
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
	seedExternalIDRelationships(t, f, owner)
	relationships := externalIDRelationships(t, f)
	before := externalIDRows(t, f)
	require.NoError(t, f.Up(t, 47))
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

	mixed := make(chan error, 20)
	for i := range 20 {
		agentID := uuid.NewString()
		if i%2 == 0 {
			agentID = insertExternalIDAgent(t, f, fmt.Sprintf("ordinary-%d", i))
		}
		wg.Go(func() {
			var err error
			if i%2 == 0 {
				_, err = f.db.ExecContext(context.Background(), `UPDATE agents SET external_id='mixed-ordinary' WHERE id=$1`, agentID)
			} else {
				_, err = f.db.ExecContext(context.Background(), `INSERT INTO agents(id,external_id,display_name,description) VALUES($1,'mixed-ordinary','Mixed','Definition')`, agentID)
			}
			mixed <- err
		})
	}
	wg.Wait()
	close(mixed)
	mixedSuccesses := 0
	for err := range mixed {
		if err == nil {
			mixedSuccesses++
			continue
		}
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "23505", pgErr.Code)
		assert.Equal(t, "uq_agents_external_id", pgErr.ConstraintName)
	}
	assert.Equal(t, 1, mixedSuccesses)
	_, err := f.db.ExecContext(context.Background(), `UPDATE agents SET external_id='' WHERE id=$1`, owner)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23514", pgErr.Code)
	after := externalIDRows(t, f)
	require.NoError(t, f.Down(t, 46))
	assert.Equal(t, after, externalIDRows(t, f))
	assert.Equal(t, relationships, externalIDRelationships(t, f))
	require.NoError(t, f.Up(t, 47))
	assert.Equal(t, after, externalIDRows(t, f))
	assert.Equal(t, relationships, externalIDRelationships(t, f))
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
	require.Error(t, f.Up(t, 47))
	require.NoError(t, tx.Rollback())
	index, err := f.IndexExists(t, "uq_agents_external_id")
	require.NoError(t, err)
	assert.False(t, index)
	require.NoError(t, f.Force(t, 46))
	require.NoError(t, f.Up(t, 47))
}
