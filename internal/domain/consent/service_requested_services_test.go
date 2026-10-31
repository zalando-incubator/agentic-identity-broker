package consent

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const requestedGitHubScope = "https://broker.example.com/sessions/github"

type requestedServiceLookup struct {
	services map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity
}

func (l *requestedServiceLookup) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if service, ok := l.services[serviceID]; ok {
		return service, nil
	}
	return nil, ports.ErrNotFound
}

func (l *requestedServiceLookup) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	for _, service := range l.services {
		if service.CanonicalID != nil && *service.CanonicalID == canonicalID {
			return service, nil
		}
	}
	return nil, ports.ErrNotFound
}

type requestedSnapshotReader map[id.ServiceID]servicescope.SessionSnapshot

func (r requestedSnapshotReader) ServiceSessionSnapshot(_ context.Context, _ id.Principal, serviceID id.ServiceID) servicescope.SessionSnapshot {
	if snapshot, ok := r[serviceID]; ok {
		return snapshot
	}
	return servicescope.SessionSnapshot{Usability: servicescope.UsabilityMissing}
}

type requestedConsentFixture struct {
	svc                       *Service
	grants                    *mockGrantRepo
	snapshots                 requestedSnapshotReader
	services                  *requestedServiceLookup
	agentID                   id.AgentID
	github                    id.ServiceID
	readSet, triageSet, other id.PermissionSetID
	future                    time.Time
}

func newRequestedConsentFixture(t *testing.T) *requestedConsentFixture {
	t.Helper()
	canonical := "github"
	f := &requestedConsentFixture{
		agentID: id.NewAgentID(), github: id.NewServiceID(), readSet: id.NewPermissionSetID(),
		triageSet: id.NewPermissionSetID(), other: id.NewPermissionSetID(), future: time.Now().Add(time.Hour),
	}
	identity := id.NewServiceID()
	agents := &mockAgentRepo{agents: map[id.AgentID]*storage.Agent{f.agentID: {
		ID: f.agentID, DisplayName: "Data Analysis Assistant",
		PermissionSets: []storage.AgentPermissionSetEntry{
			{PermissionSetID: f.other, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: f.readSet, RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: f.triageSet, RequirementType: storage.RequirementTypeOptional},
		},
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: f.github, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"repo", "user:email"}},
			{ServiceID: identity, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"profile"}},
		},
	}}}
	sets := &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
		f.other:     {ID: f.other, ServiceScopes: []storage.ServiceScope{{ServiceID: identity, Scopes: []string{"profile"}, RequirementType: storage.RequirementTypeOptional}}},
		f.readSet:   {ID: f.readSet, ServiceScopes: []storage.ServiceScope{{ServiceID: f.github, Scopes: []string{"repo", "user:email"}, RequirementType: storage.RequirementTypeOptional}}},
		f.triageSet: {ID: f.triageSet, ServiceScopes: []storage.ServiceScope{{ServiceID: f.github, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional}}},
	}}
	sessions := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{
		id.NewSessionID(): {Principal: "alice@example.com", ServiceID: identity, RefreshTokenExpiresAt: &f.future},
		id.NewSessionID(): {Principal: "alice@example.com", ServiceID: f.github, RefreshTokenExpiresAt: &f.future},
	}}
	f.grants = &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}
	existing := &storage.UserGrant{ID: id.NewGrantID(), Principal: "alice@example.com", AgentID: f.agentID, ValidUntil: &f.future,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: f.other, IncludedServiceIDs: []id.ServiceID{identity}}}}
	f.grants.grants[existing.ID] = existing
	f.snapshots = requestedSnapshotReader{identity: {Usability: servicescope.UsabilityUsable, Scopes: []string{"profile"}}}
	f.services = &requestedServiceLookup{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{f.github: {ID: f.github, CanonicalID: &canonical}}}
	f.svc = NewService(agents, nil, f.grants, sessions, &mockConsentRefreshRepo{}, sets, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithRequestedServices(servicescope.NewResolver(f.services, sets, "https://broker.example.com"), servicescope.NewEvaluator(sets, f.snapshots))
	return f
}

