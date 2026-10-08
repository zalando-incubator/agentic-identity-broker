package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type dcrClientIdentity struct {
	issuerURI string
	clientID  id.ClientID
}

func providerDCRIdentity(entity *model.ThirdpartyOAuth2ProviderEntity) (dcrClientIdentity, bool) {
	if entity.Discovery.ClientMethod != model.ClientBootstrapDCR {
		return dcrClientIdentity{}, false
	}
	return dcrClientIdentity{issuerURI: entity.IssuerURI, clientID: entity.ClientID}, true
}

// InMemoryThirdpartyOAuth2ProviderRepository implements ports.ThirdpartyOAuth2ProviderRepository
// with thread-safe in-memory storage. Protected resources are stored as an
// authoritative child set and indexed globally by their normalized URI.
type InMemoryThirdpartyOAuth2ProviderRepository struct {
	mu                sync.RWMutex
	issuerSessionGate sync.RWMutex
	sessions          *InMemoryUserSessionRepository
	providers         map[id.ServiceID]*thirdpartyOAuth2ProviderRecord
	canonicalIndex    map[string]id.ServiceID
	dcrIndex          map[dcrClientIdentity]id.ServiceID
	resources         map[id.ServiceID]map[string]struct{}
	resourceOwners    map[string]id.ServiceID
}

var _ ports.ThirdpartyOAuth2ProviderRepository = (*InMemoryThirdpartyOAuth2ProviderRepository)(nil)
var _ ports.ThirdpartyOAuth2ProviderDiscoveryStatusWriter = (*InMemoryThirdpartyOAuth2ProviderRepository)(nil)

// NewInMemoryThirdpartyOAuth2ProviderRepository creates a new in-memory provider repository.
func NewInMemoryThirdpartyOAuth2ProviderRepository() *InMemoryThirdpartyOAuth2ProviderRepository {
	return &InMemoryThirdpartyOAuth2ProviderRepository{
		providers:      make(map[id.ServiceID]*thirdpartyOAuth2ProviderRecord),
		canonicalIndex: make(map[string]id.ServiceID),
		dcrIndex:       make(map[dcrClientIdentity]id.ServiceID),
		resources:      make(map[id.ServiceID]map[string]struct{}),
		resourceOwners: make(map[string]id.ServiceID),
	}
}

// WithUserSessionRepository binds issuer changes to session insertion for this adapter.
// Call it before using either repository concurrently.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) WithUserSessionRepository(sessions *InMemoryUserSessionRepository) *InMemoryThirdpartyOAuth2ProviderRepository {
	r.sessions = sessions
	if sessions != nil {
		sessions.provider = r
		sessions.issuerSessionGate = &r.issuerSessionGate
	}
	return r
}

// Create stores a new provider and its complete protected-resource set.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) Create(_ context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	if entity == nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "entity cannot be nil")
	}
	if entity.ID.IsZero() {
		return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "provider ID cannot be empty: caller must set ID before storing")
	}

	resources, err := normalizedResourceSet(entity.ProtectedResources)
	if err != nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "invalid protected resources")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[entity.ID]; exists {
		return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider with this ID already exists")
	}
	if entity.CanonicalID != nil {
		if _, exists := r.canonicalIndex[*entity.CanonicalID]; exists {
			return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider canonical_id already exists")
		}
	}
	if identity, isDCR := providerDCRIdentity(entity); isDCR {
		if _, exists := r.dcrIndex[identity]; exists {
			return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindConflict, storage.ErrDuplicateDCRClientIdentity, "provider DCR issuer and client_id already exist")
		}
	}

	for resource := range resources {
		if owner, claimed := r.resourceOwners[resource]; claimed && owner != entity.ID {
			return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "protected resource is already owned")
		}
	}

	entity.ProtectedResources = resourceSetSlice(resources)
	entity.Version = 1
	record, err := providerEntityToRecord(entity)
	if err != nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "failed to store provider")
	}
	r.providers[entity.ID] = record
	r.resources[entity.ID] = resources
	if entity.CanonicalID != nil {
		r.canonicalIndex[*entity.CanonicalID] = entity.ID
	}
	if identity, isDCR := providerDCRIdentity(record.entity); isDCR {
		r.dcrIndex[identity] = entity.ID
	}
	for resource := range resources {
		r.resourceOwners[resource] = entity.ID
	}
	return nil
}

