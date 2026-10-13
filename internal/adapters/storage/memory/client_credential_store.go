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
	mu           sync.RWMutex
	transactions *TransactionManager
	byID         map[id.CredentialID]*storage.ClientCredential
	byAgentID    map[id.AgentID]*storage.ClientCredential
}

// NewClientCredentialStore creates a new in-memory broker client credential store.
func NewClientCredentialStore(transactions *TransactionManager) *ClientCredentialStore {
	return &ClientCredentialStore{
		transactions: transactions,
		byID:         make(map[id.CredentialID]*storage.ClientCredential),
		byAgentID:    make(map[id.AgentID]*storage.ClientCredential),
	}
}

func (s *ClientCredentialStore) Create(ctx context.Context, credential *storage.ClientCredential) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.byAgentID[credential.AgentID]; exists {
		return storage.NewStorageError("ClientCredentialStore.Create", storage.ErrorKindConflict, nil,
			fmt.Sprintf("credential for agent %s already exists", credential.AgentID))
	}

	cred := copyClientCredential(credential)
	journalEntry(ctx, s.byID, cred.ID)
	s.byID[cred.ID] = cred
	journalEntry(ctx, s.byAgentID, cred.AgentID)
	s.byAgentID[cred.AgentID] = cred
	return nil
}

func (s *ClientCredentialStore) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	guard, gateErr := s.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	s.mu.RLock()
	defer s.mu.RUnlock()

	cred, exists := s.byAgentID[agentID]
	if !exists {
		return nil, storage.NewStorageError("ClientCredentialStore.GetByAgentID", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no credential for agent %s", agentID))
	}
	return copyClientCredential(cred), nil
}

// GetByClientID looks up credentials by the OAuth2 client_id string.
// Since client_id equals the agent UUID, this parses the string and delegates to GetByAgentID.
func (s *ClientCredentialStore) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.ClientCredential, error) {
	ctx, guard, gateErr := s.transactions.enter(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	agentID, err := id.ParseAgentID(clientID.String())
	if err != nil {
		return nil, storage.NewStorageError("ClientCredentialStore.GetByClientID", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("invalid client_id %s", clientID))
	}
	return s.GetByAgentID(ctx, agentID)
}

func (s *ClientCredentialStore) Delete(ctx context.Context, agentID id.AgentID) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	cred, exists := s.byAgentID[agentID]
	if !exists {
		return storage.NewStorageError("ClientCredentialStore.Delete", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no credential for agent %s", agentID))
	}

	journalEntry(ctx, s.byID, cred.ID)
	delete(s.byID, cred.ID)
	journalEntry(ctx, s.byAgentID, agentID)
	delete(s.byAgentID, agentID)
	return nil
}

func (s *ClientCredentialStore) Rotate(ctx context.Context, agentID id.AgentID, newCredential *storage.ClientCredential) error {
	guard, gateErr := s.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.byAgentID[agentID]
	if !exists {
		return storage.NewStorageError("ClientCredentialStore.Rotate", storage.ErrorKindNotFound, nil,
			fmt.Sprintf("no existing credential for agent %s", agentID))
	}

	newCred := copyClientCredential(newCredential)
	journalEntry(ctx, s.byID, old.ID)
	delete(s.byID, old.ID)
	journalEntry(ctx, s.byID, newCred.ID)
	s.byID[newCred.ID] = newCred
	journalEntry(ctx, s.byAgentID, agentID)
	s.byAgentID[agentID] = newCred
	return nil
}
