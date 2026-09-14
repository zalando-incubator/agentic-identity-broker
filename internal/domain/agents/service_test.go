package agents

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- hand-rolled mocks (domain convention) ---

type mockAgentRepo struct {
	agents map[id.AgentID]*storage.Agent

	createFn         func(ctx context.Context, agent *storage.Agent) error
	getFn            func(ctx context.Context, id id.AgentID) (*storage.Agent, error)
	updateFn         func(ctx context.Context, agent *storage.Agent) error
	deleteFn         func(ctx context.Context, id id.AgentID) error
	listFn           func(ctx context.Context) ([]*storage.Agent, error)
	getByClientIDFn  func(ctx context.Context, clientID id.ClientID) (*storage.Agent, error)
	existsOtherFn    func(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) (bool, error)
	getByClientURIFn func(ctx context.Context, uri string) (*storage.Agent, error)
}

func newMockAgentRepo() *mockAgentRepo {
	return &mockAgentRepo{agents: make(map[id.AgentID]*storage.Agent)}
}

func (m *mockAgentRepo) Create(ctx context.Context, agent *storage.Agent) error {
	if m.createFn != nil {
		return m.createFn(ctx, agent)
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepo) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	if m.getFn != nil {
		return m.getFn(ctx, agentID)
	}
	a, ok := m.agents[agentID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	return a.Copy(), nil
}

func (m *mockAgentRepo) Update(ctx context.Context, agent *storage.Agent) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, agent)
	}
	m.agents[agent.ID] = agent
	return nil
}

func (m *mockAgentRepo) Delete(ctx context.Context, agentID id.AgentID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, agentID)
	}
	delete(m.agents, agentID)
	return nil
}

func (m *mockAgentRepo) List(ctx context.Context) ([]*storage.Agent, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockAgentRepo) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	if m.getByClientIDFn != nil {
		return m.getByClientIDFn(ctx, clientID)
	}
	for _, a := range m.agents {
		if a.ClientID != nil && *a.ClientID == clientID {
			return a.Copy(), nil
		}
	}
	return nil, storage.NewStorageError("GetByClientID", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
}

func (m *mockAgentRepo) ExistsOtherWithClientID(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) (bool, error) {
	if m.existsOtherFn != nil {
		return m.existsOtherFn(ctx, clientID, excludeAgentID)
	}
	for _, a := range m.agents {
		if a.ClientID != nil && *a.ClientID == clientID {
			if excludeAgentID == nil || a.ID != *excludeAgentID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockAgentRepo) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	if m.getByClientURIFn != nil {
		return m.getByClientURIFn(ctx, uri)
	}
	return nil, storage.NewStorageError("GetByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "not found")
}

func (m *mockAgentRepo) GetByCanonicalID(_ context.Context, canonicalID string) (*storage.Agent, error) {
	for _, agent := range m.agents {
		if agent.CanonicalID != nil && *agent.CanonicalID == canonicalID {
			return agent.Copy(), nil
		}
	}
	return nil, storage.NewStorageError("GetByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
}

type mockServiceReqValidator struct {
	err error
}

func (m *mockServiceReqValidator) ValidateServiceRequirements(_ context.Context, _ []storage.ServiceRequirement) error {
	return m.err
}

func newTestService(repo ports.AgentRepository, multiAgent bool) *Service {
	return NewService(repo, &mockServiceReqValidator{}, slog.Default(), multiAgent)
}

func testPermissionSets() []storage.AgentPermissionSetEntry {
	return []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeOptional,
	}}
}

// --- Create tests ---

func TestCreate_NilClientIDRemainsNil(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	agent := &storage.Agent{
		DisplayName: "Test Agent",
		Description: "A test agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: id.NewPermissionSetID(),
			RequirementType: storage.RequirementTypeOptional,
		}},
	}

	err := svc.Create(context.Background(), agent)
	require.NoError(t, err)

	assert.False(t, agent.ID.IsZero(), "ID should be generated")
	assert.Nil(t, agent.ClientID, "client_id should remain nil when not provided")
}

func TestCreate_UsesProvidedClientID(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	agent := &storage.Agent{
		ClientID:       ptr.To(id.ClientID("my-custom-client")),
		DisplayName:    "Test Agent",
		Description:    "A test agent",
		PermissionSets: testPermissionSets(),
	}

	err := svc.Create(context.Background(), agent)
	require.NoError(t, err)
	assert.Equal(t, ptr.To(id.ClientID("my-custom-client")), agent.ClientID)
}

func TestCreate_EnforcesUniquenessWhenMultiAgentDisabled(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	existing := &storage.Agent{
		ID:          id.NewAgentID(),
		ClientID:    ptr.To(id.ClientID("taken-client")),
		DisplayName: "Existing",
		Description: "Already here",
	}
	repo.agents[existing.ID] = existing

	agent := &storage.Agent{
		ClientID:       ptr.To(id.ClientID("taken-client")),
		DisplayName:    "New Agent",
		Description:    "Wants same client_id",
		PermissionSets: testPermissionSets(),
	}

	err := svc.Create(context.Background(), agent)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

func TestCreate_SkipsUniquenessWhenClientIDIsOmitted(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	// No client_id provided → client_id remains nil → uniqueness check skipped
	agent := &storage.Agent{
		DisplayName:    "Agent Without Client ID",
		Description:    "Nil client_id",
		PermissionSets: testPermissionSets(),
	}

	err := svc.Create(context.Background(), agent)
	require.NoError(t, err)
}

func TestCreate_AllowsDuplicatesWhenMultiAgentEnabled(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, true)

	existing := &storage.Agent{
		ID:          id.NewAgentID(),
		ClientID:    ptr.To(id.ClientID("shared-client")),
		DisplayName: "Agent A",
		Description: "First agent",
	}
	repo.agents[existing.ID] = existing

	agent := &storage.Agent{
		ClientID:       ptr.To(id.ClientID("shared-client")),
		DisplayName:    "Agent B",
		Description:    "Second agent, same client_id",
		PermissionSets: testPermissionSets(),
	}

	err := svc.Create(context.Background(), agent)
	require.NoError(t, err)
}

