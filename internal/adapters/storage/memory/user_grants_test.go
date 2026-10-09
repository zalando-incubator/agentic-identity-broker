package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testAgentID1   = id.MustParseAgentID("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01")
	testAgentID2   = id.MustParseAgentID("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02")
	testPrincipal1 = id.Principal("user@example.com")
	testPrincipal2 = id.Principal("user1@example.com")
	testPrincipal3 = id.Principal("user2@example.com")
)

func TestUserGrantRepository_Create(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	validUntil := time.Now().Add(24 * time.Hour)
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)
	assert.False(t, grant.ID.IsZero(), "ID should be generated")

	// Verify grant was stored
	retrieved, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	assert.Equal(t, grant.Principal, retrieved.Principal)
	assert.Equal(t, grant.AgentID, retrieved.AgentID)
	assert.Equal(t, grant, retrieved)
	expected := retrieved.Copy()
	validUntil = validUntil.Add(time.Hour)
	require.NotNil(t, grant.ValidUntil)
	assert.Equal(t, *expected.ValidUntil, *grant.ValidUntil, "copyback must not retain the input expiry pointer")
	*grant.ValidUntil = grant.ValidUntil.Add(2 * time.Hour)
	retrieved, err = repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	assert.Equal(t, expected, retrieved)
}

func TestUserGrantRepository_UpsertSemantics(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	validUntil1 := time.Now().Add(24 * time.Hour)

	psID1 := id.NewPermissionSetID()
	psID2 := id.NewPermissionSetID()

	// Create first grant
	grant1 := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID1, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant1)
	require.NoError(t, err)
	firstID := grant1.ID

	// Create second grant for same principal+agent (should update, not create new)
	validUntil2 := time.Now().Add(48 * time.Hour)
	grant2 := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil2,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID2, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err = repo.Create(ctx, grant2)
	require.NoError(t, err)

	// Should reuse the same ID (upsert)
	assert.Equal(t, firstID, grant2.ID, "upsert should reuse existing ID")

	// Verify only one grant exists for this principal+agent pair
	grants, err := repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Len(t, grants, 1, "should have exactly one grant after upsert")

	// Verify the grant was updated with new permission set IDs
	assert.Len(t, grants[0].GrantedPermissionSets, 1)
	assert.Equal(t, psID2, grants[0].GrantedPermissionSets[0].PermissionSetID)
}

func TestUserGrantRepository_Create_CommittedMetadata(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, time.October, 8, 12, 0, 0, 123456789, time.UTC)
	for _, indefinite := range []bool{false, true} {
		name := "finite"
		if indefinite {
			name = "indefinite"
		}
		t.Run(name, func(t *testing.T) {
			repo := NewUserGrantRepository()
			first := &storage.UserGrant{
				ID:        id.NewGrantID(),
				Principal: testPrincipal1,
				AgentID:   testAgentID1,
				CreatedAt: createdAt,
				UpdatedAt: createdAt,
			}
			require.NoError(t, repo.Create(ctx, first))
			validUntil := createdAt.Add(24 * time.Hour)
			upsert := &storage.UserGrant{
				ID:                    id.NewGrantID(),
				Principal:             testPrincipal1,
				AgentID:               testAgentID1,
				ValidUntil:            &validUntil,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				CreatedAt:             createdAt.Add(time.Hour),
				UpdatedAt:             createdAt.Add(2 * time.Hour),
			}
			if indefinite {
				upsert.ValidUntil = nil
			}
			expected := upsert.Copy()
			expected.ID = first.ID
			expected.CreatedAt = first.CreatedAt
			require.NoError(t, repo.Create(ctx, upsert))
			assert.Equal(t, expected, upsert)
			stored, err := repo.Get(ctx, upsert.ID)
			require.NoError(t, err)
			assert.Equal(t, upsert, stored)

			validUntil = validUntil.Add(time.Hour)
			if upsert.ValidUntil != nil {
				assert.Equal(t, *expected.ValidUntil, *upsert.ValidUntil)
				*upsert.ValidUntil = upsert.ValidUntil.Add(2 * time.Hour)
			}
			upsert.GrantedPermissionSets[0].PermissionSetID = id.NewPermissionSetID()
			upsert.GrantedPermissionSets[0].IncludedServiceIDs[0] = id.NewServiceID()
			stored, err = repo.Get(ctx, expected.ID)
			require.NoError(t, err)
			assert.Equal(t, expected, stored, "upsert input must not alias persisted metadata or grants")
		})
	}
}

