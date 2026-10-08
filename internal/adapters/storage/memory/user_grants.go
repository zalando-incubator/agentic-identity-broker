package memory

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// UserGrantRepository provides in-memory storage for UserGrant entities.
// Thread-safe implementation using sync.RWMutex.
// Implements upsert semantics: one grant per (principal, agent_id) pair.
type UserGrantRepository struct {
	mu                    sync.RWMutex
	transactions          *TransactionManager
	grants                map[id.GrantID]*storage.UserGrant // ID -> Grant
	byPrincipalAndAgent   map[string]id.GrantID             // "principal:agent_id" -> ID
	grantIDsByAgent       map[id.AgentID][]id.GrantID       // agent_id -> []grant_id (for cascade delete)
	psRepo                ports.PermissionSetRepository     // optional: for service-based lookups
	expirationRecordedFor map[id.GrantID]time.Time
}

// NewUserGrantRepository creates a new in-memory user grant repository.
func NewUserGrantRepository(transactions *TransactionManager) *UserGrantRepository {
	return &UserGrantRepository{
		transactions:          transactions,
		grants:                make(map[id.GrantID]*storage.UserGrant),
		byPrincipalAndAgent:   make(map[string]id.GrantID),
		grantIDsByAgent:       make(map[id.AgentID][]id.GrantID),
		expirationRecordedFor: make(map[id.GrantID]time.Time),
	}
}

