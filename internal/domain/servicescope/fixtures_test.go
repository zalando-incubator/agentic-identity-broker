package servicescope

import (
	"context"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var (
	githubID      = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440010")
	jiraID        = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440020")
	salesforceID  = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440030")
	identityID    = id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440040")
	identitySetID = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440002")
	readSetID     = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440001")
	triageSetID   = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440003")
	jiraSetID     = id.MustParsePermissionSetID("770e8400-e29b-41d4-a716-446655440004")
	errStorage    = errors.New("storage unavailable")
)

func ptrTo(value string) *string { return &value }

type fakeServices struct {
	services map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity
	err      error
	calls    int
}

func newFakeServices() *fakeServices {
	services := map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}
	for serviceID, canonicalID := range map[id.ServiceID]string{githubID: "github", jiraID: "jira", salesforceID: "salesforce", identityID: "corporate-identity"} {
		services[serviceID] = &model.ThirdpartyOAuth2ProviderEntity{ID: serviceID, CanonicalID: ptrTo(canonicalID), DisplayName: canonicalID}
	}
	return &fakeServices{services: services}
}

func (f *fakeServices) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if service, ok := f.services[serviceID]; ok {
		return service, nil
	}
	return nil, ports.ErrNotFound
}

func (f *fakeServices) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	for _, service := range f.services {
		if service.CanonicalID != nil && *service.CanonicalID == canonicalID {
			return service, nil
		}
	}
	return nil, &storage.StorageError{Kind: storage.ErrorKindNotFound}
}

type fakePermissionSets struct {
	sets map[id.PermissionSetID]*storage.PermissionSet
	err  error
}

func (f *fakePermissionSets) GetByIDs(_ context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	if f.err != nil {
		return nil, f.err
	}
	var result []*storage.PermissionSet
	for _, setID := range ids {
		if set, ok := f.sets[setID]; ok {
			result = append(result, set)
		}
	}
	return result, nil
}

func castPermissionSets() map[id.PermissionSetID]*storage.PermissionSet {
	set := func(setID id.PermissionSetID, serviceID id.ServiceID, scopes ...string) *storage.PermissionSet {
		return &storage.PermissionSet{ID: setID, ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, Scopes: scopes, RequirementType: storage.RequirementTypeOptional}}}
	}
	return map[id.PermissionSetID]*storage.PermissionSet{
		identitySetID: set(identitySetID, identityID, "profile"),
		readSetID:     set(readSetID, githubID, "repo", "user:email"),
		triageSetID:   set(triageSetID, githubID, "repo"),
		jiraSetID:     set(jiraSetID, jiraID, "read:jira-work"),
	}
}

// castAgent declares Identity Access, both GitHub sets and Jira Read, with optional fixed
// requirements for GitHub (repo user:email) and Jira.
func castAgent() *storage.Agent {
	return &storage.Agent{
		ID: id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
		PermissionSets: []storage.AgentPermissionSetEntry{
			{PermissionSetID: identitySetID, RequirementType: storage.RequirementTypeMandatory},
			{PermissionSetID: readSetID, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: triageSetID, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: jiraSetID, RequirementType: storage.RequirementTypeOptional},
		},
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: githubID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"repo", "user:email"}},
			{ServiceID: jiraID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"read:jira-work"}},
		},
	}
}

func entry(setID id.PermissionSetID, services ...id.ServiceID) storage.GrantedPermissionSetEntry {
	return storage.GrantedPermissionSetEntry{PermissionSetID: setID, IncludedServiceIDs: services}
}
