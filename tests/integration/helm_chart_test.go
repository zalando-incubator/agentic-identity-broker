package integration

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func renderHelmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}

	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)

	args := []string{
		"template",
		"broker",
		chartPath,
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token",
	}
	args = append(args, extraArgs...)

	cmd := exec.Command("helm", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.NoError(t, err, stderr.String())

	return stdout.String()
}

func TestHelmTemplate_ProxyUpstreamTimeout(t *testing.T) {
	t.Run("quotes valid Go duration strings", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTimeout=30s",
		)

		require.Contains(t, output, `upstream_timeout: "30s"`)
	})

	t.Run("omits upstream_timeout when not set", func(t *testing.T) {
		output := renderHelmTemplate(t)

		require.NotContains(t, output, "upstream_timeout:")
	})
}

func TestHelmTemplate_ManagedKeysUseBase64StringData(t *testing.T) {
	key := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	output := renderHelmTemplate(t,
		"--set-string", "broker.thirdPartyOauth2.jweSigningKeyBase64="+key,
		"--set-string", "broker.encryption.memory.rawKey="+key,
	)

	require.Contains(t, output, "stringData:\n  signing-key: \""+key+"\"")
	require.Contains(t, output, "stringData:\n  memory-raw-key: \""+key+"\"")
}

func renderedHelmObjects(t *testing.T, extraArgs ...string) []map[string]any {
	t.Helper()

	decoder := yaml.NewDecoder(strings.NewReader(renderHelmTemplate(t, extraArgs...)))
	var objects []map[string]any
	for {
		var object map[string]any
		err := decoder.Decode(&object)
		if err == io.EOF {
			return objects
		}
		require.NoError(t, err)
		if len(object) != 0 {
			objects = append(objects, object)
		}
	}
}

func helmMapping(t *testing.T, object map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, key := range keys {
		value, ok := object[key].(map[string]any)
		require.True(t, ok, "missing YAML mapping %q", key)
		object = value
	}
	return object
}

func helmObjectsOfKind(objects []map[string]any, kind string) []map[string]any {
	var matches []map[string]any
	for _, object := range objects {
		if object["kind"] == kind {
			matches = append(matches, object)
		}
	}
	return matches
}

func helmOnlyObject(t *testing.T, objects []map[string]any, kind string) map[string]any {
	t.Helper()
	matches := helmObjectsOfKind(objects, kind)
	require.Len(t, matches, 1, "expected exactly one %s", kind)
	return matches[0]
}

func helmConfigData(t *testing.T, objects []map[string]any, key string) string {
	t.Helper()
	var matches []string
	for _, object := range helmObjectsOfKind(objects, "ConfigMap") {
		data := helmMapping(t, object, "data")
		if value, ok := data[key].(string); ok {
			matches = append(matches, value)
		}
	}
	require.Len(t, matches, 1, "expected exactly one ConfigMap with %s", key)
	return matches[0]
}

func helmContainers(t *testing.T, podSpec map[string]any, field string) []map[string]any {
	t.Helper()
	values, ok := podSpec[field].([]any)
	require.True(t, ok, "missing pod %s", field)
	var containers []map[string]any
	for _, value := range values {
		container, ok := value.(map[string]any)
		require.True(t, ok, "invalid pod %s", field)
		containers = append(containers, container)
	}
	return containers
}

