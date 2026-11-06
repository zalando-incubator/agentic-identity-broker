package servicescope

import (
	"context"
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// SnapshotReader returns one verified session snapshot for a principal's service.
type SnapshotReader interface {
	ServiceSessionSnapshot(ctx context.Context, principal id.Principal, serviceID id.ServiceID) SessionSnapshot
}

// Evaluator evaluates requested services against a grant selection, loading the agent's
// declared and selected permission sets once and one session snapshot per service.
type Evaluator struct {
	permissionSets PermissionSetLookup
	sessions       SnapshotReader
}

// NewEvaluator creates an Evaluator.
func NewEvaluator(permissionSets PermissionSetLookup, sessions SnapshotReader) *Evaluator {
	if permissionSets == nil || sessions == nil {
		panic("servicescope.NewEvaluator: dependencies must not be nil")
	}
	return &Evaluator{permissionSets: permissionSets, sessions: sessions}
}

// Evaluate returns one outcome per requested service, in request order. Only a permission-set
// repository failure is an error; session failures are usability outcomes.
func (e *Evaluator) Evaluate(ctx context.Context, principal id.Principal, agent *storage.Agent, selection []storage.GrantedPermissionSetEntry, serviceIDs []id.ServiceID) ([]Outcome, error) {
	ids := make([]id.PermissionSetID, 0, len(agent.PermissionSets)+len(selection))
	seen := map[id.PermissionSetID]bool{}
	for _, declared := range agent.PermissionSets {
		if !seen[declared.PermissionSetID] {
			seen[declared.PermissionSetID] = true
			ids = append(ids, declared.PermissionSetID)
		}
	}
	for _, selected := range selection {
		if !seen[selected.PermissionSetID] {
			seen[selected.PermissionSetID] = true
			ids = append(ids, selected.PermissionSetID)
		}
	}
	sets := map[id.PermissionSetID]*storage.PermissionSet{}
	if len(ids) > 0 {
		resolved, err := e.permissionSets.GetByIDs(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("resolve permission sets: %w", err)
		}
		for _, set := range resolved {
			sets[set.ID] = set
		}
	}
	outcomes := make([]Outcome, len(serviceIDs))
	for i, serviceID := range serviceIDs {
		outcomes[i] = Evaluate(agent, selection, sets, serviceID, e.sessions.ServiceSessionSnapshot(ctx, principal, serviceID))
	}
	return outcomes, nil
}

// AllSatisfied reports whether every outcome is satisfied.
func AllSatisfied(outcomes []Outcome) bool {
	for _, outcome := range outcomes {
		if !outcome.Satisfied {
			return false
		}
	}
	return true
}

// AnyTransient reports whether usability of a selected requested service could not be verified.
func AnyTransient(outcomes []Outcome) bool {
	for _, outcome := range outcomes {
		if outcome.Usability == UsabilityUnverifiableTransient {
			return true
		}
	}
	return false
}
