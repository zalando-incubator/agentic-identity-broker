//go:build integration

package postgres

import (
	"context"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (r *AgentRepository) externalIDMatches(ctx context.Context, key id.ExternalID) ([]*storage.Agent, error) {
	agent, err := r.GetByExternalID(ctx, key)
	if errors.Is(err, ports.ErrNotFound) {
		return []*storage.Agent{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []*storage.Agent{agent}, nil
}
