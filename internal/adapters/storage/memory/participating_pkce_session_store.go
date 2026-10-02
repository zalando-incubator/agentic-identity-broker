package memory

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (scope *memoryScope) pkceOwner(signature string) id.AgentID {
	if owner := scope.pkceOwners[signature]; !owner.IsZero() {
		return owner
	}
	if owner := scope.store.pkce.owners[signature]; !owner.IsZero() {
		return owner
	}
	var agentID id.AgentID
	scope.eachCode(func(code *storage.AuthorizationCode) {
		if code.CodeHash == signature {
			agentID = code.AgentID
		}
	})
	return agentID
}

func (scope *memoryScope) eachPKCE(visit func(*storage.PKCESession)) {
	for key, original := range scope.store.pkce.data {
		if session, changed := scope.pkce[key]; changed {
			if session != nil {
				visit(session)
			}
		} else {
			visit(original)
		}
	}
	for key, session := range scope.pkce {
		if _, committed := scope.store.pkce.data[key]; !committed && session != nil {
			visit(session)
		}
	}
}

func (s *PKCESessionStore) scopedCreate(ctx context.Context, session *storage.PKCESession) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	owner := scope.pkceOwner(session.Signature)
	if owner.IsZero() {
		owner = scope.agentID
	}
	if err := scope.checkAgent(owner); err != nil {
		return err
	}
	if scope.pkceSession(session.Signature) != nil {
		return storage.NewStorageError("PKCESessionStore.Create", storage.ErrorKindConflict, nil, "PKCE signature already exists")
	}
	if scope.pkce == nil {
		scope.pkce = make(map[string]*storage.PKCESession)
	}
	if scope.pkceOwners == nil {
		scope.pkceOwners = make(map[string]id.AgentID)
	}
	scope.pkceOwners[session.Signature] = owner
	copy := *session
	scope.touch(owner)
	scope.pkce[session.Signature] = &copy
	return nil
}

func (s *PKCESessionStore) scopedFind(ctx context.Context, signature string) (*storage.PKCESession, error) {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if err := scope.checkAgent(scope.pkceOwner(signature)); err != nil {
		return nil, err
	}
	session := scope.pkceSession(signature)
	if session == nil {
		return nil, storage.NewStorageError("PKCESessionStore.FindBySignature", storage.ErrorKindNotFound, ports.ErrNotFound, "PKCE session not found")
	}
	copy := *session
	return &copy, nil
}

func (s *PKCESessionStore) scopedDelete(ctx context.Context, signature string) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	owner := scope.pkceOwner(signature)
	if err := scope.checkAgent(owner); err != nil {
		return err
	}
	if scope.pkceSession(signature) == nil {
		return storage.NewStorageError("PKCESessionStore.Delete", storage.ErrorKindNotFound, ports.ErrNotFound, "PKCE session not found")
	}
	if scope.pkce == nil {
		scope.pkce = make(map[string]*storage.PKCESession)
	}
	scope.touch(owner)
	scope.pkce[signature] = nil
	return nil
}

func (s *PKCESessionStore) scopedDeleteExpired(ctx context.Context) (int, error) {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if scope.closed {
		return 0, errors.New("authorization transaction is closed")
	}
	count := 0
	now := time.Now()
	scope.eachPKCE(func(session *storage.PKCESession) {
		if session.ExpiresAt.Before(now) {
			owner := scope.pkceOwner(session.Signature)
			if !owner.IsZero() && (scope.agentID.IsZero() || scope.agentID == owner) {
				if scope.pkce == nil {
					scope.pkce = make(map[string]*storage.PKCESession)
				}
				scope.touch(owner)
				scope.pkce[session.Signature] = nil
				count++
			}
		}
	})
	return count, nil
}

func (s *PKCESessionStore) bumpAgentVersion(signature string) {
	if s.refresh == nil {
		return
	}
	if owner := s.owners[signature]; !owner.IsZero() {
		s.refresh.versions[owner]++
	} else if code := s.refresh.codes.byCodeHash[signature]; code != nil {
		s.refresh.versions[code.AgentID]++
	}
}
