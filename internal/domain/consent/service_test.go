package consent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// testEncryption is a non-identity test double for EncryptionPort used in domain-layer
// unit tests. It applies an invertible XOR transformation (key byte 0x55) so that
// ciphertext ≠ plaintext, keeping the Secret value-object state machine honest without
// importing any adapter package.
//
// This is NOT a production noop. The runtime noop fallback was removed in Phase 6.
// Adapter-layer and integration tests should use testutil.NewTestEncryptionAdapter(t)
// for real AES-256-GCM roundtrips.
type testEncryption struct{}

type noopBranchKeyManager struct{}

func newNoopBranchKeyManager() *noopBranchKeyManager {
	return &noopBranchKeyManager{}
}

func (m *noopBranchKeyManager) Create(_ context.Context, _ domainencryption.BranchKeySubject) (string, error) {
	return "", nil
}

func (e *testEncryption) Encrypt(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
	out := make([]byte, len(plaintext))
	for i, b := range plaintext {
		out[i] = b ^ 0x55
	}
	return out, nil
}

func (e *testEncryption) Decrypt(_ context.Context, ciphertext []byte, _ map[string]string) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		out[i] = b ^ 0x55
	}
	return out, nil
}

// newTestProviderService wraps a ThirdpartyOAuth2ProviderRepository in a domain service
// with a non-identity test double for encryption. Used in domain-layer tests that exercise
// consent business logic, not encryption correctness.
func newTestProviderService(repo ports.ThirdpartyOAuth2ProviderRepository) *thirdparty.ThirdpartyOAuth2ProviderService {
	return thirdparty.NewThirdpartyOAuth2ProviderService(repo, &testEncryption{}, newNoopBranchKeyManager(), nil, false, nil)
}

// Mock implementations for testing

type mockAgentRepo struct {
	agents map[id.AgentID]*storage.Agent
	err    error
}

func (m *mockAgentRepo) Create(ctx context.Context, agent *storage.Agent) error {
	if m.err != nil {
		return m.err
	}
	m.agents[agent.ID] = agent.Copy()
	return nil
}

func (m *mockAgentRepo) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	agent, exists := m.agents[agentID]
	if !exists {
		return nil, ports.ErrNotFound
	}
	return agent.Copy(), nil
}

func (m *mockAgentRepo) Update(ctx context.Context, agent *storage.Agent) error {
	if m.err != nil {
		return m.err
	}
	if _, exists := m.agents[agent.ID]; !exists {
		return ports.ErrNotFound
	}
	m.agents[agent.ID] = agent.Copy()
	return nil
}

func (m *mockAgentRepo) Delete(ctx context.Context, agentID id.AgentID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.agents, agentID)
	return nil
}

func (m *mockAgentRepo) List(ctx context.Context) ([]*storage.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*storage.Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		result = append(result, agent.Copy())
	}
	return result, nil
}

func (m *mockAgentRepo) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, agent := range m.agents {
		if agent.ClientID != nil && *agent.ClientID == clientID {
			return agent.Copy(), nil
		}
	}
	return nil, ports.ErrNotFound
}

func (m *mockAgentRepo) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (m *mockAgentRepo) ExistsOtherWithClientID(_ context.Context, _ id.ClientID, _ *id.AgentID) (bool, error) {
	return false, nil
}

type mockServiceRepo struct {
	services map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity
	err      error
}

func (m *mockServiceRepo) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	if m.err != nil {
		return m.err
	}
	m.services[entity.ID] = entity.Copy()
	return nil
}

func (m *mockServiceRepo) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if m.err != nil {
		return nil, m.err
	}
	entity, exists := m.services[serviceID]
	if !exists {
		return nil, ports.ErrNotFound
	}
	return entity.Copy(), nil
}

func (m *mockServiceRepo) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	if m.err != nil {
		return m.err
	}
	if _, exists := m.services[entity.ID]; !exists {
		return ports.ErrNotFound
	}
	m.services[entity.ID] = entity.Copy()
	return nil
}

func (m *mockServiceRepo) Delete(ctx context.Context, serviceID id.ServiceID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.services, serviceID)
	return nil
}

func (m *mockServiceRepo) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*model.ThirdpartyOAuth2ProviderEntity, 0, len(m.services))
	for _, entity := range m.services {
		result = append(result, entity.Copy())
	}
	return result, nil
}

func (m *mockServiceRepo) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, entity := range m.services {
		for _, resource := range entity.ProtectedResources {
			if resource == resourceURI {
				return entity.Copy(), nil
			}
		}
	}
	return nil, ports.ErrNotFound
}

func (m *mockServiceRepo) AddProtectedResource(_ context.Context, _ id.ServiceID, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, m.err
}

func (m *mockServiceRepo) RemoveProtectedResource(_ context.Context, _ id.ServiceID, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, m.err
}

func (m *mockServiceRepo) RenameProtectedResource(_ context.Context, _ id.ServiceID, _, _ string) (ports.ProtectedResourceMutationResult, error) {
	return ports.ProtectedResourceMutationResult{}, m.err
}

func (m *mockServiceRepo) ListProtectedResources(_ context.Context, _ id.ServiceID) ([]string, int64, error) {
	return nil, 0, m.err
}

type mockPermissionSetService struct {
	permissionSets map[id.PermissionSetID]*storage.PermissionSet
	err            error
}

func (m *mockPermissionSetService) Create(ctx context.Context, ps *storage.PermissionSet) error {
	if m.err != nil {
		return m.err
	}
	m.permissionSets[ps.ID] = ps.Copy()
	return nil
}

func (m *mockPermissionSetService) Get(ctx context.Context, psID id.PermissionSetID) (*storage.PermissionSet, error) {
	if m.err != nil {
		return nil, m.err
	}
	ps, exists := m.permissionSets[psID]
	if !exists {
		return nil, ports.ErrNotFound
	}
	return ps.Copy(), nil
}

func (m *mockPermissionSetService) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*storage.PermissionSet, 0)
	for _, id := range ids {
		if ps, exists := m.permissionSets[id]; exists {
			result = append(result, ps.Copy())
		}
	}
	return result, nil
}

func (m *mockPermissionSetService) Update(ctx context.Context, ps *storage.PermissionSet) error {
	if m.err != nil {
		return m.err
	}
	if _, exists := m.permissionSets[ps.ID]; !exists {
		return ports.ErrNotFound
	}
	m.permissionSets[ps.ID] = ps.Copy()
	return nil
}

func (m *mockPermissionSetService) Delete(ctx context.Context, psID id.PermissionSetID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.permissionSets, psID)
	return nil
}

func (m *mockPermissionSetService) List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*storage.PermissionSet, 0)
	for _, ps := range m.permissionSets {
		result = append(result, ps.Copy())
	}
	return result, nil
}

