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
	mu          sync.RWMutex
	bySignature map[string]*refreshTokenSession
}

type refreshTokenSession struct {
	storage.RefreshTokenSession
	revoked bool
}

// NewRefreshTokenSessionStore creates a new in-memory refresh token session store.
func NewRefreshTokenSessionStore() *RefreshTokenSessionStore {
	return &RefreshTokenSessionStore{
		bySignature: make(map[string]*refreshTokenSession),
	}
}

func (s *RefreshTokenSessionStore) Create(_ context.Context, session *storage.RefreshTokenSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked(session)
}

func (s *RefreshTokenSessionStore) CreateSuccessor(_ context.Context, session *storage.RefreshTokenSession, previousSignature string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, exists := s.bySignature[previousSignature]
	if !exists || previous.revoked || previous.UsedAt == nil || previous.RequestID != session.RequestID {
		return storage.NewStorageError("RefreshTokenSessionStore.CreateSuccessor", storage.ErrorKindNotFound, nil, "refresh chain is no longer active")
	}
	return s.createLocked(session)
}

func (s *RefreshTokenSessionStore) createLocked(session *storage.RefreshTokenSession) error {
	if _, exists := s.bySignature[session.Signature]; exists {
		return storage.NewStorageError("RefreshTokenSessionStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("refresh token session with signature %s already exists", session.Signature))
	}

	copy := refreshTokenSession{RefreshTokenSession: *session}
	if session.GrantID != nil {
		grantID := *session.GrantID
		copy.GrantID = &grantID
	}
	s.bySignature[copy.Signature] = &copy
	return nil
}

func (s *RefreshTokenSessionStore) FindBySignature(_ context.Context, signature string) (*storage.RefreshTokenSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.bySignature[signature]
	if !exists {
		return nil, storage.NewStorageError("RefreshTokenSessionStore.FindBySignature", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("refresh token session with signature %s not found", signature))
	}

	copy := session.RefreshTokenSession
	if session.GrantID != nil {
		grantID := *session.GrantID
		copy.GrantID = &grantID
	}
	return &copy, nil
}

func (s *RefreshTokenSessionStore) MarkUsed(_ context.Context, signature string) error {
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
	session.UsedAt = &now
	return nil
}

func (s *RefreshTokenSessionStore) RevokeByRequestID(_ context.Context, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, session := range s.bySignature {
		if session.RequestID == requestID {
			session.revoked = true
			if session.UsedAt == nil {
				session.UsedAt = &now
			}
		}
	}
	return nil
}

func (s *RefreshTokenSessionStore) RevokeByPrincipalAndAgent(_ context.Context, principal id.Principal, agentID id.AgentID) (int, error) {
	return s.revokeByAgent(agentID, &principal), nil
}

func (s *RefreshTokenSessionStore) RevokeByAgent(_ context.Context, agentID id.AgentID) (int, error) {
	return s.revokeByAgent(agentID, nil), nil
}

func (s *RefreshTokenSessionStore) revokeByAgent(agentID id.AgentID, principal *id.Principal) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for _, session := range s.bySignature {
		if session.AgentID == agentID && (principal == nil || session.Principal == *principal) {
			session.revoked = true
			if session.UsedAt == nil {
				session.UsedAt = &now
				count++
			}
		}
	}
	return count
}

func (s *RefreshTokenSessionStore) DeleteExpired(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for signature, session := range s.bySignature {
		if session.ExpiresAt.Before(now) {
			delete(s.bySignature, signature)
			count++
		}
	}
	return count, nil
}
