package consent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ErrRequestedServiceNotSatisfied rejects a grant submission that leaves a requested service
// unselected, without required-scope coverage or without a usable session.
var ErrRequestedServiceNotSatisfied = errors.New("requested service not satisfied")

// WithRequestedServices enables requested-service coverage in consent detail and enforcement
// on grant submission (ADR 046).
func (s *Service) WithRequestedServices(resolver *servicescope.Resolver, evaluator *servicescope.Evaluator) *Service {
	s.requestedServices = resolver
	s.serviceEvaluator = evaluator
	return s
}

// RequestedServiceCoverage evaluates server-derived requested services against the
// principal's current grant. Baseline and candidate usability share one verified snapshot per
// service and never expose raw permission-set scopes.
func (s *Service) RequestedServiceCoverage(ctx context.Context, principal id.Principal, agentID id.AgentID, serviceIDs []id.ServiceID) ([]servicescope.Outcome, error) {
	if len(serviceIDs) == 0 || s.serviceEvaluator == nil {
		return []servicescope.Outcome{}, nil
	}
	agent, err := s.agentRepo.Get(ctx, agentID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, fmt.Errorf("%w: agent_id=%s", ErrAgentNotFound, agentID)
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	var selection []storage.GrantedPermissionSetEntry
	grant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil && !errors.Is(err, ports.ErrNotFound) {
		return nil, fmt.Errorf("failed to find existing grant: %w", err)
	}
	if grant != nil {
		selection = grant.GrantedPermissionSets
	}
	return s.serviceEvaluator.Evaluate(ctx, principal, agent, selection, serviceIDs)
}

// validateRequestedServices rechecks the sealed bindings and evaluates the submitted selection
// afresh before any grant is written.
func (s *Service) validateRequestedServices(ctx context.Context, agent *storage.Agent, req *GrantRequest) error {
	if len(req.RequestedServiceIDs) == 0 {
		return nil
	}
	if s.serviceEvaluator == nil || s.requestedServices == nil {
		return fmt.Errorf("%w: requested services cannot be evaluated", ErrRequestedServiceNotSatisfied)
	}
	if len(req.ServiceScopeBindings) > 0 {
		if _, err := s.requestedServices.ResolveBindings(ctx, req.ServiceScopeBindings, agent); err != nil {
			if servicescope.IsPermissionFailure(err) {
				return s.rejectRequestedServices(ctx, req, req.RequestedServiceIDs, "binding_mismatch")
			}
			return fmt.Errorf("failed to resolve requested services: %w", err)
		}
	}
	outcomes, err := s.serviceEvaluator.Evaluate(ctx, req.Principal, agent, req.GrantedPermissionSets, req.RequestedServiceIDs)
	if err != nil {
		return fmt.Errorf("failed to evaluate requested services: %w", err)
	}
	var unsatisfied []id.ServiceID
	for _, outcome := range outcomes {
		if !outcome.Satisfied {
			unsatisfied = append(unsatisfied, outcome.ServiceID)
		}
	}
	if len(unsatisfied) == 0 {
		return nil
	}
	return s.rejectRequestedServices(ctx, req, unsatisfied, "unsatisfied")
}

func (s *Service) rejectRequestedServices(ctx context.Context, req *GrantRequest, serviceIDs []id.ServiceID, reason string) error {
	ids := make([]string, len(serviceIDs))
	for i, serviceID := range serviceIDs {
		ids[i] = serviceID.String()
	}
	s.logger.WarnContext(ctx, "requested_service_submission_rejected",
		"principal", req.Principal, "agent_id", req.AgentID, "service_ids", ids, "reason", reason)
	return fmt.Errorf("%w: requested services must be selected, cover required scopes and have a usable session: %s",
		ErrRequestedServiceNotSatisfied, strings.Join(ids, ", "))
}
