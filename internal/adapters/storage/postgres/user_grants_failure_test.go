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
)

func TestUserGrantRepositoryFailuresAndExpiry(t *testing.T) {
	adapter, agentRepo, grantRepo, cleanup := setupUserGrantTestDB(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("missing permission set does not create a grant", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "create-missing-ps")
		principal := id.Principal("create-missing-ps-" + id.NewAgentID().String() + "@example.com")
		grant := newUserGrant(principal, agent.ID,
			newGrantedPermissionSetEntry(t, adapter),
			storage.GrantedPermissionSetEntry{
				PermissionSetID:    id.NewPermissionSetID(),
				IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
			},
		)
		grant.ID = id.NewGrantID()

		err := grantRepo.Create(ctx, grant)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

		var count int
		require.NoError(t, adapter.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM user_grants WHERE id = $1 OR (principal = $2 AND agent_id = $3)`,
			grant.ID, principal, agent.ID,
		).Scan(&count))
		require.Zero(t, count)
	})

	t.Run("missing agent fails foreign key without creating a grant", func(t *testing.T) {
		agentID := id.NewAgentID()
		principal := id.Principal("create-missing-agent-" + agentID.String() + "@example.com")
		grant := newUserGrant(principal, agentID, newGrantedPermissionSetEntry(t, adapter))
		grant.ID = id.NewGrantID()

		err := grantRepo.Create(ctx, grant)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

		var count int
		require.NoError(t, adapter.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM user_grants WHERE id = $1 OR (principal = $2 AND agent_id = $3)`,
			grant.ID, principal, agentID,
		).Scan(&count))
		require.Zero(t, count)
	})

	t.Run("missing permission set rolls back update", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "update-missing-ps")
		principal := id.Principal("update-missing-ps-" + id.NewAgentID().String() + "@example.com")
		grant := newUserGrant(principal, agent.ID, newGrantedPermissionSetEntry(t, adapter))
		grant.ValidUntil = ptr.To(time.Now().UTC().Add(24 * time.Hour))
		require.NoError(t, grantRepo.Create(ctx, grant))

		before, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		attempt := before.Copy()
		attempt.ValidUntil = ptr.To(time.Now().UTC().Add(48 * time.Hour))
		attempt.UpdatedAt = attempt.UpdatedAt.Add(time.Minute)
		attempt.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{
			newGrantedPermissionSetEntry(t, adapter),
			{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}},
		}

		err = grantRepo.Update(ctx, attempt)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

		after, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		assert.Equal(t, before.ID, after.ID)
		assert.Equal(t, before.ValidUntil, after.ValidUntil)
		assert.Equal(t, before.GrantedPermissionSets, after.GrantedPermissionSets)
		assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
	})

	t.Run("ListByPrincipal filters expired grants but direct lookups retain them", func(t *testing.T) {
		principal := id.Principal("list-by-principal-" + id.NewAgentID().String() + "@example.com")
		otherPrincipal := id.Principal("other-list-principal-" + id.NewAgentID().String() + "@example.com")
		futureAgent := createUserGrantTestAgent(t, agentRepo, "list-principal-future")
		indefiniteAgent := createUserGrantTestAgent(t, agentRepo, "list-principal-indefinite")
		expiredAgent := createUserGrantTestAgent(t, agentRepo, "list-principal-expired")
		otherAgent := createUserGrantTestAgent(t, agentRepo, "list-principal-other")

		future := newUserGrant(principal, futureAgent.ID, newGrantedPermissionSetEntry(t, adapter))
		future.ValidUntil = ptr.To(time.Now().UTC().Add(24 * time.Hour))
		indefinite := newUserGrant(principal, indefiniteAgent.ID, newGrantedPermissionSetEntry(t, adapter))
		expired := newUserGrant(principal, expiredAgent.ID, newGrantedPermissionSetEntry(t, adapter))
		other := newUserGrant(otherPrincipal, otherAgent.ID, newGrantedPermissionSetEntry(t, adapter))
		for _, grant := range []*storage.UserGrant{future, indefinite, expired, other} {
			require.NoError(t, grantRepo.Create(ctx, grant))
		}

		past := time.Now().UTC().Add(-time.Hour)
		_, err := adapter.db.ExecContext(ctx, `UPDATE user_grants SET valid_until = $1 WHERE id = $2`, past, expired.ID)
		require.NoError(t, err)

		grants, err := grantRepo.ListByPrincipal(ctx, principal)
		require.NoError(t, err)
		ids := make([]id.GrantID, 0, len(grants))
		for _, grant := range grants {
			ids = append(ids, grant.ID)
		}
		assert.ElementsMatch(t, []id.GrantID{future.ID, indefinite.ID}, ids)

		byID, err := grantRepo.Get(ctx, expired.ID)
		require.NoError(t, err)
		assert.Equal(t, expired.ID, byID.ID)
		require.NotNil(t, byID.ValidUntil)
		assert.True(t, byID.ValidUntil.Before(time.Now()))

		byPrincipalAndAgent, err := grantRepo.FindByPrincipalAndAgent(ctx, principal, expiredAgent.ID)
		require.NoError(t, err)
		assert.Equal(t, expired.ID, byPrincipalAndAgent.ID)
	})
}
