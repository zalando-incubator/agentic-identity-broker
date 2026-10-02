package app

import (
	"context"
	"errors"
	"fmt"
	"time"

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

func (v userDelegationVerifier) VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, decisionTime time.Time) (ports.UserDelegationDecision, error) {
	if v.consentService == nil {
		return ports.UserDelegationDecision{}, fmt.Errorf("user delegation verifier is not configured")
	}

	grant, err := v.consentService.VerifyAgentAccess(ctx, principal, agentID, decisionTime)
	if err == nil {
		return ports.UserDelegationDecision{Status: ports.UserDelegationActive, GrantID: grant.ID, ValidUntil: grant.ValidUntil}, nil
	}
	if errors.Is(err, consent.ErrAgentAccessDenied) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationMissing}, nil
	}
	if errors.Is(err, consent.ErrGrantExpired) {
		return ports.UserDelegationDecision{Status: ports.UserDelegationExpired}, nil
	}
	return ports.UserDelegationDecision{}, fmt.Errorf("verify user delegation: %w", err)
}
