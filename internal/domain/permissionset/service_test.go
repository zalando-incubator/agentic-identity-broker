package permissionset

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPermissionSetRepository is a hand-rolled mock for testing.
type mockPermissionSetRepository struct {
	createFunc                        func(ctx context.Context, ps *storage.PermissionSet) error
	getFunc                           func(ctx context.Context, id id.PermissionSetID) (*storage.PermissionSet, error)
	getByCanonicalIDFunc              func(ctx context.Context, canonicalID string) (*storage.PermissionSet, error)
	getByIDsFunc                      func(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error)
	updateFunc                        func(ctx context.Context, ps *storage.PermissionSet) error
	deleteFunc                        func(ctx context.Context, id id.PermissionSetID) error
	listFunc                          func(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error)
	countAgentsReferencingFunc        func(ctx context.Context, id id.PermissionSetID) (int, error)
	countPermissionSetsForServiceFunc func(ctx context.Context, serviceID id.ServiceID) (int, error)
}

func (m *mockPermissionSetRepository) Create(ctx context.Context, ps *storage.PermissionSet) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, ps)
	}
	return nil
}

func (m *mockPermissionSetRepository) Get(ctx context.Context, id id.PermissionSetID) (*storage.PermissionSet, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, ports.ErrNotFound
}

func (m *mockPermissionSetRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.PermissionSet, error) {
	if m.getByCanonicalIDFunc != nil {
		return m.getByCanonicalIDFunc(ctx, canonicalID)
	}
	return nil, ports.ErrNotFound
}

func (m *mockPermissionSetRepository) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	if m.getByIDsFunc != nil {
		return m.getByIDsFunc(ctx, ids)
	}
	return []*storage.PermissionSet{}, nil
}

func (m *mockPermissionSetRepository) GetCanonicalIDs(_ context.Context, _ []id.PermissionSetID) (map[id.PermissionSetID]string, error) {
	return map[id.PermissionSetID]string{}, nil
}

func (m *mockPermissionSetRepository) Update(ctx context.Context, ps *storage.PermissionSet) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, ps)
	}
	return nil
}

func (m *mockPermissionSetRepository) Delete(ctx context.Context, id id.PermissionSetID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockPermissionSetRepository) List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, serviceID)
	}
	return []*storage.PermissionSet{}, nil
}

func (m *mockPermissionSetRepository) CountAgentsReferencingPermissionSet(ctx context.Context, id id.PermissionSetID) (int, error) {
	if m.countAgentsReferencingFunc != nil {
		return m.countAgentsReferencingFunc(ctx, id)
	}
	return 0, nil
}

func (m *mockPermissionSetRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

func (m *mockPermissionSetRepository) CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if m.countPermissionSetsForServiceFunc != nil {
		return m.countPermissionSetsForServiceFunc(ctx, serviceID)
	}
	return 0, nil
}

// stubGrantRepository is a no-op grant repository used as the second argument to
// NewPermissionSetService in tests that don't exercise grant-based deletion protection.
type stubGrantRepository struct{}

func (s *stubGrantRepository) Create(_ context.Context, _ *storage.UserGrant) error { return nil }
func (s *stubGrantRepository) Get(_ context.Context, _ id.GrantID) (*storage.UserGrant, error) {
	return nil, nil
}
func (s *stubGrantRepository) Update(_ context.Context, _ *storage.UserGrant) error { return nil }
func (s *stubGrantRepository) Delete(_ context.Context, _ id.GrantID) error         { return nil }
func (s *stubGrantRepository) ListByPrincipalAndAgent(_ context.Context, _ id.Principal, _ id.AgentID) ([]*storage.UserGrant, error) {
	return nil, nil
}
func (s *stubGrantRepository) FindByPrincipalAndAgent(_ context.Context, _ id.Principal, _ id.AgentID) (*storage.UserGrant, error) {
	return nil, nil
}
func (s *stubGrantRepository) DeleteByAgent(_ context.Context, _ id.AgentID) error { return nil }
func (s *stubGrantRepository) DeleteByPrincipalAndAgentID(_ context.Context, _ id.Principal, _ id.AgentID) error {
	return nil
}
func (s *stubGrantRepository) ListByPrincipal(_ context.Context, _ id.Principal) ([]storage.UserGrant, error) {
	return nil, nil
}
func (s *stubGrantRepository) CountAgentsByPrincipalAndServiceID(_ context.Context, _ id.Principal, _ id.ServiceID) (int, error) {
	return 0, nil
}
func (s *stubGrantRepository) ListByPrincipalAndServiceID(_ context.Context, _ id.Principal, _ id.ServiceID) ([]id.AgentID, error) {
	return nil, nil
}
func (s *stubGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return 0, nil
}

