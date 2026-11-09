//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientAssertionReplayRepoConsumeConcurrent(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewClientAssertionReplayRepo(adapter)
	const workers = 25
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := repo.Consume(context.Background(), "client-a", "shared-jti", time.Now().Add(time.Minute))
			if err == nil {
				successes.Add(1)
				return
			}
			var storageErr *storage.StorageError
			if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict {
				conflicts.Add(1)
				return
			}
			t.Errorf("unexpected consume error: %v", err)
		}()
	}
	wait.Wait()
	assert.EqualValues(t, 1, successes.Load())
	assert.EqualValues(t, workers-1, conflicts.Load())
}

func TestClientAssertionReplayRepoScopesJTIByClient(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewClientAssertionReplayRepo(adapter)
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, repo.Consume(context.Background(), "client-a", "shared-jti", expiresAt))
	require.NoError(t, repo.Consume(context.Background(), "client-b", "shared-jti", expiresAt))
}

func TestClientAssertionReplayRepoIgnoresAmbientTransaction(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewClientAssertionReplayRepo(adapter)
	txCtx, err := adapter.BeginTX(context.Background())
	require.NoError(t, err)
	require.NoError(t, repo.Consume(txCtx, "client-a", "rollback-independent", time.Now().Add(time.Minute)))
	require.NoError(t, adapter.Rollback(txCtx))

	err = repo.Consume(context.Background(), "client-a", "rollback-independent", time.Now().Add(time.Minute))
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

func TestClientAssertionReplayRepoDeleteExpired(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewClientAssertionReplayRepo(adapter)
	require.NoError(t, repo.Consume(context.Background(), "client-a", "expired", time.Now().Add(-time.Minute)))
	require.NoError(t, repo.Consume(context.Background(), "client-a", "active", time.Now().Add(time.Minute)))

	deleted, err := repo.DeleteExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	err = repo.Consume(context.Background(), "client-a", "expired", time.Now().Add(time.Minute))
	require.NoError(t, err)
	err = repo.Consume(context.Background(), "client-a", "active", time.Now().Add(time.Minute))
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}