func (f *requestedConsentFixture) request(sets ...id.PermissionSetID) *GrantRequest {
	entries := []storage.GrantedPermissionSetEntry{{PermissionSetID: f.other, IncludedServiceIDs: f.grants.firstGrant().GrantedPermissionSets[0].IncludedServiceIDs}}
	for _, set := range sets {
		entries = append(entries, storage.GrantedPermissionSetEntry{PermissionSetID: set, IncludedServiceIDs: []id.ServiceID{f.github}})
	}
	return &GrantRequest{
		Principal: "alice@example.com", AgentID: f.agentID, ValidUntil: &f.future, GrantedPermissionSets: entries,
		RequestedServiceIDs:  []id.ServiceID{f.github},
		ServiceScopeBindings: map[string]id.ServiceID{requestedGitHubScope: f.github},
	}
}

// Consent with requested services from specs/055-incremental-service-authorization (FR-009–FR-011; SR-001–SR-003).
func TestRequestedServiceConsent(t *testing.T) {
	t.Run("detail lists every candidate with supplied scopes and contribution usability", func(t *testing.T) {
		f := newRequestedConsentFixture(t)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}}
		outcomes, err := f.svc.RequestedServiceCoverage(context.Background(), "alice@example.com", f.agentID, []id.ServiceID{f.github})
		require.NoError(t, err)
		require.Len(t, outcomes, 1)
		assert.Equal(t, []string{"repo", "user:email"}, outcomes[0].RequiredScopes)
		assert.Equal(t, servicescope.UsabilityUnusableDefinitive, outcomes[0].Baseline)
		assert.Equal(t, []servicescope.Candidate{
			{PermissionSetID: f.readSet, SuppliedRequiredScopes: []string{"repo", "user:email"}, Usability: servicescope.UsabilityUnusableDefinitive},
			{PermissionSetID: f.triageSet, SuppliedRequiredScopes: []string{"repo"}, Usability: servicescope.UsabilityUsable},
		}, outcomes[0].Candidates)
	})

	t.Run("submission without the requested service is rejected even with a usable session", func(t *testing.T) {
		f := newRequestedConsentFixture(t)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo", "user:email"}}
		req := f.request()
		_, err := f.svc.GrantConsent(context.Background(), req)
		assert.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
		assert.ErrorContains(t, err, f.github.String())
		assert.Zero(t, f.grants.updateCalls+f.grants.createCalls, "no grant is written before the check")
	})

	t.Run("partial coverage and unusable sessions are rejected before persistence", func(t *testing.T) {
		f := newRequestedConsentFixture(t)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo", "user:email"}}
		_, err := f.svc.GrantConsent(context.Background(), f.request(f.triageSet))
		assert.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityMissing}
		_, err = f.svc.GrantConsent(context.Background(), f.request(f.readSet))
		assert.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
		assert.Zero(t, f.grants.updateCalls+f.grants.createCalls)
	})

	t.Run("a covered and usable selection is saved without auto-selecting other sets", func(t *testing.T) {
		f := newRequestedConsentFixture(t)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo", "user:email"}}
		grant, err := f.svc.GrantConsent(context.Background(), f.request(f.readSet))
		require.NoError(t, err)
		require.Len(t, grant.GrantedPermissionSets, 2)
		assert.Equal(t, f.future, *grant.ValidUntil)
	})

	t.Run("a sealed binding reassigned before Save is rejected", func(t *testing.T) {
		f := newRequestedConsentFixture(t)
		f.snapshots[f.github] = servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo", "user:email"}}
		other := "github-old"
		f.services.services[f.github].CanonicalID = &other
		_, err := f.svc.GrantConsent(context.Background(), f.request(f.readSet))
		assert.ErrorIs(t, err, ErrRequestedServiceNotSatisfied)
	})
}

func (m *mockGrantRepo) firstGrant() *storage.UserGrant {
	for _, grant := range m.grants {
		return grant
	}
	return nil
}
