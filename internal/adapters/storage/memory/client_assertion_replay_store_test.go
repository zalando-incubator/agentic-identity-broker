package memory

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

func TestClientAssertionReplayStoreConsume(t *testing.T) {
	store := NewClientAssertionReplayStore()
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, store.Consume(context.Background(), "client-a", "jti-1", expiresAt))
	err := store.Consume(context.Background(), "client-a", "jti-1", expiresAt)
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

func TestClientAssertionReplayStoreConsumeConcurrent(t *testing.T) {
	store := NewClientAssertionReplayStore()
	const workers = 50
	var successes atomic.Int32
	var conflicts atomic.Int32
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			err := store.Consume(context.Background(), "client-a", "shared-jti", time.Now().Add(time.Minute))
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

func TestClientAssertionReplayStoreScopesJTIByClient(t *testing.T) {
	store := NewClientAssertionReplayStore()
	expiresAt := time.Now().Add(time.Minute)
	require.NoError(t, store.Consume(context.Background(), "client-a", "shared-jti", expiresAt))
	require.NoError(t, store.Consume(context.Background(), "client-b", "shared-jti", expiresAt))
}

func TestClientAssertionReplayStoreDeleteExpired(t *testing.T) {
	store := NewClientAssertionReplayStore()
	require.NoError(t, store.Consume(context.Background(), "client-a", "expired", time.Now().Add(-time.Second)))
	require.NoError(t, store.Consume(context.Background(), "client-a", "active", time.Now().Add(time.Minute)))

	deleted, err := store.DeleteExpired(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, deleted)

	err = store.Consume(context.Background(), "client-a", "expired", time.Now().Add(time.Minute))
	require.NoError(t, err, "expired replay record should be removable")
	err = store.Consume(context.Background(), "client-a", "active", time.Now().Add(time.Minute))
	require.Error(t, err, "active replay record must remain")
}
