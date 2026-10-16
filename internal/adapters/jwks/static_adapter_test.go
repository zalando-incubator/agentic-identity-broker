package jwks

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func parsedTestKeySet(t *testing.T) trustanchor.LocalKeySet {
	t.Helper()
	set, err := trustanchor.ParseLocalKeySet([]byte(testKeySet))
	require.NoError(t, err)
	return set
}

func TestStaticAdapterServesTheInlineSet(t *testing.T) {
	goroutines := runtime.NumGoroutine()
	adapter := NewStaticAdapter(parsedTestKeySet(t))
	assert.Equal(t, goroutines, runtime.NumGoroutine(), "an inline set starts no background work")

	set, err := adapter.GetKeySet(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, set.Len())

	key, err := adapter.GetKey(context.Background(), "k1")
	require.NoError(t, err)
	kid, _ := key.KeyID()
	assert.Equal(t, "k1", kid)

	_, err = adapter.GetKey(context.Background(), "unknown")
	assert.EqualError(t, err, "key not found in jwks (kid: unknown)")

	assert.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())
	assert.Equal(t, []string{"k1"}, adapter.LocalKeySet().KeyIDs)
}
