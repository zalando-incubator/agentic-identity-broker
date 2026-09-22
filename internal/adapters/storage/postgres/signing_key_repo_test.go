//go:build integration
// +build integration

package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupSigningKeyTestDB(t *testing.T) (*Adapter, func()) {
	t.Helper()
	return setupMigratedAdapter(t)
}

func testSigningKeyInDomain(kid string, domain storage.KeyDomain, isCurrent bool) *storage.SigningKey {
	now := time.Now().UTC()
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID(kid),
		KeyDomain:           domain,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           isCurrent,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
}

func TestSigningKeyRepo_Create(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-create-test"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}

	err := repo.Create(context.Background(), key)
	require.NoError(t, err)
}

func TestSigningKeyRepo_CreateRejectsSecondActiveCurrentKey(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	now := time.Now().UTC()

	key1 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-1"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-2"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key2"),
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}

	require.NoError(t, repo.Create(ctx, key1))
	err := repo.Create(ctx, key2)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

func TestSigningKeyRepo_GetByKID(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	kid := id.NewKeyID("kid-get-test")
	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 kid,
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	err := repo.Create(ctx, key)
	require.NoError(t, err)

	got, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, kid)
	require.NoError(t, err)
	assert.Equal(t, kid, got.KID)
	assert.Equal(t, "ES256", got.Algorithm)
	assert.True(t, got.IsCurrent)
}

func TestSigningKeyRepo_GetCurrent(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	err := repo.Create(ctx, key)
	require.NoError(t, err)

	got, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, key.KID, got.KID)
	assert.True(t, got.IsCurrent)
}

func TestSigningKeyRepo_GetCurrentFallsBackToLatestActiveKeyDuringGracePeriod(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	now := time.Now().UTC()

	fallback := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-fallback"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("fallback-key"),
		IsCurrent:           false,
		ActivatesAt:         now.Add(-1 * time.Minute),
		CreatedAt:           now.Add(-2 * time.Minute),
	}
	futureCurrent := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-future"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("future-key"),
		IsCurrent:           true,
		ActivatesAt:         now.Add(10 * time.Minute),
		CreatedAt:           now.Add(-1 * time.Minute),
	}
	require.NoError(t, repo.Create(ctx, fallback))
	require.NoError(t, repo.Create(ctx, futureCurrent))

	got, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, fallback.KID, got.KID)
	assert.False(t, got.IsCurrent)
}

func TestSigningKeyRepo_ListActive(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	// Create two active keys
	key1 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-list-1"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-list-2"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "RS256",
		PrivateKeyEncrypted: []byte("key2"),
		IsCurrent:           false,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key1))
	require.NoError(t, repo.Create(ctx, key2))

	active, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Len(t, active, 2)
}

func TestSigningKeyRepo_SetCurrent(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	key1 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-set-1"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-set-2"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key2"),
		IsCurrent:           false,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key1))
	require.NoError(t, repo.Create(ctx, key2))

	// Promote key2 using an explicit activation time supplied by the domain layer.
	activatesAt := time.Now().UTC().Add(-time.Minute).Round(time.Second)
	promoted, err := repo.SetCurrentInDomain(ctx, storage.KeyDomainTokenSigning, key2.KID, activatesAt)
	require.NoError(t, err)
	assert.Equal(t, key2.KID, promoted.KID)
	assert.True(t, promoted.IsCurrent)
	assert.WithinDuration(t, activatesAt, promoted.ActivatesAt, time.Second)

	// Verify key2 is now current
	got, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, key2.KID, got.KID)
	assert.WithinDuration(t, activatesAt, got.ActivatesAt, time.Second)

	// Verify key1 is no longer current
	got1, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, key1.KID)
	require.NoError(t, err)
	assert.False(t, got1.IsCurrent)
}

func TestSigningKeyRepo_Delete(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	current := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-current"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("current-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	other := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-other"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("other-key"),
		IsCurrent:           false,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, current))
	require.NoError(t, repo.Create(ctx, other))

	err := repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, other.KID)
	require.NoError(t, err)

	// Should not appear in active list
	active, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	for _, k := range active {
		assert.NotEqual(t, other.KID, k.KID)
	}
}