// Get retrieves a provider with protected resources materialized from its child set.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, exists := r.providers[serviceID]
	if !exists {
		return nil, providerStorageError("GetThirdpartyOAuth2Provider", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	return providerRecordToEntity(record, r.resources[serviceID]), nil
}

func (r *InMemoryThirdpartyOAuth2ProviderRepository) GetByCanonicalID(_ context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	serviceID, exists := r.canonicalIndex[canonicalID]
	if !exists {
		return nil, providerStorageError("GetThirdpartyOAuth2ProviderByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	return providerRecordToEntity(r.providers[serviceID], r.resources[serviceID]), nil
}

func (r *InMemoryThirdpartyOAuth2ProviderRepository) GetCanonicalIDs(_ context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	canonicalIDs := make(map[id.ServiceID]string, len(ids))
	for _, serviceID := range ids {
		if record, exists := r.providers[serviceID]; exists && record.entity.CanonicalID != nil {
			canonicalIDs[serviceID] = *record.entity.CanonicalID
		}
	}
	return canonicalIDs, nil
}

// Update changes provider fields and increments its version. A nil expectedVersion
// preserves the current child resource set; a non-nil value replaces it atomically.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	if entity == nil {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "entity cannot be nil")
	}
	r.issuerSessionGate.Lock()
	defer r.issuerSessionGate.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.providers[entity.ID]
	if !exists {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if expectedVersion != nil && existing.entity.Version != *expectedVersion {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider version does not match")
	}
	// A failed refresh does not bump the version, so CAS alone cannot prevent an
	// earlier successful refresh from erasing its newer failure status.
	if entity.Discovery.ResourceURL != nil && entity.DiscoveryStatus.LastAttemptAt != nil {
		if lastAttempt := existing.entity.DiscoveryStatus.LastAttemptAt; lastAttempt != nil && entity.DiscoveryStatus.LastAttemptAt.Before(*lastAttempt) {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider discovery attempt is no longer current")
		}
	}
	if entity.ClearCanonicalID {
		entity.CanonicalID = nil
	} else if entity.CanonicalID == nil {
		entity.CanonicalID = existing.entity.CanonicalID
	}

	if entity.CanonicalID != nil {
		if existingID, exists := r.canonicalIndex[*entity.CanonicalID]; exists && existingID != entity.ID {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider canonical_id already exists")
		}
	}
	if identity, isDCR := providerDCRIdentity(entity); isDCR {
		if existingID, exists := r.dcrIndex[identity]; exists && existingID != entity.ID {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, storage.ErrDuplicateDCRClientIdentity, "provider DCR issuer and client_id already exist")
		}
	}

	resourceSet := r.resources[entity.ID]
	if expectedVersion != nil {
		var err error
		resourceSet, err = normalizedResourceSet(entity.ProtectedResources)
		if err != nil {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "invalid protected resources")
		}
		for resource := range resourceSet {
			if owner, claimed := r.resourceOwners[resource]; claimed && owner != entity.ID {
				return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "protected resource is already owned")
			}
		}
	}
	if entity.IssuerURI != existing.entity.IssuerURI && (existing.entity.Discovery.ResourceURL != nil || entity.Discovery.ResourceURL != nil) {
		if r.sessions == nil {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider session state is unavailable")
		}
		count, err := r.sessions.CountByService(ctx, entity.ID)
		if err != nil {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindUnknown, err, "failed to check provider sessions")
		}
		if count != 0 {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, storage.ErrIssuerChangeHasSessions, "provider has user sessions")
		}
	}

	entity.CreatedAt = existing.entity.CreatedAt
	entity.Version = existing.entity.Version + 1
	if entity.AuthorizationParams == nil {
		entity.AuthorizationParams = existing.entity.Copy().AuthorizationParams
	}
	entity.ProtectedResources = resourceSetSlice(resourceSet)
	record, err := providerEntityToRecord(entity)
	if err != nil {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "failed to store provider")
	}

	if expectedVersion != nil {
		for resource := range r.resources[entity.ID] {
			delete(r.resourceOwners, resource)
		}
		r.resources[entity.ID] = resourceSet
		for resource := range resourceSet {
			r.resourceOwners[resource] = entity.ID
		}
	}
	r.providers[entity.ID] = record
	if identity, isDCR := providerDCRIdentity(existing.entity); isDCR {
		delete(r.dcrIndex, identity)
	}
	if identity, isDCR := providerDCRIdentity(record.entity); isDCR {
		r.dcrIndex[identity] = entity.ID
	}
	if existing.entity.CanonicalID != nil {
		delete(r.canonicalIndex, *existing.entity.CanonicalID)
	}
	if entity.CanonicalID != nil {
		r.canonicalIndex[*entity.CanonicalID] = entity.ID
	}
	return nil
}

