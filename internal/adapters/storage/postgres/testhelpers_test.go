//go:build integration
// +build integration

package postgres

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var (
	sharedTestContainer testcontainers.Container
	sharedTestHost      string
	sharedTestPort      string
	sharedTestErr       error

	templateDBs   = map[string]string{}
	templateDBsMu sync.Mutex
	databaseSeq   atomic.Uint64
)

// init disables Ryuk for Podman compatibility.
func init() {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	}
}

func TestMain(m *testing.M) {
	ctx := context.Background()
	if canAccessContainerRuntime() {
		container, host, port, err := startSharedTestContainer(ctx)
		if err != nil {
			sharedTestErr = err
		} else {
			sharedTestContainer = container
			sharedTestHost = host
			sharedTestPort = port
		}
	} else {
		sharedTestErr = fmt.Errorf("no container runtime available")
	}

	code := m.Run()

	if sharedTestContainer != nil {
		_ = sharedTestContainer.Terminate(ctx)
	}

	os.Exit(code)
}

// canAccessContainerRuntime checks if Docker or Podman is available.
func canAccessContainerRuntime() bool {
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err == nil {
		return true
	}
	cmd = exec.Command("podman", "ps")
	return cmd.Run() == nil
}

func startSharedTestContainer(ctx context.Context) (testcontainers.Container, string, string, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(30 * time.Second),
	}

	genericReq := testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	}
	if os.Getenv("DOCKER_HOST") != "" {
		genericReq.ProviderType = testcontainers.ProviderPodman
	}

	container, err := testcontainers.GenericContainer(ctx, genericReq)
	if err != nil {
		return nil, "", "", err
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, "", "", err
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, "", "", err
	}

	return container, host, port.Port(), nil
}

func requireSharedTestContainer(t *testing.T) testcontainers.Container {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if sharedTestErr != nil {
		t.Skipf("Skipping test: %v", sharedTestErr)
	}
	if sharedTestContainer == nil {
		t.Skip("Skipping test: shared PostgreSQL container unavailable")
	}

	return sharedTestContainer
}

func nextDatabaseName(prefix string) string {
	sanitized := sanitizeDatabaseName(prefix)
	seq := databaseSeq.Add(1)
	name := fmt.Sprintf("%s_%d", sanitized, seq)
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func sanitizeDatabaseName(prefix string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	sanitized := re.ReplaceAllString(prefix, "_")
	sanitized = strings.Trim(sanitized, "_")
	if sanitized == "" {
		return "testdb"
	}
	return sanitized
}

func buildConnString(dbName string) string {
	return fmt.Sprintf("postgres://testuser:testpass@%s:%s/%s?sslmode=disable", sharedTestHost, sharedTestPort, dbName)
}

func execContainerCommand(t *testing.T, args ...string) string {
	t.Helper()

	container := requireSharedTestContainer(t)
	ctx := context.Background()
	exitCode, outReader, err := container.Exec(ctx, args)
	output, readErr := io.ReadAll(outReader)
	if readErr != nil {
		require.NoError(t, readErr)
	}
	require.NoError(t, err, "container exec failed: %s", strings.Join(args, " "))
	require.Equalf(t, 0, exitCode, "container exec failed: %s\noutput: %s", strings.Join(args, " "), string(output))
	return string(output)
}

func createDatabase(t *testing.T, dbName string, templateName string) {
	t.Helper()
	args := []string{"createdb", "-U", "testuser", "--maintenance-db=postgres"}
	if templateName != "" {
		args = append(args, "-T", templateName)
	}
	args = append(args, dbName)
	execContainerCommand(t, args...)
}

func dropDatabase(t *testing.T, dbName string) {
	t.Helper()
	terminateDatabaseConnections(t, dbName)
	execContainerCommand(t, "dropdb", "--if-exists", "-U", "testuser", "--maintenance-db=postgres", dbName)
}

func terminateDatabaseConnections(t *testing.T, dbName string) {
	t.Helper()
	sql := fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();", dbName)
	execContainerCommand(t, "psql", "-U", "testuser", "-d", "postgres", "-c", sql)
}

func ensureTemplateDatabase(t *testing.T, templateKey string, provision func(t *testing.T, dbName string)) string {
	t.Helper()

	templateDBsMu.Lock()
	defer templateDBsMu.Unlock()

	if name, ok := templateDBs[templateKey]; ok {
		return name
	}

	name := nextDatabaseName("template_" + templateKey)
	createDatabase(t, name, "")
	provision(t, name)
	templateDBs[templateKey] = name
	return name
}

func setupDatabaseFromTemplate(t *testing.T, templateKey string, provision func(t *testing.T, dbName string)) (string, func()) {
	t.Helper()

	requireSharedTestContainer(t)
	templateName := ensureTemplateDatabase(t, templateKey, provision)
	dbName := nextDatabaseName("test_" + templateKey)
	createDatabase(t, dbName, templateName)

	cleanup := func() {
		dropDatabase(t, dbName)
	}
	return buildConnString(dbName), cleanup
}

func setupMigratedConnString(t *testing.T) (string, func()) {
	t.Helper()
	return setupDatabaseFromTemplate(t, "full_migrations", func(t *testing.T, dbName string) {
		applyMigrationsToDatabase(t, requireSharedTestContainer(t), dbName)
	})
}

func setupMigratedAdapter(t *testing.T) (*Adapter, func()) {
	t.Helper()

	connString, cleanup := setupMigratedConnString(t)
	config := testStorageConfig(connString)
	adapter, err := NewAdapter(config)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, adapter.Initialize(ctx))

	return adapter, func() {
		require.NoError(t, adapter.Close(ctx))
		cleanup()
	}
}