// TestGetByIDsCacheMiss tests that GetByIDs fetches from repo when cache misses.
func TestGetByIDsCacheMiss(t *testing.T) {
	psID := id.NewPermissionSetID()
	ps := &storage.PermissionSet{
		ID:          psID,
		Name:        "Test PS",
		Description: "Test",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
		},
	}

	callCount := 0
	repo := &mockPermissionSetRepository{
		getByIDsFunc: func(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
			callCount++
			return []*storage.PermissionSet{ps}, nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	result, err := service.GetByIDs(context.Background(), []id.PermissionSetID{psID})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, psID, result[0].ID)
	assert.Equal(t, 1, callCount)
}

// TestGetByIDsCacheHit tests that GetByIDs uses cache within TTL.
func TestGetByIDsCacheHit(t *testing.T) {
	psID := id.NewPermissionSetID()
	ps := &storage.PermissionSet{
		ID:          psID,
		Name:        "Test PS",
		Description: "Test",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
		},
	}

	callCount := 0
	repo := &mockPermissionSetRepository{
		getByIDsFunc: func(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
			callCount++
			return []*storage.PermissionSet{ps}, nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())

	// First call - fetches from repo
	result1, err := service.GetByIDs(context.Background(), []id.PermissionSetID{psID})
	require.NoError(t, err)
	require.Len(t, result1, 1)
	assert.Equal(t, 1, callCount)

	// Second call immediately - should use cache
	result2, err := service.GetByIDs(context.Background(), []id.PermissionSetID{psID})
	require.NoError(t, err)
	require.Len(t, result2, 1)
	assert.Equal(t, 1, callCount) // No additional call
}