// Delete removes the provider and releases every resource it owns.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	r.issuerSessionGate.Lock()
	defer r.issuerSessionGate.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[serviceID]; exists && r.sessions != nil {
		count, err := r.sessions.CountByService(ctx, serviceID)
		if err != nil {
			return providerStorageError("DeleteThirdpartyOAuth2Provider", storage.ErrorKindUnknown, err, "failed to check provider sessions")
		}
		if count != 0 {
			return providerStorageError("DeleteThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider has user sessions")
		}
	}
	for resource := range r.resources[serviceID] {
		delete(r.resourceOwners, resource)
	}
	if record, exists := r.providers[serviceID]; exists && record.entity.CanonicalID != nil {
		delete(r.canonicalIndex, *record.entity.CanonicalID)
	}
	if record, exists := r.providers[serviceID]; exists {
		if identity, isDCR := providerDCRIdentity(record.entity); isDCR {
			delete(r.dcrIndex, identity)
		}
	}
	delete(r.resources, serviceID)
	delete(r.providers, serviceID)
	return nil
}

// List retrieves all providers with resources materialized from child sets.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) List(_ context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*model.ThirdpartyOAuth2ProviderEntity, 0, len(r.providers))
	for serviceID, record := range r.providers {
		result = append(result, providerRecordToEntity(record, r.resources[serviceID]))
	}
	return result, nil
}

// FindByProtectedResource resolves a normalized resource through the authoritative owner index.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	select {
	case <-ctx.Done():
		return nil, providerStorageError("FindByProtectedResource", storage.ErrorKindTimeout, ctx.Err(), "context cancelled or timeout")
	default:
	}
	if resourceURI == "" {
		return nil, tokenexchange.NewInvalidTargetError("resource parameter cannot be empty")
	}
	resource, err := model.NormalizeAndValidateProtectedResource(resourceURI)
	if err != nil {
		return nil, tokenexchange.NewInvalidTargetError("resource parameter is invalid")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	owner, found := r.resourceOwners[resource]
	if !found {
		return nil, tokenexchange.NewInvalidTargetError("no service configured for the requested resource")
	}
	return providerRecordToEntity(r.providers[owner], r.resources[owner]), nil
}

// AddProtectedResource atomically claims a normalized URI for a provider.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) AddProtectedResource(_ context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	resource, err := model.NormalizeAndValidateProtectedResource(resourceURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("AddProtectedResource", storage.ErrorKindValidation, err, "invalid protected resource")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.providers[serviceID]
	if !exists {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("AddProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if owner, claimed := r.resourceOwners[resource]; claimed {
		if owner != serviceID {
			return ports.ProtectedResourceMutationResult{}, providerStorageError("AddProtectedResource", storage.ErrorKindConflict, nil, "protected resource is already owned")
		}
		return r.mutationResultLocked(serviceID, resource, false), nil
	}
	r.resources[serviceID][resource] = struct{}{}
	r.resourceOwners[resource] = serviceID
	record.entity.Version++
	return r.mutationResultLocked(serviceID, resource, true), nil
}

// RemoveProtectedResource atomically releases a normalized URI from its owner.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) RemoveProtectedResource(_ context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	resource, err := model.NormalizeAndValidateProtectedResource(resourceURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RemoveProtectedResource", storage.ErrorKindValidation, err, "invalid protected resource")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.providers[serviceID]
	if !exists {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RemoveProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if owner, found := r.resourceOwners[resource]; !found || owner != serviceID {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RemoveProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "protected resource not found")
	}
	delete(r.resources[serviceID], resource)
	delete(r.resourceOwners, resource)
	record.entity.Version++
	return r.mutationResultLocked(serviceID, resource, true), nil
}

