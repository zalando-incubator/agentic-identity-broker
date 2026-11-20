package memory

import (
	"context"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"math"
)

var _ ports.ThirdpartyOAuth2ClientIdentityRepository = (*InMemoryThirdpartyOAuth2ProviderRepository)(nil)

func (r *InMemoryThirdpartyOAuth2ProviderRepository) RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, observed id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
	const operation = "RecordObservedClientIdentity"
	if observed.IsZero() {
		return ports.ClientIdentityState{}, false, false, providerStorageError(operation, storage.ErrorKindValidation, nil, "observed identity cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return ports.ClientIdentityState{}, false, false, providerStorageError(operation, storage.ErrorKindTimeout, err, "observation canceled")
	}
	record, exists := r.providers[serviceID]
	if !exists {
		return ports.ClientIdentityState{}, false, false, providerStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	entity := record.entity
	state := ports.ClientIdentityState{Identity: entity.Copy().ClientIdentity, Revision: entity.ClientIdentityRevision}
	if entity.CredentialSource != model.CredentialSourceFilesystem || entity.Version != expected.ServiceVersion || entity.CanonicalID == nil || *entity.CanonicalID != expected.CanonicalID || entity.ClientIdentityRevision != expected.Revision {
		return state, false, false, nil
	}
	changed := entity.ClientIdentity != nil && *entity.ClientIdentity != observed
	if changed && state.Revision == math.MaxInt64 {
		return ports.ClientIdentityState{}, false, false, providerStorageError(operation, storage.ErrorKindValidation, nil, "client identity revision exhausted")
	}
	if changed {
		entity.ClientIdentityRevision++
	}
	entity.ClientIdentity = new(observed)
	return ports.ClientIdentityState{Identity: new(observed), Revision: entity.ClientIdentityRevision}, true, changed, nil
}
