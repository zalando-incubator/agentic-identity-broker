package consent

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var _ ConsentService = (*mockConsentService)(nil)

type mockConsentService struct {
	getAgentConsentDetailFunc     func(ctx context.Context, agentID id.AgentID, principal id.Principal) (*consent.AgentConsentDetail, error)
	grantConsentFunc              func(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error)
	revokeConsentForPrincipalFunc func(ctx context.Context, principal id.Principal, agentID id.AgentID) error
	getAgentDelegationsFunc       func(ctx context.Context, principal id.Principal) ([]consent.AgentDelegation, error)
	getUserGrantsFunc             func(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error)
}

func (m *mockConsentService) GetAgentConsentDetail(ctx context.Context, agentID id.AgentID, principal id.Principal) (*consent.AgentConsentDetail, error) {
	if m.getAgentConsentDetailFunc != nil {
		return m.getAgentConsentDetailFunc(ctx, agentID, principal)
	}
	return &consent.AgentConsentDetail{}, nil
}

func (m *mockConsentService) GrantConsent(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
	if m.grantConsentFunc != nil {
		return m.grantConsentFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockConsentService) RevokeConsentForPrincipal(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	if m.revokeConsentForPrincipalFunc != nil {
		return m.revokeConsentForPrincipalFunc(ctx, principal, agentID)
	}
	return nil
}

func (m *mockConsentService) GetAgentDelegations(ctx context.Context, principal id.Principal) ([]consent.AgentDelegation, error) {
	if m.getAgentDelegationsFunc != nil {
		return m.getAgentDelegationsFunc(ctx, principal)
	}
	return nil, nil
}

func (m *mockConsentService) GetUserGrants(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	if m.getUserGrantsFunc != nil {
		return m.getUserGrantsFunc(ctx, principal, agentID)
	}
	return nil, nil
}
