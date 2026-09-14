package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

func testPermissionSets() []storage.AgentPermissionSetEntry {
	return []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeOptional,
	}}
}

func TestAgentRepository_Create(t *testing.T) {
	ctx := context.Background()

	t.Run("success with generated ID", func(t *testing.T) {
		repo := NewAgentRepository()
		agent := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		assert.False(t, agent.ID.IsZero()) // ID should be generated

		// Verify agent was stored
		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, agent.ClientID, retrieved.ClientID)
	})

	t.Run("success with provided ID", func(t *testing.T) {
		repo := NewAgentRepository()
		customID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01")
		agent := &storage.Agent{
			ID:             customID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		assert.Equal(t, customID, agent.ID)
	})

	t.Run("duplicate ID conflict", func(t *testing.T) {
		repo := NewAgentRepository()
		dupID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02")
		agent1 := &storage.Agent{
			ID:             dupID,
			ClientID:       ptr.To(id.ClientID("client-1")),
			DisplayName:    "Agent 1",
			Description:    "First agent",
			PermissionSets: testPermissionSets(),
		}
		agent2 := &storage.Agent{
			ID:             dupID,
			ClientID:       ptr.To(id.ClientID("client-2")),
			DisplayName:    "Agent 2",
			Description:    "Second agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent1)
		require.NoError(t, err)

		err = repo.Create(ctx, agent2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("duplicate client_id allowed (Feature 021: multiple agents share one upstream client_id)", func(t *testing.T) {
		repo := NewAgentRepository()
		agent1 := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("shared-upstream-client")),
			DisplayName:    "Agent 1",
			Description:    "First agent",
			PermissionSets: testPermissionSets(),
		}
		agent2 := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("shared-upstream-client")),
			DisplayName:    "Agent 2",
			Description:    "Second agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent1)
		require.NoError(t, err)

		// Feature 021: duplicate client_id must NOT return an error
		err = repo.Create(ctx, agent2)
		require.NoError(t, err, "multiple agents may share the same upstream client_id")
	})

	t.Run("validation failure", func(t *testing.T) {
		repo := NewAgentRepository()
		agent := &storage.Agent{
			ClientID: ptr.To(id.ClientID("test-client")),
			// Missing required DisplayName
			Description: "A test agent",
		}

		err := repo.Create(ctx, agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})
}

func TestAgentRepository_Get(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, testAgentID)
		require.NoError(t, err)
		assert.Equal(t, testAgentID, retrieved.ID)
		assert.Equal(t, ptr.To(id.ClientID("test-client")), retrieved.ClientID)
	})

	t.Run("not found", func(t *testing.T) {
		repo := NewAgentRepository()

		retrieved, err := repo.Get(ctx, id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"))
		require.Error(t, err)
		assert.Nil(t, retrieved)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("returns copy prevents external mutation", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Original Name",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		// Get and modify
		retrieved, err := repo.Get(ctx, testAgentID)
		require.NoError(t, err)
		retrieved.DisplayName = "Modified Name"

		// Original should be unchanged
		original, err := repo.Get(ctx, testAgentID)
		require.NoError(t, err)
		assert.Equal(t, "Original Name", original.DisplayName)
	})
}

func TestAgentRepository_Update(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Original Name",
			Description:    "Original description",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		// Update
		agent.DisplayName = "Updated Name"
		agent.Description = "Updated description"
		err = repo.Update(ctx, agent)
		require.NoError(t, err)

		// Verify update
		updated, err := repo.Get(ctx, testAgentID)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", updated.DisplayName)
		assert.Equal(t, "Updated description", updated.Description)
	})

	t.Run("not found", func(t *testing.T) {
		repo := NewAgentRepository()
		agent := &storage.Agent{
			ID:             id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"),
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Update(ctx, agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("client_id shared on update (multi-agent mode)", func(t *testing.T) {
		// Feature 021: the repo no longer enforces client_id uniqueness on update.
		// Uniqueness is the app handler's responsibility (checkClientIDUniqueness).
		// Multiple agents may share the same upstream client_id.
		repo := NewAgentRepository()

		agent1 := &storage.Agent{
			ID:             id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a21"),
			ClientID:       ptr.To(id.ClientID("client-1")),
			DisplayName:    "Agent 1",
			Description:    "First agent",
			PermissionSets: testPermissionSets(),
		}
		agent2 := &storage.Agent{
			ID:             id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a22"),
			ClientID:       ptr.To(id.ClientID("client-2")),
			DisplayName:    "Agent 2",
			Description:    "Second agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent1)
		require.NoError(t, err)
		err = repo.Create(ctx, agent2)
		require.NoError(t, err)

		// Update agent2 to share agent1's client_id — allowed in multi-agent mode
		agent2.ClientID = ptr.To(id.ClientID("client-1"))
		err = repo.Update(ctx, agent2)
		require.NoError(t, err)

		// Both agents should be present in the index for "client-1"
		found, err := repo.GetByClientID(ctx, id.ClientID("client-1"))
		require.NoError(t, err)
		assert.NotNil(t, found)
	})

	t.Run("validation failure", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		// Invalid update (empty description)
		agent.Description = ""
		err = repo.Update(ctx, agent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validation failed")
	})
}

func TestAgentRepository_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		err = repo.Delete(ctx, testAgentID)
		require.NoError(t, err)

		// Verify deletion
		_, err = repo.Get(ctx, testAgentID)
		require.Error(t, err)
	})

	t.Run("idempotent - nonexistent agent", func(t *testing.T) {
		repo := NewAgentRepository()

		err := repo.Delete(ctx, id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"))
		require.NoError(t, err) // Should not error
	})

	t.Run("cleans up client_id index", func(t *testing.T) {
		repo := NewAgentRepository()
		testAgentID := id.MustParseAgentID("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a10")
		agent := &storage.Agent{
			ID:             testAgentID,
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Test Agent",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		err = repo.Delete(ctx, testAgentID)
		require.NoError(t, err)

		// Should be able to reuse client_id
		newAgent := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "New Agent",
			Description:    "A new agent",
			PermissionSets: testPermissionSets(),
		}
		err = repo.Create(ctx, newAgent)
		require.NoError(t, err)
	})
}

func TestAgentRepository_MultipleAgentsShareClientID(t *testing.T) {
	ctx := context.Background()

	// Regression test for the byClientID 1:1 index bug (Feature 021):
	// When two agents share a ClientID and one is updated to use a new ClientID,
	// delete(byClientID, oldClientID) removes the *entire* index entry, making the
	// other agent invisible via GetByClientID even though its ClientID is unchanged.
	t.Run("GetByClientID returns remaining agent after sibling client_id is changed", func(t *testing.T) {
		repo := NewAgentRepository()

		alpha := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("shared-client")),
			DisplayName:    "Alpha Agent",
			Description:    "First agent sharing a client_id",
			PermissionSets: testPermissionSets(),
		}
		beta := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("shared-client")),
			DisplayName:    "Beta Agent",
			Description:    "Second agent sharing a client_id",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, alpha)
		require.NoError(t, err)

		// Feature 021: duplicate client_id is allowed
		err = repo.Create(ctx, beta)
		require.NoError(t, err, "multiple agents may share the same upstream client_id")

		// Update alpha to a distinct client_id — this triggers the bug:
		// delete(byClientID["shared-client"]) removes the ENTIRE entry,
		// making beta invisible to GetByClientID even though beta is unchanged.
		alpha.ClientID = ptr.To(id.ClientID("other-client"))
		err = repo.Update(ctx, alpha)
		require.NoError(t, err)

		// Beta must still be reachable by its original ClientID.
		retrieved, err := repo.GetByClientID(ctx, id.ClientID("shared-client"))
		require.NoError(t, err, "beta should still be findable by shared-client after alpha's client_id was changed")
		assert.Equal(t, beta.ID, retrieved.ID, "GetByClientID(shared-client) should return beta, not an error")
	})
}

