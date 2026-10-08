package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ErrAmbiguousClientID means more than one agent matches a client ID.
var ErrAmbiguousClientID = errors.New("client ID resolves to multiple agents")

// Service encapsulates agent business logic: client ID generation, uniqueness enforcement,
// and unambiguous client ID resolution. It sits between the HTTP handler and the repository,
// owning the policies that were previously scattered across the handler and builder.
type Service struct {
	repo              ports.AgentRepository
	providerService   ServiceRequirementValidator
	logger            *slog.Logger
	multiAgentEnabled bool
}

// ServiceRequirementValidator validates that service requirements reference existing services
// with valid scopes. Satisfied by *thirdparty.ThirdpartyOAuth2ProviderService.
type ServiceRequirementValidator interface {
	ValidateServiceRequirements(ctx context.Context, reqs []storage.ServiceRequirement) error
}

// NewService creates a new agent domain service.
func NewService(
	repo ports.AgentRepository,
	providerService ServiceRequirementValidator,
	logger *slog.Logger,
	multiAgentEnabled bool,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:              repo,
		providerService:   providerService,
		logger:            logger,
		multiAgentEnabled: multiAgentEnabled,
	}
}

// ResolveID accepts a UUID or a type-scoped canonical ID.
func (s *Service) ResolveID(ctx context.Context, value string) (id.AgentID, error) {
	if parsed, err := id.ParseAgentID(value); err == nil {
		return parsed, nil
	}
	repo, ok := s.repo.(ports.AgentCanonicalIDRepository)
	if !ok {
		return id.AgentID{}, storage.NewStorageError("ResolveAgentID", storage.ErrorKindNotFound, nil, "agent not found")
	}
	agent, err := repo.GetByCanonicalID(ctx, value)
	if err != nil {
		return id.AgentID{}, err
	}
	return agent.ID, nil
}

// Create validates and persists a new agent. When clientID is nil, the agent has no
// upstream client_id (local-minting or CIMD agents). When multiAgentEnabled is false,
// client_id uniqueness is enforced for agents that have one.
func (s *Service) Create(ctx context.Context, agent *storage.Agent) error {
	if agent.ID.IsZero() {
		agent.ID = id.NewAgentID()
	}

	if err := agent.ValidateForCreate(); err != nil {
		return storage.NewStorageError("Create", storage.ErrorKindValidation, err, err.Error())
	}

	if err := s.providerService.ValidateServiceRequirements(ctx, agent.ServiceRequirements); err != nil {
		return err
	}

	if !s.multiAgentEnabled && agent.ClientID != nil {
		if err := s.checkClientIDUniqueness(ctx, *agent.ClientID, nil); err != nil {
			return err
		}
	}

	if err := s.repo.Create(ctx, agent); err != nil {
		return err
	}

	s.logger.Info("agent created", "agent_id", agent.ID, "client_id", agent.ClientID)
	return nil
}

// Update validates and persists changes to an existing agent. When clientID is nil
// and clearClientID is false, the existing client_id is preserved (ADR 017). When
// clearClientID is true, client_id is set to nil regardless of the existing value.
// When multiAgentEnabled is false, uniqueness is enforced for the effective client_id.
func (s *Service) Update(ctx context.Context, agentID id.AgentID, agent *storage.Agent, clearClientID bool) error {
	existing, err := s.repo.Get(ctx, agentID)
	if err != nil {
		return err
	}

	agent.ID = agentID
	if agent.ClearCanonicalID {
		agent.CanonicalID = nil
	} else if agent.CanonicalID == nil {
		agent.CanonicalID = existing.CanonicalID
	}
	agent.CreatedAt = existing.CreatedAt

	if agent.ClientID == nil && !clearClientID {
		agent.ClientID = existing.ClientID
	}

	if err := agent.Validate(); err != nil {
		return storage.NewStorageError("Update", storage.ErrorKindValidation, err, err.Error())
	}

	if err := s.providerService.ValidateServiceRequirements(ctx, agent.ServiceRequirements); err != nil {
		return err
	}

	if !s.multiAgentEnabled && agent.ClientID != nil {
		if err := s.checkClientIDUniqueness(ctx, *agent.ClientID, &agentID); err != nil {
			return err
		}
	}

	if err := s.repo.Update(ctx, agent); err != nil {
		return err
	}

	s.logger.Info("agent updated", "agent_id", agent.ID, "client_id", agent.ClientID)
	return nil
}

// ResolveUniqueByClientID looks up an agent by client_id and verifies that the resolution
// is unambiguous — no other agent shares the same client_id. Used by the CEL resolver
// when multi_agent_client is disabled.
func (s *Service) ResolveUniqueByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	agent, err := s.repo.GetByClientID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("resolveAgentByClientID: %w", err)
	}

	dup, err := s.repo.ExistsOtherWithClientID(ctx, clientID, &agent.ID)
	if err != nil {
		return nil, fmt.Errorf("resolveAgentByClientID: duplicate check failed: %w", err)
	}
	if dup {
		return nil, fmt.Errorf("resolveAgentByClientID: %w", ErrAmbiguousClientID)
	}

	return agent, nil
}

// Get retrieves an agent by ID.
func (s *Service) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	return s.repo.Get(ctx, agentID)
}

// Delete removes an agent by ID.
func (s *Service) Delete(ctx context.Context, agentID id.AgentID) error {
	if err := s.repo.Delete(ctx, agentID); err != nil {
		return err
	}
	s.logger.Info("agent deleted", "agent_id", agentID)
	return nil
}

// List retrieves all agents.
func (s *Service) List(ctx context.Context) ([]*storage.Agent, error) {
	return s.repo.List(ctx)
}

// checkClientIDUniqueness returns an error if another agent already uses the given client_id.
// excludeAgentID exempts one agent from the check (self-update).
func (s *Service) checkClientIDUniqueness(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) error {
	exists, err := s.repo.ExistsOtherWithClientID(ctx, clientID, excludeAgentID)
	if err != nil {
		return fmt.Errorf("failed to check client_id uniqueness: %w", err)
	}
	if exists {
		return storage.NewStorageError(
			"checkClientIDUniqueness",
			storage.ErrorKindConflict,
			nil,
			"agent with this client_id already exists",
		)
	}
	return nil
}
