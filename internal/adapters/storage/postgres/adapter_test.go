package postgres

import (
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
)

func TestNewAdapter(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config, slog.New(slog.DiscardHandler))

	assert.NoError(t, err)
	assert.NotNil(t, adapter)
}

func TestNewAdapter_NilConfig(t *testing.T) {
	adapter, err := NewAdapter(nil, slog.New(slog.DiscardHandler))

	assert.Error(t, err)
	assert.Nil(t, adapter)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindValidation, storErr.Kind)
	}
}

func TestNewAdapter_WrongBackend(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config, slog.New(slog.DiscardHandler))

	assert.Error(t, err)
	assert.Nil(t, adapter)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindValidation, storErr.Kind)
	}
}

func TestNewAdapter_EmptyConnectionURL(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	adapter, err := NewAdapter(config, slog.New(slog.DiscardHandler))

	assert.Error(t, err)
	assert.Nil(t, adapter)
	if storErr, ok := err.(*storage.StorageError); ok {
		assert.Equal(t, storage.ErrorKindValidation, storErr.Kind)
	}
}

func TestNewAdapter_DefaultTimeouts(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  0,
			Write: 0,
		},
	}

	adapter, err := NewAdapter(config, slog.New(slog.DiscardHandler))

	assert.NoError(t, err)
	assert.NotNil(t, adapter)
	// Verify defaults were set
	assert.Equal(t, 5*time.Second, adapter.timeouts.Read)
	assert.Equal(t, 10*time.Second, adapter.timeouts.Write)
}

// Note: Full integration tests with real database would be in test/integration/storage/postgres_test.go
// These unit tests verify adapter creation and configuration handling
