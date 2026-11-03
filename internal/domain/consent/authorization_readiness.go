package consent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// EvaluateAuthorizationReadiness checks the consent prerequisites shared by browser
// authorization and inbound broker-token minting, without decrypting or refreshing tokens.
func EvaluateAuthorizationReadiness(ctx context.Context, principal id.Principal, agent *storage.Agent, grants ports.UserGrantRepository, sessions ports.UserSessionRepository) (ports.UserDelegationStatus, error) {
	if agent == nil || agent.ID.IsZero() || grants == nil || sessions == nil {
		return "", fmt.Errorf("authorization readiness is not configured")
	}
	grant, err := grants.FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return "", fmt.Errorf("check authorization grant: %w", err)
	}
	if errors.Is(err, ports.ErrNotFound) || grant == nil {
		return ports.UserDelegationMissing, nil
	}
	if !grant.IsActive() {
		return ports.UserDelegationExpired, nil
	}
	for _, entry := range grant.GrantedPermissionSets {
		if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
			return declared.PermissionSetID == entry.PermissionSetID
		}) {
			return ports.UserDelegationConsentRequired, nil
		}
	}

	checked := make(map[id.ServiceID]*storage.UserSession)
	findSession := func(serviceID id.ServiceID) (*storage.UserSession, error) {
		if session, ok := checked[serviceID]; ok {
			return session, nil
		}
		session, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
		if err != nil {
			return nil, fmt.Errorf("check authorization session: %w", err)
		}
		checked[serviceID] = session
		return session, nil
	}
	for _, entry := range grant.GrantedPermissionSets {
		for _, serviceID := range entry.IncludedServiceIDs {
			session, err := findSession(serviceID)
			if err != nil {
				return "", err
			}
			if session == nil || session.IsExpired() {
				return ports.UserDelegationConsentRequired, nil
			}
		}
	}
	for _, requirement := range agent.ServiceRequirements {
		if !requirement.IsMandatory() {
			continue
		}
		session, err := findSession(requirement.ServiceID)
		if err != nil {
			return "", err
		}
		if session == nil || session.IsExpired() {
			return ports.UserDelegationConsentRequired, nil
		}
		for _, requiredScope := range requirement.RequiredScopes {
			if !slices.Contains(session.Scope, requiredScope) {
				return ports.UserDelegationConsentRequired, nil
			}
		}
	}
	return ports.UserDelegationActive, nil
}

func (s *Service) VerifyAuthorizationReadiness(ctx context.Context, principal id.Principal, agentID id.AgentID) (ports.UserDelegationStatus, error) {
	if s.agentRepo == nil {
		return "", fmt.Errorf("authorization readiness agent repository is not configured")
	}
	agent, err := s.agentRepo.Get(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("resolve authorization agent: %w", err)
	}
	return EvaluateAuthorizationReadiness(ctx, principal, agent, s.grantRepo, s.sessionRepo)
}
