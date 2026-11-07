package servicescope

import (
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Usability is the verified ability of a user session to supply a service's access now.
type Usability string

const (
	UsabilityUsable                Usability = "usable"
	UsabilityMissing               Usability = "missing"
	UsabilityUnusableDefinitive    Usability = "unusable_definitive"
	UsabilityUnverifiableTransient Usability = "unverifiable_transient"
)

// SessionSnapshot is one verified read of a session: whether valid (or successfully
// refreshed) access with a matching client identity exists, and its stored provider scopes.
type SessionSnapshot struct {
	Usability Usability
	Scopes    []string
}

// For returns the snapshot's usability for an effective provider-scope set.
func (s SessionSnapshot) For(effectiveScopes []string) Usability {
	if s.Usability != UsabilityUsable {
		return s.Usability
	}
	if !containsAll(s.Scopes, effectiveScopes) {
		return UsabilityUnusableDefinitive
	}
	return UsabilityUsable
}

// Candidate is a declared permission set that covers a requested service.
type Candidate struct {
	PermissionSetID        id.PermissionSetID
	SuppliedRequiredScopes []string
	Usability              Usability
}

// Outcome is the evaluation of one requested service against a grant selection.
type Outcome struct {
	ServiceID             id.ServiceID
	RequiredScopes        []string
	Candidates            []Candidate
	Selected              bool
	RequiredScopesCovered bool
	EffectiveScopes       []string
	Baseline              Usability
	Usability             Usability
	Satisfied             bool
	Failure               CoverageFailure
}

// Evaluate checks one requested service against the selected grant entries and a session
// snapshot. sets must contain every selected and declared permission set the caller resolved;
// declared sets absent from sets are not candidates.
func Evaluate(agent *storage.Agent, selection []storage.GrantedPermissionSetEntry, sets map[id.PermissionSetID]*storage.PermissionSet, serviceID id.ServiceID, snapshot SessionSnapshot) Outcome {
	required, fixed := RequiredScopes(agent, serviceID)
	outcome := Outcome{ServiceID: serviceID, RequiredScopes: slices.Clone(required)}
	if outcome.RequiredScopes == nil {
		outcome.RequiredScopes = []string{}
	}
	outcome.Baseline = snapshot.For(required)
	for _, declared := range agent.PermissionSets {
		set, ok := sets[declared.PermissionSetID]
		if !ok {
			continue
		}
		contribution := map[string]bool{}
		covers := false
		for _, scope := range set.ServiceScopes {
			if scope.ServiceID != serviceID {
				continue
			}
			covers = true
			for _, value := range scope.Scopes {
				contribution[value] = true
			}
		}
		if !covers {
			continue
		}
		supplied := []string{}
		if fixed {
			for _, scope := range required {
				if contribution[scope] {
					supplied = append(supplied, scope)
				}
			}
		}
		outcome.Candidates = append(outcome.Candidates, Candidate{
			PermissionSetID:        set.ID,
			SuppliedRequiredScopes: supplied,
			Usability:              snapshot.For(effectiveContribution(agent, serviceID, contribution)),
		})
	}

	coverage := Cover(agent, selection, sets, serviceID)
	outcome.Failure = coverage.Failure
	outcome.Selected = coverage.Failure != CoverageServiceOmitted && coverage.Failure != CoveragePermissionSetMissing
	outcome.RequiredScopesCovered = coverage.Failure == CoverageSatisfied && coverage.RequiredScopesCovered
	outcome.EffectiveScopes = coverage.EffectiveScopes
	outcome.Usability = snapshot.For(coverage.EffectiveScopes)
	outcome.Satisfied = outcome.Selected && outcome.RequiredScopesCovered && outcome.Usability == UsabilityUsable
	return outcome
}