func (r *UserGrantRepository) ListUnrecordedExpiredForPrincipal(ctx context.Context, principal id.Principal, at time.Time, limit int) ([]*storage.UserGrant, error) {
	if principal.IsZero() || at.IsZero() || limit <= 0 || limit > 1000 {
		return nil, storage.NewStorageError("ListExpiredUserGrants", storage.ErrorKindValidation, nil, "invalid expiration query")
	}
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*storage.UserGrant
	for _, grant := range r.grants {
		if grant.Principal != principal {
			continue
		}
		if grant.ValidUntil != nil && !grant.ValidUntil.After(at) && !r.expirationRecordedFor[grant.ID].Equal(*grant.ValidUntil) {
			result = append(result, grant.Copy())
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (r *UserGrantRepository) RecordExpiration(ctx context.Context, grantID id.GrantID, effectiveExpiry time.Time) (bool, error) {
	if grantID.IsZero() || effectiveExpiry.IsZero() {
		return false, storage.NewStorageError("RecordUserGrantExpiration", storage.ErrorKindValidation, nil, "invalid expiration recognition")
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return false, err
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()
	grant := r.grants[grantID]
	if grant == nil || grant.ValidUntil == nil || grant.ValidUntil.After(time.Now()) || !grant.ValidUntil.Equal(effectiveExpiry) || r.expirationRecordedFor[grantID].Equal(effectiveExpiry) {
		return false, nil
	}
	journalEntry(ctx, r.expirationRecordedFor, grantID)
	r.expirationRecordedFor[grantID] = effectiveExpiry
	return true, nil
}

// WithPermissionSetRepository sets the permission set repository for service-based lookups.
func (r *UserGrantRepository) WithPermissionSetRepository(psRepo ports.PermissionSetRepository) *UserGrantRepository {
	r.psRepo = psRepo
	return r
}

// Create creates a new user grant or updates existing grant for same principal+agent (upsert semantics).
// Returns deep copy of the created/updated grant.
func (r *UserGrantRepository) Create(ctx context.Context, grant *storage.UserGrant) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	if grant.ID.IsZero() {
		grant.ID = id.NewGrantID()
	}

	// Check if grant already exists for this principal+agent pair (upsert semantics)
	key := principalAgentKey(grant.Principal, grant.AgentID)
	if existingID, exists := r.byPrincipalAndAgent[key]; exists {
		// Update existing grant
		updated := grant.Copy()
		updated.ID = existingID
		updated.CreatedAt = r.grants[existingID].CreatedAt
		journalEntry(ctx, r.grants, existingID)
		r.grants[existingID] = updated

		// Copy back the existing ID to the provided grant
		grant.ID = existingID
	} else {
		// Store new grant
		journalEntry(ctx, r.grants, grant.ID)
		r.grants[grant.ID] = grant.Copy()
		journalEntry(ctx, r.byPrincipalAndAgent, key)
		r.byPrincipalAndAgent[key] = grant.ID

		// Update agent index for cascade delete
		journalEntry(ctx, r.grantIDsByAgent, grant.AgentID)
		r.grantIDsByAgent[grant.AgentID] = append(r.grantIDsByAgent[grant.AgentID], grant.ID)
	}

	return nil
}

// Get retrieves a user grant by ID.
// Returns StorageError with Kind=NotFound if grant not found.
// Returns deep copy to prevent external mutation.
func (r *UserGrantRepository) Get(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	grant, exists := r.grants[grantID]
	if !exists {
		return nil, storage.NewStorageError(
			"GetUserGrant",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"user grant not found",
		)
	}

	return grant.Copy(), nil
}

// Update updates an existing user grant.
// Returns StorageError with Kind=NotFound if grant not found.
func (r *UserGrantRepository) Update(ctx context.Context, grant *storage.UserGrant) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if grant exists
	_, exists := r.grants[grant.ID]
	if !exists {
		return storage.NewStorageError(
			"UpdateUserGrant",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"user grant not found",
		)
	}

	// Store deep copy
	journalEntry(ctx, r.grants, grant.ID)
	r.grants[grant.ID] = grant.Copy()

	return nil
}

// Delete deletes a user grant by ID.
// Idempotent: returns nil if grant doesn't exist.
func (r *UserGrantRepository) Delete(ctx context.Context, grantID id.GrantID) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get grant to clean up indexes
	if grant, exists := r.grants[grantID]; exists {
		// Remove from principal+agent index
		key := principalAgentKey(grant.Principal, grant.AgentID)
		journalEntry(ctx, r.byPrincipalAndAgent, key)
		delete(r.byPrincipalAndAgent, key)

		// Remove from agent index
		r.removeGrantFromAgentIndex(ctx, grant.AgentID, grantID)

		// Remove grant
		journalEntry(ctx, r.grants, grantID)
		delete(r.grants, grantID)
		journalEntry(ctx, r.expirationRecordedFor, grantID)
		delete(r.expirationRecordedFor, grantID)
	}

	return nil
}

// ListByPrincipalAndAgent retrieves all grants for a principal and specific agent.
// Includes expired grants (filtering happens in ConsentService).
// Returns empty slice if no grants exist (not an error).
// Returns deep copies to prevent external mutation.
func (r *UserGrantRepository) ListByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := principalAgentKey(principal, agentID)
	grantID, exists := r.byPrincipalAndAgent[key]
	if !exists {
		return []*storage.UserGrant{}, nil
	}

	grant, exists := r.grants[grantID]
	if !exists {
		return []*storage.UserGrant{}, nil
	}

	return []*storage.UserGrant{grant.Copy()}, nil
}

// FindByPrincipalAndAgent retrieves the grant for a principal and agent pair.
// Returns nil if no grant exists (checked by caller to distinguish from error).
// Returns StorageError with Kind=NotFound if grant not found.
// Returns deep copy to prevent external mutation.
func (r *UserGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := principalAgentKey(principal, agentID)
	grantID, exists := r.byPrincipalAndAgent[key]
	if !exists {
		return nil, storage.NewStorageError(
			"FindUserGrant",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"user grant not found",
		)
	}

	grant, exists := r.grants[grantID]
	if !exists {
		return nil, storage.NewStorageError(
			"FindUserGrant",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"user grant not found",
		)
	}

	return grant.Copy(), nil
}

