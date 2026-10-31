package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Each denial row changes one condition of the known valid delegation before the broker
// boots and expects a credential-free consent-recovery decision, not an error.
var _ = Describe("AuthZEN delegation denials", Label("authzen-delegation"), func() {
	var f *authzenFixture

	BeforeEach(func() {
		f = newAuthZENData(nil)
	})

	// Scenario US2.1 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_missing when no grant exists", func() {
		Expect(f.Storage.UserGrants().Delete(context.Background(), f.Grant.ID)).To(Succeed())
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_missing")
	})

	// Scenario US2.2 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_missing after revocation of a permitted grant", func() {
		f.Boot(nil)
		permitContext(f.Evaluate(authzenRepoResource, nil))
		Expect(f.Storage.UserGrants().Delete(context.Background(), f.Grant.ID)).To(Succeed())
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_missing")
	})

	// Scenario US2.3 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_expired for an expired grant", func() {
		expired := time.Now().Add(-time.Minute)
		f.Grant.ValidUntil = &expired
		Expect(f.Storage.UserGrants().Create(context.Background(), f.Grant)).To(Succeed())
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_expired")
	})

	// Scenario US2.4 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_empty for a grant without permission sets", func() {
		f.Grant.GrantedPermissionSets = []storagedomain.GrantedPermissionSetEntry{}
		f.SaveGrant()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_empty")
	})

	// Scenario US2.5 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_permission_set_undeclared", func() {
		f.Agent.PermissionSets = f.Agent.PermissionSets[:1]
		f.SaveAgent()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_permission_set_undeclared")
	})

	// Scenario US2.6 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_permission_set_missing", func() {
		missing := id.NewPermissionSetID()
		f.Agent.PermissionSets = append(f.Agent.PermissionSets, storagedomain.AgentPermissionSetEntry{
			PermissionSetID: missing, RequirementType: storagedomain.RequirementTypeOptional})
		f.SaveAgent()
		f.Grant.GrantedPermissionSets = append(f.Grant.GrantedPermissionSets, storagedomain.GrantedPermissionSetEntry{
			PermissionSetID: missing, IncludedServiceIDs: []id.ServiceID{f.Repo.ID}})
		f.SaveGrant()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_permission_set_missing")
	})

	// Scenario US2.7 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_service_omitted", func() {
		f.Grant.GrantedPermissionSets = []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: f.Writer.ID, IncludedServiceIDs: []id.ServiceID{f.Wiki.ID}}}
		f.SaveGrant()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_service_omitted")
	})

	// Scenario US2.8 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_service_definition_missing", func() {
		f.Grant.GrantedPermissionSets = []storagedomain.GrantedPermissionSetEntry{
			{PermissionSetID: f.Writer.ID, IncludedServiceIDs: []id.ServiceID{f.Wiki.ID, f.Repo.ID}}}
		f.SaveGrant()
		f.Writer.ServiceScopes = []storagedomain.ServiceScope{
			{ServiceID: f.Wiki.ID, Scopes: []string{"read:wiki"}, RequirementType: storagedomain.RequirementTypeOptional}}
		Expect(f.Storage.PermissionSets().Update(context.Background(), f.Writer)).To(Succeed())
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_service_definition_missing")
	})

	// Scenario US2.9 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_service_requirement_excluded", func() {
		f.Agent.ServiceRequirements = f.Agent.ServiceRequirements[1:]
		f.SaveAgent()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_service_requirement_excluded")
	})

	// Scenario US2.10 from specs/054-authzen-delegation-pip/spec.md
	It("denies with grant_scope_intersection_empty", func() {
		f.Agent.ServiceRequirements[0].RequiredScopes = []string{"admin:repo"}
		f.SaveAgent()
		f.Boot(nil)
		f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_scope_intersection_empty")
	})
})
