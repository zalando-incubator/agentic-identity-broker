package memory

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/canonical"
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

	if err := entity.TokenEndpointAuthMethod.Validate(); err != nil {
		return nil, err
	}
	source := entity.CredentialSource
	if source == "" && !entity.CredentialSourceProvided {
		source = model.CredentialSourceStored
	}
	switch source {
	case model.CredentialSourceStored:
		if entity.ClientID.IsZero() {
			return nil, errors.New("client_id is required for stored credential_source")
		}
		if (entity.IsPublicClient() || entity.IsCIMDConfidentialClient()) != entity.Secret.IsAbsent() {
			return nil, errors.New("token_endpoint_auth_method and client_secret state must agree")
		}
		if !entity.IsPublicClient() && !entity.IsCIMDConfidentialClient() {
			if _, err := entity.Secret.GetCiphertext(); err != nil {
				return nil, fmt.Errorf("entity secret must be encrypted with non-empty ciphertext: %w", err)
			}
		}
	case model.CredentialSourceFilesystem:
		if !entity.TokenEndpointAuthMethod.IsAbsent() || (entity.Flavor != "" && entity.Flavor != model.OAuth2FlavorStandard && entity.Flavor != model.OAuth2FlavorGitHub) {
			return nil, errors.New("filesystem credential_source requires standard or github shared-secret authentication")
		}
		if entity.CanonicalID == nil || entity.ClearCanonicalID {
			return nil, errors.New("canonical_id is required for filesystem credential_source")
		}
		if err := canonical.Validate(entity.CanonicalID); err != nil {
			return nil, err
		}
		if entity.ClientIDProvided || entity.ClientSecretProvided || !entity.ClientID.IsZero() || !entity.Secret.IsAbsent() {
			return nil, errors.New("inline credentials must be absent for filesystem credential_source")
		}
	default:
		return nil, errors.New("credential_source must be stored or filesystem")
	}

	copy := entity.Copy()
	copy.CredentialSource = source
	copy.ClearCanonicalID = false
	copy.CredentialSourceProvided = false
	copy.ClientIDProvided = false
	copy.ClientSecretProvided = false
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
