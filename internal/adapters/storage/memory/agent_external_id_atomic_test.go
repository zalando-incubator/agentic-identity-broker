package memory

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalIDOtherConflictsPreserveAllIndexes(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, conflict := range []string{"canonical", "client URI"} {
			t.Run(operation+"/"+conflict, func(t *testing.T) {
				ctx := context.Background()
				r := NewAgentRepository(NewTransactionManager())
				owner := externalAgent(ptr.To(id.ExternalID("owner")))
				owner.CanonicalID = ptr.To("owner")
				owner.ClientURIs = []string{"https://owner.example/client.json"}
				target := externalAgent(ptr.To(id.ExternalID("target")))
				target.CanonicalID = ptr.To("target")
				target.ClientURIs = []string{"https://target.example/client.json"}
				require.NoError(t, r.Create(ctx, owner))
				require.NoError(t, r.Create(ctx, target))
				clientOwner := externalAgent(ptr.To(id.ExternalID("client-owner")))
				clientOwner.ClientID = ptr.To(id.ClientID("original-client"))
				require.NoError(t, r.Create(ctx, clientOwner))
				external, canonical, uris := maps.Clone(r.byExternalID), maps.Clone(r.byCanonicalID), maps.Clone(r.byClientURI)
				clients := make(map[id.ClientID][]id.AgentID)
				for key, ids := range r.byClientID {
					clients[key] = slices.Clone(ids)
				}
				before, err := r.List(ctx)
				require.NoError(t, err)
				candidate := target.Copy()
				candidate.ExternalID = ptr.To(id.ExternalID("unused"))
				candidate.ClientURIs = []string{"https://unused.example/client.json"}
				candidate.CanonicalID = ptr.To("unused")
				if conflict == "canonical" {
					candidate.CanonicalID = owner.CanonicalID
					candidate.ClientURIs = nil
					candidate.ClientID = ptr.To(id.ClientID("replacement-client"))
				} else {
					candidate.ClientURIs = owner.ClientURIs
				}
				if operation == "create" {
					candidate.ID = id.NewAgentID()
					err = r.Create(ctx, candidate)
				} else {
					err = r.Update(ctx, candidate)
				}
				require.Error(t, err)
				var se *storage.StorageError
				require.ErrorAs(t, err, &se)
				assert.Equal(t, storage.ErrorKindConflict, se.Kind)
				assert.Equal(t, external, r.byExternalID)
				assert.Equal(t, canonical, r.byCanonicalID)
				assert.Equal(t, uris, r.byClientURI)
				assert.Equal(t, clients, r.byClientID)
				after, err := r.List(ctx)
				require.NoError(t, err)
				assert.ElementsMatch(t, before, after)
				_, err = r.GetByExternalID(ctx, "unused")
				require.ErrorIs(t, err, ports.ErrNotFound)
				for key, agentID := range external {
					found, err := r.GetByExternalID(ctx, key)
					require.NoError(t, err)
					assert.Equal(t, agentID, found.ID)
				}
			})
		}
	}
}

func TestExternalIDConcurrentMixedWrites(t *testing.T) {
	ctx := context.Background()
	r := NewAgentRepository(NewTransactionManager())
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			key := id.ExternalID(fmt.Sprintf("registry-%d", i))
			moved := id.ExternalID(fmt.Sprintf("moved-%d", i))
			a := externalAgent(&key)
			if !assert.NoError(t, r.Create(ctx, a)) {
				return
			}
			for range 10 {
				a.ExternalID = &moved
				if !assert.NoError(t, r.Update(ctx, a)) {
					return
				}
				found, err := r.GetByExternalID(ctx, moved)
				if !assert.NoError(t, err) {
					return
				}
				*found.ExternalID = "clone-only"
				a.ExternalID = nil
				if !assert.NoError(t, r.Update(ctx, a)) {
					return
				}
				a.ExternalID = &key
				if !assert.NoError(t, r.Update(ctx, a)) {
					return
				}
			}
			_, err := r.Delete(ctx, a.ID)
			assert.NoError(t, err)
			replacement := externalAgent(&key)
			assert.NoError(t, r.Create(ctx, replacement))
		})
	}
	wg.Wait()
	assert.Len(t, r.agents, 20)
	assert.Len(t, r.byExternalID, 20)
	for key, agentID := range r.byExternalID {
		found, err := r.GetByExternalID(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, agentID, found.ID)
	}
}