func TestUserGrantRepository_Create_ConcurrentCommittedMetadata(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()
	const count = 10
	createdAt := time.Date(2026, time.October, 8, 12, 0, 0, 123456789, time.UTC)
	grants := make([]*storage.UserGrant, count)
	candidateIDs := make([]id.GrantID, count)
	validUntilInputs := make([]time.Time, count)
	start := make(chan struct{})
	done := make(chan error, count)
	for i := range count {
		candidateIDs[i] = id.NewGrantID()
		validUntilInputs[i] = createdAt.Add(time.Duration(i+1) * time.Hour)
		grants[i] = &storage.UserGrant{
			ID:         candidateIDs[i],
			Principal:  testPrincipal1,
			AgentID:    testAgentID1,
			ValidUntil: &validUntilInputs[i],
			CreatedAt:  createdAt.Add(time.Duration(i) * time.Minute),
			UpdatedAt:  createdAt.Add(time.Duration(i) * time.Second),
		}
		go func(grant *storage.UserGrant) {
			<-start
			done <- repo.Create(ctx, grant)
		}(grants[i])
	}
	close(start)
	for range count {
		require.NoError(t, <-done)
	}
	stored, err := repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	winners := 0
	for i, grant := range grants {
		assert.Equal(t, stored.ID, grant.ID)
		assert.Equal(t, stored.CreatedAt, grant.CreatedAt)
		assert.Equal(t, createdAt.Add(time.Duration(i)*time.Second), grant.UpdatedAt)
		require.NotNil(t, grant.ValidUntil)
		assert.Equal(t, validUntilInputs[i], *grant.ValidUntil)
		if candidateIDs[i] == stored.ID {
			winners++
			assert.Equal(t, createdAt.Add(time.Duration(i)*time.Minute), stored.CreatedAt)
		}
		validUntilInputs[i] = validUntilInputs[i].Add(time.Hour)
		assert.Equal(t, createdAt.Add(time.Duration(i+1)*time.Hour), *grant.ValidUntil)
		*grant.ValidUntil = grant.ValidUntil.Add(2 * time.Hour)
	}
	assert.Equal(t, 1, winners)
	unchanged, err := repo.Get(ctx, stored.ID)
	require.NoError(t, err)
	assert.Equal(t, stored, unchanged)
	listed, err := repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Len(t, listed, 1)
}

func TestUserGrantRepository_Get(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Get non-existent grant
	_, err := repo.Get(ctx, id.MustParseGrantID("d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"))
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

	// Create and get grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err = repo.Create(ctx, grant)
	require.NoError(t, err)

	retrieved, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	assert.Equal(t, grant.ID, retrieved.ID)
	assert.Equal(t, grant.Principal, retrieved.Principal)
}

func TestUserGrantRepository_Update(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	psID1 := id.NewPermissionSetID()
	psID2 := id.NewPermissionSetID()

	// Create grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID1, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)

	// Update grant
	validUntil := time.Now().Add(48 * time.Hour)
	grant.ValidUntil = &validUntil
	grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: psID2, IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}}

	err = repo.Update(ctx, grant)
	require.NoError(t, err)

	// Verify update
	retrieved, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	assert.NotNil(t, retrieved.ValidUntil)
	assert.Len(t, retrieved.GrantedPermissionSets, 1)
	assert.Equal(t, psID2, retrieved.GrantedPermissionSets[0].PermissionSetID)
}

