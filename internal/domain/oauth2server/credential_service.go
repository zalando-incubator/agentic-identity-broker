package oauth2server

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type CredentialService struct {
	agents      ports.AgentRepository
	credentials ports.ClientCredentialRepository
	generator   ports.CredentialGenerator
	logger      *slog.Logger
	coordinator ports.AuthorizationSessionCoordinator
	revocations ports.RefreshSessionRevocationRepository
}

func NewCredentialService(
	agents ports.AgentRepository,
	credentials ports.ClientCredentialRepository,
	generator ports.CredentialGenerator,
	logger *slog.Logger,
	coordinator ports.AuthorizationSessionCoordinator,
	revocations ports.RefreshSessionRevocationRepository,
) *CredentialService {
	if coordinator == nil || revocations == nil {
		panic("oauth2server.NewCredentialService: coordinator and revocations must not be nil")
	}
	return &CredentialService{
		agents:      agents,
		credentials: credentials,
		generator:   generator,
		logger:      logger,
		coordinator: coordinator,
		revocations: revocations,
	}
}

func (s *CredentialService) Generate(ctx context.Context, agentID id.AgentID) (ports.CredentialGenerationResult, error) {
	if _, err := s.agents.Get(ctx, agentID); err != nil {
		if ports.IsNotFoundErr(err) {
			return ports.CredentialGenerationResult{}, ports.ErrCredentialAgentNotFound
		}
		return ports.CredentialGenerationResult{}, err
	}
	credential, plaintextSecret, err := s.generator.GenerateCredentials(agentID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to prepare credentials", "agent_id", agentID)
		return ports.CredentialGenerationResult{}, err
	}

	var isRotation, entered bool
	err = s.coordinator.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
		entered = true
		agent, err := s.agents.Get(owner, agentID)
		if ports.IsNotFoundErr(err) {
			return ports.ErrCredentialAgentNotFound
		}
		if err != nil {
			return err
		}
		if agent == nil {
			return errors.New("agent lookup returned no result")
		}

		existing, err := s.credentials.GetByAgentID(owner, agentID)
		if err != nil && !ports.IsNotFoundErr(err) {
			return err
		}
		if err == nil && existing == nil {
			return errors.New("credential lookup returned no result")
		}
		isRotation = err == nil
		now := at.UTC()
		credential.CreatedAt = now
		if isRotation {
			credential.RotatedAt = &now
			return s.credentials.Rotate(owner, agentID, credential)
		}
		return s.credentials.Create(owner, credential)
	})
	if !entered && ports.IsNotFoundErr(err) {
		err = ports.ErrCredentialAgentNotFound
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to generate credentials", "agent_id", agentID)
		return ports.CredentialGenerationResult{}, err
	}

	s.logger.InfoContext(ctx, "credentials generated", "agent_id", agentID, "rotated", isRotation)
	return ports.CredentialGenerationResult{Credential: credential, PlaintextSecret: plaintextSecret, Rotated: isRotation}, nil
}

func (s *CredentialService) Get(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	return s.credentials.GetByAgentID(ctx, agentID)
}

func (s *CredentialService) Revoke(ctx context.Context, agentID id.AgentID) error {
	err := s.coordinator.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
		credential, err := s.credentials.GetByAgentID(owner, agentID)
		if err != nil {
			return err
		}
		if credential == nil {
			return errors.New("credential lookup returned no result")
		}
		if err := s.revocations.RevokeByAgent(owner, agentID, at.UTC(), storage.RefreshReasonCredentialRevoked); err != nil {
			return err
		}
		return s.credentials.Delete(owner, agentID)
	})
	if err != nil {
		if !ports.IsNotFoundErr(err) {
			s.logger.ErrorContext(ctx, "failed to revoke credentials", "agent_id", agentID)
		}
		return err
	}
	s.logger.InfoContext(ctx, "credentials revoked", "agent_id", agentID)
	return nil
}
