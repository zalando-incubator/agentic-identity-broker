package memory

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestMemoryApprovalExpirationCandidatesRespectExistingLifecycle(t *testing.T) {
	repo := NewToolApprovalRepository(NewTransactionManager())
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(-time.Hour)
	pending := &storage.ToolApproval{ID: id.NewApprovalID(), Principal: "ledger-user", AgentID: id.NewAgentID(), Status: storage.ApprovalStatusPending, ExpiresAt: expiry}
	active := *pending
	active.ID, active.ExpiresAt = id.NewApprovalID(), now.Add(time.Hour)
	permanent := *pending
	persistence := storage.ApprovalPersistencePermanent
	permanent.ID, permanent.Status, permanent.Persistence = id.NewApprovalID(), storage.ApprovalStatusApproved, &persistence
	other := *pending
	other.ID, other.Principal = id.NewApprovalID(), "another-owner"
	repo.approvals[pending.ID], repo.approvals[active.ID], repo.approvals[permanent.ID], repo.approvals[other.ID] = pending, &active, &permanent, &other
	candidates, err := repo.ListUnrecordedExpiredForPrincipal(ctx, pending.Principal, now, 1)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "resolved permanent approvals do not expire with the pending TTL")
	require.Equal(t, pending.ID, candidates[0].ID)
	winner, err := repo.RecordExpiration(ctx, permanent.ID, expiry)
	require.NoError(t, err)
	require.False(t, winner)
	winner, err = repo.RecordExpiration(ctx, pending.ID, expiry)
	require.NoError(t, err)
	require.True(t, winner)
	winner, err = repo.RecordExpiration(ctx, pending.ID, expiry)
	require.NoError(t, err)
	require.False(t, winner)
	candidates, err = repo.ListUnrecordedExpiredForPrincipal(ctx, pending.Principal, now, 10)
	require.NoError(t, err)
	require.Empty(t, candidates)
	candidates, err = repo.ListUnrecordedExpiredForPrincipal(ctx, other.Principal, now, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "another principal's expiration is not consumed by this scan")
	require.Equal(t, other.ID, candidates[0].ID)
	pending.ExpiresAt = expiry.Add(time.Minute)
	candidates, err = repo.ListUnrecordedExpiredForPrincipal(ctx, pending.Principal, now, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "changing effective expiry makes a new recognition candidate")
	winner, err = repo.RecordExpiration(ctx, pending.ID, expiry)
	require.NoError(t, err)
	require.False(t, winner)
	winner, err = repo.RecordExpiration(ctx, pending.ID, pending.ExpiresAt)
	require.NoError(t, err)
	require.True(t, winner, "a genuinely changed pending expiry is a new recognition")
}

func TestMemoryApprovalExpirationRollbackAndCompetingRecognizers(t *testing.T) {
	tx := NewTransactionManager()
	repo := NewToolApprovalRepository(tx)
	ctx := context.Background()
	expiry := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	approval := &storage.ToolApproval{ID: id.NewApprovalID(), Principal: "ledger-user", AgentID: id.NewAgentID(), Status: storage.ApprovalStatusPending, ExpiresAt: expiry}
	repo.approvals[approval.ID] = approval
	owner, err := tx.BeginTX(ctx)
	require.NoError(t, err)
	won, err := repo.RecordExpiration(owner, approval.ID, expiry)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(owner))
	require.True(t, won)
	type result struct {
		won bool
		err error
	}
	results := make(chan result, 16)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() { won, err := repo.RecordExpiration(ctx, approval.ID, expiry); results <- result{won, err} })
	}
	workers.Wait()
	close(results)
	winners := 0
	for result := range results {
		require.NoError(t, result.err)
		if result.won {
			winners++
		}
	}
	require.Equal(t, 1, winners, "rollback permits recognition, then only one competing recognizer wins")
}
