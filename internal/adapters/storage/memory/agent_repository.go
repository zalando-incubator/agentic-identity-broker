package memory

import (
	"context"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// AgentRepository provides in-memory storage for Agent entities.
// Thread-safe implementation using sync.RWMutex.
//
// The byClientID index is a 1:many map to correctly support multi-agent sharing
// (multiple agents sharing the same upstream OAuth2 client_id).
// The byClientURI index enforces global uniqueness of Client ID Metadata Document URLs.
type AgentRepository struct {
	mu            sync.RWMutex
	refresh       *RefreshSessionStore
	agents        map[id.AgentID]*storage.Agent // ID -> Agent
	byCanonicalID map[string]id.AgentID         // canonical ID -> AgentID
	byClientID    map[id.ClientID][]id.AgentID  // ClientID -> []ID (1:many for multi-agent support)
	byClientURI   map[string]id.AgentID         // clientURI -> AgentID (global uniqueness)
}

// NewAgentRepository creates a new in-memory agent repository.
func NewAgentRepository() *AgentRepository {
	return &AgentRepository{
		agents:        make(map[id.AgentID]*storage.Agent),
		byCanonicalID: make(map[string]id.AgentID),
		byClientID:    make(map[id.ClientID][]id.AgentID),
		byClientURI:   make(map[string]id.AgentID),
	}
}

// Create creates a new agent entity in storage.
// Generates a UUID for the agent if ID is empty.
// Returns StorageError with Kind=Conflict if agent ID already exists.
// Multiple agents may share the same client_id (multi-agent mode support).
func (r *AgentRepository) Create(ctx context.Context, agent *storage.Agent) error {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedCreate(ctx, agent)
	}
	unlock := r.refresh.lockWrite()
	defer unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Generate ID if not provided
	if agent.ID.IsZero() {
		agent.ID = id.NewAgentID()
	}

	// Check for duplicate ID
	if _, exists := r.agents[agent.ID]; exists {
		return storage.NewStorageError(
			"CreateAgent",
			storage.ErrorKindConflict,
			nil,
			"agent with this ID already exists",
		)
	}

	// Validate before storing
	if err := agent.ValidateForCreate(); err != nil {
		return storage.NewStorageError(
			"CreateAgent",
			storage.ErrorKindValidation,
			err,
			"agent validation failed",
		)
	}

	if agent.CanonicalID != nil {
		if _, exists := r.byCanonicalID[*agent.CanonicalID]; exists {
			return storage.NewStorageError("CreateAgent", storage.ErrorKindConflict, nil, "agent canonical_id already exists")
		}
	}

	// Enforce global uniqueness of client URIs
	for _, uri := range agent.ClientURIs {
		if existingID, exists := r.byClientURI[uri]; exists && existingID != agent.ID {
			return storage.NewStorageError(
				"CreateAgent",
				storage.ErrorKindConflict,
				nil,
				"client_uri already registered to another agent",
			)
		}
	}

	// Store deep copy to prevent external mutation
	r.agents[agent.ID] = agent.Copy()
	if agent.CanonicalID != nil {
		r.byCanonicalID[*agent.CanonicalID] = agent.ID
	}
	if agent.ClientID != nil {
		r.byClientID[*agent.ClientID] = append(r.byClientID[*agent.ClientID], agent.ID)
	}
	for _, uri := range agent.ClientURIs {
		r.byClientURI[uri] = agent.ID
	}
	if r.refresh != nil {
		r.refresh.versions[agent.ID]++
	}

	return nil
}

// Get retrieves an agent entity by ID.
// Returns StorageError with Kind=NotFound if agent not found.
func (r *AgentRepository) Get(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedGet(ctx, agentID)
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return nil, storage.NewStorageError(
			"GetAgent",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"agent not found",
		)
	}

	// Return deep copy to prevent external mutation
	return agent.Copy(), nil
}