func (m *mockPermissionSetService) ValidateIDs(ctx context.Context, ids []id.PermissionSetID) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockPermissionSetService) CountAgentsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

func (m *mockPermissionSetService) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

func (m *mockPermissionSetService) CountPermissionSetsForService(_ context.Context, _ id.ServiceID) (int, error) {
	return 0, nil
}

func (m *mockPermissionSetService) Close() {
	// no-op
}

type mockUserSessionRepo struct {
	sessions map[id.SessionID]*storage.UserSession
	err      error
}

func (m *mockUserSessionRepo) Create(ctx context.Context, session *storage.UserSession) error {
	if m.err != nil {
		return m.err
	}
	m.sessions[session.ID] = session
	return nil
}

func (m *mockUserSessionRepo) Get(ctx context.Context, id id.SessionID) (*storage.UserSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	session, exists := m.sessions[id]
	if !exists {
		return nil, ports.ErrNotFound
	}
	return session, nil
}

func (m *mockUserSessionRepo) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, session := range m.sessions {
		if session.Principal == principal && session.ServiceID == serviceID {
			return session, nil
		}
	}
	return nil, nil
}

func (m *mockUserSessionRepo) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*storage.UserSession, 0)
	for _, session := range m.sessions {
		if session.Principal == principal {
			result = append(result, session)
		}
	}
	return result, nil
}

func (m *mockUserSessionRepo) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([]*storage.UserSession, 0)
	for _, session := range m.sessions {
		if session.Principal == principal && !session.IsExpired() {
			result = append(result, session)
		}
	}
	return result, nil
}

func (m *mockUserSessionRepo) Delete(ctx context.Context, id id.SessionID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.sessions, id)
	return nil
}

func (m *mockUserSessionRepo) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	if m.err != nil {
		return m.err
	}
	for sessionID, session := range m.sessions {
		if session.Principal == principal && session.ServiceID == serviceID {
			delete(m.sessions, sessionID)
			return nil
		}
	}
	return ports.ErrNotFound
}

func (m *mockUserSessionRepo) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	count := 0
	for _, session := range m.sessions {
		if session.ServiceID == serviceID && !session.IsExpired() {
			count++
		}
	}
	return count, nil
}

type mockGrantRepo struct {
	grants      map[id.GrantID]*storage.UserGrant
	err         error
	createCalls int
	updateCalls int
}

func (m *mockGrantRepo) Create(ctx context.Context, grant *storage.UserGrant) error {
	if m.err != nil {
		return m.err
	}
	m.createCalls++
	// Generate ID if not set
	if grant.ID.IsZero() {
		grant.ID = id.NewGrantID()
	}
	m.grants[grant.ID] = grant.Copy()
	return nil
}

func (m *mockGrantRepo) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	if m.err != nil {
		return nil, m.err
	}
	grant, exists := m.grants[grantID]
	if !exists {
		return nil, ports.ErrNotFound
	}
	return grant.Copy(), nil
}

func (m *mockGrantRepo) Update(ctx context.Context, grant *storage.UserGrant) error {
	if m.err != nil {
		return m.err
	}
	m.updateCalls++
	if _, exists := m.grants[grant.ID]; !exists {
		return ports.ErrNotFound
	}
	m.grants[grant.ID] = grant.Copy()
	return nil
}

func (m *mockGrantRepo) Delete(ctx context.Context, grantID id.GrantID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.grants, grantID)
	return nil
}

func (m *mockGrantRepo) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := []*storage.UserGrant{}
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			result = append(result, grant.Copy())
		}
	}
	return result, nil
}

func (m *mockGrantRepo) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			return grant.Copy(), nil
		}
	}
	return nil, ports.ErrNotFound
}

func (m *mockGrantRepo) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	if m.err != nil {
		return m.err
	}
	for grantKey, grant := range m.grants {
		if grant.AgentID == agentID {
			delete(m.grants, grantKey)
		}
	}
	return nil
}

func (m *mockGrantRepo) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := []storage.UserGrant{}
	for _, grant := range m.grants {
		if grant.Principal == principal && grant.IsActive() {
			result = append(result, *grant.Copy())
		}
	}
	return result, nil
}

func (m *mockGrantRepo) CountAgentsByServiceID(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	uniqueAgents := make(map[id.AgentID]bool)
	for _, grant := range m.grants {
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					uniqueAgents[grant.AgentID] = true
				}
			}
		}
	}
	return len(uniqueAgents), nil
}

func (m *mockGrantRepo) ListByServiceID(ctx context.Context, serviceID id.ServiceID) ([]id.AgentID, error) {
	if m.err != nil {
		return nil, m.err
	}
	uniqueAgents := make(map[id.AgentID]bool)
	for _, grant := range m.grants {
		for _, entry := range grant.GrantedPermissionSets {
			for _, svcID := range entry.IncludedServiceIDs {
				if svcID == serviceID {
					uniqueAgents[grant.AgentID] = true
				}
			}
		}
	}
	agentIDs := make([]id.AgentID, 0, len(uniqueAgents))
	for agentID := range uniqueAgents {
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs, nil
}

func (m *mockGrantRepo) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	if m.err != nil {
		return m.err
	}
	for grantID, grant := range m.grants {
		if grant.Principal == principal && grant.AgentID == agentID {
			delete(m.grants, grantID)
			return nil
		}
	}
	return ports.ErrNotFound
}

func (m *mockGrantRepo) CountGrantsReferencingPermissionSet(_ context.Context, psID id.PermissionSetID) (int, error) {
	count := 0
	for _, grant := range m.grants {
		for _, entry := range grant.GrantedPermissionSets {
			if entry.PermissionSetID == psID {
				count++
				break
			}
		}
	}
	return count, nil
}

type mockSessionRepo struct {
	findFunc func(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error)
}

func (m *mockSessionRepo) FindByPrincipalAndService(ctx context.Context, p id.Principal, svcID id.ServiceID) (*storage.UserSession, error) {
	if m.findFunc != nil {
		return m.findFunc(ctx, p, svcID)
	}
	return nil, nil
}

func (m *mockSessionRepo) Create(_ context.Context, _ *storage.UserSession) error { return nil }
func (m *mockSessionRepo) Get(_ context.Context, _ id.SessionID) (*storage.UserSession, error) {
	return nil, nil
}
func (m *mockSessionRepo) ListByPrincipal(_ context.Context, _ id.Principal) ([]*storage.UserSession, error) {
	return nil, nil
}
func (m *mockSessionRepo) Delete(_ context.Context, _ id.SessionID) error { return nil }
func (m *mockSessionRepo) DeleteByPrincipalAndService(_ context.Context, _ id.Principal, _ id.ServiceID) error {
	return nil
}
func (m *mockSessionRepo) CountByService(_ context.Context, _ id.ServiceID) (int, error) {
	return 0, nil
}
func (m *mockSessionRepo) ListActiveByPrincipal(_ context.Context, _ id.Principal) ([]*storage.UserSession, error) {
	return nil, nil
}