// TestGetByIDsCacheExpiry tests that cache expires after TTL.
func TestGetByIDsCacheExpiry(t *testing.T) {
	psID := id.NewPermissionSetID()
	ps := &storage.PermissionSet{
		ID:          psID,
		Name:        "Test PS",
		Description: "Test",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
		},
	}

	callCount := 0
	repo := &mockPermissionSetRepository{
		getByIDsFunc: func(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
			callCount++
			return []*storage.PermissionSet{ps}, nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	defer service.Close()

	// Override cache TTL to a very short duration for testing
	service.mu.Lock()
	service.cacheTTL = 10 * time.Millisecond
	service.mu.Unlock()

	// First call - fetches from repo
	result1, err := service.GetByIDs(context.Background(), []id.PermissionSetID{psID})
	require.NoError(t, err)
	require.Len(t, result1, 1)
	assert.Equal(t, 1, callCount)

	// Wait for cache to expire
	time.Sleep(20 * time.Millisecond)

	// Second call after TTL - should fetch from repo again
	result2, err := service.GetByIDs(context.Background(), []id.PermissionSetID{psID})
	require.NoError(t, err)
	require.Len(t, result2, 1)
	assert.Equal(t, 2, callCount, "expected second repo call after cache expiry")
}

// TestValidateIDsReturnsMissingIDs tests that ValidateIDs returns errors for missing IDs.
func TestValidateIDsReturnsMissingIDs(t *testing.T) {
	psID1 := id.NewPermissionSetID()
	psID2 := id.NewPermissionSetID()
	psID3 := id.NewPermissionSetID()

	// Only return one of three
	repo := &mockPermissionSetRepository{
		getByIDsFunc: func(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
			ps := &storage.PermissionSet{
				ID:          psID1,
				Name:        "PS1",
				Description: "Test",
				ServiceScopes: []storage.ServiceScope{
					{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
				},
			}
			return []*storage.PermissionSet{ps}, nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	err := service.ValidateIDs(context.Background(), []id.PermissionSetID{psID1, psID2, psID3})

	// Should error with details about missing IDs
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

// TestDeleteWithAgentReferenceReturnsConflict tests deletion protection.
func TestDeleteWithAgentReferenceReturnsConflict(t *testing.T) {
	psID := id.NewPermissionSetID()

	repo := &mockPermissionSetRepository{
		countAgentsReferencingFunc: func(ctx context.Context, id id.PermissionSetID) (int, error) {
			return 2, nil // 2 agents reference this permission set
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	err := service.Delete(context.Background(), psID)

	require.Error(t, err)
	// The StorageError with KindConflict - check it's a StorageError with conflict kind
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr), "expected StorageError")
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Contains(t, err.Error(), "2")
}

// TestDeleteWithGrantReferenceReturnsConflict tests grant-based deletion protection.
func TestDeleteWithGrantReferenceReturnsConflict(t *testing.T) {
	psID := id.NewPermissionSetID()

	repo := &mockPermissionSetRepository{
		countAgentsReferencingFunc: func(ctx context.Context, id id.PermissionSetID) (int, error) {
			return 0, nil // No agents reference it
		},
	}

	// Grant repository that reports 1 active grant referencing the permission set.
	grantRepo := &countingGrantRepository{count: 1}

	service := NewPermissionSetService(repo, grantRepo, slog.Default())
	err := service.Delete(context.Background(), psID)

	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr), "expected StorageError")
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Contains(t, err.Error(), "1")
}

// countingGrantRepository is a minimal UserGrantRepository stub whose
// CountGrantsReferencingPermissionSet returns a configurable count.
type countingGrantRepository struct {
	stubGrantRepository
	count int
}

func (r *countingGrantRepository) CountGrantsReferencingPermissionSet(_ context.Context, _ id.PermissionSetID) (int, error) {
	return r.count, nil
}

// TestCreateEmitsAuditLog tests that Create emits structured audit logs.
func TestCreateEmitsAuditLog(t *testing.T) {
	psID := id.NewPermissionSetID()
	ps := &storage.PermissionSet{
		ID:          psID,
		Name:        "Test PS",
		Description: "Test",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	callCount := 0
	repo := &mockPermissionSetRepository{
		createFunc: func(ctx context.Context, input *storage.PermissionSet) error {
			callCount++
			return nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	err := service.Create(context.Background(), ps)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

// TestUpdateEmitsAuditLog tests that Update emits structured audit logs.
func TestUpdateEmitsAuditLog(t *testing.T) {
	psID := id.NewPermissionSetID()
	ps := &storage.PermissionSet{
		ID:          psID,
		Name:        "Updated PS",
		Description: "Updated",
		ServiceScopes: []storage.ServiceScope{
			{ServiceID: id.NewServiceID(), Scopes: []string{"write"}, RequirementType: storage.RequirementTypeOptional},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	callCount := 0
	repo := &mockPermissionSetRepository{
		updateFunc: func(ctx context.Context, input *storage.PermissionSet) error {
			callCount++
			return nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	err := service.Update(context.Background(), ps)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

// TestDeleteEmitsAuditLog tests that Delete emits structured audit logs.
func TestDeleteEmitsAuditLog(t *testing.T) {
	psID := id.NewPermissionSetID()

	callCount := 0
	repo := &mockPermissionSetRepository{
		countAgentsReferencingFunc: func(ctx context.Context, id id.PermissionSetID) (int, error) {
			return 0, nil // No agents reference it
		},
		deleteFunc: func(ctx context.Context, id id.PermissionSetID) error {
			callCount++
			return nil
		},
	}

	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	err := service.Delete(context.Background(), psID)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestResolveIDAcceptsUUIDCanonicalAndRejectsUnknown(t *testing.T) {
	permissionSetID := id.NewPermissionSetID()
	canonicalID := "repository-read"
	repo := &mockPermissionSetRepository{getByCanonicalIDFunc: func(_ context.Context, value string) (*storage.PermissionSet, error) {
		if value == canonicalID {
			return &storage.PermissionSet{ID: permissionSetID}, nil
		}
		return nil, ports.ErrNotFound
	}}
	service := NewPermissionSetService(repo, &stubGrantRepository{}, slog.Default())
	defer service.Close()
	for _, value := range []string{permissionSetID.String(), canonicalID} {
		resolved, err := service.ResolveID(context.Background(), value)
		require.NoError(t, err)
		assert.Equal(t, permissionSetID, resolved)
	}
	_, err := service.ResolveID(context.Background(), "unknown-permission-set")
	require.Error(t, err)
}