func TestUserGrantRepository_Update_CommittedMetadata(t *testing.T) {
	ctx := context.Background()
	createdAt := time.Date(2026, time.October, 8, 12, 0, 0, 123456789, time.UTC)
	for _, indefinite := range []bool{false, true} {
		name := "finite"
		if indefinite {
			name = "indefinite"
		}
		t.Run(name, func(t *testing.T) {
			repo := NewUserGrantRepository()
			initialValidUntil := createdAt.Add(48 * time.Hour)
			grant := &storage.UserGrant{
				ID:         id.NewGrantID(),
				Principal:  testPrincipal1,
				AgentID:    testAgentID1,
				ValidUntil: &initialValidUntil,
				CreatedAt:  createdAt,
				UpdatedAt:  createdAt,
			}
			require.NoError(t, repo.Create(ctx, grant))
			validUntil := createdAt.Add(24 * time.Hour)
			grant.ValidUntil = &validUntil
			if indefinite {
				grant.ValidUntil = nil
			}
			grant.CreatedAt = createdAt.Add(time.Hour)
			grant.UpdatedAt = createdAt.Add(2 * time.Hour)
			expected := grant.Copy()
			expected.CreatedAt = createdAt
			require.NoError(t, repo.Update(ctx, grant))
			assert.Equal(t, expected, grant)
			stored, err := repo.Get(ctx, grant.ID)
			require.NoError(t, err)
			assert.Equal(t, grant, stored)
			validUntil = validUntil.Add(time.Hour)
			if grant.ValidUntil != nil {
				assert.Equal(t, *expected.ValidUntil, *grant.ValidUntil)
				*grant.ValidUntil = grant.ValidUntil.Add(2 * time.Hour)
			}
			stored, err = repo.Get(ctx, grant.ID)
			require.NoError(t, err)
			assert.Equal(t, expected, stored)
		})
	}
}

func TestUserGrantRepository_Update_NotFound(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	grant := &storage.UserGrant{
		ID:                    id.MustParseGrantID("d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"),
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	expected := grant.Copy()
	err := repo.Update(ctx, grant)
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	assert.Equal(t, expected, grant, "failed update must not publish metadata")
}

func TestUserGrantRepository_Delete(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)

	// Delete grant
	err = repo.Delete(ctx, grant.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.Get(ctx, grant.ID)
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
}

func TestUserGrantRepository_Delete_Idempotent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Delete non-existent grant (should not error)
	err := repo.Delete(ctx, id.MustParseGrantID("d0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"))
	require.NoError(t, err)
}

func TestUserGrantRepository_ListByPrincipalAndAgent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// List when no grants exist
	grants, err := repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Empty(t, grants)

	// Create grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err = repo.Create(ctx, grant)
	require.NoError(t, err)

	// List grants
	grants, err = repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Len(t, grants, 1)
	assert.Equal(t, grant.ID, grants[0].ID)
}

func TestUserGrantRepository_ListByPrincipalAndAgent_IncludesExpired(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create grant with very short validity (1 millisecond in future)
	validUntil := time.Now().Add(1 * time.Millisecond)
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)

	// Wait for grant to expire
	time.Sleep(10 * time.Millisecond)

	// List should include expired grants (filtering happens in service layer)
	grants, err := repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Len(t, grants, 1, "expired grants should be included in repository results")
}

func TestUserGrantRepository_FindByPrincipalAndAgent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Find when no grant exists
	_, err := repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

	// Create grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err = repo.Create(ctx, grant)
	require.NoError(t, err)

	// Find grant
	found, err := repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Equal(t, grant.ID, found.ID)
	assert.Equal(t, testPrincipal1, found.Principal)
	assert.Equal(t, testAgentID1, found.AgentID)
}

func TestUserGrantRepository_DeleteByAgent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create multiple grants for same agent with different principals
	grant1 := &storage.UserGrant{
		Principal:             testPrincipal2,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	grant2 := &storage.UserGrant{
		Principal:             testPrincipal3,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	// Create grant for different agent
	grant3 := &storage.UserGrant{
		Principal:             testPrincipal2,
		AgentID:               testAgentID2,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant1)
	require.NoError(t, err)
	err = repo.Create(ctx, grant2)
	require.NoError(t, err)
	err = repo.Create(ctx, grant3)
	require.NoError(t, err)

	// Delete all grants for testAgentID1
	err = repo.DeleteByAgent(ctx, testAgentID1)
	require.NoError(t, err)

	// Verify grants for testAgentID1 are deleted
	_, err = repo.Get(ctx, grant1.ID)
	require.Error(t, err)
	_, err = repo.Get(ctx, grant2.ID)
	require.Error(t, err)

	// Verify grant for testAgentID2 still exists
	_, err = repo.Get(ctx, grant3.ID)
	require.NoError(t, err)
}

func TestUserGrantRepository_DeleteByAgent_Idempotent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Delete grants for non-existent agent (should not error)
	err := repo.DeleteByAgent(ctx, id.MustParseAgentID("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a99"))
	require.NoError(t, err)
}

