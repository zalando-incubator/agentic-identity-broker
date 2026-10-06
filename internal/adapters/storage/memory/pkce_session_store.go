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
var _ ports.PKCESessionRepository = (*PKCESessionStore)(nil)

// PKCESessionStore is an in-memory implementation of PKCESessionRepository.
type PKCESessionStore struct {
	mu      sync.RWMutex
	refresh *RefreshSessionStore
	data    map[string]*storage.PKCESession
	owners  map[string]id.AgentID
}

// NewPKCESessionStore creates a new in-memory PKCE session store.
func NewPKCESessionStore() *PKCESessionStore {
	return &PKCESessionStore{
		data:   make(map[string]*storage.PKCESession),
		owners: make(map[string]id.AgentID),
	}
}

func (s *PKCESessionStore) Create(ctx context.Context, session *storage.PKCESession) error {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedCreate(ctx, session)
	}
	if s.refresh != nil {
		unlock := s.refresh.lockRead()
		code := s.refresh.codes.byCodeHash[session.Signature]
		var owner id.AgentID
		if code != nil {
			owner = code.AgentID
		}
		unlock()
		if !owner.IsZero() {
			gate, err := s.refresh.lockAgent(ctx, owner)
			if err != nil {
				return err
			}
			defer func() { gate <- struct{}{} }()
		}
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[session.Signature]; exists {
		return storage.NewStorageError("PKCESessionStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("PKCE session with signature %s already exists", session.Signature))
	}

	c := *session
	s.data[c.Signature] = &c
	if s.refresh != nil {
		if code := s.refresh.codes.byCodeHash[session.Signature]; code != nil {
			s.owners[session.Signature] = code.AgentID
		}
	}
	s.bumpAgentVersion(session.Signature)
	return nil
}

func (s *PKCESessionStore) FindBySignature(ctx context.Context, signature string) (*storage.PKCESession, error) {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedFind(ctx, signature)
	}
	unlock := s.refresh.lockRead()
	defer unlock()
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
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedDelete(ctx, signature)
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.data[signature]; !exists {
		return storage.NewStorageError("PKCESessionStore.Delete", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("PKCE session with signature %s not found", signature))
	}
	s.bumpAgentVersion(signature)
	delete(s.data, signature)
	delete(s.owners, signature)
	return nil
}

func (s *PKCESessionStore) DeleteExpired(ctx context.Context) (int, error) {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedDeleteExpired(ctx)
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for sig, session := range s.data {
		if session.ExpiresAt.Before(now) {
			s.bumpAgentVersion(sig)
			delete(s.data, sig)
			delete(s.owners, sig)
			count++
		}
	}
	return count, nil
}
