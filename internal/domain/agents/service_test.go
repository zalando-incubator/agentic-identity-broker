package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	ports.AgentRepository
	agents map[id.AgentID]*storage.Agent

	createFn         func(ctx context.Context, agent *storage.Agent) error
	getFn            func(ctx context.Context, id id.AgentID) (*storage.Agent, error)
	updateFn         func(ctx context.Context, agent *storage.Agent) error
	deleteFn         func(ctx context.Context, id id.AgentID) (bool, error)
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

func (m *mockAgentRepo) Delete(ctx context.Context, agentID id.AgentID) (bool, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, agentID)
	}
	_, exists := m.agents[agentID]
	delete(m.agents, agentID)
	return exists, nil
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
	assert.ErrorIs(t, err, ErrAmbiguousClientID)
	assert.NotContains(t, err.Error(), "shared")
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

func TestDeleteLogsOnlyActualDeletion(t *testing.T) {
	t.Parallel()
	deleteCause := errors.New("agent delete failed")
	for _, tc := range []struct {
		name    string
		present bool
		err     error
	}{
		{name: "existing then absent", present: true},
		{name: "absent"},
		{name: "repository failure", present: true, err: deleteCause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agentID := id.NewAgentID()
			repo := newMockAgentRepo()
			if tc.present {
				repo.agents[agentID] = &storage.Agent{ID: agentID}
			}
			if tc.err != nil {
				repo.deleteFn = func(context.Context, id.AgentID) (bool, error) { return false, tc.err }
			}
			var logs bytes.Buffer
			svc := NewService(repo, nil, slog.New(slog.NewJSONHandler(&logs, nil)), false)
			err := svc.Delete(context.Background(), agentID)
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				assert.Contains(t, repo.agents, agentID)
			} else {
				require.NoError(t, err)
				require.NoError(t, svc.Delete(context.Background(), agentID))
				assert.NotContains(t, repo.agents, agentID)
			}
			wantRecords := 0
			if tc.present && tc.err == nil {
				wantRecords = 1
			}
			assert.Equal(t, wantRecords, bytes.Count(logs.Bytes(), []byte(`"msg":"agent deleted"`)))
		})
	}
}

type agentAuditContextKey struct{}

type agentAuditCapture struct {
	records  []slog.Record
	contexts []context.Context
}

func (h *agentAuditCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h *agentAuditCapture) Handle(ctx context.Context, record slog.Record) error {
	h.records = append(h.records, record.Clone())
	h.contexts = append(h.contexts, ctx)
	return nil
}
func (h *agentAuditCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *agentAuditCapture) WithGroup(string) slog.Handler      { return h }

func agentAuditFields(t *testing.T, record slog.Record) map[string]any {
	t.Helper()
	fields := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})
	assert.NotContains(t, fields, "actor", "direct domain calls must not invent an actor")
	assert.NotContains(t, fields, "trace_id", "direct domain calls must not invent a trace")
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "PRIVATE_AGENT_METADATA")
	return fields
}