func TestUserGrantRepository_ConcurrentAccess(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Test concurrent creates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			grant := &storage.UserGrant{
				Principal:             testPrincipal1,
				AgentID:               testAgentID1,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
				CreatedAt:             time.Now(),
				UpdatedAt:             time.Now(),
			}
			_ = repo.Create(ctx, grant)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify only one grant exists (upsert semantics should handle concurrency)
	grants, err := repo.ListByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Len(t, grants, 1, "upsert should result in one grant despite concurrent creates")
}

func TestUserGrantRepository_DeleteByPrincipalAndAgentID(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()
	validUntil := time.Now().Add(24 * time.Hour)

	// Create a grant for testPrincipal1 + testAgentID1
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)
	expected := grant.Copy()
	persisted := repo.grants[grant.ID]

	// Delete it
	deleted, err := repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, expected, deleted)
	require.NotNil(t, deleted.ValidUntil)
	*deleted.ValidUntil = deleted.ValidUntil.Add(time.Hour)
	deleted.GrantedPermissionSets[0].PermissionSetID = id.NewPermissionSetID()
	deleted.GrantedPermissionSets[0].IncludedServiceIDs[0] = id.NewServiceID()
	assert.Equal(t, expected, grant, "deleted snapshot must not alias the input")
	assert.Equal(t, expected, persisted, "deleted snapshot must not alias the stored row")

	// Verify the grant is gone by ID
	_, err = repo.Get(ctx, grant.ID)
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

	// Verify the principal+agent index is cleaned up
	_, err = repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.Error(t, err)
	storageErr, ok = err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
	assert.NotContains(t, repo.grantIDsByAgent, testAgentID1)
	deleted, err = repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
	require.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, deleted)
}

func TestUserGrantRepository_DeleteByPrincipalAndAgentID_NotFound(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Delete when no grant exists — must return NotFound (NOT idempotent, unlike DeleteByAgent)
	deleted, err := repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
	require.Error(t, err)
	require.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, deleted)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)
}

func TestUserGrantRepository_DeleteByPrincipalAndAgentID_CrossPrincipalIsolation(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create grants for two different principals, same agent
	grant1 := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	grant2 := &storage.UserGrant{
		Principal:             testPrincipal2,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant1)
	require.NoError(t, err)
	err = repo.Create(ctx, grant2)
	require.NoError(t, err)
	deleted, err := repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal3, testAgentID1)
	require.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, deleted)

	// Delete only principal1's grant
	deleted, err = repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Equal(t, grant1, deleted)

	// principal1's grant is gone
	_, err = repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID1)
	require.Error(t, err)
	storageErr, ok := err.(*storage.StorageError)
	require.True(t, ok)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

	// principal2's grant remains untouched (SR-001: cross-principal isolation)
	found, err := repo.FindByPrincipalAndAgent(ctx, testPrincipal2, testAgentID1)
	require.NoError(t, err)
	assert.Equal(t, grant2.ID, found.ID)
}

func TestUserGrantRepository_DeleteByPrincipalAndAgentID_AgentIndexCleanup(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create grant for agent1 and agent2 under same principal
	grantA1 := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	grantA2 := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID2,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grantA1)
	require.NoError(t, err)
	err = repo.Create(ctx, grantA2)
	require.NoError(t, err)

	// Delete agent1's grant
	deleted, err := repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
	require.NoError(t, err)
	assert.Equal(t, grantA1, deleted)
	assert.NotContains(t, repo.grantIDsByAgent, testAgentID1)
	assert.Equal(t, []id.GrantID{grantA2.ID}, repo.grantIDsByAgent[testAgentID2])

	// Cascade delete by agent1 should now be a no-op (index cleaned up)
	err = repo.DeleteByAgent(ctx, testAgentID1)
	require.NoError(t, err)

	// agent2's grant is still intact
	found, err := repo.FindByPrincipalAndAgent(ctx, testPrincipal1, testAgentID2)
	require.NoError(t, err)
	assert.Equal(t, grantA2.ID, found.ID)
}

