package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestNewAdapterRequiresLogger(t *testing.T) {
	adapter, err := NewAdapter(&ports.StorageConfig{Backend: "memory"}, nil)

	require.EqualError(t, err, "storage.NewAdapter: logger must not be nil")
	assert.Nil(t, adapter)
}