func TestCreateAuditContainsOnlyDefensiveTypedDefinitions(t *testing.T) {
	repo := newMockAgentRepo()
	capture := &agentAuditCapture{}
	svc := NewService(repo, &mockServiceReqValidator{}, slog.New(capture), true)
	ctx := context.WithValue(context.Background(), agentAuditContextKey{}, "operation-context")
	agent := &storage.Agent{
		ID: id.NewAgentID(), ClientID: ptr.To(id.ClientID("PRIVATE_AGENT_METADATA")),
		DisplayName: "PRIVATE_AGENT_METADATA", Description: "PRIVATE_AGENT_METADATA",
		GovernanceURL: ptr.To("https://PRIVATE_AGENT_METADATA.invalid/governance"),
		PermissionSets: []storage.AgentPermissionSetEntry{
			{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional},
			{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeMandatory},
		},
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"write", "read"}},
			{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory, RequireAllScopes: true},
		},
	}
	want := agent.Copy()
	require.NoError(t, svc.Create(ctx, agent))
	require.Len(t, capture.records, 1)
	assert.Equal(t, "agent created", capture.records[0].Message)
	assert.Equal(t, "operation-context", capture.contexts[0].Value(agentAuditContextKey{}))
	// A handler may retain the record after the caller mutates its input.
	agent.PermissionSets[0].PermissionSetID = id.NewPermissionSetID()
	agent.ServiceRequirements[0].RequiredScopes[0] = "mutated"
	agent.ServiceRequirements[1].RequireAllScopes = false
	fields := agentAuditFields(t, capture.records[0])
	require.Len(t, fields, 4)
	assert.Equal(t, "agent_created", fields["action"])
	assert.Equal(t, want.ID, fields["agent_id"])
	assert.Equal(t, want.PermissionSets, fields["permission_sets"])
	assert.Equal(t, want.ServiceRequirements, fields["service_requirements"])
}

func TestUpdateAuditPreservesObservedBeforeAndNewDefinitions(t *testing.T) {
	previous := &storage.Agent{
		ID: id.NewAgentID(), DisplayName: "PRIVATE_AGENT_METADATA", Description: "PRIVATE_AGENT_METADATA",
		PermissionSets: testPermissionSets(),
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory, RequiredScopes: []string{"read", "write"}},
		},
		UpdatedAt: time.Date(2026, 10, 8, 12, 3, 4, 123456789, time.FixedZone("offset", 2*60*60)),
	}
	wantPrevious := previous.Copy()
	agent := &storage.Agent{
		DisplayName: "PRIVATE_AGENT_METADATA", Description: "PRIVATE_AGENT_METADATA",
		PermissionSets: testPermissionSets(),
		ServiceRequirements: []storage.ServiceRequirement{
			{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"write"}},
			{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory, RequireAllScopes: true},
		},
	}
	wantNew := agent.Copy()
	writes := 0
	repo := newMockAgentRepo()
	repo.getFn = func(context.Context, id.AgentID) (*storage.Agent, error) { return previous, nil }
	repo.updateFn = func(context.Context, *storage.Agent) error {
		writes++
		// A shared repository object can change during persistence.
		previous.PermissionSets[0].RequirementType = storage.RequirementTypeMandatory
		previous.ServiceRequirements[0].RequiredScopes[0] = "overwritten"
		previous.UpdatedAt = time.Now()
		return nil
	}
	capture := &agentAuditCapture{}
	svc := NewService(repo, &mockServiceReqValidator{}, slog.New(capture), true)
	ctx := context.WithValue(context.Background(), agentAuditContextKey{}, "operation-context")
	require.NoError(t, svc.Update(ctx, previous.ID, agent, false))
	assert.Equal(t, 1, writes)
	require.Len(t, capture.records, 1)
	assert.Equal(t, "agent updated", capture.records[0].Message)
	assert.Equal(t, "operation-context", capture.contexts[0].Value(agentAuditContextKey{}))
	agent.PermissionSets[0].PermissionSetID = id.NewPermissionSetID()
	agent.ServiceRequirements[0].RequiredScopes[0] = "mutated"
	fields := agentAuditFields(t, capture.records[0])
	require.Len(t, fields, 7)
	assert.Equal(t, "agent_updated", fields["action"])
	assert.Equal(t, previous.ID, fields["agent_id"])
	assert.Equal(t, wantNew.PermissionSets, fields["permission_sets"])
	assert.Equal(t, wantNew.ServiceRequirements, fields["service_requirements"])
	assert.Equal(t, wantPrevious.PermissionSets, fields["previous_observed_permission_sets"])
	assert.Equal(t, wantPrevious.ServiceRequirements, fields["previous_observed_service_requirements"])
	assert.Equal(t, wantPrevious.UpdatedAt.UTC().Format(time.RFC3339Nano), fields["previous_observed_updated_at"])
}

