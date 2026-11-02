package agents

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// ReconcileInput carries JSON presence independently of nullable values.
type ReconcileInput struct {
	CanonicalIDPresent bool
	ExternalIDPresent  bool
	ExternalIDNull     bool
}

type ReconcileOutcome int

const (
	Created ReconcileOutcome = iota
	Updated
	Unchanged
)

func (s *Service) ReconcileByExternalID(ctx context.Context, selector id.ExternalID, desired *storage.Agent, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error) {
	if err := validateExternalID(selector); err != nil {
		return nil, Unchanged, err
	}
	if input.ExternalIDNull {
		return nil, Unchanged, invalidExternalID("external_id must not be null")
	}
	if input.ExternalIDPresent && (desired.ExternalID == nil || *desired.ExternalID != selector) {
		return nil, Unchanged, invalidExternalID("body external_id must equal the selector")
	}
	holders, err := s.repo.ListByExternalID(ctx, selector)
	if err != nil {
		return nil, Unchanged, err
	}
	if len(holders) > 1 {
		return nil, Unchanged, ambiguousExternalID()
	}
	if len(holders) == 1 {
		return s.reconcileExisting(ctx, holders[0], desired, selector, input)
	}
	candidate := desired.Copy()
	candidate.ID = id.NewAgentID()
	candidate.ExternalID = &selector
	candidate.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	candidate.UpdatedAt = candidate.CreatedAt
	if err := s.Create(ctx, candidate); err == nil {
		return candidate, Created, nil
	} else {
		var conflict *storage.StorageError
		if !errors.As(err, &conflict) || conflict.Kind != storage.ErrorKindConflict {
			return nil, Unchanged, err
		}
		holders, lookupErr := s.repo.ListByExternalID(ctx, selector)
		if lookupErr != nil {
			return nil, Unchanged, lookupErr
		}
		if len(holders) > 1 {
			return nil, Unchanged, ambiguousExternalID()
		}
		if len(holders) == 0 {
			return nil, Unchanged, err
		}
		return s.reconcileExisting(ctx, holders[0], desired, selector, input)
	}
}

func ambiguousExternalID() error {
	return storage.NewStorageError("ReconcileAgent", storage.ErrorKindUnknown, nil, "external_id resolves to multiple agents")
}

func (s *Service) reconcileExisting(ctx context.Context, existing, desired *storage.Agent, selector id.ExternalID, input ReconcileInput) (*storage.Agent, ReconcileOutcome, error) {
	candidate := desired.Copy()
	candidate.ID = existing.ID
	candidate.ExternalID = &selector
	candidate.CreatedAt = existing.CreatedAt
	if !input.CanonicalIDPresent {
		candidate.CanonicalID = existing.Copy().CanonicalID
	}
	candidate.ClearCanonicalID = input.CanonicalIDPresent && desired.CanonicalID == nil
	if candidate.ClientID == nil {
		candidate.ClientID = existing.Copy().ClientID
	}
	if err := candidate.Validate(); err != nil {
		return nil, Unchanged, storage.NewStorageError("ReconcileAgent", storage.ErrorKindValidation, err, err.Error())
	}
	if err := s.providerService.ValidateServiceRequirements(ctx, candidate.ServiceRequirements); err != nil {
		return nil, Unchanged, err
	}
	if !s.multiAgentEnabled && candidate.ClientID != nil {
		if err := s.checkClientIDUniqueness(ctx, *candidate.ClientID, &existing.ID); err != nil {
			return nil, Unchanged, err
		}
	}
	if sameDefinition(existing, candidate) {
		return existing.Copy(), Unchanged, nil
	}
	candidate.UpdatedAt = reconcileUpdatedAt(time.Now(), existing.UpdatedAt)
	if err := s.Update(ctx, existing.ID, candidate, false); err != nil {
		return nil, Unchanged, err
	}
	return candidate, Updated, nil
}

func reconcileUpdatedAt(now, observed time.Time) time.Time {
	now = now.UTC().Truncate(time.Microsecond)
	minimum := observed.UTC().Truncate(time.Microsecond).Add(time.Microsecond)
	if now.Before(minimum) {
		return minimum
	}
	return now
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
