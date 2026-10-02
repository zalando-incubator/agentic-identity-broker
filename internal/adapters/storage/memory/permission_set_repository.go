package memory

import (
	"context"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// PermissionSetRepository provides in-memory storage for PermissionSet entities.
// Thread-safe implementation using sync.RWMutex.
type PermissionSetRepository struct {
	mu             sync.RWMutex
	transactions   *TransactionManager
	permissionSets map[id.PermissionSetID]*storage.PermissionSet
	canonicalIndex map[string]id.PermissionSetID
	nameIndex      map[string]id.PermissionSetID // Name -> ID for uniqueness check
	agentRepo      ports.AgentRepository         // used for CountAgentsReferencingPermissionSet
}

// NewPermissionSetRepository creates a new in-memory permission set repository.
func NewPermissionSetRepository(transactions *TransactionManager) *PermissionSetRepository {
	return &PermissionSetRepository{
		transactions:   transactions,
		permissionSets: make(map[id.PermissionSetID]*storage.PermissionSet),
		canonicalIndex: make(map[string]id.PermissionSetID),
		nameIndex:      make(map[string]id.PermissionSetID),
	}
}

// WithAgentRepository sets the agent repository for deletion protection checks.
func (r *PermissionSetRepository) WithAgentRepository(agentRepo ports.AgentRepository) *PermissionSetRepository {
	r.mu.Lock()
	r.agentRepo = agentRepo
	r.mu.Unlock()
	return r
}

// Create stores a new permission set.
func (r *PermissionSetRepository) Create(ctx context.Context, ps *storage.PermissionSet) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	if ps.ID.IsZero() {
		ps.ID = id.NewPermissionSetID()
	}

	// Check for duplicate ID
	if _, exists := r.permissionSets[ps.ID]; exists {
		return storage.NewStorageError(
			"CreatePermissionSet",
			storage.ErrorKindConflict,
			nil,
			"permission set with this ID already exists",
		)
	}

	// Check for duplicate name
	if _, exists := r.nameIndex[ps.Name]; exists {
		return storage.NewStorageError(
			"CreatePermissionSet",
			storage.ErrorKindConflict,
			nil,
			"permission set with this name already exists",
		)
	}

	if ps.CanonicalID != nil {
		if _, exists := r.canonicalIndex[*ps.CanonicalID]; exists {
			return storage.NewStorageError("CreatePermissionSet", storage.ErrorKindConflict, nil, "permission set canonical_id already exists")
		}
	}

	// Store the permission set
	journalEntry(ctx, r.permissionSets, ps.ID)
	r.permissionSets[ps.ID] = ps.Copy()
	journalEntry(ctx, r.nameIndex, ps.Name)
	r.nameIndex[ps.Name] = ps.ID
	if ps.CanonicalID != nil {
		journalEntry(ctx, r.canonicalIndex, *ps.CanonicalID)
		r.canonicalIndex[*ps.CanonicalID] = ps.ID
	}

	return nil
}

// Get retrieves a permission set by ID.
func (r *PermissionSetRepository) Get(ctx context.Context, id id.PermissionSetID) (*storage.PermissionSet, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	ps, exists := r.permissionSets[id]
	if !exists {
		return nil, storage.NewStorageError(
			"GetPermissionSet",
			storage.ErrorKindNotFound,
			nil,
			"permission set not found",
		)
	}

	return ps.Copy(), nil
}

func (r *PermissionSetRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.PermissionSet, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	psID, exists := r.canonicalIndex[canonicalID]
	if !exists {
		return nil, storage.NewStorageError("GetPermissionSetByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "permission set not found")
	}
	return r.permissionSets[psID].Copy(), nil
}

func (r *PermissionSetRepository) GetCanonicalIDs(ctx context.Context, ids []id.PermissionSetID) (map[id.PermissionSetID]string, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	canonicalIDs := make(map[id.PermissionSetID]string, len(ids))
	for _, permissionSetID := range ids {
		if permissionSet, exists := r.permissionSets[permissionSetID]; exists && permissionSet.CanonicalID != nil {
			canonicalIDs[permissionSetID] = *permissionSet.CanonicalID
		}
	}
	return canonicalIDs, nil
}

// GetByIDs retrieves multiple permission sets by IDs.
func (r *PermissionSetRepository) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.PermissionSet, 0, len(ids))
	for _, id := range ids {
		if ps, exists := r.permissionSets[id]; exists {
			result = append(result, ps.Copy())
		}
	}

	return result, nil
}

