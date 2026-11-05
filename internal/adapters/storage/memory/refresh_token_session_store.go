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
var _ ports.RefreshTokenSessionRepository = (*RefreshTokenSessionStore)(nil)

// RefreshTokenSessionStore is an in-memory implementation of RefreshTokenSessionRepository.
type RefreshTokenSessionStore struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	bySignature  map[string]*refreshTokenSession
}

type refreshTokenSession struct {
	storage.RefreshTokenSession
	revoked bool
}

// NewRefreshTokenSessionStore creates a new in-memory refresh token session store.
func NewRefreshTokenSessionStore(transactions *TransactionManager) *RefreshTokenSessionStore {
	return &RefreshTokenSessionStore{
		transactions: transactions,
		bySignature:  make(map[string]*refreshTokenSession),
	}
}

func (s *RefreshTokenSessionStore) Create(ctx context.Context, session *storage.RefreshTokenSession) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(ctx, session)
}

func (s *RefreshTokenSessionStore) CreateSuccessor(ctx context.Context, session *storage.RefreshTokenSession, previousSignature string) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.bySignature[previousSignature]
	if !exists || previous.revoked || previous.UsedAt == nil || previous.RequestID != session.RequestID {
		return storage.NewStorageError("RefreshTokenSessionStore.CreateSuccessor", storage.ErrorKindNotFound, nil, "refresh chain is no longer active")
	}
	return s.createLocked(ctx, session)
}

func (s *RefreshTokenSessionStore) createLocked(ctx context.Context, session *storage.RefreshTokenSession) error {
	if _, exists := s.bySignature[session.Signature]; exists {
		return storage.NewStorageError("RefreshTokenSessionStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("refresh token session with signature %s already exists", session.Signature))
	}

	copy := &refreshTokenSession{RefreshTokenSession: *copyRefreshTokenSession(session)}
	journalEntry(ctx, s.bySignature, copy.Signature)
	s.bySignature[copy.Signature] = copy
	return nil
}

func (s *RefreshTokenSessionStore) FindBySignature(ctx context.Context, signature string) (*storage.RefreshTokenSession, error) {
	guard, gateErr := s.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.bySignature[signature]
	if !exists {
		return nil, storage.NewStorageError("RefreshTokenSessionStore.FindBySignature", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("refresh token session with signature %s not found", signature))
	}

	return copyRefreshTokenSession(&session.RefreshTokenSession), nil
}

func (s *RefreshTokenSessionStore) MarkUsed(ctx context.Context, signature string) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.bySignature[signature]
	if !exists {
		return storage.NewStorageError("RefreshTokenSessionStore.MarkUsed", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("refresh token session with signature %s not found", signature))
	}

	if session.UsedAt != nil {
		return storage.NewStorageError("RefreshTokenSessionStore.MarkUsed", storage.ErrorKindNotFound, nil,
			"refresh token session not found or already used")
	}

	now := time.Now()
	updated := *session
	updated.UsedAt = &now
	journalEntry(ctx, s.bySignature, signature)
	s.bySignature[signature] = &updated
	return nil
}

func (s *RefreshTokenSessionStore) RevokeByRequestID(ctx context.Context, requestID string) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for signature, session := range s.bySignature {
		if session.RequestID == requestID && !session.revoked {
			updated := *session
			updated.revoked = true
			if updated.UsedAt == nil {
				updated.UsedAt = &now
			}
			journalEntry(ctx, s.bySignature, signature)
			s.bySignature[signature] = &updated
		}
	}
	return nil
}

func (s *RefreshTokenSessionStore) RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	return s.revokeByAgent(ctx, agentID, &principal)
}

func (s *RefreshTokenSessionStore) RevokeByAgent(ctx context.Context, agentID id.AgentID) (int, error) {
	return s.revokeByAgent(ctx, agentID, nil)
}

func (s *RefreshTokenSessionStore) revokeByAgent(ctx context.Context, agentID id.AgentID, principal *id.Principal) (int, error) {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for signature, session := range s.bySignature {
		if session.AgentID == agentID && (principal == nil || session.Principal == *principal) && !session.revoked {
			updated := *session
			updated.revoked = true
			if updated.UsedAt == nil {
				updated.UsedAt = &now
				count++
			}
			journalEntry(ctx, s.bySignature, signature)
			s.bySignature[signature] = &updated
		}
	}
	return count, nil
}

func (s *RefreshTokenSessionStore) DeleteExpired(ctx context.Context) (int, error) {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for signature, session := range s.bySignature {
		if session.ExpiresAt.Before(now) {
			journalEntry(ctx, s.bySignature, signature)
			delete(s.bySignature, signature)
			count++
		}
	}
	return count, nil
}
