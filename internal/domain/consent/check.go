package consent

import (
	"context"
	"fmt"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// CheckConsent determines whether the user must return to the agent consent page.
// It reads grant and session metadata without decrypting tokens or refreshing them.
func CheckConsent(
	ctx context.Context,
	grants ports.UserGrantRepository,
	sessions ports.UserSessionRepository,
	principal id.Principal,
	agent *storage.Agent,
) (ports.ConsentStatus, error) {
	grant, err := grants.FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if err != nil && !ports.IsNotFoundErr(err) {
		return ports.ConsentStatus{Reason: ports.ConsentGrantLookupFailed}, fmt.Errorf("find user grant: %w", err)
	}
	if grant == nil || ports.IsNotFoundErr(err) {
		return ports.ConsentStatus{Reason: ports.ConsentGrantMissing}, nil
	}
	if !grant.IsActive() {
		return ports.ConsentStatus{Reason: ports.ConsentGrantExpired}, nil
	}

	for _, entry := range grant.GrantedPermissionSets {
		if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
			return declared.PermissionSetID == entry.PermissionSetID
		}) {
			return ports.ConsentStatus{Reason: ports.ConsentPermissionSetUndeclared}, nil
		}
	}

	// One lookup per service, even when multiple permission sets or mandatory requirements
	// include the same service. Keep the first failing service in grant order.
	var seen map[id.ServiceID]*storage.UserSession
	for _, entry := range grant.GrantedPermissionSets {
		for _, serviceID := range entry.IncludedServiceIDs {
			if _, exists := seen[serviceID]; exists {
				continue
			}
			session, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			if err != nil {
				return ports.ConsentStatus{Reason: ports.ConsentSessionLookupFailed, ServiceID: serviceID}, fmt.Errorf("find included session for service %s: %w", serviceID, err)
			}
			if session == nil {
				return ports.ConsentStatus{Reason: ports.ConsentIncludedSessionMissing, ServiceID: serviceID}, nil
			}
			if session.RequiresReauthentication() {
				return ports.ConsentStatus{Reason: ports.ConsentIncludedSessionUnusable, ServiceID: serviceID}, nil
			}
			if seen == nil {
				seen = make(map[id.ServiceID]*storage.UserSession)
			}
			seen[serviceID] = session
		}
	}

	for _, requirement := range agent.ServiceRequirements {
		if !requirement.IsMandatory() {
			continue
		}
		session, exists := seen[requirement.ServiceID]
		if !exists {
			var err error
			session, err = sessions.FindByPrincipalAndService(ctx, principal, requirement.ServiceID)
			if err != nil {
				return ports.ConsentStatus{Reason: ports.ConsentSessionLookupFailed, ServiceID: requirement.ServiceID}, fmt.Errorf("find mandatory session for service %s: %w", requirement.ServiceID, err)
			}
			if session != nil && !session.RequiresReauthentication() {
				if seen == nil {
					seen = make(map[id.ServiceID]*storage.UserSession)
				}
				seen[requirement.ServiceID] = session
			}
		}
		if session == nil {
			return ports.ConsentStatus{Reason: ports.ConsentMandatorySessionMissing, ServiceID: requirement.ServiceID}, nil
		}
		if session.RequiresReauthentication() {
			return ports.ConsentStatus{Reason: ports.ConsentMandatorySessionUnusable, ServiceID: requirement.ServiceID}, nil
		}
		for _, scope := range requirement.RequiredScopes {
			if !slices.Contains(session.Scope, scope) {
				return ports.ConsentStatus{
					Reason:         ports.ConsentMandatoryScopeInsufficient,
					ServiceID:      requirement.ServiceID,
					RequiredScopes: slices.Clone(requirement.RequiredScopes),
					SessionScopes:  slices.Clone(session.Scope),
				}, nil
			}
		}
	}
	return ports.ConsentStatus{}, nil
}

// CheckConsent uses the consent service's repositories and the resolved agent.
func (s *Service) CheckConsent(ctx context.Context, principal id.Principal, agent *storage.Agent) (ports.ConsentStatus, error) {
	return CheckConsent(ctx, s.grantRepo, s.sessionRepo, principal, agent)
}