func TestSigningKeyRepo_DeleteRejectsCurrentKey(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	current := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-current"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("current-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	other := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-other"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("other-key"),
		IsCurrent:           false,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, current))
	require.NoError(t, repo.Create(ctx, other))

	err := repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, current.KID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrCurrentKey)
}

func TestSigningKeyRepo_DeleteRejectsCurrentlyUsableFallbackKey(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	now := time.Now().UTC()

	fallback := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-fallback"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("fallback-key"),
		IsCurrent:           false,
		ActivatesAt:         now.Add(-time.Minute),
		CreatedAt:           now.Add(-2 * time.Minute),
	}
	replacement := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-future-current"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("future-key"),
		IsCurrent:           true,
		ActivatesAt:         now.Add(time.Hour),
		CreatedAt:           now.Add(-time.Minute),
	}
	require.NoError(t, repo.Create(ctx, fallback))
	require.NoError(t, repo.Create(ctx, replacement))

	err := repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, fallback.KID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrEffectiveCurrentKey)
}

func TestSigningKeyRepo_DeleteRejectsLastActiveKey(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-last"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("last-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key))

	err := repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, key.KID)
	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrLastActiveKey)
}

func TestSigningKeyRepo_CountActive(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-count"),
		KeyDomain:           storage.KeyDomainTokenSigning,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key))

	count, err := repo.CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSigningKeyRepo_WithBootstrapLockSerializesCallers(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	secondEntered := make(chan struct{})
	secondDone := make(chan error, 1)

	go func() {
		firstDone <- repo.WithBootstrapLock(ctx, func(context.Context) error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()

	<-firstEntered

	go func() {
		secondDone <- repo.WithBootstrapLock(ctx, func(context.Context) error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondEntered:
		t.Fatal("second caller acquired bootstrap lock before first released it")
	case <-time.After(200 * time.Millisecond):
	}

	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)

	select {
	case <-secondEntered:
	case <-time.After(time.Second):
		t.Fatal("second caller never acquired bootstrap lock after first released it")
	}
}

func TestSigningKeyRepo_WithBootstrapLockSerializesBootstrapFlow(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repos := []*SigningKeyRepo{
		NewSigningKeyRepo(adapter),
		NewSigningKeyRepo(adapter),
	}

	type result struct {
		created bool
		err     error
	}

	results := make(chan result, len(repos))
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		kid := id.NewKeyID(fmt.Sprintf("kid-%d", i))
		go func(repo *SigningKeyRepo, kid id.KeyID) {
			defer wg.Done()
			<-start

			created := false
			err := repo.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
				count, err := repo.CountActiveInDomain(lockCtx, storage.KeyDomainTokenSigning)
				if err != nil {
					return err
				}
				if count > 0 {
					return nil
				}

				created = true
				now := time.Now().UTC()
				return repo.CreateAndSetCurrent(lockCtx, &storage.SigningKey{
					ID:                  id.NewSigningKeyID(),
					KID:                 kid,
					KeyDomain:           storage.KeyDomainTokenSigning,
					Algorithm:           "ES256",
					PrivateKeyEncrypted: []byte("ciphertext"),
					IsCurrent:           true,
					ActivatesAt:         now,
					CreatedAt:           now,
				})
			})

			results <- result{created: created, err: err}
		}(repo, kid)
	}

	close(start)
	wg.Wait()
	close(results)

	createdCount := 0
	for result := range results {
		require.NoError(t, result.err)
		if result.created {
			createdCount++
		}
	}
	assert.Equal(t, 1, createdCount)

	count, err := repos[0].CountActiveInDomain(ctx, storage.KeyDomainTokenSigning)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestSigningKeyRepo_KeyDomainIsolation(t *testing.T) {
	ctx := context.Background()

	t.Run("filters lookup, active keys, and counts by key domain", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		tokenKey := testSigningKeyInDomain("token-key", storage.KeyDomainTokenSigning, false)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenKey))
		require.NoError(t, repo.Create(ctx, cimdKey))

		got, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, tokenKey.KID)
		require.NoError(t, err)
		assert.Equal(t, tokenKey.KID, got.KID)
		assert.Equal(t, storage.KeyDomainTokenSigning, got.KeyDomain)

		_, err = repo.GetByKIDInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		tokenKeys, err := repo.ListActiveInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		require.Len(t, tokenKeys, 1)
		assert.Equal(t, tokenKey.KID, tokenKeys[0].KID)

		count, err := repo.CountActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("only promotes a key from the requested key domain", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		tokenCurrent := testSigningKeyInDomain("token-current", storage.KeyDomainTokenSigning, true)
		cimdCandidate := testSigningKeyInDomain("cimd-candidate", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenCurrent))
		require.NoError(t, repo.Create(ctx, cimdCandidate))

		_, err := repo.SetCurrentInDomain(ctx, storage.KeyDomainTokenSigning, cimdCandidate.KID, time.Now().UTC())
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		current, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenCurrent.KID, current.KID)
	})

	t.Run("does not delete a key from another key domain", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		tokenCurrent := testSigningKeyInDomain("token-current", storage.KeyDomainTokenSigning, true)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		require.NoError(t, repo.Create(ctx, tokenCurrent))
		require.NoError(t, repo.Create(ctx, cimdKey))

		err := repo.DeleteInDomain(ctx, storage.KeyDomainTokenSigning, cimdKey.KID)
		require.Error(t, err)
		assert.True(t, ports.IsNotFoundErr(err))

		remaining, err := repo.GetByKIDInDomain(ctx, storage.KeyDomainCIMDClientAuthentication, cimdKey.KID)
		require.NoError(t, err)
		assert.Equal(t, cimdKey.KID, remaining.KID)
	})

	t.Run("keeps grace-period fallback selection within the requested key domain", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		now := time.Now().UTC()
		tokenFallback := testSigningKeyInDomain("token-fallback", storage.KeyDomainTokenSigning, false)
		tokenFallback.ActivatesAt = now.Add(-2 * time.Minute)
		tokenPending := testSigningKeyInDomain("token-pending", storage.KeyDomainTokenSigning, true)
		tokenPending.ActivatesAt = now.Add(time.Hour)
		cimdKey := testSigningKeyInDomain("cimd-key", storage.KeyDomainCIMDClientAuthentication, false)
		cimdKey.ActivatesAt = now.Add(-time.Minute)
		require.NoError(t, repo.Create(ctx, tokenFallback))
		require.NoError(t, repo.Create(ctx, tokenPending))
		require.NoError(t, repo.Create(ctx, cimdKey))

		current, err := repo.GetCurrentInDomain(ctx, storage.KeyDomainTokenSigning)
		require.NoError(t, err)
		assert.Equal(t, tokenFallback.KID, current.KID)
	})

	t.Run("observes an empty domain while bootstrapping under the lock", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		tokenKey := testSigningKeyInDomain("token-key", storage.KeyDomainTokenSigning, true)
		require.NoError(t, repo.Create(ctx, tokenKey))

		err := repo.WithBootstrapLock(ctx, func(lockCtx context.Context) error {
			count, err := repo.CountActiveInDomain(lockCtx, storage.KeyDomainCIMDClientAuthentication)
			require.NoError(t, err)
			assert.Zero(t, count)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("rejects duplicate kids across key domains", func(t *testing.T) {
		adapter, cleanup := setupSigningKeyTestDB(t)
		defer cleanup()
		repo := NewSigningKeyRepo(adapter)
		tokenKey := testSigningKeyInDomain("shared-kid", storage.KeyDomainTokenSigning, true)
		cimdKey := testSigningKeyInDomain("shared-kid", storage.KeyDomainCIMDClientAuthentication, true)
		require.NoError(t, repo.Create(ctx, tokenKey))

		err := repo.Create(ctx, cimdKey)
		require.Error(t, err)
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	})
}
