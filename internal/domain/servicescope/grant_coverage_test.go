package servicescope

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestCover(t *testing.T) {
	sets := castPermissionSets()

	t.Run("fixed requirement intersects the selected union with the ceiling", func(t *testing.T) {
		coverage := Cover(castAgent(), []storage.GrantedPermissionSetEntry{entry(readSetID, githubID), entry(triageSetID, githubID)}, sets, githubID)
		assert.Equal(t, CoverageSatisfied, coverage.Failure)
		assert.Equal(t, []string{"repo", "user:email"}, coverage.EffectiveScopes)
		assert.True(t, coverage.RequiredScopesCovered)
	})

	t.Run("complementary sets cover required scopes together", func(t *testing.T) {
		agent := castAgent()
		emailSet := id.NewPermissionSetID()
		withEmail := map[id.PermissionSetID]*storage.PermissionSet{triageSetID: sets[triageSetID], emailSet: {ID: emailSet, ServiceScopes: []storage.ServiceScope{{ServiceID: githubID, Scopes: []string{"user:email"}}}}}
		agent.PermissionSets = append(agent.PermissionSets, storage.AgentPermissionSetEntry{PermissionSetID: emailSet, RequirementType: storage.RequirementTypeOptional})
		assert.False(t, Cover(agent, []storage.GrantedPermissionSetEntry{entry(triageSetID, githubID)}, withEmail, githubID).RequiredScopesCovered)
		coverage := Cover(agent, []storage.GrantedPermissionSetEntry{entry(triageSetID, githubID), entry(emailSet, githubID)}, withEmail, githubID)
		assert.True(t, coverage.RequiredScopesCovered)
		assert.Equal(t, []string{"repo", "user:email"}, coverage.EffectiveScopes)
	})

	t.Run("require_all_scopes and no requirements use the full union despite empty required scopes", func(t *testing.T) {
		agent := castAgent()
		agent.ServiceRequirements[0] = storage.ServiceRequirement{ServiceID: githubID, RequirementType: storage.RequirementTypeOptional, RequireAllScopes: true}
		coverage := Cover(agent, []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID)
		assert.Equal(t, []string{"repo", "user:email"}, coverage.EffectiveScopes)
		assert.True(t, coverage.RequiredScopesCovered)

		agent.ServiceRequirements = nil
		coverage = Cover(agent, []storage.GrantedPermissionSetEntry{entry(triageSetID, githubID)}, sets, githubID)
		assert.Equal(t, []string{"repo"}, coverage.EffectiveScopes)
		assert.True(t, coverage.RequiredScopesCovered)
	})

	t.Run("structural and inclusion failures are preserved", func(t *testing.T) {
		assert.Equal(t, CoveragePermissionSetMissing, Cover(castAgent(), []storage.GrantedPermissionSetEntry{entry(id.NewPermissionSetID(), githubID)}, sets, githubID).Failure)
		assert.Equal(t, CoverageServiceOmitted, Cover(castAgent(), []storage.GrantedPermissionSetEntry{entry(identitySetID, identityID)}, sets, githubID).Failure)
		assert.Equal(t, CoverageServiceOmitted, Cover(castAgent(), []storage.GrantedPermissionSetEntry{entry(readSetID)}, sets, githubID).Failure)
		assert.Equal(t, CoverageServiceDefinitionMissing, Cover(castAgent(), []storage.GrantedPermissionSetEntry{entry(identitySetID, identityID, githubID)}, sets, githubID).Failure)

		agent := castAgent()
		agent.ServiceRequirements = agent.ServiceRequirements[1:]
		assert.Equal(t, CoverageServiceRequirementExcluded, Cover(agent, []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID).Failure)

		agent = castAgent()
		agent.ServiceRequirements[0].RequiredScopes = []string{"admin:org"}
		coverage := Cover(agent, []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}, sets, githubID)
		assert.Equal(t, CoverageScopeIntersectionEmpty, coverage.Failure)
		assert.False(t, coverage.RequiredScopesCovered)
	})

	t.Run("undeclared permission sets are detected before resolution", func(t *testing.T) {
		assert.True(t, UndeclaredPermissionSet(castAgent(), []storage.GrantedPermissionSetEntry{entry(id.NewPermissionSetID(), githubID)}))
		assert.False(t, UndeclaredPermissionSet(castAgent(), []storage.GrantedPermissionSetEntry{entry(readSetID, githubID)}))
	})
}

func TestCoverDuplicateEntryUsesFinalInclusion(t *testing.T) {
	entries := []storage.GrantedPermissionSetEntry{entry(readSetID, githubID), entry(readSetID, jiraID)}
	assert.Equal(t, CoverageServiceOmitted, Cover(castAgent(), entries, castPermissionSets(), githubID).Failure)
}