func (r *AgentRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*storage.Agent, error) {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedFind(ctx, func(agent *storage.Agent) bool { return agent.CanonicalID != nil && *agent.CanonicalID == canonicalID })
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	agentID, exists := r.byCanonicalID[canonicalID]
	if !exists {
		return nil, storage.NewStorageError("GetAgentByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	return r.agents[agentID].Copy(), nil
}

// Update updates an existing agent entity.
// Returns StorageError with Kind=NotFound if agent ID not found.
func (r *AgentRepository) Update(ctx context.Context, agent *storage.Agent) error {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedPut(ctx, agent, false)
	}
	unlock := r.refresh.lockWrite()
	defer unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if agent exists
	existing, exists := r.agents[agent.ID]
	if !exists {
		return storage.NewStorageError(
			"UpdateAgent",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"agent not found",
		)
	}

	// Enforce global uniqueness of client URIs before touching any indexes
	for _, uri := range agent.ClientURIs {
		if existingID, exists := r.byClientURI[uri]; exists && existingID != agent.ID {
			return storage.NewStorageError(
				"UpdateAgent",
				storage.ErrorKindConflict,
				nil,
				"client_uri already registered to another agent",
			)
		}
	}

	// Validate before updating
	if err := agent.Validate(); err != nil {
		return storage.NewStorageError(
			"UpdateAgent",
			storage.ErrorKindValidation,
			err,
			"agent validation failed",
		)
	}

	if agent.CanonicalID != nil {
		if existingID, exists := r.byCanonicalID[*agent.CanonicalID]; exists && existingID != agent.ID {
			return storage.NewStorageError("UpdateAgent", storage.ErrorKindConflict, nil, "agent canonical_id already exists")
		}
	}

	// All checks passed — update indexes and stored entity atomically
	oldClientID := existing.ClientID
	newClientID := agent.ClientID
	if !clientIDPtrEqual(oldClientID, newClientID) {
		if oldClientID != nil {
			r.removeFromClientIDIndex(*oldClientID, agent.ID)
		}
		if newClientID != nil {
			r.byClientID[*newClientID] = append(r.byClientID[*newClientID], agent.ID)
		}
	}

	if existing.CanonicalID != nil {
		delete(r.byCanonicalID, *existing.CanonicalID)
	}
	if agent.CanonicalID != nil {
		r.byCanonicalID[*agent.CanonicalID] = agent.ID
	}
	// Rebuild client URI index: remove old URIs, add new ones
	for _, uri := range existing.ClientURIs {
		delete(r.byClientURI, uri)
	}
	for _, uri := range agent.ClientURIs {
		r.byClientURI[uri] = agent.ID
	}

	// Store deep copy
	r.agents[agent.ID] = agent.Copy()
	if r.refresh != nil {
		r.refresh.versions[agent.ID]++
	}

	return nil
}

// Delete deletes an agent entity by ID.
// Idempotent: returns nil if agent doesn't exist.
func (r *AgentRepository) Delete(ctx context.Context, agentID id.AgentID) error {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedDelete(ctx, agentID)
	}
	if r.refresh != nil {
		var entered bool
		err := r.refresh.Run(ctx, agentID, func(scoped context.Context, at time.Time) error {
			entered = true
			if err := r.refresh.RevokeByAgent(scoped, agentID, at, storage.RefreshReasonAgentDeleted); err != nil {
				return err
			}
			return r.scopedDelete(scoped, agentID)
		})
		if !entered && ports.IsNotFoundErr(err) {
			return nil
		}
		return err
	}
	unlock := r.refresh.lockWrite()
	defer unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get agent to clean up indexes
	if agent, exists := r.agents[agentID]; exists {
		if agent.ClientID != nil {
			r.removeFromClientIDIndex(*agent.ClientID, agentID)
		}
		for _, uri := range agent.ClientURIs {
			delete(r.byClientURI, uri)
		}
		delete(r.agents, agentID)
		if agent.CanonicalID != nil {
			delete(r.byCanonicalID, *agent.CanonicalID)
		}
	}
	if r.refresh != nil {
		r.refresh.versions[agentID]++
	}

	return nil
}