// Test cases

func TestService_GetAgentConsentDetail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	principal := id.Principal("user@example.com")

	githubID := id.NewServiceID()
	googleID := id.NewServiceID()
	agentID := id.NewAgentID()
	permissionSetID := id.NewPermissionSetID()
	permissionSetService := &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
		permissionSetID: {
			ID:          permissionSetID,
			Name:        "Test Permission Set",
			Description: "Covers the test services",
			ServiceScopes: []storage.ServiceScope{
				{ServiceID: githubID, Scopes: []string{"read:user", "repo"}, RequirementType: storage.RequirementTypeMandatory},
				{ServiceID: googleID, Scopes: []string{"email"}, RequirementType: storage.RequirementTypeOptional},
			},
		},
	}}

	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("agent-client")),
		DisplayName: "Test Agent",
		Description: "desc",
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: githubID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read:user", "repo"}},
			{ServiceID: googleID, RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"email"}},
		},
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeMandatory,
		}},
	}

	github := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          githubID,
		DisplayName: "GitHub",
		Scopes:      []model.OAuthScope{{ScopeValue: "read:user", Description: "Read user"}, {ScopeValue: "repo", Description: "Repos"}},
		Secret:      model.NewEncryptedSecret([]byte("cipher")),
	}
	google := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          googleID,
		DisplayName: "Google",
		Scopes:      []model.OAuthScope{{ScopeValue: "email", Description: "View email"}},
		Secret:      model.NewEncryptedSecret([]byte("cipher")),
	}

	t.Run("agent not found", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{},
			permissionSetService,
			slog.Default(),
		)
		_, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.ErrorIs(t, err, ErrAgentNotFound)
	})

	t.Run("no service requirements returns empty slice", func(t *testing.T) {
		t.Parallel()
		agentNoReqs := &storage.Agent{ID: agentID, ClientID: ptr.To(id.ClientID("c")), DisplayName: "A", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: permissionSetID, RequirementType: storage.RequirementTypeMandatory}}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agentNoReqs}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{},
			permissionSetService,
			slog.Default(),
		)
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.NoError(t, err)
		assert.Equal(t, agentNoReqs.ID, detail.Agent.ID)
		assert.Empty(t, detail.ServiceRequirements)
	})

	t.Run("connected user shows IsConnected true", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{githubID: github, googleID: google}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{findFunc: func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
				return &storage.UserSession{ID: id.NewSessionID(), ServiceID: githubID}, nil
			}},
			permissionSetService,
			slog.Default(),
		)
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.NoError(t, err)
		require.Len(t, detail.ServiceRequirements, 2)
		for _, r := range detail.ServiceRequirements {
			assert.True(t, r.IsConnected)
		}
	})

	t.Run("no session shows IsConnected false", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{githubID: github, googleID: google}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{},
			permissionSetService,
			slog.Default(),
		)
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.NoError(t, err)
		require.Len(t, detail.ServiceRequirements, 2)
		for _, r := range detail.ServiceRequirements {
			assert.False(t, r.IsConnected)
		}
	})

	t.Run("missing service is skipped", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{githubID: github}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{},
			permissionSetService,
			slog.Default(),
		)
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.NoError(t, err)
		require.Len(t, detail.ServiceRequirements, 1)
		assert.Equal(t, githubID, detail.ServiceRequirements[0].ServiceID)
	})

	t.Run("scope descriptions are populated", func(t *testing.T) {
		t.Parallel()
		singleReqAgent := &storage.Agent{
			ID:       agentID,
			ClientID: ptr.To(id.ClientID("c")),
			ServiceRequirements: []storage.ServiceRequirement{
				{ServiceID: githubID, RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read:user"}},
			},
			PermissionSets: []storage.AgentPermissionSetEntry{{
				PermissionSetID: permissionSetID,
				RequirementType: storage.RequirementTypeMandatory,
			}},
		}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: singleReqAgent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{githubID: github}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{},
			permissionSetService,
			slog.Default(),
		)
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.NoError(t, err)
		require.Len(t, detail.ServiceRequirements, 1)
		require.Len(t, detail.ServiceRequirements[0].RequiredScopes, 1)
		assert.Equal(t, "read:user", detail.ServiceRequirements[0].RequiredScopes[0].Name)
		assert.Equal(t, "Read user", detail.ServiceRequirements[0].RequiredScopes[0].Description)
	})

	t.Run("session lookup error propagates", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{githubID: github, googleID: google}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockSessionRepo{findFunc: func(_ context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
				return nil, storage.NewStorageError("FindByPrincipalAndService", storage.ErrorKindConnection, nil, "db down")
			}},
			permissionSetService,
			slog.Default(),
		)
		_, err := svc.GetAgentConsentDetail(ctx, agentID, principal)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checking session status")
	})
}

func TestService_GetAgentConsentDetail_Errors(t *testing.T) {
	t.Parallel()

	agentID := id.NewAgentID()
	permissionlessAgent := &storage.Agent{ID: agentID}
	tests := []struct {
		name   string
		agents map[id.AgentID]*storage.Agent
		want   error
	}{
		{
			name:   "agent not found",
			agents: map[id.AgentID]*storage.Agent{},
			want:   ErrAgentNotFound,
		},
		{
			name:   "agent without permission sets",
			agents: map[id.AgentID]*storage.Agent{agentID: permissionlessAgent},
			want:   ErrMissingMandatoryPS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&mockAgentRepo{agents: tt.agents}, nil, nil, nil, nil, slog.Default())
			detail, err := svc.GetAgentConsentDetail(context.Background(), agentID, id.Principal("user@example.com"))
			require.ErrorIs(t, err, tt.want)
			assert.Nil(t, detail)
		})
	}
}

