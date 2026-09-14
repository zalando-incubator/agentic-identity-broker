//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestAgentRepository_Get_EmitsSpan verifies that AgentRepository.Get emits an OTel span
// named "storage.get.agent" with db.system=postgresql attribute (T030).
// TDD Red Phase: this test must fail before T033 implements the child span.
func TestAgentRepository_Get_EmitsSpan(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	// Set up an in-memory SpanRecorder as the global TracerProvider
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
		otel.SetTracerProvider(prevTP)
	})

	repo := NewAgentRepository(adapter)
	ctx := context.Background()

	// Create an agent so Get can find it
	now := time.Now().UTC()
	agent := &storage.Agent{
		ClientID:    ptr.To(id.ClientID("span-test-client")),
		DisplayName: "Span Test Agent",
		Description: "Agent for span testing",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	attachTestPermissionSet(t, adapter, agent)
	err := repo.Create(ctx, agent)
	require.NoError(t, err)

	// Call Get — should emit a span named "storage.get.agent"
	_, err = repo.Get(ctx, agent.ID)
	require.NoError(t, err)

	// Flush spans
	require.NoError(t, tp.ForceFlush(ctx))

	// Assert span was emitted
	ended := recorder.Ended()
	require.NotEmpty(t, ended, "expected at least one span to be recorded")

	var foundSpan bool
	for _, span := range ended {
		if span.Name() == "storage.get.agent" {
			foundSpan = true
			// Verify db.system=postgresql attribute
			foundAttr := false
			for _, attr := range span.Attributes() {
				if string(attr.Key) == "db.system" && attr.Value.AsString() == "postgresql" {
					foundAttr = true
					break
				}
			}
			assert.True(t, foundAttr, "expected span to have attribute db.system=postgresql")
			break
		}
	}
	assert.True(t, foundSpan, "expected a span named 'storage.get.agent' to be recorded")
}

