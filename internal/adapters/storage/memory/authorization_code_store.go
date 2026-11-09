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

// Compile-time interface check
var _ ports.AuthorizationCodeRepository = (*AuthorizationCodeStore)(nil)

// AuthorizationCodeStore is an in-memory implementation of AuthorizationCodeRepository.
type AuthorizationCodeStore struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	byID         map[id.AuthorizationCodeID]*storage.AuthorizationCode
	byCodeHash   map[string]*storage.AuthorizationCode
}

// NewAuthorizationCodeStore creates a new in-memory authorization code store.
func NewAuthorizationCodeStore(transactions *TransactionManager) *AuthorizationCodeStore {
	return &AuthorizationCodeStore{
		transactions: transactions,
		byID:         make(map[id.AuthorizationCodeID]*storage.AuthorizationCode),
		byCodeHash:   make(map[string]*storage.AuthorizationCode),
	}
}

func (s *AuthorizationCodeStore) Create(ctx context.Context, code *storage.AuthorizationCode) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byCodeHash[code.CodeHash]; exists {
		return storage.NewStorageError("AuthorizationCodeStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("authorization code with hash %s already exists", code.CodeHash))
	}

	c := copyAuthorizationCode(code)
	journalEntry(ctx, s.byID, c.ID)
	s.byID[c.ID] = c
	journalEntry(ctx, s.byCodeHash, c.CodeHash)
	s.byCodeHash[c.CodeHash] = c
	return nil
}

func (s *AuthorizationCodeStore) FindByCodeHash(ctx context.Context, codeHash string) (*storage.AuthorizationCode, error) {
	guard, gateErr := s.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	s.mu.RLock()
	defer s.mu.RUnlock()

	code, exists := s.byCodeHash[codeHash]
	if !exists || !code.ExpiresAt.After(time.Now()) {
		return nil, storage.NewStorageError("AuthorizationCodeStore.FindByCodeHash", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("authorization code with hash %s not found", codeHash))
	}
	return copyAuthorizationCode(code), nil
}

func (s *AuthorizationCodeStore) MarkUsed(ctx context.Context, codeID id.AuthorizationCodeID) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	code, exists := s.byID[codeID]
	if !exists {
		return storage.NewStorageError("AuthorizationCodeStore.MarkUsed", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("authorization code %s not found", codeID))
	}

	now := time.Now()
	updated := *code
	updated.UsedAt = &now
	journalEntry(ctx, s.byID, updated.ID)
	s.byID[updated.ID] = &updated
	journalEntry(ctx, s.byCodeHash, updated.CodeHash)
	s.byCodeHash[updated.CodeHash] = &updated
	return nil
}

func (s *AuthorizationCodeStore) DeleteExpired(ctx context.Context) (int, error) {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for hash, code := range s.byCodeHash {
		if code.ExpiresAt.Before(now) {
			journalEntry(ctx, s.byID, code.ID)
			delete(s.byID, code.ID)
			journalEntry(ctx, s.byCodeHash, hash)
			delete(s.byCodeHash, hash)
			count++
		}
	}
	return count, nil
}
