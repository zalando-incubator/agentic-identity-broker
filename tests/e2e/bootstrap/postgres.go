package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type PostgresFixture struct {
	container     testcontainers.Container
	ConnectionURL string
}

func CanAccessContainerRuntime() error {
	for _, runtime := range []string{"docker", "podman"} {
		if err := exec.Command(runtime, "ps").Run(); err == nil { // #nosec G204 -- runtime is selected only from the fixed docker/podman list above.
			return nil
		}
	}
	return fmt.Errorf("neither docker nor podman is accessible")
}

func NewPostgresFixture(ctx context.Context) (*PostgresFixture, error) {
	if os.Getenv("DOCKER_HOST") != "" && os.Getenv("TESTCONTAINERS_RYUK_DISABLED") == "" {
		if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
			return nil, fmt.Errorf("set TESTCONTAINERS_RYUK_DISABLED: %w", err)
		}
	}

	request := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine@sha256:09e4f20b14ddb3dfe3a0c825b206032aaf8f28300ba2070c0b60fc1c10c6abc7",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(120 * time.Second),
	}
	containerRequest := testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	}
	if os.Getenv("DOCKER_HOST") != "" {
		containerRequest.ProviderType = testcontainers.ProviderPodman
	}

	container, err := testcontainers.GenericContainer(ctx, containerRequest)
	if err != nil {
		return nil, err
	}
	fixture := &PostgresFixture{container: container}
	if err := fixture.initialize(ctx); err != nil {
		_ = fixture.Close(ctx)
		return nil, err
	}
	return fixture, nil
}

func (f *PostgresFixture) Close(ctx context.Context) error {
	if f == nil || f.container == nil {
		return nil
	}
	if err := f.container.Terminate(ctx); err != nil {
		return err
	}
	f.container = nil
	return nil
}

func (f *PostgresFixture) initialize(ctx context.Context) error {
	host, err := f.container.Host(ctx)
	if err != nil {
		return err
	}
	port, err := f.container.MappedPort(ctx, "5432")
	if err != nil {
		return err
	}
	f.ConnectionURL = fmt.Sprintf("postgres://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate E2E bootstrap source")
	}
	migrationsDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "migrations")
	migrationRunner, err := migrate.New("file://"+migrationsDir, f.ConnectionURL)
	if err != nil {
		return err
	}
	defer func() { _, _ = migrationRunner.Close() }()
	if err := migrationRunner.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	return nil
}
