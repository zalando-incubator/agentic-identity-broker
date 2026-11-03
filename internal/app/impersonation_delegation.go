package app

import (
	"context"
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

func (v userDelegationVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID) (ports.UserDelegationStatus, error) {
	if v.consentService == nil {
		return "", fmt.Errorf("user delegation verifier is not configured")
	}

	return v.consentService.VerifyAuthorizationReadiness(ctx, principal, agentID)
}
