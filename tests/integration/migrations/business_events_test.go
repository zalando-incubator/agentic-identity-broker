//go:build integration

package migrations

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration049BusinessEvents(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 48))
	exists, err := f.TableExists(t, "business_events")
	require.NoError(t, err)
	require.False(t, exists)
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO agents (id, client_id, external_id, display_name, description)
		VALUES ('04900000-0000-0000-0000-000000000001', 'ledger-migration-client',
		        'ledger-migration-external', 'Preserved agent', 'Unrelated agent data');
	`))
	const tablesSQL = `
		SELECT string_agg(tablename, ',' ORDER BY tablename)
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'business_events'
	`
	const agentSQL = `
		SELECT jsonb_build_array(id, client_id, external_id, display_name, description, created_at, updated_at)::text
		FROM agents WHERE id = '04900000-0000-0000-0000-000000000001'
	`
	tablesBefore, err := f.QuerySQL(t, tablesSQL)
	require.NoError(t, err)
	agentBefore, err := f.QuerySQL(t, agentSQL)
	require.NoError(t, err)
	assertUnrelatedData := func(t *testing.T) {
		t.Helper()
		tables, err := f.QuerySQL(t, tablesSQL)
		require.NoError(t, err)
		assert.Equal(t, tablesBefore, tables, "unrelated tables must survive the migration lifecycle")
		agent, err := f.QuerySQL(t, agentSQL)
		require.NoError(t, err)
		assert.JSONEq(t, agentBefore, agent, "unrelated agent data must remain unchanged")
	}

	require.NoError(t, f.Up(t, 49))
	assertBusinessEventsSchema(t, f)
	assertUnrelatedData(t)
	t.Run("defaults and nullable metadata", func(t *testing.T) {
		result, err := f.QuerySQL(t, `
			INSERT INTO business_events (id, event, occurred_at)
			VALUES ('04900000-0000-0000-0000-000000000002', 'consent.grant.revoked', '2025-01-01T00:00:00Z')
			RETURNING recorded_at = now() AND attributes = '{}'::jsonb
			          AND principal IS NULL AND actor IS NULL AND trace_id IS NULL
		`)
		require.NoError(t, err)
		assert.Equal(t, "true", result)

		err = f.ExecuteSQL(t, `
			INSERT INTO business_events (id, event, occurred_at)
			VALUES ('04900000-0000-0000-0000-000000000002', 'consent.grant.revoked', now())
		`)
		var pgErr *pgconn.PgError
		require.ErrorAs(t, err, &pgErr)
		assert.Equal(t, "23505", pgErr.Code)
		assert.Equal(t, "business_events_pkey", pgErr.ConstraintName)
	})
	t.Run("constraints", func(t *testing.T) {
		testBusinessEventsConstraints(t, f)
	})
	t.Run("principal and agent history", func(t *testing.T) {
		testBusinessEventsHistory(t, f)
	})

	require.NoError(t, f.Down(t, 48))
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(48), version)
	assert.False(t, dirty)
	exists, err = f.TableExists(t, "business_events")
	require.NoError(t, err)
	assert.False(t, exists)
	for _, index := range []string{
		"business_events_pkey", "idx_business_events_recorded",
		"idx_business_events_principal", "idx_business_events_agent",
	} {
		exists, err := f.IndexExists(t, index)
		require.NoError(t, err)
		assert.False(t, exists, "%s must be removed by rollback", index)
	}
	assertUnrelatedData(t)

	require.NoError(t, f.Up(t, 49))
	assertBusinessEventsSchema(t, f)
	assertUnrelatedData(t)
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO business_events (id, event, occurred_at)
		VALUES ('04900000-0000-0000-0000-000000000002', 'consent.grant.revoked', now())
	`), "the reapplied table must accept an ID from before rollback")
}

