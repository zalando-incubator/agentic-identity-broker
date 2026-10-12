package permissionset

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// psCacheEntry represents a cached permission set with expiration.
type psCacheEntry struct {
	ps        *storage.PermissionSet
	expiresAt time.Time
}

// Service provides business logic for permission set operations.
// Includes TTL caching, validation, and deletion protection.
type Service struct {
	repo      ports.PermissionSetRepository
	grantRepo ports.UserGrantRepository
	logger    *slog.Logger

	mu    sync.RWMutex
	cache map[id.PermissionSetID]psCacheEntry

	cacheTTL  time.Duration
	stopChan  chan struct{}
	closeOnce sync.Once
}

const defaultCacheTTL = 60 * time.Second

// NewPermissionSetService creates a new PermissionSetService with TTL caching.
// Background eviction goroutine is started automatically.
func NewPermissionSetService(repo ports.PermissionSetRepository, grantRepo ports.UserGrantRepository, logger *slog.Logger) *Service {
	s := &Service{
		repo:      repo,
		grantRepo: grantRepo,
		logger:    logger,
		cache:     make(map[id.PermissionSetID]psCacheEntry),
		cacheTTL:  defaultCacheTTL,
		stopChan:  make(chan struct{}),
	}

	// Start background eviction goroutine
	go s.evictionLoop()

	return s
}

// evictionLoop periodically evicts expired cache entries.
func (s *Service) evictionLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.evictExpired()
		case <-s.stopChan:
			return
		}
	}
}

// evictExpired removes expired entries from the cache.
func (s *Service) evictExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for id, entry := range s.cache {
		if now.After(entry.expiresAt) {
			delete(s.cache, id)
		}
	}
}

// Close stops the background eviction goroutine. Safe to call multiple times.
func (s *Service) Close() {
	s.closeOnce.Do(func() { close(s.stopChan) })
}

// ResolveID accepts a UUID or a type-scoped canonical ID.
func (s *Service) ResolveID(ctx context.Context, value string) (id.PermissionSetID, error) {
	if parsed, err := id.ParsePermissionSetID(value); err == nil {
		return parsed, nil
	}
	repo, ok := s.repo.(ports.PermissionSetCanonicalIDRepository)
	if !ok {
		return id.PermissionSetID{}, storage.NewStorageError("ResolvePermissionSetID", storage.ErrorKindNotFound, nil, "permission set not found")
	}
	permissionSet, err := repo.GetByCanonicalID(ctx, value)
	if err != nil {
		return id.PermissionSetID{}, err
	}
	return permissionSet.ID, nil
}

// CanonicalIDs returns canonical IDs for the requested permission sets.
func (s *Service) CanonicalIDs(ctx context.Context, ids []id.PermissionSetID) (map[id.PermissionSetID]string, error) {
	if repo, ok := s.repo.(ports.PermissionSetCanonicalIDRepository); ok {
		return repo.GetCanonicalIDs(ctx, ids)
	}
	return map[id.PermissionSetID]string{}, nil
}

// Create stores a new permission set.
// Emits a structured audit log entry on success.
func (s *Service) Create(ctx context.Context, ps *storage.PermissionSet) error {
	if err := ps.ValidateForCreate(); err != nil {
		return err
	}

	// Generate ID if not provided
	if ps.ID.IsZero() {
		ps.ID = id.NewPermissionSetID()
	}

	if err := s.repo.Create(ctx, ps); err != nil {
		return err
	}

	definition := (&storage.PermissionSet{ServiceScopes: ps.ServiceScopes}).Copy()
	s.logger.InfoContext(ctx, "PermissionSetCreated",
		"action", "permission_set_created",
		"permission_set_id", ps.ID,
		"service_scopes", definition.ServiceScopes)

	// Invalidate cache for this permission set
	s.mu.Lock()
	delete(s.cache, ps.ID)
	s.mu.Unlock()

	return nil
}

