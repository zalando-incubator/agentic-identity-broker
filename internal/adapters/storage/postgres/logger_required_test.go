package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestNewAdapterRequiresLogger(t *testing.T) {
	adapter, err := NewAdapter(&ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: "postgresql://user:pass@localhost:5432/testdb"},
	}, nil)

	require.Error(t, err)
	assert.Nil(t, adapter)
	var storErr *storage.StorageError
	require.ErrorAs(t, err, &storErr)
	assert.Equal(t, storage.ErrorKindValidation, storErr.Kind)
	assert.Contains(t, err.Error(), "logger must not be nil")
}
