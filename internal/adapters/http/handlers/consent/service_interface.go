package consent

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// ConsentService defines the interface that handlers depend on.
type ConsentService interface {
	GetAgentConsentDetail(ctx context.Context, agentID id.AgentID, principal id.Principal) (*consent.AgentConsentDetail, error)
	GrantConsent(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error)
	RevokeConsentForPrincipal(ctx context.Context, principal id.Principal, agentID id.AgentID) error
	GetAgentDelegations(ctx context.Context, principal id.Principal) ([]consent.AgentDelegation, error)
	GetUserGrants(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error)
}

var _ ConsentService = (*consent.Service)(nil)