func helmCommand(t *testing.T, containers ...map[string]any) string {
	t.Helper()
	var parts []string
	for _, container := range containers {
		for _, key := range []string{"command", "args"} {
			if values, ok := container[key].([]any); ok {
				for _, value := range values {
					part, ok := value.(string)
					require.True(t, ok, "invalid %s entry", key)
					parts = append(parts, part)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func helmEnv(t *testing.T, container map[string]any, name string) map[string]any {
	t.Helper()
	env, ok := container["env"].([]any)
	require.True(t, ok, "missing container environment")
	for _, value := range env {
		entry, ok := value.(map[string]any)
		require.True(t, ok, "invalid environment entry")
		if entry["name"] == name {
			return entry
		}
	}
	require.FailNow(t, "missing container environment variable", name)
	return nil
}

func helmExecutableSQL(sql string) string {
	var lines []string
	for _, line := range strings.Split(sql, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func TestHelmTemplate_BusinessEventMaintenance(t *testing.T) {
	t.Run("PostgreSQL schedules maintenance even without optional grants", func(t *testing.T) {
		objects := renderedHelmObjects(t,
			"--set", "storage.type=postgres",
			"--set", "postgresql.external.enabled=true",
			"--set-string", "postgresql.external.host=ledger-db.internal",
			"--set-string", "postgresql.external.database=ledger_db",
			"--set-string", "postgresql.external.sslMode=verify-full",
			"--set-string", "postgresql.external.migrationSecretName=ledger-migration-creds",
			"--set-string", "postgresql.external.brokerSecretName=ledger-broker-creds",
			"--set-string", "postgresql.external.migrationSecretKeys.username=migration_user",
			"--set-string", "postgresql.external.migrationSecretKeys.password=migration_password",
			"--set-string", "migration.grants.image=postgres:16-alpine",
			"--set", "migration.backoffLimit=2",
			"--set", "migration.grants.enabled=false",
			"--set", "broker.businessEvents.telemetryCopyEnabled=false",
		)
		cron := helmOnlyObject(t, objects, "CronJob")
		cronSpec := helmMapping(t, cron, "spec")
		require.Equal(t, "*/5 * * * *", cronSpec["schedule"])
		require.Equal(t, "Forbid", cronSpec["concurrencyPolicy"])
		_, suspended := cronSpec["suspend"]
		require.False(t, suspended, "the chart must not reset an operator's suspension on upgrade")

		jobSpec := helmMapping(t, cronSpec, "jobTemplate", "spec")
		backoff, ok := jobSpec["backoffLimit"].(int)
		require.True(t, ok, "maintenance retries must be explicitly bounded")
		require.GreaterOrEqual(t, backoff, 0)
		require.LessOrEqual(t, backoff, 2)
		podSpec := helmMapping(t, jobSpec, "template", "spec")
		require.Equal(t, "Never", podSpec["restartPolicy"])
		require.Equal(t, true, helmMapping(t, podSpec, "securityContext")["runAsNonRoot"])
		containers := helmContainers(t, podSpec, "containers")
		require.Len(t, containers, 1)
		container := containers[0]
		require.Equal(t, "postgres:16-alpine", container["image"])
		security := helmMapping(t, container, "securityContext")
		require.Equal(t, false, security["allowPrivilegeEscalation"])
		require.Equal(t, true, security["runAsNonRoot"])
		require.Equal(t, true, security["readOnlyRootFilesystem"])
		require.Contains(t, helmMapping(t, security, "capabilities")["drop"], "ALL")
		resources := helmMapping(t, container, "resources")
		for _, field := range []string{"requests", "limits"} {
			values := helmMapping(t, resources, field)
			require.NotEmpty(t, values["cpu"], "maintenance %s must bound CPU", field)
			require.NotEmpty(t, values["memory"], "maintenance %s must bound memory", field)
		}
		command := helmCommand(t, container)
		require.Regexp(t, `(?is)\bpsql\b.*?-v\s+ON_ERROR_STOP=1\b.*?SELECT\s+public\.business_event_maintain_partitions\(\)\s*;`, command)
		require.NotContains(t, command, "business_event_provision_partitions()")
		require.Equal(t, "ledger-db.internal", helmEnv(t, container, "PGHOST")["value"])
		require.Equal(t, "ledger_db", helmEnv(t, container, "PGDATABASE")["value"])
		require.Equal(t, "verify-full", helmEnv(t, container, "PGSSLMODE")["value"])
		require.Equal(t, "-c lock_timeout=5s -c statement_timeout=30s", helmEnv(t, container, "PGOPTIONS")["value"],
			"the deadline must be armed when psql connects, not inside the running function")
		for name, key := range map[string]string{"PGUSER": "migration_user", "PGPASSWORD": "migration_password"} {
			secret := helmMapping(t, helmEnv(t, container, name), "valueFrom", "secretKeyRef")
			require.Equal(t, "ledger-migration-creds", secret["name"])
			require.Equal(t, key, secret["key"])
		}

		migration := helmOnlyObject(t, objects, "Job")
		hooks := helmMapping(t, migration, "metadata", "annotations")["helm.sh/hook"]
		require.Equal(t, "pre-install,pre-upgrade", hooks)
		migrationSpec := helmMapping(t, migration, "spec", "template", "spec")
		require.Equal(t, migrationSpec["serviceAccountName"], podSpec["serviceAccountName"])
		brokerPod := helmMapping(t, helmOnlyObject(t, objects, "Deployment"), "spec", "template", "spec")
		require.NotEqual(t, brokerPod["serviceAccountName"], podSpec["serviceAccountName"])
		migrationContainers := helmContainers(t, migrationSpec, "containers")
		if migrationSpec["initContainers"] != nil {
			migrationContainers = append(helmContainers(t, migrationSpec, "initContainers"), migrationContainers...)
		}
		provisioned := false
		for _, migrationContainer := range migrationContainers {
			migrationCommand := helmCommand(t, migrationContainer)
			require.NotContains(t, migrationCommand, "business_event_maintain_partitions()")
			require.NotContains(t, migrationCommand, "DROP TABLE")
			if strings.Contains(migrationCommand, "business_event_provision_partitions()") {
				require.Regexp(t, `(?is)\bpsql\b.*?-v\s+ON_ERROR_STOP=1\b.*?SELECT\s+public\.business_event_provision_partitions\(\)\s*;`, migrationCommand)
				require.Equal(t, "-c lock_timeout=5s -c statement_timeout=30s", helmEnv(t, migrationContainer, "PGOPTIONS")["value"])
				provisioned = true
			}
		}
		require.True(t, provisioned, "pre-install/pre-upgrade migration must provision partitions")
		for _, object := range helmObjectsOfKind(objects, "ConfigMap") {
			_, hasGrants := helmMapping(t, object, "data")["grants.sql"]
			require.False(t, hasGrants, "optional grants must not be rendered when disabled")
		}
	})

	t.Run("memory has no database maintenance", func(t *testing.T) {
		objects := renderedHelmObjects(t, "--set", "storage.type=memory")
		require.Empty(t, helmObjectsOfKind(objects, "CronJob"))
		require.Empty(t, helmObjectsOfKind(objects, "Job"))
	})
}

func TestHelmTemplate_BusinessEventRuntimeCredentialsAndFalseSwitch(t *testing.T) {
	objects := renderedHelmObjects(t,
		"--set", "storage.type=postgres",
		"--set", "postgresql.external.enabled=true",
		"--set-string", "postgresql.external.migrationSecretName=ledger-migration-creds",
		"--set-string", "postgresql.external.brokerSecretName=ledger-broker-creds",
		"--set", "broker.businessEvents.telemetryCopyEnabled=false",
		"--set", "broker.telemetry.enabled=true",
	)
	configYAML := helmConfigData(t, objects, "config.yaml")
	var config map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(configYAML), &config))
	require.Equal(t, false, helmMapping(t, config, "business_events")["telemetry_copy_enabled"])

	deployment := helmOnlyObject(t, objects, "Deployment")
	podSpec := helmMapping(t, deployment, "spec", "template", "spec")
	for _, field := range []string{"containers", "initContainers"} {
		if podSpec[field] == nil {
			continue
		}
		for _, container := range helmContainers(t, podSpec, field) {
			if env, ok := container["env"].([]any); ok {
				for _, value := range env {
					entry, ok := value.(map[string]any)
					require.True(t, ok)
					if source, ok := entry["valueFrom"].(map[string]any); ok {
						if secret, ok := source["secretKeyRef"].(map[string]any); ok {
							require.NotEqual(t, "ledger-migration-creds", secret["name"])
						}
					}
				}
			}
			if envFrom, ok := container["envFrom"].([]any); ok {
				for _, value := range envFrom {
					entry, ok := value.(map[string]any)
					require.True(t, ok)
					if secret, ok := entry["secretRef"].(map[string]any); ok {
						require.NotEqual(t, "ledger-migration-creds", secret["name"])
					}
				}
			}
		}
	}
	for _, value := range podSpec["volumes"].([]any) {
		volume, ok := value.(map[string]any)
		require.True(t, ok)
		if secret, ok := volume["secret"].(map[string]any); ok {
			require.NotEqual(t, "ledger-migration-creds", secret["secretName"])
		}
		if projected, ok := volume["projected"].(map[string]any); ok {
			for _, source := range projected["sources"].([]any) {
				projection, ok := source.(map[string]any)
				require.True(t, ok)
				if secret, ok := projection["secret"].(map[string]any); ok {
					require.NotEqual(t, "ledger-migration-creds", secret["name"])
				}
			}
		}
	}
	broker := helmContainers(t, podSpec, "containers")[0]
	for _, name := range []string{"DB_USERNAME", "DB_PASSWORD"} {
		secret := helmMapping(t, helmEnv(t, broker, name), "valueFrom", "secretKeyRef")
		require.Equal(t, "ledger-broker-creds", secret["name"])
	}
}

func TestHelmTemplate_BusinessEventLeastPrivilegeGrants(t *testing.T) {
	baseArgs := []string{
		"--set", "storage.type=postgres",
		"--set", "postgresql.external.enabled=true",
	}
	defaultObjects := renderedHelmObjects(t, baseArgs...)
	require.Len(t, helmObjectsOfKind(defaultObjects, "CronJob"), 1)
	migration := helmOnlyObject(t, defaultObjects, "Job")
	migrationSpec := helmMapping(t, migration, "spec", "template", "spec")
	migrationContainers := helmContainers(t, migrationSpec, "containers")
	if migrationSpec["initContainers"] != nil {
		migrationContainers = append(helmContainers(t, migrationSpec, "initContainers"), migrationContainers...)
	}
	provisioned := false
	for _, container := range migrationContainers {
		command := helmCommand(t, container)
		require.NotContains(t, command, "business_event_maintain_partitions()")
		if strings.Contains(command, "business_event_provision_partitions()") {
			require.Regexp(t, `(?is)\bpsql\b.*?-v\s+ON_ERROR_STOP=1\b.*?SELECT\s+public\.business_event_provision_partitions\(\)\s*;`, command)
			provisioned = true
		}
	}
	require.True(t, provisioned, "migration must provision partitions with grants enabled")
	defaultSQL := helmExecutableSQL(helmConfigData(t, defaultObjects, "grants.sql"))
	require.Regexp(t, `(?is)\bREVOKE\s+(?:ALL(?:\s+PRIVILEGES)?|UPDATE\s*,\s*DELETE)\s+ON\s+(?:TABLE\s+)?public\.business_events\b`, defaultSQL)
	require.Regexp(t, `(?is)\b(?:pg_catalog\.)?(?:pg_inherits|pg_partition_tree)\b`, defaultSQL)
	require.Regexp(t, `(?is)\bREVOKE\s+(?:ALL(?:\s+PRIVILEGES)?|INSERT\s*,\s*UPDATE\s*,\s*DELETE)\s+ON\s+`, defaultSQL)
	require.Regexp(t, `(?is)\bALTER\s+DEFAULT\s+PRIVILEGES\b`, defaultSQL)
	for _, function := range []string{
		"business_event_provision_partitions()",
		"business_event_maintain_partitions()",
		"business_event_erase_subject(text)",
	} {
		require.Contains(t, defaultSQL, function)
	}
	require.Regexp(t, `(?is)\bREVOKE\s+(?:ALL|EXECUTE)\s+ON\s+FUNCTION\s+public\.business_event_maintain_partitions\(\)\s+FROM\s+PUBLIC\b`, defaultSQL)
	require.Regexp(t, `(?is)\bREVOKE\s+(?:ALL|EXECUTE)\s+ON\s+FUNCTION\s+public\.business_event_erase_subject\(text\)\s+FROM\s+PUBLIC\b`, defaultSQL)
	require.NotRegexp(t, `(?is)\bGRANT\s+EXECUTE\s+ON\s+FUNCTION\s+public\.business_event_erase_subject\(text\)\b`, defaultSQL)
	require.NotRegexp(t, `(?is)\bGRANT\s+SELECT\s+ON\s+(?:TABLE\s+)?public\.business_events\s*,\s*public\.business_event_delivery_pending\s+TO\b`, defaultSQL)

	readerArgs := append(append([]string{}, baseArgs...), "--set-string", "migration.grants.businessEvents.readerRole=ledger_investigator")
	readerSQL := helmExecutableSQL(helmConfigData(t, renderedHelmObjects(t, readerArgs...), "grants.sql"))
	require.Regexp(t, `(?is)\bGRANT\s+SELECT\s+ON\s+(?:TABLE\s+)?public\.business_events\s*,\s*public\.business_event_delivery_pending\s+TO\b`, readerSQL)
	require.Contains(t, readerSQL, "ledger_investigator")
	require.NotRegexp(t, `(?is)\bGRANT\s+EXECUTE\s+ON\s+FUNCTION\s+public\.business_event_erase_subject\(text\)\b`, readerSQL)

	erasureArgs := append(append([]string{}, baseArgs...), "--set-string", "migration.grants.businessEvents.erasureRole=ledger_eraser")
	erasureSQL := helmExecutableSQL(helmConfigData(t, renderedHelmObjects(t, erasureArgs...), "grants.sql"))
	require.Regexp(t, `(?is)\bGRANT\s+EXECUTE\s+ON\s+FUNCTION\s+public\.business_event_erase_subject\(text\)\s+TO\b`, erasureSQL)
	require.Contains(t, erasureSQL, "ledger_eraser")
	require.NotRegexp(t, `(?is)\bGRANT\s+SELECT\s+ON\s+(?:TABLE\s+)?public\.business_events\s*,\s*public\.business_event_delivery_pending\s+TO\b`, erasureSQL)
	require.NotRegexp(t, `(?i)\bCREATE\s+ROLE\b`, erasureSQL)

	bothArgs := append(append([]string{}, readerArgs...), "--set-string", "migration.grants.businessEvents.erasureRole=ledger_eraser")
	bothSQL := helmExecutableSQL(helmConfigData(t, renderedHelmObjects(t, bothArgs...), "grants.sql"))
	require.Contains(t, bothSQL, "ledger_investigator")
	require.Contains(t, bothSQL, "ledger_eraser")
	require.Regexp(t, `(?is)\bGRANT\s+SELECT\s+ON\s+(?:TABLE\s+)?public\.business_events\s*,\s*public\.business_event_delivery_pending\s+TO\b`, bothSQL)
	require.Regexp(t, `(?is)\bGRANT\s+EXECUTE\s+ON\s+FUNCTION\s+public\.business_event_erase_subject\(text\)\s+TO\b`, bothSQL)
}
