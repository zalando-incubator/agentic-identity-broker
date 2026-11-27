//go:build integration

package storage_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/storage/contract"
)

func TestServiceAccountRepositoryContract_Postgres(t *testing.T) {
	pg := bootstrap.RequireSharedPostgres(t)
	contract.RunServiceAccountRepository(t, func(t *testing.T) *storageadapter.Adapter {
		_, url, cleanup := pg.SetupMigratedDatabase(t)
		config := &ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: url}, Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}}
		st, err := storageadapter.NewAdapter(config, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = st.Close(context.Background())
			cleanup()
		})
		return st
	})
}