// List retrieves all agent entities.
// Returns empty slice if no agents exist (not an error).
func (r *AgentRepository) List(ctx context.Context) ([]*storage.Agent, error) {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedList(ctx)
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*storage.Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		result = append(result, agent.Copy())
	}

	return result, nil
}

// GetByClientID retrieves an agent entity by client_id.
// When multiple agents share the same client_id (multi-agent mode), returns the first registered one.
// Returns StorageError with Kind=NotFound if no agent with that client_id exists.
func (r *AgentRepository) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.Agent, error) {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedFind(ctx, func(agent *storage.Agent) bool { return agent.ClientID != nil && *agent.ClientID == clientID })
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byClientID[clientID]
	if !exists || len(ids) == 0 {
		return nil, storage.NewStorageError(
			"GetAgentByClientID",
			storage.ErrorKindNotFound,
			ports.ErrNotFound,
			"agent not found",
		)
	}

	agent := r.agents[ids[0]]
	// Return deep copy to prevent external mutation
	return agent.Copy(), nil
}

// ExistsOtherWithClientID reports whether any agent other than excludeAgentID shares the given client_id.
// When excludeAgentID is nil, all agents with that client_id are considered (create path).
func (r *AgentRepository) ExistsOtherWithClientID(ctx context.Context, clientID id.ClientID, excludeAgentID *id.AgentID) (bool, error) {
	if scopeFor(ctx, r.refresh) != nil {
		list, err := r.scopedList(ctx)
		if err != nil {
			return false, err
		}
		for _, agent := range list {
			if agent.ClientID != nil && *agent.ClientID == clientID && (excludeAgentID == nil || agent.ID != *excludeAgentID) {
				return true, nil
			}
		}
		return false, nil
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids, exists := r.byClientID[clientID]
	if !exists {
		return false, nil
	}
	for _, agentID := range ids {
		if excludeAgentID == nil || agentID != *excludeAgentID {
			return true, nil
		}
	}
	return false, nil
}

// GetByClientURI retrieves an agent by a pre-registered CIMD URL or URI pattern.
// Exact registrations take precedence. Matching patterns on multiple agents are conflicts.
func (r *AgentRepository) GetByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	if scopeFor(ctx, r.refresh) != nil {
		return r.scopedByClientURI(ctx, uri)
	}
	unlock := r.refresh.lockRead()
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()

	if agentID, exists := r.byClientURI[uri]; exists {
		if agent, ok := r.agents[agentID]; ok {
			return agent.Copy(), nil
		}
	}

	matchedAgentIDs := make(map[id.AgentID]struct{})
	for registeredURI, agentID := range r.byClientURI {
		if _, exists := r.agents[agentID]; exists && urivalidation.MatchesCIMDClientURI(registeredURI, uri) {
			matchedAgentIDs[agentID] = struct{}{}
		}
	}
	if len(matchedAgentIDs) > 1 {
		return nil, storage.NewStorageError(
			"GetAgentByClientURI",
			storage.ErrorKindConflict,
			nil,
			"CIMD client URI matches multiple agents",
		)
	}
	for agentID := range matchedAgentIDs {
		return r.agents[agentID].Copy(), nil
	}

	return nil, storage.NewStorageError(
		"GetAgentByClientURI",
		storage.ErrorKindNotFound,
		ports.ErrNotFound,
		"agent not found",
	)
}

// removeFromClientIDIndex removes a specific agentID from the byClientID slice for clientID.
// Deletes the map entry if the slice becomes empty. Must be called with r.mu held (write lock).
func (r *AgentRepository) removeFromClientIDIndex(clientID id.ClientID, agentID id.AgentID) {
	ids := r.byClientID[clientID]
	for i, v := range ids {
		if v == agentID {
			ids = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(ids) == 0 {
		delete(r.byClientID, clientID)
	} else {
		r.byClientID[clientID] = ids
	}
}

func clientIDPtrEqual(a, b *id.ClientID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
