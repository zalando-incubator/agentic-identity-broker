//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresAgentExternalIDInvariant(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()
	repo := NewAgentRepository(adapter)
	ctx := context.Background()
	makeAgent := func(value *id.ExternalID) *storage.Agent {
		a := &storage.Agent{ExternalID: value, DisplayName: "Research", Description: "Definition", CreatedAt: time.Now().UTC().Truncate(time.Microsecond), UpdatedAt: time.Now().UTC().Truncate(time.Microsecond)}
		attachTestPermissionSet(t, adapter, a)
		return a
	}
	a := makeAgent(ptr.To(id.ExternalID("registry")))
	require.NoError(t, repo.Create(ctx, a))
	duplicate := makeAgent(a.ExternalID)
	err := repo.Create(ctx, duplicate)
	require.ErrorIs(t, err, ports.ErrExternalIDConflict)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	b := makeAgent(ptr.To(id.ExternalID("other")))
	b.ClientURIs = []string{"https://agent.example/original.json"}
	require.NoError(t, repo.Create(ctx, b))
	before, err := repo.Get(ctx, b.ID)
	require.NoError(t, err)
	b.ExternalID = a.ExternalID
	b.ClientURIs = []string{"https://agent.example/replacement.json"}
	require.ErrorIs(t, repo.Update(ctx, b), ports.ErrExternalIDConflict)
	actual, err := repo.Get(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, before, actual)
	require.NoError(t, repo.Update(ctx, a))
	for _, value := range []*id.ExternalID{nil, nil, ptr.To(id.ExternalID("Registry")), ptr.To(id.ExternalID(" registry"))} {
		require.NoError(t, repo.Create(ctx, makeAgent(value)))
	}
	_, err = adapter.db.ExecContext(ctx, `UPDATE agents SET external_id='' WHERE id=$1`, a.ID)
	require.Error(t, err)
	mapped := mapAgentExternalIDError("UpdateAgent", err)
	require.ErrorAs(t, mapped, &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}
