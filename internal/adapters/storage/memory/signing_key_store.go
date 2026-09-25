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
	if err := key.KeyDomain.Validate(); err != nil {
		return storage.NewStorageError("SigningKeyStore.Create", storage.ErrorKindValidation, err, "key_domain is invalid")
	}
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
	if err := key.KeyDomain.Validate(); err != nil {
		return storage.NewStorageError("SigningKeyStore.CreateAndSetCurrent", storage.ErrorKindValidation, err, "key_domain is invalid")
	}
	if _, exists := s.byKID[key.KID]; exists {
		return storage.NewStorageError("SigningKeyStore.CreateAndSetCurrent", storage.ErrorKindConflict, nil,
			fmt.Sprintf("signing key with kid %s already exists", key.KID))
	}

	for _, existing := range s.byID {
		if existing.KeyDomain == key.KeyDomain {
			existing.IsCurrent = false
		}
	}

	clone := cloneSigningKey(key)
	clone.IsCurrent = true
	s.byID[clone.ID] = clone
	s.byKID[clone.KID] = clone
	return nil
}

func (s *SigningKeyStore) getByKIDInDomainLocked(domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil || key.KeyDomain != domain {
		return nil, storage.NewStorageError("SigningKeyStore.GetByKIDInDomain", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}
	return cloneSigningKey(key), nil
}

func (s *SigningKeyStore) GetByKIDInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.getByKIDInDomainLocked(domain, kid)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getByKIDInDomainLocked(domain, kid)
}

// GetCurrent returns the signing key to use for token issuance. It prefers the key flagged
// as is_current provided its activates_at has passed. If the current key is still in its
// grace period, it falls back to the most recently activated key.

func (s *SigningKeyStore) GetCurrentInDomain(ctx context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.getCurrentInDomainLocked(domain)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getCurrentInDomainLocked(domain)
}

func (s *SigningKeyStore) getCurrentInDomainLocked(domain storage.KeyDomain) (*storage.SigningKey, error) {
	current := currentUsableSigningKeyInDomain(s.byID, domain, time.Now())
	if current == nil {
		return nil, storage.NewStorageError("SigningKeyStore.GetCurrentInDomain", storage.ErrorKindNotFound, nil, "no current signing key")
	}
	return cloneSigningKey(current), nil
}

func currentUsableSigningKeyInDomain(keys map[id.SigningKeyID]*storage.SigningKey, domain storage.KeyDomain, now time.Time) *storage.SigningKey {
	var best *storage.SigningKey
	for _, key := range keys {
		if key.KeyDomain != domain || key.RemovedAt != nil || key.ActivatesAt.After(now) {
			continue
		}
		if best == nil || (!best.IsCurrent && key.IsCurrent) || (best.IsCurrent == key.IsCurrent && key.ActivatesAt.After(best.ActivatesAt)) {
			best = key
		}
	}
	return best
}

func (s *SigningKeyStore) listActiveInDomainLocked(domain storage.KeyDomain) []*storage.SigningKey {
	result := make([]*storage.SigningKey, 0, len(s.byID))
	for _, key := range s.byID {
		if key.KeyDomain == domain && key.RemovedAt == nil {
			result = append(result, cloneSigningKey(key))
		}
	}
	return result
}

func (s *SigningKeyStore) ListActiveInDomain(ctx context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.listActiveInDomainLocked(domain), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listActiveInDomainLocked(domain), nil
}

// SetCurrent promotes a key to be the current signing key using the domain-supplied
// activation timestamp.

func (s *SigningKeyStore) setCurrentInDomainLocked(domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	target, exists := s.byKID[kid]
	if !exists || target.RemovedAt != nil || target.KeyDomain != domain {
		return nil, storage.NewStorageError("SigningKeyStore.SetCurrentInDomain", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}

	for _, key := range s.byID {
		if key.KeyDomain == domain {
			key.IsCurrent = false
		}
	}
	target.IsCurrent = true
	target.ActivatesAt = activatesAt
	return cloneSigningKey(target), nil
}

func (s *SigningKeyStore) SetCurrentInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.setCurrentInDomainLocked(domain, kid, activatesAt)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setCurrentInDomainLocked(domain, kid, activatesAt)
}

func (s *SigningKeyStore) deleteInDomainLocked(domain storage.KeyDomain, kid id.KeyID) error {
	key, exists := s.byKID[kid]
	if !exists || key.RemovedAt != nil || key.KeyDomain != domain {
		return storage.NewStorageError("SigningKeyStore.DeleteInDomain", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("signing key with kid %s not found", kid))
	}

	activeCount := 0
	for _, existing := range s.byID {
		if existing.KeyDomain == domain && existing.RemovedAt == nil {
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
	if current := currentUsableSigningKeyInDomain(s.byID, domain, now); current != nil && current.KID == kid {
		return ports.ErrEffectiveCurrentKey
	}

	key.RemovedAt = &now
	return nil
}

func (s *SigningKeyStore) DeleteInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) error {
	if bootstrapWriteLockHeld(ctx) {
		return s.deleteInDomainLocked(domain, kid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteInDomainLocked(domain, kid)
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

func (s *SigningKeyStore) countActiveInDomainLocked(domain storage.KeyDomain) int {
	count := 0
	for _, key := range s.byID {
		if key.KeyDomain == domain && key.RemovedAt == nil {
			count++
		}
	}
	return count
}

func (s *SigningKeyStore) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	if bootstrapWriteLockHeld(ctx) {
		return s.countActiveInDomainLocked(domain), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.countActiveInDomainLocked(domain), nil
}