func TestAgentRepository_List(t *testing.T) {
	ctx := context.Background()

	t.Run("returns all agents", func(t *testing.T) {
		repo := NewAgentRepository()

		agent1 := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("client-1")),
			DisplayName:    "Agent 1",
			Description:    "First agent",
			PermissionSets: testPermissionSets(),
		}
		agent2 := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("client-2")),
			DisplayName:    "Agent 2",
			Description:    "Second agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent1)
		require.NoError(t, err)
		err = repo.Create(ctx, agent2)
		require.NoError(t, err)

		agents, err := repo.List(ctx)
		require.NoError(t, err)
		assert.Len(t, agents, 2)
	})

	t.Run("returns empty slice when no agents", func(t *testing.T) {
		repo := NewAgentRepository()

		agents, err := repo.List(ctx)
		require.NoError(t, err)
		assert.Empty(t, agents)
		assert.NotNil(t, agents) // Should be empty slice, not nil
	})

	t.Run("returns copies prevent external mutation", func(t *testing.T) {
		repo := NewAgentRepository()
		agent := &storage.Agent{
			ClientID:       ptr.To(id.ClientID("test-client")),
			DisplayName:    "Original Name",
			Description:    "A test agent",
			PermissionSets: testPermissionSets(),
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		agents, err := repo.List(ctx)
		require.NoError(t, err)
		require.Len(t, agents, 1)

		// Modify returned agent
		agents[0].DisplayName = "Modified Name"

		// Original should be unchanged
		original, err := repo.Get(ctx, agents[0].ID)
		require.NoError(t, err)
		assert.Equal(t, "Original Name", original.DisplayName)
	})
}

