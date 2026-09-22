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
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("ciphertext"),
		IsCurrent:           isCurrent,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
}

func testSigningKeyInDomain(kid string, domain storage.KeyDomain, isCurrent bool) *storage.SigningKey {
	key := testSigningKey(kid, isCurrent)
	key.KeyDomain = domain
	return key
}

func TestSigningKeyStore_WithBootstrapLock(t *testing.T) {
	t.Run("returns context error when already canceled", func(t *testing.T) {
		store := NewSigningKeyStore()
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
		store := NewSigningKeyStore()
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
		store := NewSigningKeyStore()
		counted := make(chan struct{})
		allowCreate := make(chan struct{})
		bootstrapDone := make(chan error, 1)
		nonBootstrapDone := make(chan error, 1)

		go func() {
			bootstrapDone <- store.WithBootstrapLock(context.Background(), func(lockCtx context.Context) error {
				count, err := store.CountActiveInDomain(lockCtx, storage.KeyDomainTokenSigning)
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
		store := NewSigningKeyStore()
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		activatesAt := time.Now().UTC().Add(-2 * time.Minute).Round(time.Second)
		promoted, err := store.SetCurrentInDomain(ctx, storage.KeyDomainTokenSigning, other.KID, activatesAt)
		require.NoError(t, err)
		assert.Equal(t, activatesAt, promoted.ActivatesAt)

		stored, err := store.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, other.KID)
		require.NoError(t, err)
		assert.Equal(t, activatesAt, stored.ActivatesAt)
	})
}

func TestSigningKeyStore_Delete(t *testing.T) {
	t.Run("rejects deleting the last active key", func(t *testing.T) {
		store := NewSigningKeyStore()
		ctx := context.Background()
		key := testSigningKey("current-kid", true)
		require.NoError(t, store.Create(ctx, key))

		err := store.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, key.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrLastActiveKey)
	})

	t.Run("rejects deleting the current key when another key exists", func(t *testing.T) {
		store := NewSigningKeyStore()
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		err := store.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, current.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrCurrentKey)
	})

	t.Run("rejects deleting the currently usable fallback key during grace period", func(t *testing.T) {
		store := NewSigningKeyStore()
		ctx := context.Background()
		fallback := testSigningKey("fallback-kid", false)
		fallback.ActivatesAt = time.Now().UTC().Add(-time.Minute)
		replacement := testSigningKey("replacement-kid", true)
		replacement.ActivatesAt = time.Now().UTC().Add(time.Hour)
		require.NoError(t, store.Create(ctx, fallback))
		require.NoError(t, store.Create(ctx, replacement))

		err := store.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, fallback.KID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)
	})

	t.Run("deletes a non-current key when another active key remains", func(t *testing.T) {
		store := NewSigningKeyStore()
		ctx := context.Background()
		current := testSigningKey("current-kid", true)
		other := testSigningKey("other-kid", false)
		require.NoError(t, store.Create(ctx, current))
		require.NoError(t, store.Create(ctx, other))

		err := store.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, other.KID)
		require.NoError(t, err)

		_, err = store.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, other.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))
	})
}

func TestSigningKeyStore_KeyDomainIsolation(t *testing.T) {
	ctx := context.Background()

	t.Run("filters lookup, active keys, and counts by key domain", func(t *testing.T) {
		store := NewSigningKeyStore()
		tokenKey := testSigningKeyInDomain("token-key", storage.KeyDomainTokenSigning, false)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, store.Create(ctx, tokenKey))
		require.NoError(t, store.Create(ctx, cimdKey))

		got, err := store.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, tokenKey.KID)
		require.NoError(t, err)
		assert.Equal(t, tokenKey.KID, got.KID)
		assert.Equal(t, storage.KeyDomainTokenSigning, got.KeyDomain)

		_, err = store.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		tokenKeys, err := store.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		require.Len(t, tokenKeys, 1)
		assert.Equal(t, tokenKey.KID, tokenKeys[0].KID)

		count, err := store.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("only promotes a key from the requested key domain", func(t *testing.T) {
		store := NewSigningKeyStore()
		tokenCurrent := testSigningKeyInDomain("token-current", storage.KeyDomainTokenSigning, true)
		cimdCandidate := testSigningKeyInDomain("cimd-candidate", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, store.Create(ctx, tokenCurrent))
		require.NoError(t, store.Create(ctx, cimdCandidate))

		_, err := store.SetCurrentInDomain(ctx, storage.KeyDomainTokenSigning, cimdCandidate.KID, time.Now().UTC())
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		current, err := store.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCurrent.KID, current.KID)
	})

	t.Run("does not delete a key from another key domain", func(t *testing.T) {
		store := NewSigningKeyStore()
		tokenCurrent := testSigningKeyInDomain("token-current", storage.KeyDomainTokenSigning, true)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, store.Create(ctx, tokenCurrent))
		require.NoError(t, store.Create(ctx, cimdKey))

		err := store.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		remaining, err := store.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, cimdKey.KID)
		require.NoError(t, err)
		assert.Equal(t, cimdKey.KID, remaining.KID)
	})

	t.Run("keeps grace-period fallback selection within the requested key domain", func(t *testing.T) {
		store := NewSigningKeyStore()
		now := time.Now().UTC()
		tokenFallback := testSigningKeyInDomain("token-fallback", storage.KeyDomainTokenSigning, false)
		tokenFallback.ActivatesAt = now.Add(-2 * time.Minute)
		tokenPending := testSigningKeyInDomain("token-pending", storage.KeyDomainTokenSigning, true)
		tokenPending.ActivatesAt = now.Add(time.Hour)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		cimdKey.ActivatesAt = now.Add(-time.Minute)
		require.NoError(t, store.Create(ctx, tokenFallback))
		require.NoError(t, store.Create(ctx, tokenPending))
		require.NoError(t, store.Create(ctx, cimdKey))

		current, err := store.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenFallback.KID, current.KID)
	})

	t.Run("observes an empty domain while bootstrapping under the lock", func(t *testing.T) {
		store := NewSigningKeyStore()
		tokenKey := testSigningKeyInDomain("token-key", storage.KeyDomainTokenSigning, true)
		require.NoError(t, store.Create(ctx, tokenKey))

		err := store.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
			count, err := store.CountActiveInDomain(lockCtx, storage.KeyDomainCIMDClientAuthentication)
			require.NoError(t, err)
			assert.Zero(t, count)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("rejects duplicate kids across key domains", func(t *testing.T) {
		store := NewSigningKeyStore()
		tokenKey := testSigningKeyInDomain("shared-kid", storage.KeyDomainTokenSigning, true)
		cimdKey := testSigningKeyInDomain("shared-kid", storage.KeyDomainCIMDClientAuthentication, true)
		require.NoError(t, store.Create(ctx, tokenKey))

		err := store.Create(ctx, cimdKey)
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})
}
