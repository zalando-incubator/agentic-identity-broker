//go:build integration
// +build integration

package postgres

import (
	"bytes"
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

func TestSigningKeyRepo_Create(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-create-test"),
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-2"),
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	err := repo.Create(ctx, key)
	require.NoError(t, err)

	got, err := repo.GetByKID(ctx, kid)
	require.NoError(t, err)
	assert.Equal(t, kid, got.KID)
	assert.Equal(t, "ES256", got.Algorithm)
	assert.True(t, got.IsCurrent)
}

func TestSigningKeyRepo_PublicJWK(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	now := time.Now().UTC()
	first := []byte(`{"alg":"ES256","kid":"kid-public-first","kty":"EC"}`)
	second := []byte(`{"alg":"RS256","kid":"kid-public-second","kty":"RSA"}`)
	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-public-first"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		PublicJWK:           first,
		IsCurrent:           true,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
	require.NoError(t, repo.Create(ctx, key))

	byKID, err := repo.GetByKID(ctx, key.KID)
	require.NoError(t, err)
	assert.Equal(t, first, byKID.PublicJWK)

	current, err := repo.GetCurrent(ctx)
	require.NoError(t, err)
	assert.Equal(t, first, current.PublicJWK)

	currentKey := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-public-second"),
		Algorithm:           "RS256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		PublicJWK:           second,
		ActivatesAt:         now,
		CreatedAt:           now,
	}
	require.NoError(t, repo.CreateAndSetCurrent(ctx, currentKey))

	active, err := repo.ListActive(ctx)
	require.NoError(t, err)
	require.Len(t, active, 2)
	for _, activeKey := range active {
		switch activeKey.KID {
		case key.KID:
			assert.Equal(t, first, activeKey.PublicJWK)
		case currentKey.KID:
			assert.Equal(t, second, activeKey.PublicJWK)
		}
	}

	promoted, err := repo.SetCurrent(ctx, key.KID, now)
	require.NoError(t, err)
	assert.Equal(t, first, promoted.PublicJWK)
}

func TestSigningKeyRepo_SetPublicJWK(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	legacy := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-legacy-public"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, legacy))

	first := []byte(`{"alg":"ES256","kid":"kid-legacy-public","kty":"EC"}`)
	second := []byte(`{"alg":"RS256","kid":"kid-legacy-public","kty":"RSA"}`)
	require.NoError(t, repo.SetPublicJWK(ctx, legacy.KID, first))
	require.NoError(t, repo.SetPublicJWK(ctx, legacy.KID, second))

	stored, err := repo.GetByKID(ctx, legacy.KID)
	require.NoError(t, err)
	assert.Equal(t, first, stored.PublicJWK)
}

func TestSigningKeyRepo_SetPublicJWKConcurrentBackfill(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()
	legacy := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-concurrent-public"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, legacy))

	first := []byte(`{"alg":"ES256","kid":"kid-concurrent-public","kty":"EC"}`)
	second := []byte(`{"alg":"RS256","kid":"kid-concurrent-public","kty":"RSA"}`)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, publicJWK := range [][]byte{first, second} {
		wg.Add(1)
		go func(publicJWK []byte) {
			defer wg.Done()
			errs <- repo.SetPublicJWK(ctx, legacy.KID, publicJWK)
		}(publicJWK)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}

	stored, err := repo.GetByKID(ctx, legacy.KID)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(first, stored.PublicJWK) || bytes.Equal(second, stored.PublicJWK))
}

func TestSigningKeyRepo_GetCurrent(t *testing.T) {
	adapter, cleanup := setupSigningKeyTestDB(t)
	defer cleanup()

	repo := NewSigningKeyRepo(adapter)
	ctx := context.Background()

	key := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("encrypted-key-material"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	err := repo.Create(ctx, key)
	require.NoError(t, err)

	got, err := repo.GetCurrent(ctx)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("fallback-key"),
		IsCurrent:           false,
		ActivatesAt:         now.Add(-1 * time.Minute),
		CreatedAt:           now.Add(-2 * time.Minute),
	}
	futureCurrent := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-current-future"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("future-key"),
		IsCurrent:           true,
		ActivatesAt:         now.Add(10 * time.Minute),
		CreatedAt:           now.Add(-1 * time.Minute),
	}
	require.NoError(t, repo.Create(ctx, fallback))
	require.NoError(t, repo.Create(ctx, futureCurrent))

	got, err := repo.GetCurrent(ctx)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-list-2"),
		Algorithm:           "RS256",
		PrivateKeyEncrypted: []byte("key2"),
		IsCurrent:           false,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key1))
	require.NoError(t, repo.Create(ctx, key2))

	active, err := repo.ListActive(ctx)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key1"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	key2 := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-set-2"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key2"),
		IsCurrent:           false,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key1))
	require.NoError(t, repo.Create(ctx, key2))

	// Promote key2 using an explicit activation time supplied by the domain layer.
	activatesAt := time.Now().UTC().Add(-time.Minute).Round(time.Second)
	promoted, err := repo.SetCurrent(ctx, key2.KID, activatesAt)
	require.NoError(t, err)
	assert.Equal(t, key2.KID, promoted.KID)
	assert.True(t, promoted.IsCurrent)
	assert.WithinDuration(t, activatesAt, promoted.ActivatesAt, time.Second)

	// Verify key2 is now current
	got, err := repo.GetCurrent(ctx)
	require.NoError(t, err)
	assert.Equal(t, key2.KID, got.KID)
	assert.WithinDuration(t, activatesAt, got.ActivatesAt, time.Second)

	// Verify key1 is no longer current
	got1, err := repo.GetByKID(ctx, key1.KID)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("current-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	other := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-other"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("other-key"),
		IsCurrent:           false,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, current))
	require.NoError(t, repo.Create(ctx, other))

	err := repo.Delete(ctx, other.KID)
	require.NoError(t, err)

	// Should not appear in active list
	active, err := repo.ListActive(ctx)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("current-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	other := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-other"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("other-key"),
		IsCurrent:           false,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, current))
	require.NoError(t, repo.Create(ctx, other))

	err := repo.Delete(ctx, current.KID)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("fallback-key"),
		IsCurrent:           false,
		ActivatesAt:         now.Add(-time.Minute),
		CreatedAt:           now.Add(-2 * time.Minute),
	}
	replacement := &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID("kid-delete-future-current"),
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("future-key"),
		IsCurrent:           true,
		ActivatesAt:         now.Add(time.Hour),
		CreatedAt:           now.Add(-time.Minute),
	}
	require.NoError(t, repo.Create(ctx, fallback))
	require.NoError(t, repo.Create(ctx, replacement))

	err := repo.Delete(ctx, fallback.KID)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("last-key"),
		IsCurrent:           true,
		ActivatesAt:         time.Now().UTC(),
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key))

	err := repo.Delete(ctx, key.KID)
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
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("key"),
		IsCurrent:           true,
		CreatedAt:           time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, key))

	count, err := repo.CountActive(ctx)
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
				count, err := repo.CountActive(lockCtx)
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

	count, err := repos[0].CountActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}