// T026b: Storage-layer mutual exclusivity — agent cannot have both ClientID and ClientURIs set.
func TestAgentRepository_Create_MutualExclusivity(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentRepository()
	clientID := id.ClientID("upstream-client-id")

	agent := &storage.Agent{
		ClientID:    &clientID,
		ClientURIs:  []string{"https://agent.example.com/.well-known/openid-configuration"},
		DisplayName: "Ambiguous Agent",
		Description: "Both ClientID and ClientURIs set — must be rejected",
	}

	err := repo.Create(ctx, agent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent validation failed")
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}

func TestAgentRepository_Update_MutualExclusivity(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentRepository()
	clientID := id.ClientID("upstream-client-id")

	// Create a valid proxy agent first
	agent := &storage.Agent{
		ClientID:       &clientID,
		DisplayName:    "Proxy Agent",
		Description:    "Valid proxy agent",
		PermissionSets: testPermissionSets(),
	}
	require.NoError(t, repo.Create(ctx, agent))

	// Update to add ClientURIs alongside existing ClientID — must be rejected
	agent.ClientURIs = []string{"https://agent.example.com/.well-known/openid-configuration"}
	err := repo.Update(ctx, agent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent validation failed")
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
}

func TestAgentRepository_GetByClientURIPattern(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentRepository()
	newAgent := func(name, uri string) *storage.Agent {
		return &storage.Agent{DisplayName: name, Description: "CIMD pattern lookup test", ClientURIs: []string{uri}, PermissionSets: testPermissionSets()}
	}

	t.Run("resolves a concrete URL through a pattern", func(t *testing.T) {
		agent := newAgent("Pattern Agent", "https://chatgpt.com/oauth/codex/*/client.json")
		require.NoError(t, repo.Create(ctx, agent))

		resolved, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/codex/dIwd44EtAHp-/client.json")
		require.NoError(t, err)
		assert.Equal(t, agent.ID, resolved.ID)
	})

	t.Run("prefers an exact registration", func(t *testing.T) {
		pattern := newAgent("Matching Pattern Agent", "https://chatgpt.com/oauth/*/literal/client.json")
		exact := newAgent("Exact Agent", "https://chatgpt.com/oauth/codex/literal/client.json")
		require.NoError(t, repo.Create(ctx, pattern))
		require.NoError(t, repo.Create(ctx, exact))

		resolved, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/codex/literal/client.json")
		require.NoError(t, err)
		assert.Equal(t, exact.ID, resolved.ID)
	})

	t.Run("rejects patterns that resolve to different agents", func(t *testing.T) {
		first := newAgent("First Ambiguous Agent", "https://chatgpt.com/oauth/*/foo/client.json")
		second := newAgent("Second Ambiguous Agent", "https://chatgpt.com/oauth/test/*/client.json")
		require.NoError(t, repo.Create(ctx, first))
		require.NoError(t, repo.Create(ctx, second))

		_, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/test/foo/client.json")
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})
}
