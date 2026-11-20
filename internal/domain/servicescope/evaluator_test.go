package servicescope

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type fakeSnapshots struct {
	snapshots map[id.ServiceID]SessionSnapshot
	reads     map[id.ServiceID]int
}

func (f *fakeSnapshots) ServiceSessionSnapshot(_ context.Context, _ id.Principal, serviceID id.ServiceID) SessionSnapshot {
	if f.reads == nil {
		f.reads = map[id.ServiceID]int{}
	}
	f.reads[serviceID]++
	if snapshot, ok := f.snapshots[serviceID]; ok {
		return snapshot
	}
	return SessionSnapshot{Usability: UsabilityMissing}
}

func TestEvaluatorEvaluateRequested(t *testing.T) {
	snapshots := &fakeSnapshots{snapshots: map[id.ServiceID]SessionSnapshot{
		githubID: {Usability: UsabilityUsable, Scopes: []string{"repo", "user:email"}},
	}}
	evaluator := NewEvaluator(&fakePermissionSets{sets: castPermissionSets()}, snapshots)
	outcomes, err := evaluator.Evaluate(context.Background(), "alice@example.com", castAgent(),
		[]storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, []id.ServiceID{githubID, jiraID})
	require.NoError(t, err)
	require.Len(t, outcomes, 2)
	assert.True(t, outcomes[0].Satisfied)
	assert.Len(t, outcomes[0].Candidates, 2)
	assert.False(t, outcomes[1].Satisfied)
	assert.Equal(t, UsabilityMissing, outcomes[1].Usability)
	assert.Equal(t, 1, snapshots.reads[githubID], "one verified snapshot per service")
	assert.False(t, AllSatisfied(outcomes))
	assert.False(t, AnyTransient(outcomes))
	assert.True(t, AnyTransient([]Outcome{{Usability: UsabilityUnverifiableTransient}}))

	_, err = NewEvaluator(&fakePermissionSets{err: errStorage}, snapshots).Evaluate(context.Background(), "alice@example.com", castAgent(), nil, []id.ServiceID{githubID})
	assert.ErrorIs(t, err, errStorage)
}
