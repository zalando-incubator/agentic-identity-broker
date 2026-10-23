package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientAssertionReplayRepoRejectsUninitializedAdapter(t *testing.T) {
	repo := NewClientAssertionReplayRepo(nil)
	err := repo.Consume(context.Background(), "client-a", "jti-1", time.Now().Add(time.Minute))
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConnection, storageErr.Kind)

	_, err = repo.DeleteExpired(context.Background())
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConnection, storageErr.Kind)
}
