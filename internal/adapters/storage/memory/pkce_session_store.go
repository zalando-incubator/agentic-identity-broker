package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.PKCESessionRepository = (*PKCESessionStore)(nil)

// PKCESessionStore is an in-memory implementation of PKCESessionRepository.
type PKCESessionStore struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	data         map[string]*storage.PKCESession
}

// NewPKCESessionStore creates a new in-memory PKCE session store.
func NewPKCESessionStore(transactions *TransactionManager) *PKCESessionStore {
	return &PKCESessionStore{
		transactions: transactions,
		data:         make(map[string]*storage.PKCESession),
	}
}

func (s *PKCESessionStore) Create(ctx context.Context, session *storage.PKCESession) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[session.Signature]; exists {
		return storage.NewStorageError("PKCESessionStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("PKCE session with signature %s already exists", session.Signature))
	}

	c := *session
	journalEntry(ctx, s.data, c.Signature)
	s.data[c.Signature] = &c
	return nil
}

func (s *PKCESessionStore) FindBySignature(ctx context.Context, signature string) (*storage.PKCESession, error) {
	guard, gateErr := s.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.data[signature]
	if !exists {
		return nil, storage.NewStorageError("PKCESessionStore.FindBySignature", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("PKCE session with signature %s not found", signature))
	}
	result := *session
	return &result, nil
}

func (s *PKCESessionStore) Delete(ctx context.Context, signature string) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[signature]; !exists {
		return storage.NewStorageError("PKCESessionStore.Delete", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("PKCE session with signature %s not found", signature))
	}
	journalEntry(ctx, s.data, signature)
	delete(s.data, signature)
	return nil
}

func (s *PKCESessionStore) DeleteExpired(ctx context.Context) (int, error) {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for sig, session := range s.data {
		if session.ExpiresAt.Before(now) {
			journalEntry(ctx, s.data, sig)
			delete(s.data, sig)
			count++
		}
	}
	return count, nil
}