// setupAgentTestDB creates an isolated migrated test database backed by the shared PostgreSQL container.
func setupAgentTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func TestAgentRepository_BasicCRUD(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)
	ctx := context.Background()
	createAgent := func(t *testing.T, agent *storage.Agent) error {
		t.Helper()
		attachTestPermissionSet(t, adapter, agent)
		return repo.Create(ctx, agent)
	}

	t.Run("Create", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-create-client-1")),
				DisplayName: "Test Agent",
				Description: "Test agent description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			err := createAgent(t, agent)
			require.NoError(t, err)
			assert.NotEmpty(t, agent.ID, "ID should be generated")

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, agent.ClientID, retrieved.ClientID)
			assert.Equal(t, agent.DisplayName, retrieved.DisplayName)
			assert.Equal(t, agent.Description, retrieved.Description)
		})

		t.Run("with optional fields", func(t *testing.T) {
			now := time.Now().UTC()
			externalID := id.ExternalID("ext-123")
			govURL := "https://example.com/governance"
			docURL := "https://example.com/docs"
			agentURL := "https://example.com/agent"

			agent := &storage.Agent{
				ClientID:             ptr.To(id.ClientID("basic-create-client-2")),
				ExternalID:           &externalID,
				DisplayName:          "Test Agent 2",
				Description:          "Test agent with optional fields",
				GovernanceURL:        &govURL,
				UserDocumentationURL: &docURL,
				AgentInterfaceURL:    &agentURL,
				CreatedAt:            now,
				UpdatedAt:            now,
			}

			err := createAgent(t, agent)
			require.NoError(t, err)

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, id.ExternalID(externalID), *retrieved.ExternalID)
			assert.Equal(t, govURL, *retrieved.GovernanceURL)
			assert.Equal(t, docURL, *retrieved.UserDocumentationURL)
			assert.Equal(t, agentURL, *retrieved.AgentInterfaceURL)
		})

		t.Run("duplicate client_id", func(t *testing.T) {
			now := time.Now().UTC()
			sharedClientID := id.ClientID("basic-duplicate-client")
			agent1 := &storage.Agent{
				ClientID:    &sharedClientID,
				DisplayName: "Agent 1",
				Description: "First agent",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent1))

			agent2 := &storage.Agent{
				ClientID:    &sharedClientID,
				DisplayName: "Agent 2",
				Description: "Second agent",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			err := createAgent(t, agent2)
			require.NoError(t, err)
			assert.NotEqual(t, agent1.ID, agent2.ID, "both agents must have distinct IDs")
		})

		t.Run("validation failure - blank client_id", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("")),
				DisplayName: "Test Agent",
				Description: "Test description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			err := repo.Create(ctx, agent)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})

		t.Run("validation failure - empty display_name", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-create-client-3")),
				DisplayName: "",
				Description: "Test description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			err := repo.Create(ctx, agent)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})

		t.Run("validation failure - invalid URL", func(t *testing.T) {
			now := time.Now().UTC()
			invalidURL := "not-a-url"
			agent := &storage.Agent{
				ClientID:      ptr.To(id.ClientID("basic-create-client-4")),
				DisplayName:   "Test Agent",
				Description:   "Test description",
				GovernanceURL: &invalidURL,
				CreatedAt:     now,
				UpdatedAt:     now,
			}

			err := repo.Create(ctx, agent)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("existing agent", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-get-client")),
				DisplayName: "Get Test Agent",
				Description: "Agent for get testing",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			require.NoError(t, createAgent(t, agent))

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, agent.ID, retrieved.ID)
			assert.Equal(t, agent.ClientID, retrieved.ClientID)
			assert.Equal(t, agent.DisplayName, retrieved.DisplayName)
		})

		t.Run("non-existent agent", func(t *testing.T) {
			retrieved, err := repo.Get(ctx, id.MustParseAgentID("00000000-0000-0000-0000-000000000001"))
			require.Error(t, err)
			assert.Nil(t, retrieved)

			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("empty ID", func(t *testing.T) {
			retrieved, err := repo.Get(ctx, id.AgentID{})
			require.Error(t, err)
			assert.Nil(t, retrieved)

			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})

		t.Run("returns copy prevents mutation", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-mutation-client")),
				DisplayName: "Mutation Test",
				Description: "Test mutation protection",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			require.NoError(t, createAgent(t, agent))

			retrieved1, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			retrieved1.DisplayName = "Modified Name"

			retrieved2, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, "Mutation Test", retrieved2.DisplayName)
			assert.NotEqual(t, retrieved1.DisplayName, retrieved2.DisplayName)
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("successful update", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-update-client")),
				DisplayName: "Original Name",
				Description: "Original description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent))

			agent.DisplayName = "Updated Name"
			agent.Description = "Updated description"
			agent.UpdatedAt = time.Now().UTC()

			err := repo.Update(ctx, agent)
			require.NoError(t, err)

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, "Updated Name", retrieved.DisplayName)
			assert.Equal(t, "Updated description", retrieved.Description)
		})

		t.Run("update with client_id change", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-update-original-client-id")),
				DisplayName: "Test Agent",
				Description: "Test description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent))

			agent.ClientID = ptr.To(id.ClientID("basic-update-new-client-id"))
			agent.UpdatedAt = time.Now().UTC()

			err := repo.Update(ctx, agent)
			require.NoError(t, err)

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, ptr.To(id.ClientID("basic-update-new-client-id")), retrieved.ClientID)
		})

		t.Run("update non-existent agent", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ID:          id.MustParseAgentID("00000000-0000-0000-0000-000000000002"),
				ClientID:    ptr.To(id.ClientID("basic-update-missing-client")),
				DisplayName: "Test Agent",
				Description: "Test description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}

			attachTestPermissionSet(t, adapter, agent)
			err := repo.Update(ctx, agent)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("update with duplicate client_id", func(t *testing.T) {
			now := time.Now().UTC()
			sharedClientID := id.ClientID("basic-update-shared-client")
			agent1 := &storage.Agent{
				ClientID:    &sharedClientID,
				DisplayName: "Agent 1",
				Description: "First agent",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent1))

			agent2 := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-update-second-client")),
				DisplayName: "Agent 2",
				Description: "Second agent",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent2))

			agent2.ClientID = &sharedClientID
			err := repo.Update(ctx, agent2)
			require.NoError(t, err)

			retrieved, err := repo.Get(ctx, agent2.ID)
			require.NoError(t, err)
			assert.Equal(t, &sharedClientID, retrieved.ClientID)
		})

		t.Run("validation failure on update", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-update-validation-client")),
				DisplayName: "Test Agent",
				Description: "Test description",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent))

			agent.DisplayName = ""
			err := repo.Update(ctx, agent)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("successful deletion", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				ClientID:    ptr.To(id.ClientID("basic-delete-client")),
				DisplayName: "Delete Test Agent",
				Description: "Agent for delete testing",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			require.NoError(t, createAgent(t, agent))

			err := repo.Delete(ctx, agent.ID)
			require.NoError(t, err)

			_, err = repo.Get(ctx, agent.ID)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("idempotent deletion", func(t *testing.T) {
			err := repo.Delete(ctx, id.MustParseAgentID("00000000-0000-0000-0000-000000000003"))
			require.NoError(t, err)
		})

		t.Run("empty ID validation", func(t *testing.T) {
			err := repo.Delete(ctx, id.AgentID{})
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		})
	})
}

