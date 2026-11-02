//go:build integration

package migrations

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

type onlineMigrationCLI struct {
	runtime     string
	image       string
	containerID string
	commands    map[uint]onlineMigrationCommand
}

type onlineMigrationCommand struct {
	Name    string   `yaml:"name"`
	Command []string `yaml:"command"`
	Args    []string `yaml:"args"`
}

func newOnlineMigrationCLI(t *testing.T, f *MigrationTestFramework) *onlineMigrationCLI {
	t.Helper()
	postgres := bootstrap.RequireSharedPostgres(t)
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	var runtime string
	for _, candidate := range []string{"docker", "podman"} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := exec.CommandContext(ctx, candidate, "ps").Run()
		cancel()
		if err == nil {
			runtime = candidate
			break
		}
	}
	require.NotEmpty(t, runtime, "the real migration CLI proof requires the same container runtime as disposable PostgreSQL")
	cli := &onlineMigrationCLI{
		runtime: runtime, image: onlineMigrationImage(t, runtime, root),
		containerID: postgres.ContainerID(), commands: make(map[uint]onlineMigrationCommand, 1),
	}
	cli.commands[46] = renderedOnlineMigrationCommand(t, root, f.dbName, 46, false)
	return cli
}

func onlineMigrationImage(t *testing.T, runtime, root string) string {
	t.Helper()
	image := os.Getenv("BROKER_MIGRATION_TEST_IMAGE")
	if image != "" {
		output, err := exec.Command(runtime, "image", "inspect", image).CombinedOutput()
		require.NoError(t, err, "BROKER_MIGRATION_TEST_IMAGE must name an existing image built from build/docker/Dockerfile.migrate: %s", output)
		return image
	}
	image = "aib-session-migrations-integration:" + id.NewSessionID().String()
	t.Cleanup(func() { _ = exec.Command(runtime, "image", "rm", image).Run() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	architecture, err := exec.CommandContext(ctx, runtime, "image", "inspect", "postgres:15-alpine", "--format", "{{.Architecture}}").CombinedOutput()
	require.NoError(t, err, "discover container architecture: %s", architecture)
	output, err := exec.CommandContext(ctx, runtime, "build", "-f", filepath.Join(root, "build/docker/Dockerfile.migrate"),
		"--build-arg", "TARGETARCH="+strings.TrimSpace(string(architecture)), "-t", image, root).CombinedOutput()
	require.NoError(t, err, "build the real migration image once for this test: %s", output)
	return image
}

func renderedOnlineMigrationCommand(t *testing.T, root, database string, phase uint, grants bool) onlineMigrationCommand {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "helm", "template", "online-session", filepath.Join(root, "charts/agentic-identity-broker"),
		"--show-only", "templates/job-migrate.yaml", "--is-upgrade",
		"--set", "storage.type=postgres", "--set", "postgresql.external.enabled=true",
		"--set", "postgresql.external.host=127.0.0.1", "--set", "postgresql.external.sslMode=disable",
		"--set", "postgresql.external.database="+database,
		"--set", fmt.Sprintf("migration.targetVersion=%d", phase),
		"--set", fmt.Sprintf("migration.grants.enabled=%t", grants),
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token").CombinedOutput()
	require.NoError(t, err, "Helm is required for the real rendered-command proof: %s", output)
	var job struct {
		Spec struct {
			Template struct {
				Spec struct {
					Containers     []onlineMigrationCommand `yaml:"containers"`
					InitContainers []onlineMigrationCommand `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	require.NoError(t, yaml.Unmarshal(output, &job))
	containers := job.Spec.Template.Spec.Containers
	if grants {
		containers = job.Spec.Template.Spec.InitContainers
	}
	for _, container := range containers {
		if container.Name != "migrate" {
			continue
		}
		require.Equal(t, []string{"/bin/sh", "-ec"}, container.Command)
		require.Len(t, container.Args, 6)
		require.Equal(t, []string{"session-migration-stage", "-path", "/app/migrations", "-database"}, container.Args[1:5])
		// Kubernetes normally expands the Secret-backed $(DB_*) placeholders.
		// Replace only that URL; execute the rendered shell and CLI arguments unchanged.
		container.Args[5] = "postgres://testuser:testpass@127.0.0.1:5432/" + database + "?sslmode=disable"
		return container
	}
	t.Fatal("rendered Job omitted its migration container")
	return onlineMigrationCommand{}
}

// Run requires serialized migration invocations, as does the deployment wrapper.
func (cli *onlineMigrationCLI) Run(phase uint) error {
	command, ok := cli.commands[phase]
	if !ok {
		return fmt.Errorf("no rendered migration command for phase %d", phase)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	name := "session-migration-" + id.NewSessionID().String()
	args := []string{"run", "--rm", "--name", name, "--network", "container:" + cli.containerID,
		"--entrypoint", command.Command[0], cli.image}
	args = append(args, command.Command[1:]...)
	args = append(args, command.Args...)
	output, err := exec.CommandContext(ctx, cli.runtime, args...).CombinedOutput()
	if ctx.Err() != nil {
		_ = exec.Command(cli.runtime, "rm", "-f", name).Run()
	}
	if err != nil {
		return fmt.Errorf("real rendered migration stage %d: %w\n%s", phase, err, output)
	}
	return nil
}

func TestRenderedSessionMigrationSafetyGuards(t *testing.T) {
	f := NewMigrationTestFramework(t)
	defer f.Cleanup(t)
	require.NoError(t, f.Up(t, 46))
	seedSessionExpiryMigration(t, f)
	cli := newOnlineMigrationCLI(t, f)
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	for _, grants := range []bool{false, true} {
		cli.commands[46] = renderedOnlineMigrationCommand(t, root, f.dbName, 46, grants)
		for _, testCase := range []struct {
			name, setup, restore, wantError string
		}{
			{"dirty", "UPDATE schema_migrations SET dirty = TRUE", "UPDATE schema_migrations SET dirty = FALSE", "Dirty migration metadata"},
			{"unreadable", "ALTER TABLE schema_migrations RENAME COLUMN version TO unreadable_version", "ALTER TABLE schema_migrations RENAME COLUMN unreadable_version TO version", "version"},
			{"newer", "UPDATE schema_migrations SET version = 47", "UPDATE schema_migrations SET version = 46", ""},
		} {
			t.Run(fmt.Sprintf("%s_grants_%t", testCase.name, grants), func(t *testing.T) {
				require.NoError(t, f.ExecuteSQL(t, testCase.setup))
				t.Cleanup(func() { require.NoError(t, f.ExecuteSQL(t, testCase.restore)) })
				rows := sessionValiditySnapshot(t, f, "")
				schema := sessionValidityBoundarySnapshot(t, f)
				metadata, err := f.QuerySQL(t, `SELECT to_jsonb(m) FROM schema_migrations m`)
				require.NoError(t, err)
				err = cli.Run(46)
				if testCase.wantError == "" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, testCase.wantError)
				}
				require.JSONEq(t, rows, sessionValiditySnapshot(t, f, ""))
				require.JSONEq(t, schema, sessionValidityBoundarySnapshot(t, f))
				after, err := f.QuerySQL(t, `SELECT to_jsonb(m) FROM schema_migrations m`)
				require.NoError(t, err)
				require.JSONEq(t, metadata, after, "the rendered runner must not repair metadata or downgrade")
			})
		}
	}
}

func (cli *onlineMigrationCLI) runtimeConnection(t *testing.T, f *MigrationTestFramework) (string, func()) {
	t.Helper()
	role := "session_runtime_" + strings.ReplaceAll(id.NewSessionID().String(), "-", "")
	require.NoError(t, f.ExecuteSQL(t, "CREATE ROLE "+role+" LOGIN PASSWORD 'runtime-test-password' NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT"))
	cleanup := func() {
		require.NoError(t, f.ExecuteSQL(t, "DROP OWNED BY "+role+"; DROP ROLE "+role))
	}
	root, err := bootstrap.FindProjectRoot()
	require.NoError(t, err)
	output, err := exec.Command("helm", "template", "online-session", filepath.Join(root, "charts/agentic-identity-broker"),
		"--show-only", "templates/configmap-grants.yaml", "--set", "storage.type=postgres",
		"--set", "postgresql.external.enabled=true", "--set", "postgresql.external.host=127.0.0.1").CombinedOutput()
	require.NoError(t, err, "%s", output)
	var grants struct {
		Data map[string]string `yaml:"data"`
	}
	require.NoError(t, yaml.Unmarshal(output, &grants))
	require.NotEmpty(t, grants.Data["grants.sql"])
	command := exec.Command(cli.runtime, "exec", "-i", cli.containerID, "psql", "-U", "testuser", "-d", f.dbName,
		"-v", "ON_ERROR_STOP=1", "-1", "-v", "broker_user="+role, "-f", "-")
	command.Stdin = strings.NewReader(grants.Data["grants.sql"])
	output, err = command.CombinedOutput()
	require.NoError(t, err, "apply the chart's configured grants: %s", output)
	connection, err := url.Parse(f.connStr)
	require.NoError(t, err)
	connection.User = url.UserPassword(role, "runtime-test-password")
	return connection.String(), cleanup
}
