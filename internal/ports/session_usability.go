package ports

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type UsabilityMode int

const (
	UsabilityGate UsabilityMode = iota
	UsabilityDisplay
)

// SessionUsabilityEvaluator combines refreshability with current client identity evidence.
// Gate reads fail closed; display reads may use persisted evidence on source failure.
type SessionUsabilityEvaluator interface {
	RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode UsabilityMode) (map[id.ServiceID]bool, error)
}
