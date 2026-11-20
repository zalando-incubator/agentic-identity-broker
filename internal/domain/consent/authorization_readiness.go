package consent

import (
	"context"
	"fmt"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// EvaluateAuthorizationReadiness applies the same grant, included-session and
// mandatory-service checks to browser authorization and human impersonation.
// It reads session metadata only: no token decryption or provider refresh occurs.
func EvaluateAuthorizationReadiness(
	ctx context.Context,
	grants ports.UserGrantRepository,
	sessions ports.UserSessionRepository,
	principal id.Principal,
	agent *storage.Agent,
) (ports.AuthorizationReadiness, error) {
	grant, err := grants.FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if err != nil && !ports.IsNotFoundErr(err) {
		return ports.AuthorizationReadiness{Reason: ports.ReadinessGrantLookupFailed}, fmt.Errorf("find user grant: %w", err)
	}
	if grant == nil || ports.IsNotFoundErr(err) {
		return ports.AuthorizationReadiness{Reason: ports.ReadinessGrantMissing}, nil
	}
	if !grant.IsActive() {
		return ports.AuthorizationReadiness{Reason: ports.ReadinessGrantExpired}, nil
	}

	for _, entry := range grant.GrantedPermissionSets {
		if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
			return declared.PermissionSetID == entry.PermissionSetID
		}) {
			return ports.AuthorizationReadiness{Reason: ports.ReadinessPermissionSetUndeclared, PermissionSetID: entry.PermissionSetID}, nil
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
				return ports.AuthorizationReadiness{Reason: ports.ReadinessSessionLookupFailed, ServiceID: serviceID}, fmt.Errorf("find included session for service %s: %w", serviceID, err)
			}
			if session == nil {
				return ports.AuthorizationReadiness{Reason: ports.ReadinessIncludedSessionMissing, ServiceID: serviceID}, nil
			}
			if session.RequiresReauthentication() {
				return ports.AuthorizationReadiness{Reason: ports.ReadinessIncludedSessionUnusable, ServiceID: serviceID}, nil
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
				return ports.AuthorizationReadiness{Reason: ports.ReadinessSessionLookupFailed, ServiceID: requirement.ServiceID}, fmt.Errorf("find mandatory session for service %s: %w", requirement.ServiceID, err)
			}
			if session != nil && !session.RequiresReauthentication() {
				if seen == nil {
					seen = make(map[id.ServiceID]*storage.UserSession)
				}
				seen[requirement.ServiceID] = session
			}
		}
		if session == nil {
			return ports.AuthorizationReadiness{Reason: ports.ReadinessMandatorySessionMissing, ServiceID: requirement.ServiceID}, nil
		}
		if session.RequiresReauthentication() {
			return ports.AuthorizationReadiness{Reason: ports.ReadinessMandatorySessionUnusable, ServiceID: requirement.ServiceID}, nil
		}
		for _, scope := range requirement.RequiredScopes {
			if !slices.Contains(session.Scope, scope) {
				return ports.AuthorizationReadiness{Reason: ports.ReadinessMandatoryScopeInsufficient, ServiceID: requirement.ServiceID}, nil
			}
		}
	}
	return ports.AuthorizationReadiness{}, nil
}

// EvaluateAuthorizationReadiness uses the consent service's repositories for
// the already-resolved target agent; it does not fetch the agent again.
func (s *Service) EvaluateAuthorizationReadiness(ctx context.Context, principal id.Principal, agent *storage.Agent) (ports.AuthorizationReadiness, error) {
	return EvaluateAuthorizationReadiness(ctx, s.grantRepo, s.sessionRepo, principal, agent)
}
