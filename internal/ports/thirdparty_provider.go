// Package ports defines interfaces for hexagonal architecture boundaries.
package ports

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// ThirdpartyOAuth2ProviderRepository defines storage operations for third-party OAuth2
// provider entities. Stored entities carry Secret in encrypted state or an explicitly
// absent state for public clients.
//
// IMPORTANT CONTRACT (Encryption Invariant):
//   - Input (Create/Update): Entity must have an encrypted or absent Secret
//   - Output (Get/List/Find): Entity has an encrypted or absent Secret; the domain service decrypts encrypted values
//   - The repository is unaware of encryption mechanics; it treats Secret as opaque ciphertext or explicit absence
//
// All third-party OAuth2 provider storage operations use ThirdpartyOAuth2ProviderEntity.

// ProtectedResourceMutationResult is the atomic post-mutation resource state.
// Version is the strong ETag value for the provider after the operation. Changed
// is false only for successful idempotent or no-op mutations.
type ProtectedResourceMutationResult struct {
	Resource           string
	ProtectedResources []string
	Version            int64
	Changed            bool
}
type ThirdpartyOAuth2ProviderRepository interface {
	Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error
	Get(ctx context.Context, id id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
	Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error
	Delete(ctx context.Context, id id.ServiceID) error
	List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error)
	FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error)
	AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ProtectedResourceMutationResult, error)
	RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ProtectedResourceMutationResult, error)
	RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ProtectedResourceMutationResult, error)
	ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error)
}

// ThirdpartyOAuth2ProviderCanonicalIDRepository resolves and batch-loads type-scoped canonical service IDs.
type ThirdpartyOAuth2ProviderCanonicalIDRepository interface {
	GetByCanonicalID(ctx context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error)
	GetCanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error)
}
