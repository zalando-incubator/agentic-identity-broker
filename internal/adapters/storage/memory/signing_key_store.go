package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface checks
var _ ports.SigningKeyRepository = (*SigningKeyStore)(nil)
var _ ports.SigningKeyBootstrapCoordinator = (*SigningKeyStore)(nil)

type bootstrapLockContextKey struct{}

// SigningKeyStore is an in-memory implementation of SigningKeyRepository.
type SigningKeyStore struct {
	mu          sync.RWMutex
	bootstrapCh chan struct{}
	byID        map[id.SigningKeyID]*storage.SigningKey
	byKID       map[id.KeyID]*storage.SigningKey
}

// NewSigningKeyStore creates a new in-memory signing key store.
func NewSigningKeyStore() *SigningKeyStore {
	return &SigningKeyStore{
		bootstrapCh: make(chan struct{}, 1),
		byID:        make(map[id.SigningKeyID]*storage.SigningKey),
		byKID:       make(map[id.KeyID]*storage.SigningKey),
	}
}

func cloneSigningKey(key *storage.SigningKey) *storage.SigningKey {
	clone := *key
	clone.PrivateKeyEncrypted = append([]byte(nil), key.PrivateKeyEncrypted...)
	clone.PublicJWK = append([]byte(nil), key.PublicJWK...)
	return &clone
}

func bootstrapWriteLockHeld(ctx context.Context) bool {
	held, _ := ctx.Value(bootstrapLockContextKey{}).(bool)
	return held
}

func (s *SigningKeyStore) Create(ctx context.Context, key *storage.SigningKey) error {
	if bootstrapWriteLockHeld(ctx) {
		return s.createLocked(key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(key)
}

func (s *SigningKeyStore) createLocked(key *storage.SigningKey) error {
	if _, exists := s.byKID[key.KID]; exists {
		return storage.NewStorageError("SigningKeyStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("signing key with kid %s already exists", key.KID))
	}

	clone := cloneSigningKey(key)
	s.byID[clone.ID] = clone
	s.byKID[clone.KID] = clone
	return nil
}

func (s *SigningKeyStore) CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error {
	if bootstrapWriteLockHeld(ctx) {
		return s.createAndSetCurrentLocked(key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createAndSetCurrentLocked(key)
}

func (s *SigningKeyStore) createAndSetCurrentLocked(key *storage.SigningKey) error {
	if _, exists := s.byKID[key.KID]; exists {
		return storage.NewStorageError("SigningKeyStore.CreateAndSetCurrent", storage.ErrorKindConflict, nil,
			fmt.Sprintf("signing key with kid %s already exists", key.KID))
	}

	for _, existing := range s.byID {
		existing.IsCurrent = false
	}

	clone := cloneSigningKey(key)
	clone.IsCurrent = true
	s.byID[clone.ID] = clone
	s.byKID[clone.KID] = clone
	return nil
}

func (s *SigningKeyStore) GetByKID(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.getByKIDLocked(kid)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getByKIDLocked(kid)
}

func (s *SigningKeyStore) getByKIDLocked(kid id.KeyID) (*storage.SigningKey, error) {
	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil {
		return nil, storage.NewStorageError("SigningKeyStore.GetByKID", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}
	return cloneSigningKey(key), nil
}

// GetCurrent returns the signing key to use for token issuance. It prefers the key flagged
// as is_current provided its activates_at has passed. If the current key is still in its
// grace period, it falls back to the most recently activated key.
func (s *SigningKeyStore) GetCurrent(ctx context.Context) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.getCurrentLocked()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getCurrentLocked()
}

func (s *SigningKeyStore) getCurrentLocked() (*storage.SigningKey, error) {
	current := currentUsableSigningKey(s.byID, time.Now())
	if current == nil {
		return nil, storage.NewStorageError("SigningKeyStore.GetCurrent", storage.ErrorKindNotFound, nil, "no current signing key")
	}
	return cloneSigningKey(current), nil
}

func currentUsableSigningKey(keys map[id.SigningKeyID]*storage.SigningKey, now time.Time) *storage.SigningKey {
	var best *storage.SigningKey
	for _, key := range keys {
		if key.RemovedAt != nil || key.ActivatesAt.After(now) {
			continue
		}
		if best == nil || (!best.IsCurrent && key.IsCurrent) || (best.IsCurrent == key.IsCurrent && key.ActivatesAt.After(best.ActivatesAt)) {
			best = key
		}
	}
	return best
}

func (s *SigningKeyStore) ListActive(ctx context.Context) ([]*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.listActiveLocked(), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listActiveLocked(), nil
}

func (s *SigningKeyStore) listActiveLocked() []*storage.SigningKey {
	result := make([]*storage.SigningKey, 0, len(s.byID))
	for _, key := range s.byID {
		if key.RemovedAt == nil {
			result = append(result, cloneSigningKey(key))
		}
	}
	return result
}

// SetCurrent promotes a key to be the current signing key using the domain-supplied
// activation timestamp.
func (s *SigningKeyStore) SetCurrent(ctx context.Context, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.setCurrentLocked(kid, activatesAt)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setCurrentLocked(kid, activatesAt)
}

func (s *SigningKeyStore) setCurrentLocked(kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	target, exists := s.byKID[kid]
	if !exists || target.RemovedAt != nil {
		return nil, storage.NewStorageError("SigningKeyStore.SetCurrent", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}

	for _, key := range s.byID {
		key.IsCurrent = false
	}
	target.IsCurrent = true
	target.ActivatesAt = activatesAt
	return cloneSigningKey(target), nil
}

func (s *SigningKeyStore) SetPublicJWK(ctx context.Context, kid id.KeyID, publicJWK []byte) error {
	if !bootstrapWriteLockHeld(ctx) {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	return s.setPublicJWKLocked(kid, publicJWK)
}

func (s *SigningKeyStore) setPublicJWKLocked(kid id.KeyID, publicJWK []byte) error {
	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil {
		return storage.NewStorageError("SigningKeyStore.SetPublicJWK", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}
	if key.PublicJWK == nil {
		key.PublicJWK = append([]byte(nil), publicJWK...)
	}
	return nil
}

func (s *SigningKeyStore) Delete(ctx context.Context, kid id.KeyID) error {
	if bootstrapWriteLockHeld(ctx) {
		return s.deleteLocked(kid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteLocked(kid)
}

func (s *SigningKeyStore) deleteLocked(kid id.KeyID) error {
	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil {
		return storage.NewStorageError("SigningKeyStore.Delete", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}

	activeCount := 0
	for _, existing := range s.byID {
		if existing.RemovedAt == nil {
			activeCount++
		}
	}
	if activeCount <= 1 {
		return ports.ErrLastActiveKey
	}
	if key.IsCurrent {
		return ports.ErrCurrentKey
	}

	now := time.Now()
	if current := currentUsableSigningKey(s.byID, now); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}

	key.RemovedAt = &now
	return nil
}

func (s *SigningKeyStore) WithBootstrapLock(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case s.bootstrapCh <- struct{}{}:
		defer func() { <-s.bootstrapCh }()
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return fn(context.WithValue(ctx, bootstrapLockContextKey{}, true))
}

func (s *SigningKeyStore) CountActive(ctx context.Context) (int, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.countActiveLocked(), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countActiveLocked(), nil
}

func (s *SigningKeyStore) countActiveLocked() int {
	count := 0
	for _, key := range s.byID {
		if key.RemovedAt == nil {
			count++
		}
	}
	return count
}
