package memory

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionSetRepository_Delete(t *testing.T) {
	repo := NewPermissionSetRepository()
	ctx := context.Background()
	ps := &storage.PermissionSet{
		ID:            id.NewPermissionSetID(),
		Name:          "Read",
		Description:   "Read service",
		ServiceScopes: []storage.ServiceScope{{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
	}
	require.NoError(t, repo.Create(ctx, ps))
	deleted, err := repo.Delete(ctx, ps.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.Delete(ctx, ps.ID)
	require.NoError(t, err)
	assert.False(t, deleted)
	deleted, err = repo.Delete(ctx, id.NewPermissionSetID())
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestPermissionSetRepository_Delete_Concurrent(t *testing.T) {
	repo := NewPermissionSetRepository()
	ctx := context.Background()
	canonicalID := "deleted-permission-set"
	ps := &storage.PermissionSet{
		ID:            id.NewPermissionSetID(),
		CanonicalID:   &canonicalID,
		Name:          "Read",
		Description:   "Read service",
		ServiceScopes: []storage.ServiceScope{{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
	}
	survivor := ps.Copy()
	survivor.ID = id.NewPermissionSetID()
	survivor.CanonicalID = nil
	survivor.Name = "Write"
	require.NoError(t, repo.Create(ctx, ps))
	require.NoError(t, repo.Create(ctx, survivor))
	type deleteResult struct {
		deleted bool
		err     error
	}
	const count = 10
	start := make(chan struct{})
	done := make(chan deleteResult, count)
	for range count {
		go func() {
			<-start
			deleted, err := repo.Delete(ctx, ps.ID)
			done <- deleteResult{deleted: deleted, err: err}
		}()
	}
	close(start)
	deletedCount := 0
	for range count {
		result := <-done
		require.NoError(t, result.err)
		if result.deleted {
			deletedCount++
		}
	}
	assert.Equal(t, 1, deletedCount)
	assert.NotContains(t, repo.permissionSets, ps.ID)
	assert.NotContains(t, repo.nameIndex, ps.Name)
	assert.NotContains(t, repo.canonicalIndex, canonicalID)
	found, err := repo.Get(ctx, survivor.ID)
	require.NoError(t, err)
	assert.Equal(t, survivor, found)
	replacement := ps.Copy()
	replacement.ID = id.NewPermissionSetID()
	require.NoError(t, repo.Create(ctx, replacement), "deletion must release both name and canonical ID indexes")
}
