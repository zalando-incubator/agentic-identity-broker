package storage_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/storage/contract"
)

func TestServiceAccountRepositoryContract_Memory(t *testing.T) {
	contract.RunServiceAccountRepository(t, func(t *testing.T) *storageadapter.Adapter {
		st, err := storageadapter.NewAdapter(&ports.StorageConfig{Backend: "memory"}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
		t.Cleanup(func() { _ = st.Close(context.Background()) })
		return st
	})
}