func TestService_GrantConsent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agentID := id.NewAgentID()
	serviceID1 := id.NewServiceID()

	permissionSetID := id.NewPermissionSetID()
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("test-client")),
		DisplayName: "Test Agent",
		Description: "A test agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetID,
			RequirementType: storage.RequirementTypeOptional,
		}},
	}

	service1 := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          serviceID1,
		DisplayName: "GitHub",
		ClientID:    id.ClientID("github-client"),
		IssuerURI:   "https://github.com",
		Discovery:   model.DiscoveryConfig{EnableDiscovery: true},
		Scopes: []model.OAuthScope{
			{ScopeValue: "repo", Description: "Repository access"},
			{ScopeValue: "user:email", Description: "Email access"},
		},
		Secret: model.NewEncryptedSecret([]byte("test-ciphertext")),
	}

	t.Run("create new grant", func(t *testing.T) {
		t.Parallel()

		future := time.Now().Add(24 * time.Hour)
		psID := id.NewPermissionSetID()
		includedServiceID := id.NewServiceID()
		sessionID := id.NewSessionID()
		psRepo := &mockPermissionSetService{
			permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
				psID: {
					ID:          psID,
					Name:        "Test PS",
					Description: "A test permission set",
					ServiceScopes: []storage.ServiceScope{
						{ServiceID: includedServiceID, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional},
					},
				},
			},
		}
		sessionRepo := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{
			sessionID: {
				ID:                    sessionID,
				Principal:             id.Principal("user@example.com"),
				ServiceID:             includedServiceID,
				RefreshTokenExpiresAt: &future,
			},
		}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: service1}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			sessionRepo,
			psRepo,
			slog.Default(),
		)

		req := &GrantRequest{
			Principal:             id.Principal("user@example.com"),
			AgentID:               agentID,
			ValidUntil:            &future,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{includedServiceID}}},
		}

		grant, err := svc.GrantConsent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, grant)
		assert.Equal(t, id.Principal("user@example.com"), grant.Principal)
		assert.Equal(t, agentID, grant.AgentID)
		assert.Len(t, grant.GrantedPermissionSets, 1)
	})

	t.Run("scope-less mandatory service requires an active session", func(t *testing.T) {
		psID := id.NewPermissionSetID()
		sessionID := id.NewSessionID()
		scopeLessAgent := &storage.Agent{
			ID:          agentID,
			ClientID:    ptr.To(id.ClientID("scope-less-agent")),
			DisplayName: "Scope-less Agent",
			Description: "Requires a scope-less service",
			ServiceRequirements: []storage.ServiceRequirement{{
				ServiceID:       serviceID1,
				RequirementType: storage.RequirementTypeMandatory,
			}},
			PermissionSets: []storage.AgentPermissionSetEntry{{
				PermissionSetID: psID,
				RequirementType: storage.RequirementTypeMandatory,
			}},
		}
		permissionSet := &storage.PermissionSet{
			ID:          psID,
			Name:        "Scope-less session",
			Description: "Requires only a provider session",
			ServiceScopes: []storage.ServiceScope{{
				ServiceID:       serviceID1,
				RequirementType: storage.RequirementTypeMandatory,
			}},
		}
		newService := func(sessions map[id.SessionID]*storage.UserSession) *Service {
			return NewService(
				&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: scopeLessAgent}},
				newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: {ID: serviceID1, DisplayName: "Scope-less IdP"}}}),
				&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
				&mockUserSessionRepo{sessions: sessions},
				&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: permissionSet}},
				slog.Default(),
			)
		}
		req := &GrantRequest{
			Principal: id.Principal("user@example.com"),
			AgentID:   agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
				PermissionSetID:    psID,
				IncludedServiceIDs: []id.ServiceID{serviceID1},
			}},
		}

		grant, err := newService(map[id.SessionID]*storage.UserSession{}).GrantConsent(ctx, req)
		require.ErrorIs(t, err, ErrUnconnectedServices)
		assert.Nil(t, grant)

		grant, err = newService(map[id.SessionID]*storage.UserSession{sessionID: {
			ID:                    sessionID,
			Principal:             req.Principal,
			ServiceID:             serviceID1,
			RefreshTokenExpiresAt: ptr.To(time.Now().Add(time.Hour)),
		}}).GrantConsent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, grant)
	})

	t.Run("invalid scopes", func(t *testing.T) {
		t.Parallel()

		psID := id.NewPermissionSetID()
		validSvcID := id.NewServiceID()
		invalidSvcID := id.NewServiceID() // not in PS ServiceScopes

		ps := &storage.PermissionSet{
			ID:          psID,
			Name:        "Test PS",
			Description: "A test permission set",
			ServiceScopes: []storage.ServiceScope{
				{ServiceID: validSvcID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
			},
		}
		psRepo := &mockPermissionSetService{
			permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: ps},
		}
		psService := permissionset.NewPermissionSetService(psRepo, &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}, slog.Default())
		defer psService.Close()

		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: service1}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			nil,
			psService,
			slog.Default(),
		)

		req := &GrantRequest{
			Principal:             id.Principal("user@example.com"),
			AgentID:               agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{invalidSvcID}}},
		}

		grant, err := svc.GrantConsent(ctx, req)
		assert.ErrorIs(t, err, ErrInvalidScopes)
		assert.Nil(t, grant)
	})

	t.Run("agent not found", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			nil,
			nil,
			slog.Default(),
		)

		req := &GrantRequest{
			Principal:             id.Principal("user@example.com"),
			AgentID:               id.NewAgentID(),
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
		}

		grant, err := svc.GrantConsent(ctx, req)
		assert.Error(t, err)
		assert.Nil(t, grant)
		assert.ErrorIs(t, err, ErrAgentNotFound)
	})

	t.Run("empty scopes returns ErrGrantValidation", func(t *testing.T) {
		t.Parallel()
		psID := id.NewPermissionSetID()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: service1}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}},
			&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
				psID: {
					ID:            psID,
					Name:          "Test PS",
					Description:   "A test permission set",
					ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID1, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional}},
				},
			}},
			slog.Default(),
		)

		req := &GrantRequest{
			Principal: id.Principal("user@example.com"),
			AgentID:   agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{}},
			},
		}

		_, err := svc.GrantConsent(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGrantValidation)
	})

	t.Run("duplicate scopes returns ErrGrantValidation", func(t *testing.T) {
		t.Parallel()
		psID := id.NewPermissionSetID()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: service1}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}},
			&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
				psID: {
					ID:            psID,
					Name:          "Test PS",
					Description:   "A test permission set",
					ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID1, Scopes: []string{"repo"}, RequirementType: storage.RequirementTypeOptional}},
				},
			}},
			slog.Default(),
		)

		req := &GrantRequest{
			Principal: id.Principal("user@example.com"),
			AgentID:   agentID,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
				{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{}},
			},
		}

		_, err := svc.GrantConsent(ctx, req)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGrantValidation)
	})

	t.Run("matching existing grant is a no-op", func(t *testing.T) {
		t.Parallel()
		existingGrantID := id.NewGrantID()
		createdAt := time.Now().Add(-2 * time.Hour).UTC().Round(time.Second)
		updatedAt := createdAt.Add(30 * time.Minute)
		validUntil := time.Now().Add(24 * time.Hour).UTC().Round(time.Second)

		existingGrant := &storage.UserGrant{
			ID:         existingGrantID,
			Principal:  id.Principal("user@example.com"),
			AgentID:    agentID,
			ValidUntil: &validUntil,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
				PermissionSetID:    permissionSetID,
				IncludedServiceIDs: []id.ServiceID{serviceID1},
			}},
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}

		grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{existingGrantID: existingGrant}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{serviceID1: service1}}),
			grantRepo,
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{id.NewSessionID(): {ID: id.NewSessionID(), Principal: id.Principal("user@example.com"), ServiceID: serviceID1, RefreshTokenExpiresAt: &validUntil}}},
			&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{permissionSetID: {ID: permissionSetID, Name: "Test Permission Set", Description: "Covers the existing grant", ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID1, RequirementType: storage.RequirementTypeOptional}}}}},
			slog.Default(),
		)

		req := &GrantRequest{
			Principal:  id.Principal("user@example.com"),
			AgentID:    agentID,
			ValidUntil: &validUntil,
			GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
				PermissionSetID:    permissionSetID,
				IncludedServiceIDs: []id.ServiceID{serviceID1},
			}},
		}

		grant, err := svc.GrantConsent(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, grant)
		assert.Equal(t, existingGrantID, grant.ID)
		assert.Equal(t, updatedAt, grant.UpdatedAt)
		assert.Equal(t, 0, grantRepo.updateCalls)
		assert.Equal(t, 0, grantRepo.createCalls)
	})

}