func TestUpdateAuditsSuccessfulUnchangedDefinition(t *testing.T) {
	repo := newMockAgentRepo()
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent", Description: "Definition", PermissionSets: testPermissionSets()}
	repo.agents[agent.ID] = agent.Copy()
	writes := 0
	repo.updateFn = func(context.Context, *storage.Agent) error { writes++; return nil }
	capture := &agentAuditCapture{}
	svc := NewService(repo, &mockServiceReqValidator{}, slog.New(capture), true)
	require.NoError(t, svc.Update(context.Background(), agent.ID, agent, false))
	assert.Equal(t, 1, writes)
	require.Len(t, capture.records, 1)
	assert.Equal(t, "agent_updated", agentAuditFields(t, capture.records[0])["action"])
}

func TestFailedAgentMutationsEmitNoSuccessfulAudit(t *testing.T) {
	cause := errors.New("PRIVATE_AGENT_METADATA")
	for _, tc := range []struct {
		name, operation                          string
		lookupErr, persistenceErr, validationErr error
		invalid                                  bool
	}{
		{name: "create validation", operation: "create", invalid: true},
		{name: "create requirements", operation: "create", validationErr: cause},
		{name: "create persistence", operation: "create", persistenceErr: cause},
		{name: "update lookup", operation: "update", lookupErr: cause},
		{name: "update validation", operation: "update", invalid: true},
		{name: "update requirements", operation: "update", validationErr: cause},
		{name: "update persistence", operation: "update", persistenceErr: cause},
		{name: "delete persistence", operation: "delete", persistenceErr: cause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent", Description: "Definition", PermissionSets: testPermissionSets()}
			repo := newMockAgentRepo()
			repo.agents[agent.ID] = agent.Copy()
			repo.getFn = func(context.Context, id.AgentID) (*storage.Agent, error) {
				return repo.agents[agent.ID], tc.lookupErr
			}
			writes := 0
			repo.createFn = func(context.Context, *storage.Agent) error { writes++; return tc.persistenceErr }
			repo.updateFn = func(context.Context, *storage.Agent) error { writes++; return tc.persistenceErr }
			repo.deleteFn = func(context.Context, id.AgentID) (bool, error) { writes++; return false, tc.persistenceErr }
			if tc.invalid {
				agent.DisplayName = ""
			}
			capture := &agentAuditCapture{}
			svc := NewService(repo, &mockServiceReqValidator{err: tc.validationErr}, slog.New(capture), true)
			var err error
			switch tc.operation {
			case "create":
				err = svc.Create(context.Background(), agent)
			case "update":
				err = svc.Update(context.Background(), agent.ID, agent, false)
			case "delete":
				err = svc.Delete(context.Background(), agent.ID)
			}
			require.Error(t, err)
			if tc.persistenceErr != nil {
				assert.Equal(t, 1, writes)
			} else {
				assert.Zero(t, writes)
			}
			assert.Empty(t, capture.records)
		})
	}
}

func TestDeleteAuditCarriesContextAndNoInventedIdentity(t *testing.T) {
	repo := newMockAgentRepo()
	agentID := id.NewAgentID()
	repo.agents[agentID] = &storage.Agent{ID: agentID}
	capture := &agentAuditCapture{}
	svc := NewService(repo, nil, slog.New(capture), true)
	ctx := context.WithValue(context.Background(), agentAuditContextKey{}, "operation-context")
	require.NoError(t, svc.Delete(ctx, agentID))
	require.NoError(t, svc.Delete(ctx, agentID))
	require.Len(t, capture.records, 1)
	assert.Equal(t, "operation-context", capture.contexts[0].Value(agentAuditContextKey{}))
	fields := agentAuditFields(t, capture.records[0])
	require.Len(t, fields, 2)
	assert.Equal(t, "agent_deleted", fields["action"])
	assert.Equal(t, agentID, fields["agent_id"])
}
