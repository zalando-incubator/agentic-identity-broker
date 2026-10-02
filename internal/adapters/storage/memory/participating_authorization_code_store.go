package memory

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func copyAuthorizationCode(code *storage.AuthorizationCode) *storage.AuthorizationCode {
	if code == nil {
		return nil
	}
	copy := *code
	if code.Email != nil {
		email := *code.Email
		copy.Email = &email
	}
	if code.UsedAt != nil {
		at := *code.UsedAt
		copy.UsedAt = &at
	}
	return &copy
}

func (scope *memoryScope) eachCode(visit func(*storage.AuthorizationCode)) {
	for key, original := range scope.store.codes.byID {
		if code, staged := scope.codes[key]; staged {
			if code != nil {
				visit(code)
			}
		} else {
			visit(original)
		}
	}
	for key, code := range scope.codes {
		if _, present := scope.store.codes.byID[key]; !present && code != nil {
			visit(code)
		}
	}
}

func (s *AuthorizationCodeStore) scopedCreate(ctx context.Context, code *storage.AuthorizationCode) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if err := scope.checkAgent(code.AgentID); err != nil {
		return err
	}
	if err := code.Validate(); err != nil {
		return err
	}
	collision := scope.code(code.ID) != nil
	scope.eachCode(func(other *storage.AuthorizationCode) {
		if other.CodeHash == code.CodeHash {
			collision = true
		}
	})
	if collision {
		return storage.NewStorageError("AuthorizationCodeStore.Create", storage.ErrorKindConflict, nil, "authorization code exists")
	}
	if scope.codes == nil {
		scope.codes = make(map[id.AuthorizationCodeID]*storage.AuthorizationCode)
	}
	scope.touch(code.AgentID)
	scope.codes[code.ID] = copyAuthorizationCode(code)
	return nil
}

func (s *AuthorizationCodeStore) scopedFind(ctx context.Context, codeHash string) (*storage.AuthorizationCode, error) {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if scope.closed {
		return nil, storage.NewStorageError("FindCode", storage.ErrorKindValidation, nil, "authorization transaction is closed")
	}
	var found *storage.AuthorizationCode
	scope.eachCode(func(code *storage.AuthorizationCode) {
		if code.CodeHash == codeHash {
			found = code
		}
	})
	if found == nil || !found.ExpiresAt.After(time.Now()) {
		return nil, storage.NewStorageError("AuthorizationCodeStore.FindByCodeHash", storage.ErrorKindNotFound, ports.ErrNotFound, "authorization code not found")
	}
	if err := scope.checkAgent(found.AgentID); err != nil {
		return nil, err
	}
	scope.observe(found.AgentID)
	return copyAuthorizationCode(found), nil
}

func (s *AuthorizationCodeStore) scopedMarkUsed(ctx context.Context, codeID id.AuthorizationCodeID) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	code := scope.code(codeID)
	if code == nil {
		return storage.NewStorageError("AuthorizationCodeStore.MarkUsed", storage.ErrorKindNotFound, ports.ErrNotFound, "authorization code not found")
	}
	if err := scope.checkAgent(code.AgentID); err != nil {
		return err
	}
	changed := copyAuthorizationCode(code)
	at := time.Now().UTC()
	changed.UsedAt = &at
	if scope.codes == nil {
		scope.codes = make(map[id.AuthorizationCodeID]*storage.AuthorizationCode)
	}
	scope.touch(code.AgentID)
	scope.codes[codeID] = changed
	return nil
}

func (s *AuthorizationCodeStore) scopedDeleteExpired(ctx context.Context) (int, error) {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if scope.closed {
		return 0, storage.NewStorageError("DeleteCodes", storage.ErrorKindValidation, nil, "authorization transaction is closed")
	}
	now := time.Now()
	count := 0
	scope.eachCode(func(code *storage.AuthorizationCode) {
		if code.ExpiresAt.Before(now) && (scope.agentID.IsZero() || scope.agentID == code.AgentID) {
			if scope.codes == nil {
				scope.codes = make(map[id.AuthorizationCodeID]*storage.AuthorizationCode)
			}
			scope.touch(code.AgentID)
			scope.codes[code.ID] = nil
			count++
		}
	})
	return count, nil
}