func TestService_GrantConsent_RejectsMissingPermissionSets(t *testing.T) {
	t.Parallel()

	agentID := id.NewAgentID()
	permissionSetID := id.NewPermissionSetID()
	serviceID := id.NewServiceID()
	grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}
	svc := NewService(
		&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: {
			ID: agentID,
		}}},
		nil,
		grantRepo,
		nil,
		&mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
			permissionSetID: {
				ID:          permissionSetID,
				Name:        "Permissionless agent test set",
				Description: "Allows the request to reach the agent guard",
				ServiceScopes: []storage.ServiceScope{{
					ServiceID:       serviceID,
					RequirementType: storage.RequirementTypeOptional,
				}},
			},
		}},
		slog.Default(),
	)

	grant, err := svc.GrantConsent(context.Background(), &GrantRequest{
		Principal: id.Principal("user@example.com"),
		AgentID:   agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
			PermissionSetID:    permissionSetID,
			IncludedServiceIDs: []id.ServiceID{serviceID},
		}},
	})
	require.ErrorIs(t, err, ErrMissingMandatoryPS)
	assert.Nil(t, grant)
	assert.Zero(t, grantRepo.createCalls)
	assert.Zero(t, grantRepo.updateCalls)
}

// toctouPermissionSetQuerier is a GrantConsent test double that simulates a TOCTOU race:
// ValidateIDs reports the permission set as valid, but GetByIDs returns an empty list
// (simulating the PS being deleted between the two calls).
type toctouPermissionSetQuerier struct {
	psID id.PermissionSetID
}

func (q *toctouPermissionSetQuerier) ValidateIDs(_ context.Context, _ []id.PermissionSetID) error {
	return nil
}

func (q *toctouPermissionSetQuerier) GetByIDs(_ context.Context, _ []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	return nil, nil
}

// TestService_GrantConsent_TOCTOUPermissionSetDeleted verifies that GrantConsent rejects
// a consent request when a permission set disappears between ValidateIDs and GetByIDs.
func TestService_GrantConsent_TOCTOUPermissionSetDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	psID := id.NewPermissionSetID()
	svcID := id.NewServiceID()

	tests := []struct {
		name  string
		agent *storage.Agent
	}{
		{
			name: "agent with declared PermissionSets",
			agent: &storage.Agent{
				ID:          id.NewAgentID(),
				ClientID:    ptr.To(id.ClientID("declared-agent")),
				DisplayName: "Declared Agent",
				PermissionSets: []storage.AgentPermissionSetEntry{
					{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := &GrantRequest{
				Principal: id.Principal("user@example.com"),
				AgentID:   tc.agent.ID,
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			}

			// toctouPermissionSetQuerier makes ValidateIDs pass but GetByIDs return empty,
			// simulating a concurrent deletion between the two calls.
			svc := NewService(
				&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{tc.agent.ID: tc.agent}},
				newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
				&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
				nil,
				nil,
				slog.Default(),
			)
			svc.psService = &toctouPermissionSetQuerier{psID: psID}

			grant, err := svc.GrantConsent(ctx, req)
			require.Error(t, err)
			assert.Nil(t, grant)
			assert.ErrorIs(t, err, ErrInvalidServiceInclusion)
		})
	}
}

// TestService_RevokeConsentForPrincipal tests the user-facing revoke method (FR-014).
// This is the dedicated method for DELETE /api/consent/agent/{agent-id}/grants — non-idempotent,
// maps storage not-found to ErrGrantNotFound so the handler can return 404.
func TestService_RevokeConsentForPrincipal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agentID := id.NewAgentID()
	grantID := id.NewGrantID()

	existingGrant := &storage.UserGrant{
		ID:                    grantID,
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	t.Run("success: grant is deleted", func(t *testing.T) {
		t.Parallel()
		grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{grantID: existingGrant}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			grantRepo,
			nil,
			nil,
			slog.Default(),
		)

		err := svc.RevokeConsentForPrincipal(ctx, id.Principal("user@example.com"), agentID)
		require.NoError(t, err)
		assert.Empty(t, grantRepo.grants)
	})

	t.Run("grant not found: returns ErrGrantNotFound (not raw ErrNotFound)", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			nil,
			nil,
			slog.Default(),
		)

		err := svc.RevokeConsentForPrincipal(ctx, id.Principal("user@example.com"), agentID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGrantNotFound, "must surface domain sentinel, not raw storage error")
	})

	t.Run("cross-principal: different principal cannot revoke another's grant (SR-001)", func(t *testing.T) {
		t.Parallel()
		grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{grantID: existingGrant}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			grantRepo,
			nil,
			nil,
			slog.Default(),
		)

		// Different principal has no grant for this agent
		err := svc.RevokeConsentForPrincipal(ctx, id.Principal("other@example.com"), agentID)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrGrantNotFound)

		// Original grant still exists
		assert.Len(t, grantRepo.grants, 1)
	})
}

func TestService_RevokeConsent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agentID := id.NewAgentID()
	grantID := id.NewGrantID()

	existingGrant := &storage.UserGrant{
		ID:                    grantID,
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{grantID: existingGrant}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			grantRepo,
			nil,
			nil,
			slog.Default(),
		)

		err := svc.RevokeConsent(ctx, id.Principal("user@example.com"), agentID)
		require.NoError(t, err)
		assert.Empty(t, grantRepo.grants)
	})

	t.Run("grant not found - returns nil (idempotent: POST empty-tokens path)", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			nil,
			nil,
			slog.Default(),
		)

		err := svc.RevokeConsent(ctx, id.Principal("user@example.com"), agentID)
		require.NoError(t, err) // Idempotent: absence is not an error
	})
}

