//go:build integration
// +build integration

package postgres

import (
	"context"
	"database/sql"
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
	if sharedTestErr != nil && os.Getenv("CI") != "" {
		fmt.Fprintf(os.Stderr, "PostgreSQL integration container unavailable in CI: %v\n", sharedTestErr)
		os.Exit(1)
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

// applyMigrationsUpToDatabase applies migrations through the requested version.
func applyMigrationsUpToDatabase(t *testing.T, _ testcontainers.Container, dbName string, upTo int) {
	t.Helper()

	projectRoot, err := findProjectRoot()
	require.NoError(t, err)
	m, err := migrate.New("file://"+filepath.Join(projectRoot, "migrations"), buildConnString(dbName))
	require.NoError(t, err)
	defer func() { _, _ = m.Close() }()
	require.NoError(t, m.Migrate(uint(upTo)))
}

func TestBoundedMigrationFixture_ReachesMigration031(t *testing.T) {
	connString, cleanup := setupDatabaseFromTemplate(t, "bounded_migration_031", func(t *testing.T, dbName string) {
		applyMigrationsUpToDatabase(t, requireSharedTestContainer(t), dbName, 31)
	})
	defer cleanup()

	db, err := sql.Open("pgx", connString)
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()

	var exists bool
	err = db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'thirdparty_oauth2_services' AND column_name = 'token_endpoint_auth_method'
	)`).Scan(&exists)
	require.NoError(t, err)
	require.True(t, exists, "bounded fixture must apply migration 031")
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