func TestUserGrantRepository_DeleteByPrincipalAndAgentID_Concurrent(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()
	validUntil := time.Now().Add(24 * time.Hour)
	grant := &storage.UserGrant{
		ID:                    id.NewGrantID(),
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		ValidUntil:            &validUntil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	otherOwner := grant.Copy()
	otherOwner.ID = id.NewGrantID()
	otherOwner.Principal = testPrincipal2
	otherAgent := grant.Copy()
	otherAgent.ID = id.NewGrantID()
	otherAgent.AgentID = testAgentID2
	for _, seed := range []*storage.UserGrant{grant, otherOwner, otherAgent} {
		require.NoError(t, repo.Create(ctx, seed))
	}
	type deleteResult struct {
		grant *storage.UserGrant
		err   error
	}
	const count = 10
	start := make(chan struct{})
	done := make(chan deleteResult, count)
	for range count {
		go func() {
			<-start
			deleted, err := repo.DeleteByPrincipalAndAgentID(ctx, testPrincipal1, testAgentID1)
			done <- deleteResult{grant: deleted, err: err}
		}()
	}
	close(start)
	deletedCount := 0
	for range count {
		result := <-done
		if result.err == nil {
			deletedCount++
			assert.Equal(t, grant, result.grant)
		} else {
			require.ErrorIs(t, result.err, ports.ErrNotFound)
			assert.Nil(t, result.grant)
		}
	}
	assert.Equal(t, 1, deletedCount)
	assert.NotContains(t, repo.grants, grant.ID)
	assert.NotContains(t, repo.byPrincipalAndAgent, principalAgentKey(testPrincipal1, testAgentID1))
	assert.Equal(t, []id.GrantID{otherOwner.ID}, repo.grantIDsByAgent[testAgentID1])
	assert.Equal(t, []id.GrantID{otherAgent.ID}, repo.grantIDsByAgent[testAgentID2])
	for _, expected := range []*storage.UserGrant{otherOwner, otherAgent} {
		stored, err := repo.FindByPrincipalAndAgent(ctx, expected.Principal, expected.AgentID)
		require.NoError(t, err)
		assert.Equal(t, expected, stored)
	}
}

func TestUserGrantRepository_DeepCopy(t *testing.T) {
	repo := NewUserGrantRepository()
	ctx := context.Background()

	// Create grant
	grant := &storage.UserGrant{
		Principal:             testPrincipal1,
		AgentID:               testAgentID1,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}

	err := repo.Create(ctx, grant)
	require.NoError(t, err)

	// Get grant
	retrieved, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)

	// Modify the retrieved slice to verify deep copy prevents mutation of stored data
	retrieved.GrantedPermissionSets = append(retrieved.GrantedPermissionSets, storage.GrantedPermissionSetEntry{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}})

	// Get again and verify original wasn't mutated
	retrieved2, err := repo.Get(ctx, grant.ID)
	require.NoError(t, err)
	assert.Len(t, retrieved2.GrantedPermissionSets, 1, "deep copy should prevent mutation")
}

