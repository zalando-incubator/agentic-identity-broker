package storage

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionSet_ValidateForCreate_ScopeLessServiceScope(t *testing.T) {
	t.Parallel()

	for _, scopes := range [][]string{nil, {}} {
		ps := &PermissionSet{
			Name:        "Scope-less service",
			Description: "Requires an authenticated session without OAuth scopes",
			ServiceScopes: []ServiceScope{{
				ServiceID:       id.NewServiceID(),
				Scopes:          scopes,
				RequirementType: RequirementTypeMandatory,
			}},
		}

		require.NoError(t, ps.ValidateForCreate())
	}
}

func TestPermissionSet_ValidateForCreate_RejectsBlankSuppliedScope(t *testing.T) {
	ps := &PermissionSet{
		Name:        "Invalid scope",
		Description: "Contains a blank scope",
		ServiceScopes: []ServiceScope{{
			ServiceID:       id.NewServiceID(),
			Scopes:          []string{""},
			RequirementType: RequirementTypeOptional,
		}},
	}

	err := ps.ValidateForCreate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scope cannot be empty")
}

func TestPermissionSetCanonicalIDValidationAndCopy(t *testing.T) {
	canonicalID := "repository-read"
	permissionSet := &PermissionSet{
		ID:          id.NewPermissionSetID(),
		CanonicalID: &canonicalID,
		Name:        "Repository Read",
		Description: "Read repository data",
		ServiceScopes: []ServiceScope{{
			ServiceID:       id.NewServiceID(),
			RequirementType: RequirementTypeOptional,
		}},
	}
	require.NoError(t, permissionSet.Validate())

	copy := permissionSet.Copy()
	require.NotNil(t, copy.CanonicalID)
	*copy.CanonicalID = "other-permission-set"
	assert.Equal(t, canonicalID, *permissionSet.CanonicalID)
}

func TestPermissionSetObjectMetadata(t *testing.T) {
	metadata, err := model.NewObjectMetadata(map[string]string{"exposure": "internal"}, nil)
	require.NoError(t, err)
	ps := &PermissionSet{
		ID: id.NewPermissionSetID(), Name: "Read", Description: "Read",
		ServiceScopes:  []ServiceScope{{ServiceID: id.NewServiceID(), Scopes: []string{"read"}, RequirementType: RequirementTypeMandatory}},
		ObjectMetadata: metadata,
	}

	t.Run("survives Copy", func(t *testing.T) {
		assert.True(t, ps.Copy().ObjectMetadata.Equal(metadata))
	})
	t.Run("validation applies the re-check", func(t *testing.T) {
		invalid := ps.Copy()
		invalid.ObjectMetadata = model.RestoreObjectMetadata(nil, map[string]string{"k": "\x00"})
		assert.ErrorIs(t, invalid.Validate(), model.ErrMetadataAnnotationsInvalidValue)
		assert.ErrorIs(t, invalid.ValidateForCreate(), model.ErrMetadataAnnotationsInvalidValue)
		require.NoError(t, ps.Validate())
	})
}
