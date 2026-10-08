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

func TestMemoryGrantExpirationCandidatesAndChangedValidity(t *testing.T) {
	tx := NewTransactionManager()
	repo := NewUserGrantRepository(tx)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	firstExpiry, secondExpiry, future := now.Add(-time.Hour), now.Add(-time.Minute), now.Add(time.Hour)
	expired := &storage.UserGrant{ID: id.NewGrantID(), Principal: "expired-user", AgentID: id.NewAgentID(), ValidUntil: &firstExpiry}
	active := &storage.UserGrant{ID: id.NewGrantID(), Principal: expired.Principal, AgentID: id.NewAgentID(), ValidUntil: &future}
	indefinite := &storage.UserGrant{ID: id.NewGrantID(), Principal: expired.Principal, AgentID: id.NewAgentID()}
	for _, grant := range []*storage.UserGrant{expired, active, indefinite} {
		require.NoError(t, repo.Create(ctx, grant))
	}
	candidates, err := repo.ListUnrecordedExpiredForPrincipal(ctx, expired.Principal, now, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, expired.ID, candidates[0].ID)
	winner, err := repo.RecordExpiration(ctx, expired.ID, firstExpiry)
	require.NoError(t, err)
	require.True(t, winner)
	winner, err = repo.RecordExpiration(ctx, expired.ID, firstExpiry)
	require.NoError(t, err)
	require.False(t, winner)
	candidates, err = repo.ListUnrecordedExpiredForPrincipal(ctx, expired.Principal, now, 10)
	require.NoError(t, err)
	require.Empty(t, candidates)
	expired.ValidUntil = &secondExpiry
	require.NoError(t, repo.Update(ctx, expired))
	winner, err = repo.RecordExpiration(ctx, expired.ID, firstExpiry)
	require.NoError(t, err)
	require.False(t, winner, "a stale recognizer cannot mark a different effective expiry")
	candidates, err = repo.ListUnrecordedExpiredForPrincipal(ctx, expired.Principal, now, 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, secondExpiry, *candidates[0].ValidUntil)
	winner, err = repo.RecordExpiration(ctx, expired.ID, secondExpiry)
	require.NoError(t, err)
	require.True(t, winner)
}

func TestMemoryGrantExpirationRollbackRestoresRecognition(t *testing.T) {
	tx := NewTransactionManager()
	repo := NewUserGrantRepository(tx)
	ctx := context.Background()
	expiry := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: "ledger-user", AgentID: id.NewAgentID(), ValidUntil: &expiry}
	require.NoError(t, repo.Create(ctx, grant))
	owner, err := tx.BeginTX(ctx)
	require.NoError(t, err)
	winner, err := repo.RecordExpiration(owner, grant.ID, expiry)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(owner))
	require.True(t, winner)
	winner, err = repo.RecordExpiration(ctx, grant.ID, expiry)
	require.NoError(t, err)
	require.True(t, winner, "rolled-back recognition must remain available")
}

func TestMemoryGrantExpirationCompetingRecognizersHaveOneWinner(t *testing.T) {
	tx := NewTransactionManager()
	repo := NewUserGrantRepository(tx)
	expiry := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	grant := &storage.UserGrant{ID: id.NewGrantID(), Principal: "ledger-user", AgentID: id.NewAgentID(), ValidUntil: &expiry}
	require.NoError(t, repo.Create(context.Background(), grant))
	type result struct {
		won bool
		err error
	}
	results := make(chan result, 16)
	var workers sync.WaitGroup
	for range 16 {
		workers.Go(func() {
			won, err := repo.RecordExpiration(context.Background(), grant.ID, expiry)
			results <- result{won, err}
		})
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
	require.Equal(t, 1, winners)
}

func TestMemoryGrantExpirationScopedCandidatesFilterBeforeLimit(t *testing.T) {
	repo := NewUserGrantRepository(NewTransactionManager())
	ctx := context.Background()
	now := time.Now().UTC()
	expiry := now.Add(-time.Hour)
	requested := &storage.UserGrant{ID: id.NewGrantID(), Principal: "requested-user", AgentID: id.NewAgentID(), ValidUntil: &expiry}
	other := &storage.UserGrant{ID: id.NewGrantID(), Principal: "other-user", AgentID: id.NewAgentID(), ValidUntil: &expiry}
	require.NoError(t, repo.Create(ctx, requested))
	require.NoError(t, repo.Create(ctx, other))
	candidates, err := repo.ListUnrecordedExpiredForPrincipal(ctx, requested.Principal, now, 1)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, requested.ID, candidates[0].ID)
}
