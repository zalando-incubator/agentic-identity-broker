package e2e_test

import (
	"context"
	"log/slog"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	. "github.com/onsi/gomega"
)

func externalIDPostgresStorage(fixture *bootstrap.PostgresFixture) *storageadapter.Adapter {
	cfg := ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: fixture.ConnectionURL}, Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}}
	st, err := storageadapter.NewAdapter(&cfg)
	Expect(err).To(Succeed())
	return st
}

func externalIDAdminServer(st *storageadapter.Adapter) *bootstrap.TestServer {
	ctx := context.Background()
	Expect(fixtures.SeedPlaceholderGrantData(ctx, st)).To(Succeed())
	permission, err := st.PermissionSets().Get(ctx, fixtures.PlaceholderPermissionSetID)
	Expect(err).To(Succeed())
	canonical := "github-read"
	permission.CanonicalID = &canonical
	Expect(st.PermissionSets().Update(ctx, permission)).To(Succeed())
	logger := bootstrap.TestLogger(slog.LevelWarn)
	application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(st)
	Expect(err).To(Succeed())
	server, err := bootstrap.NewAdminTestServer(application, logger)
	Expect(err).To(Succeed())
	return server
}

func withExternalIDPostgresBrokers(action func(*bootstrap.TestServer, *bootstrap.TestServer, *storageadapter.Adapter)) {
	ctx := context.Background()
	fixture, err := bootstrap.NewPostgresFixture(ctx)
	Expect(err).To(Succeed())
	defer func() { Expect(fixture.Close(ctx)).To(Succeed()) }()
	firstStore, secondStore := externalIDPostgresStorage(fixture), externalIDPostgresStorage(fixture)
	defer func() { Expect(firstStore.Close(ctx)).To(Succeed()); Expect(secondStore.Close(ctx)).To(Succeed()) }()
	first := externalIDAdminServer(firstStore)
	defer first.Close()
	logger := bootstrap.TestLogger(slog.LevelWarn)
	application, err := bootstrap.NewServerFactory(fixtures.LocalConfig(), logger).BuildApp(secondStore)
	Expect(err).To(Succeed())
	second, err := bootstrap.NewAdminTestServer(application, logger)
	Expect(err).To(Succeed())
	defer second.Close()
	action(first, second, firstStore)
}

func withExternalIDBackend(backend string, action func(*bootstrap.TestServer, *storageadapter.Adapter)) {
	ctx := context.Background()
	var st *storageadapter.Adapter
	if backend == "postgres" {
		fixture, err := bootstrap.NewPostgresFixture(ctx)
		Expect(err).To(Succeed())
		defer func() { Expect(fixture.Close(ctx)).To(Succeed()) }()
		st = externalIDPostgresStorage(fixture)
	} else {
		var err error
		st, err = bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelWarn)).NewTestStorage()
		Expect(err).To(Succeed())
	}
	defer func() { Expect(st.Close(ctx)).To(Succeed()) }()
	server := externalIDAdminServer(st)
	defer server.Close()
	action(server, st)
}