// Update replaces a permission set's fields.
func (r *PermissionSetRepository) Update(ctx context.Context, ps *storage.PermissionSet) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.permissionSets[ps.ID]
	if !exists {
		return storage.NewStorageError(
			"UpdatePermissionSet",
			storage.ErrorKindNotFound,
			nil,
			"permission set not found",
		)
	}

	if ps.CanonicalID != nil {
		if existingID, exists := r.canonicalIndex[*ps.CanonicalID]; exists && existingID != ps.ID {
			return storage.NewStorageError("UpdatePermissionSet", storage.ErrorKindConflict, nil, "permission set canonical_id already exists")
		}
	}

	// Check for duplicate name (if name changed)
	if ps.Name != existing.Name {
		if _, exists := r.nameIndex[ps.Name]; exists {
			return storage.NewStorageError(
				"UpdatePermissionSet",
				storage.ErrorKindConflict,
				nil,
				"permission set with this name already exists",
			)
		}
		// Update name index
		journalEntry(ctx, r.nameIndex, existing.Name)
		delete(r.nameIndex, existing.Name)
		journalEntry(ctx, r.nameIndex, ps.Name)
		r.nameIndex[ps.Name] = ps.ID
	}
	if existing.CanonicalID != nil {
		journalEntry(ctx, r.canonicalIndex, *existing.CanonicalID)
		delete(r.canonicalIndex, *existing.CanonicalID)
	}
	if ps.CanonicalID != nil {
		journalEntry(ctx, r.canonicalIndex, *ps.CanonicalID)
		r.canonicalIndex[*ps.CanonicalID] = ps.ID
	}

	// Store the updated permission set
	journalEntry(ctx, r.permissionSets, ps.ID)
	r.permissionSets[ps.ID] = ps.Copy()

	return nil
}

// Delete removes a permission set by ID.
func (r *PermissionSetRepository) Delete(ctx context.Context, id id.PermissionSetID) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	ps, exists := r.permissionSets[id]
	if !exists {
		// Idempotent: safe to delete non-existent
		return nil
	}

	journalEntry(ctx, r.permissionSets, id)
	delete(r.permissionSets, id)
	journalEntry(ctx, r.nameIndex, ps.Name)
	delete(r.nameIndex, ps.Name)
	if ps.CanonicalID != nil {
		journalEntry(ctx, r.canonicalIndex, *ps.CanonicalID)
		delete(r.canonicalIndex, *ps.CanonicalID)
	}

	return nil
}

// List returns all permission sets, optionally filtered by service ID.
func (r *PermissionSetRepository) List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.PermissionSet, 0)

	for _, ps := range r.permissionSets {
		// Filter by service if provided
		if !serviceID.IsZero() {
			found := false
			for _, ss := range ps.ServiceScopes {
				if ss.ServiceID == serviceID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, ps.Copy())
	}

	return result, nil
}

// CountAgentsReferencingPermissionSet counts agents whose permission_sets list contains the given ID.
func (r *PermissionSetRepository) CountAgentsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error) {
	ctx, guard, gateErr := r.transactions.enter(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	agentRepo := r.agentRepo
	r.mu.RUnlock()

	if agentRepo == nil {
		return 0, nil
	}
	agents, err := agentRepo.List(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, agent := range agents {
		for _, entry := range agent.PermissionSets {
			if entry.PermissionSetID == psID {
				count++
				break
			}
		}
	}
	return count, nil
}

// CountPermissionSetsForService counts permission sets containing a scope for the given service.
func (r *PermissionSetRepository) CountPermissionSetsForService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, ps := range r.permissionSets {
		for _, ss := range ps.ServiceScopes {
			if ss.ServiceID == serviceID {
				count++
				break
			}
		}
	}

	return count, nil
}