// --- Update tests ---

func TestUpdate_PreservesExistingClientID(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, true)

	agentID := id.NewAgentID()
	existing := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("original-client")),
		DisplayName: "Original",
		Description: "Original desc",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	repo.agents[agentID] = existing

	update := &storage.Agent{
		// ClientID intentionally empty → should preserve "original-client"
		DisplayName:    "Updated",
		Description:    "Updated desc",
		PermissionSets: testPermissionSets(),
		UpdatedAt:      time.Now().UTC(),
	}

	err := svc.Update(context.Background(), agentID, update, false)
	require.NoError(t, err)
	assert.Equal(t, ptr.To(id.ClientID("original-client")), update.ClientID)
	assert.Equal(t, existing.CreatedAt, update.CreatedAt, "CreatedAt should be preserved")
}

func TestUpdate_ClearsCanonicalID(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, true)
	agentID := id.NewAgentID()
	canonicalID := "research-agent"
	repo.agents[agentID] = &storage.Agent{
		ID:          agentID,
		CanonicalID: &canonicalID,
		DisplayName: "Original",
		Description: "Original description",
		CreatedAt:   time.Now().UTC(),
	}

	update := &storage.Agent{
		ClearCanonicalID: true,
		DisplayName:      "Updated",
		Description:      "Updated description",
		PermissionSets:   testPermissionSets(),
		UpdatedAt:        time.Now().UTC(),
	}

	require.NoError(t, svc.Update(context.Background(), agentID, update, false))
	assert.Nil(t, update.CanonicalID)
}

func TestUpdate_EnforcesUniquenessOnClientIDChange(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	agentA := &storage.Agent{
		ID:          id.NewAgentID(),
		ClientID:    ptr.To(id.ClientID("client-a")),
		DisplayName: "Agent A",
		Description: "desc",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	agentB := &storage.Agent{
		ID:          id.NewAgentID(),
		ClientID:    ptr.To(id.ClientID("client-b")),
		DisplayName: "Agent B",
		Description: "desc",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	repo.agents[agentA.ID] = agentA
	repo.agents[agentB.ID] = agentB

	update := &storage.Agent{
		ClientID:       ptr.To(id.ClientID("client-a")), // conflicts with agentA
		DisplayName:    "Agent B Updated",
		Description:    "desc",
		PermissionSets: testPermissionSets(),
		UpdatedAt:      time.Now().UTC(),
	}

	err := svc.Update(context.Background(), agentB.ID, update, false)
	require.Error(t, err)

	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
}

// --- ResolveUniqueByClientID tests ---

func TestResolveUniqueByClientID_Success(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	agentID := id.NewAgentID()
	agent := &storage.Agent{
		ID:          agentID,
		ClientID:    ptr.To(id.ClientID("unique-client")),
		DisplayName: "Agent",
		Description: "desc",
	}
	repo.agents[agentID] = agent

	resolved, err := svc.ResolveUniqueByClientID(context.Background(), "unique-client")
	require.NoError(t, err)
	assert.Equal(t, agentID, resolved.ID)
}

func TestResolveUniqueByClientID_Ambiguous(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	a1 := &storage.Agent{ID: id.NewAgentID(), ClientID: ptr.To(id.ClientID("shared")), DisplayName: "A1", Description: "d"}
	a2 := &storage.Agent{ID: id.NewAgentID(), ClientID: ptr.To(id.ClientID("shared")), DisplayName: "A2", Description: "d"}
	repo.agents[a1.ID] = a1
	repo.agents[a2.ID] = a2

	// Mock GetByClientID to return first match
	repo.getByClientIDFn = func(_ context.Context, cid id.ClientID) (*storage.Agent, error) {
		return a1.Copy(), nil
	}

	_, err := svc.ResolveUniqueByClientID(context.Background(), "shared")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestResolveUniqueByClientID_NotFound(t *testing.T) {
	repo := newMockAgentRepo()
	svc := newTestService(repo, false)

	_, err := svc.ResolveUniqueByClientID(context.Background(), "nonexistent")
	require.Error(t, err)
}

func TestResolveIDAcceptsUUIDCanonicalAndRejectsUnknown(t *testing.T) {
	repo := newMockAgentRepo()
	service := newTestService(repo, true)
	canonicalID := "research-agent"
	agent := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &canonicalID, DisplayName: "Research", Description: "Research agent"}
	repo.agents[agent.ID] = agent

	for _, value := range []string{agent.ID.String(), canonicalID} {
		resolved, err := service.ResolveID(context.Background(), value)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, resolved)
	}
	_, err := service.ResolveID(context.Background(), "unknown-agent")
	require.Error(t, err)
}
