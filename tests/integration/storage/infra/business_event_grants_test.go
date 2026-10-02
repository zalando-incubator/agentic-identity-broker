//go:build integration
// +build integration

package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func renderBusinessEventGrants(t *testing.T, readerRole, erasureRole string) string {
	t.Helper()
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	args := []string{
		"template", "broker", filepath.Join(root, "charts", "agentic-identity-broker"),
		"--show-only", "templates/configmap-grants.yaml",
		"--set", "storage.type=postgres",
		"--set", "postgresql.external.enabled=true",
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token",
	}
	if readerRole != "" {
		args = append(args, "--set-string", "migration.grants.businessEvents.readerRole="+readerRole)
	}
	if erasureRole != "" {
		args = append(args, "--set-string", "migration.grants.businessEvents.erasureRole="+erasureRole)
	}
	output, err := exec.Command("helm", args...).CombinedOutput()
	require.NoError(t, err, string(output))
	var configMap struct {
		Kind string            `yaml:"kind"`
		Data map[string]string `yaml:"data"`
	}
	require.NoError(t, yaml.Unmarshal(output, &configMap))
	require.Equal(t, "ConfigMap", configMap.Kind)
	require.NotEmpty(t, configMap.Data["grants.sql"])
	return configMap.Data["grants.sql"]
}

func requireBusinessEventGrantDenied(t *testing.T, db *sqlx.DB, role, query string, args ...any) {
	t.Helper()
	tx, err := db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(`SET LOCAL ROLE ` + role)
	require.NoError(t, err)
	_, err = tx.Exec(query, args...)
	var permission *pgconn.PgError
	require.ErrorAs(t, err, &permission, "statement must be rejected by PostgreSQL privileges: %s", query)
	require.Equal(t, "42501", permission.Code, "statement must be rejected by PostgreSQL privileges: %s", query)
}

