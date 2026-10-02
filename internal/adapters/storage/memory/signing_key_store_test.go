package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func testSigningKey(kid string, isCurrent bool) *storage.SigningKey {
	now := time.Now().UTC()
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID(kid),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("ciphertext"),
		IsCurrent:           isCurrent,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
}

func TestSigningKeyStore_WithBootstrapLock(t *testing.T) {
	t.Run("returns context error when already canceled", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		called := false
		err := store.WithBootstrapLock(ctx, func(context.Context) error {
			called = true
			return nil
		})

		require.ErrorIs(t, err, context.Canceled)
		assert.False(t, called)
	})

	t.Run("returns context error while waiting for held lock", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		lockHeld := make(chan struct{})
		releaseLock := make(chan struct{})
		firstDone := make(chan error, 1)

		go func() {
			firstDone <- store.WithBootstrapLock(context.Background(), func(context.Context) error {
				close(lockHeld)
				<-releaseLock
				return nil
			})
		}()

		<-lockHeld

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		secondDone := make(chan error, 1)
		go func() {
			secondDone <- store.WithBootstrapLock(ctx, func(context.Context) error {
				return nil
			})
		}()

		select {
		case err := <-secondDone:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(200 * time.Millisecond):
			t.Fatal("WithBootstrapLock did not respect canceled context while waiting for lock")
		}

		close(releaseLock)
		require.NoError(t, <-firstDone)
	})

	t.Run("blocks non-bootstrap writes until the bootstrap flow completes", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		counted := make(chan struct{})
		allowCreate := make(chan struct{})
		bootstrapDone := make(chan error, 1)
		nonBootstrapDone := make(chan error, 1)

		go func() {
			bootstrapDone <- store.WithBootstrapLock(context.Background(), func(lockCtx context.Context) error {
				count, err := store.CountActive(lockCtx)
				if err != nil {
					return err
				}
				if count != 0 {
					return assert.AnError
				}
				close(counted)
				<-allowCreate
				return store.CreateAndSetCurrent(lockCtx, testSigningKey("bootstrap-kid", true))
			})
		}()

		<-counted

		go func() {
			nonBootstrapDone <- store.Create(context.Background(), testSigningKey("competing-kid", false))
		}()

		select {
		case err := <-nonBootstrapDone:
			t.Fatalf("non-bootstrap write completed before bootstrap flow finished: %v", err)
		case <-time.After(50 * time.Millisecond):
		}

		close(allowCreate)
		require.NoError(t, <-bootstrapDone)
		require.NoError(t, <-nonBootstrapDone)
	})
}

func TestSigningKeyStore_SetCurrent(t *testing.T) {
	t.Run("uses the provided activation time", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		activatesAt := time.Now().UTC().Add(-2 * time.Minute).Round(time.Second)
		promoted, err := store.SetCurrent(ctx, other.KID, activatesAt)
		require.NoError(t, err)
		assert.Equal(t, activatesAt, promoted.ActivatesAt)

		stored, err := store.GetByKID(ctx, other.KID)
		require.NoError(t, err)
		assert.Equal(t, activatesAt, stored.ActivatesAt)
	})
}

func TestSigningKeyStore_Delete(t *testing.T) {
	t.Run("rejects deleting the last active key", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx := context.Background()
		key := testSigningKey("current-kid", true)
		require.NoError(t, store.Create(ctx, key))

		err := store.Delete(ctx, key.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrLastActiveKey)
	})

	t.Run("rejects deleting the current key when another key exists", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		err := store.Delete(ctx, current.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrCurrentKey)
	})

	t.Run("rejects deleting the currently usable fallback key during grace period", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx := context.Background()
		fallback := testSigningKey("fallback-kid", false)
		fallback.ActivatesAt = time.Now().UTC().Add(-time.Minute)
		replacement := testSigningKey("replacement-kid", true)
		replacement.ActivatesAt = time.Now().UTC().Add(time.Hour)
		require.NoError(t, store.Create(ctx, fallback))
		require.NoError(t, store.Create(ctx, replacement))

		err := store.Delete(ctx, fallback.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)
	})

	t.Run("deletes a non-current key when another active key remains", func(t *testing.T) {
		store := NewSigningKeyStore(NewTransactionManager())
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		err := store.Delete(ctx, other.KID)
		require.NoError(t, err)

		_, err = store.GetByKID(ctx, other.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
	})
}
