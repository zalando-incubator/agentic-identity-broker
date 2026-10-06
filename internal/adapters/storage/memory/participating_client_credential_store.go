package memory

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func copyCredential(credential *storage.ClientCredential) *storage.ClientCredential {
	if credential == nil {
		return nil
	}
	copy := *credential
	if credential.RotatedAt != nil {
		at := *credential.RotatedAt
		copy.RotatedAt = &at
	}
	return &copy
}

func (s *ClientCredentialStore) scopedGet(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if err := scope.checkAgent(agentID); err != nil {
		return nil, err
	}
	credential := scope.credential(agentID)
	if credential == nil {
		return nil, storage.NewStorageError("ClientCredentialStore.GetByAgentID", storage.ErrorKindNotFound, ports.ErrNotFound, "credential not found")
	}
	return copyCredential(credential), nil
}

func (s *ClientCredentialStore) scopedPut(ctx context.Context, credential *storage.ClientCredential, rotating bool) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if err := scope.checkAgent(credential.AgentID); err != nil {
		return err
	}
	if err := credential.Validate(); err != nil {
		return err
	}
	previous := scope.credential(credential.AgentID)
	if rotating && previous == nil {
		return storage.NewStorageError("ClientCredentialStore.Rotate", storage.ErrorKindNotFound, ports.ErrNotFound, "credential not found")
	}
	if !rotating && previous != nil {
		return storage.NewStorageError("ClientCredentialStore.Create", storage.ErrorKindConflict, nil, "credential already exists")
	}
	if sameID := s.byID[credential.ID]; sameID != nil && sameID.AgentID != credential.AgentID {
		if other := scope.credential(sameID.AgentID); other != nil && other.ID == credential.ID {
			return storage.NewStorageError("ClientCredentialStore.Create", storage.ErrorKindConflict, nil, "credential ID belongs to another agent")
		}
	}
	if scope.credentials == nil {
		scope.credentials = make(map[id.AgentID]*storage.ClientCredential)
	}
	scope.touch(credential.AgentID)
	scope.credentials[credential.AgentID] = copyCredential(credential)
	return nil
}

func (s *ClientCredentialStore) scopedDelete(ctx context.Context, agentID id.AgentID) error {
	scope := scopeFor(ctx, s.refresh)
	s.refresh.mu.RLock()
	defer s.refresh.mu.RUnlock()
	if err := scope.checkAgent(agentID); err != nil {
		return err
	}
	if scope.credential(agentID) == nil {
		return storage.NewStorageError("ClientCredentialStore.Delete", storage.ErrorKindNotFound, ports.ErrNotFound, "credential not found")
	}
	if scope.credentials == nil {
		scope.credentials = make(map[id.AgentID]*storage.ClientCredential)
	}
	scope.touch(agentID)
	scope.credentials[agentID] = nil
	return nil
}
