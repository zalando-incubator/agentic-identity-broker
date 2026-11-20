package app

import (
	"context"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type authorizationReadinessVerifier struct {
	consentService *consent.Service
}

func newAuthorizationReadinessVerifier(consentService *consent.Service) ports.AuthorizationReadinessVerifier {
	return authorizationReadinessVerifier{consentService: consentService}
}

func (v authorizationReadinessVerifier) EvaluateAuthorizationReadiness(ctx context.Context, principal id.Principal, agent *storage.Agent) (ports.AuthorizationReadiness, error) {
	if v.consentService == nil {
		return ports.AuthorizationReadiness{Reason: ports.ReadinessGrantLookupFailed}, fmt.Errorf("authorization readiness verifier is not configured")
	}
	return v.consentService.EvaluateAuthorizationReadiness(ctx, principal, agent)
}
