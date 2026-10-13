package oauth2server

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type CredentialService struct {
	agents      ports.AgentRepository
	credentials ports.ClientCredentialRepository
	refreshRepo ports.RefreshTokenSessionRepository
	generator   ports.CredentialGenerator
	logger      *slog.Logger
	ledger      *ledger.Service
}

func NewCredentialService(agents ports.AgentRepository, credentials ports.ClientCredentialRepository, refreshRepo ports.RefreshTokenSessionRepository, generator ports.CredentialGenerator, logger *slog.Logger, recorder *ledger.Service) *CredentialService {
	if recorder == nil {
		panic("credential recorder is required")
	}
	return &CredentialService{agents: agents, credentials: credentials, refreshRepo: refreshRepo, generator: generator, logger: logger, ledger: recorder}
}

func (s *CredentialService) Generate(ctx context.Context, agentID id.AgentID) (ports.CredentialGenerationResult, error) {
	if _, err := s.agents.Get(ctx, agentID); err != nil {
		return ports.CredentialGenerationResult{}, ports.ErrCredentialAgentNotFound
	}
	credential, plaintextSecret, err := s.generator.GenerateCredentials(agentID)
	if err != nil {
		s.logger.Error("failed to generate credentials", "agent_id", agentID, "error", err)
		return ports.CredentialGenerationResult{}, err
	}
	isRotation := false
	err = s.ledger.WithTransaction(ctx, ports.StorageTransactionHintsFromContext(ctx), func(txCtx context.Context) error {
		if _, err := s.agents.Get(txCtx, agentID); err != nil {
			return ports.ErrCredentialAgentNotFound
		}
		existing, err := s.credentials.GetByAgentID(txCtx, agentID)
		if err != nil && !ports.IsNotFoundErr(err) {
			return err
		}
		isRotation = existing != nil
		now := time.Now().UTC()
		credential.CreatedAt = now
		eventType := "credential-generated"
		if isRotation {
			credential.RotatedAt = &now
			if err := s.credentials.Rotate(txCtx, agentID, credential); err != nil {
				s.logger.Error("failed to rotate credentials", "agent_id", agentID, "error", err)
				return err
			}
			eventType = "credential-rotated"
		} else if err := s.credentials.Create(txCtx, credential); err != nil {
			s.logger.Error("failed to store credentials", "agent_id", agentID, "error", err)
			return err
		}
		return s.recordCredential(txCtx, eventType, credential)
	})
	if err != nil {
		return ports.CredentialGenerationResult{}, err
	}
	return ports.CredentialGenerationResult{Credential: credential, PlaintextSecret: plaintextSecret, Rotated: isRotation}, nil
}

func (s *CredentialService) Get(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	return s.credentials.GetByAgentID(ctx, agentID)
}

func (s *CredentialService) Revoke(ctx context.Context, agentID id.AgentID) error {
	return s.ledger.WithTransaction(ctx, ports.StorageTransactionHintsFromContext(ctx), func(txCtx context.Context) error {
		if _, err := s.agents.Get(txCtx, agentID); err != nil {
			return err
		}
		credential, err := s.credentials.GetByAgentID(txCtx, agentID)
		if err != nil {
			return err
		}
		if err := s.credentials.Delete(txCtx, agentID); err != nil {
			return err
		}
		count, err := s.refreshRepo.RevokeByAgent(txCtx, agentID)
		if err != nil {
			return fmt.Errorf("failed to revoke refresh sessions: %w", err)
		}
		if err := s.recordCredential(txCtx, "credential-revoked", credential); err != nil {
			return err
		}
		effects, ok := ports.StorageTransactionEffectsFromContext(txCtx)
		if !ok {
			return fmt.Errorf("refresh revocation requires a storage transaction")
		}
		return effects.AfterCommit(func() {
			s.logger.InfoContext(ctx, "refresh sessions revoked", "trigger", "credential_revoked", "agent_id", agentID, "row_count", count)
		})
	})
}

func (s *CredentialService) recordCredential(ctx context.Context, eventType string, credential *storage.ClientCredential) error {
	event, err := s.ledger.NewEvent(ctx, model.BusinessEventTypePrefix+eventType, model.BusinessEvent{
		OccurredAt: time.Now().UTC(), AgentID: credential.AgentID,
		Actor: model.BusinessEventActor{Kind: "admin"}, Data: map[string]any{"credential_id": credential.ID.String()},
	})
	if err != nil {
		return err
	}
	return s.ledger.Record(ctx, event)
}
