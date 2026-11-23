package app

import (
	"context"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type impersonationConsentChecker struct {
	consentService *consent.Service
}

func newImpersonationConsentChecker(consentService *consent.Service) ports.ConsentChecker {
	return impersonationConsentChecker{consentService: consentService}
}

func (c impersonationConsentChecker) CheckConsent(ctx context.Context, principal id.Principal, agent *storage.Agent) (ports.ConsentStatus, error) {
	if c.consentService == nil {
		return ports.ConsentStatus{Reason: ports.ConsentGrantLookupFailed}, fmt.Errorf("impersonation consent checker is not configured")
	}
	return c.consentService.CheckConsent(ctx, principal, agent)
}