func TestService_GetActiveGrants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agentID := id.NewAgentID()
	grantID1 := id.NewGrantID()
	grantID2 := id.NewGrantID()
	grantID3 := id.NewGrantID()

	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	activeGrant := &storage.UserGrant{
		ID:                    grantID1,
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            &future,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	expiredGrant := &storage.UserGrant{
		ID:                    grantID2,
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            &past,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	indefiniteGrant := &storage.UserGrant{
		ID:                    grantID3,
		Principal:             id.Principal("user@example.com"),
		AgentID:               agentID,
		ValidUntil:            nil,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
	}

	t.Run("filters expired grants", func(t *testing.T) {
		t.Parallel()
		grantRepo := &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{
			grantID1: activeGrant,
			grantID2: expiredGrant,
			grantID3: indefiniteGrant,
		}}
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			grantRepo,
			nil,
			nil,
			slog.Default(),
		)

		grants, err := svc.GetActiveGrants(ctx, id.Principal("user@example.com"), agentID)
		require.NoError(t, err)
		// Should return only active and indefinite grants (not expired)
		assert.Len(t, grants, 2)
	})

	t.Run("no grants - returns empty slice", func(t *testing.T) {
		t.Parallel()
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			nil,
			nil,
			slog.Default(),
		)

		grants, err := svc.GetActiveGrants(ctx, id.Principal("user@example.com"), agentID)
		require.NoError(t, err)
		assert.Empty(t, grants)
	})
}

func TestService_GetAgentDelegations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agent1ID := id.NewAgentID()
	agent2ID := id.NewAgentID()
	grantID1 := id.NewGrantID()
	grantID2 := id.NewGrantID()

	agent1 := &storage.Agent{
		ID:          agent1ID,
		ClientID:    ptr.To(id.ClientID("test-client-1")),
		DisplayName: "Test Agent 1",
		Description: "First test agent",
	}

	agent2 := &storage.Agent{
		ID:          agent2ID,
		ClientID:    ptr.To(id.ClientID("test-client-2")),
		DisplayName: "Test Agent 2",
		Description: "Second test agent",
	}

	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	tests := []struct {
		name          string
		principal     id.Principal
		grants        map[id.GrantID]*storage.UserGrant
		agents        map[id.AgentID]*storage.Agent
		expectedCount int
		expectError   bool
		validate      func(t *testing.T, delegations []AgentDelegation)
	}{
		{
			name:          "empty grants returns empty list",
			principal:     id.Principal("user@example.com"),
			grants:        map[id.GrantID]*storage.UserGrant{},
			agents:        map[id.AgentID]*storage.Agent{agent1ID: agent1},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:      "single grant returns one delegation",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				grantID1: {
					ID:                    grantID1,
					Principal:             id.Principal("user@example.com"),
					AgentID:               agent1ID,
					ValidUntil:            &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now,
					UpdatedAt:             now,
				},
			},
			agents:        map[id.AgentID]*storage.Agent{agent1ID: agent1},
			expectedCount: 1,
			expectError:   false,
			validate: func(t *testing.T, delegations []AgentDelegation) {
				require.Len(t, delegations, 1)
				assert.Equal(t, agent1ID, delegations[0].AgentID)
				assert.Equal(t, "Test Agent 1", delegations[0].DisplayName)
				assert.Equal(t, 1, delegations[0].ActiveGrantCount)
			},
		},
		{
			name:      "multiple grants for same agent groups correctly",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				grantID1: {
					ID:                    grantID1,
					Principal:             id.Principal("user@example.com"),
					AgentID:               agent1ID,
					ValidUntil:            &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now,
					UpdatedAt:             now,
				},
			},
			agents:        map[id.AgentID]*storage.Agent{agent1ID: agent1},
			expectedCount: 1,
			expectError:   false,
			validate: func(t *testing.T, delegations []AgentDelegation) {
				require.Len(t, delegations, 1)
				assert.Equal(t, agent1ID, delegations[0].AgentID)
				assert.Equal(t, 1, delegations[0].ActiveGrantCount)
			},
		},
		{
			name:      "multiple agents returns multiple delegations",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				grantID1: {
					ID:                    grantID1,
					Principal:             id.Principal("user@example.com"),
					AgentID:               agent1ID,
					ValidUntil:            &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now,
					UpdatedAt:             now,
				},
				grantID2: {
					ID:                    grantID2,
					Principal:             id.Principal("user@example.com"),
					AgentID:               agent2ID,
					ValidUntil:            &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now,
					UpdatedAt:             now.Add(1 * time.Hour),
				},
			},
			agents: map[id.AgentID]*storage.Agent{
				agent1ID: agent1,
				agent2ID: agent2,
			},
			expectedCount: 2,
			expectError:   false,
			validate: func(t *testing.T, delegations []AgentDelegation) {
				require.Len(t, delegations, 2)

				// Find each agent in results
				var agent1Delegation, agent2Delegation *AgentDelegation
				for i := range delegations {
					if delegations[i].AgentID == agent1ID {
						agent1Delegation = &delegations[i]
					}
					if delegations[i].AgentID == agent2ID {
						agent2Delegation = &delegations[i]
					}
				}

				require.NotNil(t, agent1Delegation)
				require.NotNil(t, agent2Delegation)
				assert.Equal(t, "Test Agent 1", agent1Delegation.DisplayName)
				assert.Equal(t, "Test Agent 2", agent2Delegation.DisplayName)
				assert.Equal(t, 1, agent1Delegation.ActiveGrantCount)
				assert.Equal(t, 1, agent2Delegation.ActiveGrantCount)
			},
		},
		{
			name:      "multiple agents are ordered by display name and agent ID",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				id.NewGrantID(): {Principal: id.Principal("user@example.com"), AgentID: id.MustParseAgentID("00000000-0000-0000-0000-000000000004"), ValidUntil: &future},
				id.NewGrantID(): {Principal: id.Principal("user@example.com"), AgentID: id.MustParseAgentID("00000000-0000-0000-0000-000000000003"), ValidUntil: &future},
				id.NewGrantID(): {Principal: id.Principal("user@example.com"), AgentID: id.MustParseAgentID("00000000-0000-0000-0000-000000000005"), ValidUntil: &future},
				id.NewGrantID(): {Principal: id.Principal("user@example.com"), AgentID: id.MustParseAgentID("00000000-0000-0000-0000-000000000002"), ValidUntil: &future},
				id.NewGrantID(): {Principal: id.Principal("user@example.com"), AgentID: id.MustParseAgentID("00000000-0000-0000-0000-000000000001"), ValidUntil: &future},
			},
			agents: map[id.AgentID]*storage.Agent{
				id.MustParseAgentID("00000000-0000-0000-0000-000000000004"): {ID: id.MustParseAgentID("00000000-0000-0000-0000-000000000004"), DisplayName: "Zebra Agent"},
				id.MustParseAgentID("00000000-0000-0000-0000-000000000003"): {ID: id.MustParseAgentID("00000000-0000-0000-0000-000000000003"), DisplayName: "Shared Name"},
				id.MustParseAgentID("00000000-0000-0000-0000-000000000005"): {ID: id.MustParseAgentID("00000000-0000-0000-0000-000000000005"), DisplayName: "Alpha Agent"},
				id.MustParseAgentID("00000000-0000-0000-0000-000000000002"): {ID: id.MustParseAgentID("00000000-0000-0000-0000-000000000002"), DisplayName: "Shared Name"},
				id.MustParseAgentID("00000000-0000-0000-0000-000000000001"): {ID: id.MustParseAgentID("00000000-0000-0000-0000-000000000001"), DisplayName: "Shared Name"},
			},
			expectedCount: 5,
			expectError:   false,
			validate: func(t *testing.T, delegations []AgentDelegation) {
				assert.Equal(t, []id.AgentID{
					id.MustParseAgentID("00000000-0000-0000-0000-000000000005"),
					id.MustParseAgentID("00000000-0000-0000-0000-000000000001"),
					id.MustParseAgentID("00000000-0000-0000-0000-000000000002"),
					id.MustParseAgentID("00000000-0000-0000-0000-000000000003"),
					id.MustParseAgentID("00000000-0000-0000-0000-000000000004"),
				}, []id.AgentID{
					delegations[0].AgentID,
					delegations[1].AgentID,
					delegations[2].AgentID,
					delegations[3].AgentID,
					delegations[4].AgentID,
				})
			},
		},
		{
			name:      "expired grants are filtered out",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				grantID1: {
					ID:                    grantID1,
					Principal:             id.Principal("user@example.com"),
					AgentID:               agent1ID,
					ValidUntil:            &past, // Expired
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now.Add(-48 * time.Hour),
					UpdatedAt:             now.Add(-48 * time.Hour),
				},
			},
			agents:        map[id.AgentID]*storage.Agent{agent1ID: agent1},
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:      "grants for different principal are not included",
			principal: id.Principal("user@example.com"),
			grants: map[id.GrantID]*storage.UserGrant{
				grantID1: {
					ID:                    grantID1,
					Principal:             id.Principal("other@example.com"),
					AgentID:               agent1ID,
					ValidUntil:            &future,
					GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}}},
					CreatedAt:             now,
					UpdatedAt:             now,
				},
			},
			agents:        map[id.AgentID]*storage.Agent{agent1ID: agent1},
			expectedCount: 0,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(
				&mockAgentRepo{agents: tt.agents},
				newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
				&mockGrantRepo{grants: tt.grants},
				nil,
				nil,
				slog.Default(),
			)

			delegations, err := svc.GetAgentDelegations(ctx, tt.principal)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, delegations, tt.expectedCount)

			if tt.validate != nil {
				tt.validate(t, delegations)
			}
		})
	}
}

