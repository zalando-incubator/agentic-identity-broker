package admin

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type adminLifecycleCoordinator struct{}

func (adminLifecycleCoordinator) Run(ctx context.Context, _ id.AgentID, operation func(context.Context, time.Time) error) error {
	return operation(ctx, time.Now().UTC())
}

type adminLifecycleRevocations struct{}

func (adminLifecycleRevocations) RevokeByID(context.Context, id.RefreshSessionID, time.Time, storage.RefreshRevocationReason) error {
	return nil
}
func (adminLifecycleRevocations) RevokeByPrincipalAndAgent(context.Context, id.Principal, id.AgentID, time.Time, storage.RefreshRevocationReason) error {
	return nil
}
func (adminLifecycleRevocations) RevokeByAgent(context.Context, id.AgentID, time.Time, storage.RefreshRevocationReason) error {
	return nil
}