func TestAgentRepository_CIMD(t *testing.T) {
	adapter, cleanup := setupAgentTestDBWithCIMD(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)
	ctx := context.Background()

	t.Run("ClientURIs", func(t *testing.T) {
		t.Run("round-trip through Create and Get", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				DisplayName: "CIMD Create/Get Agent",
				Description: "Tests ClientURIs round-trip",
				ClientURIs:  []string{"https://example.com/client1"},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent)
			require.NoError(t, repo.Create(ctx, agent))

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.ElementsMatch(t, agent.ClientURIs, retrieved.ClientURIs)
		})

		t.Run("round-trip through Create and List", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				DisplayName: "CIMD List Agent",
				Description: "Tests ClientURIs in List",
				ClientURIs:  []string{"https://example.com/list-client1"},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent)
			require.NoError(t, repo.Create(ctx, agent))

			agents, err := repo.List(ctx)
			require.NoError(t, err)
			var found *storage.Agent
			for _, a := range agents {
				if a.ID == agent.ID {
					found = a
					break
				}
			}
			require.NotNil(t, found)
			assert.Equal(t, agent.ClientURIs, found.ClientURIs)
		})

		t.Run("round-trip through Update and Get", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				DisplayName: "CIMD Update Agent",
				Description: "Tests ClientURIs update",
				ClientURIs:  []string{"https://example.com/update-original"},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent)
			require.NoError(t, repo.Create(ctx, agent))

			agent.ClientURIs = []string{"https://example.com/update-new"}
			agent.UpdatedAt = time.Now().UTC()
			require.NoError(t, repo.Update(ctx, agent))

			retrieved, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, []string{"https://example.com/update-new"}, retrieved.ClientURIs)
		})

		t.Run("GetByClientID returns empty ClientURIs for proxy agent", func(t *testing.T) {
			now := time.Now().UTC()
			clientID := id.ClientID("proxy-getclientid-client")
			agent := &storage.Agent{
				ClientID:    &clientID,
				DisplayName: "Proxy GetByClientID Agent",
				Description: "Tests GetByClientID returns empty ClientURIs for proxy agents",
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent)
			require.NoError(t, repo.Create(ctx, agent))

			retrieved, err := repo.GetByClientID(ctx, clientID)
			require.NoError(t, err)
			assert.Empty(t, retrieved.ClientURIs)
		})
	})

	t.Run("GetByClientURI", func(t *testing.T) {
		t.Run("returns agent with ClientURIs populated", func(t *testing.T) {
			now := time.Now().UTC()
			agent := &storage.Agent{
				DisplayName: "CIMD GetByClientURI Agent",
				Description: "Tests GetByClientURI",
				ClientURIs:  []string{"https://example.com/lookup-uri"},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent)
			require.NoError(t, repo.Create(ctx, agent))

			retrieved, err := repo.GetByClientURI(ctx, "https://example.com/lookup-uri")
			require.NoError(t, err)
			assert.Equal(t, agent.ID, retrieved.ID)
			assert.ElementsMatch(t, agent.ClientURIs, retrieved.ClientURIs)
		})

		t.Run("returns not-found for unregistered URI", func(t *testing.T) {
			_, err := repo.GetByClientURI(ctx, "https://example.com/not-registered")
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("duplicate URI on Create rolls back entire operation", func(t *testing.T) {
			now := time.Now().UTC()
			sharedURI := "https://example.com/conflict-uri"
			agent1 := &storage.Agent{
				DisplayName: "Conflict Agent 1",
				Description: "Agent with the URI that will conflict",
				ClientURIs:  []string{sharedURI},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent1)
			require.NoError(t, repo.Create(ctx, agent1))

			agent2 := &storage.Agent{
				DisplayName: "Conflict Agent 2",
				Description: "Agent that conflicts on URI",
				ClientURIs:  []string{sharedURI},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agent2)
			err := repo.Create(ctx, agent2)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

			_, getErr := repo.Get(ctx, agent2.ID)
			require.Error(t, getErr)
			conflictStorageErr, ok := getErr.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, conflictStorageErr.Kind)
		})

		t.Run("duplicate URI on Update rolls back entire operation", func(t *testing.T) {
			now := time.Now().UTC()
			uri1 := "https://example.com/update-conflict-uri1"
			uri2 := "https://example.com/update-conflict-uri2"
			agentA := &storage.Agent{
				DisplayName: "Update Conflict Agent A",
				Description: "Holds URI1 permanently",
				ClientURIs:  []string{uri1},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agentA)
			require.NoError(t, repo.Create(ctx, agentA))

			agentB := &storage.Agent{
				DisplayName: "Update Conflict Agent B",
				Description: "Tries to steal URI1 on update",
				ClientURIs:  []string{uri2},
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			attachTestPermissionSet(t, adapter, agentB)
			require.NoError(t, repo.Create(ctx, agentB))

			agentB.ClientURIs = []string{uri1}
			agentB.UpdatedAt = time.Now().UTC()
			err := repo.Update(ctx, agentB)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

			retrieved, getErr := repo.Get(ctx, agentB.ID)
			require.NoError(t, getErr)
			assert.Equal(t, []string{uri2}, retrieved.ClientURIs)
		})
	})
}

func TestAgentRepository_List_Empty(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)

	agents, err := repo.List(context.Background())
	require.NoError(t, err)
	assert.Empty(t, agents)
}

