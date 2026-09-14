//go:build integration
// +build integration

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testGrantNotFound = id.MustParseGrantID("99000000-0000-0000-0000-000000000001")
	testAgentNotFound = id.MustParseAgentID("99000000-0000-0000-0000-000000000001")
)

// setupUserGrantTestDB creates a test database with migrations applied.
func setupUserGrantTestDB(t *testing.T) (*Adapter, *AgentRepository, *UserGrantRepository, func()) {
	t.Helper()

	adapter, cleanup := setupAgentTestDB(t)
	return adapter, NewAgentRepository(adapter), NewUserGrantRepository(adapter), cleanup
}

func createUserGrantTestAgent(t *testing.T, agentRepo *AgentRepository, label string) *storage.Agent {
	t.Helper()

	now := time.Now().UTC()
	agent := &storage.Agent{
		ClientID:    ptr.To(id.ClientID("grant-agent-" + label + "-" + id.NewAgentID().String()[:8])),
		DisplayName: "Grant Test Agent " + label,
		Description: "Test description",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	attachTestPermissionSet(t, agentRepo.adapter, agent)
	require.NoError(t, agentRepo.Create(context.Background(), agent))
	return agent
}

func newGrantedPermissionSetEntry(t *testing.T, adapter *Adapter, serviceIDs ...id.ServiceID) storage.GrantedPermissionSetEntry {
	t.Helper()

	psID := id.NewPermissionSetID()
	seedPermissionSet(t, adapter, psID)
	if len(serviceIDs) == 0 {
		serviceIDs = []id.ServiceID{id.NewServiceID()}
	}
	return storage.GrantedPermissionSetEntry{
		PermissionSetID:    psID,
		IncludedServiceIDs: append([]id.ServiceID(nil), serviceIDs...),
	}
}

func newUserGrant(principal id.Principal, agentID id.AgentID, entries ...storage.GrantedPermissionSetEntry) *storage.UserGrant {
	now := time.Now().UTC()
	return &storage.UserGrant{
		Principal:             principal,
		AgentID:               agentID,
		GrantedPermissionSets: append([]storage.GrantedPermissionSetEntry(nil), entries...),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
}

func TestUserGrantRepository(t *testing.T) {
	adapter, agentRepo, grantRepo, cleanup := setupUserGrantTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		t.Run("successful creation", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "create-success")
			grant := newUserGrant(id.Principal("user@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
			validUntil := time.Now().Add(24 * time.Hour)
			grant.ValidUntil = &validUntil

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)
			assert.NotEmpty(t, grant.ID)

			retrieved, err := grantRepo.Get(ctx, grant.ID)
			require.NoError(t, err)
			assert.Equal(t, grant.Principal, retrieved.Principal)
			assert.Equal(t, grant.AgentID, retrieved.AgentID)
			assert.Len(t, retrieved.GrantedPermissionSets, 1)
		})

		t.Run("upsert semantics", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "create-upsert")
			principal := id.Principal("user2@example.com")
			validUntil1 := time.Now().Add(24 * time.Hour)
			grant1 := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))
			grant1.ValidUntil = &validUntil1

			err := grantRepo.Create(ctx, grant1)
			require.NoError(t, err)
			firstID := grant1.ID

			validUntil2 := time.Now().Add(48 * time.Hour)
			grant2 := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))
			grant2.ValidUntil = &validUntil2

			err = grantRepo.Create(ctx, grant2)
			require.NoError(t, err)

			grants, err := grantRepo.ListByPrincipalAndAgent(ctx, principal, agent.ID)
			require.NoError(t, err)
			assert.Len(t, grants, 1)
			assert.Len(t, grants[0].GrantedPermissionSets, 1)
			assert.Equal(t, grant2.GrantedPermissionSets[0].PermissionSetID, grants[0].GrantedPermissionSets[0].PermissionSetID)
			assert.Equal(t, firstID, grants[0].ID)
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("get non-existent grant", func(t *testing.T) {
			_, err := grantRepo.Get(ctx, testGrantNotFound)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("get existing grant", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "get-existing")
			grant := newUserGrant(id.Principal("user-get@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			retrieved, err := grantRepo.Get(ctx, grant.ID)
			require.NoError(t, err)
			assert.Equal(t, grant.ID, retrieved.ID)
			assert.Equal(t, grant.Principal, retrieved.Principal)
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("update existing grant", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "update-existing")
			grant := newUserGrant(id.Principal("user-update@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			validUntil := time.Now().Add(48 * time.Hour)
			grant.ValidUntil = &validUntil
			grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{newGrantedPermissionSetEntry(t, adapter)}
			grant.UpdatedAt = time.Now().UTC()

			err = grantRepo.Update(ctx, grant)
			require.NoError(t, err)

			retrieved, err := grantRepo.Get(ctx, grant.ID)
			require.NoError(t, err)
			assert.NotNil(t, retrieved.ValidUntil)
			assert.Len(t, retrieved.GrantedPermissionSets, 1)
			assert.Equal(t, grant.GrantedPermissionSets[0].PermissionSetID, retrieved.GrantedPermissionSets[0].PermissionSetID)
		})

		t.Run("update non-existent grant returns NotFound before PS conflict", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "update-missing")
			grant := newUserGrant(id.Principal("user-update-missing@example.com"), agent.ID, storage.GrantedPermissionSetEntry{
				PermissionSetID:    id.NewPermissionSetID(),
				IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
			})
			grant.ID = testGrantNotFound

			err := grantRepo.Update(ctx, grant)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("delete existing grant", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-existing")
			grant := newUserGrant(id.Principal("user-delete@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			err = grantRepo.Delete(ctx, grant.ID)
			require.NoError(t, err)

			_, err = grantRepo.Get(ctx, grant.ID)
			require.Error(t, err)
		})

		t.Run("delete non-existent grant is idempotent", func(t *testing.T) {
			err := grantRepo.Delete(ctx, testGrantNotFound)
			require.NoError(t, err)
		})
	})

	t.Run("ListByPrincipalAndAgent", func(t *testing.T) {
		t.Run("list when no grants exist", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "list-empty")
			principal := id.Principal("list-empty@example.com")

			grants, err := grantRepo.ListByPrincipalAndAgent(ctx, principal, agent.ID)
			require.NoError(t, err)
			assert.Empty(t, grants)
		})

		t.Run("list existing grants", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "list-existing")
			principal := id.Principal("list-existing@example.com")
			grant := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			grants, err := grantRepo.ListByPrincipalAndAgent(ctx, principal, agent.ID)
			require.NoError(t, err)
			assert.Len(t, grants, 1)
			assert.Equal(t, grant.ID, grants[0].ID)
		})

		t.Run("includes expired grants", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "list-expired")
			principal := id.Principal("user-expired@example.com")
			pastTime := time.Now().Add(-time.Hour).UTC()
			expiredGrantID := id.NewGrantID()
			expiredPSID := id.NewPermissionSetID()
			grantedPS := fmt.Sprintf(`[{"permission_set_id":"%s","included_service_ids":[]}]`, expiredPSID.String())

			_, err := adapter.db.ExecContext(ctx,
				`INSERT INTO user_grants (id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
				expiredGrantID.String(), string(principal), agent.ID.String(),
				pastTime, grantedPS, pastTime, pastTime,
			)
			require.NoError(t, err)

			grants, err := grantRepo.ListByPrincipalAndAgent(ctx, principal, agent.ID)
			require.NoError(t, err)
			assert.Len(t, grants, 1)
		})
	})

	t.Run("FindByPrincipalAndAgent", func(t *testing.T) {
		t.Run("find when no grant exists", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "find-empty")
			principal := id.Principal("find-empty@example.com")

			_, err := grantRepo.FindByPrincipalAndAgent(ctx, principal, agent.ID)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("find existing grant", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "find-existing")
			principal := id.Principal("find-existing@example.com")
			grant := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			found, err := grantRepo.FindByPrincipalAndAgent(ctx, principal, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, grant.ID, found.ID)
		})
	})

	t.Run("DeleteByAgent", func(t *testing.T) {
		t.Run("cascade delete all grants for agent", func(t *testing.T) {
			agent1 := createUserGrantTestAgent(t, agentRepo, "delete-agent-a")
			agent2 := createUserGrantTestAgent(t, agentRepo, "delete-agent-b")

			grant1 := newUserGrant(id.Principal("delete-agent-user1@example.com"), agent1.ID, newGrantedPermissionSetEntry(t, adapter))
			grant2 := newUserGrant(id.Principal("delete-agent-user2@example.com"), agent1.ID, newGrantedPermissionSetEntry(t, adapter))
			grant3 := newUserGrant(id.Principal("delete-agent-user3@example.com"), agent2.ID, newGrantedPermissionSetEntry(t, adapter))

			require.NoError(t, grantRepo.Create(ctx, grant1))
			require.NoError(t, grantRepo.Create(ctx, grant2))
			require.NoError(t, grantRepo.Create(ctx, grant3))

			err := grantRepo.DeleteByAgent(ctx, agent1.ID)
			require.NoError(t, err)

			_, err = grantRepo.Get(ctx, grant1.ID)
			require.Error(t, err)
			_, err = grantRepo.Get(ctx, grant2.ID)
			require.Error(t, err)
			_, err = grantRepo.Get(ctx, grant3.ID)
			require.NoError(t, err)
		})

		t.Run("delete by non-existent agent is idempotent", func(t *testing.T) {
			err := grantRepo.DeleteByAgent(ctx, testAgentNotFound)
			require.NoError(t, err)
		})
	})

	t.Run("PermissionSetIDMarshaling", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "marshal")
		grant := newUserGrant(
			id.Principal("marshal-user@example.com"),
			agent.ID,
			newGrantedPermissionSetEntry(t, adapter),
			newGrantedPermissionSetEntry(t, adapter),
		)

		err := grantRepo.Create(ctx, grant)
		require.NoError(t, err)

		retrieved, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved.GrantedPermissionSets, 2)
	})

	t.Run("DeepCopy", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "deepcopy")
		grant := newUserGrant(id.Principal("deepcopy-user@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))

		err := grantRepo.Create(ctx, grant)
		require.NoError(t, err)

		retrieved, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		retrieved.GrantedPermissionSets = append(retrieved.GrantedPermissionSets, storage.GrantedPermissionSetEntry{
			PermissionSetID:    id.NewPermissionSetID(),
			IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
		})

		retrieved2, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved2.GrantedPermissionSets, 1)
	})

	t.Run("DeleteByPrincipalAndAgentID", func(t *testing.T) {
		t.Run("success deletes grant and cleans up indexes", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-principal-success")
			principal := id.Principal("revoke-user@example.com")
			grant := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			err = grantRepo.DeleteByPrincipalAndAgentID(ctx, principal, agent.ID)
			require.NoError(t, err)

			_, err = grantRepo.Get(ctx, grant.ID)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

			_, err = grantRepo.FindByPrincipalAndAgent(ctx, principal, agent.ID)
			require.Error(t, err)
			storageErr, ok = err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("not found returns NotFound error", func(t *testing.T) {
			err := grantRepo.DeleteByPrincipalAndAgentID(ctx, id.Principal("no-grant@example.com"), testAgentNotFound)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
		})

		t.Run("CountGrantsReferencingPermissionSet returns correct count", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-principal-count")
			psID := id.NewPermissionSetID()
			otherPSID := id.NewPermissionSetID()
			seedPermissionSet(t, adapter, psID)

			count, err := grantRepo.CountGrantsReferencingPermissionSet(ctx, psID)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			grant := &storage.UserGrant{
				Principal: id.Principal("ps-count-user@example.com"),
				AgentID:   agent.ID,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
					PermissionSetID:    psID,
					IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
				}},
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			err = grantRepo.Create(ctx, grant)
			require.NoError(t, err)

			count, err = grantRepo.CountGrantsReferencingPermissionSet(ctx, psID)
			require.NoError(t, err)
			assert.Equal(t, 1, count)

			count, err = grantRepo.CountGrantsReferencingPermissionSet(ctx, otherPSID)
			require.NoError(t, err)
			assert.Equal(t, 0, count)

			err = grantRepo.Delete(ctx, grant.ID)
			require.NoError(t, err)

			count, err = grantRepo.CountGrantsReferencingPermissionSet(ctx, psID)
			require.NoError(t, err)
			assert.Equal(t, 0, count)
		})

		t.Run("expired grant is excluded from count", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-principal-expired")
			expiredPSID := id.NewPermissionSetID()
			pastTime := time.Now().Add(-time.Hour).UTC()
			expiredGrantID := id.NewGrantID()
			grantedPS := fmt.Sprintf(`[{"permission_set_id":"%s","included_service_ids":[]}]`, expiredPSID.String())

			_, execErr := adapter.db.ExecContext(ctx,
				`INSERT INTO user_grants (id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
				expiredGrantID.String(), "expired-ps-user@example.com", agent.ID.String(),
				pastTime, grantedPS, pastTime, pastTime,
			)
			require.NoError(t, execErr)

			count, err := grantRepo.CountGrantsReferencingPermissionSet(ctx, expiredPSID)
			require.NoError(t, err)
			assert.Equal(t, 0, count, "expired grant must not be counted")
		})

		t.Run("future valid_until grant is included in count", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-principal-future")
			futurePSID := id.NewPermissionSetID()
			futureTime := time.Now().Add(time.Hour).UTC()
			futureGrantID := id.NewGrantID()
			grantedPS := fmt.Sprintf(`[{"permission_set_id":"%s","included_service_ids":[]}]`, futurePSID.String())
			now := time.Now().UTC()

			_, execErr := adapter.db.ExecContext(ctx,
				`INSERT INTO user_grants (id, principal, agent_id, valid_until, granted_permission_sets, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7)`,
				futureGrantID.String(), "future-ps-user@example.com", agent.ID.String(),
				futureTime, grantedPS, now, now,
			)
			require.NoError(t, execErr)

			count, err := grantRepo.CountGrantsReferencingPermissionSet(ctx, futurePSID)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "grant with future valid_until must be counted")
		})

		t.Run("cross-principal isolation only deletes the specified principal's grant", func(t *testing.T) {
			agent := createUserGrantTestAgent(t, agentRepo, "delete-principal-isolation")
			principalA := id.Principal("isolation-user-a@example.com")
			principalB := id.Principal("isolation-user-b@example.com")
			grantA := newUserGrant(principalA, agent.ID, newGrantedPermissionSetEntry(t, adapter))
			grantB := newUserGrant(principalB, agent.ID, newGrantedPermissionSetEntry(t, adapter))

			err := grantRepo.Create(ctx, grantA)
			require.NoError(t, err)
			err = grantRepo.Create(ctx, grantB)
			require.NoError(t, err)

			err = grantRepo.DeleteByPrincipalAndAgentID(ctx, principalA, agent.ID)
			require.NoError(t, err)

			_, err = grantRepo.FindByPrincipalAndAgent(ctx, principalA, agent.ID)
			require.Error(t, err)
			storageErr, ok := err.(*storage.StorageError)
			require.True(t, ok)
			assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

			found, err := grantRepo.FindByPrincipalAndAgent(ctx, principalB, agent.ID)
			require.NoError(t, err)
			assert.Equal(t, grantB.ID, found.ID)
		})
	})
}