// applyMigrations applies all database migrations to the test container's default database.
func applyMigrations(t *testing.T, container testcontainers.Container) {
	t.Helper()
	applyMigrationsToDatabase(t, container, "testdb")
}

func applyMigrationsToDatabase(t *testing.T, _ testcontainers.Container, dbName string) {
	t.Helper()

	projectRoot, err := findProjectRoot()
	require.NoError(t, err)
	m, err := migrate.New("file://"+filepath.Join(projectRoot, "migrations"), buildConnString(dbName))
	require.NoError(t, err)
	defer func() { _, _ = m.Close() }()
	require.NoError(t, m.Up())
}

// applyMigrationsUpTo applies migrations sequentially against the default database.
func applyMigrationsUpTo(t *testing.T, container testcontainers.Container, upTo int) {
	t.Helper()
	applyMigrationsUpToDatabase(t, container, "testdb", upTo)
}

// applyMigrationsUpToDatabase applies migrations sequentially from 001 up to and including
// the migration with the given version number to the specified database.
func applyMigrationsUpToDatabase(t *testing.T, container testcontainers.Container, dbName string, upTo int) {
	t.Helper()

	ctx := context.Background()
	createSchemaMigrationsTable(t, ctx, container, dbName)

	projectRoot, err := findProjectRoot()
	require.NoError(t, err)
	migrationsDir := filepath.Join(projectRoot, "migrations")

	migrations := []struct {
		file    string
		version int64
	}{
		{"001_create_agents.up.sql", 1},
		{"002_create_thirdparty_services.up.sql", 2},
		{"003_create_user_grants.up.sql", 3},
		{"004_create_user_sessions.up.sql", 4},
		{"005_add_agent_service_requirements.up.sql", 5},
		{"006_add_service_protected_resources.up.sql", 6},
		{"007_add_oauth2_flavor.up.sql", 7},
		{"008_drop_agent_client_id_unique.up.sql", 8},
		{"009_add_agent_redirect_uris.up.sql", 9},
		{"010_create_client_credentials.up.sql", 10},
		{"011_create_signing_keys.up.sql", 11},
		{"012_create_authorization_codes.up.sql", 12},
		{"013_add_client_id_to_auth_codes.up.sql", 13},
		{"014_create_pkce_sessions.up.sql", 14},
		{"015_add_cimd_support.up.sql", 15},
		{"016_nullable_agent_client_id.up.sql", 16},
		{"017_add_permission_sets.up.sql", 17},
		{"018_add_agent_permission_sets.up.sql", 18},
		{"019_migrate_user_grants_to_permission_sets.up.sql", 19},
		{"020_add_service_scope_requirement_type.up.sql", 20},
		{"021_enforce_single_current_signing_key.up.sql", 21},
		{"022_add_service_authorization_params.up.sql", 22},
		{"023_create_refresh_token_sessions.up.sql", 23},
		{"024_create_tool_approvals.up.sql", 24},
		{"025_create_approval_sync_state.up.sql", 25},
		{"026_sync_approval_mutations.up.sql", 26},
		{"027_add_profile_to_oauth2_codes.up.sql", 27},
		{"028_normalize_service_protected_resources.up.sql", 28},
	}

	for _, migration := range migrations {
		if int(migration.version) > upTo {
			break
		}
		applyOneMigration(t, ctx, container, dbName, migrationsDir, migration.file, migration.version)
	}
}