func TestMigration049BusinessEventsAtomicOnIndexFailure(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 48))
	require.NoError(t, f.ExecuteSQL(t, `
		CREATE TABLE ledger_migration_fixture (value text);
		CREATE INDEX idx_business_events_agent ON ledger_migration_fixture (value);
		INSERT INTO ledger_migration_fixture (value) VALUES ('preserved');
	`))
	err := f.Up(t, 49)
	require.ErrorContains(t, err, "idx_business_events_agent")
	require.ErrorContains(t, err, "already exists")
	exists, err := f.TableExists(t, "business_events")
	require.NoError(t, err)
	assert.False(t, exists)
	for _, name := range []string{"business_events_pkey", "idx_business_events_recorded", "idx_business_events_principal"} {
		exists, err := f.IndexExists(t, name)
		require.NoError(t, err)
		assert.False(t, exists, name)
	}
	value, err := f.QuerySQL(t, `SELECT value FROM ledger_migration_fixture`)
	require.NoError(t, err)
	assert.Equal(t, "preserved", value)
}

func assertBusinessEventsSchema(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	version, dirty, err := f.Version(t)
	require.NoError(t, err)
	assert.Equal(t, uint(49), version)
	assert.False(t, dirty)

	columns, err := f.QuerySQL(t, `
		SELECT string_agg(column_name || ':' || udt_name || ':' || is_nullable || ':' ||
		                  COALESCE(column_default, ''), E'\n' ORDER BY ordinal_position)
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = 'business_events'
	`)
	require.NoError(t, err)
	assert.Equal(t, strings.Join([]string{
		"id:uuid:NO:",
		"event:text:NO:",
		"occurred_at:timestamptz:NO:",
		"recorded_at:timestamptz:NO:now()",
		"principal:text:YES:",
		"actor:text:YES:",
		"trace_id:text:YES:",
		"attributes:jsonb:NO:'{}'::jsonb",
	}, "\n"), columns)

	constraints, err := f.QuerySQL(t, `
		SELECT jsonb_object_agg(conname, pg_get_constraintdef(oid))::text
		FROM pg_constraint
		WHERE conrelid = 'public.business_events'::regclass AND contype IN ('p', 'c', 'f', 'u', 'x')
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"business_events_pkey": "PRIMARY KEY (id)",
		"chk_business_events_event_not_empty": "CHECK ((event <> ''::text))",
		"chk_business_events_attributes_object": "CHECK ((jsonb_typeof(attributes) = 'object'::text))"
	}`, constraints, "the ledger must have only its primary key and two checks, with no foreign keys")

	indexes, err := f.QuerySQL(t, `
		SELECT jsonb_object_agg(indexname, indexdef)::text
		FROM pg_indexes WHERE schemaname = 'public' AND tablename = 'business_events'
	`)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"business_events_pkey": "CREATE UNIQUE INDEX business_events_pkey ON public.business_events USING btree (id)",
		"idx_business_events_recorded": "CREATE INDEX idx_business_events_recorded ON public.business_events USING btree (recorded_at, id)",
		"idx_business_events_principal": "CREATE INDEX idx_business_events_principal ON public.business_events USING btree (principal, occurred_at DESC, id DESC)",
		"idx_business_events_agent": "CREATE INDEX idx_business_events_agent ON public.business_events USING btree (((attributes ->> 'agent_id'::text)), occurred_at DESC, id DESC)"
	}`, indexes)
}

func testBusinessEventsConstraints(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	for _, tc := range []struct {
		name       string
		column     int
		value      string
		code       string
		constraint string
	}{
		{name: "null ID", column: 0, value: "NULL", code: "23502"},
		{name: "null event", column: 1, value: "NULL", code: "23502"},
		{name: "null occurrence time", column: 2, value: "NULL", code: "23502"},
		{name: "null storage time", column: 3, value: "NULL", code: "23502"},
		{name: "null attributes", column: 4, value: "NULL", code: "23502"},
		{name: "empty event", column: 1, value: "''", code: "23514", constraint: "chk_business_events_event_not_empty"},
		{name: "array attributes", column: 4, value: "'[]'::jsonb", code: "23514", constraint: "chk_business_events_attributes_object"},
		{name: "JSON null attributes", column: 4, value: "'null'::jsonb", code: "23514", constraint: "chk_business_events_attributes_object"},
		{name: "string attributes", column: 4, value: "'\"value\"'::jsonb", code: "23514", constraint: "chk_business_events_attributes_object"},
		{name: "number attributes", column: 4, value: "'42'::jsonb", code: "23514", constraint: "chk_business_events_attributes_object"},
		{name: "boolean attributes", column: 4, value: "'true'::jsonb", code: "23514", constraint: "chk_business_events_attributes_object"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := []string{"'04900000-0000-0000-0000-000000000003'", "'consent.grant.revoked'", "now()", "now()", "'{}'::jsonb"}
			values[tc.column] = tc.value
			err := f.ExecuteSQL(t, fmt.Sprintf(`
				INSERT INTO business_events (id, event, occurred_at, recorded_at, attributes)
				VALUES (%s)
			`, strings.Join(values, ", ")))
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			assert.Equal(t, tc.code, pgErr.Code)
			if tc.constraint != "" {
				assert.Equal(t, tc.constraint, pgErr.ConstraintName)
			}
		})
	}
}

func testBusinessEventsHistory(t *testing.T, f *MigrationTestFramework) {
	t.Helper()
	require.NoError(t, f.ExecuteSQL(t, `
		INSERT INTO business_events (id, event, occurred_at, principal, attributes) VALUES
		('04900000-0000-0000-0001-000000000003', 'consent.grant.revoked', '2025-01-02T00:00:00Z', 'sha256:owner', '{"agent_id":"external/agent"}'),
		('04900000-0000-0000-0001-000000000001', 'consent.grant.revoked', '2025-01-01T00:00:00Z', 'sha256:owner', '{"agent_id":"external/agent"}'),
		('04900000-0000-0000-0001-000000000002', 'consent.grant.revoked', '2025-01-02T00:00:00Z', 'sha256:owner', '{"agent_id":"external/agent"}'),
		('04900000-0000-0000-0001-000000000004', 'consent.grant.revoked', '2025-01-03T00:00:00Z', 'sha256:other', '{"agent_id":"other-agent"}'),
		('04900000-0000-0000-0001-000000000005', 'consent.grant.revoked', '2025-01-03T00:00:00Z', NULL, '{"agent_id":"external/agent"}'),
		('04900000-0000-0000-0001-000000000006', 'consent.grant.revoked', '2025-01-03T00:00:00Z', 'sha256:owner', '{}'),
		('04900000-0000-0000-0001-000000000007', 'consent.grant.revoked', '2025-01-03T00:00:00Z', NULL, '{}')
	`))
	for _, tc := range []struct {
		name      string
		predicate string
		want      string
	}{
		{
			name:      "principal equality",
			predicate: "principal = 'sha256:owner'",
			want: `["04900000-0000-0000-0001-000000000006", "04900000-0000-0000-0001-000000000003",
			        "04900000-0000-0000-0001-000000000002", "04900000-0000-0000-0001-000000000001"]`,
		},
		{
			name:      "agent equality",
			predicate: "attributes->>'agent_id' = 'external/agent'",
			want: `["04900000-0000-0000-0001-000000000005", "04900000-0000-0000-0001-000000000003",
			        "04900000-0000-0000-0001-000000000002", "04900000-0000-0000-0001-000000000001"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := f.QuerySQL(t, fmt.Sprintf(`
				SELECT array_to_json(ARRAY(
					SELECT id FROM business_events WHERE %s ORDER BY occurred_at DESC, id DESC
				))::text
			`, tc.predicate))
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, rows)
		})
	}
}
