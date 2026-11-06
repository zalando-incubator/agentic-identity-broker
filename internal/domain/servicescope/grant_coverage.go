package servicescope

import (
	"slices"
	"sort"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// CoverageFailure explains why a grant selection does not authorize a service.
type CoverageFailure string

const (
	CoverageSatisfied                  CoverageFailure = ""
	CoveragePermissionSetMissing       CoverageFailure = "permission_set_missing"
	CoverageServiceOmitted             CoverageFailure = "service_omitted"
	CoverageServiceDefinitionMissing   CoverageFailure = "service_definition_missing"
	CoverageServiceRequirementExcluded CoverageFailure = "service_requirement_excluded"
	CoverageScopeIntersectionEmpty     CoverageFailure = "scope_intersection_empty"
)

// Coverage is the grant-derived authorization of one service. EffectiveScopes is the union of
// the selected permission sets' scopes for the service, intersected with an explicit fixed
// requirement's required_scopes. require_all_scopes and agents without requirements use the
// full union. This is the single rule shared by exchange, authorization, consent and refresh.
type Coverage struct {
	EffectiveScopes       []string
	RequiredScopesCovered bool
	Failure               CoverageFailure
}

// UndeclaredPermissionSet reports whether a grant entry names a permission set the agent no
// longer declares.
func UndeclaredPermissionSet(agent *storage.Agent, entries []storage.GrantedPermissionSetEntry) bool {
	for _, entry := range entries {
		if !slices.ContainsFunc(agent.PermissionSets, func(declared storage.AgentPermissionSetEntry) bool {
			return declared.PermissionSetID == entry.PermissionSetID
		}) {
			return true
		}
	}
	return false
}

// RequiredScopes returns the service's configured required scopes and whether they form a
// fixed-scope ceiling. A require_all_scopes requirement or no explicit requirement has none.
func RequiredScopes(agent *storage.Agent, serviceID id.ServiceID) ([]string, bool) {
	for _, requirement := range agent.ServiceRequirements {
		if requirement.ServiceID == serviceID && !requirement.RequireAllScopes {
			return requirement.RequiredScopes, true
		}
	}
	return nil, false
}

// requirementMode reports whether the agent declares requirements and whether the service is
// among them.
func requirementMode(agent *storage.Agent, serviceID id.ServiceID) (hasRequirements, listed bool) {
	for _, requirement := range agent.ServiceRequirements {
		if requirement.ServiceID == serviceID {
			return true, true
		}
	}
	return len(agent.ServiceRequirements) > 0, false
}

// effectiveContribution applies the requirement ceiling to a scope union.
func effectiveContribution(agent *storage.Agent, serviceID id.ServiceID, union map[string]bool) []string {
	required, fixed := RequiredScopes(agent, serviceID)
	var scopes []string
	for scope := range union {
		if !fixed || slices.Contains(required, scope) {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	return scopes
}

// Cover computes the coverage of serviceID by the selected grant entries. Every set referenced
// by entries must be present in sets.
func Cover(agent *storage.Agent, entries []storage.GrantedPermissionSetEntry, sets map[id.PermissionSetID]*storage.PermissionSet, serviceID id.ServiceID) Coverage {
	// A persisted grant can repeat a permission set; its final entry decides inclusion.
	inclusion := make(map[id.PermissionSetID][]id.ServiceID, len(entries))
	for _, entry := range entries {
		inclusion[entry.PermissionSetID] = entry.IncludedServiceIDs
	}
	union := map[string]bool{}
	included, defined := false, false
	for _, entry := range entries {
		set, ok := sets[entry.PermissionSetID]
		if !ok {
			return Coverage{Failure: CoveragePermissionSetMissing}
		}
		if !slices.Contains(inclusion[entry.PermissionSetID], serviceID) {
			continue
		}
		included = true
		for _, scope := range set.ServiceScopes {
			if scope.ServiceID != serviceID {
				continue
			}
			defined = true
			for _, value := range scope.Scopes {
				union[value] = true
			}
		}
	}
	if !included {
		return Coverage{Failure: CoverageServiceOmitted}
	}
	if !defined {
		return Coverage{Failure: CoverageServiceDefinitionMissing}
	}
	if hasRequirements, listed := requirementMode(agent, serviceID); hasRequirements && !listed {
		return Coverage{Failure: CoverageServiceRequirementExcluded}
	}
	effective := effectiveContribution(agent, serviceID, union)
	if len(effective) == 0 && len(union) > 0 {
		return Coverage{Failure: CoverageScopeIntersectionEmpty}
	}
	required, _ := RequiredScopes(agent, serviceID)
	return Coverage{EffectiveScopes: effective, RequiredScopesCovered: containsAll(effective, required)}
}

func containsAll(scopes, required []string) bool {
	for _, scope := range required {
		if !slices.Contains(scopes, scope) {
			return false
		}
	}
	return true
}
