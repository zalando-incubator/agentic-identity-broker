package integration_test

import (
	"context"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func externalIDMatches(ctx context.Context, repo ports.AgentRepository, key id.ExternalID) ([]*storage.Agent, error) {
	agent, err := repo.GetByExternalID(ctx, key)
	if errors.Is(err, ports.ErrNotFound) {
		return []*storage.Agent{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []*storage.Agent{agent}, nil
}