// RenameProtectedResource atomically replaces one normalized URI with another.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) RenameProtectedResource(_ context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	from, err := model.NormalizeAndValidateProtectedResource(fromURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RenameProtectedResource", storage.ErrorKindValidation, err, "invalid source protected resource")
	}
	to, err := model.NormalizeAndValidateProtectedResource(toURI)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RenameProtectedResource", storage.ErrorKindValidation, err, "invalid target protected resource")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, exists := r.providers[serviceID]
	if !exists {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RenameProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if owner, found := r.resourceOwners[from]; !found || owner != serviceID {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RenameProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "protected resource not found")
	}
	if from == to {
		return r.mutationResultLocked(serviceID, to, false), nil
	}
	if _, claimed := r.resourceOwners[to]; claimed {
		return ports.ProtectedResourceMutationResult{}, providerStorageError("RenameProtectedResource", storage.ErrorKindConflict, nil, "protected resource is already owned")
	}
	delete(r.resources[serviceID], from)
	delete(r.resourceOwners, from)
	r.resources[serviceID][to] = struct{}{}
	r.resourceOwners[to] = serviceID
	record.entity.Version++
	return r.mutationResultLocked(serviceID, to, true), nil
}

// ListProtectedResources returns a copy of the normalized child set and its version.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) ListProtectedResources(_ context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, exists := r.providers[serviceID]
	if !exists {
		return nil, 0, providerStorageError("ListProtectedResources", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	return resourceSetSlice(r.resources[serviceID]), record.entity.Version, nil
}

func (r *InMemoryThirdpartyOAuth2ProviderRepository) mutationResultLocked(serviceID id.ServiceID, resource string, changed bool) ports.ProtectedResourceMutationResult {
	return ports.ProtectedResourceMutationResult{
		Resource:           resource,
		ProtectedResources: resourceSetSlice(r.resources[serviceID]),
		Version:            r.providers[serviceID].entity.Version,
		Changed:            changed,
	}
}

func normalizedResourceSet(resources []string) (map[string]struct{}, error) {
	resourceSet := make(map[string]struct{}, len(resources))
	for _, resourceURI := range resources {
		resource, err := model.NormalizeAndValidateProtectedResource(resourceURI)
		if err != nil {
			return nil, err
		}
		if _, duplicate := resourceSet[resource]; duplicate {
			return nil, fmt.Errorf("duplicate protected resource: %s", resource)
		}
		resourceSet[resource] = struct{}{}
	}
	return resourceSet, nil
}

func providerStorageError(operation string, kind storage.ErrorKind, cause error, message string) error {
	return storage.NewStorageError(operation, kind, cause, message)
}

// RecordDiscoveryFailure changes only the status of a still-current discovery-backed provider.
func (r *InMemoryThirdpartyOAuth2ProviderRepository) RecordDiscoveryFailure(_ context.Context, serviceID id.ServiceID, expectedVersion int64, completedAt time.Time, failureCode string) error {
	const operation = "RecordDiscoveryFailure"
	if !model.ValidDiscoveryFailureCode(failureCode) {
		return providerStorageError(operation, storage.ErrorKindValidation, nil, "invalid discovery failure code")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	record, exists := r.providers[serviceID]
	if !exists {
		return providerStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	entity := record.entity
	if entity.Version != expectedVersion || entity.Discovery.ResourceURL == nil {
		return providerStorageError(operation, storage.ErrorKindConflict, nil, "provider discovery state is no longer current")
	}
	status := &entity.DiscoveryStatus
	if status.LastAttemptAt == nil || status.LastSuccessAt == nil ||
		!completedAt.After(*status.LastAttemptAt) || !completedAt.After(*status.LastSuccessAt) {
		return providerStorageError(operation, storage.ErrorKindConflict, nil, "provider discovery attempt is no longer current")
	}

	status.LastAttemptAt = &completedAt
	status.FailureReason = &failureCode
	return nil
}