func TestSortAgentDelegations(t *testing.T) {
	t.Parallel()

	oldest := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	newest := oldest.Add(4 * time.Hour)
	delegations := []AgentDelegation{
		{
			AgentID:        id.MustParseAgentID("00000000-0000-0000-0000-000000000004"),
			DisplayName:    "Zebra Agent",
			LastModifiedAt: newest,
		},
		{
			AgentID:        id.MustParseAgentID("00000000-0000-0000-0000-000000000003"),
			DisplayName:    "Shared Name",
			LastModifiedAt: oldest.Add(time.Hour),
		},
		{
			AgentID:        id.MustParseAgentID("00000000-0000-0000-0000-000000000001"),
			DisplayName:    "Shared Name",
			LastModifiedAt: oldest.Add(3 * time.Hour),
		},
		{
			AgentID:        id.MustParseAgentID("00000000-0000-0000-0000-000000000002"),
			DisplayName:    "Shared Name",
			LastModifiedAt: oldest.Add(2 * time.Hour),
		},
		{
			AgentID:        id.MustParseAgentID("00000000-0000-0000-0000-000000000005"),
			DisplayName:    "Alpha Agent",
			LastModifiedAt: oldest,
		},
	}

	sortAgentDelegations(delegations)

	assert.Equal(t, []id.AgentID{
		id.MustParseAgentID("00000000-0000-0000-0000-000000000005"),
		id.MustParseAgentID("00000000-0000-0000-0000-000000000001"),
		id.MustParseAgentID("00000000-0000-0000-0000-000000000002"),
		id.MustParseAgentID("00000000-0000-0000-0000-000000000003"),
		id.MustParseAgentID("00000000-0000-0000-0000-000000000004"),
	}, []id.AgentID{
		delegations[0].AgentID,
		delegations[1].AgentID,
		delegations[2].AgentID,
		delegations[3].AgentID,
		delegations[4].AgentID,
	})
}