// DeleteByAgent deletes all grants associated with an agent.
// Used during cascade deletion when agent is deleted (FR-021).
// Idempotent: returns nil if agent has no grants.
func (r *UserGrantRepository) DeleteByAgent(ctx context.Context, agentID id.AgentID) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get all grant IDs for this agent
	grantIDs, exists := r.grantIDsByAgent[agentID]
	if !exists || len(grantIDs) == 0 {
		return nil
	}

	// Delete all grants for this agent
	for _, grantID := range grantIDs {
		if grant, exists := r.grants[grantID]; exists {
			// Remove from principal+agent index
			key := principalAgentKey(grant.Principal, grant.AgentID)
			journalEntry(ctx, r.byPrincipalAndAgent, key)
			delete(r.byPrincipalAndAgent, key)

			// Remove grant
			journalEntry(ctx, r.grants, grantID)
			delete(r.grants, grantID)
			journalEntry(ctx, r.expirationRecordedFor, grantID)
			delete(r.expirationRecordedFor, grantID)
		}
	}

	// Remove agent index
	journalEntry(ctx, r.grantIDsByAgent, agentID)
	delete(r.grantIDsByAgent, agentID)

	return nil
}

// DeleteByPrincipalAndAgentID deletes the grant owned by principal for the given agent.
// Returns StorageError wrapping ports.ErrNotFound when no grant exists for the pair.
// This is NOT idempotent: absence of a grant is an error (revocation semantics FR-014).
func (r *UserGrantRepository) DeleteByPrincipalAndAgentID(ctx context.Context, principal id.Principal, agentID id.AgentID) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalAgentKey(principal, agentID)
	grantID, exists := r.byPrincipalAndAgent[key]
	if !exists {
		return storage.NewStorageError(
			"DeleteByPrincipalAndAgentID",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"no active grant exists for this principal and agent",
		)
	}

	// Remove from principal+agent index
	journalEntry(ctx, r.byPrincipalAndAgent, key)
	delete(r.byPrincipalAndAgent, key)

	// Remove from agent index
	r.removeGrantFromAgentIndex(ctx, agentID, grantID)

	// Remove grant
	journalEntry(ctx, r.grants, grantID)
	delete(r.grants, grantID)
	journalEntry(ctx, r.expirationRecordedFor, grantID)
	delete(r.expirationRecordedFor, grantID)

	return nil
}

// principalAgentKey creates a composite key for indexing by principal and agent.
func principalAgentKey(principal id.Principal, agentID id.AgentID) string {
	return principal.String() + ":" + agentID.String()
}

// ListByPrincipal retrieves all active grants for a principal across all agents.
// Filters expired grants (valid_until < NOW()).
// Returns empty slice if no active grants exist (not an error).
// Returns deep copies to prevent external mutation.
func (r *UserGrantRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]storage.UserGrant, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var activeGrants []storage.UserGrant

	// Iterate through all grants and filter by principal
	for _, grant := range r.grants {
		if grant.Principal == principal && grant.IsActive() {
			activeGrants = append(activeGrants, *grant.Copy())
		}
	}

	return activeGrants, nil
}

// CountAgentsByPrincipalAndServiceID counts distinct agents for the exact principal
// whose GrantedPermissionSets include the given service. Expired grants are included for session dependency warnings.
func (r *UserGrantRepository) CountAgentsByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (int, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	agentSet := make(map[id.AgentID]bool)
	for _, grant := range r.grants {
		if grant.Principal == principal && r.grantReferencesService(ctx, grant, serviceID) {
			agentSet[grant.AgentID] = true
		}
	}

	return len(agentSet), nil
}

// ListByPrincipalAndServiceID returns distinct agent IDs for the exact principal
// whose GrantedPermissionSets include the given service. Expired grants are included for session dependency warnings.
func (r *UserGrantRepository) ListByPrincipalAndServiceID(ctx context.Context, principal id.Principal, serviceID id.ServiceID) ([]id.AgentID, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	agentSet := make(map[id.AgentID]bool)
	for _, grant := range r.grants {
		if grant.Principal == principal && r.grantReferencesService(ctx, grant, serviceID) {
			agentSet[grant.AgentID] = true
		}
	}

	result := make([]id.AgentID, 0, len(agentSet))
	for agentID := range agentSet {
		result = append(result, agentID)
	}

	return result, nil
}

