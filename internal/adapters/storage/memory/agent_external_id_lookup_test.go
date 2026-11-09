package memory

import (
	"context"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestGetByExternalIDContract(t *testing.T) {
	ctx := context.Background()
	r := NewAgentRepository(NewTransactionManager())
	missing, err := r.GetByExternalID(ctx, "registry")
	require.Nil(t, missing)
	require.ErrorIs(t, err, ports.ErrNotFound)
	key := id.ExternalID("registry")
	a := externalAgent(&key)
	require.NoError(t, r.Create(ctx, a))
	found, err := r.GetByExternalID(ctx, key)
	require.NoError(t, err)
	require.Equal(t, a, found)
	*found.ExternalID = "other"
	found, err = r.GetByExternalID(ctx, key)
	require.NoError(t, err)
	require.Equal(t, key, *found.ExternalID)
}

func TestExternalIDFormatAcrossMemoryWrites(t *testing.T) {
	for _, value := range []id.ExternalID{"bad\x00id", "\xff", id.ExternalID(strings.Repeat("界", 256))} {
		t.Run(string(value), func(t *testing.T) {
			ctx := context.Background()
			r := NewAgentRepository(NewTransactionManager())
			invalid := externalAgent(&value)
			require.Error(t, r.Create(ctx, invalid))
			valid := externalAgent(nil)
			require.NoError(t, r.Create(ctx, valid))
			candidate := valid.Copy()
			candidate.ExternalID = &value
			require.Error(t, r.Update(ctx, candidate))
			stored, err := r.Get(ctx, valid.ID)
			require.NoError(t, err)
			require.Equal(t, valid, stored)
		})
	}
}