func TestBusinessEventRenderedHelmGrantsEnforceRuntimePrivileges(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	pg := bootstrap.RequireSharedPostgres(t)
	migrations := ledgerFoundationMigrations(t)
	for _, tc := range []struct {
		name, operationalRoles string
	}{
		{name: "default"},
		{name: "reader and erasure roles equal runtime", operationalRoles: "runtime"},
		{name: "distinct operational roles", operationalRoles: "distinct"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbName, connectionURL, drop := pg.SetupDatabaseFromTemplate(t, "business_event_foundation_035", func(name string) {
				pg.ApplyMigrationsUpTo(t, name, migrations, 35)
			})
			runtimeRole := "grants_" + dbName
			roles := []string{runtimeRole}
			readerRole, erasureRole := "", ""
			switch tc.operationalRoles {
			case "runtime":
				readerRole, erasureRole = runtimeRole, runtimeRole
			case "distinct":
				readerRole, erasureRole = "reader_"+dbName, "eraser_"+dbName
				roles = append(roles, readerRole, erasureRole)
			}
			t.Cleanup(func() {
				drop()
				for _, role := range roles {
					pg.ExecuteSQL(t, "postgres", `DROP ROLE IF EXISTS "`+role+`"`)
				}
			})
			for _, role := range roles {
				pg.ExecuteSQL(t, "postgres", `CREATE ROLE "`+role+`" NOLOGIN`)
			}
			pg.ExecutePSQLScript(t, dbName, runtimeRole, renderBusinessEventGrants(t, readerRole, erasureRole))

			db, err := sqlx.Connect("pgx", connectionURL)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			ctx := context.Background()
			event := ledgerExample(t, "agent-registered", id.Principal(""))
			event.RecordedAt = time.Now().UTC().Truncate(time.Microsecond)
			event.OccurredAt = event.RecordedAt
			envelope, err := json.Marshal(event)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
				VALUES ($1,$2,$3,$4,$5,NULL,$6::jsonb)`, event.RecordedAt, event.ID, event.OccurredAt,
				event.Type, event.Outcome, envelope)
			require.NoError(t, err)

			copiedID := id.NewBusinessEventID()
			tx, err := db.BeginTxx(ctx, nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			_, err = tx.Exec(`SET LOCAL ROLE ` + runtimeRole)
			require.NoError(t, err)
			var count int
			require.NoError(t, tx.Get(&count, `SELECT count(*) FROM public.business_events WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID))
			require.Equal(t, 1, count, "runtime can read ledger history through its parent")
			_, err = tx.Exec(`INSERT INTO public.business_events(recorded_at,id,occurred_at,type,outcome,subject,envelope)
				SELECT recorded_at,$1::uuid,occurred_at,type,outcome,subject,jsonb_set(envelope,'{id}',to_jsonb(($1::uuid)::text))
				FROM public.business_events WHERE recorded_at=$2 AND id=$3`, copiedID, event.RecordedAt, event.ID)
			require.NoError(t, err, "runtime can insert through the ledger parent")
			_, err = tx.Exec(`INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at)
				VALUES($1,$2,clock_timestamp())`, event.RecordedAt, copiedID)
			require.NoError(t, err, "runtime can queue delivery references through their parent")
			require.NoError(t, tx.Get(&count, `SELECT count(*) FROM public.business_event_delivery_pending WHERE recorded_at=$1 AND event_id=$2`, event.RecordedAt, copiedID))
			require.Equal(t, 1, count)
			_, err = tx.Exec(`UPDATE public.business_event_policy SET retention_microseconds=$1 WHERE singleton`, (36 * time.Hour).Microseconds())
			require.NoError(t, err, "runtime can persist validated retention")
			require.NoError(t, tx.Commit())
			var retention int64
			require.NoError(t, db.Get(&retention, `SELECT retention_microseconds FROM public.business_event_policy WHERE singleton`))
			require.Equal(t, (36 * time.Hour).Microseconds(), retention)

			requireBusinessEventGrantDenied(t, db, runtimeRole,
				`UPDATE public.business_events SET outcome=outcome WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID)
			requireBusinessEventGrantDenied(t, db, runtimeRole,
				`DELETE FROM public.business_events WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID)
			for _, function := range []string{
				"business_event_erase_subject('not-recorded')",
				"business_event_provision_partitions()",
				"business_event_maintain_partitions()",
			} {
				requireBusinessEventGrantDenied(t, db, runtimeRole, `SELECT public.`+function)
			}
			for _, parent := range []struct{ table, key string }{
				{table: "business_events", key: "id"},
				{table: "business_event_delivery_pending", key: "event_id"},
			} {
				var child string
				require.NoError(t, db.Get(&child, `SELECT format('%I.%I',n.nspname,c.relname)
					FROM pg_catalog.pg_inherits i JOIN pg_catalog.pg_class c ON c.oid=i.inhrelid
					JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
					WHERE i.inhparent=$1::regclass ORDER BY c.relname LIMIT 1`, "public."+parent.table))
				requireBusinessEventGrantDenied(t, db, runtimeRole,
					fmt.Sprintf(`SELECT count(*) FROM %s WHERE recorded_at=$1 AND %s=$2`, child, parent.key), event.RecordedAt, copiedID)
				requireBusinessEventGrantDenied(t, db, runtimeRole,
					fmt.Sprintf(`INSERT INTO %s SELECT * FROM public.%s WHERE recorded_at=$1 AND %s=$2`, child, parent.table, parent.key),
					event.RecordedAt, copiedID)
				for _, privilege := range []string{"SELECT", "INSERT", "UPDATE", "DELETE"} {
					var granted bool
					require.NoError(t, db.Get(&granted, `SELECT has_table_privilege($1, $2, $3)`, runtimeRole, child, privilege))
					require.False(t, granted, "runtime must not directly %s partition %s", privilege, child)
				}
			}
			if tc.operationalRoles == "distinct" {
				reader, err := db.BeginTxx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = reader.Rollback() }()
				_, err = reader.Exec(`SET LOCAL ROLE ` + readerRole)
				require.NoError(t, err)
				require.NoError(t, reader.Get(&count, `SELECT count(*) FROM public.business_events WHERE recorded_at=$1 AND id=$2`, event.RecordedAt, event.ID))
				require.Equal(t, 1, count, "rendered reader role can read parent history")
				require.NoError(t, reader.Rollback())
				var erased int64
				eraser, err := db.BeginTxx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = eraser.Rollback() }()
				_, err = eraser.Exec(`SET LOCAL ROLE ` + erasureRole)
				require.NoError(t, err)
				require.NoError(t, eraser.Get(&erased, `SELECT public.business_event_erase_subject('not-recorded')`))
				require.Zero(t, erased, "rendered erasure role can invoke the protected function")
				require.NoError(t, eraser.Rollback())
			}
		})
	}
}
