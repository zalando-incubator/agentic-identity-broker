package memory

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// thirdpartyOAuth2ProviderRecord holds provider data independently from its
// protected-resource children. The child set is authoritative; this record never
// retains a ProtectedResources slice.
type thirdpartyOAuth2ProviderRecord struct {
	entity *model.ThirdpartyOAuth2ProviderEntity
}

// providerEntityCopy creates a deep copy for internal storage and verifies that
// the client-authentication state satisfies the persistence invariant.
func providerEntityCopy(entity *model.ThirdpartyOAuth2ProviderEntity) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if entity == nil {
		return nil, errors.New("entity cannot be nil")
	}

	if err := entity.ValidateStoredClientAuthentication(); err != nil {
		return nil, err
	}

	copy := entity.Copy()
	if entity.Secret.IsAbsent() {
		copy.Secret = model.NewAbsentSecret()
	} else {
		ciphertext, err := entity.Secret.GetCiphertext()
		if err != nil {
			return nil, fmt.Errorf("entity secret must be encrypted with non-empty ciphertext: %w", err)
		}
		copy.Secret = model.NewEncryptedSecret(ciphertext)
	}
	return copy, nil
}

func providerEntityToRecord(entity *model.ThirdpartyOAuth2ProviderEntity) (*thirdpartyOAuth2ProviderRecord, error) {
	copy, err := providerEntityCopy(entity)
	if err != nil {
		return nil, err
	}
	copy.ProtectedResources = nil
	return &thirdpartyOAuth2ProviderRecord{entity: copy}, nil
}

func providerRecordToEntity(record *thirdpartyOAuth2ProviderRecord, resourceSet map[string]struct{}) *model.ThirdpartyOAuth2ProviderEntity {
	if record == nil || record.entity == nil {
		return nil
	}

	entity := record.entity.Copy()
	entity.ProtectedResources = resourceSetSlice(resourceSet)
	return entity
}

func resourceSetSlice(resourceSet map[string]struct{}) []string {
	resources := slices.Sorted(maps.Keys(resourceSet))
	if resources == nil {
		return []string{}
	}
	return resources
}
