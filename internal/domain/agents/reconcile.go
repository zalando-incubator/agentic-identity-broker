package agents

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ReconcileInput carries JSON presence independently of nullable values.
type ReconcileInput struct {
	CanonicalIDPresent bool
	ExternalIDPresent  bool
	ExternalIDNull     bool
}

type ReconcileOutcome int

const (
	ReconcileCreated ReconcileOutcome = iota + 1
	ReconcileUpdated
	ReconcileUnchanged
)

func (s *Service) ReconcileByExternalID(ctx context.Context, selector id.ExternalID, desired *storage.Agent, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error) {
	if err := validateExternalID(selector); err != nil {
		return nil, 0, err
	}
	if input.ExternalIDNull {
		return nil, 0, invalidExternalID("external_id must not be null")
	}
	if input.ExternalIDPresent && (desired.ExternalID == nil || *desired.ExternalID != selector) {
		return nil, 0, invalidExternalID("body external_id must equal the selector")
	}
	existing, err := s.repo.GetByExternalID(ctx, selector)
	if err == nil {
		agent, outcome, err := s.reconcileExisting(ctx, existing, desired, selector, input)
		if !errors.Is(err, errAgentRemoved) {
			return agent, outcome, err
		}
	} else if !errors.Is(err, ports.ErrNotFound) {
		return nil, 0, err
	}
	return s.reconcileNew(ctx, desired, selector, input)
}

// errAgentRemoved marks an update that found no agent because it was deleted after the lookup.
var errAgentRemoved = errors.New("agent removed during reconciliation")

func (s *Service) reconcileNew(ctx context.Context, desired *storage.Agent, selector id.ExternalID, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error) {
	candidate := desired.Copy()
	candidate.ID = id.NewAgentID()
	candidate.ExternalID = &selector
	candidate.CreatedAt = time.Now().UTC()
	candidate.UpdatedAt = candidate.CreatedAt
	err := s.Create(ctx, candidate)
	if err == nil {
		return candidate, ReconcileCreated, nil
	}
	// A concurrent first call can win on any unique key, such as canonical_id, so any conflict
	// re-reads once. Without a holder of the external ID the conflict is real.
	var storageErr *storage.StorageError
	if !errors.As(err, &storageErr) || storageErr.Kind != storage.ErrorKindConflict {
		return nil, 0, err
	}
	holder, lookupErr := s.repo.GetByExternalID(ctx, selector)
	if errors.Is(lookupErr, ports.ErrNotFound) {
		return nil, 0, err
	}
	if lookupErr != nil {
		return nil, 0, lookupErr
	}
	agent, outcome, updateErr := s.reconcileExisting(ctx, holder, desired, selector, input)
	if errors.Is(updateErr, errAgentRemoved) {
		return nil, 0, err
	}
	return agent, outcome, updateErr
}

func (s *Service) reconcileExisting(ctx context.Context, existing, desired *storage.Agent, selector id.ExternalID, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error) {
	candidate := desired.Copy()
	candidate.ExternalID = &selector
	candidate, err := s.updateCandidate(ctx, existing, candidate, UpdateInput{CanonicalIDPresent: input.CanonicalIDPresent})
	if err != nil {
		return nil, 0, err
	}
	if sameDefinition(existing, candidate) {
		return existing.Copy(), ReconcileUnchanged, nil
	}
	candidate.UpdatedAt = time.Now().UTC()
	if err := s.persistUpdate(ctx, existing, candidate); err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			return nil, 0, fmt.Errorf("%w: %w", errAgentRemoved, err)
		}
		return nil, 0, err
	}
	return candidate, ReconcileUpdated, nil
}

func samePointer[T comparable](a, b *T) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func sameDefinition(a, b *storage.Agent) bool {
	if !samePointer(a.CanonicalID, b.CanonicalID) || !samePointer(a.ClientID, b.ClientID) || !samePointer(a.ExternalID, b.ExternalID) ||
		a.DisplayName != b.DisplayName || a.Description != b.Description || !samePointer(a.GovernanceURL, b.GovernanceURL) ||
		!samePointer(a.UserDocumentationURL, b.UserDocumentationURL) || !samePointer(a.AgentInterfaceURL, b.AgentInterfaceURL) ||
		!slices.Equal(a.PermissionSets, b.PermissionSets) || !slices.Equal(a.RedirectURIs, b.RedirectURIs) || !slices.Equal(a.AllowedScopes, b.AllowedScopes) {
		return false
	}
	if !slices.EqualFunc(a.ServiceRequirements, b.ServiceRequirements, func(x, y storage.ServiceRequirement) bool {
		return x.ServiceID == y.ServiceID && x.RequirementType == y.RequirementType && x.RequireAllScopes == y.RequireAllScopes && slices.Equal(x.RequiredScopes, y.RequiredScopes)
	}) {
		return false
	}
	left, right := slices.Clone(a.ClientURIs), slices.Clone(b.ClientURIs)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}
