package bootstrap

import (
	"context"
	"encoding/binary"
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

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type SQLMigration struct {
	File    string
	Version int64
}

type SharedPostgres struct {
	container testcontainers.Container
	host      string
	port      string

	templateMu  sync.Mutex
	templateDBs map[string]string
	databaseSeq atomic.Uint64
}

var (
	sharedPostgresOnce sync.Once
	sharedPostgresInst *SharedPostgres
	sharedPostgresErr  error
)

func RequireSharedPostgres(t *testing.T) *SharedPostgres {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	if err := CanAccessContainerRuntime(); err != nil {
		t.Skipf("Skipping PostgreSQL integration test: %v", err)
	}

	sharedPostgresOnce.Do(func() {
		sharedPostgresInst, sharedPostgresErr = startSharedPostgres(context.Background())
	})

	if sharedPostgresErr != nil {
		t.Skipf("Skipping PostgreSQL integration test: %v", sharedPostgresErr)
	}

	return sharedPostgresInst
}

func TerminateSharedPostgres(ctx context.Context) error {
	if sharedPostgresInst == nil || sharedPostgresInst.container == nil {
		return nil
	}

	err := sharedPostgresInst.container.Terminate(ctx)
	sharedPostgresInst.container = nil
	sharedPostgresInst.templateDBs = nil
	return err
}

func CanAccessContainerRuntime() error {
	cmd := exec.Command("docker", "ps")
	if err := cmd.Run(); err == nil {
		return nil
	}

	cmd = exec.Command("podman", "ps")
	if err := cmd.Run(); err == nil {
		return nil
	}

	return fmt.Errorf("neither docker nor podman is accessible")
}

func startSharedPostgres(ctx context.Context) (*SharedPostgres, error) {
	if os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
			return nil, fmt.Errorf("set TESTCONTAINERS_RYUK_DISABLED: %w", err)
		}
	}

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
		return nil, err
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, err
	}

	return &SharedPostgres{
		container:   container,
		host:        host,
		port:        port.Port(),
		templateDBs: map[string]string{},
	}, nil
}

func (pg *SharedPostgres) SetupDatabaseFromTemplate(
	t *testing.T,
	templateKey string,
	provision func(t *testing.T, dbName string),
) (string, string, func()) {
	t.Helper()

	templateName := pg.ensureTemplateDatabase(t, templateKey, provision)
	dbName := pg.nextDatabaseName("test_" + templateKey)
	pg.createDatabase(t, dbName, templateName)

	cleanup := func() {
		pg.dropDatabase(t, dbName)
	}

	return dbName, pg.ConnectionString(dbName), cleanup
}

func (pg *SharedPostgres) ConnectionString(dbName string) string {
	return fmt.Sprintf("postgres://testuser:testpass@%s:%s/%s?sslmode=disable", pg.host, pg.port, dbName)
}

func (pg *SharedPostgres) ApplyMigrationsUpTo(t *testing.T, dbName string, migrations []SQLMigration, upTo int) {
	t.Helper()

	pg.CreateSchemaMigrationsTable(t, dbName)
	for _, migration := range migrations {
		if int(migration.Version) > upTo {
			break
		}
		pg.ApplyMigration(t, dbName, migration.File)
		pg.recordMigrationVersion(t, dbName, migration.Version)
	}
}