func TestAgentRepository_List(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)
	ctx := context.Background()

	t.Run("list multiple agents", func(t *testing.T) {
		before, err := repo.List(ctx)
		require.NoError(t, err)

		now := time.Now().UTC()

		// Create multiple agents
		agent1 := &storage.Agent{
			ClientID:    ptr.To(id.ClientID("list-client-1")),
			DisplayName: "Agent 1",
			Description: "First agent",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		attachTestPermissionSet(t, adapter, agent1)
		err = repo.Create(ctx, agent1)
		require.NoError(t, err)

		agent2 := &storage.Agent{
			ClientID:    ptr.To(id.ClientID("list-client-2")),
			DisplayName: "Agent 2",
			Description: "Second agent",
			CreatedAt:   now.Add(1 * time.Second),
			UpdatedAt:   now.Add(1 * time.Second),
		}
		attachTestPermissionSet(t, adapter, agent2)
		err = repo.Create(ctx, agent2)
		require.NoError(t, err)

		agent3 := &storage.Agent{
			ClientID:    ptr.To(id.ClientID("list-client-3")),
			DisplayName: "Agent 3",
			Description: "Third agent",
			CreatedAt:   now.Add(2 * time.Second),
			UpdatedAt:   now.Add(2 * time.Second),
		}
		attachTestPermissionSet(t, adapter, agent3)
		err = repo.Create(ctx, agent3)
		require.NoError(t, err)

		// List all agents
		agents, err := repo.List(ctx)
		require.NoError(t, err)
		assert.Len(t, agents, len(before)+3)

		// Verify order (should be DESC by created_at)
		assert.Equal(t, agent3.ID, agents[0].ID)
		assert.Equal(t, agent2.ID, agents[1].ID)
		assert.Equal(t, agent1.ID, agents[2].ID)
	})

	t.Run("returns copies prevent mutation", func(t *testing.T) {
		now := time.Now().UTC()
		agent := &storage.Agent{
			ClientID:    ptr.To(id.ClientID("list-mutation-client")),
			DisplayName: "Mutation Test",
			Description: "Test mutation protection",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		agents1, err := repo.List(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, agents1)

		// Mutate list item
		for _, a := range agents1 {
			if a.ID == agent.ID {
				a.DisplayName = "Modified Name"
			}
		}

		// List again and verify original is unchanged
		agents2, err := repo.List(ctx)
		require.NoError(t, err)
		for _, a := range agents2 {
			if a.ID == agent.ID {
				assert.Equal(t, "Mutation Test", a.DisplayName)
			}
		}
	})
}

// TestAgentRepository_PermissionSets_RoundTrip verifies that permission_sets
// are persisted and retrieved correctly through Create, Update, Get, List,
// and GetByClientID. Create/Update call verifyPermissionSetExistenceInTx, so
// permission_sets rows must be seeded before the test references their IDs.
func TestAgentRepository_PermissionSets_RoundTrip(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)
	ctx := context.Background()

	psID1 := id.NewPermissionSetID()
	psID2 := id.NewPermissionSetID()

	// Seed permission_sets rows so verifyPermissionSetExistenceInTx can find them.
	_, err := adapter.db.ExecContext(ctx, `
		INSERT INTO permission_sets (id, name, description, created_at, updated_at)
		VALUES ($1, 'PS One', 'Test permission set one', NOW(), NOW()),
		       ($2, 'PS Two', 'Test permission set two', NOW(), NOW())
	`, psID1.String(), psID2.String())
	require.NoError(t, err, "failed to seed permission_sets for round-trip test")

	now := time.Now().UTC()
	psRoundtripClientID := id.ClientID("ps-roundtrip-client")
	agent := &storage.Agent{
		ClientID:    &psRoundtripClientID,
		DisplayName: "Permission Sets Round-trip",
		Description: "Agent to verify permission_sets persistence",
		PermissionSets: []storage.AgentPermissionSetEntry{
			{PermissionSetID: psID1, RequirementType: storage.RequirementTypeMandatory},
			{PermissionSetID: psID2, RequirementType: storage.RequirementTypeOptional},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	t.Run("Create preserves permission_sets", func(t *testing.T) {
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		require.Len(t, retrieved.PermissionSets, 2)
		assert.Equal(t, psID1, retrieved.PermissionSets[0].PermissionSetID)
		assert.Equal(t, storage.RequirementTypeMandatory, retrieved.PermissionSets[0].RequirementType)
		assert.Equal(t, psID2, retrieved.PermissionSets[1].PermissionSetID)
		assert.Equal(t, storage.RequirementTypeOptional, retrieved.PermissionSets[1].RequirementType)
	})

	t.Run("List returns permission_sets", func(t *testing.T) {
		agents, err := repo.List(ctx)
		require.NoError(t, err)
		var found *storage.Agent
		for _, a := range agents {
			if a.ID == agent.ID {
				found = a
				break
			}
		}
		require.NotNil(t, found)
		require.Len(t, found.PermissionSets, 2)
		assert.Equal(t, psID1, found.PermissionSets[0].PermissionSetID)
	})

	t.Run("GetByClientID returns permission_sets", func(t *testing.T) {
		retrieved, err := repo.GetByClientID(ctx, *agent.ClientID)
		require.NoError(t, err)
		require.Len(t, retrieved.PermissionSets, 2)
		assert.Equal(t, psID1, retrieved.PermissionSets[0].PermissionSetID)
	})

	t.Run("Update replaces permission_sets", func(t *testing.T) {
		psID3 := id.NewPermissionSetID()

		_, err := adapter.db.ExecContext(ctx, `
			INSERT INTO permission_sets (id, name, description, created_at, updated_at)
			VALUES ($1, 'PS Three', 'Test permission set three', NOW(), NOW())
		`, psID3.String())
		require.NoError(t, err, "failed to seed psID3 for update subtest")

		updated := agent.Copy()
		updated.PermissionSets = []storage.AgentPermissionSetEntry{
			{PermissionSetID: psID3, RequirementType: storage.RequirementTypeMandatory},
		}
		updated.UpdatedAt = time.Now().UTC()

		err = repo.Update(ctx, updated)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		require.Len(t, retrieved.PermissionSets, 1)
		assert.Equal(t, psID3, retrieved.PermissionSets[0].PermissionSetID)

		agents, err := repo.List(ctx)
		require.NoError(t, err)
		var foundInList *storage.Agent
		for _, a := range agents {
			if a.ID == agent.ID {
				foundInList = a
				break
			}
		}
		require.NotNil(t, foundInList)
		require.Len(t, foundInList.PermissionSets, 1)
		assert.Equal(t, psID3, foundInList.PermissionSets[0].PermissionSetID)

		byClientID, err := repo.GetByClientID(ctx, *agent.ClientID)
		require.NoError(t, err)
		require.Len(t, byClientID.PermissionSets, 1)
		assert.Equal(t, psID3, byClientID.PermissionSets[0].PermissionSetID)
	})
}

func TestAgentRepository_ServiceRequirements_RoundTrip(t *testing.T) {
	adapter, cleanup := setupAgentTestDB(t)
	defer cleanup()

	repo := NewAgentRepository(adapter)
	ctx := context.Background()

	findListedAgent := func(t *testing.T, agentID id.AgentID) *storage.Agent {
		t.Helper()

		agents, err := repo.List(ctx)
		require.NoError(t, err)
		for _, agent := range agents {
			if agent.ID == agentID {
				return agent
			}
		}

		t.Fatalf("agent %s not found in list", agentID)
		return nil
	}

	t.Run("create preserves service requirements across read methods", func(t *testing.T) {
		now := time.Now().UTC()
		githubID := id.MustParseServiceID("c1234567-1111-1111-1111-111111111111")
		gitlabID := id.MustParseServiceID("c1234567-2222-2222-2222-222222222222")
		clientID := id.ClientID("sr-roundtrip-client")
		insertTestService(t, adapter, githubID)
		insertTestService(t, adapter, gitlabID)
		agent := &storage.Agent{
			ClientID:    &clientID,
			DisplayName: "Service Requirements Round-trip",
			Description: "Agent to verify service requirement persistence",
			ServiceRequirements: []storage.ServiceRequirement{
				{
					ServiceID:       githubID,
					RequirementType: storage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo", "user:email"},
				},
				{
					ServiceID:       gitlabID,
					RequirementType: storage.RequirementTypeOptional,
					RequiredScopes:  []string{"api"},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, agent.ServiceRequirements, retrieved.ServiceRequirements)

		listed := findListedAgent(t, agent.ID)
		assert.Equal(t, agent.ServiceRequirements, listed.ServiceRequirements)

		byClientID, err := repo.GetByClientID(ctx, *agent.ClientID)
		require.NoError(t, err)
		assert.Equal(t, agent.ServiceRequirements, byClientID.ServiceRequirements)
	})

	t.Run("update replaces service requirements", func(t *testing.T) {
		now := time.Now().UTC()
		githubID := id.MustParseServiceID("c1234567-3333-3333-3333-333333333333")
		slackID := id.MustParseServiceID("c1234567-4444-4444-4444-444444444444")
		clientID := id.ClientID("sr-update-client")
		insertTestService(t, adapter, githubID)
		insertTestService(t, adapter, slackID)
		agent := &storage.Agent{
			ClientID:    &clientID,
			DisplayName: "Service Requirements Update",
			Description: "Agent to verify service requirement replacement",
			ServiceRequirements: []storage.ServiceRequirement{
				{
					ServiceID:       githubID,
					RequirementType: storage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo"},
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		updated := agent.Copy()
		updated.ServiceRequirements = []storage.ServiceRequirement{
			{
				ServiceID:       slackID,
				RequirementType: storage.RequirementTypeOptional,
				RequiredScopes:  []string{"users:read", "channels:read"},
			},
		}
		updated.UpdatedAt = time.Now().UTC()

		err = repo.Update(ctx, updated)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, updated.ServiceRequirements, retrieved.ServiceRequirements)
	})

	t.Run("create rejects a missing service without persisting the agent", func(t *testing.T) {
		now := time.Now().UTC()
		clientID := id.ClientID("sr-missing-service-client")
		agent := &storage.Agent{
			ID:          id.NewAgentID(),
			ClientID:    &clientID,
			DisplayName: "Missing Service Requirement",
			Description: "Must not persist when its service is missing",
			ServiceRequirements: []storage.ServiceRequirement{{
				ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read"},
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

		_, err = repo.Get(ctx, agent.ID)
		require.Error(t, err)
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	})

	t.Run("nil service requirements remain nil", func(t *testing.T) {
		now := time.Now().UTC()
		clientID := id.ClientID("sr-nil-client")
		agent := &storage.Agent{
			ClientID:    &clientID,
			DisplayName: "Service Requirements Nil",
			Description: "Agent without service requirements",
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved.ServiceRequirements)
	})

	t.Run("empty service requirements remain an empty slice", func(t *testing.T) {
		now := time.Now().UTC()
		clientID := id.ClientID("sr-empty-client")
		agent := &storage.Agent{
			ClientID:            &clientID,
			DisplayName:         "Service Requirements Empty",
			Description:         "Agent with empty service requirements",
			ServiceRequirements: []storage.ServiceRequirement{},
			CreatedAt:           now,
			UpdatedAt:           now,
		}

		attachTestPermissionSet(t, adapter, agent)
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved.ServiceRequirements)
		assert.Len(t, retrieved.ServiceRequirements, 0)
	})
}

func TestAgentRepository_GetByClientURIPattern(t *testing.T) {
	adapter, cleanup := setupAgentTestDBWithCIMD(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewAgentRepository(adapter)
	newAgent := func(name, uri string) *storage.Agent {
		now := time.Now().UTC()
		return &storage.Agent{
			DisplayName: name,
			Description: "CIMD pattern lookup test",
			ClientURIs:  []string{uri},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
	}

	t.Run("resolves a concrete URL through a pattern", func(t *testing.T) {
		agent := newAgent("Pattern Agent", "https://chatgpt.com/oauth/codex/*/client.json")
		attachTestPermissionSet(t, adapter, agent)
		require.NoError(t, repo.Create(ctx, agent))

		resolved, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/codex/dIwd44EtAHp-/client.json")
		require.NoError(t, err)
		assert.Equal(t, agent.ID, resolved.ID)
	})

	t.Run("prefers an exact registration", func(t *testing.T) {
		pattern := newAgent("Matching Pattern Agent", "https://chatgpt.com/oauth/*/literal/client.json")
		exact := newAgent("Exact Agent", "https://chatgpt.com/oauth/codex/literal/client.json")
		attachTestPermissionSet(t, adapter, pattern)
		require.NoError(t, repo.Create(ctx, pattern))
		attachTestPermissionSet(t, adapter, exact)
		require.NoError(t, repo.Create(ctx, exact))

		resolved, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/codex/literal/client.json")
		require.NoError(t, err)
		assert.Equal(t, exact.ID, resolved.ID)
	})

	t.Run("rejects patterns that resolve to different agents", func(t *testing.T) {
		first := newAgent("First Ambiguous Agent", "https://chatgpt.com/oauth/*/foo/client.json")
		second := newAgent("Second Ambiguous Agent", "https://chatgpt.com/oauth/test/*/client.json")
		attachTestPermissionSet(t, adapter, first)
		require.NoError(t, repo.Create(ctx, first))
		attachTestPermissionSet(t, adapter, second)
		require.NoError(t, repo.Create(ctx, second))

		_, err := repo.GetByClientURI(ctx, "https://chatgpt.com/oauth/test/foo/client.json")
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})
}
