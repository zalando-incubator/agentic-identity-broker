package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type userDelegationVerifier struct {
	consentService *consent.Service
}

func newUserDelegationVerifier(consentService *consent.Service) ports.UserDelegationVerifier {
	return userDelegationVerifier{consentService: consentService}
}

func (v userDelegationVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID) (ports.UserDelegation, error) {
	if v.consentService == nil {
		return ports.UserDelegation{}, fmt.Errorf("user delegation verifier is not configured")
	}

	grant, err := v.consentService.VerifyAgentAccess(ctx, principal, agentID)
	if err == nil {
		return ports.UserDelegation{Status: ports.UserDelegationActive, GrantedPermissionSets: grant.GrantedPermissionSets}, nil
	}
	if errors.Is(err, consent.ErrAgentAccessDenied) {
		return ports.UserDelegation{Status: ports.UserDelegationMissing}, nil
	}
	if errors.Is(err, consent.ErrGrantExpired) {
		return ports.UserDelegation{Status: ports.UserDelegationExpired}, nil
	}
	return ports.UserDelegation{}, fmt.Errorf("verify user delegation: %w", err)
}
