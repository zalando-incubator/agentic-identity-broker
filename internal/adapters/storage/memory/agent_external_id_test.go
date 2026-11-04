package memory

import (
	"context"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func externalAgent(value *id.ExternalID) *storage.Agent {
	return &storage.Agent{ExternalID: value, DisplayName: "Agent", Description: "Definition", PermissionSets: testPermissionSets()}
}

func TestExternalIDAtomicLifecycle(t *testing.T) {
	ctx := context.Background()
	r := NewAgentRepository()
	key, moved := id.ExternalID("Registry"), id.ExternalID("Moved")
	a := externalAgent(&key)
	require.NoError(t, r.Create(ctx, a))
	duplicate := externalAgent(&key)
	require.ErrorIs(t, r.Create(ctx, duplicate), ports.ErrExternalIDConflict)
	assert.Equal(t, map[id.ExternalID]id.AgentID{key: a.ID}, r.byExternalID)
	b := externalAgent(&moved)
	require.NoError(t, r.Create(ctx, b))
	before := b.Copy()
	b.ExternalID = &key
	b.ClientURIs = []string{"https://new.example/client.json"}
	require.ErrorIs(t, r.Update(ctx, b), ports.ErrExternalIDConflict)
	assert.Equal(t, map[id.ExternalID]id.AgentID{key: a.ID, moved: b.ID}, r.byExternalID)
	assert.Empty(t, r.byClientURI)
	actual, err := r.Get(ctx, b.ID)
	require.NoError(t, err)
	assert.Equal(t, before, actual)
	require.NoError(t, r.Update(ctx, a))
	a.ExternalID = nil
	require.NoError(t, r.Update(ctx, a))
	found, err := r.externalIDMatches(ctx, key)
	require.NoError(t, err)
	require.Empty(t, found)
	require.NoError(t, r.Create(ctx, duplicate))
	_, err = r.Delete(ctx, duplicate.ID)
	require.NoError(t, err)
	replacement := externalAgent(&key)
	require.NoError(t, r.Create(ctx, replacement))
	for _, value := range []*id.ExternalID{nil, nil, new(id.ExternalID("registry")), new(id.ExternalID(" Registry"))} {
		require.NoError(t, r.Create(ctx, externalAgent(value)))
	}
	found, err = r.externalIDMatches(ctx, key)
	require.NoError(t, err)
	require.Len(t, found, 1)
	*found[0].ExternalID = "changed"
	found, err = r.externalIDMatches(ctx, key)
	require.NoError(t, err)
	require.Equal(t, key, *found[0].ExternalID)
	assert.Equal(t, replacement.ID, r.byExternalID[key])
}

func TestExternalIDConcurrentCreate(t *testing.T) {
	r := NewAgentRepository()
	ctx := context.Background()
	key := id.ExternalID("shared")
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for range 10 {
		wg.Go(func() { results <- r.Create(ctx, externalAgent(&key)) })
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ports.ErrExternalIDConflict)
		}
	}
	require.Equal(t, 1, successes)
	found, err := r.externalIDMatches(ctx, key)
	require.NoError(t, err)
	require.Len(t, found, 1)
}
