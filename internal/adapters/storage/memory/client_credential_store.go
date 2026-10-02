package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Compile-time interface check
var _ ports.ClientCredentialRepository = (*ClientCredentialStore)(nil)

// ClientCredentialStore is an in-memory implementation of ClientCredentialRepository.
type ClientCredentialStore struct {
	mu        sync.RWMutex
	refresh   *RefreshSessionStore
	byID      map[id.CredentialID]*storage.ClientCredential
	byAgentID map[id.AgentID]*storage.ClientCredential
}

// NewClientCredentialStore creates a new in-memory broker client credential store.
func NewClientCredentialStore() *ClientCredentialStore {
	return &ClientCredentialStore{
		byID:      make(map[id.CredentialID]*storage.ClientCredential),
		byAgentID: make(map[id.AgentID]*storage.ClientCredential),
	}
}

func (s *ClientCredentialStore) Create(ctx context.Context, credential *storage.ClientCredential) error {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedPut(ctx, credential, false)
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byAgentID[credential.AgentID]; exists {
		return storage.NewStorageError("ClientCredentialStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("credential for agent %s already exists", credential.AgentID))
	}
	if existing := s.byID[credential.ID]; existing != nil && existing.AgentID != credential.AgentID {
		return storage.NewStorageError("ClientCredentialStore.Create", storage.ErrorKindConflict, nil, "credential ID belongs to another agent")
	}

	cred := copyCredential(credential)
	s.byID[cred.ID] = cred
	s.byAgentID[cred.AgentID] = cred
	if s.refresh != nil {
		s.refresh.versions[credential.AgentID]++
	}
	return nil
}

func (s *ClientCredentialStore) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedGet(ctx, agentID)
	}
	unlock := s.refresh.lockRead()
	defer unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, exists := s.byAgentID[agentID]
	if !exists {
		return nil, storage.NewStorageError("ClientCredentialStore.GetByAgentID", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no credential for agent %s", agentID))
	}
	return copyCredential(cred), nil
}

// GetByClientID looks up credentials by the OAuth2 client_id string.
// Since client_id equals the agent UUID, this parses the string and delegates to GetByAgentID.
func (s *ClientCredentialStore) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.ClientCredential, error) {
	agentID, err := id.ParseAgentID(clientID.String())
	if err != nil {
		return nil, storage.NewStorageError("ClientCredentialStore.GetByClientID", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("invalid client_id %s", clientID))
	}
	return s.GetByAgentID(ctx, agentID)
}

func (s *ClientCredentialStore) Delete(ctx context.Context, agentID id.AgentID) error {
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedDelete(ctx, agentID)
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	cred, exists := s.byAgentID[agentID]
	if !exists {
		return storage.NewStorageError("ClientCredentialStore.Delete", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no credential for agent %s", agentID))
	}

	delete(s.byID, cred.ID)
	delete(s.byAgentID, agentID)
	if s.refresh != nil {
		s.refresh.versions[agentID]++
	}
	return nil
}

func (s *ClientCredentialStore) Rotate(ctx context.Context, agentID id.AgentID, newCredential *storage.ClientCredential) error {
	if newCredential == nil || newCredential.AgentID != agentID {
		return storage.NewStorageError("ClientCredentialStore.Rotate", storage.ErrorKindValidation, nil, "replacement credential must belong to same agent")
	}
	if scopeFor(ctx, s.refresh) != nil {
		return s.scopedPut(ctx, newCredential, true)
	}
	unlock := s.refresh.lockWrite()
	defer unlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.byAgentID[agentID]
	if !exists {
		return storage.NewStorageError("ClientCredentialStore.Rotate", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no existing credential for agent %s", agentID))
	}
	if existing := s.byID[newCredential.ID]; existing != nil && existing.AgentID != agentID {
		return storage.NewStorageError("ClientCredentialStore.Rotate", storage.ErrorKindConflict, nil, "credential ID belongs to another agent")
	}

	newCred := copyCredential(newCredential)
	delete(s.byID, old.ID)
	s.byID[newCred.ID] = newCred
	s.byAgentID[agentID] = newCred
	if s.refresh != nil {
		s.refresh.versions[agentID]++
	}
	return nil
}
