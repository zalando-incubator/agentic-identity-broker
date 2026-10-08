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
		before := grant.Copy()

		err := grantRepo.Create(ctx, grant)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		assert.Equal(t, before, grant, "failed create must not publish metadata")

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
		attempt.CreatedAt = attempt.CreatedAt.Add(-time.Hour)
		attempt.UpdatedAt = attempt.UpdatedAt.Truncate(time.Second).Add(123456789 * time.Nanosecond)
		beforeAttempt := attempt.Copy()

		err = grantRepo.Update(ctx, attempt)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		require.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		assert.Equal(t, beforeAttempt, attempt, "rolled-back update must not publish returned metadata")

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

func installGrantCommitFailure(t *testing.T, adapter *Adapter) {
	t.Helper()
	ctx := context.Background()
	_, err := adapter.db.ExecContext(ctx, `
		CREATE FUNCTION reject_grant_commit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'injected grant commit failure' USING ERRCODE = '23514';
		END;
		$$;
		CREATE CONSTRAINT TRIGGER reject_grant_commit
		AFTER INSERT OR UPDATE OR DELETE ON user_grants
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_grant_commit();
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adapter.db.ExecContext(ctx, `
			DROP TRIGGER reject_grant_commit ON user_grants;
			DROP FUNCTION reject_grant_commit();
		`)
		require.NoError(t, err)
	})
}

func TestUserGrantRepositoryCommitFailureDoesNotPublishMetadata(t *testing.T) {
	adapter, agentRepo, grantRepo, cleanup := setupUserGrantTestDB(t)
	defer cleanup()
	ctx := context.Background()
	precise := time.Now().UTC().Truncate(time.Second).Add(123456789 * time.Nanosecond)

	t.Run("create", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "commit-create")
		grant := newUserGrant(id.Principal("commit-create@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		grant.ID = id.NewGrantID()
		grant.CreatedAt = precise
		grant.UpdatedAt = precise
		grant.ValidUntil = ptr.To(precise.Add(24 * time.Hour))
		before := grant.Copy()
		installGrantCommitFailure(t, adapter)
		require.Error(t, grantRepo.Create(ctx, grant))
		assert.Equal(t, before, grant)
		_, err := grantRepo.Get(ctx, grant.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	})

	t.Run("generated ID remains unpublished on commit failure", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "commit-generated-id")
		grant := newUserGrant(id.Principal("commit-generated-id@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		before := grant.Copy()
		installGrantCommitFailure(t, adapter)
		require.Error(t, grantRepo.Create(ctx, grant))
		assert.Equal(t, before, grant)
		_, err := grantRepo.FindByPrincipalAndAgent(ctx, grant.Principal, agent.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	})

	t.Run("upsert", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "commit-upsert")
		grant := newUserGrant(id.Principal("commit-upsert@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		require.NoError(t, grantRepo.Create(ctx, grant))
		stored, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		attempt := stored.Copy()
		attempt.ID = id.NewGrantID()
		attempt.CreatedAt = precise.Add(time.Hour)
		attempt.UpdatedAt = precise.Add(2 * time.Hour)
		attempt.ValidUntil = ptr.To(precise.Add(24 * time.Hour))
		before := attempt.Copy()
		installGrantCommitFailure(t, adapter)
		require.Error(t, grantRepo.Create(ctx, attempt))
		assert.Equal(t, before, attempt)
		after, err := grantRepo.Get(ctx, stored.ID)
		require.NoError(t, err)
		assert.Equal(t, stored, after)
	})

	t.Run("update", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "commit-update")
		grant := newUserGrant(id.Principal("commit-update@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		require.NoError(t, grantRepo.Create(ctx, grant))
		stored, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		attempt := stored.Copy()
		attempt.CreatedAt = precise.Add(-time.Hour)
		attempt.UpdatedAt = precise.Add(time.Hour)
		attempt.ValidUntil = ptr.To(precise.Add(24 * time.Hour))
		before := attempt.Copy()
		installGrantCommitFailure(t, adapter)
		require.Error(t, grantRepo.Update(ctx, attempt))
		assert.Equal(t, before, attempt)
		after, err := grantRepo.Get(ctx, stored.ID)
		require.NoError(t, err)
		assert.Equal(t, stored, after)
	})

	t.Run("snapshot delete", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agentRepo, "commit-delete")
		grant := newUserGrant(id.Principal("commit-delete@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		grant.ValidUntil = ptr.To(precise.Add(24 * time.Hour))
		require.NoError(t, grantRepo.Create(ctx, grant))
		stored, err := grantRepo.Get(ctx, grant.ID)
		require.NoError(t, err)
		installGrantCommitFailure(t, adapter)
		deleted, err := grantRepo.DeleteByPrincipalAndAgentID(ctx, grant.Principal, agent.ID)
		require.Error(t, err)
		assert.Nil(t, deleted)
		after, err := grantRepo.Get(ctx, stored.ID)
		require.NoError(t, err)
		assert.Equal(t, stored, after)
	})
}

func TestUserGrantRepositorySnapshotDeletionFailures(t *testing.T) {
	adapter, agents, grants, cleanup := setupUserGrantTestDB(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("invalid stored grant JSON rolls back deletion", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agents, "delete-invalid-json")
		grant := newUserGrant(id.Principal("delete-invalid-json@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		require.NoError(t, grants.Create(ctx, grant))
		_, err := adapter.db.ExecContext(ctx, `UPDATE user_grants SET granted_permission_sets = '{}'::jsonb WHERE id = $1`, grant.ID)
		require.NoError(t, err)
		deleted, err := grants.DeleteByPrincipalAndAgentID(ctx, grant.Principal, agent.ID)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindUnknown, storageErr.Kind)
		assert.Nil(t, deleted)
		var storedJSON string
		require.NoError(t, adapter.db.QueryRowContext(ctx,
			`SELECT granted_permission_sets::text FROM user_grants WHERE id = $1`, grant.ID,
		).Scan(&storedJSON))
		assert.JSONEq(t, `{}`, storedJSON)
	})

	t.Run("canceled deletion returns no snapshot and retains grant", func(t *testing.T) {
		agent := createUserGrantTestAgent(t, agents, "delete-canceled")
		grant := newUserGrant(id.Principal("delete-canceled@example.com"), agent.ID, newGrantedPermissionSetEntry(t, adapter))
		require.NoError(t, grants.Create(ctx, grant))
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		deleted, err := grants.DeleteByPrincipalAndAgentID(canceled, grant.Principal, agent.ID)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, deleted)
		stored, err := grants.Get(ctx, grant.ID)
		require.NoError(t, err)
		assert.Equal(t, grant, stored)
	})
}
