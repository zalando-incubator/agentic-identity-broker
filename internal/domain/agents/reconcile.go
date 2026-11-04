package agents

import (
	"context"
	"errors"
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
		return s.reconcileExisting(ctx, existing, desired, selector, input)
	}
	if !errors.Is(err, ports.ErrNotFound) {
		return nil, 0, err
	}
	candidate := desired.Copy()
	candidate.ID = id.NewAgentID()
	candidate.ExternalID = &selector
	candidate.CreatedAt = time.Now().UTC()
	candidate.UpdatedAt = candidate.CreatedAt
	if err := s.Create(ctx, candidate); err == nil {
		return candidate, ReconcileCreated, nil
	} else {
		if !errors.Is(err, ports.ErrExternalIDConflict) {
			return nil, 0, err
		}
		holder, lookupErr := s.repo.GetByExternalID(ctx, selector)
		if errors.Is(lookupErr, ports.ErrNotFound) {
			return nil, 0, err
		}
		if lookupErr != nil {
			return nil, 0, lookupErr
		}
		return s.reconcileExisting(ctx, holder, desired, selector, input)
	}
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