func TestUserGrantRepository_CountGrantsReferencingPermissionSet(t *testing.T) {
	ctx := context.Background()
	psID := id.NewPermissionSetID()
	svcID := id.NewServiceID()

	t.Run("counts active grants referencing the permission set", func(t *testing.T) {
		repo := NewUserGrantRepository()
		grant := &storage.UserGrant{
			Principal: testPrincipal1,
			AgentID:   testAgentID1,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, repo.Create(ctx, grant))

		count, err := repo.CountGrantsReferencingPermissionSet(ctx, psID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("excludes expired grants", func(t *testing.T) {
		repo := NewUserGrantRepository()
		past := time.Now().Add(-time.Hour)
		grant := &storage.UserGrant{
			Principal:  testPrincipal1,
			AgentID:    testAgentID1,
			ValidUntil: &past,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
			},
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		}
		repo.grants[id.NewGrantID()] = grant

		count, err := repo.CountGrantsReferencingPermissionSet(ctx, psID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("counts indefinite grants (no valid_until)", func(t *testing.T) {
		repo := NewUserGrantRepository()
		grant := &storage.UserGrant{
			Principal: testPrincipal1,
			AgentID:   testAgentID1,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, repo.Create(ctx, grant))

		count, err := repo.CountGrantsReferencingPermissionSet(ctx, psID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("counts grants with future valid_until as active", func(t *testing.T) {
		repo := NewUserGrantRepository()
		future := time.Now().Add(time.Hour)
		grant := &storage.UserGrant{
			Principal:  testPrincipal1,
			AgentID:    testAgentID1,
			ValidUntil: &future,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		require.NoError(t, repo.Create(ctx, grant))

		count, err := repo.CountGrantsReferencingPermissionSet(ctx, psID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("returns zero when no grants reference the permission set", func(t *testing.T) {
		repo := NewUserGrantRepository()
		count, err := repo.CountGrantsReferencingPermissionSet(ctx, psID)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestUserGrantRepository_ListByPrincipalAndServiceID(t *testing.T) {
	ctx := context.Background()
	repo := NewUserGrantRepository()
	principalA := id.Principal("principal-a@example.com")
	principalB := id.Principal("principal-b@example.com")
	targetServiceID := id.NewServiceID()
	nonTargetServiceID := id.NewServiceID()
	activeAgentID := id.NewAgentID()
	otherPrincipalAgentID := id.NewAgentID()
	nonTargetAgentID := id.NewAgentID()
	expiredAgentID := id.NewAgentID()

	for _, grant := range []*storage.UserGrant{
		{Principal: principalA, AgentID: activeAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}}},
		{Principal: principalB, AgentID: otherPrincipalAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}}},
		{Principal: principalA, AgentID: nonTargetAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{nonTargetServiceID}}}},
	} {
		require.NoError(t, repo.Create(ctx, grant))
	}

	past := time.Now().Add(-time.Hour)
	repo.grants[id.NewGrantID()] = &storage.UserGrant{
		Principal:             principalA,
		AgentID:               expiredAgentID,
		ValidUntil:            &past,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}},
	}

	agentIDs, err := repo.ListByPrincipalAndServiceID(ctx, principalA, targetServiceID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []id.AgentID{activeAgentID, expiredAgentID}, agentIDs)
	assert.NotContains(t, agentIDs, otherPrincipalAgentID)
	assert.NotContains(t, agentIDs, nonTargetAgentID)
}

func TestUserGrantRepository_CountAgentsByPrincipalAndServiceID(t *testing.T) {
	ctx := context.Background()
	repo := NewUserGrantRepository()
	principalA := id.Principal("principal-a@example.com")
	principalB := id.Principal("principal-b@example.com")
	targetServiceID := id.NewServiceID()
	nonTargetServiceID := id.NewServiceID()
	activeAgentID := id.NewAgentID()
	otherPrincipalAgentID := id.NewAgentID()
	nonTargetAgentID := id.NewAgentID()
	expiredAgentID := id.NewAgentID()

	for _, grant := range []*storage.UserGrant{
		{Principal: principalA, AgentID: activeAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}}},
		{Principal: principalB, AgentID: otherPrincipalAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}}},
		{Principal: principalA, AgentID: nonTargetAgentID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{nonTargetServiceID}}}},
	} {
		require.NoError(t, repo.Create(ctx, grant))
	}

	past := time.Now().Add(-time.Hour)
	repo.grants[id.NewGrantID()] = &storage.UserGrant{
		Principal:             principalA,
		AgentID:               expiredAgentID,
		ValidUntil:            &past,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{targetServiceID}}},
	}

	count, err := repo.CountAgentsByPrincipalAndServiceID(ctx, principalA, targetServiceID)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}
