package memory

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.RefreshSessionMaintenanceControl = RefreshMaintenanceControl{}

type RefreshMaintenanceControl struct{}

func (RefreshMaintenanceControl) TryAcquireLease(ctx context.Context, _ time.Duration) (bool, error) {
	return ctx.Err() == nil, ctx.Err()
}

func (RefreshMaintenanceControl) DatabaseID(context.Context) (string, error) {
	return "", errors.New("memory storage has no persistent database identity")
}
