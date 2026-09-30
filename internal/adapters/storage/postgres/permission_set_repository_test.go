//go:build integration
// +build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestPermissionSetRepositoryTransactions(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	permissionSets := NewPermissionSetRepository(adapter)
	agents := NewAgentRepository(adapter)
	grants := NewUserGrantRepository(adapter)
	originalServiceID := id.NewServiceID()
	replacementServiceID := id.NewServiceID()
	insertTestService(t, adapter, originalServiceID)
	insertTestService(t, adapter, replacementServiceID)

	newSet := func(name string) *storage.PermissionSet {
		now := time.Now().UTC()
		return &storage.PermissionSet{
			ID:          id.NewPermissionSetID(),
			Name:        name,
			Description: "Original description",
			ServiceScopes: []storage.ServiceScope{{
				ServiceID:       originalServiceID,
				Scopes:          []string{"read"},
				RequirementType: storage.RequirementTypeMandatory,
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	t.Run("create rolls back parent and first scope when second service is missing", func(t *testing.T) {
		ps := newSet("rollback-create")
		ps.ServiceScopes = append(ps.ServiceScopes, storage.ServiceScope{
			ServiceID:       id.NewServiceID(),
			Scopes:          []string{"write"},
			RequirementType: storage.RequirementTypeOptional,
		})

		err := permissionSets.Create(ctx, ps)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		require.Equal(t, "InsertServiceScopes", storageErr.Operation)

		_, err = permissionSets.Get(ctx, ps.ID)
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

		var scopeCount int
		require.NoError(t, adapter.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM permission_set_service_scopes WHERE permission_set_id = $1`, ps.ID,
		).Scan(&scopeCount))
		require.Zero(t, scopeCount, "first valid scope must roll back with its parent")
	})

	t.Run("update rolls back changed fields and scope replacement", func(t *testing.T) {
		ps := newSet("rollback-update")
		originalCanonicalID := "rollback-update-original"
		ps.CanonicalID = &originalCanonicalID
		require.NoError(t, permissionSets.Create(ctx, ps))
		before, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)

		changedCanonicalID := "rollback-update-changed"
		ps.Name = "rollback-update-changed"
		ps.CanonicalID = &changedCanonicalID
		ps.Description = "Changed description"
		ps.UpdatedAt = time.Now().UTC().Add(time.Minute)
		ps.ServiceScopes = []storage.ServiceScope{
			{ServiceID: replacementServiceID, Scopes: []string{"write"}, RequirementType: storage.RequirementTypeOptional},
			{ServiceID: id.NewServiceID(), Scopes: []string{"missing"}, RequirementType: storage.RequirementTypeMandatory},
		}

		err = permissionSets.Update(ctx, ps)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
		require.Equal(t, "InsertServiceScopes", storageErr.Operation)

		after, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)
		require.Equal(t, before, after, "original name, canonical ID, timestamps, and scope must survive the failed replacement")
	})

	t.Run("agent reference blocks delete without changing parent or scopes", func(t *testing.T) {
		ps := newSet("agent-blocked-delete")
		require.NoError(t, permissionSets.Create(ctx, ps))
		now := time.Now().UTC()
		agent := &storage.Agent{
			ID:          id.NewAgentID(),
			DisplayName: "Permission-set delete blocker",
			Description: "References the permission set",
			PermissionSets: []storage.AgentPermissionSetEntry{{
				PermissionSetID: ps.ID,
				RequirementType: storage.RequirementTypeMandatory,
			}},
			CreatedAt: now,
			UpdatedAt: now,
		}
		require.NoError(t, agents.Create(ctx, agent))
		before, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)

		err = permissionSets.Delete(ctx, ps.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

		after, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)
		storedAgent, err := agents.Get(ctx, agent.ID)
		require.NoError(t, err)
		require.Equal(t, agent.PermissionSets, storedAgent.PermissionSets)
	})

	t.Run("active grant blocks delete without changing parent or scopes", func(t *testing.T) {
		ps := newSet("active-grant-blocked-delete")
		require.NoError(t, permissionSets.Create(ctx, ps))
		agent := createUserGrantTestAgent(t, agents, "permission-set-active")
		entry := storage.GrantedPermissionSetEntry{
			PermissionSetID:    ps.ID,
			IncludedServiceIDs: []id.ServiceID{originalServiceID},
		}
		grant := newUserGrant(id.Principal("permission-set-active@example.com"), agent.ID, entry)
		future := time.Now().UTC().Add(24 * time.Hour)
		grant.ValidUntil = &future
		require.NoError(t, grants.Create(ctx, grant))
		before, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)

		err = permissionSets.Delete(ctx, ps.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

		after, err := permissionSets.Get(ctx, ps.ID)
		require.NoError(t, err)
		require.Equal(t, before, after)
		storedGrant, err := grants.Get(ctx, grant.ID)
		require.NoError(t, err)
		require.Equal(t, []storage.GrantedPermissionSetEntry{entry}, storedGrant.GrantedPermissionSets)
	})

	t.Run("expired grant allows delete and cascades only its scopes", func(t *testing.T) {
		ps := newSet("expired-grant-deletable")
		unrelated := newSet("unrelated-after-delete")
		require.NoError(t, permissionSets.Create(ctx, ps))
		require.NoError(t, permissionSets.Create(ctx, unrelated))
		agent := createUserGrantTestAgent(t, agents, "permission-set-expired")
		entry := storage.GrantedPermissionSetEntry{
			PermissionSetID:    ps.ID,
			IncludedServiceIDs: []id.ServiceID{originalServiceID},
		}
		grant := newUserGrant(id.Principal("permission-set-expired@example.com"), agent.ID, entry)
		future := time.Now().UTC().Add(24 * time.Hour)
		grant.ValidUntil = &future
		require.NoError(t, grants.Create(ctx, grant))

		past := time.Now().UTC().Add(-time.Hour)
		result, err := adapter.db.ExecContext(ctx, `UPDATE user_grants SET valid_until = $1 WHERE id = $2`, past, grant.ID)
		require.NoError(t, err)
		updated, err := result.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 1, updated)
		expired, err := grants.Get(ctx, grant.ID)
		require.NoError(t, err)
		require.NotNil(t, expired.ValidUntil)
		require.True(t, expired.ValidUntil.Before(time.Now()))
		require.Equal(t, []storage.GrantedPermissionSetEntry{entry}, expired.GrantedPermissionSets)

		require.NoError(t, permissionSets.Delete(ctx, ps.ID))
		_, err = permissionSets.Get(ctx, ps.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

		var scopeCount int
		require.NoError(t, adapter.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM permission_set_service_scopes WHERE permission_set_id = $1`, ps.ID,
		).Scan(&scopeCount))
		require.Zero(t, scopeCount, "delete must cascade to the child scope")

		stillPresent, err := permissionSets.Get(ctx, unrelated.ID)
		require.NoError(t, err)
		require.Equal(t, unrelated.ServiceScopes, stillPresent.ServiceScopes)
		stillExpired, err := grants.Get(ctx, grant.ID)
		require.NoError(t, err)
		require.Equal(t, expired, stillExpired, "deletion must not remove the expired grant")
	})
}