// createSchemaMigrationsTable creates the schema_migrations tracking table in the target database.
func createSchemaMigrationsTable(t *testing.T, ctx context.Context, container testcontainers.Container, dbName string) {
	t.Helper()
	schemaSQL := []byte(`CREATE TABLE IF NOT EXISTS schema_migrations (version BIGINT PRIMARY KEY, dirty BOOLEAN NOT NULL DEFAULT FALSE);`)
	if err := container.CopyToContainer(ctx, schemaSQL, "/tmp/schema_migrations.sql", 0644); err != nil {
		t.Logf("Warning: Failed to copy schema_migrations.sql: %v", err)
		return
	}
	exitCode, _, err := container.Exec(ctx, []string{"psql", "-U", "testuser", "-d", dbName, "-f", "/tmp/schema_migrations.sql"})
	if err != nil || exitCode != 0 {
		t.Logf("Warning: Failed to create schema_migrations table in %s (exit %d): %v", dbName, exitCode, err)
	}
}

// applyOneMigration copies a migration file into the container and runs it via psql -f.
func applyOneMigration(t *testing.T, ctx context.Context, container testcontainers.Container, dbName, migrationsDir, file string, version int64) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(migrationsDir, file))
	if err != nil {
		t.Logf("Warning: Could not read migration %s: %v", file, err)
		return
	}

	containerPath := fmt.Sprintf("/tmp/migration_%s_%03d.sql", sanitizeDatabaseName(dbName), version)
	if err := container.CopyToContainer(ctx, data, containerPath, 0644); err != nil {
		t.Logf("Warning: Could not copy migration %s to container: %v", file, err)
		return
	}

	exitCode, _, err := container.Exec(ctx, []string{"psql", "-U", "testuser", "-d", dbName, "-f", containerPath})
	if err != nil || exitCode != 0 {
		t.Logf("Warning: Migration %s failed against %s (exit %d): %v", file, dbName, exitCode, err)
		return
	}

	versionSQL := []byte(fmt.Sprintf("INSERT INTO schema_migrations (version, dirty) VALUES (%d, FALSE) ON CONFLICT DO NOTHING;", version))
	versionPath := fmt.Sprintf("/tmp/migration_%s_%03d_version.sql", sanitizeDatabaseName(dbName), version)
	if err := container.CopyToContainer(ctx, versionSQL, versionPath, 0644); err == nil {
		container.Exec(ctx, []string{"psql", "-U", "testuser", "-d", dbName, "-f", versionPath}) //nolint:errcheck
	}
}

// findProjectRoot walks up the directory tree to find the project root.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root")
		}
		dir = parent
	}
}

// setupAgentTestDBWithCIMD creates a test database with all migrations applied.
// Use for tests that exercise CIMD features (ClientURIs, GetByClientURI).
func setupAgentTestDBWithCIMD(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

// testStorageConfig creates a StorageConfig for integration testing with the given connection string.
func testStorageConfig(connString string) *ports.StorageConfig {
	return &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: connString,
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}
}

// seedPermissionSet inserts a permission_sets row so verifyPermissionSetExistenceInTx can find it.
// Must be called before creating UserGrant rows that reference the given permission set ID.
func seedPermissionSet(t *testing.T, adapter *Adapter, psID id.PermissionSetID) {
	t.Helper()
	_, err := adapter.db.ExecContext(context.Background(),
		`INSERT INTO permission_sets (id, name, description, created_at, updated_at)
		 VALUES ($1, $2, 'test permission set', NOW(), NOW())
		 ON CONFLICT DO NOTHING`,
		psID.String(), "test-ps-"+psID.String()[:8],
	)
	require.NoError(t, err, "failed to seed permission set %s", psID)
}

// attachTestPermissionSet gives an agent the persisted permission-set reference required by repository validation.
func attachTestPermissionSet(t *testing.T, adapter *Adapter, agent *storage.Agent) {
	t.Helper()
	if len(agent.PermissionSets) > 0 {
		return
	}

	permissionSetID := id.NewPermissionSetID()
	seedPermissionSet(t, adapter, permissionSetID)
	agent.PermissionSets = []storage.AgentPermissionSetEntry{{
		PermissionSetID: permissionSetID,
		RequirementType: storage.RequirementTypeOptional,
	}}
}
