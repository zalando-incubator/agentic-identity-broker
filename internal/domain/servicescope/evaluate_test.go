package servicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestSessionSnapshotFor(t *testing.T) {
	snapshot := SessionSnapshot{Usability: UsabilityUsable, Scopes: []string{"repo", "user:email"}}
	assert.Equal(t, UsabilityUsable, snapshot.For(nil))
	assert.Equal(t, UsabilityUsable, snapshot.For([]string{"repo"}))
	assert.Equal(t, UsabilityUnusableDefinitive, snapshot.For([]string{"repo", "admin:org"}))
	for _, state := range []Usability{UsabilityMissing, UsabilityUnusableDefinitive, UsabilityUnverifiableTransient} {
		assert.Equal(t, state, SessionSnapshot{Usability: state, Scopes: []string{"repo"}}.For([]string{"repo"}))
	}
}

func TestEvaluate(t *testing.T) {
	sets := castPermissionSets()
	usable := SessionSnapshot{Usability: UsabilityUsable, Scopes: []string{"repo", "user:email"}}

	t.Run("selected, covered and usable is satisfied", func(t *testing.T) {
		outcome := Evaluate(castAgent(), []storage.GrantedPermissionSetEntry{entry(identitySetID, identityID), entry(readSetID, githubID)}, sets, githubID, usable)
		assert.True(t, outcome.Selected)
		assert.True(t, outcome.RequiredScopesCovered)
		assert.Equal(t, UsabilityUsable, outcome.Usability)
		assert.True(t, outcome.Satisfied)
		assert.Equal(t, []string{"repo", "user:email"}, outcome.RequiredScopes)
	})

	t.Run("every declared candidate is listed with supplied required scopes and contribution usability", func(t *testing.T) {
		outcome := Evaluate(castAgent(), []storage.GrantedPermissionSetEntry{entry(identitySetID, identityID)}, sets, githubID, SessionSnapshot{Usability: UsabilityUsable, Scopes: []string{"repo"}})
		assert.False(t, outcome.Selected)
		assert.False(t, outcome.Satisfied)
		assert.Equal(t, UsabilityUnusableDefinitive, outcome.Baseline, "the baseline checks the configured required scopes")
		assert.Equal(t, []Candidate{
			{PermissionSetID: readSetID, SuppliedRequiredScopes: []string{"repo", "user:email"}, Usability: UsabilityUnusableDefinitive},
			{PermissionSetID: triageSetID, SuppliedRequiredScopes: []string{"repo"}, Usability: UsabilityUsable},
		}, outcome.Candidates)
	})

	t.Run("a partial selection does not cover fixed required scopes", func(t *testing.T) {
		outcome := Evaluate(castAgent(), []storage.GrantedPermissionSetEntry{entry(triageSetID, githubID)}, sets, githubID, usable)
		assert.True(t, outcome.Selected)
		assert.False(t, outcome.RequiredScopesCovered)
		assert.False(t, outcome.Satisfied)
	})

	t.Run("valid tokens missing a selected contribution are unusable under all-scopes and no requirements", func(t *testing.T) {
		repoOnly := SessionSnapshot{Usability: UsabilityUsable, Scopes: []string{"repo"}}
		agent := castAgent()
		agent.ServiceRequirements[0] = storage.ServiceRequirement{ServiceID: githubID, RequirementType: storage.RequirementTypeOptional, RequireAllScopes: true}
		for _, candidate := range []*storage.Agent{agent, {ID: agent.ID, PermissionSets: agent.PermissionSets}} {
			outcome := Evaluate(candidate, []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID, repoOnly)
			assert.Empty(t, outcome.RequiredScopes)
			assert.Equal(t, UsabilityUsable, outcome.Baseline)
			assert.True(t, outcome.RequiredScopesCovered)
			assert.Equal(t, UsabilityUnusableDefinitive, outcome.Usability)
			assert.False(t, outcome.Satisfied)
			assert.Equal(t, UsabilityUnusableDefinitive, outcome.Candidates[0].Usability)
			assert.Equal(t, UsabilityUsable, outcome.Candidates[1].Usability)

			outcome = Evaluate(candidate, []storage.GrantedPermissionSetEntry{entry(triageSetID, githubID)}, sets, githubID, repoOnly)
			assert.True(t, outcome.Satisfied)
		}
	})

	t.Run("every usability outcome is preserved", func(t *testing.T) {
		for _, state := range []Usability{UsabilityMissing, UsabilityUnusableDefinitive, UsabilityUnverifiableTransient} {
			outcome := Evaluate(castAgent(), []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID, SessionSnapshot{Usability: state})
			assert.Equal(t, state, outcome.Usability)
			assert.Equal(t, state, outcome.Baseline)
			assert.False(t, outcome.Satisfied)
		}
	})

	t.Run("structural coverage denials are never satisfied", func(t *testing.T) {
		agent := castAgent()
		agent.ServiceRequirements[0].RequiredScopes = []string{"admin:org"}
		outcome := Evaluate(agent, []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID, usable)
		assert.Equal(t, CoverageScopeIntersectionEmpty, outcome.Failure)
		assert.False(t, outcome.Satisfied)
	})
}
