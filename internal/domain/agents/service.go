package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ErrAmbiguousClientID means more than one agent matches a client ID.
var ErrAmbiguousClientID = errors.New("client ID resolves to multiple agents")

// ErrInvalidExternalID marks an unusable agent-list filter.
var ErrInvalidExternalID = errors.New("invalid external_id")

// Service encapsulates agent business logic: client ID generation, uniqueness enforcement,
// and unambiguous client ID resolution. It sits between the HTTP handler and the repository,
// owning the policies that were previously scattered across the handler and builder.
type Service struct {
	repo              ports.AgentRepository
	providerService   ServiceRequirementValidator
	permissionSets    PermissionSetResolver
	logger            *slog.Logger
	multiAgentEnabled bool
}

// ServiceRequirementValidator validates that service requirements reference existing services
// with valid scopes. Satisfied by *thirdparty.ThirdpartyOAuth2ProviderService.
type ServiceRequirementValidator interface {
	ValidateServiceRequirements(ctx context.Context, reqs []storage.ServiceRequirement) error
}

type PermissionSetResolver interface {
	GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error)
}

// NewService creates a new agent domain service.
func NewService(
	repo ports.AgentRepository,
	providerService ServiceRequirementValidator,
	logger *slog.Logger,
	multiAgentEnabled bool,
	permissionSets PermissionSetResolver,
) *Service {
	if logger == nil {
		panic("agents.NewService: logger must not be nil")
	}
	return &Service{
		repo:              repo,
		permissionSets:    permissionSets,
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

	if err := s.validateServiceRequirementsCoverage(ctx, agent.ServiceRequirements, agent.PermissionSets); err != nil {
		return err
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

	definition := (&storage.Agent{
		PermissionSets: agent.PermissionSets, ServiceRequirements: agent.ServiceRequirements,
	}).Copy()
	s.logger.InfoContext(ctx, "agent created",
		"event", "agents.agent.created",
		"agent_id", agent.ID,
		"permission_sets", definition.PermissionSets,
		"service_requirements", definition.ServiceRequirements)
	return nil
}

// UpdateInput carries explicit clear/presence instructions independently of nullable values.
type UpdateInput struct {
	ClearClientID      bool
	CanonicalIDPresent bool
}

// Update validates and persists the effective definition, preserving omitted identities (ADR 017).
func (s *Service) Update(ctx context.Context, agentID id.AgentID, agent *storage.Agent, input UpdateInput) error {
	existing, err := s.repo.Get(ctx, agentID)
	if err != nil {
		return err
	}
	candidate, err := s.updateCandidate(ctx, existing, agent, input)
	if err != nil {
		return err
	}
	*agent = *candidate
	return s.persistUpdate(ctx, existing, agent)
}

func (s *Service) updateCandidate(ctx context.Context, existing, desired *storage.Agent, input UpdateInput) (*storage.Agent, error) {
	agent := desired.Copy()
	agent.ID = existing.ID
	if agent.CanonicalID == nil && !input.CanonicalIDPresent {
		agent.CanonicalID = existing.CanonicalID
	}
	agent.CreatedAt = existing.CreatedAt

	if input.ClearClientID {
		agent.ClientID = nil
	} else if agent.ClientID == nil {
		agent.ClientID = existing.ClientID
	}

	if err := agent.Validate(); err != nil {
		return nil, storage.NewStorageError("Update", storage.ErrorKindValidation, err, err.Error())
	}

	if err := s.validateServiceRequirementsCoverage(ctx, agent.ServiceRequirements, agent.PermissionSets); err != nil {
		return nil, err
	}
	if err := s.providerService.ValidateServiceRequirements(ctx, agent.ServiceRequirements); err != nil {
		return nil, err
	}

	if !s.multiAgentEnabled && agent.ClientID != nil {
		if err := s.checkClientIDUniqueness(ctx, *agent.ClientID, &existing.ID); err != nil {
			return nil, err
		}
	}

	return agent, nil
}

func (s *Service) persistUpdate(ctx context.Context, existing, agent *storage.Agent) error {
	previous := existing.Copy()

	if err := s.repo.Update(ctx, agent); err != nil {
		return err
	}

	definition := (&storage.Agent{
		PermissionSets: agent.PermissionSets, ServiceRequirements: agent.ServiceRequirements,
	}).Copy()
	s.logger.InfoContext(ctx, "agent updated",
		"event", "agents.agent.updated",
		"agent_id", agent.ID,
		"permission_sets", definition.PermissionSets,
		"service_requirements", definition.ServiceRequirements,
		"previous_observed_permission_sets", previous.PermissionSets,
		"previous_observed_service_requirements", previous.ServiceRequirements,
		"previous_observed_updated_at", previous.UpdatedAt.UTC().Format(time.RFC3339Nano))
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
	deleted, err := s.repo.Delete(ctx, agentID)
	if err != nil {
		return err
	}
	if deleted {
		s.logger.InfoContext(ctx, "agent deleted", "event", "agents.agent.deleted", "agent_id", agentID)
	}
	return nil
}

// List retrieves all agents.
func (s *Service) List(ctx context.Context) ([]*storage.Agent, error) {
	return s.repo.List(ctx)
}

// ListByExternalID rejects unusable filters before querying storage.
func (s *Service) ListByExternalID(ctx context.Context, externalID id.ExternalID) ([]*storage.Agent, error) {
	if err := validateExternalID(externalID); err != nil {
		return nil, err
	}
	agent, err := s.repo.GetByExternalID(ctx, externalID)
	if errors.Is(err, ports.ErrNotFound) {
		return []*storage.Agent{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []*storage.Agent{agent}, nil
}

func validateExternalID(externalID id.ExternalID) error {
	if err := storage.ValidateExternalID(externalID); err != nil {
		return invalidExternalID(err.Error())
	}
	return nil
}

func invalidExternalID(message string) error {
	return storage.NewStorageError("ValidateExternalID", storage.ErrorKindValidation, ErrInvalidExternalID, message)
}

// FR-019 requires every service requirement to be covered by a declared permission set.
func (s *Service) validateServiceRequirementsCoverage(ctx context.Context, serviceReqs []storage.ServiceRequirement, psEntries []storage.AgentPermissionSetEntry) error {
	if len(serviceReqs) == 0 {
		return nil
	}

	if s.permissionSets == nil {
		return storage.NewStorageError("ValidateAgent", storage.ErrorKindValidation, nil, "permission set resolver is required for service coverage")
	}
	psIDs := make([]id.PermissionSetID, len(psEntries))
	for i, e := range psEntries {
		psIDs[i] = e.PermissionSetID
	}

	resolvedSets, err := s.permissionSets.GetByIDs(ctx, psIDs)
	if err != nil {
		return fmt.Errorf("failed to resolve permission sets for coverage check: %w", err)
	}

	coveredServiceIDs := make(map[id.ServiceID]bool)
	for _, ps := range resolvedSets {
		for _, ss := range ps.ServiceScopes {
			coveredServiceIDs[ss.ServiceID] = true
		}
	}

	var uncoveredIDs []string
	for _, sr := range serviceReqs {
		if !coveredServiceIDs[sr.ServiceID] {
			uncoveredIDs = append(uncoveredIDs, sr.ServiceID.String())
		}
	}

	if len(uncoveredIDs) > 0 {
		return storage.NewStorageError("ValidateAgent", storage.ErrorKindValidation, nil, fmt.Sprintf("service_requirements reference services not covered by any permission set: %v", uncoveredIDs))
	}

	return nil
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