// CountGrantsReferencingPermissionSet counts active user grants that contain the given permission set ID.
func (r *UserGrantRepository) CountGrantsReferencingPermissionSet(ctx context.Context, psID id.PermissionSetID) (int, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, grant := range r.grants {
		if !grant.IsActive() {
			continue
		}
		for _, entry := range grant.GrantedPermissionSets {
			if entry.PermissionSetID == psID {
				count++
				break
			}
		}
	}
	return count, nil
}

// grantReferencesService checks if any of the grant's permission set entries
// include the given service ID. Unlike CountGrantsReferencingPermissionSet,
// this helper does NOT filter by expiry — it matches all grants regardless of
// valid_until. Callers use it for principal-scoped agent-association lookups,
// not deletion-protection, so expired grants are intentionally included.
func (r *UserGrantRepository) grantReferencesService(ctx context.Context, grant *storage.UserGrant, serviceID id.ServiceID) bool {
	for _, entry := range grant.GrantedPermissionSets {
		for _, svcID := range entry.IncludedServiceIDs {
			if svcID == serviceID {
				return true
			}
		}
	}

	return false
}

// removeGrantFromAgentIndex removes a grant ID from the agent's grant list.
func (r *UserGrantRepository) removeGrantFromAgentIndex(ctx context.Context, agentID id.AgentID, grantID id.GrantID) {
	grantIDs, exists := r.grantIDsByAgent[agentID]
	if !exists {
		return
	}
	grantIDs = slices.Clone(grantIDs)

	// Find and remove grant ID
	for i, id := range grantIDs {
		if id == grantID {
			// Remove by swapping with last element and truncating
			grantIDs[i] = grantIDs[len(grantIDs)-1]
			journalEntry(ctx, r.grantIDsByAgent, agentID)
			r.grantIDsByAgent[agentID] = grantIDs[:len(grantIDs)-1]
			break
		}
	}

	// Clean up empty agent index
	if len(r.grantIDsByAgent[agentID]) == 0 {
		journalEntry(ctx, r.grantIDsByAgent, agentID)
		delete(r.grantIDsByAgent, agentID)
	}
}

// CreateTestGrant creates a user grant WITHOUT validation - for testing expired/invalid grants.
// This method MUST NOT be used in production - it bypasses all validation.
// Only available on memory storage for testing purposes.
func (r *UserGrantRepository) CreateTestGrant(ctx context.Context, grant *storage.UserGrant) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	if grant.ID.IsZero() {
		grant.ID = id.NewGrantID()
	}

	// Check if grant already exists for this principal+agent pair (upsert semantics)
	key := principalAgentKey(grant.Principal, grant.AgentID)
	if existingID, exists := r.byPrincipalAndAgent[key]; exists {
		// Update existing grant
		updated := grant.Copy()
		updated.ID = existingID
		updated.CreatedAt = r.grants[existingID].CreatedAt
		journalEntry(ctx, r.grants, existingID)
		r.grants[existingID] = updated

		// Copy back the existing ID to the provided grant
		grant.ID = existingID
	} else {
		// Store new grant
		journalEntry(ctx, r.grants, grant.ID)
		r.grants[grant.ID] = grant.Copy()
		journalEntry(ctx, r.byPrincipalAndAgent, key)
		r.byPrincipalAndAgent[key] = grant.ID

		// Update agent index for cascade delete
		journalEntry(ctx, r.grantIDsByAgent, grant.AgentID)
		r.grantIDsByAgent[grant.AgentID] = append(r.grantIDsByAgent[grant.AgentID], grant.ID)
	}

	return nil
}
