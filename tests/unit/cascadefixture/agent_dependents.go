package cascadefixture

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func NewAgentDependents() ports.AgentDependentRepository {
	return memory.NewAgentRepository(memory.NewTransactionManager())
}

func NewCredentialRepository() ports.ClientCredentialRepository {
	return memory.NewClientCredentialStore(memory.NewTransactionManager())
}