func TestService_GetAgentConsentDetail_WithPermissionSets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	agentID := id.NewAgentID()
	serviceID1 := id.NewServiceID()
	sessionID1 := id.NewSessionID()

	now := time.Now()
	future := now.Add(24 * time.Hour)
	permissionSetID := id.NewPermissionSetID()
	permissionSetService := &mockPermissionSetService{permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
		permissionSetID: {ID: permissionSetID, Name: "Test Permission Set", Description: "Used by detail tests"},
	}}

	session1 := &storage.UserSession{
		ID:                    sessionID1,
		Principal:             id.Principal("user@example.com"),
		ServiceID:             serviceID1,
		RefreshTokenExpiresAt: &future,
		TokenType:             "Bearer",
		EncryptedAccessToken:  []byte("test-token"),
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID1},
		InitiatedAt:           now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	t.Run("returns active session service IDs", func(t *testing.T) {
		t.Parallel()

		agent := &storage.Agent{
			ID:          agentID,
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			PermissionSets: []storage.AgentPermissionSetEntry{{
				PermissionSetID: permissionSetID,
				RequirementType: storage.RequirementTypeOptional,
			}},
		}

		sessionRepo := &mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{
			sessionID1: session1,
		}}

		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			sessionRepo,
			permissionSetService,
			slog.Default(),
		)

		principal := id.Principal("user@example.com")
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)

		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Equal(t, agentID, detail.Agent.ID)
		require.NotNil(t, detail.ActiveSessionServiceIDs)
		assert.Len(t, detail.ActiveSessionServiceIDs, 1)
		assert.Equal(t, serviceID1, detail.ActiveSessionServiceIDs[0])
	})

	t.Run("handles missing sessions gracefully", func(t *testing.T) {
		t.Parallel()

		agent := &storage.Agent{
			ID:          agentID,
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			PermissionSets: []storage.AgentPermissionSetEntry{{
				PermissionSetID: permissionSetID,
				RequirementType: storage.RequirementTypeOptional,
			}},
		}

		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agent}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}},
			permissionSetService,
			slog.Default(),
		)

		principal := id.Principal("user@example.com")
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)

		require.NoError(t, err)
		require.NotNil(t, detail)
		assert.Empty(t, detail.ActiveSessionServiceIDs)
	})

	t.Run("returns resolved permission sets when psService is wired", func(t *testing.T) {
		t.Parallel()

		psID := id.NewPermissionSetID()
		ps := &storage.PermissionSet{
			ID:          psID,
			Name:        "GitHub Read",
			Description: "Read GitHub repos",
			ServiceScopes: []storage.ServiceScope{
				{ServiceID: serviceID1, Scopes: []string{"repo:read"}, RequirementType: storage.RequirementTypeOptional},
			},
		}

		psRepo := &mockPermissionSetService{
			permissionSets: map[id.PermissionSetID]*storage.PermissionSet{psID: ps},
		}
		psService := permissionset.NewPermissionSetService(psRepo, &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}, slog.Default())

		agentWithPS := &storage.Agent{
			ID:          agentID,
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			PermissionSets: []storage.AgentPermissionSetEntry{
				{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory},
			},
		}

		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agentWithPS}},
			newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{}}),
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}},
			psService,
			slog.Default(),
		)

		principal := id.Principal("user@example.com")
		detail, err := svc.GetAgentConsentDetail(ctx, agentID, principal)

		require.NoError(t, err)
		require.NotNil(t, detail)
		require.Len(t, detail.ResolvedPermissionSets, 1)
		assert.Equal(t, psID, detail.ResolvedPermissionSets[0].PermissionSet.ID)
		assert.Equal(t, storage.RequirementTypeMandatory, detail.ResolvedPermissionSets[0].RequirementType)
	})

	t.Run("discloses the all-scopes union for the required service only", func(t *testing.T) {
		t.Parallel()

		serviceID2 := id.NewServiceID()
		psID1 := id.NewPermissionSetID()
		psID2 := id.NewPermissionSetID()
		psService := permissionset.NewPermissionSetService(&mockPermissionSetService{
			permissionSets: map[id.PermissionSetID]*storage.PermissionSet{
				psID1: {
					ID: psID1,
					ServiceScopes: []storage.ServiceScope{
						{ServiceID: serviceID1, Scopes: []string{"write", "read"}},
						{ServiceID: serviceID2, Scopes: []string{"unrelated"}},
					},
				},
				psID2: {
					ID: psID2,
					ServiceScopes: []storage.ServiceScope{
						{ServiceID: serviceID1, Scopes: []string{"admin", "read"}},
					},
				},
			},
		}, &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}, slog.Default())
		defer psService.Close()

		agentWithAllScopes := &storage.Agent{
			ID:          agentID,
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			ServiceRequirements: []storage.ServiceRequirement{{
				ServiceID:        serviceID1,
				RequirementType:  storage.RequirementTypeMandatory,
				RequireAllScopes: true,
			}},
			PermissionSets: []storage.AgentPermissionSetEntry{
				{PermissionSetID: psID1, RequirementType: storage.RequirementTypeMandatory},
				{PermissionSetID: psID2, RequirementType: storage.RequirementTypeOptional},
			},
		}
		providerService := newTestProviderService(&mockServiceRepo{services: map[id.ServiceID]*model.ThirdpartyOAuth2ProviderEntity{
			serviceID1: {
				ID: serviceID1,
				Scopes: []model.OAuthScope{
					{ScopeValue: "admin", Description: "Admin access"},
					{ScopeValue: "read", Description: "Read access"},
					{ScopeValue: "write", Description: "Write access"},
				},
			},
		}})
		svc := NewService(
			&mockAgentRepo{agents: map[id.AgentID]*storage.Agent{agentID: agentWithAllScopes}},
			providerService,
			&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
			&mockUserSessionRepo{sessions: map[id.SessionID]*storage.UserSession{}},
			psService,
			slog.Default(),
		)

		detail, err := svc.GetAgentConsentDetail(ctx, agentID, id.Principal("user@example.com"))

		require.NoError(t, err)
		require.Len(t, detail.ServiceRequirements, 1)
		assert.Equal(t, []ServiceScopeInfo{
			{Name: "admin", Description: "Admin access"},
			{Name: "read", Description: "Read access"},
			{Name: "write", Description: "Write access"},
		}, detail.ServiceRequirements[0].RequiredScopes)
	})
}

func TestService_ValidateSubmission(t *testing.T) {
	t.Parallel()

	svcID1 := id.NewServiceID()
	svcID2 := id.NewServiceID()
	svcID3 := id.NewServiceID()
	psID := id.NewPermissionSetID()

	agent := &storage.Agent{
		ID:          id.NewAgentID(),
		ClientID:    ptr.To(id.ClientID("test-client")),
		DisplayName: "Test Agent",
		Description: "Test agent",
	}

	svc := NewService(
		&mockAgentRepo{},
		nil,
		&mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}},
		nil,
		nil,
		slog.Default(),
	)

	t.Run("passes when all included services have active sessions", func(t *testing.T) {
		t.Parallel()

		grantedPS := []storage.GrantedPermissionSetEntry{
			{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID1, svcID2}},
		}
		activeSessionServiceIDs := []id.ServiceID{svcID1, svcID2, svcID3}

		err := svc.ValidateSubmission(context.Background(), agent, grantedPS, activeSessionServiceIDs)
		assert.NoError(t, err)
	})

	t.Run("fails when included service lacks active session", func(t *testing.T) {
		t.Parallel()

		grantedPS := []storage.GrantedPermissionSetEntry{
			{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID1, svcID2}},
		}
		activeSessionServiceIDs := []id.ServiceID{svcID1} // svcID2 not connected

		err := svc.ValidateSubmission(context.Background(), agent, grantedPS, activeSessionServiceIDs)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnconnectedServices)
		assert.Contains(t, err.Error(), svcID2.String())
	})

	t.Run("passes with empty granted permission sets", func(t *testing.T) {
		t.Parallel()

		err := svc.ValidateSubmission(context.Background(), agent, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("checks services across multiple permission sets", func(t *testing.T) {
		t.Parallel()

		psID2 := id.NewPermissionSetID()
		grantedPS := []storage.GrantedPermissionSetEntry{
			{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID1}},
			{PermissionSetID: psID2, IncludedServiceIDs: []id.ServiceID{svcID2, svcID3}},
		}
		activeSessionServiceIDs := []id.ServiceID{svcID1, svcID2} // svcID3 not connected

		err := svc.ValidateSubmission(context.Background(), agent, grantedPS, activeSessionServiceIDs)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUnconnectedServices)
		assert.Contains(t, err.Error(), svcID3.String())
	})
}