// Get retrieves a permission set by ID.
// Results are cached with TTL.
func (s *Service) Get(ctx context.Context, psID id.PermissionSetID) (*storage.PermissionSet, error) {
	// Check cache first
	s.mu.RLock()
	if entry, exists := s.cache[psID]; exists && time.Now().Before(entry.expiresAt) {
		defer s.mu.RUnlock()
		return entry.ps.Copy(), nil
	}
	s.mu.RUnlock()

	// Fetch from repo
	ps, err := s.repo.Get(ctx, psID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	s.mu.Lock()
	s.cache[psID] = psCacheEntry{
		ps:        ps,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
	s.mu.Unlock()

	return ps.Copy(), nil
}

// GetByIDs retrieves multiple permission sets by IDs.
// Results are cached with TTL. Missing IDs are silently absent (caller validates).
func (s *Service) GetByIDs(ctx context.Context, ids []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	// Snapshot valid cache entries and collect IDs that need a repo fetch.
	s.mu.RLock()
	now := time.Now()
	cached := make(map[id.PermissionSetID]*storage.PermissionSet, len(ids))
	uncachedIDs := make([]id.PermissionSetID, 0)
	for _, psID := range ids {
		if entry, exists := s.cache[psID]; exists && now.Before(entry.expiresAt) {
			cached[psID] = entry.ps.Copy()
		} else {
			uncachedIDs = append(uncachedIDs, psID)
		}
	}
	s.mu.RUnlock()

	// Fetch uncached IDs from repo and update cache.
	if len(uncachedIDs) > 0 {
		repoResults, err := s.repo.GetByIDs(ctx, uncachedIDs)
		if err != nil {
			return nil, err
		}
		s.mu.Lock()
		for _, ps := range repoResults {
			s.cache[ps.ID] = psCacheEntry{ps: ps, expiresAt: time.Now().Add(s.cacheTTL)}
			cached[ps.ID] = ps.Copy()
		}
		s.mu.Unlock()
	}

	// Assemble result in original request order.
	result := make([]*storage.PermissionSet, 0, len(ids))
	for _, psID := range ids {
		if ps, ok := cached[psID]; ok {
			result = append(result, ps)
		}
	}
	return result, nil
}

// Update updates an existing permission set.
// Emits a structured audit log entry on success.
// Invalidates cache entry.
func (s *Service) Update(ctx context.Context, ps *storage.PermissionSet) error {
	if err := ps.Validate(); err != nil {
		return err
	}

	existing, err := s.repo.Get(ctx, ps.ID)
	if err != nil {
		return err
	}
	previous := (&storage.PermissionSet{
		ServiceScopes: existing.ServiceScopes, UpdatedAt: existing.UpdatedAt,
	}).Copy()

	if err := s.repo.Update(ctx, ps); err != nil {
		return err
	}

	definition := (&storage.PermissionSet{ServiceScopes: ps.ServiceScopes}).Copy()
	s.logger.InfoContext(ctx, "PermissionSetUpdated",
		"action", "permission_set_updated",
		"permission_set_id", ps.ID,
		"service_scopes", definition.ServiceScopes,
		"previous_observed_service_scopes", previous.ServiceScopes,
		"previous_observed_updated_at", previous.UpdatedAt.UTC().Format(time.RFC3339Nano))

	// Invalidate cache for this permission set
	s.mu.Lock()
	delete(s.cache, ps.ID)
	s.mu.Unlock()

	return nil
}

// Delete removes a permission set by ID.
// Returns a conflict error if agents or user grants reference this permission set.
// Emits a structured audit log entry on success.
func (s *Service) Delete(ctx context.Context, psID id.PermissionSetID) error {
	// The agent and grant reference checks below are advisory: they surface a clear error
	// message before hitting the database. The true atomicity guarantee lives in the
	// repository's serializable transaction, which enforces the same constraints at the
	// database level and prevents a TOCTOU race between check and delete.
	// These counts use a regular (non-serializable) read, so a concurrent creation
	// between the advisory check and the repo-level check may cause the advisory count
	// to differ from what the serializable transaction sees. The advisory counts are
	// informational only; the caller should treat the repo-level conflict error as
	// authoritative if the advisory checks pass but Delete still fails.
	agentCount, err := s.repo.CountAgentsReferencingPermissionSet(ctx, psID)
	if err != nil {
		return fmt.Errorf("failed to check agent references: %w", err)
	}
	if agentCount > 0 {
		return storage.NewStorageError(
			"DeletePermissionSet",
			storage.ErrorKindConflict,
			nil,
			fmt.Sprintf("cannot delete permission set: %d agent(s) reference it", agentCount),
		)
	}

	grantCount, err := s.grantRepo.CountGrantsReferencingPermissionSet(ctx, psID)
	if err != nil {
		return fmt.Errorf("failed to check grant references: %w", err)
	}
	if grantCount > 0 {
		return storage.NewStorageError(
			"DeletePermissionSet",
			storage.ErrorKindConflict,
			nil,
			fmt.Sprintf("cannot delete permission set: %d user grant(s) reference it", grantCount),
		)
	}

	deleted, err := s.repo.Delete(ctx, psID)
	if err != nil {
		return err
	}

	if deleted {
		s.logger.InfoContext(ctx, "PermissionSetDeleted",
			"action", "permission_set_deleted",
			"permission_set_id", psID)
	}

	// Invalidate cache for this permission set
	s.mu.Lock()
	delete(s.cache, psID)
	s.mu.Unlock()

	return nil
}

// List returns all permission sets, optionally filtered by service ID.
// Results are NOT cached.
func (s *Service) List(ctx context.Context, serviceID id.ServiceID) ([]*storage.PermissionSet, error) {
	return s.repo.List(ctx, serviceID)
}

// ValidateIDs checks that all permission set IDs exist.
// Returns an error listing missing IDs if any are not found.
func (s *Service) ValidateIDs(ctx context.Context, ids []id.PermissionSetID) error {
	if len(ids) == 0 {
		return nil
	}

	// Fetch all permission sets
	results, err := s.GetByIDs(ctx, ids)
	if err != nil {
		return err
	}

	// Find missing IDs
	found := make(map[id.PermissionSetID]bool)
	for _, ps := range results {
		found[ps.ID] = true
	}

	var missing []id.PermissionSetID
	for _, id := range ids {
		if !found[id] {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("validation failed: missing permission set IDs: %v", missing)
	}

	return nil
}
