package oauth2server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type CredentialService struct {
	agents      ports.AgentRepository
	credentials ports.ClientCredentialRepository
	refreshRepo ports.RefreshTokenSessionRepository
	generator   ports.CredentialGenerator
	logger      *slog.Logger
}

func NewCredentialService(agents ports.AgentRepository, credentials ports.ClientCredentialRepository, refreshRepo ports.RefreshTokenSessionRepository, generator ports.CredentialGenerator, logger *slog.Logger) *CredentialService {
	return &CredentialService{agents: agents, credentials: credentials, refreshRepo: refreshRepo, generator: generator, logger: logger}
}

func (s *CredentialService) Generate(ctx context.Context, agentID id.AgentID) (ports.CredentialGenerationResult, error) {
	if _, err := s.agents.Get(ctx, agentID); err != nil {
		return ports.CredentialGenerationResult{}, ports.ErrCredentialAgentNotFound
	}

	existing, _ := s.credentials.GetByAgentID(ctx, agentID)
	isRotation := existing != nil
	credential, plaintextSecret, err := s.generator.GenerateCredentials(agentID)
	if err != nil {
		s.logger.Error("failed to generate credentials", "agent_id", agentID, "error", err)
		return ports.CredentialGenerationResult{}, err
	}

	now := time.Now()
	credential.CreatedAt = now
	if isRotation {
		credential.RotatedAt = &now
		if err := s.credentials.Rotate(ctx, agentID, credential); err != nil {
			s.logger.Error("failed to rotate credentials", "agent_id", agentID, "error", err)
			return ports.CredentialGenerationResult{}, err
		}
	} else {
		if err := s.credentials.Create(ctx, credential); err != nil {
			s.logger.Error("failed to store credentials", "agent_id", agentID, "error", err)
			return ports.CredentialGenerationResult{}, err
		}
	}

	return ports.CredentialGenerationResult{Credential: credential, PlaintextSecret: plaintextSecret, Rotated: isRotation}, nil
}

func (s *CredentialService) Get(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	return s.credentials.GetByAgentID(ctx, agentID)
}

func (s *CredentialService) Revoke(ctx context.Context, agentID id.AgentID) error {
	if err := s.credentials.Delete(ctx, agentID); err != nil {
		return err
	}
	count, err := s.refreshRepo.RevokeByAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to revoke refresh sessions: %w", err)
	}
	s.logger.Info("refresh sessions revoked", "trigger", "credential_revoked", "agent_id", agentID, "row_count", count)
	return nil
}