func (pg *SharedPostgres) CreateSchemaMigrationsTable(t *testing.T, dbName string) {
	t.Helper()
	pg.ExecuteSQL(t, dbName, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT FALSE
		);
	`)
}

func (pg *SharedPostgres) ApplyMigration(t *testing.T, dbName, filename string) {
	t.Helper()

	projectRoot, err := FindProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	migrationPath := filepath.Join(projectRoot, "migrations", filename)
	data, err := os.ReadFile(migrationPath) // #nosec G304 -- test helper reads repository-owned migration filenames.
	require.NoErrorf(t, err, "Failed to read migration file %s", filename)

	pg.ExecuteSQL(t, dbName, string(data))
}

func (pg *SharedPostgres) QuerySQL(t *testing.T, dbName, query string) string {
	t.Helper()
	return strings.TrimSpace(pg.execPSQL(t, dbName, "-t", "-A", "-c", query))
}

func (pg *SharedPostgres) ExecuteSQL(t *testing.T, dbName, query string) {
	t.Helper()
	_ = pg.execPSQL(t, dbName, "-c", query)
}

func (pg *SharedPostgres) execPSQL(t *testing.T, dbName string, args ...string) string {
	t.Helper()

	baseArgs := []string{"psql", "-U", "testuser", "-d", dbName}
	baseArgs = append(baseArgs, args...)
	return pg.execContainerCommand(t, baseArgs...)
}

func (pg *SharedPostgres) execContainerCommand(t *testing.T, args ...string) string {
	t.Helper()

	exitCode, outReader, err := pg.container.Exec(context.Background(), args)
	var output []byte
	if outReader != nil {
		readOutput, readErr := io.ReadAll(outReader)
		require.NoError(t, readErr)
		output = readOutput
	}
	decoded := decodeExecOutput(output)
	require.NoErrorf(t, err, "container exec failed: %s", strings.Join(args, " "))
	require.Equalf(t, 0, exitCode, "container exec failed: %s\noutput: %s", strings.Join(args, " "), string(decoded))
	return string(decoded)
}

func decodeExecOutput(raw []byte) []byte {
	if len(raw) < 8 {
		return raw
	}

	decoded := make([]byte, 0, len(raw))
	for offset := 0; offset < len(raw); {
		if offset+8 > len(raw) {
			return raw
		}
		if raw[offset] > 2 || raw[offset+1] != 0 || raw[offset+2] != 0 || raw[offset+3] != 0 {
			return raw
		}

		frameLen := int(binary.BigEndian.Uint32(raw[offset+4 : offset+8]))
		offset += 8
		if frameLen < 0 || offset+frameLen > len(raw) {
			return raw
		}

		decoded = append(decoded, raw[offset:offset+frameLen]...)
		offset += frameLen
	}

	if len(decoded) == 0 {
		return raw
	}

	return decoded
}

func (pg *SharedPostgres) ensureTemplateDatabase(t *testing.T, templateKey string, provision func(t *testing.T, dbName string)) string {
	t.Helper()

	pg.templateMu.Lock()
	defer pg.templateMu.Unlock()

	if templateName, ok := pg.templateDBs[templateKey]; ok {
		return templateName
	}

	templateName := pg.nextDatabaseName("template_" + templateKey)
	pg.createDatabase(t, templateName, "")
	provision(t, templateName)
	pg.templateDBs[templateKey] = templateName
	return templateName
}

func (pg *SharedPostgres) createDatabase(t *testing.T, dbName, templateName string) {
	t.Helper()

	args := []string{"createdb", "-U", "testuser", "--maintenance-db=postgres"}
	if templateName != "" {
		args = append(args, "-T", templateName)
	}
	args = append(args, dbName)
	pg.execContainerCommand(t, args...)
}

func (pg *SharedPostgres) dropDatabase(t *testing.T, dbName string) {
	t.Helper()
	pg.terminateDatabaseConnections(t, dbName)
	pg.execContainerCommand(t, "dropdb", "--if-exists", "-U", "testuser", "--maintenance-db=postgres", dbName)
}

func (pg *SharedPostgres) terminateDatabaseConnections(t *testing.T, dbName string) {
	t.Helper()
	pg.execPSQL(t, "postgres", "-c", fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		dbName,
	))
}

func (pg *SharedPostgres) recordMigrationVersion(t *testing.T, dbName string, version int64) {
	t.Helper()
	pg.ExecuteSQL(t, dbName, fmt.Sprintf(
		"INSERT INTO schema_migrations (version, dirty) VALUES (%d, FALSE) ON CONFLICT DO NOTHING;",
		version,
	))
}

func (pg *SharedPostgres) nextDatabaseName(prefix string) string {
	sanitized := sanitizeDatabaseName(prefix)
	seq := pg.databaseSeq.Add(1)
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

func FindProjectRoot() (string, error) {
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
